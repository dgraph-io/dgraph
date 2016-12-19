package worker

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"log"
	"math/rand"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/coreos/etcd/raft"
	"github.com/coreos/etcd/raft/raftpb"
	"golang.org/x/net/context"

	"github.com/dgraph-io/dgraph/raftwal"
	"github.com/dgraph-io/dgraph/task"
	"github.com/dgraph-io/dgraph/x"
)

type peerPool struct {
	sync.RWMutex
	peers map[uint64]string
}

func (p *peerPool) Get(id uint64) string {
	p.RLock()
	defer p.RUnlock()
	return p.peers[id]
}

func (p *peerPool) Set(id uint64, addr string) {
	p.Lock()
	defer p.Unlock()
	p.peers[id] = addr
}

type proposals struct {
	sync.RWMutex
	ids map[uint32]chan error
}

func (p *proposals) Store(pid uint32, ch chan error) {
	p.Lock()
	defer p.Unlock()
	_, has := p.ids[pid]
	x.AssertTruef(!has, "Same proposal is being stored again.")
	p.ids[pid] = ch
}

func (p *proposals) Done(pid uint32, err error) {
	var ch chan error
	p.Lock()
	ch, has := p.ids[pid]
	if has {
		delete(p.ids, pid)
	}
	p.Unlock()
	if !has {
		return
	}
	ch <- err
}

type sendmsg struct {
	to   uint64
	data []byte
}

type node struct {
	canCampaign bool
	cfg         *raft.Config
	commitCh    chan raftpb.Entry
	ctx         context.Context
	done        chan struct{}
	gid         uint32
	id          uint64
	messages    chan sendmsg
	peers       peerPool
	props       proposals
	raft        raft.Node
	raftContext *task.RaftContext
	store       *raft.MemoryStorage
	wal         *raftwal.Wal
	// applied is used to keep track of the applied RAFT proposals.
	// The stages are proposed -> committed (accepted by cluster) ->
	// applied (to PL) -> synced (to RocksDB).
	applied x.WaterMark
}

func newNode(gid uint32, id uint64, myAddr string) *node {
	fmt.Printf("NEW NODE GID, ID: [%v, %v]\n", gid, id)

	peers := peerPool{
		peers: make(map[uint64]string),
	}
	props := proposals{
		ids: make(map[uint32]chan error),
	}

	store := raft.NewMemoryStorage()
	rc := &task.RaftContext{
		Addr:  myAddr,
		Group: gid,
		Id:    id,
	}

	n := &node{
		ctx:   context.Background(),
		id:    id,
		gid:   gid,
		store: store,
		cfg: &raft.Config{
			ID:              id,
			ElectionTick:    10,
			HeartbeatTick:   1,
			Storage:         store,
			MaxSizePerMsg:   4096,
			MaxInflightMsgs: 256,
		},
		commitCh:    make(chan raftpb.Entry, numPendingMutations),
		peers:       peers,
		props:       props,
		raftContext: rc,
		messages:    make(chan sendmsg, 1000),
	}
	n.applied = x.WaterMark{Name: "Committed Mark"}
	n.applied.Init()

	return n
}

func (n *node) Connect(pid uint64, addr string) {
	if pid == n.id {
		return
	}
	if paddr := n.peers.Get(pid); paddr == addr {
		return
	}
	pools().connect(addr)
	n.peers.Set(pid, addr)
}

func (n *node) AddToCluster(ctx context.Context, pid uint64) error {
	addr := n.peers.Get(pid)
	x.AssertTruef(len(addr) > 0, "Unable to find conn pool for peer: %d", pid)
	rc := &task.RaftContext{
		Addr:  addr,
		Group: n.raftContext.Group,
		Id:    pid,
	}
	rcBytes, err := rc.Marshal()
	x.Check(err)
	return n.raft.ProposeConfChange(ctx, raftpb.ConfChange{
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

var slicePool = sync.Pool{
	New: func() interface{} {
		return make([]byte, 256<<10)
	},
}

func (n *node) ProposeAndWait(ctx context.Context, proposal *task.Proposal) error {
	if n.raft == nil {
		return x.Errorf("RAFT isn't initialized yet")
	}

	proposal.Id = rand.Uint32()

	slice := slicePool.Get().([]byte)
	if len(slice) < proposal.Size() {
		slice = make([]byte, proposal.Size())
	}
	defer slicePool.Put(slice)

	upto, err := proposal.MarshalTo(slice)
	if err != nil {
		return err
	}
	proposalData := slice[:upto]

	che := make(chan error, 1)
	n.props.Store(proposal.Id, che)

	if err = n.raft.Propose(ctx, proposalData); err != nil {
		return x.Wrapf(err, "While proposing")
	}

	// Wait for the proposal to be committed.
	if proposal.Mutations != nil {
		x.Trace(ctx, "Waiting for the proposal: mutations.")
	} else {
		x.Trace(ctx, "Waiting for the proposal: membership update.")
	}

	select {
	case err = <-che:
		x.TraceError(ctx, err)
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (n *node) send(m raftpb.Message) {
	x.AssertTruef(n.id != m.To, "Seding message to itself")
	data, err := m.Marshal()
	x.Check(err)
	if m.Type != raftpb.MsgHeartbeat && m.Type != raftpb.MsgHeartbeatResp {
		fmt.Printf("\t\tSENDING: %v %v-->%v\n", m.Type, m.From, m.To)
	}
	select {
	case n.messages <- sendmsg{to: m.To, data: data}:
		// pass
	default:
		log.Fatalf("Unable to push messages to channel in send")
	}
}

func (n *node) doSendMessage(to uint64, data []byte) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	addr := n.peers.Get(to)
	if len(addr) == 0 {
		return
	}
	pool := pools().get(addr)
	conn, err := pool.Get()
	x.Check(err)
	defer pool.Put(conn)

	c := NewWorkerClient(conn)
	p := &Payload{Data: data}

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

func (n *node) batchAndSendMessages() {
	batches := make(map[uint64]*bytes.Buffer)
	ticker := time.NewTicker(10 * time.Millisecond)
	for {
		select {
		case sm := <-n.messages:
			if _, ok := batches[sm.to]; !ok {
				batches[sm.to] = new(bytes.Buffer)
			}
			buf := batches[sm.to]
			binary.Write(buf, binary.LittleEndian, uint32(len(sm.data)))
			buf.Write(sm.data)

		case <-ticker.C:
			for to, buf := range batches {
				if buf.Len() == 0 {
					continue
				}
				go n.doSendMessage(to, buf.Bytes())
				buf.Reset()
			}
		}
	}
}

func (n *node) processMutation(e raftpb.Entry, m *task.Mutations) error {
	// TODO: Need to pass node and entry index.
	rv := x.RaftValue{Group: n.gid, Index: e.Index}
	ctx := context.WithValue(n.ctx, "raft", rv)
	if err := runMutations(ctx, m.Edges); err != nil {
		x.TraceError(n.ctx, err)
		return err
	}
	return nil
}

func (n *node) processMembership(e raftpb.Entry, mm *task.Membership) error {
	x.AssertTrue(n.gid == 0)

	x.Printf("group: %v Addr: %q leader: %v dead: %v\n",
		mm.GroupId, mm.Addr, mm.Leader, mm.AmDead)
	groups().applyMembershipUpdate(e.Index, mm)
	return nil
}

func (n *node) process(e raftpb.Entry, pending chan struct{}) {
	defer func() {
		n.applied.Ch <- x.Mark{Index: e.Index, Done: true}
	}()

	if e.Type != raftpb.EntryNormal {
		return
	}

	pending <- struct{}{} // This will block until we can write to it.
	var proposal task.Proposal
	x.Check(proposal.Unmarshal(e.Data))

	var err error
	if proposal.Mutations != nil {
		err = n.processMutation(e, proposal.Mutations)
	} else if proposal.Membership != nil {
		err = n.processMembership(e, proposal.Membership)
	}
	n.props.Done(proposal.Id, err)
	<-pending // Release one.
}

const numPendingMutations = 10000

func (n *node) processCommitCh() {
	pending := make(chan struct{}, numPendingMutations)

	for e := range n.commitCh {
		if e.Data == nil {
			n.applied.Ch <- x.Mark{Index: e.Index, Done: true}
			continue
		}

		if e.Type == raftpb.EntryConfChange {
			var cc raftpb.ConfChange
			cc.Unmarshal(e.Data)

			if len(cc.Context) > 0 {
				var rc task.RaftContext
				x.Check(rc.Unmarshal(cc.Context))
				n.Connect(rc.Id, rc.Addr)
			}

			n.raft.ApplyConfChange(cc)
			n.applied.Ch <- x.Mark{Index: e.Index, Done: true}

		} else {
			go n.process(e, pending)
		}
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

func (n *node) processSnapshot(s raftpb.Snapshot) {
	lead := n.raft.Status().Lead
	if lead == 0 {
		return
	}
	addr := n.peers.Get(lead)
	x.AssertTruef(addr != "", "Should have the leader address: %v", lead)
	pool := pools().get(addr)
	x.AssertTruef(pool != nil, "Leader: %d pool should not be nil", lead)

	_, err := populateShard(context.TODO(), pool, 0)
	x.Checkf(err, "processSnapshot")
}

func (n *node) Run() {
	firstRun := true
	ticker := time.NewTicker(time.Second)
	for {
		select {
		case <-ticker.C:
			n.raft.Tick()

		case rd := <-n.raft.Ready():
			x.Check(n.wal.Store(n.gid, rd.Snapshot, rd.HardState, rd.Entries))
			n.saveToStorage(rd.Snapshot, rd.HardState, rd.Entries)
			rcBytes, err := n.raftContext.Marshal()
			for _, msg := range rd.Messages {
				// NOTE: We can do some optimizations here to drop messages.
				x.Check(err)
				msg.Context = rcBytes
				n.send(msg)
			}

			if !raft.IsEmptySnap(rd.Snapshot) {
				n.processSnapshot(rd.Snapshot)
			}
			if len(rd.CommittedEntries) > 0 {
				x.Trace(n.ctx, "Found %d committed entries", len(rd.CommittedEntries))
			}
			for _, entry := range rd.CommittedEntries {
				// Just queue up to be processed. Don't wait on them.
				n.commitCh <- entry
				status := x.Mark{Index: entry.Index, Done: false}
				if entry.Index == 2 {
					fmt.Printf("%+v\n", entry)
				}
				n.applied.Ch <- status
			}

			n.raft.Advance()
			if firstRun && n.canCampaign {
				go n.raft.Campaign(n.ctx)
				firstRun = false
			}

		case <-n.done:
			return
		}
	}
}

func (n *node) Stop() {
	close(n.done)
}

func (n *node) Step(ctx context.Context, msg raftpb.Message) error {
	return n.raft.Step(ctx, msg)
}

func (n *node) snapshotPeriodically() {
	ticker := time.NewTicker(10 * time.Minute)
	for {
		select {
		case <-ticker.C:
			le, err := n.store.LastIndex()
			x.Checkf(err, "Unable to retrieve last index")

			existing, err := n.store.Snapshot()
			x.Checkf(err, "Unable to get existing snapshot")

			si := existing.Metadata.Index
			if le <= si {
				continue
			}

			msg := fmt.Sprintf("Snapshot from %v", strconv.FormatUint(n.id, 10))
			_, err = n.store.CreateSnapshot(le, nil, []byte(msg))
			x.Checkf(err, "While creating snapshot")
			x.Checkf(n.store.Compact(le), "While compacting snapshot")

		case <-n.done:
			return
		}
	}
}

func parsePeer(peer string) (uint64, string) {
	x.AssertTrue(len(peer) > 0)

	kv := strings.SplitN(peer, ":", 2)
	x.AssertTruef(len(kv) == 2, "Invalid peer format: %v", peer)
	pid, err := strconv.ParseUint(kv[0], 10, 64)
	x.Checkf(err, "Invalid peer id: %v", kv[0])
	// TODO: Validate the url kv[1]
	return pid, kv[1]
}

func (n *node) joinPeers() {
	// Get leader information for MY group.
	pid, paddr := groups().Leader(n.gid)
	n.Connect(pid, paddr)
	fmt.Printf("Connected with: %v\n", paddr)

	addr := n.peers.Get(pid)
	pool := pools().get(addr)
	x.AssertTruef(pool != nil, "Unable to find addr for peer: %d", pid)

	// Bring the instance up to speed first.
	_, err := populateShard(n.ctx, pool, 0)
	x.Checkf(err, "Error while populating shard")

	conn, err := pool.Get()
	x.Check(err)
	defer pool.Put(conn)

	c := NewWorkerClient(conn)
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
		fmt.Printf("Found Snapshot: %+v\n", sp)
		restart = true
		if rerr = n.store.ApplySnapshot(sp); rerr != nil {
			return
		}
		term = sp.Metadata.Term
		idx = sp.Metadata.Index
	}

	var hd raftpb.HardState
	hd, rerr = wal.HardState(n.gid)
	if rerr != nil {
		return
	}
	if !raft.IsEmptyHardState(hd) {
		fmt.Printf("Found hardstate: %+v\n", sp)
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
	fmt.Printf("Found %d entries\n", len(es))
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
		fmt.Printf("RESTARTING\n")
		n.raft = raft.RestartNode(n.cfg)

	} else {
		if groups().HasPeer(n.gid) {
			n.joinPeers()
			n.raft = raft.StartNode(n.cfg, nil)

		} else {
			peers := []raft.Peer{{ID: n.id}}
			n.raft = raft.StartNode(n.cfg, peers)
			// Trigger election, so this node can become the leader of this single-node cluster.
			n.canCampaign = true
		}
	}
	go n.processCommitCh()
	go n.Run()
	// TODO: Find a better way to snapshot, so we don't lose the membership
	// state information, which isn't persisted.
	// go n.snapshotPeriodically()
	go n.batchAndSendMessages()
}

func (n *node) AmLeader() bool {
	if n == nil || n.raft == nil {
		return false
	}
	return n.raft.Status().Lead == n.raft.Status().ID
}

func (w *grpcWorker) applyMessage(ctx context.Context, msg raftpb.Message) error {
	var rc task.RaftContext
	x.Check(rc.Unmarshal(msg.Context))
	node := groups().Node(rc.Group)
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

func (w *grpcWorker) RaftMessage(ctx context.Context, query *Payload) (*Payload, error) {
	if ctx.Err() != nil {
		return &Payload{}, ctx.Err()
	}

	for idx := 0; idx < len(query.Data); {
		sz := int(binary.LittleEndian.Uint32(query.Data[idx : idx+4]))
		idx += 4
		msg := raftpb.Message{}
		if idx+sz-1 > len(query.Data) {
			return &Payload{}, x.Errorf(
				"Invalid query. Size specified: %v. Size of array: %v\n", sz, len(query.Data))
		}
		if err := msg.Unmarshal(query.Data[idx : idx+sz]); err != nil {
			x.Check(err)
		}
		if msg.Type != raftpb.MsgHeartbeat && msg.Type != raftpb.MsgHeartbeatResp {
			fmt.Printf("RECEIVED: %v %v-->%v\n", msg.Type, msg.From, msg.To)
		}
		if err := w.applyMessage(ctx, msg); err != nil {
			return &Payload{}, err
		}
		idx += sz
	}
	// fmt.Printf("Got %d messages\n", count)
	return &Payload{}, nil
}

func (w *grpcWorker) JoinCluster(ctx context.Context, rc *task.RaftContext) (*Payload, error) {
	if ctx.Err() != nil {
		return &Payload{}, ctx.Err()
	}

	node := groups().Node(rc.Group)
	if node == nil {
		return &Payload{}, nil
	}
	node.Connect(rc.Id, rc.Addr)

	c := make(chan error, 1)
	go func() { c <- node.AddToCluster(ctx, rc.Id) }()

	select {
	case <-ctx.Done():
		return &Payload{}, ctx.Err()
	case err := <-c:
		return &Payload{}, err
	}
}
