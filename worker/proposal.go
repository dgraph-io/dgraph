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
	"fmt"
	"sync/atomic"
	"time"

	"github.com/dgraph-io/dgraph/conn"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/x"

	ostats "go.opencensus.io/stats"
	tag "go.opencensus.io/tag"
	otrace "go.opencensus.io/trace"

	"github.com/pkg/errors"
	"golang.org/x/net/context"
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
	ctx := context.Background()

	tick := time.NewTicker(time.Second)
	defer tick.Stop()

	for range tick.C {
		if atomic.AddInt32(&rl.iou, -1) >= 0 {
			<-pendingProposals
			ostats.Record(ctx, x.PendingProposals.M(-1))
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
			ostats.Record(context.Background(), x.PendingProposals.M(1))
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
		ostats.Record(context.Background(), x.PendingProposals.M(-1))
		return
	}
	weight := 1 << uint(retry) // Ensure that the weight calculation is a copy of incr.
	atomic.AddInt32(&rl.iou, int32(weight))
}

// uniqueKey is meant to be unique across all the replicas.
func uniqueKey() string {
	return fmt.Sprintf("%02d-%d", groups().Node.Id, groups().Node.Rand.Uint64())
}

var errInternalRetry = errors.New("Retry Raft proposal internally")
var errUnableToServe = errors.New("Server overloaded with pending proposals. Please retry later")

// proposeAndWait sends a proposal through RAFT. It waits on a channel for the proposal
// to be applied(written to WAL) to all the nodes in the group.
func (n *node) proposeAndWait(ctx context.Context, proposal *pb.Proposal) (perr error) {
	startTime := time.Now()
	ctx = x.WithMethod(ctx, "n.proposeAndWait")
	defer func() {
		v := x.TagValueStatusOK
		if perr != nil {
			v = x.TagValueStatusError
		}
		ctx, _ = tag.New(ctx, tag.Upsert(x.KeyStatus, v))
		timeMs := x.SinceMs(startTime)
		ostats.Record(ctx, x.LatencyMs.M(timeMs))
	}()

	if n.Raft() == nil {
		return errors.Errorf("Raft isn't initialized yet")
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	// Set this to disable retrying mechanism, and using the user-specified
	// timeout.
	var noTimeout bool

	checkTablet := func(pred string) error {
		if tablet, err := groups().Tablet(pred); err != nil {
			return err
		} else if tablet == nil || tablet.GroupId == 0 {
			return errNonExistentTablet
		} else if tablet.GroupId != groups().groupId() {
			return errUnservedTablet
		}
		return nil
	}

	// Do a type check here if schema is present
	// In very rare cases invalid entries might pass through raft, which would
	// be persisted, we do best effort schema check while writing
	if proposal.Mutations != nil {
		for _, edge := range proposal.Mutations.Edges {
			if err := checkTablet(edge.Attr); err != nil {
				return err
			}
			su, ok := schema.State().Get(edge.Attr)
			if !ok {
				continue
			} else if err := ValidateAndConvert(edge, &su); err != nil {
				return err
			}
		}
		for _, schema := range proposal.Mutations.Schema {
			if err := checkTablet(schema.Predicate); err != nil {
				return err
			}
			if err := checkSchema(schema); err != nil {
				return err
			}
			noTimeout = true
		}
		for _, typ := range proposal.Mutations.Types {
			if err := checkType(typ); err != nil {
				return err
			}
		}
	}

	// Let's keep the same key, so multiple retries of the same proposal would
	// have this shared key. Thus, each server in the group can identify
	// whether it has already done this work, and if so, skip it.
	key := uniqueKey()
	proposal.Key = key
	span := otrace.FromContext(ctx)

	stop := x.SpanTimer(span, "n.proposeAndWait")
	defer stop()

	propose := func(timeout time.Duration) error {
		cctx, cancel := context.WithCancel(ctx)
		defer cancel()

		errCh := make(chan error, 1)
		pctx := &conn.ProposalCtx{
			ErrCh: errCh,
			Ctx:   cctx,
		}
		x.AssertTruef(n.Proposals.Store(key, pctx), "Found existing proposal with key: [%v]", key)
		defer n.Proposals.Delete(key) // Ensure that it gets deleted on return.

		span.Annotatef(nil, "Proposing with key: %s. Timeout: %v", key, timeout)
		data, err := proposal.Marshal()
		if err != nil {
			return err
		}
		if err = n.Raft().Propose(cctx, data); err != nil {
			return errors.Wrapf(err, "While proposing")
		}

		timer := time.NewTimer(timeout)
		defer timer.Stop()

		for {
			select {
			case err = <-errCh:
				// We arrived here by a call to n.Proposals.Done().
				return err
			case <-ctx.Done():
				return ctx.Err()
			case <-timer.C:
				if atomic.LoadUint32(&pctx.Found) > 0 {
					// We found the proposal in CommittedEntries. No need to retry.
				} else {
					span.Annotatef(nil, "Timeout %s reached. Cancelling...", timeout)
					cancel()
				}
			case <-cctx.Done():
				return errInternalRetry
			}
		}
	}

	// Some proposals, like schema updates are very expensive to retry. So, let's
	// not do the retry mechanism on them. Instead, we can set a long timeout.
	//
	// Note that timeout only affects how long it takes us to find the proposal back via Raft logs.
	// It does not consider the amount of time it takes to actually apply the proposal.
	//
	// Based on updated logic, once we find the proposal in the raft log, we would not cancel it
	// anyways. Instead, we'd let the proposal run its course.
	if noTimeout {
		return propose(3 * time.Minute)
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
		switch {
		case proposal.Delta != nil: // Is a delta.
			// If a proposal is important (like delta updates), let's not run it via the limiter
			// below. We should always propose it irrespective of how many pending proposals there
			// might be.
		default:
			if err := limiter.incr(ctx, i); err != nil {
				return err
			}
			defer limiter.decr(i)
		}

		if err := propose(newTimeout(i)); err != errInternalRetry {
			return err
		}
	}
	return errUnableToServe
}
