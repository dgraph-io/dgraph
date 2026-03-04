/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
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

	"github.com/dgraph-io/dgraph/v25/protos/pb"
	"github.com/dgraph-io/dgraph/v25/raftwal"
)

func (n *Node) run(wg *sync.WaitGroup) {
	ticker := time.Tick(20 * time.Millisecond)

	for {
		select {
		case <-ticker:
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

func TestConfChangeStoreAndCleanup(t *testing.T) {
	dir := t.TempDir()
	store := raftwal.Init(dir)
	rc := &pb.RaftContext{Id: 1}
	n := NewNode(rc, store, nil)

	// Store a conf change entry.
	ch := make(chan error, 1)
	id := n.storeConfChange(ch)

	// Verify it was stored.
	n.RLock()
	_, exists := n.confChanges[id]
	n.RUnlock()
	require.True(t, exists, "conf change should be stored")

	// Simulate cleanup (as done in proposeConfChange on timeout).
	n.Lock()
	delete(n.confChanges, id)
	n.Unlock()

	// Verify it was cleaned up.
	n.RLock()
	_, exists = n.confChanges[id]
	n.RUnlock()
	require.False(t, exists, "conf change should be cleaned up")
}

func TestDoneConfChangeTolerantOfMissing(t *testing.T) {
	dir := t.TempDir()
	store := raftwal.Init(dir)
	rc := &pb.RaftContext{Id: 1}
	n := NewNode(rc, store, nil)

	// DoneConfChange with a non-existent ID should not panic.
	n.DoneConfChange(12345, nil)

	// Store and then clean up, then DoneConfChange should be tolerant.
	ch := make(chan error, 1)
	id := n.storeConfChange(ch)
	n.Lock()
	delete(n.confChanges, id)
	n.Unlock()
	n.DoneConfChange(id, nil) // Should not panic.
}

func TestConfChangeStoreUniqueness(t *testing.T) {
	dir := t.TempDir()
	store := raftwal.Init(dir)
	rc := &pb.RaftContext{Id: 1}
	n := NewNode(rc, store, nil)

	ids := make(map[uint64]bool)
	for i := 0; i < 100; i++ {
		ch := make(chan error, 1)
		id := n.storeConfChange(ch)
		require.False(t, ids[id], "storeConfChange should produce unique IDs")
		ids[id] = true
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
