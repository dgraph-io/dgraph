/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package conn

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.etcd.io/etcd/raft/v3"
	"go.etcd.io/etcd/raft/v3/raftpb"

	"github.com/hypermodeinc/dgraph/v25/protos/pb"
	"github.com/hypermodeinc/dgraph/v25/raftwal"
)

func (n *Node) run(wg *sync.WaitGroup) {
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			n.Raft().Tick()
		case rd := <-n.Raft().Ready():
			n.SaveToStorage(&rd.HardState, rd.Entries, &rd.Snapshot)
			for _, entry := range rd.CommittedEntries {
				if entry.Type == raftpb.EntryConfChange {
					var cc raftpb.ConfChange
					if err := cc.Unmarshal(entry.Data); err != nil {
						fmt.Printf("error in unmarshalling: %v\n", err)
					}
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
	dir := t.TempDir()
	store := raftwal.Init(dir)

	rc := &pb.RaftContext{Id: 1}
	n := NewNode(rc, store, nil)

	peers := []raft.Peer{{ID: n.Id}}
	n.SetRaft(raft.StartNode(n.Cfg, peers))

	loop := 5
	var wg sync.WaitGroup
	wg.Add(loop)
	go n.run(&wg)

	for i := range loop {
		data := []byte(fmt.Sprintf("hey-%d", i))
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		require.NoError(t, n.Raft().Propose(ctx, data))
	}
	wg.Wait()
}
