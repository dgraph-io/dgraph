/*
 * Copyright 2017-2018 Dgraph Labs, Inc.
 *
 * This file is available under the Apache License, Version 2.0,
 * with the Commons Clause restriction.
 */

package zero

import (
	"encoding/base64"
	"errors"
	"math/rand"
	"time"

	"github.com/dgraph-io/dgo/protos/api"
	"github.com/dgraph-io/dgraph/protos/intern"
	"github.com/dgraph-io/dgraph/x"
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
	rowCommit  map[string]uint64   // fp(key) -> commitTs. Used to detect conflict.
	aborts     map[uint64]struct{} // key is startTs
	maxPending uint64              // max transaction startTs given out by us.

	// timestamp at the time of start of server or when it became leader. Used to detect conflicts.
	tmax uint64
	// All transactions with startTs < startTxnTs return true for hasConflict.
	startTxnTs  uint64
	subscribers map[int]chan *intern.OracleDelta
	updates     chan *intern.OracleDelta
	doneUntil   x.WaterMark
	syncMarks   []syncMark
}

func (o *Oracle) Init() {
	o.commits = make(map[uint64]uint64)
	o.rowCommit = make(map[string]uint64)
	o.aborts = make(map[uint64]struct{})
	o.subscribers = make(map[int]chan *intern.OracleDelta)
	o.updates = make(chan *intern.OracleDelta, 100000) // Keeping 1 second worth of updates.
	o.doneUntil.Init()
	go o.sendDeltasToSubscribers()
}

func (o *Oracle) updateStartTxnTs(ts uint64) {
	o.Lock()
	defer o.Unlock()
	o.startTxnTs = ts
	o.rowCommit = make(map[string]uint64)
}

func (o *Oracle) hasConflict(src *api.TxnContext) bool {
	// This transaction was started before I became leader.
	if src.StartTs < o.startTxnTs {
		return true
	}
	for _, k := range src.Keys {
		if last := o.rowCommit[k]; last > src.StartTs {
			return true
		}
	}
	return false
}

func (o *Oracle) purgeBelow(minTs uint64) {
	x.Printf("purging below ts:%d, len(o.commits):%d, len(o.aborts):%d"+
		", len(o.rowCommit):%d\n",
		minTs, len(o.commits), len(o.aborts), len(o.rowCommit))
	o.Lock()
	defer o.Unlock()

	// Dropping would be cheaper if abort/commits map is sharded
	for ts := range o.commits {
		if ts < minTs {
			delete(o.commits, ts)
		}
	}
	for ts := range o.aborts {
		if ts < minTs {
			delete(o.aborts, ts)
		}
	}
	// There is no transaction running with startTs less than minTs
	// So we can delete everything from rowCommit whose commitTs < minTs
	for key, ts := range o.rowCommit {
		if ts < minTs {
			delete(o.rowCommit, key)
		}
	}
	o.tmax = minTs
}

func (o *Oracle) commit(src *api.TxnContext) error {
	o.Lock()
	defer o.Unlock()

	if o.hasConflict(src) {
		return errConflict
	}
	for _, k := range src.Keys {
		o.rowCommit[k] = src.CommitTs // CommitTs is handed out before calling this func.
	}
	return nil
}

func (o *Oracle) aborted(startTs uint64) bool {
	o.Lock()
	defer o.Unlock()
	_, ok := o.aborts[startTs]
	return ok
}

func (o *Oracle) currentState() *intern.OracleDelta {
	o.AssertRLock()
	resp := &intern.OracleDelta{
		Commits: make(map[uint64]uint64, len(o.commits)),
	}
	for start, commit := range o.commits {
		resp.Commits[start] = commit
	}
	for abort := range o.aborts {
		resp.Aborts = append(resp.Aborts, abort)
	}
	resp.MaxPending = o.maxPending
	return resp
}

func (o *Oracle) newSubscriber() (<-chan *intern.OracleDelta, int) {
	o.Lock()
	defer o.Unlock()
	var id int
	for {
		id = rand.Int()
		if _, has := o.subscribers[id]; !has {
			break
		}
	}
	ch := make(chan *intern.OracleDelta, 1000)
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
	delta := &intern.OracleDelta{
		Commits: make(map[uint64]uint64),
	}
	for {
		update, open := <-o.updates
		if !open {
			return
		}
	slurp_loop:
		for {
			// Consume tctx.
			if update.MaxPending > delta.MaxPending {
				delta.MaxPending = update.MaxPending
			}
			for _, startTs := range update.Aborts {
				delta.Aborts = append(delta.Aborts, startTs)
			}
			for startTs, commitTs := range update.Commits {
				delta.Commits[startTs] = commitTs
			}
			select {
			case update, open = <-o.updates:
				if !open {
					return
				}
			default:
				break slurp_loop
			}
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
		delta = &intern.OracleDelta{
			Commits: make(map[uint64]uint64),
		}
	}
}

func (o *Oracle) updateCommitStatusHelper(index uint64, src *api.TxnContext) bool {
	o.Lock()
	defer o.Unlock()
	if _, ok := o.commits[src.StartTs]; ok {
		return false
	}
	if _, ok := o.aborts[src.StartTs]; ok {
		return false
	}
	if src.Aborted {
		o.aborts[src.StartTs] = struct{}{}
	} else {
		o.commits[src.StartTs] = src.CommitTs
	}
	o.syncMarks = append(o.syncMarks, syncMark{index: index, ts: src.StartTs})
	return true
}

func (o *Oracle) updateCommitStatus(index uint64, src *api.TxnContext) {
	if o.updateCommitStatusHelper(index, src) {
		delta := new(intern.OracleDelta)
		if src.Aborted {
			delta.Aborts = append(delta.Aborts, src.StartTs)
		} else {
			delta.Commits = make(map[uint64]uint64)
			delta.Commits[src.StartTs] = src.CommitTs
		}
		o.updates <- delta
	}
}

func (o *Oracle) commitTs(startTs uint64) uint64 {
	o.RLock()
	defer o.RUnlock()
	return o.commits[startTs]
}

func (o *Oracle) storePending(ids *api.AssignedIds) {
	// Wait to finish up processing everything before start id.
	o.doneUntil.WaitForMark(context.Background(), ids.EndId)
	// Now send it out to updates.
	o.updates <- &intern.OracleDelta{MaxPending: ids.EndId}
	o.Lock()
	defer o.Unlock()
	max := ids.EndId
	if o.maxPending < max {
		o.maxPending = max
	}
}

func (o *Oracle) MaxPending() uint64 {
	o.RLock()
	defer o.RUnlock()
	return o.maxPending
}

var errConflict = errors.New("Transaction conflict")

func (s *Server) proposeTxn(ctx context.Context, src *api.TxnContext) error {
	var zp intern.ZeroProposal
	zp.Txn = &api.TxnContext{
		StartTs:  src.StartTs,
		CommitTs: src.CommitTs,
		Aborted:  src.Aborted,
	}
	return s.Node.proposeAndWait(ctx, &zp)
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
	preds["_predicate_"] = struct{}{}

	for _, k := range src.Keys {
		key, err := base64.StdEncoding.DecodeString(k)
		if err != nil {
			continue
		}
		pk := x.Parse(key)
		if pk != nil {
			preds[pk.Attr] = struct{}{}
		}
	}
	for pred := range preds {
		tablet := s.ServingTablet(pred)
		if tablet == nil || tablet.GetReadOnly() {
			src.Aborted = true
			return s.proposeTxn(ctx, src)
		}
	}

	// TODO: We could take fingerprint of the keys, and store them in uint64, allowing the rowCommit
	// map to be keyed by uint64, which would be cheaper. But, unsure about the repurcussions of
	// that. It would save some memory. So, worth a try.

	var num intern.Num
	num.Val = 1
	assigned, err := s.lease(ctx, &num, true)
	if err != nil {
		return err
	}
	src.CommitTs = assigned.StartId

	if err := s.orc.commit(src); err != nil {
		src.Aborted = true
	}
	// Propose txn should be used to set watermark as done.
	err = s.proposeTxn(ctx, src)
	// There might be race between this proposal trying to commit and predicate
	// move aborting it. A predicate move, triggered by Zero, would abort all pending transactions.
	// At the same time, a client which has already done mutations, can proceed to commit it. A race
	// condition can happen here, with both proposing their respective states, only one can succeed
	// after the proposal is done. So, check again to see the fate of the transaction here.
	if s.orc.aborted(src.StartTs) {
		src.Aborted = true
	}
	// Mark the transaction as done, irrespective of whether the proposal succeeded or not.
	s.orc.doneUntil.Done(src.CommitTs)
	return err
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

func (s *Server) Oracle(unused *api.Payload, server intern.Zero_OracleServer) error {
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
	return nil
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
	ticker := time.NewTicker(time.Second * 10)
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
				x.Printf("No healthy connection found to leader of group %d\n", group)
				goto OUTER
			}
			c := intern.NewWorkerClient(pl.Get())
			num, err := c.MinTxnTs(context.Background(), &api.Payload{})
			if err != nil {
				x.Printf("Error while fetching minTs from group %d, err: %v\n", group, err)
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

func (s *Server) TryAbort(ctx context.Context, txns *intern.TxnTimestamps) (*intern.TxnTimestamps, error) {
	commitTimestamps := new(intern.TxnTimestamps)
	for _, startTs := range txns.Ts {
		// Do via proposals to avoid race
		tctx := &api.TxnContext{StartTs: startTs, Aborted: true}
		if err := s.proposeTxn(ctx, tctx); err != nil {
			return commitTimestamps, err
		}
		// Txn should be aborted if not already committed.
		commitTimestamps.Ts = append(commitTimestamps.Ts, s.orc.commitTs(startTs))
	}
	return commitTimestamps, nil
}

// Timestamps is used to assign startTs for a new transaction
func (s *Server) Timestamps(ctx context.Context, num *intern.Num) (*api.AssignedIds, error) {
	if ctx.Err() != nil {
		return &emptyAssignedIds, ctx.Err()
	}

	reply, err := s.lease(ctx, num, true)
	if err == nil {
		s.orc.doneUntil.Done(reply.EndId)
		go s.orc.storePending(reply)
	}
	return reply, err
}
