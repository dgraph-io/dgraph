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

func TestProposal(t *testing.T) {
	dir := t.TempDir()
	store := raftwal.Init(dir)

	rc := &pb.RaftContext{Id: 1}
	n := NewNode(rc, store, nil, 0)

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

func TestNormalizeElectionTick(t *testing.T) {
	tests := []struct {
		name         string
		electionTick int
		wantTick     int
		wantWarning  string
	}{
		{
			name:         "zero defaults silently",
			electionTick: 0,
			wantTick:     20,
			wantWarning:  "",
		},
		{
			name:         "negative defaults with warning",
			electionTick: -1,
			wantTick:     20,
			wantWarning:  "--raft election-tick=-1 is invalid; defaulting to 20. Use 0 or omit the flag to accept the default.",
		},
		{
			name:         "large negative defaults with warning",
			electionTick: -100,
			wantTick:     20,
			wantWarning:  "--raft election-tick=-100 is invalid; defaulting to 20. Use 0 or omit the flag to accept the default.",
		},
		{
			name:         "low valid value warns below recommended",
			electionTick: 2,
			wantTick:     2,
			wantWarning:  "--raft election-tick=2 gives a 200ms minimum election timeout. Values below 10 (1s) may cause spurious leader elections under GC pauses or network jitter.",
		},
		{
			name:         "recommended minimum no warning",
			electionTick: 10,
			wantTick:     10,
			wantWarning:  "",
		},
		{
			name:         "default value no warning",
			electionTick: 20,
			wantTick:     20,
			wantWarning:  "",
		},
		{
			name:         "election tick 1 coerces to default with warning",
			electionTick: 1,
			wantTick:     20,
			wantWarning:  "--raft election-tick=1 is invalid (must be > heartbeat tick 1); defaulting to 20.",
		},
		{
			name:         "large value accepted",
			electionTick: 100,
			wantTick:     100,
			wantWarning:  "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotTick, gotWarning := normalizeElectionTick(tc.electionTick)
			require.Equal(t, tc.wantTick, gotTick)
			require.Equal(t, tc.wantWarning, gotWarning)
		})
	}
}
