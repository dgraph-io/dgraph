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
	"sync/atomic"
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

var limiter rateLimiter

func init() {
	go limiter.bleed()
}

type rateLimiter struct {
	iou int32
}

// Instead of using the time/rate package, we use this simple one, because that
// allows a certain number of ops per second, without taking any feedback into
// account. We however, limit solely based on feedback, allowing a certain
// number of ops to remain pending, and not anymore.
func (rl *rateLimiter) bleed() {
	tick := time.NewTicker(time.Second)
	defer tick.Stop()

	for range tick.C {
		if atomic.AddInt32(&rl.iou, -1) >= 0 {
			<-pendingProposals
			x.PendingProposals.Add(-1)
		} else {
			atomic.AddInt32(&rl.iou, 1)
		}
	}
}

func (rl *rateLimiter) incr(ctx context.Context, retry int) error {
	// Let's not wait here via time.Sleep or similar. Let pendingProposals
	// channel do its natural rate limiting.
	weight := 1 << uint(retry) // Use an exponentially increasing weight.
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
func (rl *rateLimiter) decr(retry int) {
	if retry == 0 {
		<-pendingProposals
		x.PendingProposals.Add(-1)
		return
	}
	weight := 1 << uint(retry) // Ensure that the weight calculation is a copy of incr.
	atomic.AddInt32(&rl.iou, int32(weight))
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

	// Set this to disable retrying mechanism, and using the user-specified
	// timeout.
	var noTimeout bool

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
			noTimeout = true
		}
	}

	// Let's keep the same key, so multiple retries of the same proposal would
	// have this shared key. Thus, each server in the group can identify
	// whether it has already done this work, and if so, skip it.
	key := uniqueKey()
	proposal.Key = key

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

	// Some proposals, like schema updates can take a long time to apply. Let's
	// not do the retry mechanism on them. Instead, we can set a long timeout of
	// 20 minutes.
	if noTimeout {
		return propose(20 * time.Minute)
	}
	// Some proposals can be stuck if leader change happens. For e.g. MsgProp message from follower
	// to leader can be dropped/end up appearing with empty Data in CommittedEntries.
	// Having a timeout here prevents the mutation being stuck forever in case they don't have a
	// timeout. We should always try with a timeout and optionally retry.
	//
	// Let's try 3 times before giving up.

	for i := 0; i < 3; i++ {
		// Each retry creates a new proposal, which adds to the number of pending proposals. We
		// should consider this into account, when adding new proposals to the system.
		if err := limiter.incr(ctx, i); err != nil {
			return err
		}
		defer limiter.decr(i)

		if err := propose(newTimeout(i)); err != errInternalRetry {
			return err
		}
	}
	return errUnableToServe
}
