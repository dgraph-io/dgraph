package worker

import (
	"encoding/binary"
	"fmt"
	"log"
	"math"
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
	x.Assertf(!has, "Same proposal is being stored again.")
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
	x.Assertf(len(addr) > 0, "Unable to find conn pool for peer: %d", pid)

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

func (n *node) ProposeAndWait(ctx context.Context, msg uint16, data []byte) error {
	var h header
	h.proposalId = rand.Uint32()
	h.msgId = msg
	hdata := h.Encode()

	proposalData := make([]byte, len(data)+len(hdata))
	x.Assert(copy(proposalData, hdata) == len(hdata))
	x.Assert(copy(proposalData[len(hdata):], data) == len(data))

	che := make(chan error, 1)
	n.props.Store(h.proposalId, che)

	err := n.raft.Propose(ctx, proposalData)
	if err != nil {
		return x.Wrapf(err, "While proposing")
	}

	// Wait for the proposal to be committed.
	x.Trace(ctx, "Waiting for the proposal to be applied.")
	select {
	case err = <-che:
		x.TraceError(ctx, err)
		fmt.Printf("DEBUG. Proposeandwait replied with: %v", err)
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (n *node) send(m raftpb.Message) {
	x.Assertf(n.id != m.To, "Seding message to itself")

	addr := n.peers.Get(m.To)
	x.Assertf(len(addr) > 0, "Don't have address for peer: %d", m.To)

	pool := pools().get(addr)
	conn, err := pool.Get()
	x.Check(err)
	defer pool.Put(conn)

	c := NewWorkerClient(conn)
	m.Context = n.raftContext
	data, err := m.Marshal()
	x.Checkf(err, "Unable to marshal: %+v", m)
	p := &Payload{Data: data}
	_, err = c.RaftMessage(context.TODO(), p)
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
	rc := task.GetRootAsRaftContext(n.raftContext, 0)
	x.Assert(rc.Group() == math.MaxUint32)

	mm := task.GetRootAsMembership(e.Data[h.Length():], 0)
	fmt.Printf("group: %v Addr: %q leader: %v dead: %v\n",
		mm.Group(), mm.Addr(), mm.Leader(), mm.Amdead())
	groups().UpdateServer(mm)
	return nil
}

func (n *node) process(e raftpb.Entry) error {
	if e.Data == nil {
		return nil
	}
	fmt.Printf("Entry type to process: %v\n", e.Type.String())

	if e.Type == raftpb.EntryConfChange {
		fmt.Printf("Configuration change\n")
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
		fmt.Printf("saveToStorage snapshot: %v\n", s.String())
		le, err := n.store.LastIndex()
		if err != nil {
			log.Fatalf("While retrieving last index: %v\n", err)
		}
		te, err := n.store.Term(le)
		if err != nil {
			log.Fatalf("While retrieving term: %v\n", err)
		}
		fmt.Printf("[Node: %d] Term: %v for le: %v\n", n.id, te, le)
		if s.Metadata.Index <= le {
			fmt.Printf("%d node ignoring snapshot. Last index: %v\n", n.id, le)
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
		fmt.Printf("Don't know who the leader is")
		return
	}
	addr := n.peers.Get(lead)
	x.Assertf(addr != "", "Should have the leader address: %v", lead)
	pool := pools().get(addr)
	x.Assertf(pool != nil, "Leader: %d pool should not be nil", lead)

	fmt.Printf("Getting snapshot from leader: %v", lead)
	_, err := ws.PopulateShard(context.TODO(), pool, 0)
	x.Checkf(err, "processSnapshot")
	fmt.Printf("DONE with snapshot")
}

func (n *node) Run() {
	fmt.Println("Run")
	for {
		select {
		case <-time.Tick(time.Second):
			n.raft.Tick()

		case rd := <-n.raft.Ready():
			x.Check(n.wal.Store(n.gid, rd.Snapshot, rd.HardState, rd.Entries))
			n.saveToStorage(rd.Snapshot, rd.HardState, rd.Entries)
			for _, msg := range rd.Messages {
				n.send(msg)
			}
			if !raft.IsEmptySnap(rd.Snapshot) {
				fmt.Printf("Got snapshot: %q\n", rd.Snapshot.Data)
				n.processSnapshot(rd.Snapshot)
			}
			for _, entry := range rd.CommittedEntries {
				x.Check(n.process(entry))
			}
			n.raft.Advance()

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
	for {
		select {
		case t := <-time.Tick(time.Minute):
			fmt.Printf("Snapshot Periodically: %v", t)

			le, err := n.store.LastIndex()
			x.Checkf(err, "Unable to retrieve last index")

			existing, err := n.store.Snapshot()
			x.Checkf(err, "Unable to get existing snapshot")

			si := existing.Metadata.Index
			fmt.Printf("le, si: %v %v\n", le, si)
			if le <= si {
				fmt.Printf("le, si: %v %v. No snapshot\n", le, si)
				continue
			}

			msg := fmt.Sprintf("Snapshot from %v", strconv.FormatUint(n.id, 10))
			_, err = n.store.CreateSnapshot(le, nil, []byte(msg))
			x.Checkf(err, "While creating snapshot")

			x.Checkf(n.store.Compact(le), "While compacting snapshot")
			fmt.Println("Snapshot DONE =================")

		case <-n.done:
			return
		}
	}
}

func parsePeer(peer string) (uint64, string) {
	kv := strings.SplitN(peer, ":", 2)
	x.Assertf(len(kv) == 2, "Invalid peer format: %v", peer)
	pid, err := strconv.ParseUint(kv[0], 10, 64)
	x.Checkf(err, "Invalid peer id: %v", kv[0])
	// TODO: Validate the url kv[1]
	return pid, kv[1]
}

func (n *node) joinPeers(any string, s *State) {
	// Tell one of the peers to join.
	pid, paddr := parsePeer(any)
	n.Connect(pid, paddr)

	addr := n.peers.Get(pid)
	pool := pools().get(addr)
	x.Assertf(pool != nil, "Unable to find addr for peer: %d", pid)

	// TODO: Ask for the leader, before running PopulateShard.
	// Bring the instance up to speed first.
	_, err := s.PopulateShard(context.TODO(), pool, 0)
	x.Checkf(err, "Error while populating shard")

	fmt.Printf("TELLING PEER TO ADD ME: %v\n", any)
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
	fmt.Printf("NEW NODE ID: %v\n", id)

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
			ElectionTick:    3,
			HeartbeatTick:   1,
			Storage:         store,
			MaxSizePerMsg:   4096,
			MaxInflightMsgs: 256,
		},
		peers:       peers,
		props:       props,
		raftContext: createRaftContext(id, gid, myAddr),
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
	if !raft.IsEmptySnap(sp) {
		fmt.Printf("Found Snapshot: %+v\n", sp)
		restart = true
		if rerr = n.store.ApplySnapshot(sp); rerr != nil {
			return
		}
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

	var term, idx uint64
	if !raft.IsEmptySnap(sp) {
		term = sp.Metadata.Term
		idx = sp.Metadata.Index
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

func (n *node) InitAndStartNode(wal *raftwal.Wal, peer string) {
	restart, err := n.initFromWal(wal)
	x.Check(err)

	if restart {
		n.raft = raft.RestartNode(n.cfg)
	} else {
		peers := []raft.Peer{{ID: n.id}}
		n.raft = raft.StartNode(n.cfg, peers)
		if len(peer) > 0 {
			n.joinPeers(peer, ws)
		}
	}
	go n.Run()
	go n.snapshotPeriodically()
	go n.Inform()
}

func (n *node) doInform(rc *task.RaftContext) {
	s, found := groups().Server(rc.Id(), rc.Group())
	if found && s.Addr == string(rc.Addr()) && s.Leader == n.AmLeader() {
		return
	}

	var l byte
	if n.AmLeader() {
		l = byte(1)
	}

	b := flatbuffers.NewBuilder(0)
	so := b.CreateString(string(rc.Addr()))
	task.MembershipStart(b)
	task.MembershipAddId(b, rc.Id())
	task.MembershipAddGroup(b, rc.Group())
	task.MembershipAddAddr(b, so)
	task.MembershipAddLeader(b, l)
	uo := task.MembershipEnd(b)
	b.Finish(uo)
	data := b.Bytes[b.Head():]

	common := groups().Node(math.MaxUint32)
	if common == nil {
		return
	}
	x.Checkf(common.ProposeAndWait(context.TODO(), membershipMsg, data),
		"Expected acceptance.")
}

func (n *node) Inform() {
	rc := task.GetRootAsRaftContext(n.raftContext, 0)
	if rc.Group() == math.MaxUint32 {
		return
	}

	select {
	case <-time.Tick(30 * time.Second):
		n.doInform(rc)
	case <-n.done:
		return
	}
}

func (n *node) AmLeader() bool {
	return n.raft.Status().Lead == n.raft.Status().ID
}

func (w *grpcWorker) RaftMessage(ctx context.Context, query *Payload) (*Payload, error) {
	if ctx.Err() != nil {
		return &Payload{}, ctx.Err()
	}

	msg := raftpb.Message{}
	if err := msg.Unmarshal(query.Data); err != nil {
		return &Payload{}, err
	}

	rc := task.GetRootAsRaftContext(msg.Context, 0)
	node := groups().Node(rc.Group())
	node.Connect(msg.From, string(rc.Addr()))

	c := make(chan error, 1)
	go func() { c <- node.Step(ctx, msg) }()

	select {
	case <-ctx.Done():
		return &Payload{}, ctx.Err()
	case err := <-c:
		return &Payload{}, err
	}
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
