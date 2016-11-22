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
	flatbuffers "github.com/google/flatbuffers/go"
	"golang.org/x/net/context"

	"github.com/dgraph-io/dgraph/raftwal"
	"github.com/dgraph-io/dgraph/task"
	"github.com/dgraph-io/dgraph/x"
)

const (
	mutationMsg   = 1
	membershipMsg = 2
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
	cfg         *raft.Config
	ctx         context.Context
	done        chan struct{}
	id          uint64
	gid         uint32
	peers       peerPool
	props       proposals
	raft        raft.Node
	store       *raft.MemoryStorage
	raftContext []byte
	wal         *raftwal.Wal
	messages    chan sendmsg
	canCampaign bool
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

func (n *node) AddToCluster(pid uint64) error {
	addr := n.peers.Get(pid)
	x.AssertTruef(len(addr) > 0, "Unable to find conn pool for peer: %d", pid)

	rc := task.GetRootAsRaftContext(n.raftContext, 0)
	return n.raft.ProposeConfChange(context.TODO(), raftpb.ConfChange{
		ID:      pid,
		Type:    raftpb.ConfChangeAddNode,
		NodeID:  pid,
		Context: createRaftContext(pid, rc.Group(), addr),
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

func (n *node) ProposeAndWait(ctx context.Context, msg uint16, m *task.Mutations) error {
	var h header
	h.proposalId = rand.Uint32()
	h.msgId = msg
	hdata := h.Encode()

	proposalData := make([]byte, len(data)+len(hdata))
	x.AssertTrue(copy(proposalData, hdata) == len(hdata))
	x.AssertTrue(copy(proposalData[len(hdata):], data) == len(data))

	che := make(chan error, 1)
	n.props.Store(h.proposalId, che)

	err := n.raft.Propose(ctx, proposalData)
	if err != nil {
		return x.Wrapf(err, "While proposing")
	}

	// Wait for the proposal to be committed.
	x.Trace(ctx, "Waiting for the proposal: %d to be applied.", msg)
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

func (n *node) processMutation(e raftpb.Entry, h header) error {
	m := new(x.Mutations)
	// Ensure that this can be decoded.
	if err := m.Decode(e.Data[h.Length():]); err != nil {
		x.TraceError(n.ctx, err)
		return err
	}
	if err := mutate(n.ctx, m); err != nil {
		x.TraceError(n.ctx, err)
		return err
	}
	return nil
}

func (n *node) processMembership(e raftpb.Entry, h header) error {
	x.AssertTrue(n.gid == 0)

	mm := task.GetRootAsMembership(e.Data[h.Length():], 0)
	fmt.Printf("group: %v Addr: %q leader: %v dead: %v\n",
		mm.Group(), mm.Addr(), mm.Leader(), mm.Amdead())
	groups().applyMembershipUpdate(e.Index, mm)
	return nil
}

func (n *node) process(e raftpb.Entry) error {
	if e.Data == nil {
		return nil
	}
	fmt.Printf("Entry type to process: [%d, %d] Type: %v\n", e.Term, e.Index, e.Type)

	if e.Type == raftpb.EntryConfChange {
		var cc raftpb.ConfChange
		cc.Unmarshal(e.Data)

		if len(cc.Context) > 0 {
			rc := task.GetRootAsRaftContext(cc.Context, 0)
			n.Connect(rc.Id(), string(rc.Addr()))
		}

		n.raft.ApplyConfChange(cc)
		return nil
	}

	if e.Type == raftpb.EntryNormal {
		var h header
		h.Decode(e.Data[0:h.Length()])

		var err error
		if h.msgId == mutationMsg {
			err = n.processMutation(e, h)
		} else if h.msgId == membershipMsg {
			err = n.processMembership(e, h)
		}
		n.props.Done(h.proposalId, err)
	}

	return nil
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
	fr := true
	ticker := time.NewTicker(time.Second)
	for {
		select {
		case <-ticker.C:
			n.raft.Tick()

		case rd := <-n.raft.Ready():
			x.Check(n.wal.Store(n.gid, rd.Snapshot, rd.HardState, rd.Entries))
			n.saveToStorage(rd.Snapshot, rd.HardState, rd.Entries)
			for _, msg := range rd.Messages {
				// TODO: Do some optimizations here to drop messages.
				msg.Context = n.raftContext
				n.send(msg)
			}

			if !raft.IsEmptySnap(rd.Snapshot) {
				n.processSnapshot(rd.Snapshot)
			}
			for _, entry := range rd.CommittedEntries {
				x.Check(n.process(entry))
			}

			n.raft.Advance()
			if fr && n.canCampaign {
				go n.raft.Campaign(context.TODO())
				fr = false
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
	pid, paddr := groups().Leader(n.gid)
	n.Connect(pid, paddr)
	fmt.Printf("Connected with: %v\n", paddr)

	addr := n.peers.Get(pid)
	pool := pools().get(addr)
	x.AssertTruef(pool != nil, "Unable to find addr for peer: %d", pid)

	// TODO: Ask for the leader, before running populateShard.
	// Bring the instance up to speed first.
	_, err := populateShard(context.TODO(), pool, 0)
	x.Checkf(err, "Error while populating shard")

	query := &Payload{}
	query.Data = n.raftContext
	conn, err := pool.Get()
	x.Check(err)
	c := NewWorkerClient(conn)
	fmt.Println("Calling JoinCluster")
	_, err = c.JoinCluster(context.Background(), query)
	x.Checkf(err, "Error while joining cluster")
	fmt.Printf("Done with JoinCluster call\n")
}

func createRaftContext(id uint64, gid uint32, addr string) []byte {
	// Create a flatbuffer for storing group id and local address.
	// This needs to send along with every raft message.
	b := flatbuffers.NewBuilder(0)
	so := b.CreateString(addr)
	task.RaftContextStart(b)
	task.RaftContextAddId(b, id)
	task.RaftContextAddGroup(b, gid)
	task.RaftContextAddAddr(b, so)
	uo := task.RaftContextEnd(b)
	b.Finish(uo)
	return b.Bytes[b.Head():]
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
	n := &node{
		ctx:   context.TODO(),
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
		peers:       peers,
		props:       props,
		raftContext: createRaftContext(id, gid, myAddr),
		messages:    make(chan sendmsg, 1000),
	}
	return n
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
	go n.Run()
	// TODO: Find a better way to snapshot, so we don't lose the membership
	// state information, which isn't persisted.
	// go n.snapshotPeriodically()
	go n.batchAndSendMessages()
}

func (n *node) AmLeader() bool {
	if n == nil {
		return false
	}
	return n.raft.Status().Lead == n.raft.Status().ID
}

func (w *grpcWorker) applyMessage(ctx context.Context, msg raftpb.Message) error {
	rc := task.GetRootAsRaftContext(msg.Context, 0)
	node := groups().Node(rc.Group())
	node.Connect(msg.From, string(rc.Addr()))

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

func (w *grpcWorker) JoinCluster(ctx context.Context, query *Payload) (*Payload, error) {
	if ctx.Err() != nil {
		return &Payload{}, ctx.Err()
	}

	if len(query.Data) == 0 {
		return &Payload{}, x.Errorf("JoinCluster: No data provided")
	}

	rc := task.GetRootAsRaftContext(query.Data, 0)
	node := groups().Node(rc.Group())
	if node == nil {
		return &Payload{}, nil
	}
	node.Connect(rc.Id(), string(rc.Addr()))

	c := make(chan error, 1)
	go func() { c <- node.AddToCluster(rc.Id()) }()

	select {
	case <-ctx.Done():
		return &Payload{}, ctx.Err()
	case err := <-c:
		return &Payload{}, err
	}
}
