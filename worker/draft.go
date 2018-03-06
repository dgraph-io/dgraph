/*
 * Copyright (C) 2017 Dgraph Labs, Inc. and Contributors
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 */

package worker

import (
	"encoding/binary"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/coreos/etcd/raft"
	"github.com/coreos/etcd/raft/raftpb"
	"golang.org/x/net/context"
	"golang.org/x/net/trace"

	"github.com/dgraph-io/badger/y"
	"github.com/dgraph-io/dgraph/conn"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/api"
	"github.com/dgraph-io/dgraph/protos/intern"
	"github.com/dgraph-io/dgraph/raftwal"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/x"
	dy "github.com/dgraph-io/dgraph/y"
)

type proposalCtx struct {
	ch  chan error
	ctx context.Context
	cnt int // used for reference counting
	// Since each proposal consists of multiple tasks we need to store
	// non-nil error returned by task
	err   error
	index uint64 // RAFT index for the proposal.
	// Used for writing all deltas at end
	txn *posting.Txn
}

type proposals struct {
	sync.RWMutex
	ids map[uint32]*proposalCtx
}

func (p *proposals) Store(pid uint32, pctx *proposalCtx) bool {
	p.Lock()
	defer p.Unlock()
	if _, has := p.ids[pid]; has {
		return false
	}
	p.ids[pid] = pctx
	return true
}

func (p *proposals) IncRef(pid uint32, count int) {
	p.Lock()
	defer p.Unlock()
	pd, has := p.ids[pid]
	x.AssertTrue(has)
	pd.cnt += count
	return
}

func (p *proposals) pctx(pid uint32) *proposalCtx {
	p.RLock()
	defer p.RUnlock()
	return p.ids[pid]
}

func (p *proposals) CtxAndTxn(pid uint32) (context.Context, *posting.Txn) {
	p.RLock()
	defer p.RUnlock()
	pd, has := p.ids[pid]
	x.AssertTrue(has)
	return pd.ctx, pd.txn
}

func (p *proposals) Done(pid uint32, err error) {
	p.Lock()
	defer p.Unlock()
	pd, has := p.ids[pid]
	if !has {
		return
	}
	x.AssertTrue(pd.cnt > 0 && pd.index != 0)
	pd.cnt -= 1
	if err != nil {
		pd.err = err
	}
	if pd.cnt > 0 {
		return
	}
	delete(p.ids, pid)
	pd.ch <- pd.err
	// We emit one pending watermark as soon as we read from rd.committedentries.
	// Since the tasks are executed in goroutines we need one guarding watermark which
	// is done only when all the pending sync/applied marks have been emitted.
	groups().Node.Applied.Done(pd.index)
}

func (p *proposals) Has(pid uint32) bool {
	p.RLock()
	defer p.RUnlock()
	_, has := p.ids[pid]
	return has
}

type node struct {
	*conn.Node

	// Fields which are never changed after init.
	applyCh chan raftpb.Entry
	ctx     context.Context
	stop    chan struct{} // to send the stop signal to Run
	done    chan struct{} // to check whether node is running or not
	gid     uint32
	props   proposals

	canCampaign bool
	sch         *scheduler
}

func newNode(gid uint32, id uint64, myAddr string) *node {
	x.Printf("Node ID: %v with GroupID: %v\n", id, gid)

	rc := &intern.RaftContext{
		Addr:  myAddr,
		Group: gid,
		Id:    id,
	}
	m := conn.NewNode(rc)
	props := proposals{
		ids: make(map[uint32]*proposalCtx),
	}

	n := &node{
		Node: m,
		ctx:  context.Background(),
		gid:  gid,
		// processConfChange etc are not throttled so some extra delta, so that we don't
		// block tick when applyCh is full
		applyCh: make(chan raftpb.Entry, Config.NumPendingProposals+1000),
		props:   props,
		stop:    make(chan struct{}),
		done:    make(chan struct{}),
		sch:     new(scheduler),
	}
	n.sch.init(n)
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

// proposeAndWait sends a proposal through RAFT. It waits on a channel for the proposal
// to be applied(written to WAL) to all the nodes in the group.
func (n *node) proposeAndWait(ctx context.Context, proposal *intern.Proposal) error {
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
			if typ, err := schema.State().TypeOf(edge.Attr); err != nil {
				continue
			} else if err := ValidateAndConvert(edge, typ); err != nil {
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

	che := make(chan error, 1)
	pctx := &proposalCtx{
		ch:  che,
		ctx: ctx,
		cnt: 1,
	}
	for {
		id := rand.Uint32()
		if n.props.Store(id, pctx) {
			proposal.Id = id
			break
		}
	}

	sz := proposal.Size()
	slice := make([]byte, sz)

	upto, err := proposal.MarshalTo(slice)
	if err != nil {
		return err
	}

	// We don't timeout on a mutation which has already been proposed.
	if err = n.Raft().Propose(ctx, slice[:upto]); err != nil {
		return x.Wrapf(err, "While proposing")
	}

	if tr, ok := trace.FromContext(ctx); ok {
		tr.LazyPrintf("Waiting for the proposal.")
	}

	err = <-che
	if err != nil {
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf("Raft Propose error: %v", err)
		}
	}
	return err
}

func (n *node) processMutation(task *task) error {
	pid := task.pid
	ridx := task.rid
	edge := task.edge

	ctx, txn := n.props.CtxAndTxn(pid)
	if txn.ShouldAbort() {
		return dy.ErrConflict
	}
	rv := x.RaftValue{Group: n.gid, Index: ridx}
	ctx = context.WithValue(ctx, "raft", rv)
	if err := runMutation(ctx, edge, txn); err != nil {
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf("process mutation: %v", err)
		}
		return err
	}
	return nil
}

func (n *node) processSchemaMutations(pid uint32, index uint64,
	startTs uint64, s *intern.SchemaUpdate) error {
	ctx, _ := n.props.CtxAndTxn(pid)
	rv := x.RaftValue{Group: n.gid, Index: index}
	ctx = context.WithValue(ctx, "raft", rv)
	if err := runSchemaMutation(ctx, s, startTs); err != nil {
		if tr, ok := trace.FromContext(n.ctx); ok {
			tr.LazyPrintf(err.Error())
		}
		return err
	}
	return nil
}

func (n *node) applyConfChange(e raftpb.Entry) {
	var cc raftpb.ConfChange
	cc.Unmarshal(e.Data)

	if cc.Type == raftpb.ConfChangeRemoveNode {
		n.DeletePeer(cc.NodeID)
	} else if len(cc.Context) > 0 {
		var rc intern.RaftContext
		x.Check(rc.Unmarshal(cc.Context))
		n.Connect(rc.Id, rc.Addr)
	}

	cs := n.Raft().ApplyConfChange(cc)
	n.SetConfState(cs)
	n.DoneConfChange(cc.ID, nil)
	// Not present in proposal map
	n.Applied.Done(e.Index)
	groups().triggerMembershipSync()
}

type KeyValueOrCleanProposal struct {
	raftIdx  uint64
	proposal *intern.Proposal
}

func (n *node) processKeyValueOrCleanProposals(
	kvChan chan KeyValueOrCleanProposal) {
	// Run KeyValueProposals and CleanPredicate one by one always.
	// During predicate move we first clean the predicate and then
	// propose key values, we wait for clean predicate to be done before
	// we propose key values. But during replay if we run these proposals
	// in goroutine then we will have no such guarantees so always run
	// them sequentially.
	for e := range kvChan {
		if len(e.proposal.Kv) > 0 {
			n.processKeyValues(e.raftIdx, e.proposal.Id, e.proposal.Kv)
		} else if len(e.proposal.CleanPredicate) > 0 {
			n.deletePredicate(e.raftIdx, e.proposal.Id, e.proposal.CleanPredicate)
		} else {
			x.Fatalf("Unknown proposal, %+v\n", e.proposal)
		}
	}
}

func (n *node) processApplyCh() {
	kvChan := make(chan KeyValueOrCleanProposal, 1000)
	go n.processKeyValueOrCleanProposals(kvChan)

	for e := range n.applyCh {
		if len(e.Data) == 0 {
			// This is not in the proposal map
			n.Applied.Done(e.Index)
			continue
		}

		if e.Type == raftpb.EntryConfChange {
			n.applyConfChange(e)
			continue
		}

		x.AssertTrue(e.Type == raftpb.EntryNormal)

		proposal := &intern.Proposal{}
		if err := proposal.Unmarshal(e.Data); err != nil {
			x.Fatalf("Unable to unmarshal proposal: %v %q\n", err, e.Data)
		}

		// One final applied and synced watermark would be emitted when proposal ctx ref count
		// becomes zero.
		pctx := n.props.pctx(proposal.Id)
		if pctx == nil {
			// This is during replay of logs after restart
			pctx = &proposalCtx{
				ch:  make(chan error, 1),
				ctx: n.ctx,
				cnt: 1,
			}
		}
		pctx.index = e.Index
		n.props.Store(proposal.Id, pctx)

		posting.TxnMarks().Begin(e.Index)
		if proposal.Mutations != nil {
			// syncmarks for this shouldn't be marked done until it's comitted.
			n.sch.schedule(proposal, e.Index)
		} else if len(proposal.Kv) > 0 {
			kvChan <- KeyValueOrCleanProposal{
				raftIdx:  e.Index,
				proposal: proposal,
			}
		} else if proposal.State != nil {
			// This state needn't be snapshotted in this group, on restart we would fetch
			// a state which is latest or equal to this.
			groups().applyState(proposal.State)
			// When proposal is done it emits done watermarks.
			posting.TxnMarks().Done(e.Index)
			n.props.Done(proposal.Id, nil)
		} else if len(proposal.CleanPredicate) > 0 {
			kvChan <- KeyValueOrCleanProposal{
				raftIdx:  e.Index,
				proposal: proposal,
			}
		} else if proposal.TxnContext != nil {
			go n.commitOrAbort(e.Index, proposal.Id, proposal.TxnContext)
		} else {
			x.Fatalf("Unknown proposal")
		}
	}
	close(kvChan)
}

func (n *node) commitOrAbort(index uint64, pid uint32, tctx *api.TxnContext) {
	ctx, _ := n.props.CtxAndTxn(pid)
	_, err := commitOrAbort(ctx, tctx)
	if tr, ok := trace.FromContext(ctx); ok {
		tr.LazyPrintf("Status of commitOrAbort %+v %v\n", tctx, err)
	}
	if err == nil {
		posting.Txns().Done(tctx.StartTs)
		posting.Oracle().Done(tctx.StartTs)
	}
	posting.TxnMarks().Done(index)
	n.props.Done(pid, err)
}

func (n *node) deletePredicate(index uint64, pid uint32, predicate string) {
	ctx, _ := n.props.CtxAndTxn(pid)
	rv := x.RaftValue{Group: n.gid, Index: index}
	ctx = context.WithValue(ctx, "raft", rv)
	err := posting.DeletePredicate(ctx, predicate)
	posting.TxnMarks().Done(index)
	n.props.Done(pid, err)
}

func (n *node) processKeyValues(index uint64, pid uint32, kvs []*intern.KV) error {
	ctx, _ := n.props.CtxAndTxn(pid)
	err := populateKeyValues(ctx, kvs)
	posting.TxnMarks().Done(index)
	n.props.Done(pid, err)
	return nil
}

func (n *node) applyAllMarks(ctx context.Context) {
	// Get index of last committed.
	lastIndex := n.Applied.LastIndex()
	n.Applied.WaitForMark(ctx, lastIndex)
}

func (n *node) retrieveSnapshot() error {
	pool := groups().Leader(groups().groupId())
	if pool == nil {
		// retrieveSnapshot is blocking at initial start and leader election for a group might
		// not have happened when it is called. If we can't find a leader, get latest state from
		// Zero.
		if err := UpdateMembershipState(context.Background()); err != nil {
			return fmt.Errorf("Error while trying to update membership state: %+v", err)
		}
		return fmt.Errorf("Unable to reach leader in group %d", n.gid)
	}

	// Wait for watermarks to sync since populateShard writes directly to db, otherwise
	// the values might get overwritten
	// Safe to keep this line
	n.applyAllMarks(n.ctx)

	// Need to clear pl's stored in memory for the case when retrieving snapshot with
	// index greater than this node's last index
	// Should invalidate/remove pl's to this group only ideally
	posting.EvictLRU()
	if _, err := n.populateShard(pstore, pool); err != nil {
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

func (n *node) Run() {
	firstRun := true
	var leader bool
	// See also our configuration of HeartbeatTick and ElectionTick.
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	rcBytes, err := n.RaftContext.Marshal()
	x.Check(err)

	// Ensure we don't exit unless any snapshot in progress in done.
	closer := y.NewCloser(1)
	go n.snapshotPeriodically(closer)

	for {
		select {
		case <-ticker.C:
			n.Raft().Tick()

		case rd := <-n.Raft().Ready():
			if rd.SoftState != nil {
				groups().triggerMembershipSync()
				leader = rd.RaftState == raft.StateLeader
			}
			if leader {
				// Leader can send messages in parallel with writing to disk.
				for _, msg := range rd.Messages {
					// NOTE: We can do some optimizations here to drop messages.
					msg.Context = rcBytes
					n.Send(msg)
				}
			}

			// First store the entries, then the hardstate and snapshot.
			x.Check(n.Wal.Store(n.gid, rd.HardState, rd.Entries))

			// Now store them in the in-memory store.
			n.SaveToStorage(rd.HardState, rd.Entries)

			if !raft.IsEmptySnap(rd.Snapshot) {
				// We don't send snapshots to other nodes. But, if we get one, that means
				// either the leader is trying to bring us up to state; or this is the
				// snapshot that I created. Only the former case should be handled.
				var rc intern.RaftContext
				x.Check(rc.Unmarshal(rd.Snapshot.Data))
				x.AssertTrue(rc.Group == n.gid)
				if rc.Id != n.Id {
					// NOTE: Retrieving snapshot here is OK, after storing it above in WAL, because
					// rc.Id != n.Id.
					x.Printf("-------> SNAPSHOT [%d] from %d\n", n.gid, rc.Id)
					// It's ok to block tick while retrieving snapshot, since it's a follower
					n.retryUntilSuccess(n.retrieveSnapshot, 100*time.Millisecond)
					x.Printf("-------> SNAPSHOT [%d]. DONE.\n", n.gid)
				} else {
					x.Printf("-------> SNAPSHOT [%d] from %d [SELF]. Ignoring.\n", n.gid, rc.Id)
				}
				x.Check(n.Wal.StoreSnapshot(n.gid, rd.Snapshot))
				n.SaveSnapshot(rd.Snapshot)
			}

			if len(rd.CommittedEntries) > 0 {
				if tr, ok := trace.FromContext(n.ctx); ok {
					tr.LazyPrintf("Found %d committed entries", len(rd.CommittedEntries))
				}
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

				if !leader && entry.Type == raftpb.EntryConfChange {
					// Config changes in followers must be applied straight away.
					n.applyConfChange(entry)
				} else {
					// TODO: Stop accepting requests when applyCh is full
					// Just queue up to be processed. Don't wait on them.
					n.applyCh <- entry
				}
			}

			if !leader {
				// Followers should send messages later.
				for _, msg := range rd.Messages {
					// NOTE: We can do some optimizations here to drop messages.
					msg.Context = rcBytes
					n.Send(msg)
				}
			}
			n.Raft().Advance()
			if firstRun && n.canCampaign {
				go n.Raft().Campaign(n.ctx)
				firstRun = false
			}

		case <-n.stop:
			if peerId, has := groups().MyPeer(); has && n.AmLeader() {
				n.Raft().TransferLeadership(n.ctx, Config.RaftId, peerId)
				go func() {
					select {
					case <-n.ctx.Done(): // time out
						if tr, ok := trace.FromContext(n.ctx); ok {
							tr.LazyPrintf("context timed out while transfering leadership")
						}
					case <-time.After(1 * time.Second):
						if tr, ok := trace.FromContext(n.ctx); ok {
							tr.LazyPrintf("Timed out transfering leadership")
						}
					}
					n.Raft().Stop()
					closer.SignalAndWait()
					close(n.done)
				}()
			} else {
				n.Raft().Stop()
				closer.SignalAndWait()
				close(n.done)
			}
		case <-n.done:
			return
		}
	}
}

func (n *node) Stop() {
	select {
	case n.stop <- struct{}{}:
	case <-n.done:
		// already stopped.
		return
	}
	<-n.done // wait for Run to respond.
}

func (n *node) snapshotPeriodically(closer *y.Closer) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			n.snapshot(Config.MaxPendingCount)

		case <-closer.HasBeenClosed():
			closer.Done()
			return
		}
	}
}

func (n *node) snapshot(skip uint64) {
	water := posting.TxnMarks()
	le := water.DoneUntil()

	existing, err := n.Store.Snapshot()
	x.Checkf(err, "Unable to get existing snapshot")

	si := existing.Metadata.Index
	if le <= si+skip {
		// If difference grows above 100*skip we try to abort old transactions, so it shouldn't
		// ideally go above 110*skip.
		applied := n.Applied.DoneUntil()
		if applied-le > 110*skip {
			x.Printf("Couldn't take snapshot, txn watermark: [%d], applied watermark: [%d]\n",
				le, applied)
		}
		return
	}
	snapshotIdx := le - skip
	if tr, ok := trace.FromContext(n.ctx); ok {
		tr.LazyPrintf("Taking snapshot for group: %d at watermark: %d\n", n.gid, snapshotIdx)
	}

	rc, err := n.RaftContext.Marshal()
	x.Check(err)

	s, err := n.Store.CreateSnapshot(snapshotIdx, n.ConfState(), rc)
	x.Checkf(err, "While creating snapshot")
	x.Checkf(n.Store.Compact(snapshotIdx), "While compacting snapshot")
	x.Check(n.Wal.StoreSnapshot(n.gid, s))
}

func (n *node) joinPeers() error {
	// Get leader information for MY group.
	pl := groups().Leader(n.gid)
	if pl == nil {
		if err := UpdateMembershipState(context.Background()); err != nil {
			return fmt.Errorf("Error while trying to update membership state: %+v", err)
		}
		return x.Errorf("Unable to reach leader or any other server in group %d", n.gid)
	}

	gconn := pl.Get()
	c := intern.NewRaftClient(gconn)
	x.Printf("Calling JoinCluster")
	ctx, cancel := context.WithTimeout(n.ctx, time.Second)
	defer cancel()
	// JoinCluster can block indefinitely, raft ignores conf change proposal
	// if it has pending configuration.
	if _, err := c.JoinCluster(ctx, n.RaftContext); err != nil {
		return x.Errorf("Error while joining cluster: %+v\n", err)
	}
	x.Printf("Done with JoinCluster call\n")
	return nil
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
func (n *node) InitAndStartNode(wal *raftwal.Wal) {
	idx, restart, err := n.InitFromWal(wal)
	x.Check(err)
	n.Applied.SetDoneUntil(idx)
	posting.TxnMarks().SetDoneUntil(idx)

	if restart {
		x.Printf("Restarting node for group: %d\n", n.gid)
		sp, err := n.Store.Snapshot()
		x.Checkf(err, "Unable to get existing snapshot")
		if !raft.IsEmptySnap(sp) {
			members := groups().members(n.gid)
			for _, id := range sp.Metadata.ConfState.Nodes {
				n.Connect(id, members[id].Addr)
			}
		}
		n.SetRaft(raft.RestartNode(n.Cfg))
	} else {
		x.Printf("New Node for group: %d\n", n.gid)
		if _, hasPeer := groups().MyPeer(); hasPeer {
			// Get snapshot before joining peers as it can take time to retrieve it and we dont
			// want the quorum to be inactive when it happens.

			x.Println("Retrieving snapshot.")
			n.retryUntilSuccess(n.retrieveSnapshot, time.Second)

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
	go n.processApplyCh()
	go n.Run()
	go n.BatchAndSendMessages()
}

func (n *node) AmLeader() bool {
	if n.Raft() == nil {
		return false
	}
	r := n.Raft()
	return r.Status().Lead == r.Status().ID
}
