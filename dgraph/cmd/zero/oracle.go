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
	"context"
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dgraph-io/badger/v2/y"
	"github.com/dgraph-io/dgo/v2/protos/api"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/x"
	"github.com/golang/glog"
	"github.com/pkg/errors"
	otrace "go.opencensus.io/trace"
)

type syncMark struct {
	index uint64
	ts    uint64
}

// Oracle stores and manages the transaction state and conflict detection.
type Oracle struct {
	x.SafeMutex
	commits map[string]map[uint64]uint64 // startTs -> commitTs
	// TODO: Check if we need LRU.
	keyCommit   map[string]uint64 // fp(key) -> commitTs. Used to detect conflict.
	maxAssigned map[string]uint64 // max transaction assigned by us.

	// timestamp at the time of start of server or when it became leader. Used to detect conflicts.
	tmax uint64
	// All transactions with startTs < startTxnTs return true for hasConflict.
	startTxnTs     uint64
	subscribers    map[int]chan pb.OracleDelta
	updates        chan *pb.OracleDelta
	doneUntilMutex sync.Mutex
	doneUntil      map[string]*y.WaterMark
	syncMarks      []syncMark
}

// Init initializes the oracle.
func (o *Oracle) Init() {
	o.commits = make(map[string]map[uint64]uint64)
	o.keyCommit = make(map[string]uint64)
	o.subscribers = make(map[int]chan pb.OracleDelta)
	o.updates = make(chan *pb.OracleDelta, 100000) // Keeping 1 second worth of updates.
	o.doneUntil = make(map[string]*y.WaterMark)
	o.maxAssigned = make(map[string]uint64)
	wm := &y.WaterMark{}
	wm.Init(nil, true)
	o.doneUntil[x.DefaultNamespace] = wm
	o.maxAssigned[x.DefaultNamespace] = 0
	go o.sendDeltasToSubscribers()
}

func (o *Oracle) updateStartTxnTs(ts uint64) {
	o.Lock()
	defer o.Unlock()
	o.startTxnTs = ts
	o.keyCommit = make(map[string]uint64)
}

// TODO: This should be done during proposal application for Txn status.
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
	for _, timeStamps := range o.commits {
		for _, commitTs := range timeStamps {
			if commitTs < minTs {
				delete(timeStamps, commitTs)
			}
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
		return ErrConflict
	}
	for _, k := range src.Keys {
		o.keyCommit[k] = src.CommitTs // CommitTs is handed out before calling this func.
	}
	return nil
}

func (o *Oracle) currentState() []*pb.OracleDelta {
	o.AssertRLock()
	out := []*pb.OracleDelta{}
	for namespace, maxAssigned := range o.maxAssigned {
		delta := &pb.OracleDelta{
			Namespace:   namespace,
			MaxAssigned: maxAssigned,
		}
		for start, commit := range o.commits[namespace] {
			delta.Txns = append(delta.Txns,
				&pb.TxnStatus{StartTs: start, CommitTs: commit})
		}
		out = append(out, delta)
	}
	return out
}

func (o *Oracle) newSubscriber() (<-chan pb.OracleDelta, int) {
	o.Lock()
	defer o.Unlock()
	var id int
	for {
		id = rand.Int()
		if _, has := o.subscribers[id]; !has {
			break
		}
	}

	// The channel takes a delta instead of a pointer as the receiver needs to
	// modify it by setting the group checksums. Passing a pointer previously
	// resulted in a race condition.
	ch := make(chan pb.OracleDelta, 1000)
	states := o.currentState()
	for _, state := range states {
		ch <- *state // Queue up the full state as the first entry.
	}
	o.subscribers[id] = ch
	return ch, id
}

func (o *Oracle) removeSubscriber(id int) {
	o.Lock()
	defer o.Unlock()
	delete(o.subscribers, id)
}

// streamDeltaToSubscriber will send orcle delta to all the listening subcriber.
func (o *Oracle) streamDeltasToSubscriber(deltas []*pb.OracleDelta) {
	o.Lock()
	for _, delta := range deltas {
		for id, ch := range o.subscribers {
			select {
			case ch <- *delta:
			default:
				close(ch)
				delete(o.subscribers, id)
			}
		}
	}
	o.Unlock()
}

// sendDeltasToSubscribers reads updates from the o.updates
// constructs a delta object containing transactions from one or more updates
// and sends the delta object to each subscriber's channel
func (o *Oracle) sendDeltasToSubscribers() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	deltas := make(map[string]*pb.OracleDelta)

	// updateDelta will batch the incoming delta for the given namespace.
	updateDelta := func(update *pb.OracleDelta) {
		fmt.Printf("update %+v \n", update)
		y.AssertTrue(update.Namespace != "")
		delta, ok := deltas[update.GetNamespace()]
		if !ok {
			delta = update
		}
		delta.MaxAssigned = x.Max(delta.MaxAssigned, update.MaxAssigned)
		delta.Txns = append(delta.Txns, update.Txns...)
		deltas[update.GetNamespace()] = delta
	}

	// waitFor calculates the maximum value of delta.MaxAssigned and all the CommitTs of delta.Txns
	//
	waitForNamespace := func(namespace string) uint64 {
		delta, ok := deltas[namespace]
		if !ok {
			return 0
		}
		w := delta.MaxAssigned
		for _, txn := range delta.Txns {
			w = x.Max(w, txn.CommitTs)
		}
		return w
	}

	for {
	get_update:
		select {
		case update := <-o.updates:
			updateDelta(update)
		case <-ticker.C:
			// progressed will tell any of the namespace is progressed for the periodic tick.
			progressed := false
			o.doneUntilMutex.Lock()
			for namespace := range deltas {
				if o.doneUntil[namespace].DoneUntil() < waitForNamespace(namespace) {
					continue
				}
				progressed = true
				break
			}
			o.doneUntilMutex.Unlock()
			if !progressed {
				// None of the namespaces are progressed. so, block for the next update or
				// the next peridic tick.
				goto get_update
			}
		}
	slurp_loop:
		for {
			select {
			case update := <-o.updates:
				updateDelta(update)
			default:
				break slurp_loop
			}
		}
		// No need to sort the txn updates here. Alpha would sort them before
		// applying.

		// Let's ensure that we have all the commits up until the max here.
		// Otherwise, we'll be sending commit timestamps out of order, which
		// would cause Alphas to drop some of them, during writes to Badger.
		batch := []*pb.OracleDelta{}
		o.doneUntilMutex.Lock()
		for namespace, delta := range deltas {
			if o.doneUntil[namespace].DoneUntil() < waitForNamespace(namespace) {
				continue
			}
			// every transaction are done for the current timestamp so send out the
			// delta for this namespace.
			batch = append(batch, delta)
			if glog.V(3) {
				glog.Infof(
					"DoneUntil: %d. Sending delta: %+v\n", o.doneUntil[namespace].DoneUntil(), delta)
			}
			delete(deltas, namespace)
		}
		o.doneUntilMutex.Unlock()
		if len(batch) != 0 {
			// stream all the batched deltas to the subscribers
			o.streamDeltasToSubscriber(batch)
		}
		// The for loop doing blocking reads from o.updates.
		// We need at least one entry from the updates channel to pick up a missing update.
		// Don't goto slurp_loop, because it would break from select immediately.
	}
}

func (o *Oracle) updateCommitStatusHelper(index uint64, src *api.TxnContext) bool {
	o.Lock()
	defer o.Unlock()
	if _, ok := o.commits[src.GetNamespace()]; !ok {
		o.commits[src.GetNamespace()] = make(map[uint64]uint64)
	}
	if _, ok := o.commits[src.GetNamespace()][src.StartTs]; ok {
		return false
	}
	if src.Aborted {
		o.commits[src.GetNamespace()][src.StartTs] = 0
	} else {
		o.commits[src.GetNamespace()][src.StartTs] = src.CommitTs
	}
	o.syncMarks = append(o.syncMarks, syncMark{index: index, ts: src.StartTs})
	return true
}

func (o *Oracle) updateCommitStatus(index uint64, src *api.TxnContext) {
	// TODO: We should check if the tablet is in read-only status here.
	if o.updateCommitStatusHelper(index, src) {
		delta := new(pb.OracleDelta)
		y.AssertTrue(src.Namespace != "")
		delta.Namespace = src.Namespace
		delta.Txns = append(delta.Txns, &pb.TxnStatus{
			StartTs:  src.StartTs,
			CommitTs: o.commitTs(src.Namespace, src.StartTs),
		})
		o.updates <- delta
	}
}

func (o *Oracle) commitTs(namespace string, startTs uint64) uint64 {
	o.RLock()
	defer o.RUnlock()
	return o.commits[namespace][startTs]
}

func (o *Oracle) storePending(namespace string, ids *pb.AssignedIds) {
	// Wait to finish up processing everything before start id.
	max := x.Max(ids.EndId, ids.ReadOnly)

	o.doneUntilMutex.Lock()
	wm := o.doneUntil[namespace]
	o.doneUntilMutex.Unlock()
	if err := wm.WaitForMark(context.Background(), max); err != nil {
		glog.Errorf("Error while waiting for mark: %+v", err)
	}

	// Now send it out to updates.
	o.updates <- &pb.OracleDelta{MaxAssigned: max, Namespace: namespace}
	o.Lock()
	defer o.Unlock()
	o.maxAssigned[namespace] = x.Max(o.maxAssigned[namespace], max)
}

// MaxPending returns the maximum assigned timestamp.
func (o *Oracle) MaxPending() map[string]uint64 {
	o.RLock()
	defer o.RUnlock()
	return o.maxAssigned
}

// ErrConflict is returned when commit couldn't succeed due to conflicts.
var ErrConflict = errors.New("Transaction conflict")

// proposeTxn proposes a txn update, and then updates src to reflect the state
// of the commit after proposal is run.
func (s *Server) proposeTxn(ctx context.Context, src *api.TxnContext) error {
	var zp pb.ZeroProposal
	zp.Txn = &api.TxnContext{
		StartTs:  src.StartTs,
		CommitTs: src.CommitTs,
		Aborted:  src.Aborted,
	}

	// NOTE: It is important that we continue retrying proposeTxn until we succeed. This should
	// happen, irrespective of what the user context timeout might be. We check for it before
	// reaching this stage, but now that we're here, we have to ensure that the commit proposal goes
	// through. Otherwise, we should block here forever. If we don't do this, we'll see txn
	// violations in Jepsen, because we'll send out a MaxAssigned higher than a commit, which would
	// cause newer txns to see older data.

	// If this node stops being the leader, we want this proposal to not be forwarded to the leader,
	// and get aborted.
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
	src.CommitTs = s.orc.commitTs(src.Namespace, src.StartTs)
	if src.CommitTs == 0 {
		src.Aborted = true
	}
	return nil
}

func (s *Server) commit(ctx context.Context, src *api.TxnContext) error {
	span := otrace.FromContext(ctx)
	span.Annotate([]otrace.Attribute{otrace.Int64Attribute("startTs", int64(src.StartTs))}, "")
	if src.Aborted {
		return s.proposeTxn(ctx, src)
	}

	// Use the start timestamp to check if we have a conflict, before we need to assign a commit ts.
	s.orc.RLock()
	conflict := s.orc.hasConflict(src)
	s.orc.RUnlock()
	if conflict {
		span.Annotate([]otrace.Attribute{otrace.BoolAttribute("abort", true)},
			"Oracle found conflict")
		src.Aborted = true
		return s.proposeTxn(ctx, src)
	}

	checkPreds := func() error {
		// Check if any of these tablets is being moved. If so, abort the transaction.
		preds := make(map[string]struct{})

		for _, k := range src.Preds {
			preds[k] = struct{}{}
		}
		for pkey := range preds {
			splits := strings.Split(pkey, "-")
			if len(splits) < 2 {
				return errors.Errorf("Unable to find group id in %s", pkey)
			}
			gid, err := strconv.Atoi(splits[0])
			if err != nil {
				return errors.Wrapf(err, "unable to parse group id from %s", pkey)
			}
			pred := strings.Join(splits[1:], "-")
			tablet := s.ServingTablet(pred)
			if tablet == nil {
				return errors.Errorf("Tablet for %s is nil", pred)
			}
			if tablet.GroupId != uint32(gid) {
				return errors.Errorf("Mutation done in group: %d. Predicate %s assigned to %d",
					gid, pred, tablet.GroupId)
			}
			if s.isBlocked(pred) {
				return errors.Errorf("Commits on predicate %s are blocked due to predicate move", pred)
			}
		}
		return nil
	}
	if err := checkPreds(); err != nil {
		span.Annotate([]otrace.Attribute{otrace.BoolAttribute("abort", true)}, err.Error())
		src.Aborted = true
		return s.proposeTxn(ctx, src)
	}

	num := pb.Num{Val: 1, Namespace: src.GetNamespace()}
	assigned, err := s.lease(ctx, &num, true)
	if err != nil {
		return err
	}
	src.CommitTs = assigned.StartId
	// Mark the transaction as done, irrespective of whether the proposal succeeded or not.
	defer func() {
		s.orc.doneUntilMutex.Lock()
		s.orc.doneUntil[src.Namespace].Done(src.CommitTs)
		s.orc.doneUntilMutex.Unlock()
	}()
	span.Annotatef([]otrace.Attribute{otrace.Int64Attribute("commitTs", int64(src.CommitTs))},
		"Node Id: %d. Proposing TxnContext: %+v", s.Node.Id, src)

	if err := s.orc.commit(src); err != nil {
		span.Annotatef(nil, "Found a conflict. Aborting.")
		src.Aborted = true
	}
	if err := ctx.Err(); err != nil {
		span.Annotatef(nil, "Aborting txn due to context timing out.")
		src.Aborted = true
	}
	// Propose txn should be used to set watermark as done.
	return s.proposeTxn(ctx, src)
}

// CommitOrAbort either commits a transaction or aborts it.
// The abortion can happen under the following conditions
// 1) the api.TxnContext.Aborted flag is set in the src argument
// 2) if there's an error (e.g server is not the leader or there's a conflicting transaction)
func (s *Server) CommitOrAbort(ctx context.Context, src *api.TxnContext) (*api.TxnContext, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	ctx, span := otrace.StartSpan(ctx, "Zero.CommitOrAbort")
	defer span.End()

	if !s.Node.AmLeader() {
		return nil, errors.Errorf("Only leader can decide to commit or abort")
	}
	err := s.commit(ctx, src)
	if err != nil {
		span.Annotate([]otrace.Attribute{otrace.BoolAttribute("error", true)}, err.Error())
	}
	return src, err
}

var errClosed = errors.New("Streaming closed by oracle")
var errNotLeader = errors.New("Node is no longer leader")

// Oracle streams the oracle state to the alphas.
// The first entry sent by Zero contains the entire state of transactions. Zero periodically
// confirms receipt from the group, and truncates its state. This 2-way acknowledgement is a
// safe way to get the status of all the transactions.
func (s *Server) Oracle(_ *api.Payload, server pb.Zero_OracleServer) error {
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
			// Pass in the latest group checksum as well, so the Alpha can use that to determine
			// when not to service a read.
			delta.GroupChecksums = s.groupChecksums()
			if err := server.Send(&delta); err != nil {
				return err
			}
		case <-ctx.Done():
			return ctx.Err()
		case <-s.closer.HasBeenClosed():
			return errServerShutDown
		}
	}
}

// SyncedUntil returns the timestamp up to which all the nodes have synced.
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

// TryAbort attempts to abort the given transactions which are not already committed..
func (s *Server) TryAbort(ctx context.Context,
	txns *pb.TxnTimestamps) (*pb.OracleDelta, error) {
	delta := &pb.OracleDelta{}
	y.AssertTrue(txns.Namespace != "")
	for _, startTs := range txns.Ts {
		// Do via proposals to avoid race
		tctx := &api.TxnContext{StartTs: startTs, Aborted: true, Namespace: txns.Namespace}
		if err := s.proposeTxn(ctx, tctx); err != nil {
			return delta, err
		}
		// Txn should be aborted if not already committed.
		delta.Namespace = txns.Namespace
		delta.Txns = append(delta.Txns, &pb.TxnStatus{
			StartTs:  startTs,
			CommitTs: s.orc.commitTs(txns.Namespace, startTs)})
	}
	return delta, nil
}

// Timestamps is used to assign startTs for a new transaction
func (s *Server) Timestamps(ctx context.Context, num *pb.Num) (*pb.AssignedIds, error) {
	ctx, span := otrace.StartSpan(ctx, "Zero.Timestamps")
	defer span.End()

	span.Annotatef(nil, "Zero id: %d. Timestamp request: %+v", s.Node.Id, num)
	if ctx.Err() != nil {
		return &emptyAssignedIds, ctx.Err()
	}

	reply, err := s.lease(ctx, num, true)
	span.Annotatef(nil, "Response: %+v. Error: %v", reply, err)

	switch err {
	case nil:
		s.orc.doneUntilMutex.Lock()
		s.orc.doneUntil[num.Namespace].Done(x.Max(reply.EndId, reply.ReadOnly))
		s.orc.doneUntilMutex.Unlock()
		go s.orc.storePending(num.Namespace, reply)
	case errServedFromMemory:
		// Avoid calling doneUntil.Done, and storePending.
		err = nil
	default:
		glog.Errorf("Got error: %v while leasing timestamps: %+v", err, num)
	}
	return reply, err
}
