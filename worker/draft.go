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
}

func (n *node) send(m raftpb.Message) {
	fmt.Printf("Sending message: %+v\n", m)
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
	log.Printf("Error: %+v", err)
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
		fmt.Printf("%d node Term for le: %v is %v\n", n.id, le, te)
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

func (n *node) Run() {
	fmt.Println("Run")
	for {
		select {
		case <-time.Tick(time.Second):
			n.raft.Tick()

		case rd := <-n.raft.Ready():
			n.saveToStorage(rd.Snapshot, rd.HardState, rd.Entries)
			for _, msg := range rd.Messages {
				go n.send(msg)
			}
			if !raft.IsEmptySnap(rd.Snapshot) {
				fmt.Printf("Got snapshot: %q\n", rd.Snapshot.Data)
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
	return
	time.Sleep(3 * time.Second)
	if len(n.peers) > 0 {
		fmt.Printf("CAMPAIGN\n")
		x.Check(n.raft.Campaign(ctx))
	}
}

func (n *node) Step(ctx context.Context, msg raftpb.Message) error {
	return n.raft.Step(ctx, msg)
}

func newNode(id uint64, my, pn string) *node {
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

	var raftPeers []raft.Peer
	raftPeers = append(raftPeers, raft.Peer{ID: id})
	if len(pn) > 0 {
		for _, p := range strings.Split(pn, ",") {
			kv := strings.SplitN(p, ":", 2)
			x.Assertf(len(kv) == 2, "Invalid peer format: %v", p)
			pid, err := strconv.ParseUint(kv[0], 10, 64)
			x.Checkf(err, "Invalid peer id: %v", kv[0])
			raftPeers = append(raftPeers, raft.Peer{ID: pid})

			n.Connect(pid, kv[1])
		}
	}
	fmt.Printf("raftpeers: %+v\n", raftPeers)

	n.raft = raft.StartNode(n.cfg, raftPeers)
	return n
}

var thisNode *node
var once sync.Once

func InitNode(id uint64, my, pn string) {
	once.Do(func() {
		thisNode = newNode(id, my, pn)
	})
}

func GetNode() *node {
	return thisNode
}
