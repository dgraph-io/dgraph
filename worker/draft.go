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
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"math"
	"sync/atomic"
	"time"

	"github.com/coreos/etcd/raft"
	"github.com/coreos/etcd/raft/raftpb"
	"github.com/golang/glog"
	"golang.org/x/net/context"
	"golang.org/x/net/trace"

	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/badger/y"
	dy "github.com/dgraph-io/dgo/y"
	"github.com/dgraph-io/dgraph/conn"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/raftwal"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
)

// uniqueKey is meant to be unique across all the replicas.
func uniqueKey() string {
	return fmt.Sprintf("%02d-%d", groups().Node.Id, groups().Node.Rand.Uint64())
}

type node struct {
	*conn.Node

	// Fields which are never changed after init.
	applyCh  chan []raftpb.Entry
	rollupCh chan uint64 // Channel to run posting list rollups.
	ctx      context.Context
	gid      uint32
	closer   *y.Closer

	streaming int32 // Used to avoid calculating snapshot

	canCampaign bool
	elog        trace.EventLog
}

// Now that we apply txn updates via Raft, waiting based on Txn timestamps is
// sufficient. We don't need to wait for proposals to be applied.

func newNode(store *raftwal.DiskStorage, gid uint32, id uint64, myAddr string) *node {
	x.Printf("Node ID: %v with GroupID: %v\n", id, gid)

	rc := &pb.RaftContext{
		Addr:  myAddr,
		Group: gid,
		Id:    id,
	}
	m := conn.NewNode(rc, store)

	n := &node{
		Node: m,
		ctx:  context.Background(),
		gid:  gid,
		// We need a generous size for applyCh, because raft.Tick happens every
		// 10ms. If we restrict the size here, then Raft goes into a loop trying
		// to maintain quorum health.
		applyCh:  make(chan []raftpb.Entry, 1000),
		rollupCh: make(chan uint64, 3),
		elog:     trace.NewEventLog("Dgraph", "ApplyCh"),
		closer:   y.NewCloser(3), // Matches CLOSER:1
	}
	return n
}

type header struct {
	proposalId uint32
	msgId      uint16
}

func (h *header) Length() int {
	return 6 // 4 bytes for proposalId, 2 bytes for msgId.
}

func (h *header) Encode() []byte {
	result := make([]byte, h.Length())
	binary.LittleEndian.PutUint32(result[0:4], h.proposalId)
	binary.LittleEndian.PutUint16(result[4:6], h.msgId)
	return result
}

func (h *header) Decode(in []byte) {
	h.proposalId = binary.LittleEndian.Uint32(in[0:4])
	h.msgId = binary.LittleEndian.Uint16(in[4:6])
}

var errInternalRetry = errors.New("Retry Raft proposal internally")

// proposeAndWait sends a proposal through RAFT. It waits on a channel for the proposal
// to be applied(written to WAL) to all the nodes in the group.
func (n *node) proposeAndWait(ctx context.Context, proposal *pb.Proposal) error {
	if n.Raft() == nil {
		return x.Errorf("Raft isn't initialized yet")
	}

	// TODO: Should be based on number of edges (amount of work)
	pendingProposals <- struct{}{}
	x.PendingProposals.Add(1)
	defer func() { <-pendingProposals; x.PendingProposals.Add(-1) }()
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
	timeout := 4 * time.Second
	for {
		// The below algorithm would run proposal with a calculated timeout. If
		// it doesn't succeed or fail, it would block for the timeout duration.
		// Then it would double the timeout.
		err := propose(timeout)
		if err != errInternalRetry {
			return err
		}
		t := time.NewTimer(timeout)
		select {
		case <-t.C:
		case <-ctx.Done():
			t.Stop()
			return ctx.Err()
		}
		timeout *= 2 // Exponential backoff
	}
	return nil
}

func (n *node) Ctx(key string) context.Context {
	ctx := context.Background()
	if pctx := n.Proposals.Get(key); pctx != nil {
		ctx = pctx.Ctx
	}
	return ctx
}

func (n *node) applyConfChange(e raftpb.Entry) {
	var cc raftpb.ConfChange
	cc.Unmarshal(e.Data)

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

var errHasPendingTxns = errors.New("Pending transactions found. Please retry operation.")

// We must not wait here. Previously, we used to block until we have aborted the
// transactions. We're now applying all updates serially, so blocking for one
// operation is not an option.
func detectPendingTxns(attr string) error {
	tctxs := posting.Oracle().IterateTxns(func(key []byte) bool {
		pk := x.Parse(key)
		return pk.Attr == attr
	})
	if len(tctxs) == 0 {
		return nil
	}
	go tryAbortTransactions(tctxs)
	return errHasPendingTxns
}

// We don't support schema mutations across nodes in a transaction.
// Wait for all transactions to either abort or complete and all write transactions
// involving the predicate are aborted until schema mutations are done.
func (n *node) applyMutations(proposal *pb.Proposal, index uint64) error {
	tr := trace.New("Dgraph.Node", "ApplyMutations")
	defer tr.Finish()

	if proposal.Mutations.DropAll {
		// Ensures nothing get written to disk due to commit proposals.
		posting.Oracle().ResetTxns()
		schema.State().DeleteAll()
		return posting.DeleteAll()
	}

	if proposal.Mutations.StartTs == 0 {
		return errors.New("StartTs must be provided.")
	}

	startTs := proposal.Mutations.StartTs
	ctx := n.Ctx(proposal.Key)

	if len(proposal.Mutations.Schema) > 0 {
		tr.LazyPrintf("Applying Schema")
		for _, supdate := range proposal.Mutations.Schema {
			// This is neceassry to ensure that there is no race between when we start reading
			// from badger and new mutation getting commited via raft and getting applied.
			// Before Moving the predicate we would flush all and wait for watermark to catch up
			// but there might be some proposals which got proposed but not comitted yet.
			// It's ok to reject the proposal here and same would happen on all nodes because we
			// would have proposed membershipstate, and all nodes would have the proposed state
			// or some state after that before reaching here.
			if tablet := groups().Tablet(supdate.Predicate); tablet != nil && tablet.ReadOnly {
				return errPredicateMoving
			}
			if err := detectPendingTxns(supdate.Predicate); err != nil {
				return err
			}
			if err := runSchemaMutation(ctx, supdate, startTs); err != nil {
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

	// stores a map of predicate and type of first mutation for each predicate
	schemaMap := make(map[string]types.TypeID)
	for _, edge := range proposal.Mutations.Edges {
		if tablet := groups().Tablet(edge.Attr); tablet != nil && tablet.ReadOnly {
			tr.LazyPrintf("Predicate Moving")
			tr.SetError()
			return errPredicateMoving
		}
		if edge.Entity == 0 && bytes.Equal(edge.Value, []byte(x.Star)) {
			// We should only drop the predicate if there is no pending
			// transaction.
			if err := detectPendingTxns(edge.Attr); err != nil {
				tr.LazyPrintf("Found pending transactions which obstruct operation.")
				tr.SetError()
				return err
			}
			tr.LazyPrintf("Deleting predicate")
			return posting.DeletePredicate(ctx, edge.Attr)
		}
		// Dont derive schema when doing deletion.
		if edge.Op == pb.DirectedEdge_DEL {
			continue
		}
		if _, ok := schemaMap[edge.Attr]; !ok {
			schemaMap[edge.Attr] = posting.TypeID(edge)
		}
	}

	total := len(proposal.Mutations.Edges)
	x.ActiveMutations.Add(int64(total))
	defer x.ActiveMutations.Add(-int64(total))

	for attr, storageType := range schemaMap {
		if _, err := schema.State().TypeOf(attr); err != nil {
			// Schema doesn't exist
			// Since committed entries are serialized, updateSchemaIfMissing is not
			// needed, In future if schema needs to be changed, it would flow through
			// raft so there won't be race conditions between read and update schema
			updateSchemaType(attr, storageType, index)
		}
	}

	m := proposal.Mutations
	txn := posting.Oracle().RegisterStartTs(m.StartTs)
	if txn.ShouldAbort() {
		tr.LazyPrintf("Should Abort")
		tr.SetError()
		return dy.ErrConflict
	}
	tr.LazyPrintf("Applying %d edges", len(m.Edges))
	for _, edge := range m.Edges {
		err := posting.ErrRetry
		for err == posting.ErrRetry {
			err = runMutation(ctx, edge, txn)
		}
		if err != nil {
			tr.SetError()
			return err
		}
	}
	tr.LazyPrintf("Done applying %d edges", len(m.Edges))
	return nil
}

func (n *node) applyCommitted(proposal *pb.Proposal, index uint64) error {
	if proposal.Mutations != nil {
		// syncmarks for this shouldn't be marked done until it's comitted.
		n.elog.Printf("Applying mutations for key: %s", proposal.Key)
		return n.applyMutations(proposal, index)
	}

	ctx := n.Ctx(proposal.Key)
	switch {
	case len(proposal.Kv) > 0:
		return populateKeyValues(ctx, proposal.Kv)

	case proposal.State != nil:
		n.elog.Printf("Applying state for key: %s", proposal.Key)
		// This state needn't be snapshotted in this group, on restart we would fetch
		// a state which is latest or equal to this.
		groups().applyState(proposal.State)
		return nil

	case len(proposal.CleanPredicate) > 0:
		n.elog.Printf("Cleaning predicate: %s", proposal.CleanPredicate)
		return posting.DeletePredicate(ctx, proposal.CleanPredicate)

	case proposal.Delta != nil:
		n.elog.Printf("Applying Oracle Delta for key: %s", proposal.Key)
		return n.commitOrAbort(proposal.Key, proposal.Delta)

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
		glog.Infof("Creating snapshot at index: %d. ReadTs: %d.\n", snap.Index, snap.ReadTs)

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
		// Roll up all posting lists as a best-effort operation.
		n.rollupCh <- snap.ReadTs
		return nil
	}
	x.Fatalf("Unknown proposal: %+v", proposal)
	return nil
}

func (n *node) processRollups() {
	defer n.closer.Done()                    // CLOSER:1
	tick := time.NewTicker(10 * time.Minute) // Rolling up once every 10 minutes seems alright.
	defer tick.Stop()

	var readTs, last uint64
	for {
		select {
		case <-n.closer.HasBeenClosed():
			return
		case readTs = <-n.rollupCh:
		case <-tick.C:
			if readTs <= last {
				break // Break out of the select case.
			}
			if err := n.rollupLists(readTs); err != nil {
				// If we encounter error here, we don't need to do anything about
				// it. Just let the user know.
				glog.Errorf("Error while rolling up lists at %d: %v\n", readTs, err)
			} else {
				last = readTs // Update last only if we succeeded.
				glog.Infof("List rollup at Ts %d: OK.\n", readTs)
			}
		}
	}
}

func (n *node) processApplyCh() {
	defer n.closer.Done() // CLOSER:1

	// Add logic here to delete everything older than 10 mins.
	// Add logic here to calculate the size of proposal, as an extra step to
	// ensure that we're dealing with the same proposal as before.
	previous := make(map[string]error)
	for entries := range n.applyCh {
		for _, e := range entries {
			switch {
			case e.Type == raftpb.EntryConfChange:
				// Already handled in the main Run loop.
			case len(e.Data) == 0:
				n.elog.Printf("Found empty data at index: %d", e.Index)
				n.Applied.Done(e.Index)
			default:
				proposal := &pb.Proposal{}
				if err := proposal.Unmarshal(e.Data); err != nil {
					x.Fatalf("Unable to unmarshal proposal: %v %q\n", err, e.Data)
				}
				var perr error
				if err, ok := previous[proposal.Key]; ok && err == nil {
					n.elog.Printf("Proposal with key: %s already applied. Skipping index: %d.\n",
						proposal.Key, e.Index)

				} else {
					perr = n.applyCommitted(proposal, e.Index)
					if len(proposal.Key) > 0 {
						previous[proposal.Key] = perr
					}
					n.elog.Printf("Applied proposal with key: %s, index: %d. Err: %v",
						proposal.Key, e.Index, perr)
				}

				n.Proposals.Done(proposal.Key, perr)
				n.Applied.Done(e.Index)
			}
		}
	}
}

func (n *node) commitOrAbort(pkey string, delta *pb.OracleDelta) error {
	// First let's commit all mutations to disk.
	writer := x.TxnWriter{DB: pstore}
	toDisk := func(start, commit uint64) {
		txn := posting.Oracle().GetTxn(start)
		if txn == nil {
			return
		}
		for err := txn.CommitToDisk(&writer, commit); err != nil; {
			glog.Warningf("Error while applying txn status to disk (%d -> %d): %v",
				start, commit, err)
			time.Sleep(10 * time.Millisecond)
		}
	}
	for _, status := range delta.Txns {
		toDisk(status.StartTs, status.CommitTs)
	}
	if err := writer.Flush(); err != nil {
		x.Errorf("Error while flushing to disk: %v", err)
		return err
	}

	// Now let's commit all mutations to memory.
	toMemory := func(start, commit uint64) {
		txn := posting.Oracle().GetTxn(start)
		if txn == nil {
			return
		}
		for err := txn.CommitToMemory(commit); err != nil; {
			x.Printf("Error while applying txn status to memory (%d -> %d): %v",
				start, commit, err)
			time.Sleep(10 * time.Millisecond)
		}
	}

	for _, txn := range delta.Txns {
		toMemory(txn.StartTs, txn.CommitTs)
	}
	// Now advance Oracle(), so we can service waiting reads.
	posting.Oracle().ProcessDelta(delta)
	return nil
}

func (n *node) applyAllMarks(ctx context.Context) {
	// Get index of last committed.
	lastIndex := n.Applied.LastIndex()
	n.Applied.WaitForMark(ctx, lastIndex)
}

func (n *node) leaderBlocking() (*conn.Pool, error) {
	pool := groups().Leader(groups().groupId())
	if pool == nil {
		// Functions like retrieveSnapshot and joinPeers are blocking at initial start and
		// leader election for a group might not have happened when it is called. If we can't
		// find a leader, get latest state from
		// Zero.
		if err := UpdateMembershipState(context.Background()); err != nil {
			return nil, fmt.Errorf("Error while trying to update membership state: %+v", err)
		}
		return nil, fmt.Errorf("Unable to reach leader in group %d", n.gid)
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

func (n *node) retrieveSnapshot() error {
	pool, err := n.leaderBlocking()
	if err != nil {
		return err
	}

	// Need to clear pl's stored in memory for the case when retrieving snapshot with
	// index greater than this node's last index
	// Should invalidate/remove pl's to this group only ideally
	//
	// We can safely evict posting lists from memory. Because, all the updates corresponding to txn
	// commits up until then have already been written to pstore. And the way we take snapshots, we
	// keep all the pre-writes for a pending transaction, so they will come back to memory, as Raft
	// logs are replayed.
	posting.EvictLRU()
	if _, err := n.populateSnapshot(pstore, pool); err != nil {
		return fmt.Errorf("Cannot retrieve snapshot from peer, error: %v\n", err)
	}
	// Populate shard stores the streamed data directly into db, so we need to refresh
	// schema for current group id
	if err := schema.LoadFromDb(); err != nil {
		return fmt.Errorf("Error while initilizating schema: %+v\n", err)
	}
	groups().triggerMembershipSync()
	return nil
}

func (n *node) proposeSnapshot(discardN int) error {
	snap, err := n.calculateSnapshot(discardN)
	if err != nil || snap == nil {
		return err
	}
	proposal := &pb.Proposal{
		Snapshot: snap,
	}
	n.elog.Printf("Proposing snapshot: %+v\n", snap)
	data, err := proposal.Marshal()
	x.Check(err)
	return n.Raft().Propose(n.ctx, data)
}

func (n *node) Run() {
	defer n.closer.Done() // CLOSER:1

	firstRun := true
	var leader bool
	// See also our configuration of HeartbeatTick and ElectionTick.
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()

	slowTicker := time.NewTicker(30 * time.Second)
	defer slowTicker.Stop()

	done := make(chan struct{})
	go func() {
		<-n.closer.HasBeenClosed()
		glog.Infof("Stopping node.Run")
		if peerId, has := groups().MyPeer(); has && n.AmLeader() {
			n.Raft().TransferLeadership(n.ctx, Config.RaftId, peerId)
			time.Sleep(time.Second) // Let transfer happen.
		}
		n.Raft().Stop()
		close(n.applyCh)
		close(done)
	}()

	var snapshotLoops uint64
	for {
		select {
		case <-done:
			log.Println("Raft node done.")
			return

		case <-slowTicker.C:
			n.elog.Printf("Size of applyCh: %d", len(n.applyCh))
			if leader {
				// We try to take a snapshot every slow tick duration, with a 1000 discard entries.
				// But, once a while, we take a snapshot with 10 discard entries. This avoids the
				// scenario where after bringing up an Alpha, and doing a hundred schema updates, we
				// don't take any snapshots because there are not enough updates (discardN=10),
				// which then really slows down restarts. At the same time, by checking more
				// frequently, we can quickly take a snapshot if a lot of mutations are coming in
				// fast (discardN=1000).
				discardN := 1000
				if snapshotLoops%5 == 0 {
					discardN = 10
				}
				snapshotLoops++
				// We use disk based storage for Raft. So, we're not too concerned about
				// snapshotting.  We just need to do enough, so that we don't have a huge backlog of
				// entries to process on a restart.
				if err := n.proposeSnapshot(discardN); err != nil {
					x.Errorf("While calculating and proposing snapshot: %v", err)
				}
				go n.abortOldTransactions()
			}

		case <-ticker.C:
			n.Raft().Tick()

		case rd := <-n.Raft().Ready():
			var tr trace.Trace
			if len(rd.Entries) > 0 || !raft.IsEmptySnap(rd.Snapshot) || !raft.IsEmptyHardState(rd.HardState) {
				// Optionally, trace this run.
				tr = trace.New("Dgraph.Raft", "RunLoop")
			}

			if rd.SoftState != nil {
				groups().triggerMembershipSync()
				leader = rd.RaftState == raft.StateLeader
			}
			if leader {
				// Leader can send messages in parallel with writing to disk.
				for _, msg := range rd.Messages {
					// NOTE: We can do some optimizations here to drop messages.
					n.Send(msg)
				}
			}
			if tr != nil {
				tr.LazyPrintf("Handled ReadStates and SoftState.")
			}

			// Store the hardstate and entries. Note that these are not CommittedEntries.
			n.SaveToStorage(rd.HardState, rd.Entries, rd.Snapshot)
			if tr != nil {
				tr.LazyPrintf("Saved %d entries. Snapshot, HardState empty? (%v, %v)",
					len(rd.Entries),
					raft.IsEmptySnap(rd.Snapshot),
					raft.IsEmptyHardState(rd.HardState))
			}

			if !raft.IsEmptySnap(rd.Snapshot) {
				// We don't send snapshots to other nodes. But, if we get one, that means
				// either the leader is trying to bring us up to state; or this is the
				// snapshot that I created. Only the former case should be handled.
				var snap pb.Snapshot
				x.Check(snap.Unmarshal(rd.Snapshot.Data))
				rc := snap.GetContext()
				x.AssertTrue(rc.GetGroup() == n.gid)
				if rc.Id != n.Id {
					// We are getting a new snapshot from leader. We need to wait for the applyCh to
					// finish applying the updates, otherwise, we'll end up overwriting the data
					// from the new snapshot that we retrieved.
					maxIndex := n.Applied.LastIndex()
					glog.Infof("Waiting for applyCh to reach %d before taking snapshot\n", maxIndex)
					n.Applied.WaitForMark(context.Background(), maxIndex)

					// It's ok to block ticks while retrieving snapshot, since it's a follower.
					glog.Infof("-------> SNAPSHOT [%d] from %d\n", n.gid, rc.Id)
					n.retryUntilSuccess(n.retrieveSnapshot, 100*time.Millisecond)
					glog.Infof("-------> SNAPSHOT [%d]. DONE.\n", n.gid)
				} else {
					glog.Infof("-------> SNAPSHOT [%d] from %d [SELF]. Ignoring.\n", n.gid, rc.Id)
				}
			}
			if tr != nil {
				tr.LazyPrintf("Applied or retrieved snapshot.")
			}

			// Now schedule or apply committed entries.
			for _, entry := range rd.CommittedEntries {
				// Need applied watermarks for schema mutation also for read linearazibility
				// Applied watermarks needs to be emitted as soon as possible sequentially.
				// If we emit Mark{4, false} and Mark{4, true} before emitting Mark{3, false}
				// then doneUntil would be set as 4 as soon as Mark{4,true} is done and before
				// Mark{3, false} is emitted. So it's safer to emit watermarks as soon as
				// possible sequentially
				n.Applied.Begin(entry.Index)

				if entry.Type == raftpb.EntryConfChange {
					n.applyConfChange(entry)
					// Not present in proposal map.
					n.Applied.Done(entry.Index)
					groups().triggerMembershipSync()
				}
			}
			// Send the whole lot to applyCh in one go, instead of sending entries one by one.
			if len(rd.CommittedEntries) > 0 {
				n.applyCh <- rd.CommittedEntries
			}

			if tr != nil {
				tr.LazyPrintf("Handled %d committed entries.", len(rd.CommittedEntries))
			}

			if !leader {
				// Followers should send messages later.
				for _, msg := range rd.Messages {
					// NOTE: We can do some optimizations here to drop messages.
					n.Send(msg)
				}
			}
			if tr != nil {
				tr.LazyPrintf("Follower queued messages.")
			}

			n.Raft().Advance()
			if firstRun && n.canCampaign {
				go n.Raft().Campaign(n.ctx)
				firstRun = false
			}
			if tr != nil {
				tr.LazyPrintf("Advanced Raft. Done.")
				tr.Finish()
			}
		}
	}
}

// rollupLists would consolidate all the deltas that constitute one posting
// list, and write back a complete posting list.
func (n *node) rollupLists(readTs uint64) error {
	writer := &x.TxnWriter{DB: pstore, BlindWrite: true}
	sl := streamLists{stream: writer, db: pstore}
	sl.chooseKey = func(item *badger.Item) bool {
		pk := x.Parse(item.Key())
		if pk.IsSchema() {
			// Skip if schema.
			return false
		}
		// Return true if we don't find the BitCompletePosting bit.
		return item.UserMeta()&posting.BitCompletePosting == 0
	}
	sl.itemToKv = func(key []byte, itr *badger.Iterator) (*pb.KV, error) {
		l, err := posting.ReadPostingList(key, itr)
		if err != nil {
			return nil, err
		}
		return l.MarshalToKv()
	}
	if err := sl.orchestrate(context.Background(), "Rolling up", readTs); err != nil {
		return err
	}
	if err := writer.Flush(); err != nil {
		return err
	}
	// We can now discard all invalid versions of keys below this ts.
	pstore.SetDiscardTs(readTs)
	return nil
}

var errConnection = errors.New("No connection exists")

func (n *node) blockingAbort(req *pb.TxnTimestamps) error {
	pl := groups().Leader(0)
	if pl == nil {
		return errConnection
	}
	zc := pb.NewZeroClient(pl.Get())
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	delta, err := zc.TryAbort(ctx, req)
	x.Printf("TryAbort %d txns with start ts. Error: %v\n", len(req.Ts), err)
	if err != nil || len(delta.Txns) == 0 {
		return err
	}

	// Let's propose the txn updates received from Zero. This is important because there are edge
	// cases where a txn status might have been missed by the group.
	n.elog.Printf("Proposing abort txn delta: %+v\n", delta)
	proposal := &pb.Proposal{Delta: delta}
	return n.proposeAndWait(n.ctx, proposal)
}

// abortOldTransactions would find txns which have done pre-writes, but have been pending for a
// while. The time that is used is based on the last pre-write seen, so if a txn is doing a
// pre-write multiple times, we'll pick the timestamp of the last pre-write. Thus, this function
// would only act on the txns which have not been active in the last N minutes, and send them for
// abort. Note that only the leader runs this function.
func (n *node) abortOldTransactions() {
	// Aborts if not already committed.
	starts := posting.Oracle().TxnOlderThan(5 * time.Minute)
	if len(starts) == 0 {
		return
	}
	x.Printf("Found %d old transactions. Acting to abort them.\n", len(starts))
	req := &pb.TxnTimestamps{Ts: starts}
	err := n.blockingAbort(req)
	x.Printf("abortOldTransactions for %d txns. Error: %+v\n", len(req.Ts), err)
}

// calculateSnapshot would calculate a snapshot index, considering these factors:
// - We only start discarding once we have at least discardN entries.
// - We are not overshooting the max applied entry. That is, we're not removing
// Raft entries before they get applied.
// - We are considering the minimum start ts that has yet to be committed or
// aborted. This way, we still keep all the mutations corresponding to this
// start ts in the Raft logs. This is important, because we don't persist
// pre-writes to disk in pstore.
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
func (n *node) calculateSnapshot(discardN int) (*pb.Snapshot, error) {
	tr := trace.New("Dgraph.Internal", "Propose.Snapshot")
	defer tr.Finish()

	if atomic.LoadInt32(&n.streaming) > 0 {
		tr.LazyPrintf("Skipping calculateSnapshot due to streaming")
		return nil, nil
	}

	first, err := n.Store.FirstIndex()
	if err != nil {
		tr.LazyPrintf("Error: %v", err)
		tr.SetError()
		return nil, err
	}
	tr.LazyPrintf("First index: %d", first)

	last := n.Applied.DoneUntil()
	if int(last-first) < discardN {
		tr.LazyPrintf("Skipping due to insufficient entries")
		return nil, nil
	}
	tr.LazyPrintf("Found Raft entries: %d", last-first)

	entries, err := n.Store.Entries(first, last+1, math.MaxUint64)
	if err != nil {
		tr.LazyPrintf("Error: %v", err)
		tr.SetError()
		return nil, err
	}

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
	minPendingStart := posting.Oracle().MinPendingStartTs()
	var maxCommitTs, snapshotIdx, maxCommitIdx uint64
	for _, entry := range entries {
		if entry.Type != raftpb.EntryNormal {
			continue
		}
		var proposal pb.Proposal
		if err := proposal.Unmarshal(entry.Data); err != nil {
			tr.LazyPrintf("Error: %v", err)
			tr.SetError()
			return nil, err
		}
		if proposal.Mutations != nil {
			start := proposal.Mutations.StartTs
			if start >= minPendingStart && snapshotIdx == 0 {
				snapshotIdx = entry.Index - 1
			}
		}
		if proposal.Delta != nil {
			for _, txn := range proposal.Delta.GetTxns() {
				maxCommitTs = x.Max(maxCommitTs, txn.CommitTs)
			}
			maxCommitIdx = entry.Index
		}
	}
	if maxCommitTs == 0 {
		tr.LazyPrintf("maxCommitTs is zero")
		return nil, nil
	}
	if snapshotIdx <= 0 {
		// It is possible that there are no pending transactions. In that case,
		// snapshotIdx would be zero.
		tr.LazyPrintf("Using maxCommitIdx as snapshotIdx: %d", maxCommitIdx)
		snapshotIdx = maxCommitIdx
	}

	numDiscarding := snapshotIdx - first + 1
	tr.LazyPrintf("Got snapshotIdx: %d. MaxCommitTs: %d. Discarding: %d. MinPendingStartTs: %d",
		snapshotIdx, maxCommitTs, numDiscarding, minPendingStart)
	if int(numDiscarding) < discardN {
		tr.LazyPrintf("Skipping snapshot because insufficient discard entries")
		x.Printf("Skipping snapshot at index: %d. Insufficient discard entries: %d."+
			" MinPendingStartTs: %d\n", snapshotIdx, numDiscarding, minPendingStart)
		return nil, nil
	}

	snap := &pb.Snapshot{
		Context: n.RaftContext,
		Index:   snapshotIdx,
		ReadTs:  maxCommitTs,
	}
	tr.LazyPrintf("Got snapshot: %+v", snap)
	return snap, nil
}

func (n *node) joinPeers() error {
	pl, err := n.leaderBlocking()
	if err != nil {
		return err
	}

	gconn := pl.Get()
	c := pb.NewRaftClient(gconn)
	x.Printf("Calling JoinCluster via leader: %s", pl.Addr)
	if _, err := c.JoinCluster(n.ctx, n.RaftContext); err != nil {
		return x.Errorf("Error while joining cluster: %+v\n", err)
	}
	x.Printf("Done with JoinCluster call\n")
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
	x.Printf("Calling IsPeer")
	pr, err := c.IsPeer(n.ctx, n.RaftContext)
	if err != nil {
		return false, x.Errorf("Error while joining cluster: %+v\n", err)
	}
	x.Printf("Done with IsPeer call\n")
	return pr.Status, nil
}

func (n *node) retryUntilSuccess(fn func() error, pause time.Duration) {
	var err error
	for {
		if err = fn(); err == nil {
			break
		}
		x.Printf("Error while calling fn: %v. Retrying...\n", err)
		time.Sleep(pause)
	}
}

// InitAndStartNode gets called after having at least one membership sync with the cluster.
func (n *node) InitAndStartNode() {
	_, restart, err := n.PastLife()
	x.Check(err)

	if _, hasPeer := groups().MyPeer(); !restart && hasPeer {
		// The node has other peers, it might have crashed after joining the cluster and before
		// writing a snapshot. Check from leader, if it is part of the cluster. Consider this a
		// restart if it is part of the cluster, else start a new node.
		for {
			if restart, err = n.isMember(); err == nil {
				break
			}
			x.Printf("Error while calling hasPeer: %v. Retrying...\n", err)
			time.Sleep(time.Second)
		}
	}

	if restart {
		x.Printf("Restarting node for group: %d\n", n.gid)
		sp, err := n.Store.Snapshot()
		x.Checkf(err, "Unable to get existing snapshot")
		if !raft.IsEmptySnap(sp) {
			// It is important that we pick up the conf state here.
			// Otherwise, we'll lose the store conf state, and it would get
			// overwritten with an empty state when a new snapshot is taken.
			// This causes a node to just hang on restart, because it finds a
			// zero-member Raft group.
			n.SetConfState(&sp.Metadata.ConfState)

			members := groups().members(n.gid)
			for _, id := range sp.Metadata.ConfState.Nodes {
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

			// Note: This is an optimization, which adds complexity because it requires us to
			// understand the Raft state of the node. Let's instead have the node retrieve the
			// snapshot as needed after joining the group, instead of us forcing one upfront.
			//
			// x.Printf("Retrieving snapshot from peer: %d", peerId)
			// n.retryUntilSuccess(n.retrieveSnapshot, time.Second)

			x.Println("Trying to join peers.")
			n.retryUntilSuccess(n.joinPeers, time.Second)
			n.SetRaft(raft.StartNode(n.Cfg, nil))
		} else {
			peers := []raft.Peer{{ID: n.Id}}
			n.SetRaft(raft.StartNode(n.Cfg, peers))
			// Trigger election, so this node can become the leader of this single-node cluster.
			n.canCampaign = true
		}
	}
	go n.processRollups()
	go n.processApplyCh()
	go n.BatchAndSendMessages()
	go n.Run()
}

func (n *node) AmLeader() bool {
	if n.Raft() == nil {
		return false
	}
	r := n.Raft()
	return r.Status().Lead == r.Status().ID
}
