package conn

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/coreos/etcd/raft"
	"github.com/coreos/etcd/raft/raftpb"
	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/dgraph/protos/intern"
	"github.com/dgraph-io/dgraph/raftwal"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/context"
)

func openBadger(dir string) (*badger.DB, error) {
	opt := badger.DefaultOptions
	opt.Dir = dir
	opt.ValueDir = dir

	return badger.Open(opt)
}

func (n *Node) run(wg *sync.WaitGroup) {
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			n.Raft().Tick()
		case rd := <-n.Raft().Ready():
			n.SaveToStorage(rd.HardState, rd.Entries, rd.Snapshot)
			for _, entry := range rd.CommittedEntries {
				if entry.Type == raftpb.EntryConfChange {
					var cc raftpb.ConfChange
					cc.Unmarshal(entry.Data)
					n.Raft().ApplyConfChange(cc)
				} else if entry.Type == raftpb.EntryNormal {
					if bytes.HasPrefix(entry.Data, []byte("hey")) {
						wg.Done()
					}
				}
			}
			n.Raft().Advance()
		}
	}
}

func TestProposal(t *testing.T) {
	dir, err := ioutil.TempDir("", "badger")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	db, err := openBadger(dir)
	require.NoError(t, err)
	store := raftwal.Init(db, 0, 0)

	rc := &intern.RaftContext{Id: 1}
	n := NewNode(rc, store)

	peers := []raft.Peer{{ID: n.Id}}
	n.SetRaft(raft.StartNode(n.Cfg, peers))

	loop := 5
	var wg sync.WaitGroup
	wg.Add(loop)
	go n.run(&wg)

	for i := 0; i < loop; i++ {
		data := []byte(fmt.Sprintf("hey-%d", i))
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		require.NoError(t, n.Raft().Propose(ctx, data))
	}
	wg.Wait()
}
