/*
 * Copyright 2016-2018 Dgraph Labs, Inc. and Contributors
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
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/golang/glog"
	"github.com/pkg/errors"
	"go.etcd.io/etcd/raft"
	"go.etcd.io/etcd/raft/raftpb"
	"golang.org/x/net/trace"

	ostats "go.opencensus.io/stats"
	"go.opencensus.io/tag"
	otrace "go.opencensus.io/trace"

	"github.com/dgraph-io/badger/v3"
	bpb "github.com/dgraph-io/badger/v3/pb"
	"github.com/dgraph-io/badger/v3/skl"
	"github.com/dgraph-io/badger/v3/table"
	"github.com/dgraph-io/badger/v3/y"
	"github.com/dgraph-io/dgraph/conn"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/raftwal"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
	"github.com/dgraph-io/ristretto/z"
)

const (
	sensitiveString = "******"
)

type node struct {
	// This needs to be 64 bit aligned for atomics to work on 32 bit machine.
	pendingSize int64

	// embedded struct
	*conn.Node

	// Fields which are never changed after init.
	applyCh      chan []raftpb.Entry
	concApplyCh  chan *pb.Proposal
	drainApplyCh chan struct{}
	ctx          context.Context
	gid          uint32
	closer       *z.Closer

	checkpointTs uint64 // Timestamp corresponding to checkpoint.
	streaming    int32  // Used to avoid calculating snapshot

	// Used to track the ops going on in the system.
	ops         map[op]*z.Closer
	opsLock     sync.Mutex
	cdcTracker  *CDC
	canCampaign bool
	elog        trace.EventLog

	keysWritten      *keysWritten
	pendingProposals []pb.Proposal
}

// keysWritten is always accessed serially via applyCh. So, we don't need to make it thread-safe.
type keysWritten struct {
	rejectBeforeIndex uint64
	keyCommitTs       map[uint64]uint64
	validTxns         int64
	invalidTxns       int64
	totalKeys         int
}

func newKeysWritten() *keysWritten {
	return &keysWritten{
		keyCommitTs: make(map[uint64]uint64),
	}
}

// We use keysWritten structure to allow mutations to be run concurrently. Consider this:
// 1. We receive a txn with mutation at start ts = Ts.
// 2. The server is at MaxAssignedTs Tm < Ts.
// 3. Before, we would block proposing until Tm >= Ts.
// 4. Now, we propose the mutation immediately.
// 5. Once the mutation goes through raft, it is executed concurrently, and the "seen" MaxAssignedTs
//    is registered as Tm-seen.
// 6. The same mutation is also pushed to applyCh.
// 7. When applyCh sees the mutation, it checks if any reads the txn incurred, have been written to
//    with a commit ts in the range (Tm-seen, Ts]. If so, the mutation is re-run. In 21M live load,
//    this happens about 3.6% of the time.
// 8. If no commits have happened for the read key set, we are done. This happens 96.4% of the time.
// 9. If multiple mutations happen for the same txn, the sequential mutations are always run
//    serially by applyCh. This is to avoid edge cases.
func (kw *keysWritten) StillValid(txn *posting.Txn) bool {
	if txn.AppliedIndexSeen < kw.rejectBeforeIndex {
		kw.invalidTxns++
		return false
	}
	if txn.MaxAssignedSeen >= txn.StartTs {
		kw.validTxns++
		return true
	}
	for hash := range txn.ReadKeys() {
		// If the commitTs is between (MaxAssignedSeen, StartTs], the txn reads were invalid. If the
		// commitTs is > StartTs, then it doesn't matter for reads. If the commit ts is <
		// MaxAssignedSeen, that means our reads are valid.
		commitTs := kw.keyCommitTs[hash]
		if commitTs > txn.MaxAssignedSeen && commitTs <= txn.StartTs {
			kw.invalidTxns++
			return false
		}
	}
	kw.validTxns++
	return true
}

type op int

func (id op) String() string {
	switch id {
	case opRollup:
		return "opRollup"
	case opSnapshot:
		return "opSnapshot"
	case opIndexing:
		return "opIndexing"
	case opRestore:
		return "opRestore"
	case opBackup:
		return "opBackup"
	case opPredMove:
		return "opPredMove"
	default:
		return "opUnknown"
	}
}

const (
	opRollup op = iota + 1
	opSnapshot
	opIndexing
	opRestore
	opBackup
	opPredMove
)

// startTask is used to check whether an op is already running. If a rollup is running,
// it is canceled and startTask will wait until it completes before returning.
// If the same task is already running, this method returns an errror.
// Restore operations have preference and cancel all other operations, not just rollups.
// You should only call Done() on the returned closer. Calling other functions (such as
// SignalAndWait) for closer could result in panics. For more details, see GitHub issue #5034.
func (n *node) startTask(id op) (*z.Closer, error) {
	n.opsLock.Lock()
	defer n.opsLock.Unlock()

	stopTask := func(id op) {
		n.opsLock.Lock()
		delete(n.ops, id)
		n.opsLock.Unlock()
		glog.Infof("Operation completed with id: %s", id)

		// Resume rollups if another operation is being stopped.
		if id != opRollup {
			time.Sleep(10 * time.Second) // Wait for 10s to start rollup operation.
			// If any other operation is running, this would error out. This error can
			// be safely ignored because rollups will resume once that other task is done.
			_, _ = n.startTask(opRollup)
		}
	}

	closer := z.NewCloser(1)
	switch id {
	case opRollup:
		if len(n.ops) > 0 {
			return nil, errors.Errorf("another operation is already running")
		}
		go posting.IncrRollup.Process(closer)
	case opRestore:
		// Restores cancel all other operations, except for other restores since
		// only one restore operation should be active any given moment.
		for otherId, otherCloser := range n.ops {
			if otherId == opRestore {
				return nil, errors.Errorf("another restore operation is already running")
			}
			// Remove from map and signal the closer to cancel the operation.
			delete(n.ops, otherId)
			otherCloser.SignalAndWait()
		}
	case opBackup:
		// Backup cancels all other operations, except for other backups since
		// only one restore operation should be active any given moment.
		for otherId, otherCloser := range n.ops {
			if otherId == opBackup {
				return nil, errors.Errorf("another backup operation is already running")
			}
			// Remove from map and signal the closer to cancel the operation.
			delete(n.ops, otherId)
			otherCloser.SignalAndWait()
		}
	case opSnapshot, opIndexing, opPredMove:
		for otherId, otherCloser := range n.ops {
			if otherId == opRollup {
				// Remove from map and signal the closer to cancel the operation.
				delete(n.ops, otherId)
				otherCloser.SignalAndWait()
			} else {
				return nil, errors.Errorf("operation %s is already running", otherId)
			}
		}
	default:
		glog.Errorf("Got an unhandled operation %s. Ignoring...", id)
		return nil, nil
	}

	n.ops[id] = closer
	glog.Infof("Operation started with id: %s", id)
	go func(id op, closer *z.Closer) {
		closer.Wait()
		stopTask(id)
	}(id, closer)
	return closer, nil
}

func (n *node) stopTask(id op) {
	n.opsLock.Lock()
	closer, ok := n.ops[id]
	n.opsLock.Unlock()
	if !ok {
		return
	}
	closer.SignalAndWait()
}

func (n *node) waitForTask(id op) {
	n.opsLock.Lock()
	closer, ok := n.ops[id]
	n.opsLock.Unlock()
	if !ok {
		return
	}
	closer.Wait()
}

func (n *node) isRunningTask(id op) bool {
	n.opsLock.Lock()
	_, ok := n.ops[id]
	n.opsLock.Unlock()
	return ok
}

func (n *node) stopAllTasks() {
	defer n.closer.Done() // CLOSER:1
	<-n.closer.HasBeenClosed()

	glog.Infof("Stopping all ongoing registered tasks...")
	n.opsLock.Lock()
	defer n.opsLock.Unlock()
	for op, closer := range n.ops {
		glog.Infof("Stopping op: %s...\n", op)
		closer.SignalAndWait()
	}
	glog.Infof("Stopped all ongoing registered tasks.")
}

// GetOngoingTasks returns the list of ongoing tasks.
func GetOngoingTasks() []string {
	n := groups().Node
	if n == nil {
		return []string{}
	}

	n.opsLock.Lock()
	defer n.opsLock.Unlock()
	var tasks []string
	for id := range n.ops {
		tasks = append(tasks, id.String())
	}
	return tasks
}

// Now that we apply txn updates via Raft, waiting based on Txn timestamps is
// sufficient. We don't need to wait for proposals to be applied.

func newNode(store *raftwal.DiskStorage, gid uint32, id uint64, myAddr string) *node {
	glog.Infof("Node ID: %#x with GroupID: %d\n", id, gid)

	isLearner := x.WorkerConfig.Raft.GetBool("learner")
	rc := &pb.RaftContext{
		Addr:      myAddr,
		Group:     gid,
		Id:        id,
		IsLearner: isLearner,
	}
	glog.Infof("RaftContext: %+v\n", rc)
	m := conn.NewNode(rc, store, x.WorkerConfig.TLSClientConfig)

	n := &node{
		Node: m,
		ctx:  context.Background(),
		gid:  gid,
		// We need a generous size for applyCh, because raft.Tick happens every
		// 10ms. If we restrict the size here, then Raft goes into a loop trying
		// to maintain quorum health.
		applyCh:      make(chan []raftpb.Entry, 1000),
		concApplyCh:  make(chan *pb.Proposal, 100),
		drainApplyCh: make(chan struct{}),
		elog:         trace.NewEventLog("Dgraph", "ApplyCh"),
		closer:       z.NewCloser(4), // Matches CLOSER:1
		ops:          make(map[op]*z.Closer),
		cdcTracker:   newCDC(),
		keysWritten:  newKeysWritten(),
	}
	return n
}

func (n *node) Ctx(key uint64) context.Context {
	if pctx := n.Proposals.Get(key); pctx != nil {
		return pctx.Ctx
	}
	return context.Background()
}

func (n *node) applyConfChange(e raftpb.Entry) {
	var cc raftpb.ConfChange
	if err := cc.Unmarshal(e.Data); err != nil {
		glog.Errorf("While unmarshalling confchange: %+v", err)
	}

	if cc.Type == raftpb.ConfChangeRemoveNode {
		n.DeletePeer(cc.NodeID)
	} else if len(cc.Context) > 0 {
		var rc pb.RaftContext
		x.Check(rc.Unmarshal(cc.Context))
		n.Connect(rc.Id, rc.Addr)
	}

	cs := n.Raft().ApplyConfChange(cc)
	n.SetConfState(cs)
	n.DoneConfChange(cc.ID, nil)
}

var errHasPendingTxns = errors.New("Pending transactions found. Please retry operation")

// We must not wait here. Previously, we used to block until we have aborted the
// transactions. We're now applying all updates serially, so blocking for one
// operation is not an option.
func detectPendingTxns(attr string) error {
	tctxs := posting.Oracle().IterateTxns(func(key []byte) bool {
		pk, err := x.Parse(key)
		if err != nil {
			glog.Errorf("error %v while parsing key %v", err, hex.EncodeToString(key))
			return false
		}
		return pk.Attr == attr
	})
	if len(tctxs) == 0 {
		return nil
	}
	go tryAbortTransactions(tctxs)
	return errHasPendingTxns
}

func (n *node) mutationWorker(workerId int) {
	handleEntry := func(p *pb.Proposal) {
		x.AssertTrue(p.Key != 0)
		x.AssertTrue(len(p.Mutations.GetEdges()) > 0)

		ctx := n.Ctx(p.Key)
		x.AssertTrue(ctx != nil)
		span := otrace.FromContext(ctx)
		span.Annotatef(nil, "Executing mutation from worker id: %d", workerId)

		txn := posting.Oracle().GetTxn(p.Mutations.StartTs)
		x.AssertTruef(txn != nil, "Unable to find txn with start ts: %d", p.Mutations.StartTs)
		txn.ErrCh <- n.concMutations(ctx, p.Mutations, txn)
		close(txn.ErrCh)
	}

	for {
		select {
		case mut, ok := <-n.concApplyCh:
			if !ok {
				return
			}
			handleEntry(mut)
		case <-n.closer.HasBeenClosed():
			return
		}
	}
}

func (n *node) concMutations(ctx context.Context, m *pb.Mutations, txn *posting.Txn) error {
	// It is possible that the user gives us multiple versions of the same edge, one with no facets
	// and another with facets. In that case, use stable sort to maintain the ordering given to us
	// by the user.
	// TODO: Do this in a way, where we don't break multiple updates for the same Edge across
	// different goroutines.
	sort.SliceStable(m.Edges, func(i, j int) bool {
		ei := m.Edges[i]
		ej := m.Edges[j]
		if ei.GetAttr() != ej.GetAttr() {
			return ei.GetAttr() < ej.GetAttr()
		}
		return ei.GetEntity() < ej.GetEntity()
	})

	span := otrace.FromContext(ctx)
	if txn.ShouldAbort() {
		span.Annotatef(nil, "Txn %d should abort.", m.StartTs)
		return x.ErrConflict
	}
	// Discard the posting lists from cache to release memory at the end.
	defer func() {
		txn.Update(ctx)
		span.Annotate(nil, "update done")
	}()

	// Update the applied index that we are seeing.
	if txn.AppliedIndexSeen == 0 {
		txn.AppliedIndexSeen = n.Applied.DoneUntil()
	}
	if txn.MaxAssignedSeen == 0 {
		txn.MaxAssignedSeen = posting.Oracle().MaxAssigned()
	}

	// This txn's Zero assigned start ts could be in the future, because we're
	// trying to greedily run mutations concurrently as soon as we see them.
	// In this case, MaxAssignedSeen could be < txn.StartTs. We'd
	// opportunistically do the processing of this mutation anyway. And later,
	// check if everything that we read is still valid, or was it changed. If
	// it was indeed changed, we can re-do the work.

	process := func(edges []*pb.DirectedEdge) error {
		var retries int
		for _, edge := range edges {
			for {
				err := runMutation(ctx, edge, txn)
				if err == nil {
					break
				}
				if err != posting.ErrRetry {
					return err
				}
				retries++
			}
		}
		if retries > 0 {
			span.Annotatef(nil, "retries=true num=%d", retries)
		}
		return nil
	}
	numGo, width := x.DivideAndRule(len(m.Edges))
	span.Annotatef(nil, "To apply: %d edges. NumGo: %d. Width: %d", len(m.Edges), numGo, width)

	if numGo == 1 {
		span.Annotate(nil, "Process mutations done.")
		return process(m.Edges)
	}
	errCh := make(chan error, numGo)
	for i := 0; i < numGo; i++ {
		start := i * width
		end := start + width
		if end > len(m.Edges) {
			end = len(m.Edges)
		}
		go func(start, end int) {
			errCh <- process(m.Edges[start:end])
		}(start, end)
	}
	var rerr error
	for i := 0; i < numGo; i++ {
		if err := <-errCh; err != nil && rerr == nil {
			rerr = err
		}
	}
	span.Annotate(nil, "Process mutations done.")
	return rerr
}

// We don't support schema mutations across nodes in a transaction.
// Wait for all transactions to either abort or complete and all write transactions
// involving the predicate are aborted until schema mutations are done.
func (n *node) applyMutations(ctx context.Context, proposal *pb.Proposal) (rerr error) {
	span := otrace.FromContext(ctx)

	if proposal.Mutations.DropOp == pb.Mutations_DATA {
		ns, err := strconv.ParseUint(proposal.Mutations.DropValue, 0, 64)
		if err != nil {
			return err
		}
		// Ensures nothing get written to disk due to commit proposals.
		n.keysWritten.rejectBeforeIndex = proposal.Index

		// Stop rollups, otherwise we might end up overwriting some new data.
		n.stopTask(opRollup)
		defer n.startTask(opRollup)

		posting.Oracle().ResetTxnsForNs(ns)
		if err := posting.DeleteData(ns); err != nil {
			return err
		}

		// TODO: Revisit this when we work on posting cache. Clear entire cache.
		// We don't want to drop entire cache, just due to one namespace.
		// posting.ResetCache()
		return nil
	}

	if proposal.Mutations.DropOp == pb.Mutations_ALL {
		// Ensures nothing get written to disk due to commit proposals.
		n.keysWritten.rejectBeforeIndex = proposal.Index

		// Stop rollups, otherwise we might end up overwriting some new data.
		n.stopTask(opRollup)
		defer n.startTask(opRollup)

		posting.Oracle().ResetTxns()
		schema.State().DeleteAll()

		if err := posting.DeleteAll(); err != nil {
			return err
		}

		// Clear entire cache.
		posting.ResetCache()

		if groups().groupId() == 1 {
			initialSchema := schema.InitialSchema(x.GalaxyNamespace)
			for _, s := range initialSchema {
				applySchema(s)
			}
		}

		// Propose initial types as well after a drop all as they would have been cleared.
		initialTypes := schema.InitialTypes(x.GalaxyNamespace)
		for _, t := range initialTypes {
			if err := updateType(t.GetTypeName(), *t); err != nil {
				return err
			}
		}

		return nil
	}

	if proposal.Mutations.DropOp == pb.Mutations_TYPE {
		n.keysWritten.rejectBeforeIndex = proposal.Index
		return schema.State().DeleteType(proposal.Mutations.DropValue)
	}

	if proposal.Mutations.StartTs == 0 {
		return errors.New("StartTs must be provided")
	}

	if len(proposal.Mutations.Schema) > 0 || len(proposal.Mutations.Types) > 0 {
		n.keysWritten.rejectBeforeIndex = proposal.Index

		// MaxAssigned would ensure that everything that's committed up until this point
		// would be picked up in building indexes. Any uncommitted txns would be cancelled
		// by detectPendingTxns below.
		startTs := posting.Oracle().MaxAssigned()

		span.Annotatef(nil, "Applying schema and types")
		for _, supdate := range proposal.Mutations.Schema {
			// We should not need to check for predicate move here.
			if err := detectPendingTxns(supdate.Predicate); err != nil {
				return err
			}
		}

		if err := runSchemaMutation(ctx, proposal.Mutations.Schema, startTs); err != nil {
			return err
		}

		// Clear the entire cache if there is a schema update because the index rebuild
		// will invalidate the state.
		if len(proposal.Mutations.Schema) > 0 {
			posting.ResetCache()
		}

		for _, tupdate := range proposal.Mutations.Types {
			if err := runTypeMutation(ctx, tupdate); err != nil {
				return err
			}
		}
		return nil
	}

	// Scheduler tracks tasks at subject, predicate level, so doing
	// schema stuff here simplies the design and we needn't worry about
	// serializing the mutations per predicate or schema mutations
	// We derive the schema here if it's not present
	// Since raft committed logs are serialized, we can derive
	// schema here without any locking

	// Stores a map of predicate and type of first mutation for each predicate.
	schemaMap := make(map[string]types.TypeID)
	for _, edge := range proposal.Mutations.Edges {
		if edge.Entity == 0 && bytes.Equal(edge.Value, []byte(x.Star)) {
			// We should only drop the predicate if there is no pending
			// transaction.
			if err := detectPendingTxns(edge.Attr); err != nil {
				span.Annotatef(nil, "Found pending transactions. Retry later.")
				return err
			}
			span.Annotatef(nil, "Deleting predicate: %s", edge.Attr)
			n.keysWritten.rejectBeforeIndex = proposal.Index
			return posting.DeletePredicate(ctx, edge.Attr)
		}
		// Don't derive schema when doing deletion.
		if edge.Op == pb.DirectedEdge_DEL {
			continue
		}
		if _, ok := schemaMap[edge.Attr]; !ok {
			schemaMap[edge.Attr] = posting.TypeID(edge)
		}
	}

	total := len(proposal.Mutations.Edges)

	// TODO: Active mutations values can go up or down but with
	// OpenCensus stats bucket boundaries start from 0, hence
	// recording negative and positive values skews up values.
	ostats.Record(ctx, x.ActiveMutations.M(int64(total)))
	defer func() {
		ostats.Record(ctx, x.ActiveMutations.M(int64(-total)))
	}()

	// Go through all the predicates and their first observed schema type. If we are unable to find
	// these predicates in the current schema state, add them to the schema state. Note that the
	// schema deduction is done by RDF/JSON chunker.
	for attr, storageType := range schemaMap {
		if _, err := schema.State().TypeOf(attr); err != nil {
			hint := pb.Metadata_DEFAULT
			if mutHint, ok := proposal.GetMutations().GetMetadata().GetPredHints()[attr]; ok {
				hint = mutHint
			}
			if err := createSchema(attr, storageType, hint); err != nil {
				return err
			}
		}
	}

	m := proposal.Mutations
	txn := posting.Oracle().GetTxn(m.StartTs)
	x.AssertTruef(txn != nil, "Unable to find txn with start ts: %d", m.StartTs)
	runs := atomic.AddInt32(&txn.Runs, 1)
	if runs <= 1 {
		// If we didn't have it in Oracle, then mutation workers won't be processing it either. So,
		// don't block on txn.ErrCh.
		err, ok := <-txn.ErrCh
		x.AssertTrue(ok)
		if err == nil && n.keysWritten.StillValid(txn) {
			span.Annotate(nil, "Mutation is still valid.")
			return nil
		}
		// If mutation is invalid or we got an error, reset the txn, so we can run again.
		txn = posting.Oracle().ResetTxn(m.StartTs)
		atomic.AddInt32(&txn.Runs, 1) // We have already run this once via serial loop.
	}

	// If we have an error, re-run this.
	span.Annotatef(nil, "Re-running mutation from applyCh. Runs: %d", runs)
	return n.concMutations(ctx, m, txn)
}

func (n *node) applyCommitted(proposal *pb.Proposal) error {
	key := proposal.Key
	ctx := n.Ctx(key)
	span := otrace.FromContext(ctx)
	span.Annotatef(nil, "node.applyCommitted Node id: %d. Group id: %d. Got proposal key: %d",
		n.Id, n.gid, key)
	if x.Debug {
		glog.Infof("applyCommitted: Proposal: %+v\n", proposal)
	}

	if proposal.Mutations != nil {
		// syncmarks for this shouldn't be marked done until it's committed.
		span.Annotate(nil, "Applying mutations")
		if x.Debug {
			glog.Infof("applyCommitted: Mutation: %+v\n", proposal.Mutations)
		}
		if err := n.applyMutations(ctx, proposal); err != nil {
			span.Annotatef(nil, "While applying mutations: %v", err)
			return err
		}

		span.Annotate(nil, "Done")
		return nil
	}

	switch {
	case len(proposal.Kv) > 0:
		return populateKeyValues(ctx, proposal.Kv)

	case proposal.State != nil:
		n.elog.Printf("Applying state for key: %s", key)
		// This state needn't be snapshotted in this group, on restart we would fetch
		// a state which is latest or equal to this.
		groups().applyState(groups().Node.Id, proposal.State)
		return nil

	case len(proposal.CleanPredicate) > 0:
		n.elog.Printf("Cleaning predicate: %s", proposal.CleanPredicate)
		end := time.Now().Add(10 * time.Second)
		for proposal.ExpectedChecksum > 0 && time.Now().Before(end) {
			cur := atomic.LoadUint64(&groups().membershipChecksum)
			if proposal.ExpectedChecksum == cur {
				break
			}
			time.Sleep(100 * time.Millisecond)
			glog.Infof("Waiting for checksums to match. Expected: %d. Current: %d\n",
				proposal.ExpectedChecksum, cur)
		}
		if time.Now().After(end) {
			glog.Warningf(
				"Giving up on predicate deletion: %q due to timeout. Wanted checksum: %d.",
				proposal.CleanPredicate, proposal.ExpectedChecksum)
			return nil
		}
		return posting.DeletePredicate(ctx, proposal.CleanPredicate)

	case proposal.Delta != nil:
		n.elog.Printf("Applying Oracle Delta for key: %d", key)
		if x.Debug {
			glog.Infof("applyCommitted: Delta: %+v\n", proposal.Delta)
		}
		return n.commitOrAbort(key, proposal.Delta)

	case proposal.Snapshot != nil:
		existing, err := n.Store.Snapshot()
		if err != nil {
			return err
		}
		snap := proposal.Snapshot
		if existing.Metadata.Index >= snap.Index {
			log := fmt.Sprintf("Skipping snapshot at %d, because found one at %d",
				snap.Index, existing.Metadata.Index)
			n.elog.Printf(log)
			glog.Info(log)
			return nil
		}
		n.elog.Printf("Creating snapshot: %+v", snap)
		glog.Infof("Creating snapshot at Index: %d, ReadTs: %d\n", snap.Index, snap.ReadTs)

		data, err := snap.Marshal()
		x.Check(err)
		for {
			// We should never let CreateSnapshot have an error.
			err := n.Store.CreateSnapshot(snap.Index, n.ConfState(), data)
			if err == nil {
				break
			}
			glog.Warningf("Error while calling CreateSnapshot: %v. Retrying...", err)
		}
		// We can now discard all invalid versions of keys below this ts.
		pstore.SetDiscardTs(snap.ReadTs)
		return nil
	case proposal.Restore != nil:
		// Enable draining mode for the duration of the restore processing.
		x.UpdateDrainingMode(true)
		defer x.UpdateDrainingMode(false)

		var err error
		var closer *z.Closer
		closer, err = n.startTask(opRestore)
		if err != nil {
			return errors.Wrapf(err, "cannot start restore task")
		}
		defer closer.Done()

		glog.Infof("Got restore proposal at Index:%d, ReadTs:%d",
			proposal.Index, proposal.Restore.RestoreTs)
		if err := handleRestoreProposal(ctx, proposal.Restore, proposal.Index); err != nil {
			return err
		}

		// Call commitOrAbort to update the group checksums.
		ts := proposal.Restore.RestoreTs
		return n.commitOrAbort(key, &pb.OracleDelta{
			Txns: []*pb.TxnStatus{
				{StartTs: ts, CommitTs: ts},
			},
		})

	case proposal.DeleteNs != nil:
		x.AssertTrue(proposal.DeleteNs.Namespace != x.GalaxyNamespace)
		n.elog.Printf("Deleting namespace: %d", proposal.DeleteNs.Namespace)
		return posting.DeleteNamespace(proposal.DeleteNs.Namespace)

	case proposal.CdcState != nil:
		n.cdcTracker.updateCDCState(proposal.CdcState)
		return nil
	}
	x.Fatalf("Unknown proposal: %+v", proposal)
	return nil
}

func (n *node) processTabletSizes() {
	defer n.closer.Done()                   // CLOSER:1
	tick := time.NewTicker(5 * time.Minute) // Once every 5 minutes seems alright.
	defer tick.Stop()

	for {
		select {
		case <-n.closer.HasBeenClosed():
			return
		case <-tick.C:
			n.calculateTabletSizes()
		}
	}
}

func getProposal(e raftpb.Entry) pb.Proposal {
	var p pb.Proposal
	key := binary.BigEndian.Uint64(e.Data[:8])
	x.Check(p.Unmarshal(e.Data[8:]))
	p.Key = key
	p.Index = e.Index
	switch {
	case p.Mutations != nil:
		p.StartTs = p.Mutations.StartTs
	case p.Snapshot != nil:
		p.StartTs = p.Snapshot.ReadTs
	case p.Delta != nil:
		p.StartTs = 0 // Run this asap.
	default:
		// For now, not covering everything.
	}
	return p
}

func (n *node) processApplyCh() {
	defer n.closer.Done() // CLOSER:1

	type P struct {
		err  error
		size int
		seen time.Time
	}
	previous := make(map[uint64]*P)

	// This function must be run serially.
	handle := func(prop pb.Proposal) {
		var perr error
		prev, ok := previous[prop.Key]
		if ok && prev.err == nil {
			n.elog.Printf("Proposal with key: %d already applied. Skipping index: %d.\n",
				prop.Key, prop.Index)
			previous[prop.Key].seen = time.Now() // Update the ts.
			// Don't break here. We still need to call the Done below.

		} else {
			if max := posting.Oracle().MaxAssigned(); prop.StartTs > max {
				// Wait to run this proposal.
				if x.Debug {
					glog.Infof("start ts: %d max: %d. Pushing to pending.\n", prop.StartTs, max)
				}
				n.pendingProposals = append(n.pendingProposals, prop)
				return
			}

			// if this applyCommited fails, how do we ensure
			start := time.Now()
			perr = n.applyCommitted(&prop)
			if prop.Key != 0 {
				p := &P{err: perr, seen: time.Now()}
				previous[prop.Key] = p
			}
			if perr != nil {
				glog.Errorf("Applying proposal. Error: %v. Proposal: %q.", perr,
					getSanitizedString(&prop))
			}
			n.elog.Printf("Applied proposal with key: %d, index: %d. Err: %v",
				prop.Key, prop.Index, perr)

			var tags []tag.Mutator
			switch {
			case prop.Mutations != nil:
				if len(prop.Mutations.Schema) == 0 {
					// Don't capture schema updates.
					tags = append(tags, tag.Upsert(x.KeyMethod, "apply.Mutations"))
				}
			case prop.Delta != nil:
				tags = append(tags, tag.Upsert(x.KeyMethod, "apply.Delta"))
			}
			ms := x.SinceMs(start)
			if err := ostats.RecordWithTags(context.Background(),
				tags, x.LatencyMs.M(ms)); err != nil {
				glog.Errorf("Error recording stats: %+v", err)
			}
		}

		n.Proposals.Done(prop.Key, perr)
		n.Applied.Done(prop.Index)
		ostats.Record(context.Background(), x.RaftAppliedIndex.M(int64(n.Applied.DoneUntil())))
	}

	loopOverPending := func(maxAssigned uint64) {
		idx := 0
		for idx < len(n.pendingProposals) {
			p := n.pendingProposals[idx]
			if maxAssigned >= p.StartTs {
				handle(p)
				n.pendingProposals = append(n.pendingProposals[:idx], n.pendingProposals[idx+1:]...)
			} else {
				idx++
			}
		}
	}

	maxAge := 2 * time.Minute
	tick := time.NewTicker(maxAge / 2)
	defer tick.Stop()

	var counter int
	var maxAssigned uint64
	orc := posting.Oracle()
	for {
		select {
		case <-n.drainApplyCh:
			numDrained := 0
			for _, p := range n.pendingProposals {
				numDrained++
				n.Proposals.Done(p.Key, nil)
				n.Applied.Done(p.Index)
			}
			n.pendingProposals = n.pendingProposals[:0]

			var done bool
			for !done {
				select {
				case entries := <-n.applyCh:
					numDrained += len(entries)
					for _, entry := range entries {
						key := binary.BigEndian.Uint64(entry.Data[:8])
						n.Proposals.Done(key, nil)
						n.Applied.Done(entry.Index)
					}
				default:
					done = true
				}
			}
			glog.Infof("Drained %d entries. Size of applyCh: %d\n", numDrained, len(n.applyCh))

		case entries, ok := <-n.applyCh:
			if !ok {
				return
			}
			var totalSize int64
			for _, e := range entries {
				x.AssertTrue(len(e.Data) > 0)
				p := getProposal(e)
				handle(p)

				if p.Delta != nil && len(n.pendingProposals) > 0 {
					// MaxAssigned would only change during deltas.
					if max := orc.MaxAssigned(); max > maxAssigned {
						loopOverPending(max)
						maxAssigned = max
					}
				}
				totalSize += int64(e.Size())
			}
			if sz := atomic.AddInt64(&n.pendingSize, -totalSize); sz < 0 {
				glog.Warningf("Pending size should remain above zero: %d", sz)
			}

		case <-tick.C:
			// We use this ticker to clear out previous map.
			counter++
			now := time.Now()
			for key, p := range previous {
				if now.Sub(p.seen) > maxAge {
					delete(previous, key)
				}
			}
			n.elog.Printf("Size of previous map: %d", len(previous))

			kw := n.keysWritten
			minSeen := posting.Oracle().MinMaxAssignedSeenTs()
			before := len(kw.keyCommitTs)
			for k, commitTs := range kw.keyCommitTs {
				// If commitTs is less than the min of all pending Txn's MaxAssignedSeen, then we
				// can safely delete the key. StillValid would only consider the commits with ts >
				// MaxAssignedSeen.
				if commitTs < minSeen {
					delete(kw.keyCommitTs, k)
				}
			}
			if counter%5 == 0 {
				// Once in 5 minutes.
				glog.V(2).Infof("Still valid: %d Invalid: %d. Size of commit map: %d -> %d."+
					" Total keys written: %d\n",
					kw.validTxns, kw.invalidTxns, before, len(kw.keyCommitTs), kw.totalKeys)
			}
		}
	}
}

func (n *node) commitOrAbort(_ uint64, delta *pb.OracleDelta) error {
	_, span := otrace.StartSpan(context.Background(), "node.commitOrAbort")
	defer span.End()

	span.Annotate(nil, "Start")
	start := time.Now()
	var numKeys int

	itrStart := time.Now()
	var itrs []y.Iterator
	var txns []*posting.Txn
	var sz int64
	for _, status := range delta.Txns {
		txn := posting.Oracle().GetTxn(status.StartTs)
		if txn == nil {
			continue
		}
		for k := range txn.Deltas() {
			n.keysWritten.keyCommitTs[z.MemHashString(k)] = status.CommitTs
		}
		n.keysWritten.totalKeys += len(txn.Deltas())
		numKeys += len(txn.Deltas())
		if len(txn.Deltas()) == 0 {
			continue
		}
		txns = append(txns, txn)

		sz += txn.Skiplist().MemSize()
		// Iterate to set the commit timestamp for all keys.
		// Skiplist can block if the conversion to Skiplist isn't done yet.
		itr := txn.Skiplist().NewIterator()
		itr.SeekToFirst()
		for itr.Valid() {
			key := itr.Key()
			// We don't expect the ordering of the keys to change due to setting their commit
			// timestamps. Each key in the skiplist should be unique already.
			y.SetKeyTs(key, status.CommitTs)
			itr.Next()
		}
		itr.Close()

		itrs = append(itrs, txn.Skiplist().NewUniIterator(false))
	}
	span.Annotatef(nil, "Num keys: %d Itr: %s\n", numKeys, time.Since(itrStart))
	ostats.Record(n.ctx, x.NumEdges.M(int64(numKeys)))

	// This would be used for callback via Badger when skiplist is pushed to
	// disk.
	deleteTxns := func() {
		posting.Oracle().DeleteTxns(delta)
	}

	if len(itrs) == 0 {
		deleteTxns()

	} else {
		sn := time.Now()
		mi := table.NewMergeIterator(itrs, false)
		mi.Rewind()

		var keys int
		b := skl.NewBuilder(int64(float64(sz) * 1.1))
		for mi.Valid() {
			b.Add(mi.Key(), mi.Value())
			keys++
			mi.Next()
		}
		span.Annotatef(nil, "Iterating and skiplist over %d keys took: %s", keys, time.Since(sn))
		err := x.RetryUntilSuccess(3600, time.Second, func() error {
			if numKeys == 0 {
				return nil
			}
			// We do the pending txn deletion in the callback, so that our snapshot and checkpoint
			// tracking would only consider the txns which have been successfully pushed to disk.
			return pstore.HandoverSkiplist(b.Skiplist(), deleteTxns)
		})
		if err != nil {
			glog.Errorf("while handing over skiplist: %v\n", err)
		}
		span.Annotatef(nil, "Handover skiplist done for %d txns, %d keys", len(delta.Txns), numKeys)
	}

	ms := x.SinceMs(start)
	tags := []tag.Mutator{tag.Upsert(x.KeyMethod, "apply.toDisk")}
	x.Check(ostats.RecordWithTags(context.Background(), tags, x.LatencyMs.M(ms)))

	// Before, we used to call pstore.Sync() here. We don't need to do that
	// anymore because we're not using Badger's WAL.

	g := groups()
	if delta.GroupChecksums != nil && delta.GroupChecksums[g.groupId()] > 0 {
		atomic.StoreUint64(&g.deltaChecksum, delta.GroupChecksums[g.groupId()])
	}

	// Clear all the cached lists that were touched by this transaction.
	for _, status := range delta.Txns {
		txn := posting.Oracle().GetTxn(status.StartTs)
		txn.RemoveCachedKeys()
	}
	posting.WaitForCache()
	span.Annotate(nil, "cache keys removed")

	// Now advance Oracle(), so we can service waiting reads.
	posting.Oracle().ProcessDelta(delta)
	span.Annotate(nil, "process delta done")
	return nil
}

func (n *node) leaderBlocking() (*conn.Pool, error) {
	pool := groups().Leader(groups().groupId())
	if pool == nil {
		// Functions like retrieveSnapshot and joinPeers are blocking at initial start and
		// leader election for a group might not have happened when it is called. If we can't
		// find a leader, get latest state from Zero.
		if err := UpdateMembershipState(context.Background()); err != nil {
			return nil, errors.Errorf("Error while trying to update membership state: %+v", err)
		}
		return nil, errors.Errorf("Unable to reach leader in group %d", n.gid)
	}
	return pool, nil
}

func (n *node) Snapshot() (*pb.Snapshot, error) {
	if n == nil || n.Store == nil {
		return nil, conn.ErrNoNode
	}
	snap, err := n.Store.Snapshot()
	if err != nil {
		return nil, err
	}
	res := &pb.Snapshot{}
	if err := res.Unmarshal(snap.Data); err != nil {
		return nil, err
	}
	return res, nil
}

func (n *node) retrieveSnapshot(snap pb.Snapshot) error {
	closer, err := n.startTask(opSnapshot)
	if err != nil {
		return err
	}
	defer closer.Done()

	// In some edge cases, the Zero leader might not have been able to update
	// the status of Alpha leader. So, instead of blocking forever on waiting
	// for Zero to send us the updates info about the leader, we can just use
	// the Snapshot RaftContext, which contains the address of the leader.
	var pool *conn.Pool
	addr := snap.Context.GetAddr()
	glog.V(2).Infof("Snapshot.RaftContext.Addr: %q", addr)
	if len(addr) > 0 {
		p, err := conn.GetPools().Get(addr)
		if err != nil {
			glog.V(2).Infof("conn.Get(%q) Error: %v", addr, err)
		} else {
			pool = p
			glog.V(2).Infof("Leader connection picked from RaftContext")
		}
	}
	if pool == nil {
		glog.V(2).Infof("No leader conn from RaftContext. Using membership state.")
		p, err := n.leaderBlocking()
		if err != nil {
			return err
		}
		pool = p
	}

	// Need to clear pl's stored in memory for the case when retrieving snapshot with
	// index greater than this node's last index
	// Should invalidate/remove pl's to this group only ideally
	//
	// We can safely evict posting lists from memory. Because, all the updates corresponding to txn
	// commits up until then have already been written to pstore. And the way we take snapshots, we
	// keep all the pre-writes for a pending transaction, so they will come back to memory, as Raft
	// logs are replayed.
	if err := n.populateSnapshot(snap, pool); err != nil {
		return errors.Wrapf(err, "cannot retrieve snapshot from peer")
	}
	// Populate shard stores the streamed data directly into db, so we need to refresh
	// schema for current group id
	if err := schema.LoadFromDb(); err != nil {
		return errors.Wrapf(err, "while initializing schema")
	}
	groups().triggerMembershipSync()
	// We set MaxAssignedTs to avoid this case. Right after snapshot, say we have mutation and its
	// commit. Without a MaxAssigned >= mutation.StartTs, we would enqueue it in pendingProposals.
	// But, then go an execute its commit. That would result in mutation loss. To avoid that, we
	// calculate the MaxAssigned and set it corresponding to the snapshot. So, we can apply the
	// mutation before its commit when we replay logs.
	posting.Oracle().SetMaxAssigned(snap.MaxAssigned)
	return nil
}

func (n *node) proposeCDCState(ts uint64) error {
	proposal := &pb.Proposal{
		CdcState: &pb.CDCState{
			SentTs: ts,
		},
	}
	glog.V(2).Infof("Proposing new CDC state ts: %d\n", ts)
	data := make([]byte, 8+proposal.Size())
	sz, err := proposal.MarshalToSizedBuffer(data[8:])
	data = data[:8+sz]
	x.Check(err)
	return n.Raft().Propose(n.ctx, data)
}

func (n *node) proposeSnapshot() error {
	lastIdx := x.Min(n.Applied.DoneUntil(), n.cdcTracker.getSeenIndex())
	// We can't rely upon the Raft entries to determine the minPendingStart,
	// because there are many cases during mutations where we don't commit or
	// abort the transaction. This might happen due to an early error thrown.
	// Only the mutations which make it to Zero for a commit/abort decision have
	// corresponding Delta entries. So, instead of replicating all that logic
	// here, we just use the MinPendingStartTs tracked by the Oracle, and look
	// for that in the logs.
	//
	// So, we iterate over logs. If we hit MinPendingStartTs, that generates our
	// snapshotIdx. In any case, we continue picking up txn updates, to generate
	// a maxCommitTs, which would become the readTs for the snapshot.
	minPendingStart := x.Min(posting.Oracle().MinPendingStartTs(), n.cdcTracker.getTs())
	snap, err := n.calculateSnapshot(0, lastIdx, minPendingStart)
	if err != nil {
		return err
	}
	if snap == nil {
		return nil
	}
	proposal := &pb.Proposal{
		Snapshot: snap,
	}
	glog.V(2).Infof("Proposing snapshot: %+v\n", snap)
	data := make([]byte, 8+proposal.Size())
	sz, err := proposal.MarshalToSizedBuffer(data[8:])
	data = data[:8+sz]
	x.Check(err)
	return n.Raft().Propose(n.ctx, data)
}

const (
	maxPendingSize int64 = 256 << 20 // in bytes.
	nodeApplyChan        = "pushing to raft node applyCh"
)

func rampMeter(address *int64, maxSize int64, component string) {
	start := time.Now()
	defer func() {
		if dur := time.Since(start); dur > time.Second {
			glog.Infof("Blocked %s for %v", component, dur.Round(time.Millisecond))
		}
	}()
	for {
		if atomic.LoadInt64(address) <= maxSize {
			return
		}
		time.Sleep(3 * time.Millisecond)
	}
}

func (n *node) updateRaftProgress() error {
	// Both leader and followers can independently update their Raft progress. We don't store
	// this in Raft WAL. Instead, this is used to just skip over log records that this Alpha
	// has already applied, to speed up things on a restart.
	//
	// Let's check what we already have. And only update if the new snap.Index is ahead of the last
	// stored applied.
	applied := n.Store.Uint(raftwal.CheckpointIndex)

	snap, err := n.calculateSnapshot(applied, n.Applied.DoneUntil(),
		posting.Oracle().MinPendingStartTs())
	if err != nil || snap == nil || snap.Index <= applied {
		return err
	}
	atomic.StoreUint64(&n.checkpointTs, snap.ReadTs)

	n.Store.SetUint(raftwal.CheckpointIndex, snap.GetIndex())
	glog.V(2).Infof("[%#x] Set Raft checkpoint to index: %d, ts: %d.",
		n.Id, snap.Index, snap.ReadTs)
	return nil
}

func (n *node) checkpointAndClose(done chan struct{}) {
	snapshotAfterEntries := x.WorkerConfig.Raft.GetUint64("snapshot-after-entries")
	x.AssertTruef(snapshotAfterEntries > 10, "raft.snapshot-after must be a number greater than 10")

	slowTicker := time.NewTicker(time.Minute)
	defer slowTicker.Stop()

	exceededSnapshotByEntries := func() bool {
		if snapshotAfterEntries == 0 {
			// If snapshot-after isn't set, return true always.
			return true
		}
		chk, err := n.Store.Checkpoint()
		if err != nil {
			glog.Errorf("While reading checkpoint: %v", err)
			return false
		}
		first, err := n.Store.FirstIndex()
		if err != nil {
			glog.Errorf("While reading first index: %v", err)
			return false
		}
		// If we're over snapshotAfterEntries, calculate would be true.
		glog.V(3).Infof("Evaluating snapshot first:%d chk:%d (chk-first:%d) "+
			"snapshotAfterEntries:%d", first, chk, chk-first,
			snapshotAfterEntries)
		return chk-first > snapshotAfterEntries
	}

	lastSnapshotTime := time.Now()
	snapshotFrequency := x.WorkerConfig.Raft.GetDuration("snapshot-after-duration")
	for {
		select {
		case <-slowTicker.C:
			// Do these operations asynchronously away from the main Run loop to allow heartbeats to
			// be sent on time. Otherwise, followers would just keep running elections.

			n.elog.Printf("Size of applyCh: %d", len(n.applyCh))
			if err := n.updateRaftProgress(); err != nil {
				glog.Errorf("While updating Raft progress: %v", err)
			}

			if n.AmLeader() {
				// If leader doesn't have a snapshot, we should create one immediately. This is very
				// useful when you bring up the cluster from bulk loader. If you remove an alpha and
				// add a new alpha, the new follower won't get a snapshot if the leader doesn't have
				// one.
				snap, err := n.Store.Snapshot()
				if err != nil {
					glog.Errorf("While retrieving snapshot from Store: %v\n", err)
					continue
				}

				// calculate would be true if:
				// - snapshot is empty.
				// - we have more than 4 log files in Raft WAL.
				// - snapshot frequency is zero and exceeding threshold entries.
				// - we have exceeded the threshold time since last snapshot and exceeding threshold
				//   entries.
				//
				// Note: In case we're exceeding threshold entries, but have not exceeded the
				// threshold time since last snapshot, calculate would be false.
				calculate := raft.IsEmptySnap(snap) || n.Store.NumLogFiles() > 4
				if snapshotFrequency == 0 {
					calculate = calculate || exceededSnapshotByEntries()

				} else if time.Since(lastSnapshotTime) > snapshotFrequency {
					// If we haven't taken a snapshot since snapshotFrequency, calculate would
					// follow snapshot entries.
					calculate = calculate || exceededSnapshotByEntries()
				}

				// Check if we're snapshotAfterEntries away from the FirstIndex based off the last
				// checkpoint. This is a cheap operation. We're not iterating over the logs.
				if chk, err := n.Store.Checkpoint(); err == nil {
					if first, err := n.Store.FirstIndex(); err == nil {
						// If we're over snapshotAfterEntries, calculate would be true.
						if snapshotAfterEntries > 0 {
							calculate = calculate || chk >= first+snapshotAfterEntries
							glog.V(3).Infof("Evaluating snapshot first:%d chk:%d (chk-first:%d) "+
								"snapshotAfterEntries:%d snap:%v", first, chk, chk-first,
								snapshotAfterEntries, calculate)
						}
					}
				}

				// We keep track of the applied index in the p directory. Even if we don't take
				// snapshot for a while and let the Raft logs grow and restart, we would not have to
				// run all the log entries, because we can tell Raft.Config to set Applied to that
				// index.
				// This applied index tracking also covers the case when we have a big index
				// rebuild. The rebuild would be tracked just like others and would not need to be
				// replayed after a restart, because the Applied config would let us skip right
				// through it.
				// We use disk based storage for Raft. So, we're not too concerned about
				// snapshotting.  We just need to do enough, so that we don't have a huge backlog of
				// entries to process on a restart.
				if calculate {
					// We can set discardN argument to zero, because we already know that calculate
					// would be true if either we absolutely needed to calculate the snapshot,
					// or our checkpoint already crossed the SnapshotAfter threshold.
					if err := n.proposeSnapshot(); err != nil {
						glog.Errorf("While calculating and proposing snapshot: %v", err)
					} else {
						lastSnapshotTime = time.Now()
					}
				}
				go n.abortOldTransactions()
			}

		case <-n.closer.HasBeenClosed():
			glog.Infof("Stopping node.Run")
			if peerId, has := groups().MyPeer(); has && n.AmLeader() {
				n.Raft().TransferLeadership(n.ctx, n.Id, peerId)
				time.Sleep(time.Second) // Let transfer happen.
			}
			n.Raft().Stop()
			close(done)
			return
		}
	}
}

const tickDur = 100 * time.Millisecond

func (n *node) Run() {
	defer n.closer.Done() // CLOSER:1

	// lastLead is for detecting leadership changes
	//
	// etcd has a similar mechanism for tracking leader changes, with their
	// raftReadyHandler.getLead() function that returns the previous leader
	lastLead := uint64(math.MaxUint64)

	firstRun := true
	var leader bool
	// See also our configuration of HeartbeatTick and ElectionTick.
	// Before we used to have 20ms ticks, but they would overload the Raft tick channel, causing
	// "tick missed to fire" logs. Etcd uses 100ms and they haven't seen those issues.
	// Additionally, using 100ms for ticks does not cause proposals to slow down, because they get
	// sent out asap and don't rely on ticks. So, setting this to 100ms instead of 20ms is a NOOP.
	ticker := time.NewTicker(tickDur)
	defer ticker.Stop()

	done := make(chan struct{})
	go n.checkpointAndClose(done)
	go n.ReportRaftComms()

	if !x.WorkerConfig.HardSync {
		closer := z.NewCloser(2)
		defer closer.SignalAndWait()
		go x.StoreSync(n.Store, closer)
		go x.StoreSync(pstore, closer)
	}

	applied, err := n.Store.Checkpoint()
	if err != nil {
		glog.Errorf("While trying to find raft progress: %v", err)
	} else {
		glog.Infof("Found Raft checkpoint: %d", applied)
	}

	var timer x.Timer
	for {
		select {
		case <-done:
			// We use done channel here instead of closer.HasBeenClosed so that we can transfer
			// leadership in a goroutine. The push to n.applyCh happens in this loop, so the close
			// should happen here too. Otherwise, race condition between push and close happens.
			close(n.applyCh)
			glog.Infoln("Raft node done.")
			return

			// Slow ticker can't be placed here because figuring out checkpoints and snapshots takes
			// time and if the leader does not send heartbeats out during this time, the followers
			// start an election process. And that election process would just continue to happen
			// indefinitely because checkpoints and snapshots are being calculated indefinitely.
		case <-ticker.C:
			n.Raft().Tick()

		case rd := <-n.Raft().Ready():
			timer.Start()
			_, span := otrace.StartSpan(n.ctx, "Alpha.RunLoop",
				otrace.WithSampler(otrace.ProbabilitySampler(0.001)))

			if rd.SoftState != nil {
				groups().triggerMembershipSync()
				leader = rd.RaftState == raft.StateLeader
				// create context with group id
				ctx, _ := tag.New(n.ctx, tag.Upsert(x.KeyGroup, fmt.Sprintf("%d", n.gid)))
				// detect leadership changes
				if rd.SoftState.Lead != lastLead {
					lastLead = rd.SoftState.Lead
					ostats.Record(ctx, x.RaftLeaderChanges.M(1))
				}
				if rd.SoftState.Lead != raft.None {
					ostats.Record(ctx, x.RaftHasLeader.M(1))
				} else {
					ostats.Record(ctx, x.RaftHasLeader.M(0))
				}
				if leader {
					ostats.Record(ctx, x.RaftIsLeader.M(1))
				} else {
					ostats.Record(ctx, x.RaftIsLeader.M(0))
				}
			}
			if leader {
				// Leader can send messages in parallel with writing to disk.
				for i := range rd.Messages {
					// NOTE: We can do some optimizations here to drop messages.
					n.Send(&rd.Messages[i])
				}
			}
			if span != nil {
				span.Annotate(nil, "Handled ReadStates and SoftState.")
			}

			// We move the retrieval of snapshot before we store the rd.Snapshot, so that in case
			// this node fails to get the snapshot, the Raft state would reflect that by not having
			// the snapshot on a future probe. This is different from the recommended order in Raft
			// docs where they assume that the Snapshot contains the full data, so even on a crash
			// between n.SaveToStorage and n.retrieveSnapshot, that Snapshot can be applied by the
			// node on a restart. In our case, we don't store the full data in snapshot, only the
			// metadata.  So, we should only store the snapshot received in Raft, iff we actually
			// were able to update the state.
			if !raft.IsEmptySnap(rd.Snapshot) {
				// We don't send snapshots to other nodes. But, if we get one, that means
				// either the leader is trying to bring us up to state; or this is the
				// snapshot that I created. Only the former case should be handled.
				var snap pb.Snapshot
				x.Check(snap.Unmarshal(rd.Snapshot.Data))
				rc := snap.GetContext()
				x.AssertTrue(rc.GetGroup() == n.gid)
				if rc.Id != n.Id {
					// Set node to unhealthy state here while it applies the snapshot.
					x.UpdateHealthStatus(false)

					// We are getting a new snapshot from leader. We need to wait for the applyCh to
					// finish applying the updates, otherwise, we'll end up overwriting the data
					// from the new snapshot that we retrieved.

					// Drain the apply channel. Snapshot will be retrieved next.
					maxIndex := n.Applied.LastIndex()
					glog.Infof("Drain applyCh by reaching %d before"+
						" retrieving snapshot\n", maxIndex)
					n.drainApplyCh <- struct{}{}

					if err := n.Applied.WaitForMark(context.Background(), maxIndex); err != nil {
						glog.Errorf("Error waiting for mark for index %d: %+v", maxIndex, err)
					}

					if currSnap, err := n.Snapshot(); err != nil {
						// Retrieve entire snapshot from leader if node does not have
						// a current snapshot.
						glog.Errorf("Could not retrieve previous snapshot. Setting SinceTs to 0.")
						snap.SinceTs = 0
					} else {
						snap.SinceTs = currSnap.ReadTs
					}

					// It's ok to block ticks while retrieving snapshot, since it's a follower.
					glog.Infof("---> SNAPSHOT: %+v. Group %d from node id %#x\n",
						snap, n.gid, rc.Id)

					for {
						err := n.retrieveSnapshot(snap)
						if err == nil {
							glog.Infoln("---> Retrieve snapshot: OK.")
							break
						}
						glog.Errorf("While retrieving snapshot, error: %v. Retrying...", err)
						time.Sleep(time.Second) // Wait for a bit.
					}
					glog.Infof("---> SNAPSHOT: %+v. Group %d. DONE.\n", snap, n.gid)

					// Set node to healthy state here.
					x.UpdateHealthStatus(true)
				} else {
					glog.Infof("---> SNAPSHOT: %+v. Group %d from node id %#x [SELF]. Ignoring.\n",
						snap, n.gid, rc.Id)
				}
				if span != nil {
					span.Annotate(nil, "Applied or retrieved snapshot.")
				}
			}

			// Store the hardstate and entries. Note that these are not CommittedEntries.
			n.SaveToStorage(&rd.HardState, rd.Entries, &rd.Snapshot)
			timer.Record("disk")
			if span != nil {
				span.Annotatef(nil, "Saved %d entries. Snapshot, HardState empty? (%v, %v)",
					len(rd.Entries),
					raft.IsEmptySnap(rd.Snapshot),
					raft.IsEmptyHardState(rd.HardState))
			}
			for x.WorkerConfig.HardSync && rd.MustSync {
				if err := n.Store.Sync(); err != nil {
					glog.Errorf("Error while calling Store.Sync: %+v", err)
					time.Sleep(10 * time.Millisecond)
					continue
				}
				timer.Record("sync")
				break
			}

			// Now schedule or apply committed entries.
			var entries []raftpb.Entry
			for _, entry := range rd.CommittedEntries {
				// Need applied watermarks for schema mutation also for read linearazibility
				// Applied watermarks needs to be emitted as soon as possible sequentially.
				// If we emit Mark{4, false} and Mark{4, true} before emitting Mark{3, false}
				// then doneUntil would be set as 4 as soon as Mark{4,true} is done and before
				// Mark{3, false} is emitted. So it's safer to emit watermarks as soon as
				// possible sequentially
				n.Applied.Begin(entry.Index)

				switch {
				case entry.Type == raftpb.EntryConfChange:
					n.applyConfChange(entry)
					// Not present in proposal map.
					n.Applied.Done(entry.Index)
					groups().triggerMembershipSync()
				case len(entry.Data) == 0:
					n.elog.Printf("Found empty data at index: %d", entry.Index)
					n.Applied.Done(entry.Index)
				case entry.Index < applied:
					n.elog.Printf("Skipping over already applied entry: %d", entry.Index)
					n.Applied.Done(entry.Index)
				default:
					key := binary.BigEndian.Uint64(entry.Data[:8])
					if pctx := n.Proposals.Get(key); pctx != nil {
						atomic.AddUint32(&pctx.Found, 1)
						if span := otrace.FromContext(pctx.Ctx); span != nil {
							span.Annotate(nil, "Proposal found in CommittedEntries")
						}
					}
					entries = append(entries, entry)
				}
			}
			// Send the whole lot to applyCh in one go, instead of sending proposals one by one.
			if len(entries) > 0 {
				// Apply the meter this before adding size to pending size so some crazy big
				// proposal can be pushed to applyCh. If this do this after adding its size to
				// pending size, we could block forever in rampMeter.
				rampMeter(&n.pendingSize, maxPendingSize, nodeApplyChan)
				var pendingSize int64
				for _, e := range entries {
					pendingSize += int64(e.Size())
				}
				if sz := atomic.AddInt64(&n.pendingSize, pendingSize); sz > 2*maxPendingSize {
					glog.Warningf("Inflight proposal size: %d. There would be some throttling.", sz)
				}

				for _, e := range entries {
					p := getProposal(e)
					if len(p.Mutations.GetEdges()) == 0 {
						continue
					}
					var skip bool
					for _, e := range p.Mutations.GetEdges() {
						// This is a drop predicate mutation. We should not try to execute it
						// concurrently.
						if e.Entity == 0 && bytes.Equal(e.Value, []byte(x.Star)) {
							skip = true
							break
						}
					}
					if skip {
						continue
					}
					// We should register this txn before sending it over for concurrent
					// application.
					txn, has := posting.Oracle().RegisterStartTs(p.StartTs)
					if x.Debug {
						glog.Infof("Registered start ts: %d txn: %p. has: %v. mutation: %+v\n",
							p.StartTs, txn, has, p.Mutations)
					}

					if has {
						// We have already registered this txn before. That means, this txn would
						// either have already been run via apply channel, or would be on its way.
						// It could even be currently being executed via concurrent mutation
						// workers.  Moreover, in concurrent execution, when MaxAssigned <
						// txn.StartTs, we might have to waste the work done, and reset the txn.
						// To avoid edge cases, it is just simpler to NOT run the txn mutation
						// concurrently.
						// There's an optimization here where if startTs < MaxAssigned, then we
						// could run it concurrently. But, we won't use that to avoid complexity of
						// figuring out whether we set it up for concurrent execution or serial.
					} else {
						n.concApplyCh <- &p
					}
				}
				n.applyCh <- entries
			}

			if span != nil {
				span.Annotatef(nil, "Handled %d committed entries.", len(rd.CommittedEntries))
			}

			if !leader {
				// Followers should send messages later.
				for i := range rd.Messages {
					// NOTE: We can do some optimizations here to drop messages.
					n.Send(&rd.Messages[i])
				}
			}
			if span != nil {
				span.Annotate(nil, "Followed queued messages.")
			}
			timer.Record("proposals")

			n.Raft().Advance()
			timer.Record("advance")

			if firstRun && n.canCampaign {
				go func() {
					if err := n.Raft().Campaign(n.ctx); err != nil {
						glog.Errorf("Error starting campaign for node %v: %+v", n.gid, err)
					}
				}()
				firstRun = false
			}
			if span != nil {
				span.Annotate(nil, "Advanced Raft. Done.")
				span.End()
				if err := ostats.RecordWithTags(context.Background(),
					[]tag.Mutator{tag.Upsert(x.KeyMethod, "alpha.RunLoop")},
					x.LatencyMs.M(float64(timer.Total())/1e6)); err != nil {
					glog.Errorf("Error recording stats: %+v", err)
				}
			}
			if timer.Total() > 5*tickDur {
				glog.Warningf(
					"Raft.Ready took too long to process: %s"+
						" Num entries: %d. MustSync: %v",
					timer.String(), len(rd.Entries), rd.MustSync)
			}
		}
	}
}

func listWrap(kv *bpb.KV) *bpb.KVList {
	return &bpb.KVList{Kv: []*bpb.KV{kv}}
}

// calculateTabletSizes updates the tablet sizes for the keys.
func (n *node) calculateTabletSizes() {
	if !n.AmLeader() {
		// Only leader sends the tablet size updates to Zero. No one else does.
		return
	}
	var total int64
	tablets := make(map[string]*pb.Tablet)
	updateSize := func(tinfo badger.TableInfo) {
		// The error has already been checked by caller.
		left, _ := x.Parse(tinfo.Left)
		pred := left.Attr
		if pred == "" {
			return
		}
		if tablet, ok := tablets[pred]; ok {
			tablet.OnDiskBytes += int64(tinfo.OnDiskSize)
			tablet.UncompressedBytes += int64(tinfo.UncompressedSize)
		} else {
			tablets[pred] = &pb.Tablet{
				GroupId:           n.gid,
				Predicate:         pred,
				OnDiskBytes:       int64(tinfo.OnDiskSize),
				UncompressedBytes: int64(tinfo.UncompressedSize),
			}
		}
		total += int64(tinfo.OnDiskSize)
	}

	tableInfos := pstore.Tables()
	glog.V(2).Infof("Calculating tablet sizes. Found %d tables\n", len(tableInfos))
	for _, tinfo := range tableInfos {
		left, err := x.Parse(tinfo.Left)
		if err != nil {
			glog.V(3).Infof("Unable to parse key: %v", err)
			continue
		}
		right, err := x.Parse(tinfo.Right)
		if err != nil {
			glog.V(3).Infof("Unable to parse key: %v", err)
			continue
		}

		// Count the table only if it is occupied by a single predicate.
		if left.Attr == right.Attr {
			updateSize(tinfo)
		} else {
			glog.V(3).Info("Skipping table not owned by one predicate")
		}
	}

	if len(tablets) == 0 {
		glog.V(2).Infof("No tablets found.")
		return
	}
	// Update Zero with the tablet sizes. If Zero sees a tablet which does not belong to
	// this group, it would send instruction to delete that tablet. There's an edge case
	// here if the followers are still running Rollup, and happen to read a key before and
	// write after the tablet deletion, causing that tablet key to resurface. Then, only the
	// follower would have that key, not the leader.
	// However, if the follower then becomes the leader, we'd be able to get rid of that
	// key then. Alternatively, we could look into cancelling the Rollup if we see a
	// predicate deletion.
	if err := groups().doSendMembership(tablets); err != nil {
		glog.Warningf("While sending membership to Zero. Error: %v", err)
	} else {
		glog.V(2).Infof("Sent tablet size update to Zero. Total size: %s",
			humanize.Bytes(uint64(total)))
	}
}

var errNoConnection = errors.New("No connection exists")

func (n *node) blockingAbort(req *pb.TxnTimestamps) error {
	pl := groups().Leader(0)
	if pl == nil {
		return errNoConnection
	}
	zc := pb.NewZeroClient(pl.Get())
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	delta, err := zc.TryAbort(ctx, req)
	glog.Infof("TryAbort %d txns with start ts. Error: %v\n", len(req.Ts), err)
	if err != nil || len(delta.Txns) == 0 {
		return err
	}

	// Let's propose the txn updates received from Zero. This is important because there are edge
	// cases where a txn status might have been missed by the group.
	aborted := &pb.OracleDelta{}
	for _, txn := range delta.Txns {
		// Only pick the aborts. DO NOT propose the commits. They must come in the right order via
		// oracle delta stream, otherwise, we'll end up losing some committed txns.
		if txn.CommitTs == 0 {
			aborted.Txns = append(aborted.Txns, txn)
		}
	}
	if len(aborted.Txns) == 0 {
		glog.Infoln("TryAbort: No aborts found. Quitting.")
		return nil
	}

	// We choose not to store the MaxAssigned, because it would cause our Oracle to move ahead
	// artificially. The Oracle delta stream moves that ahead in the right order, and we shouldn't
	// muck with that order here.
	glog.Infof("TryAbort selectively proposing only aborted txns: %+v\n", aborted)
	proposal := &pb.Proposal{Delta: aborted}
	return n.proposeAndWait(n.ctx, proposal)
}

// abortOldTransactions would find txns which have done pre-writes, but have been pending for a
// while. The time that is used is based on the last pre-write seen, so if a txn is doing a
// pre-write multiple times, we'll pick the timestamp of the last pre-write. Thus, this function
// would only act on the txns which have not been active in the last N minutes, and send them for
// abort. Note that only the leader runs this function.
func (n *node) abortOldTransactions() {
	// Aborts if not already committed.
	starts := posting.Oracle().TxnOlderThan(x.WorkerConfig.AbortOlderThan)
	if len(starts) == 0 {
		return
	}
	glog.Infof("Found %d old transactions. Acting to abort them.\n", len(starts))
	req := &pb.TxnTimestamps{Ts: starts}
	err := n.blockingAbort(req)
	glog.Infof("Done abortOldTransactions for %d txns. Error: %v\n", len(req.Ts), err)
}

// calculateSnapshot would calculate a snapshot index, considering these factors:
// - We only start discarding once we have at least discardN entries.
// - We are not overshooting the max applied entry. That is, we're not removing
// Raft entries before they get applied.
// - We are considering the minimum start ts that has yet to be committed or
// aborted. This way, we still keep all the mutations corresponding to this
// start ts in the Raft logs. This is important, because we don't persist
// pre-writes to disk in pstore.
// - In simple terms, this means we MUST keep all pending transactions in the Raft logs.
// - Find the maximum commit timestamp that we have seen.
// That would tell us about the maximum timestamp used to do any commits. This
// ts is what we can use for future reads of this snapshot.
// - Finally, this function would propose this snapshot index, so the entire
// group can apply it to their Raft stores.
//
// Txn0  | S0 |    |    | C0 |    |    |
// Txn1  |    | S1 |    |    |    | C1 |
// Txn2  |    |    | S2 | C2 |    |    |
// Txn3  |    |    |    |    | S3 |    |
// Txn4  |    |    |    |    |    |    | S4
// Index | i1 | i2 | i3 | i4 | i5 | i6 | i7
//
// At i7, min pending start ts = S3, therefore snapshotIdx = i5 - 1 = i4.
// At i7, max commit ts = C1, therefore readTs = C1.
//
// This function also takes a startIdx, which can be used an optimization to skip over Raft entries.
// This is useful when we already have a previous snapshot checkpoint (all txns have concluded up
// until that last checkpoint) that we can use as a new start point for the snapshot calculation.
func (n *node) calculateSnapshot(startIdx, lastIdx, minPendingStart uint64) (*pb.Snapshot, error) {
	_, span := otrace.StartSpan(n.ctx, "Calculate.Snapshot",
		otrace.WithSampler(otrace.AlwaysSample()))
	defer span.End()
	discardN := 1

	// We do not need to block snapshot calculation because of a pending stream. Badger would have
	// pending iterators which would ensure that the data above their read ts would not be
	// discarded. Secondly, if a new snapshot does get calculated and applied, the follower can just
	// ask for the new snapshot. Blocking snapshot calculation has caused us issues when a follower
	// somehow kept streaming forever. Then, the leader didn't calculate snapshot, instead it
	// kept appending to Raft logs forever causing group wide issues.

	first, err := n.Store.FirstIndex()
	if err != nil {
		span.Annotatef(nil, "Error: %v", err)
		return nil, err
	}
	span.Annotatef(nil, "First index: %d", first)
	if startIdx > first {
		// If we're starting from a higher index, set first to that.
		first = startIdx
		span.Annotatef(nil, "Setting first to: %d", startIdx)
	}

	rsnap, err := n.Store.Snapshot()
	if err != nil {
		return nil, err
	}
	var snap pb.Snapshot
	if len(rsnap.Data) > 0 {
		if err := snap.Unmarshal(rsnap.Data); err != nil {
			return nil, err
		}
	}
	span.Annotatef(nil, "Last snapshot: %+v", snap)

	if int(lastIdx-first) < discardN {
		span.Annotate(nil, "Skipping due to insufficient entries")
		return nil, nil
	}
	span.Annotatef(nil, "Found Raft entries: %d", lastIdx-first)

	if num := posting.Oracle().NumPendingTxns(); num > 0 {
		glog.V(2).Infof("Num pending txns: %d", num)
	}

	maxCommitTs := snap.ReadTs
	var snapshotIdx uint64
	var maxAssigned uint64

	// Trying to retrieve all entries at once might cause out-of-memory issues in
	// cases where the raft log is too big to fit into memory. Instead of retrieving
	// all entries at once, retrieve it in batches of 64MB.
	var lastEntry raftpb.Entry
	for batchFirst := first; batchFirst <= lastIdx; {
		entries, err := n.Store.Entries(batchFirst, lastIdx+1, 256<<20)
		if err != nil {
			span.Annotatef(nil, "Error: %v", err)
			return nil, err
		}
		// Exit early from the loop if no entries were found.
		if len(entries) == 0 {
			break
		}

		// Store the last entry (as it might be needed outside the loop) and set the
		// start of the new batch at the entry following it. Also set foundEntries to
		// true to indicate to the code outside the loop that entries were retrieved.
		lastEntry = entries[len(entries)-1]
		batchFirst = lastEntry.Index + 1

		for _, entry := range entries {
			if entry.Type != raftpb.EntryNormal || len(entry.Data) == 0 {
				continue
			}
			proposal := getProposal(entry)

			// The way this works is, we figured out the Raft's lastIdx and minPendingStart before
			// calling this function. minPendingStart is calculated by choosing minimum start
			// timestamp of all the pending transactions. We need to ensure that we leave the
			// mutations corresponding to this start ts in the Raft log, and not truncate them.
			// We should however choose all the deltas, even if they occur later in the log, because
			// they track all the commits we have done.
			var start uint64
			if proposal.Mutations != nil {
				start = proposal.Mutations.StartTs
				if start >= minPendingStart && snapshotIdx == 0 {
					// This would only be set once. Note the snapshotIdx == 0 condition.
					snapshotIdx = entry.Index - 1
				}
			}
			if proposal.Delta != nil {
				maxAssigned = x.Max(maxAssigned, proposal.Delta.MaxAssigned)
				for _, txn := range proposal.Delta.GetTxns() {
					maxCommitTs = x.Max(maxCommitTs, txn.CommitTs)
				}
			}
		}
	}

	if maxCommitTs == 0 {
		span.Annotate(nil, "maxCommitTs is zero")
		return nil, nil
	}
	if snapshotIdx == 0 {
		// It is possible that there are no pending transactions. In that case,
		// snapshotIdx would be zero. Instead, set it to last entry's index.
		snapshotIdx = lastEntry.Index
		span.Annotatef(nil, "snapshotIdx is zero. Using last entry's index: %d", snapshotIdx)
	}

	numDiscarding := snapshotIdx - first + 1
	span.Annotatef(nil,
		"Got snapshotIdx: %d. MaxCommitTs: %d. Discarding: %d. MinPendingStartTs: %d",
		snapshotIdx, maxCommitTs, numDiscarding, minPendingStart)

	if int(numDiscarding) < discardN {
		span.Annotate(nil, "Skipping snapshot because insufficient discard entries")
		glog.Infof("Skipping snapshot at index: %d. Insufficient discard entries: %d."+
			" MinPendingStartTs: %d\n", snapshotIdx, numDiscarding, minPendingStart)
		return nil, nil
	}

	result := &pb.Snapshot{
		Context:     n.RaftContext,
		Index:       snapshotIdx,
		ReadTs:      maxCommitTs,
		MaxAssigned: maxAssigned,
	}
	span.Annotatef(nil, "Got snapshot: %+v", result)
	return result, nil
}

func (n *node) joinPeers() error {
	pl, err := n.leaderBlocking()
	if err != nil {
		return err
	}

	gconn := pl.Get()
	c := pb.NewRaftClient(gconn)
	glog.Infof("Calling JoinCluster via leader: %s", pl.Addr)
	if _, err := c.JoinCluster(n.ctx, n.RaftContext); err != nil {
		return errors.Wrapf(err, "error while joining cluster")
	}
	glog.Infof("Done with JoinCluster call\n")
	return nil
}

// Checks if its a peer from the leader of the group.
func (n *node) isMember() (bool, error) {
	pl, err := n.leaderBlocking()
	if err != nil {
		return false, err
	}

	gconn := pl.Get()
	c := pb.NewRaftClient(gconn)
	glog.Infof("Calling IsPeer")
	pr, err := c.IsPeer(n.ctx, n.RaftContext)
	if err != nil {
		return false, errors.Wrapf(err, "error while joining cluster")
	}
	glog.Infof("Done with IsPeer call\n")
	return pr.Status, nil
}

func (n *node) retryUntilSuccess(fn func() error, pause time.Duration) {
	var err error
	for {
		if err = fn(); err == nil {
			break
		}
		glog.Errorf("Error while calling fn: %v. Retrying...\n", err)
		time.Sleep(pause)
	}
}

// InitAndStartNode gets called after having at least one membership sync with the cluster.
func (n *node) InitAndStartNode() {
	initProposalKey(n.Id)
	_, restart, err := n.PastLife()
	x.Check(err)

	_, hasPeer := groups().MyPeer()
	if !restart && hasPeer {
		// The node has other peers, it might have crashed after joining the cluster and before
		// writing a snapshot. Check from leader, if it is part of the cluster. Consider this a
		// restart if it is part of the cluster, else start a new node.
		for {
			if restart, err = n.isMember(); err == nil {
				break
			}
			glog.Errorf("Error while calling hasPeer: %v. Retrying...\n", err)
			time.Sleep(time.Second)
		}
	}

	if n.RaftContext.IsLearner && !hasPeer {
		glog.Fatal("Cannot start a learner node without peer alpha nodes")
	}

	if restart {
		glog.Infof("Restarting node for group: %d\n", n.gid)
		sp, err := n.Store.Snapshot()
		x.Checkf(err, "Unable to get existing snapshot")
		if !raft.IsEmptySnap(sp) {
			// It is important that we pick up the conf state here.
			// Otherwise, we'll lose the store conf state, and it would get
			// overwritten with an empty state when a new snapshot is taken.
			// This causes a node to just hang on restart, because it finds a
			// zero-member Raft group.
			n.SetConfState(&sp.Metadata.ConfState)

			// TODO: Making connections here seems unnecessary, evaluate.
			members := groups().members(n.gid)
			for _, id := range sp.Metadata.ConfState.Nodes {
				m, ok := members[id]
				if ok {
					n.Connect(id, m.Addr)
				}
			}
			for _, id := range sp.Metadata.ConfState.Learners {
				m, ok := members[id]
				if ok {
					n.Connect(id, m.Addr)
				}
			}
		}
		n.SetRaft(raft.RestartNode(n.Cfg))
		glog.V(2).Infoln("Restart node complete")

	} else {
		glog.Infof("New Node for group: %d\n", n.gid)
		if _, hasPeer := groups().MyPeer(); hasPeer {
			// Get snapshot before joining peers as it can take time to retrieve it and we dont
			// want the quorum to be inactive when it happens.
			// Update: This is an optimization, which adds complexity because it requires us to
			// understand the Raft state of the node. Let's instead have the node retrieve the
			// snapshot as needed after joining the group, instead of us forcing one upfront.
			glog.Infoln("Trying to join peers.")
			n.retryUntilSuccess(n.joinPeers, time.Second)
			n.SetRaft(raft.StartNode(n.Cfg, nil))
		} else {
			peers := []raft.Peer{{ID: n.Id}}
			n.SetRaft(raft.StartNode(n.Cfg, peers))
			// Trigger election, so this node can become the leader of this single-node cluster.
			n.canCampaign = true
		}
	}
	go n.processTabletSizes()
	go n.processApplyCh()
	go n.BatchAndSendMessages()
	go n.monitorRaftMetrics()
	go n.cdcTracker.processCDCEvents()
	// Ignoring the error since InitAndStartNode does not return an error and using x.Check would
	// not be the right thing to do.
	_, _ = n.startTask(opRollup)
	go n.stopAllTasks()
	for i := 0; i < 8; i++ {
		go n.mutationWorker(i)
	}
	go n.Run()
}

func (n *node) AmLeader() bool {
	if n.Raft() == nil {
		return false
	}
	r := n.Raft()
	return r.Status().Lead == r.Status().ID
}

func (n *node) monitorRaftMetrics() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		curPendingSize := atomic.LoadInt64(&n.pendingSize)
		ostats.Record(n.ctx, x.RaftPendingSize.M(curPendingSize))
		ostats.Record(n.ctx, x.RaftApplyCh.M(int64(len(n.applyCh))))
	}
}

func getSanitizedString(proposal *pb.Proposal) string {
	ps := proposal.String()
	if proposal.GetRestore() != nil {
		if len(proposal.GetRestore().GetAccessKey()) != 0 {
			ps = strings.Replace(ps, proposal.GetRestore().GetAccessKey(),
				sensitiveString, 1)
		}
		if len(proposal.GetRestore().GetSecretKey()) != 0 {
			ps = strings.Replace(ps, proposal.GetRestore().GetSecretKey(),
				sensitiveString, 1)
		}
	}
	return ps
}
