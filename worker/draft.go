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
	"math"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	humanize "github.com/dustin/go-humanize"
	"go.etcd.io/etcd/raft"
	"go.etcd.io/etcd/raft/raftpb"

	ostats "go.opencensus.io/stats"
	"go.opencensus.io/tag"
	otrace "go.opencensus.io/trace"

	"github.com/dgraph-io/badger"
	bpb "github.com/dgraph-io/badger/pb"
	"github.com/dgraph-io/badger/y"
	dy "github.com/dgraph-io/dgo/y"
	"github.com/dgraph-io/dgraph/conn"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/raftwal"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"

	"github.com/golang/glog"
	"golang.org/x/net/context"
	"golang.org/x/net/trace"
)

type node struct {
	*conn.Node

	// Fields which are never changed after init.
	applyCh  chan []*pb.Proposal
	rollupCh chan uint64 // Channel to run posting list rollups.
	ctx      context.Context
	gid      uint32
	closer   *y.Closer

	lastCommitTs uint64 // Only used to ensure that our commit Ts is monotonically increasing.

	streaming int32 // Used to avoid calculating snapshot

	canCampaign bool
	elog        trace.EventLog

	pendingSize int64
}

// Now that we apply txn updates via Raft, waiting based on Txn timestamps is
// sufficient. We don't need to wait for proposals to be applied.

func newNode(store *raftwal.DiskStorage, gid uint32, id uint64, myAddr string) *node {
	glog.Infof("Node ID: %#x with GroupID: %d\n", id, gid)

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
		applyCh:  make(chan []*pb.Proposal, 1000),
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

func (n *node) Ctx(key string) context.Context {
	if pctx := n.Proposals.Get(key); pctx != nil {
		return pctx.Ctx
	}
	return context.Background()
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

var errHasPendingTxns = errors.New("Pending transactions found. Please retry operation")

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
func (n *node) applyMutations(ctx context.Context, proposal *pb.Proposal) (rerr error) {
	span := otrace.FromContext(ctx)

	if proposal.Mutations.DropOp == pb.Mutations_DATA {
		// Ensures nothing get written to disk due to commit proposals.
		posting.Oracle().ResetTxns()
		return posting.DeleteData()
	}

	if proposal.Mutations.DropOp == pb.Mutations_ALL {
		// Ensures nothing get written to disk due to commit proposals.
		posting.Oracle().ResetTxns()
		schema.State().DeleteAll()

		if err := posting.DeleteAll(); err != nil {
			return err
		}

		if groups().groupId() == 1 {
			initialSchema := schema.InitialSchema()
			for _, s := range initialSchema {
				if err := updateSchema(s.Predicate, *s); err != nil {
					return err
				}

				if servesTablet, err := groups().ServesTablet(s.Predicate); err != nil {
					return err
				} else if !servesTablet {
					return fmt.Errorf("group 1 should always serve reserved predicate %s",
						s.Predicate)
				}
			}
		}

		return nil
	}

	if proposal.Mutations.DropOp == pb.Mutations_TYPE {
		return schema.State().DeleteType(proposal.Mutations.DropValue)
	}

	if proposal.Mutations.StartTs == 0 {
		return errors.New("StartTs must be provided")
	}
	startTs := proposal.Mutations.StartTs

	if len(proposal.Mutations.Schema) > 0 || len(proposal.Mutations.Types) > 0 {
		span.Annotatef(nil, "Applying schema and types")
		for _, supdate := range proposal.Mutations.Schema {
			// We should not need to check for predicate move here.
			if err := detectPendingTxns(supdate.Predicate); err != nil {
				return err
			}
			if err := runSchemaMutation(ctx, supdate, startTs); err != nil {
				return err
			}
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

	// stores a map of predicate and type of first mutation for each predicate
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

	// TODO: Active mutations values can go up or down but with
	// OpenCensus stats bucket boundaries start from 0, hence
	// recording negative and positive values skews up values.
	ostats.Record(ctx, x.ActiveMutations.M(int64(total)))
	defer func() {
		ostats.Record(ctx, x.ActiveMutations.M(int64(-total)))
	}()

	for attr, storageType := range schemaMap {
		if _, err := schema.State().TypeOf(attr); err != nil {
			createSchema(attr, storageType)
		}
	}

	m := proposal.Mutations
	txn := posting.Oracle().RegisterStartTs(m.StartTs)
	if txn.ShouldAbort() {
		span.Annotatef(nil, "Txn %d should abort.", m.StartTs)
		return dy.ErrConflict
	}

	// Discard the posting lists from cache to release memory at the end.
	defer txn.Update()

	sort.Slice(m.Edges, func(i, j int) bool {
		ei := m.Edges[i]
		ej := m.Edges[j]
		if ei.GetAttr() != ej.GetAttr() {
			return ei.GetAttr() < ej.GetAttr()
		}
		return ei.GetEntity() < ej.GetEntity()
	})

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
	for i := 0; i < numGo; i++ {
		if err := <-errCh; err != nil {
			return err
		}
	}
	return nil
}

func (n *node) applyCommitted(proposal *pb.Proposal) error {
	ctx := n.Ctx(proposal.Key)
	span := otrace.FromContext(ctx)
	span.Annotatef(nil, "node.applyCommitted Node id: %d. Group id: %d. Got proposal key: %s",
		n.Id, n.gid, proposal.Key)

	if proposal.Mutations != nil {
		// syncmarks for this shouldn't be marked done until it's committed.
		span.Annotate(nil, "Applying mutations")
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
	defer n.closer.Done()                   // CLOSER:1
	tick := time.NewTicker(5 * time.Minute) // Rolling up once every 5 minutes seems alright.
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

	type P struct {
		err  error
		size int
		seen time.Time
	}
	previous := make(map[string]*P)

	// This function must be run serially.
	handle := func(proposals []*pb.Proposal) {
		var totalSize int64
		for _, proposal := range proposals {
			// We use the size as a double check to ensure that we're
			// working with the same proposal as before.
			psz := proposal.Size()
			totalSize += int64(psz)

			var perr error
			p, ok := previous[proposal.Key]
			if ok && p.err == nil && p.size == psz {
				n.elog.Printf("Proposal with key: %s already applied. Skipping index: %d.\n",
					proposal.Key, proposal.Index)
				previous[proposal.Key].seen = time.Now() // Update the ts.
				// Don't break here. We still need to call the Done below.

			} else {
				start := time.Now()
				perr = n.applyCommitted(proposal)
				if len(proposal.Key) > 0 {
					p := &P{err: perr, size: psz, seen: time.Now()}
					previous[proposal.Key] = p
				}
				if perr != nil {
					glog.Errorf("Applying proposal. Error: %v. Proposal: %q.", perr, proposal)
				}
				n.elog.Printf("Applied proposal with key: %s, index: %d. Err: %v",
					proposal.Key, proposal.Index, perr)

				var tags []tag.Mutator
				switch {
				case proposal.Mutations != nil:
					tags = append(tags, tag.Upsert(x.KeyMethod, "apply.Mutations"))
				case proposal.Delta != nil:
					tags = append(tags, tag.Upsert(x.KeyMethod, "apply.Delta"))
				}
				ms := x.SinceMs(start)
				ostats.RecordWithTags(context.Background(), tags, x.LatencyMs.M(ms))
			}

			n.Proposals.Done(proposal.Key, perr)
			n.Applied.Done(proposal.Index)
		}
		if sz := atomic.AddInt64(&n.pendingSize, -totalSize); sz < 0 {
			glog.Warningf("Pending size should remain above zero: %d", sz)
		}
	}

	maxAge := 10 * time.Minute
	tick := time.NewTicker(maxAge / 2)
	defer tick.Stop()

	for {
		select {
		case entries, ok := <-n.applyCh:
			if !ok {
				return
			}
			handle(entries)
		case <-tick.C:
			// We use this ticker to clear out previous map.
			now := time.Now()
			for key, p := range previous {
				if now.Sub(p.seen) > maxAge {
					delete(previous, key)
				}
			}
			n.elog.Printf("Size of previous map: %d", len(previous))
		}
	}
}

func (n *node) commitOrAbort(pkey string, delta *pb.OracleDelta) error {
	// First let's commit all mutations to disk.
	writer := posting.NewTxnWriter(pstore)
	toDisk := func(start, commit uint64) {
		txn := posting.Oracle().GetTxn(start)
		if txn == nil {
			return
		}
		txn.Update()
		err := x.RetryUntilSuccess(x.WorkerConfig.MaxRetries, 10*time.Millisecond, func() error {
			return txn.CommitToDisk(writer, commit)
		})

		if err != nil {
			glog.Errorf("Error while applying txn status to disk (%d -> %d): %v",
				start, commit, err)
		}
	}

	for _, status := range delta.Txns {
		if status.CommitTs > 0 && status.CommitTs < n.lastCommitTs {
			glog.Errorf("Lastcommit %d > current %d. This would cause some commits to be lost.",
				n.lastCommitTs, status.CommitTs)
		}
		toDisk(status.StartTs, status.CommitTs)
		n.lastCommitTs = status.CommitTs
	}
	if err := writer.Flush(); err != nil {
		return x.Errorf("Error while flushing to disk: %v", err)
	}

	g := groups()
	atomic.StoreUint64(&g.deltaChecksum, delta.GroupChecksums[g.gid])

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

func (n *node) retrieveSnapshot(snap pb.Snapshot) error {
	// In some edge cases, the Zero leader might not have been able to update
	// the status of Alpha leader. So, instead of blocking forever on waiting
	// for Zero to send us the updates info about the leader, we can just use
	// the Snapshot RaftContext, which contains the address of the leader.
	var pool *conn.Pool
	addr := snap.Context.GetAddr()
	glog.V(2).Infof("Snapshot.RaftContext.Addr: %q", addr)
	if len(addr) > 0 {
		p, err := conn.Get().Get(addr)
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
	if _, err := n.populateSnapshot(snap, pool); err != nil {
		return fmt.Errorf("Cannot retrieve snapshot from peer, error: %v", err)
	}
	// Populate shard stores the streamed data directly into db, so we need to refresh
	// schema for current group id
	if err := schema.LoadFromDb(); err != nil {
		return fmt.Errorf("Error while initilizating schema: %+v", err)
	}
	groups().triggerMembershipSync()
	return nil
}

func (n *node) proposeSnapshot(discardN int) error {
	snap, err := n.calculateSnapshot(discardN)
	if err != nil {
		glog.Warningf("Got error while calculating snapshot: %v", err)
		return err
	}
	if snap == nil {
		return nil
	}
	proposal := &pb.Proposal{
		Snapshot: snap,
	}
	n.elog.Printf("Proposing snapshot: %+v\n", snap)
	data, err := proposal.Marshal()
	x.Check(err)
	return n.Raft().Propose(n.ctx, data)
}

const maxPendingSize int64 = 64 << 20 // in bytes.

func (n *node) rampMeter() {
	start := time.Now()
	defer func() {
		if dur := time.Since(start); dur > time.Second {
			glog.Infof("Blocked pushing to applyCh for %v", dur.Round(time.Millisecond))
		}
	}()
	for {
		if atomic.LoadInt64(&n.pendingSize) <= maxPendingSize {
			return
		}
		time.Sleep(3 * time.Millisecond)
	}
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
			n.Raft().TransferLeadership(n.ctx, x.WorkerConfig.RaftId, peerId)
			time.Sleep(time.Second) // Let transfer happen.
		}
		n.Raft().Stop()
		close(done)
	}()

	var snapshotLoops uint64
	for {
		select {
		case <-done:
			// We use done channel here instead of closer.HasBeenClosed so that we can transfer
			// leadership in a goroutine. The push to n.applyCh happens in this loop, so the close
			// should happen here too. Otherwise, race condition between push and close happens.
			close(n.applyCh)
			glog.Infoln("Raft node done.")
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
			start := time.Now()
			_, span := otrace.StartSpan(n.ctx, "Alpha.RunLoop",
				otrace.WithSampler(otrace.ProbabilitySampler(0.001)))

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
					// We are getting a new snapshot from leader. We need to wait for the applyCh to
					// finish applying the updates, otherwise, we'll end up overwriting the data
					// from the new snapshot that we retrieved.
					maxIndex := n.Applied.LastIndex()
					glog.Infof("Waiting for applyCh to become empty by reaching %d before"+
						" retrieving snapshot\n", maxIndex)
					n.Applied.WaitForMark(context.Background(), maxIndex)

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
						time.Sleep(100 * time.Millisecond) // Wait for a bit.
					}
					glog.Infof("---> SNAPSHOT: %+v. Group %d. DONE.\n", snap, n.gid)
				} else {
					glog.Infof("---> SNAPSHOT: %+v. Group %d from node id %#x [SELF]. Ignoring.\n",
						snap, n.gid, rc.Id)
				}
				if span != nil {
					span.Annotate(nil, "Applied or retrieved snapshot.")
				}
			}

			// Store the hardstate and entries. Note that these are not CommittedEntries.
			n.SaveToStorage(rd.HardState, rd.Entries, rd.Snapshot)
			diskDur := time.Since(start)
			if span != nil {
				span.Annotatef(nil, "Saved %d entries. Snapshot, HardState empty? (%v, %v)",
					len(rd.Entries),
					raft.IsEmptySnap(rd.Snapshot),
					raft.IsEmptyHardState(rd.HardState))
			}

			// Now schedule or apply committed entries.
			var proposals []*pb.Proposal
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

				} else if len(entry.Data) == 0 {
					n.elog.Printf("Found empty data at index: %d", entry.Index)
					n.Applied.Done(entry.Index)

				} else {
					proposal := &pb.Proposal{}
					if err := proposal.Unmarshal(entry.Data); err != nil {
						x.Fatalf("Unable to unmarshal proposal: %v %q\n", err, entry.Data)
					}
					if pctx := n.Proposals.Get(proposal.Key); pctx != nil {
						atomic.AddUint32(&pctx.Found, 1)
						if span := otrace.FromContext(pctx.Ctx); span != nil {
							span.Annotate(nil, "Proposal found in CommittedEntries")
						}
					}
					proposal.Index = entry.Index
					proposals = append(proposals, proposal)
				}
			}
			// Send the whole lot to applyCh in one go, instead of sending proposals one by one.
			if len(proposals) > 0 {
				// Apply the meter this before adding size to pending size so some crazy big
				// proposal can be pushed to applyCh. If this do this after adding its size to
				// pending size, we could block forever in rampMeter.
				n.rampMeter()
				var pendingSize int64
				for _, p := range proposals {
					pendingSize += int64(p.Size())
				}
				if sz := atomic.AddInt64(&n.pendingSize, pendingSize); sz > 2*maxPendingSize {
					glog.Warningf("Inflight proposal size: %d. There would be some throttling.", sz)
				}
				n.applyCh <- proposals
			}

			if span != nil {
				span.Annotatef(nil, "Handled %d committed entries.", len(rd.CommittedEntries))
			}

			if !leader {
				// Followers should send messages later.
				for _, msg := range rd.Messages {
					// NOTE: We can do some optimizations here to drop messages.
					n.Send(msg)
				}
			}
			if span != nil {
				span.Annotate(nil, "Followed queued messages.")
			}

			n.Raft().Advance()
			if firstRun && n.canCampaign {
				go n.Raft().Campaign(n.ctx)
				firstRun = false
			}
			if span != nil {
				span.Annotate(nil, "Advanced Raft. Done.")
				span.End()
				ostats.RecordWithTags(context.Background(),
					[]tag.Mutator{tag.Upsert(x.KeyMethod, "alpha.RunLoop")},
					x.LatencyMs.M(x.SinceMs(start)))
			}
			if time.Since(start) > 100*time.Millisecond {
				glog.Warningf(
					"Raft.Ready took too long to process: %v. Most likely due to slow disk: %v."+
						" Num entries: %d. MustSync: %v",
					time.Since(start).Round(time.Millisecond), diskDur.Round(time.Millisecond),
					len(rd.Entries), rd.MustSync)
			}
		}
	}
}

func listWrap(kv *bpb.KV) *bpb.KVList {
	return &bpb.KVList{Kv: []*bpb.KV{kv}}
}

// rollupLists would consolidate all the deltas that constitute one posting
// list, and write back a complete posting list.
func (n *node) rollupLists(readTs uint64) error {
	writer := posting.NewTxnWriter(pstore)

	// We're doing rollups. We should use this opportunity to calculate the tablet sizes.
	amLeader := n.AmLeader()
	m := new(sync.Map)

	addTo := func(key []byte, delta int64) {
		if !amLeader {
			// Only leader needs to calculate the tablet sizes.
			return
		}
		pk := x.Parse(key)
		if pk == nil {
			return
		}
		val, ok := m.Load(pk.Attr)
		if !ok {
			sz := new(int64)
			val, _ = m.LoadOrStore(pk.Attr, sz)
		}
		size := val.(*int64)
		atomic.AddInt64(size, delta)
	}

	stream := pstore.NewStreamAt(readTs)
	stream.LogPrefix = "Rolling up"
	stream.ChooseKey = func(item *badger.Item) bool {
		switch item.UserMeta() {
		case posting.BitSchemaPosting, posting.BitCompletePosting, posting.BitEmptyPosting:
			addTo(item.Key(), item.EstimatedSize())
			return false
		default:
			return true
		}
	}
	var numKeys uint64
	stream.KeyToList = func(key []byte, itr *badger.Iterator) (*bpb.KVList, error) {
		l, err := posting.ReadPostingList(key, itr)
		if err != nil {
			return nil, err
		}
		atomic.AddUint64(&numKeys, 1)
		kvs, err := l.Rollup()

		// If there are multiple keys, the posting list was split into multiple
		// parts. The key of the first part is the right key to use for tablet
		// size calculations.
		for _, kv := range kvs {
			addTo(kvs[0].Key, int64(kv.Size()))
		}

		return &bpb.KVList{Kv: kvs}, err
	}
	stream.Send = func(list *bpb.KVList) error {
		return writer.Send(&pb.KVS{Kv: list.Kv})
	}
	if err := stream.Orchestrate(context.Background()); err != nil {
		return err
	}
	if err := writer.Flush(); err != nil {
		return err
	}
	// For all the keys, let's see if they're in the LRU cache. If so, we can roll them up.
	glog.Infof("Rolled up %d keys. Done", atomic.LoadUint64(&numKeys))

	// We can now discard all invalid versions of keys below this ts.
	pstore.SetDiscardTs(readTs)

	if amLeader {
		// Only leader sends the tablet size updates to Zero. No one else does.
		// doSendMembership is also being concurrently called from another goroutine.
		go func() {
			tablets := make(map[string]*pb.Tablet)
			var total int64
			m.Range(func(key, val interface{}) bool {
				pred := key.(string)
				size := atomic.LoadInt64(val.(*int64))
				tablets[pred] = &pb.Tablet{
					GroupId:   n.gid,
					Predicate: pred,
					Space:     size,
				}
				total += size
				return true
			})
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
		}()
	}
	return nil
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
	glog.Infof("TryAbort returned with delta: %+v\n", delta)
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
	starts := posting.Oracle().TxnOlderThan(5 * time.Minute)
	if len(starts) == 0 {
		return
	}
	glog.Infof("Found %d old transactions. Acting to abort them.\n", len(starts))
	req := &pb.TxnTimestamps{Ts: starts}
	err := n.blockingAbort(req)
	glog.Infof("abortOldTransactions for %d txns. Error: %+v\n", len(req.Ts), err)
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
	_, span := otrace.StartSpan(n.ctx, "Calculate.Snapshot")
	defer span.End()

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

	last := n.Applied.DoneUntil()
	if int(last-first) < discardN {
		span.Annotate(nil, "Skipping due to insufficient entries")
		return nil, nil
	}
	span.Annotatef(nil, "Found Raft entries: %d", last-first)

	entries, err := n.Store.Entries(first, last+1, math.MaxUint64)
	if err != nil {
		span.Annotatef(nil, "Error: %v", err)
		return nil, err
	}

	if num := posting.Oracle().NumPendingTxns(); num > 0 {
		glog.Infof("Num pending txns: %d", num)
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
	maxCommitTs := snap.ReadTs
	var snapshotIdx uint64
	for _, entry := range entries {
		if entry.Type != raftpb.EntryNormal {
			continue
		}
		var proposal pb.Proposal
		if err := proposal.Unmarshal(entry.Data); err != nil {
			span.Annotatef(nil, "Error: %v", err)
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
		}
	}
	if maxCommitTs == 0 {
		span.Annotate(nil, "maxCommitTs is zero")
		return nil, nil
	}
	if snapshotIdx <= 0 {
		// It is possible that there are no pending transactions. In that case,
		// snapshotIdx would be zero.
		if len(entries) > 0 {
			snapshotIdx = entries[len(entries)-1].Index
		}
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
		Context: n.RaftContext,
		Index:   snapshotIdx,
		ReadTs:  maxCommitTs,
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
		return x.Errorf("Error while joining cluster: %+v\n", err)
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
		return false, x.Errorf("Error while joining cluster: %+v\n", err)
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
			glog.Errorf("Error while calling hasPeer: %v. Retrying...\n", err)
			time.Sleep(time.Second)
		}
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
