package worker

import (
	"context"
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
	"github.com/dgraph-io/dgraph/x"
)

const (
	mutationMsg = 1
)

type peerPool struct {
	sync.RWMutex
	peers map[uint64]*Pool
}

func (p *peerPool) Get(id uint64) *Pool {
	p.RLock()
	defer p.RUnlock()
	return p.peers[id]
}

func (p *peerPool) Set(id uint64, pool *Pool) {
	p.Lock()
	defer p.Unlock()
	p.peers[id] = pool
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
	cfg       *raft.Config
	ctx       context.Context
	done      chan struct{}
	id        uint64
	localAddr string
	peers     peerPool
	props     proposals
	raft      raft.Node
	store     *raft.MemoryStorage
}

func (n *node) Connect(pid uint64, addr string) {
	if pid == n.id {
		return
	}
	if pool := n.peers.Get(pid); pool != nil {
		return
	}

	fmt.Printf("connect addr: %v\n", addr)
	pool := NewPool(addr, 5)
	query := new(Payload)
	query.Data = []byte("hello")

	conn, err := pool.Get()
	if err != nil {
		log.Fatalf("Unable to connect: %v", err)
	}

	c := NewWorkerClient(conn)
	_, err = c.Hello(context.Background(), query)
	if err != nil {
		log.Fatalf("Unable to connect: %v", err)
	}
	x.Check(pool.Put(conn))

	n.peers.Set(pid, pool)
	fmt.Printf("CONNECTED TO %d %v\n", pid, addr)
	return
}

func (n *node) AddToCluster(pid uint64) {
	pool := n.peers.Get(pid)
	x.Assertf(pool != nil, "Unable to find conn pool for peer: %d", pid)

	n.raft.ProposeConfChange(context.TODO(), raftpb.ConfChange{
		ID:      pid,
		Type:    raftpb.ConfChangeAddNode,
		NodeID:  pid,
		Context: []byte(strconv.FormatUint(pid, 10) + ":" + pool.Addr),
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

	pool := n.peers.Get(m.To)
	x.Assertf(pool != nil, "Don't have address for peer: %d", m.To)

	conn, err := pool.Get()
	x.Check(err)

	c := NewWorkerClient(conn)
	m.Context = []byte(n.localAddr)

	data, err := m.Marshal()
	x.Checkf(err, "Unable to marshal: %+v", m)
	p := &Payload{Data: data}
	_, err = c.RaftMessage(context.TODO(), p)
}

func (n *node) process(e raftpb.Entry) error {
	if e.Data == nil {
		return nil
	}

	if e.Type == raftpb.EntryConfChange {
		fmt.Printf("Configuration change\n")
		var cc raftpb.ConfChange
		cc.Unmarshal(e.Data)

		if len(cc.Context) > 0 {
			pid, paddr := parsePeer(string(cc.Context))
			n.Connect(pid, paddr)
		}

		n.raft.ApplyConfChange(cc)
		return nil
	}

	if e.Type == raftpb.EntryNormal {
		var h header
		h.Decode(e.Data[0:h.Length()])
		x.Assertf(h.msgId == 1, "We only handle mutations for now.")

		m := new(x.Mutations)
		// Ensure that this can be decoded.
		if err := m.Decode(e.Data[h.Length():]); err != nil {
			x.TraceError(n.ctx, err)
			n.props.Done(h.proposalId, err)
			return err
		}
		if err := mutate(n.ctx, m); err != nil {
			x.TraceError(n.ctx, err)
			n.props.Done(h.proposalId, err)
			return err
		}
		n.props.Done(h.proposalId, nil)
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
	pool := n.peers.Get(lead)
	x.Assertf(pool != nil, "Leader: %d pool should not be nil", lead)
	fmt.Printf("Getting snapshot from leader: %v", lead)
	_, err := ws.PopulateShard(context.TODO(), pool, 0)
	x.Checkf(err, "processSnapshot")
	fmt.Printf("DONE with snapshot ============================")
}

func (n *node) Run() {
	fmt.Println("Run")
	for {
		select {
		case <-time.Tick(time.Second):
			n.raft.Tick()

		case rd := <-n.raft.Ready():
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
	n.done <- struct{}{}
}

func (n *node) Step(ctx context.Context, msg raftpb.Message) error {
	return n.raft.Step(ctx, msg)
}

func (n *node) SnapshotPeriodically() {
	for t := range time.Tick(time.Minute) {
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

func (n *node) JoinCluster(any string, s *State) {
	// Tell one of the peers to join.
	pid, paddr := parsePeer(any)
	n.Connect(pid, paddr)

	pool := n.peers.Get(pid)
	x.Assertf(pool != nil, "Unable to find pool for peer: %d", pid)

	// TODO: Ask for the leader, before running PopulateShard.
	// Bring the instance up to speed first.
	_, err := s.PopulateShard(context.TODO(), pool, 0)
	x.Checkf(err, "Error while populating shard")

	fmt.Printf("TELLING PEER TO ADD ME: %v\n", any)
	query := &Payload{}
	query.Data = []byte(strconv.FormatUint(n.id, 10) + ":" + n.localAddr)
	conn, err := pool.Get()
	x.Check(err)
	c := NewWorkerClient(conn)
	fmt.Println("Calling JoinCluster")
	_, err = c.JoinCluster(context.Background(), query)
	x.Checkf(err, "Error while joining cluster")
	fmt.Printf("Done with JoinCluster call\n")
}

func newNode(id uint64, my string) *node {
	fmt.Printf("NEW NODE ID: %v\n", id)

	peers := peerPool{
		peers: make(map[uint64]*Pool),
	}
	props := proposals{
		ids: make(map[uint32]chan error),
	}

	store := raft.NewMemoryStorage()
	n := &node{
		ctx:   context.TODO(),
		done:  make(chan struct{}),
		id:    id,
		store: store,
		cfg: &raft.Config{
			ID:              id,
			ElectionTick:    3,
			HeartbeatTick:   1,
			Storage:         store,
			MaxSizePerMsg:   4096,
			MaxInflightMsgs: 256,
		},
		peers:     peers,
		props:     props,
		localAddr: my,
	}
	return n
}

func (n *node) StartNode(cluster string) {
	var peers []raft.Peer
	if len(cluster) > 0 {
		for _, p := range strings.Split(cluster, ",") {
			pid, paddr := parsePeer(p)
			peers = append(peers, raft.Peer{ID: pid})
			n.Connect(pid, paddr)
		}
	}

	n.raft = raft.StartNode(n.cfg, peers)
	go n.Run()
}

func (n *node) AmLeader() bool {
	return n.raft.Status().Lead == n.raft.Status().ID
}

var thisNode *node
var once sync.Once

func InitNode(id uint64, my string) {
	once.Do(func() {
		thisNode = newNode(id, my)
	})
}

func GetNode() *node {
	return thisNode
}
