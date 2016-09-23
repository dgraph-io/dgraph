package draft

import (
	"fmt"
	"sync"
	"time"

	"github.com/coreos/etcd/raft"
	"github.com/coreos/etcd/raft/raftpb"
	"github.com/dgraph-io/dgraph/x"
)

type node struct {
	cfg   *raft.Config
	data  map[string]string
	done  <-chan struct{}
	id    uint64
	raft  raft.Node
	store *raft.MemoryStorage
}

func (n *node) send(msgs []raftpb.Message) {
	fmt.Printf("Sending %d messages", len(msgs))
	// TODO: Implement this.
	return
}

func (n *node) process(e raftpb.Entry) error {
	// TODO: Implement this.
	return nil
}

func (n *node) saveToStorage(s raftpb.Snapshot, h raftpb.HardState,
	es []raftpb.Entry) {
	// TODO: Implement this.
	return
}

func (n *node) Run() {
	for {
		select {
		case <-time.Tick(time.Second):
			n.raft.Tick()

		case rd := <-n.raft.Ready():
			n.saveToStorage(rd.Snapshot, rd.HardState, rd.Entries)
			n.send(rd.Messages)
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

func newNode(id uint64) *node {
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
		data: make(map[string]string),
	}
	n.raft = raft.StartNode(n.cfg, []raft.Peer{})
	return n
}

var thisNode *node
var once sync.Once

func GetNode(id uint64) *node {
	once.Do(func() {
		thisNode = newNode(id)
	})
	return thisNode
}
