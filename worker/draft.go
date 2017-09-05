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
	"bytes"
	"encoding/binary"
	"fmt"
	"log"
	"math/rand"
	"sync"
	"time"

	"github.com/coreos/etcd/raft"
	"github.com/coreos/etcd/raft/raftpb"
	"golang.org/x/net/context"
	"golang.org/x/net/trace"

	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/raftwal"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
)

const (
	errorNodeIDExists = "Error Node ID already exists in the cluster"
)

type peerPoolEntry struct {
	// Never the empty string.  Possibly a bogus address -- bad port number, the value
	// of *myAddr, or some screwed up Raft config.
	addr string
	// An owning reference to a pool for this peer (or nil if addr is sufficiently bogus).
	poolOrNil *pool
}

// peerPool stores the peers' addresses and our connections to them.  It has exactly one
// entry for every peer other than ourselves.  Some of these peers might be unreachable or
// have bogus (but never empty) addresses.
type peerPool struct {
	sync.RWMutex
	peers map[uint64]peerPoolEntry
}

var (
	errNoPeerPoolEntry = fmt.Errorf("no peerPool entry")
	errNoPeerPool      = fmt.Errorf("no peerPool pool, could not connect")
)

// getPool returns the non-nil pool for a peer.  This might error even if get(id)
// succeeds, if the pool is nil.  This happens if the peer was configured so badly (it had
// a totally bogus addr) we can't make a pool.  (A reasonable refactoring would have us
// make a pool, one that has a nil gRPC connection.)
//
// You must call pools().release on the pool.
func (p *peerPool) getPool(id uint64) (*pool, error) {
	p.RLock()
	defer p.RUnlock()
	ent, ok := p.peers[id]
	if !ok {
		return nil, errNoPeerPoolEntry
	}
	if ent.poolOrNil == nil {
		return nil, errNoPeerPool
	}
	ent.poolOrNil.AddOwner()
	return ent.poolOrNil, nil
}

func (p *peerPool) get(id uint64) (string, bool) {
	p.RLock()
	defer p.RUnlock()
	ret, ok := p.peers[id]
	return ret.addr, ok
}

func (p *peerPool) set(id uint64, addr string, pl *pool) {
	p.Lock()
	defer p.Unlock()
	if old, ok := p.peers[id]; ok {
		if old.poolOrNil != nil {
			pools().release(old.poolOrNil)
		}
	}
	p.peers[id] = peerPoolEntry{addr, pl}
}

type proposalCtx struct {
	ch  chan error
	ctx context.Context
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

func (p *proposals) Ctx(pid uint32) (context.Context, bool) {
	p.RLock()
	defer p.RUnlock()
	if pd, has := p.ids[pid]; has {
		return pd.ctx, true
	}
	return nil, false
}

func (p *proposals) Done(pid uint32, err error) {
	p.Lock()
	pd, has := p.ids[pid]
	if has {
		delete(p.ids, pid)
	}
	p.Unlock()
	if !has {
		return
	}
	pd.ch <- err
}

func (p *proposals) Has(pid uint32) bool {
	p.RLock()
	defer p.RUnlock()
	_, has := p.ids[pid]
	return has
}

type sendmsg struct {
	to   uint64
	data []byte
}

type node struct {
	x.SafeMutex

	// SafeMutex is for fields which can be changed after init.
	_confState *raftpb.ConfState
	_raft      raft.Node

	// Changed after init but not protected by SafeMutex
	requestCh chan linReadReq

	// Fields which are never changed after init.
	cfg         *raft.Config
	applyCh     chan raftpb.Entry
	ctx         context.Context
	stop        chan struct{} // to send the stop signal to Run
	done        chan struct{} // to check whether node is running or not
	gid         uint32
	id          uint64
	messages    chan sendmsg
	peers       peerPool
	props       proposals
	raftContext *protos.RaftContext
	store       *raft.MemoryStorage
	wal         *raftwal.Wal

	canCampaign bool
	// applied is used to keep track of the applied RAFT proposals.
	// The stages are proposed -> committed (accepted by cluster) ->
	// applied (to PL) -> synced (to RocksDB).
	applied x.WaterMark
}

// SetRaft would set the provided raft.Node to this node.
// It would check fail if the node is already set.
func (n *node) SetRaft(r raft.Node) {
	n.Lock()
	defer n.Unlock()
	x.AssertTrue(n._raft == nil)
	n._raft = r
}

// Raft would return back the raft.Node stored in the node.
func (n *node) Raft() raft.Node {
	n.RLock()
	defer n.RUnlock()
	return n._raft
}

// SetConfState would store the latest ConfState generated by ApplyConfChange.
func (n *node) SetConfState(cs *raftpb.ConfState) {
	n.Lock()
	defer n.Unlock()
	n._confState = cs
}

// ConfState would return the latest ConfState stored in node.
func (n *node) ConfState() *raftpb.ConfState {
	n.RLock()
	defer n.RUnlock()
	return n._confState
}

func newNode(gid uint32, id uint64, myAddr string) *node {
	x.Printf("Node with GroupID: %v, ID: %v\n", gid, id)

	peers := peerPool{
		peers: make(map[uint64]peerPoolEntry),
	}
	props := proposals{
		ids: make(map[uint32]*proposalCtx),
	}

	store := raft.NewMemoryStorage()
	rc := &protos.RaftContext{
		Addr:  myAddr,
		Group: gid,
		Id:    id,
	}

	n := &node{
		requestCh: make(chan linReadReq),
		ctx:       context.Background(),
		id:        id,
		gid:       gid,
		store:     store,
		cfg: &raft.Config{
			ID:              id,
			ElectionTick:    100, // 200 ms if we call Tick() every 20 ms.
			HeartbeatTick:   1,   // 20 ms if we call Tick() every 20 ms.
			Storage:         store,
			MaxSizePerMsg:   256 << 10,
			MaxInflightMsgs: 256,
			Logger:          &raft.DefaultLogger{Logger: x.Logger},
			// Must be true for lease-based linearizable ReadIndex to work properly
			CheckQuorum: true,
			// We use lease-based linearizable ReadIndex for performance, at the cost of
			// correctness.  With it, communication goes follower->leader->follower, instead of
			// follower->leader->majority_of_followers->leader->follower.  We lose correctness
			// because the Raft ticker might not arrive promptly, in which case the leader would
			// falsely believe that its lease is still good.
			ReadOnlyOption: raft.ReadOnlyLeaseBased,
		},
		applyCh:     make(chan raftpb.Entry, numPendingMutations),
		peers:       peers,
		props:       props,
		raftContext: rc,
		messages:    make(chan sendmsg, 1000),
		stop:        make(chan struct{}),
		done:        make(chan struct{}),
	}
	n.applied = x.WaterMark{Name: fmt.Sprintf("Committed: Group %d", n.gid)}
	n.applied.Init()

	return n
}

// Never returns ("", true)
func (n *node) GetPeer(pid uint64) (string, bool) {
	return n.peers.get(pid)
}

// You must call release on the pool.  Can error for some pid's for which GetPeer
// succeeds.
func (n *node) GetPeerPool(pid uint64) (*pool, error) {
	return n.peers.getPool(pid)
}

// addr must not be empty.
func (n *node) SetPeer(pid uint64, addr string, poolOrNil *pool) {
	x.AssertTruef(addr != "", "SetPeer for peer %d has empty addr.", pid)
	n.peers.set(pid, addr, poolOrNil)
}

// Connects the node and makes its peerPool refer to the constructed pool and address
// (possibly updating ourselves from the old address.)  (Unless pid is ourselves, in which
// case this does nothing.)
func (n *node) Connect(pid uint64, addr string) {
	if pid == n.id {
		return
	}
	if paddr, ok := n.GetPeer(pid); ok && paddr == addr {
		// Already connected.
		return
	}
	// Here's what we do.  Right now peerPool maps peer node id's to addr values.  If
	// a *pool can be created, good, but if not, we still create a peerPoolEntry with
	// a nil *pool.
	p, ok := pools().connect(addr)
	if !ok {
		// TODO: Note this fact in more general peer health info somehow.
		x.Printf("Peer %d claims same host as me\n", pid)
	}
	n.SetPeer(pid, addr, p)
}

func (n *node) AddToCluster(ctx context.Context, pid uint64) error {
	addr, ok := n.GetPeer(pid)
	x.AssertTruef(ok, "Unable to find conn pool for peer: %d", pid)
	rc := &protos.RaftContext{
		Addr:  addr,
		Group: n.raftContext.Group,
		Id:    pid,
	}
	rcBytes, err := rc.Marshal()
	x.Check(err)
	return n.Raft().ProposeConfChange(ctx, raftpb.ConfChange{
		ID:      pid,
		Type:    raftpb.ConfChangeAddNode,
		NodeID:  pid,
		Context: rcBytes,
	})
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

func (n *node) ProposeAndWait(ctx context.Context, proposal *protos.Proposal) error {
	if n.Raft() == nil {
		return x.Errorf("RAFT isn't initialized yet")
	}
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
			if typ, err := schema.State().TypeOf(edge.Attr); err != nil {
				continue
			} else if err := validateAndConvert(edge, typ); err != nil {
				return err
			}
		}
		for _, schema := range proposal.Mutations.Schema {
			if err := checkSchema(schema); err != nil {
				return err
			}
		}
	}

	che := make(chan error, 1)
	pctx := &proposalCtx{
		ch:  che,
		ctx: ctx,
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

	//	we don't timeout on a mutation which has already been proposed.
	if err = n.Raft().Propose(ctx, slice[:upto]); err != nil {
		return x.Wrapf(err, "While proposing")
	}

	// Wait for the proposal to be committed.
	if proposal.Mutations != nil {
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf("Waiting for the proposal: mutations.")
		}
	} else if proposal.Membership != nil {
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf("Waiting for the proposal: membership update.")
		}
	} else {
		log.Fatalf("Unknown proposal")
	}

	err = <-che
	if err != nil {
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf(err.Error())
		}
	}
	return err
}

func (n *node) send(m raftpb.Message) {
	x.AssertTruef(n.id != m.To, "Seding message to itself")
	data, err := m.Marshal()
	x.Check(err)
	if m.Type != raftpb.MsgHeartbeat && m.Type != raftpb.MsgHeartbeatResp {
		x.Printf("\t\tSENDING: %v %v-->%v\n", m.Type, m.From, m.To)
	}
	select {
	case n.messages <- sendmsg{to: m.To, data: data}:
		// pass
	default:
		// TODO: It's bad to fail like this.
		x.Fatalf("Unable to push messages to channel in send")
	}
}

const (
	messageBatchSoftLimit = 10000000
)

func (n *node) batchAndSendMessages() {
	batches := make(map[uint64]*bytes.Buffer)
	for {
		totalSize := 0
		sm := <-n.messages
	slurp_loop:
		for {
			var buf *bytes.Buffer
			if b, ok := batches[sm.to]; !ok {
				buf = new(bytes.Buffer)
				batches[sm.to] = buf
			} else {
				buf = b
			}
			totalSize += 4 + len(sm.data)
			x.Check(binary.Write(buf, binary.LittleEndian, uint32(len(sm.data))))
			x.Check2(buf.Write(sm.data))

			if totalSize > messageBatchSoftLimit {
				// We limit the batch size, but we aren't pushing back on
				// n.messages, because the loop below spawns a goroutine
				// to do its dirty work.  This is good because right now
				// (*node).send fails(!) if the channel is full.
				break
			}

			select {
			case sm = <-n.messages:
			default:
				break slurp_loop
			}
		}

		for to, buf := range batches {
			if buf.Len() == 0 {
				continue
			}
			data := make([]byte, buf.Len())
			copy(data, buf.Bytes())
			go n.doSendMessage(to, data)
			buf.Reset()
		}
	}
}

func (n *node) doSendMessage(to uint64, data []byte) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	pool, err := n.GetPeerPool(to)
	if err != nil {
		// No such peer exists or we got handed a bogus config (bad addr), so we
		// can't send messages to this peer.
		return
	}
	defer pools().release(pool)
	conn := pool.Get()

	c := protos.NewWorkerClient(conn)
	p := &protos.Payload{Data: data}

	ch := make(chan error, 1)
	go func() {
		_, err = c.RaftMessage(ctx, p)
		ch <- err
	}()

	select {
	case <-ctx.Done():
		return
	case <-ch:
		// We don't need to do anything if we receive any error while sending message.
		// RAFT would automatically retry.
		return
	}
}

func (n *node) processMutation(ctx context.Context, index uint64, m *protos.Mutations) error {
	// TODO: Need to pass node and entry index.
	rv := x.RaftValue{Group: n.gid, Index: index}
	ctx = context.WithValue(ctx, "raft", rv)
	if err := runMutations(ctx, m.Edges); err != nil {
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf(err.Error())
		}
		return err
	}
	return nil
}

func (n *node) processSchemaMutations(e raftpb.Entry, m *protos.Mutations) error {
	// TODO: Need to pass node and entry index.
	rv := x.RaftValue{Group: n.gid, Index: e.Index}
	ctx := context.WithValue(n.ctx, "raft", rv)
	if err := runSchemaMutations(ctx, m.Schema); err != nil {
		if tr, ok := trace.FromContext(n.ctx); ok {
			tr.LazyPrintf(err.Error())
		}
		return err
	}
	return nil
}

func (n *node) processMembership(index uint64, mm *protos.Membership) error {
	x.AssertTrue(n.gid == 0)

	x.Printf("group: %v Addr: %q leader: %v dead: %v\n",
		mm.GroupId, mm.Addr, mm.Leader, mm.AmDead)
	groups().applyMembershipUpdate(index, mm)
	return nil
}

func (n *node) process(index uint64, proposal *protos.Proposal, pending chan struct{}) {
	defer func() {
		n.applied.Done(index)
		posting.SyncMarkFor(n.gid).Done(index)
	}()

	pending <- struct{}{} // This will block until we can write to it.
	x.ActiveMutations.Add(1)
	defer x.ActiveMutations.Add(-1)

	var err error
	if proposal.Mutations != nil {
		var ctx context.Context
		var has bool
		if ctx, has = n.props.Ctx(proposal.Id); !has {
			ctx = n.ctx
		}
		err = n.processMutation(ctx, index, proposal.Mutations)
	} else if proposal.Membership != nil {
		err = n.processMembership(index, proposal.Membership)
	}
	n.props.Done(proposal.Id, err)
	<-pending // Release one.
}

const numPendingMutations = 10000

func (n *node) processApplyCh() {
	pending := make(chan struct{}, numPendingMutations)

	for e := range n.applyCh {
		if len(e.Data) == 0 {
			n.applied.Done(e.Index)
			posting.SyncMarkFor(n.gid).Done(e.Index)
			continue
		}

		if e.Type == raftpb.EntryConfChange {
			var cc raftpb.ConfChange
			cc.Unmarshal(e.Data)

			if len(cc.Context) > 0 {
				var rc protos.RaftContext
				x.Check(rc.Unmarshal(cc.Context))
				n.Connect(rc.Id, rc.Addr)
			}

			cs := n.Raft().ApplyConfChange(cc)
			n.SetConfState(cs)
			n.applied.Done(e.Index)
			posting.SyncMarkFor(n.gid).Done(e.Index)
			continue
		}

		x.AssertTrue(e.Type == raftpb.EntryNormal)

		// The following effort is only to apply schema in a blocking fashion.
		// Once we have a scheduler, this should go away.
		// TODO: Move the following to scheduler.

		// We derive the schema here if it's not present
		// Since raft committed logs are serialized, we can derive
		// schema here without any locking
		proposal := &protos.Proposal{}
		if err := proposal.Unmarshal(e.Data); err != nil {
			log.Fatalf("Unable to unmarshal proposal: %v %q\n", err, e.Data)
		}

		if proposal.Mutations != nil {
			// process schema mutations before
			if proposal.Mutations.Schema != nil {
				// Wait for applied watermark to reach till previous index
				// All mutations before this should use old schema and after this
				// should use new schema

				// TODO: If n.ctx ever becomes cancellable, we need to handle the error somehow --
				// the watermark hasn't been hit.  This was changed to use context.Background()
				// when waitForSyncMark actually began using the context for cancellation.
				n.waitForSyncMark( /* n.ctx */ context.Background(), e.Index-1)
				if err := n.processSchemaMutations(e, proposal.Mutations); err != nil {
					n.applied.Done(e.Index)
					posting.SyncMarkFor(n.gid).Done(e.Index)
					n.props.Done(proposal.Id, err)
					continue
				}
			}

			// stores a map of predicate and type of first mutation for each predicate
			schemaMap := make(map[string]types.TypeID)
			for _, edge := range proposal.Mutations.Edges {
				if _, ok := schemaMap[edge.Attr]; !ok {
					schemaMap[edge.Attr] = posting.TypeID(edge)
				}
			}

			for attr, storageType := range schemaMap {
				if _, err := schema.State().TypeOf(attr); err != nil {
					// Schema doesn't exist
					// Since committed entries are serialized, updateSchemaIfMissing is not
					// needed, In future if schema needs to be changed, it would flow through
					// raft so there won't be race conditions between read and update schema
					updateSchemaType(attr, storageType, e.Index, n.raftContext.Group)
				}
			}
		}

		go n.process(e.Index, proposal, pending)
	}
}

func (n *node) saveToStorage(s raftpb.Snapshot, h raftpb.HardState,
	es []raftpb.Entry) {
	if !raft.IsEmptySnap(s) {
		le, err := n.store.LastIndex()
		if err != nil {
			log.Fatalf("While retrieving last index: %v\n", err)
		}
		if s.Metadata.Index <= le {
			return
		}

		if err := n.store.ApplySnapshot(s); err != nil {
			log.Fatalf("Applying snapshot: %v", err)
		}
	}

	if !raft.IsEmptyHardState(h) {
		n.store.SetHardState(h)
	}
	n.store.Append(es)
}

func (n *node) retrieveSnapshot(peerID uint64) {
	pool, err := n.GetPeerPool(peerID)
	if err != nil {
		// err is just going to be errNoConnection
		log.Fatalf("Cannot retrieve snapshot from peer %v, no connection.  Error: %v\n",
			peerID, err)
	}
	defer pools().release(pool)

	// Get index of last committed.
	lastIndex, err := n.store.LastIndex()
	x.Checkf(err, "Error while getting last index")
	// Wait for watermarks to sync since populateShard writes directly to db, otherwise
	// the values might get overwritten
	// Safe to keep this line
	n.syncAllMarks(n.ctx, lastIndex)
	// Need to clear pl's stored in memory for the case when retrieving snapshot with
	// index greater than this node's last index
	// Should invalidate/remove pl's to this group only ideally
	posting.EvictGroup(n.gid)
	if _, err := populateShard(n.ctx, pool, n.gid); err != nil {
		// TODO: We definitely don't want to just fall flat on our face if we can't
		// retrieve a simple snapshot.
		log.Fatalf("Cannot retrieve snapshot from peer %v, error: %v\n", peerID, err)
	}
	// Populate shard stores the streamed data directly into db, so we need to refresh
	// schema for current group id
	x.Checkf(schema.LoadFromDb(n.gid), "Error while initilizating schema")
}

type linReadReq struct {
	// A one-shot chan which we send a raft index upon
	indexCh chan<- uint64
}

func (n *node) readIndex(ctx context.Context) (chan uint64, error) {
	ch := make(chan uint64, 1)
	select {
	case n.requestCh <- linReadReq{ch}:
		return ch, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (n *node) runReadIndexLoop(stop <-chan struct{}, finished chan<- struct{},
	requestCh <-chan linReadReq, readStateCh <-chan raft.ReadState) {
	defer close(finished)
	counter := x.NewNonceCounter()
	requests := []linReadReq{}
	// We maintain one linearizable ReadIndex request at a time.  Others wait queued behind
	// requestCh.
	for {
		select {
		case <-stop:
			return
		case <-readStateCh:
			// Do nothing, discard ReadState info we don't have an activeRctx for
		case req := <-requestCh:
		slurpLoop:
			for {
				requests = append(requests, req)
				select {
				case req = <-requestCh:
				default:
					break slurpLoop
				}
			}
			activeRctx := counter.Generate()
			// We ignore the err - it would be n.ctx cancellation (which we must ignore because
			// it's our duty to continue until `stop` is triggered) or raft.ErrStopped (which we
			// must ignore for the same reason).
			_ = n.Raft().ReadIndex(n.ctx, activeRctx[:])
			// To see if the ReadIndex request succeeds, we need to use a timeout and wait for a
			// successful response.  If we don't see one, the raft leader wasn't configured, or the
			// raft leader didn't respond.
			timer := time.NewTimer(10 * time.Millisecond)
		again:
			select {
			case <-stop:
				timer.Stop()
				return
			case rs := <-readStateCh:
				if 0 != bytes.Compare(activeRctx[:], rs.RequestCtx) {
					goto again
				}
				timer.Stop()
				index := rs.Index
				for _, req := range requests {
					req.indexCh <- index
				}
			case <-timer.C:
				for _, req := range requests {
					req.indexCh <- raft.None
				}
			}
			requests = requests[:0]
		}
	}
}

func (n *node) Run() {
	firstRun := true
	var leader bool
	// See also our configuration of HeartbeatTick and ElectionTick.
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	rcBytes, err := n.raftContext.Marshal()
	x.Check(err)

	// This chan could have capacity zero, because runReadIndexLoop never blocks without selecting
	// on readStateCh.  It's 2 so that sending rarely blocks (so the Go runtime doesn't have to
	// switch threads as much.)
	readStateCh := make(chan raft.ReadState, 2)

	{
		// We only stop runReadIndexLoop after the for loop below has finished interacting with it.
		// That way we know sending to readStateCh will not deadlock.
		finished := make(chan struct{})
		stop := make(chan struct{})
		defer func() { <-finished }()
		defer close(stop)
		go n.runReadIndexLoop(stop, finished, n.requestCh, readStateCh)
	}

	for {
		select {
		case <-ticker.C:
			n.Raft().Tick()

		case rd := <-n.Raft().Ready():
			for _, rs := range rd.ReadStates {
				readStateCh <- rs
			}

			if rd.SoftState != nil {
				if rd.RaftState == raft.StateFollower && leader {
					// stepped down as leader do a sync membership immediately
					groups().syncMemberships()
				} else if rd.RaftState == raft.StateLeader && !leader {
					// TODO:wait for apply watermark ??
					leaseMgr().resetLease(n.gid)
					groups().syncMemberships()
				}
				leader = rd.RaftState == raft.StateLeader
			}
			x.Check(n.wal.StoreSnapshot(n.gid, rd.Snapshot))
			x.Check(n.wal.Store(n.gid, rd.HardState, rd.Entries))

			n.saveToStorage(rd.Snapshot, rd.HardState, rd.Entries)

			for _, msg := range rd.Messages {
				// NOTE: We can do some optimizations here to drop messages.
				msg.Context = rcBytes
				n.send(msg)
			}

			if !raft.IsEmptySnap(rd.Snapshot) {
				// We don't send snapshots to other nodes. But, if we get one, that means
				// either the leader is trying to bring us up to state; or this is the
				// snapshot that I created. Only the former case should be handled.
				var rc protos.RaftContext
				x.Check(rc.Unmarshal(rd.Snapshot.Data))
				x.AssertTrue(rc.Group == n.gid)
				if rc.Id != n.id {
					x.Printf("-------> SNAPSHOT [%d] from %d\n", n.gid, rc.Id)
					n.retrieveSnapshot(rc.Id)
					x.Printf("-------> SNAPSHOT [%d]. DONE.\n", n.gid)
				} else {
					x.Printf("-------> SNAPSHOT [%d] from %d [SELF]. Ignoring.\n", n.gid, rc.Id)
				}
			}
			if len(rd.CommittedEntries) > 0 {
				if tr, ok := trace.FromContext(n.ctx); ok {
					tr.LazyPrintf("Found %d committed entries", len(rd.CommittedEntries))
				}
			}

			for _, entry := range rd.CommittedEntries {
				// Need applied watermarks for schema mutation also for read linearazibility
				// Applied watermarks needs to be emitted as soon as possible sequentially.
				// If we emit Mark{4, false} and Mark{4, true} before emitting Mark{3, false}
				// then doneUntil would be set as 4 as soon as Mark{4,true} is done and before
				// Mark{3, false} is emitted. So it's safer to emit watermarks as soon as
				// possible sequentially
				n.applied.Begin(entry.Index)
				posting.SyncMarkFor(n.gid).Begin(entry.Index)

				// Just queue up to be processed. Don't wait on them.
				n.applyCh <- entry
			}

			n.Raft().Advance()
			if firstRun && n.canCampaign {
				go n.Raft().Campaign(n.ctx)
				firstRun = false
			}

		case <-n.stop:
			if peerId, has := groups().Peer(n.gid, Config.RaftId); has && n.AmLeader() {
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
					close(n.done)
				}()
			} else {
				n.Raft().Stop()
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

func (n *node) Step(ctx context.Context, msg raftpb.Message) error {
	return n.Raft().Step(ctx, msg)
}

func (n *node) snapshotPeriodically() {
	if n.gid == 0 {
		// Group zero is dedicated for membership information, whose state we don't persist.
		// So, taking snapshots would end up deleting the RAFT entries that we need to
		// regenerate the state on a crash. Therefore, don't take snapshots.
		return
	}

	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			n.snapshot(Config.MaxPendingCount)

		case <-n.done:
			return
		}
	}
}

func (n *node) snapshot(skip uint64) {
	if n.gid == 0 {
		// Group zero is dedicated for membership information, whose state we don't persist.
		// So, taking snapshots would end up deleting the RAFT entries that we need to
		// regenerate the state on a crash. Therefore, don't take snapshots.
		return
	}
	water := posting.SyncMarkFor(n.gid)
	le := water.DoneUntil()

	existing, err := n.store.Snapshot()
	x.Checkf(err, "Unable to get existing snapshot")

	si := existing.Metadata.Index
	if le <= si+skip {
		return
	}
	snapshotIdx := le - skip
	if tr, ok := trace.FromContext(n.ctx); ok {
		tr.LazyPrintf("Taking snapshot for group: %d at watermark: %d\n", n.gid, snapshotIdx)
	}
	rc, err := n.raftContext.Marshal()
	x.Check(err)

	s, err := n.store.CreateSnapshot(snapshotIdx, n.ConfState(), rc)
	x.Checkf(err, "While creating snapshot")
	x.Checkf(n.store.Compact(snapshotIdx), "While compacting snapshot")
	x.Check(n.wal.StoreSnapshot(n.gid, s))
}

func (n *node) joinPeers() {
	// Get leader information for MY group.
	pid, paddr := groups().Leader(n.gid)
	n.Connect(pid, paddr)
	x.Printf("joinPeers connected with: %q with peer id: %d\n", paddr, pid)

	pool, err := pools().get(paddr)
	if err != nil {
		log.Fatalf("Unable to get pool for addr: %q for peer: %d, error: %v\n", paddr, pid, err)
	}
	defer pools().release(pool)

	// Bring the instance up to speed first.
	// Raft would decide whether snapshot needs to fetched or not
	// so populateShard is not needed
	// _, err := populateShard(n.ctx, pool, n.gid)
	// x.Checkf(err, "Error while populating shard")

	conn := pool.Get()

	c := protos.NewWorkerClient(conn)
	x.Printf("Calling JoinCluster")
	_, err = c.JoinCluster(n.ctx, n.raftContext)
	x.Checkf(err, "Error while joining cluster")
	x.Printf("Done with JoinCluster call\n")
}

func (n *node) initFromWal(wal *raftwal.Wal) (restart bool, rerr error) {
	n.wal = wal

	var sp raftpb.Snapshot
	sp, rerr = wal.Snapshot(n.gid)
	if rerr != nil {
		return
	}
	var term, idx uint64
	if !raft.IsEmptySnap(sp) {
		x.Printf("Found Snapshot: %+v\n", sp)
		restart = true
		if rerr = n.store.ApplySnapshot(sp); rerr != nil {
			return
		}
		term = sp.Metadata.Term
		idx = sp.Metadata.Index
		n.applied.SetDoneUntil(idx)
		posting.SyncMarkFor(n.gid).SetDoneUntil(idx)
	}

	var hd raftpb.HardState
	hd, rerr = wal.HardState(n.gid)
	if rerr != nil {
		return
	}
	if !raft.IsEmptyHardState(hd) {
		x.Printf("Found hardstate: %+v\n", hd)
		restart = true
		if rerr = n.store.SetHardState(hd); rerr != nil {
			return
		}
	}

	var es []raftpb.Entry
	es, rerr = wal.Entries(n.gid, term, idx)
	if rerr != nil {
		return
	}
	x.Printf("Group %d found %d entries\n", n.gid, len(es))
	if len(es) > 0 {
		restart = true
	}
	rerr = n.store.Append(es)
	return
}

// InitAndStartNode gets called after having at least one membership sync with the cluster.
func (n *node) InitAndStartNode(wal *raftwal.Wal) {
	restart, err := n.initFromWal(wal)
	x.Check(err)

	if restart {
		x.Printf("Restarting node for group: %d\n", n.gid)
		_, found := groups().Server(Config.RaftId, n.gid)
		if !found && groups().HasPeer(n.gid) {
			n.joinPeers()
		}
		n.SetRaft(raft.RestartNode(n.cfg))

	} else {
		x.Printf("New Node for group: %d\n", n.gid)
		if groups().HasPeer(n.gid) {
			n.joinPeers()
			n.SetRaft(raft.StartNode(n.cfg, nil))

		} else {
			peers := []raft.Peer{{ID: n.id}}
			n.SetRaft(raft.StartNode(n.cfg, peers))
			// Trigger election, so this node can become the leader of this single-node cluster.
			n.canCampaign = true
		}
	}
	go n.processApplyCh()
	go n.Run()
	// TODO: Find a better way to snapshot, so we don't lose the membership
	// state information, which isn't persisted.
	go n.snapshotPeriodically()
	go n.batchAndSendMessages()
}

func (n *node) AmLeader() bool {
	if n.Raft() == nil {
		return false
	}
	r := n.Raft()
	return r.Status().Lead == r.Status().ID
}

var (
	errNoNode = fmt.Errorf("No node has been set up yet")
)

func applyMessage(ctx context.Context, msg raftpb.Message) error {
	var rc protos.RaftContext
	x.Check(rc.Unmarshal(msg.Context))
	node := groups().Node(rc.Group)
	if node == nil {
		// Maybe we went down, went back up, reconnected, and got an RPC
		// message before we set up Raft?
		return errNoNode
	}
	node.Connect(msg.From, rc.Addr)

	c := make(chan error, 1)
	go func() { c <- node.Step(ctx, msg) }()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-c:
		return err
	}
}

func (w *grpcWorker) RaftMessage(ctx context.Context, query *protos.Payload) (*protos.Payload, error) {
	if ctx.Err() != nil {
		return &protos.Payload{}, ctx.Err()
	}

	for idx := 0; idx < len(query.Data); {
		x.AssertTruef(len(query.Data[idx:]) >= 4,
			"Slice left of size: %v. Expected at least 4.", len(query.Data[idx:]))

		sz := int(binary.LittleEndian.Uint32(query.Data[idx : idx+4]))
		idx += 4
		msg := raftpb.Message{}
		if idx+sz > len(query.Data) {
			return &protos.Payload{}, x.Errorf(
				"Invalid query. Specified size %v overflows slice [%v,%v)\n",
				sz, idx, len(query.Data))
		}
		if err := msg.Unmarshal(query.Data[idx : idx+sz]); err != nil {
			x.Check(err)
		}
		if msg.Type != raftpb.MsgHeartbeat && msg.Type != raftpb.MsgHeartbeatResp {
			x.Printf("RECEIVED: %v %v-->%v\n", msg.Type, msg.From, msg.To)
		}
		if err := applyMessage(ctx, msg); err != nil {
			return &protos.Payload{}, err
		}
		idx += sz
	}
	// fmt.Printf("Got %d messages\n", count)
	return &protos.Payload{}, nil
}

func (w *grpcWorker) JoinCluster(ctx context.Context, rc *protos.RaftContext) (*protos.Payload, error) {
	if ctx.Err() != nil {
		return &protos.Payload{}, ctx.Err()
	}

	// Best effor reject
	if _, found := groups().Server(rc.Id, rc.Group); found || rc.Id == Config.RaftId {
		return &protos.Payload{}, x.Errorf(errorNodeIDExists)
	}

	node := groups().Node(rc.Group)
	if node == nil {
		return &protos.Payload{}, nil
	}
	node.Connect(rc.Id, rc.Addr)

	c := make(chan error, 1)
	go func() { c <- node.AddToCluster(ctx, rc.Id) }()

	select {
	case <-ctx.Done():
		return &protos.Payload{}, ctx.Err()
	case err := <-c:
		return &protos.Payload{}, err
	}
}

func waitLinearizableRead(ctx context.Context, gid uint32) error {
	n := groups().Node(gid)
	replyCh, err := n.readIndex(ctx)
	if err != nil {
		return err
	}
	select {
	case index := <-replyCh:
		if index == raft.None {
			return x.Errorf("cannot get linearized read (time expired or no configured leader)")
		}
		if err := n.applied.WaitForMark(ctx, index); err != nil {
			return err
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
