package worker

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/coreos/etcd/raft"
	"github.com/coreos/etcd/raft/raftpb"
	"github.com/dgraph-io/dgraph/x"
)

type node struct {
	cfg       *raft.Config
	data      map[string]string
	done      <-chan struct{}
	id        uint64
	raft      raft.Node
	store     *raft.MemoryStorage
	peers     map[uint64]*Pool
	localAddr string
}

// TODO: Make this thread safe.
func (n *node) Connect(pid uint64, addr string) {
	if _, has := n.peers[pid]; has {
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
	_ = pool.Put(conn)
	n.peers[pid] = pool
	fmt.Printf("CONNECTED TO %d %v\n", pid, addr)
	return
}

func (n *node) AddToCluster(pid uint64) {
	n.raft.ProposeConfChange(context.TODO(), raftpb.ConfChange{
		ID:      pid,
		Type:    raftpb.ConfChangeAddNode,
		NodeID:  pid,
		Context: []byte(strconv.FormatUint(pid, 10) + ":" + n.peers[pid].Addr),
	})
}

func (n *node) send(m raftpb.Message) {
	pool, has := n.peers[m.To]
	x.Assertf(has, "Don't have address for peer: %d", m.To)

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
	// TODO: Implement this.
	fmt.Printf("process: %+v\n", e)
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
		parts := bytes.SplitN(e.Data, []byte(":"), 2)
		k := string(parts[0])
		v := string(parts[1])
		n.data[k] = v
		fmt.Printf(" Key: %v Val: %v\n", k, v)
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
	pool := n.peers[lead]
	fmt.Printf("Getting snapshot from leader: %v", lead)
	x.Checkf(ws.PopulateShard(context.TODO(), pool, 0), "processSnapshot")
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

func (n *node) Campaign(ctx context.Context) {
	if len(n.peers) > 0 {
		fmt.Printf("CAMPAIGN\n")
		x.Check(n.raft.Campaign(ctx))
	}
}

func (n *node) Step(ctx context.Context, msg raftpb.Message) error {
	return n.raft.Step(ctx, msg)
}

func (n *node) SnapshotPeriodically() {
	for t := range time.Tick(10 * time.Second) {
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
	return pid, kv[1]
}

func (n *node) JoinCluster(any string) {
	// Tell one of the peers to join.

	pid, paddr := parsePeer(any)
	n.Connect(pid, paddr)

	fmt.Printf("TELLING PEER TO ADD ME: %v\n", any)
	pool := n.peers[pid]
	query := &Payload{}
	query.Data = []byte(strconv.FormatUint(n.id, 10) + ":" + n.localAddr)
	conn, err := pool.Get()
	x.Check(err)
	c := NewWorkerClient(conn)
	fmt.Println("Calling JoinCluster")
	_, err = c.JoinCluster(context.Background(), query)
	x.Checkf(err, "Error while joining cluster")
	fmt.Printf("Done with JoinCluster call\n")

	// No need to campaign.
	// go n.Campaign(context.Background())
}

func newNode(id uint64, my string) *node {
	fmt.Printf("NEW NODE ID: %v\n", id)
	fmt.Println("local address", net.Addr(nil))

	store := raft.NewMemoryStorage()
	n := &node{
		id:    id,
		store: store,
		cfg: &raft.Config{
			ID:              uint64(id),
			ElectionTick:    3,
			HeartbeatTick:   1,
			Storage:         store,
			MaxSizePerMsg:   4096,
			MaxInflightMsgs: 256,
		},
		data:      make(map[string]string),
		peers:     make(map[uint64]*Pool),
		localAddr: my,
	}
	return n
}

func (n *node) StartNode(cluster string) {
	var raftPeers []raft.Peer
	if len(cluster) > 0 {
		raftPeers = append(raftPeers, raft.Peer{ID: n.id})

		for _, p := range strings.Split(cluster, ",") {
			pid, paddr := parsePeer(p)
			raftPeers = append(raftPeers, raft.Peer{ID: pid})
			n.Connect(pid, paddr)
		}
	}

	n.raft = raft.StartNode(n.cfg, raftPeers)
	go n.Run()
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
