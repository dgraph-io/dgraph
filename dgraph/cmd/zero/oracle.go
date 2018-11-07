/*
 * Copyright 2017-2018 Dgraph Labs, Inc. and Contributors
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

package zero

import (
	"errors"
	"math/rand"
	"time"

	"github.com/dgraph-io/dgo/protos/api"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/x"
	"github.com/golang/glog"
	"golang.org/x/net/context"
)

type syncMark struct {
	index uint64
	ts    uint64
}

type Oracle struct {
	x.SafeMutex
	commits map[uint64]uint64 // startTs -> commitTs
	// TODO: Check if we need LRU.
	keyCommit   map[string]uint64 // fp(key) -> commitTs. Used to detect conflict.
	maxAssigned uint64            // max transaction assigned by us.

	// timestamp at the time of start of server or when it became leader. Used to detect conflicts.
	tmax uint64
	// All transactions with startTs < startTxnTs return true for hasConflict.
	startTxnTs  uint64
	subscribers map[int]chan *pb.OracleDelta
	updates     chan *pb.OracleDelta
	doneUntil   x.WaterMark
	syncMarks   []syncMark
}

func (o *Oracle) Init() {
	o.commits = make(map[uint64]uint64)
	o.keyCommit = make(map[string]uint64)
	o.subscribers = make(map[int]chan *pb.OracleDelta)
	o.updates = make(chan *pb.OracleDelta, 100000) // Keeping 1 second worth of updates.
	o.doneUntil.Init()
	go o.sendDeltasToSubscribers()
}

func (o *Oracle) updateStartTxnTs(ts uint64) {
	o.Lock()
	defer o.Unlock()
	o.startTxnTs = ts
	o.keyCommit = make(map[string]uint64)
}

func (o *Oracle) hasConflict(src *api.TxnContext) bool {
	// This transaction was started before I became leader.
	if src.StartTs < o.startTxnTs {
		return true
	}
	for _, k := range src.Keys {
		if last := o.keyCommit[k]; last > src.StartTs {
			return true
		}
	}
	return false
}

func (o *Oracle) purgeBelow(minTs uint64) {
	o.Lock()
	defer o.Unlock()

	// Dropping would be cheaper if abort/commits map is sharded
	for ts := range o.commits {
		if ts < minTs {
			delete(o.commits, ts)
		}
	}
	// There is no transaction running with startTs less than minTs
	// So we can delete everything from rowCommit whose commitTs < minTs
	for key, ts := range o.keyCommit {
		if ts < minTs {
			delete(o.keyCommit, key)
		}
	}
	o.tmax = minTs
	glog.Infof("Purged below ts:%d, len(o.commits):%d"+
		", len(o.rowCommit):%d\n",
		minTs, len(o.commits), len(o.keyCommit))
}

func (o *Oracle) commit(src *api.TxnContext) error {
	o.Lock()
	defer o.Unlock()

	if o.hasConflict(src) {
		return errConflict
	}
	for _, k := range src.Keys {
		o.keyCommit[k] = src.CommitTs // CommitTs is handed out before calling this func.
	}
	return nil
}

func (o *Oracle) currentState() *pb.OracleDelta {
	o.AssertRLock()
	resp := &pb.OracleDelta{}
	for start, commit := range o.commits {
		resp.Txns = append(resp.Txns,
			&pb.TxnStatus{StartTs: start, CommitTs: commit})
	}
	resp.MaxAssigned = o.maxAssigned
	return resp
}

func (o *Oracle) newSubscriber() (<-chan *pb.OracleDelta, int) {
	o.Lock()
	defer o.Unlock()
	var id int
	for {
		id = rand.Int()
		if _, has := o.subscribers[id]; !has {
			break
		}
	}
	ch := make(chan *pb.OracleDelta, 1000)
	ch <- o.currentState() // Queue up the full state as the first entry.
	o.subscribers[id] = ch
	return ch, id
}

func (o *Oracle) removeSubscriber(id int) {
	o.Lock()
	defer o.Unlock()
	delete(o.subscribers, id)
}

func (o *Oracle) sendDeltasToSubscribers() {
	delta := &pb.OracleDelta{}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	waitFor := func() uint64 {
		w := delta.MaxAssigned
		for _, txn := range delta.Txns {
			w = x.Max(w, txn.CommitTs)
		}
		return w
	}

	for {
	get_update:
		var update *pb.OracleDelta
		select {
		case update = <-o.updates:
		case <-ticker.C:
			wait := waitFor()
			if wait == 0 || o.doneUntil.DoneUntil() < wait {
				goto get_update
			}
			// Send empty update.
			update = &pb.OracleDelta{}
		}
	slurp_loop:
		for {
			delta.MaxAssigned = x.Max(delta.MaxAssigned, update.MaxAssigned)
			delta.Txns = append(delta.Txns, update.Txns...)
			select {
			case update = <-o.updates:
			default:
				break slurp_loop
			}
		}
		// No need to sort the txn updates here. Alpha would sort them before
		// applying.

		// Let's ensure that we have all the commits up until the max here.
		// Otherwise, we'll be sending commit timestamps out of order, which
		// would cause Alphas to drop some of them, during writes to Badger.
		if o.doneUntil.DoneUntil() < waitFor() {
			continue // The for loop doing blocking reads from o.updates.
			// We need at least one entry from the updates channel to pick up a missing update.
			// Don't goto slurp_loop, because it would break from select immediately.
		}

		o.Lock()
		for id, ch := range o.subscribers {
			select {
			case ch <- delta:
			default:
				close(ch)
				delete(o.subscribers, id)
			}
		}
		o.Unlock()
		delta = &pb.OracleDelta{}
	}
}

func (o *Oracle) updateCommitStatusHelper(index uint64, src *api.TxnContext) bool {
	o.Lock()
	defer o.Unlock()
	if _, ok := o.commits[src.StartTs]; ok {
		return false
	}
	if src.Aborted {
		o.commits[src.StartTs] = 0
	} else {
		o.commits[src.StartTs] = src.CommitTs
	}
	o.syncMarks = append(o.syncMarks, syncMark{index: index, ts: src.StartTs})
	return true
}

func (o *Oracle) updateCommitStatus(index uint64, src *api.TxnContext) {
	// TODO: We should check if the tablet is in read-only status here.
	if o.updateCommitStatusHelper(index, src) {
		delta := new(pb.OracleDelta)
		delta.Txns = append(delta.Txns, &pb.TxnStatus{
			StartTs:  src.StartTs,
			CommitTs: o.commitTs(src.StartTs),
		})
		o.updates <- delta
	}
}

func (o *Oracle) commitTs(startTs uint64) uint64 {
	o.RLock()
	defer o.RUnlock()
	return o.commits[startTs]
}

func (o *Oracle) storePending(ids *pb.AssignedIds) {
	// Wait to finish up processing everything before start id.
	max := x.Max(ids.EndId, ids.ReadOnly)
	o.doneUntil.WaitForMark(context.Background(), max)
	// Now send it out to updates.
	o.updates <- &pb.OracleDelta{MaxAssigned: max}

	o.Lock()
	defer o.Unlock()
	o.maxAssigned = x.Max(o.maxAssigned, max)
}

func (o *Oracle) MaxPending() uint64 {
	o.RLock()
	defer o.RUnlock()
	return o.maxAssigned
}

var errConflict = errors.New("Transaction conflict")

// proposeTxn proposes a txn update, and then updates src to reflect the state
// of the commit after proposal is run.
func (s *Server) proposeTxn(ctx context.Context, src *api.TxnContext) error {
	var zp pb.ZeroProposal
	zp.Txn = &api.TxnContext{
		StartTs:  src.StartTs,
		CommitTs: src.CommitTs,
		Aborted:  src.Aborted,
	}
	if err := s.Node.proposeAndWait(ctx, &zp); err != nil {
		return err
	}

	// There might be race between this proposal trying to commit and predicate
	// move aborting it. A predicate move, triggered by Zero, would abort all
	// pending transactions.  At the same time, a client which has already done
	// mutations, can proceed to commit it. A race condition can happen here,
	// with both proposing their respective states, only one can succeed after
	// the proposal is done. So, check again to see the fate of the transaction
	// here.
	src.CommitTs = s.orc.commitTs(src.StartTs)
	if src.CommitTs == 0 {
		src.Aborted = true
	}
	return nil
}

func (s *Server) commit(ctx context.Context, src *api.TxnContext) error {
	if src.Aborted {
		return s.proposeTxn(ctx, src)
	}

	// Use the start timestamp to check if we have a conflict, before we need to assign a commit ts.
	s.orc.RLock()
	conflict := s.orc.hasConflict(src)
	s.orc.RUnlock()
	if conflict {
		src.Aborted = true
		return s.proposeTxn(ctx, src)
	}

	// Check if any of these tablets is being moved. If so, abort the transaction.
	preds := make(map[string]struct{})
	// _predicate_ would never be part of conflict detection, so keys corresponding to any
	// modifications to this predicate would not be sent to Zero. But, we still need to abort
	// transactions which are coming in, while this predicate is being moved. This means that if
	// _predicate_ expansion is enabled, and a move for this predicate is happening, NO transactions
	// across the entire cluster would commit. Sorry! But if we don't do this, we might lose commits
	// which sneaked in during the move.

	// Ensure that we only consider checking _predicate_, if expand_edge flag is
	// set to true, i.e. we are actually serving _predicate_. Otherwise, the
	// code below, which checks if a tablet is present or is readonly, causes
	// ALL txns to abort. See #2547.
	if tablet := s.ServingTablet("_predicate_"); tablet != nil {
		preds["_predicate_"] = struct{}{}
	}

	for _, k := range src.Preds {
		preds[k] = struct{}{}
	}
	for pred := range preds {
		tablet := s.ServingTablet(pred)
		if tablet == nil || tablet.GetReadOnly() {
			src.Aborted = true
			return s.proposeTxn(ctx, src)
		}
	}

	num := pb.Num{Val: 1}
	assigned, err := s.lease(ctx, &num, true)
	if err != nil {
		return err
	}
	src.CommitTs = assigned.StartId
	// Mark the transaction as done, irrespective of whether the proposal succeeded or not.
	defer s.orc.doneUntil.Done(src.CommitTs)

	if err := s.orc.commit(src); err != nil {
		src.Aborted = true
	}
	// Propose txn should be used to set watermark as done.
	return s.proposeTxn(ctx, src)
}

func (s *Server) CommitOrAbort(ctx context.Context, src *api.TxnContext) (*api.TxnContext, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if !s.Node.AmLeader() {
		return nil, x.Errorf("Only leader can decide to commit or abort")
	}
	err := s.commit(ctx, src)
	return src, err
}

var errClosed = errors.New("Streaming closed by Oracle.")
var errNotLeader = errors.New("Node is no longer leader.")

func (s *Server) Oracle(unused *api.Payload, server pb.Zero_OracleServer) error {
	if !s.Node.AmLeader() {
		return errNotLeader
	}
	ch, id := s.orc.newSubscriber()
	defer s.orc.removeSubscriber(id)

	ctx := server.Context()
	leaderChangeCh := s.leaderChangeChannel()
	for {
		select {
		case <-leaderChangeCh:
			return errNotLeader
		case delta, open := <-ch:
			if !open {
				return errClosed
			}
			if err := server.Send(delta); err != nil {
				return err
			}
		case <-ctx.Done():
			return ctx.Err()
		case <-s.shutDownCh:
			return errServerShutDown
		}
	}
}

func (s *Server) SyncedUntil() uint64 {
	s.orc.Lock()
	defer s.orc.Unlock()
	// Find max index with timestamp less than tmax
	var idx int
	for i, sm := range s.orc.syncMarks {
		idx = i
		if sm.ts >= s.orc.tmax {
			break
		}
	}
	var syncUntil uint64
	if idx > 0 {
		syncUntil = s.orc.syncMarks[idx-1].index
	}
	s.orc.syncMarks = s.orc.syncMarks[idx:]
	return syncUntil
}

func (s *Server) purgeOracle() {
	ticker := time.NewTicker(time.Second * 30)
	defer ticker.Stop()

	var lastPurgeTs uint64
OUTER:
	for {
		<-ticker.C
		groups := s.KnownGroups()
		var minTs uint64
		for _, group := range groups {
			pl := s.Leader(group)
			if pl == nil {
				glog.Errorf("No healthy connection found to leader of group %d\n", group)
				goto OUTER
			}
			c := pb.NewWorkerClient(pl.Get())
			num, err := c.PurgeTs(context.Background(), &api.Payload{})
			if err != nil {
				glog.Errorf("Error while fetching minTs from group %d, err: %v\n", group, err)
				goto OUTER
			}
			if minTs == 0 || num.Val < minTs {
				minTs = num.Val
			}
		}

		if minTs > 0 && minTs != lastPurgeTs {
			s.orc.purgeBelow(minTs)
			lastPurgeTs = minTs
		}
	}
}

func (s *Server) TryAbort(ctx context.Context,
	txns *pb.TxnTimestamps) (*pb.OracleDelta, error) {
	delta := &pb.OracleDelta{}
	for _, startTs := range txns.Ts {
		// Do via proposals to avoid race
		tctx := &api.TxnContext{StartTs: startTs, Aborted: true}
		if err := s.proposeTxn(ctx, tctx); err != nil {
			return delta, err
		}
		// Txn should be aborted if not already committed.
		delta.Txns = append(delta.Txns, &pb.TxnStatus{
			StartTs:  startTs,
			CommitTs: s.orc.commitTs(startTs)})
	}
	return delta, nil
}

// Timestamps is used to assign startTs for a new transaction
func (s *Server) Timestamps(ctx context.Context, num *pb.Num) (*pb.AssignedIds, error) {
	if ctx.Err() != nil {
		return &emptyAssignedIds, ctx.Err()
	}

	reply, err := s.lease(ctx, num, true)
	if err == nil {
		s.orc.doneUntil.Done(x.Max(reply.EndId, reply.ReadOnly))
		go s.orc.storePending(reply)

	} else if err == servedFromMemory {
		// Avoid calling doneUntil.Done, and storePending.
		err = nil

	} else {
		glog.Errorf("Got error: %v while leasing timestamps: %+v", err, num)
	}
	return reply, err
}
