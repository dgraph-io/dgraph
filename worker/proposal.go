/*
 * Copyright 2018 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package worker

import (
	"errors"
	"time"

	"github.com/dgraph-io/dgraph/conn"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/x"
	"golang.org/x/net/context"
	"golang.org/x/net/trace"
)

const baseTimeout time.Duration = 4 * time.Second

func newTimeout(retry int) time.Duration {
	timeout := baseTimeout
	for i := 0; i < retry; i++ {
		timeout *= 2
	}
	return timeout
}

func incrLimit(ctx context.Context, retry int) error {
	if retry > 0 {
		timeout := newTimeout(retry)
		t := time.NewTimer(timeout)
		select {
		case <-t.C:
		case <-ctx.Done():
			t.Stop()
			return ctx.Err()
		}
	}
	weight := 1 << uint(retry)
	for i := 0; i < weight; i++ {
		select {
		case pendingProposals <- struct{}{}:
			x.PendingProposals.Add(1)
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

// Done would slowly bleed the retries out.
func decrLimit(retry int) {
	weight := 1 << uint(retry)
	for i := 0; i < weight; i++ {
		<-pendingProposals
		x.PendingProposals.Add(-1)
	}
}

var errInternalRetry = errors.New("Retry Raft proposal internally")
var errUnableToServe = errors.New("Server unavailable. Please retry later")

// proposeAndWait sends a proposal through RAFT. It waits on a channel for the proposal
// to be applied(written to WAL) to all the nodes in the group.
func (n *node) proposeAndWait(ctx context.Context, proposal *pb.Proposal) error {
	if n.Raft() == nil {
		return x.Errorf("Raft isn't initialized yet")
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Do a type check here if schema is present
	// In very rare cases invalid entries might pass through raft, which would
	// be persisted, we do best effort schema check while writing
	if proposal.Mutations != nil {
		for _, edge := range proposal.Mutations.Edges {
			if tablet := groups().Tablet(edge.Attr); tablet != nil && tablet.ReadOnly {
				return errPredicateMoving
			} else if tablet.GroupId != groups().groupId() {
				// Tablet can move by the time request reaches here.
				return errUnservedTablet
			}

			su, ok := schema.State().Get(edge.Attr)
			if !ok {
				continue
			} else if err := ValidateAndConvert(edge, &su); err != nil {
				return err
			}
		}
		for _, schema := range proposal.Mutations.Schema {
			if tablet := groups().Tablet(schema.Predicate); tablet != nil && tablet.ReadOnly {
				return errPredicateMoving
			}
			if err := checkSchema(schema); err != nil {
				return err
			}
		}
	}

	// Let's keep the same key, so multiple retries of the same proposal would
	// have this shared key. Thus, each server in the group can identify
	// whether it has already done this work, and if so, skip it.
	key := uniqueKey()

	propose := func(timeout time.Duration) error {
		cctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

		che := make(chan error, 1)
		pctx := &conn.ProposalCtx{
			Ch:  che,
			Ctx: cctx,
		}
		x.AssertTruef(n.Proposals.Store(key, pctx), "Found existing proposal with key: [%v]", key)
		defer n.Proposals.Delete(key) // Ensure that it gets deleted on return.
		proposal.Key = key

		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf("Proposing data with key: %s. Timeout: %v", key, timeout)
		}

		data, err := proposal.Marshal()
		if err != nil {
			return err
		}

		if err = n.Raft().Propose(cctx, data); err != nil {
			return x.Wrapf(err, "While proposing")
		}
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf("Waiting for the proposal.")
		}

		select {
		case err = <-che:
			// We arrived here by a call to n.Proposals.Done().
			if tr, ok := trace.FromContext(ctx); ok {
				tr.LazyPrintf("Done with error: %v", err)
			}
			return err
		case <-ctx.Done():
			if tr, ok := trace.FromContext(ctx); ok {
				tr.LazyPrintf("External context timed out with error: %v.", ctx.Err())
			}
			return ctx.Err()
		case <-cctx.Done():
			if tr, ok := trace.FromContext(ctx); ok {
				tr.LazyPrintf("Internal context timed out with error: %v. Retrying...", cctx.Err())
			}
			return errInternalRetry
		}
	}

	// Some proposals can be stuck if leader change happens. For e.g. MsgProp message from follower
	// to leader can be dropped/end up appearing with empty Data in CommittedEntries.
	// Having a timeout here prevents the mutation being stuck forever in case they don't have a
	// timeout. We should always try with a timeout and optionally retry.
	//
	// Let's only try for 2 minutes, before giving up.
	limit := time.Now().Add(2 * time.Minute)
	for i := 0; ; i++ {
		// Each retry creates a new proposal, which adds to the number of pending proposals. We
		// should consider this into account, when adding new proposals to the system.
		if err := incrLimit(ctx, i); err != nil {
			return err
		}
		defer decrLimit(i)

		// The below algorithm would run proposal with a calculated timeout. If
		// it doesn't succeed or fail, it would block for the timeout duration.
		// Then it would double the timeout.
		if time.Now().After(limit) {
			return errUnableToServe
		}
		err := propose(newTimeout(i))
		if err != errInternalRetry {
			return err
		}
	}
}
