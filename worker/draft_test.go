/*
 * Copyright 2023 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package worker

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"go.etcd.io/etcd/raft/raftpb"

	"github.com/dgraph-io/badger/v4"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/raftwal"
	"github.com/dgraph-io/dgraph/x"
)

func getEntryForMutation(index, startTs uint64) raftpb.Entry {
	proposal := pb.Proposal{Mutations: &pb.Mutations{StartTs: startTs}}
	data := make([]byte, 8+proposal.Size())
	sz, err := proposal.MarshalToSizedBuffer(data)
	x.Check(err)
	data = data[:8+sz]
	return raftpb.Entry{Index: index, Term: 1, Type: raftpb.EntryNormal, Data: data}
}

func getEntryForCommit(index, startTs, commitTs uint64) raftpb.Entry {
	delta := &pb.OracleDelta{}
	delta.Txns = append(delta.Txns, &pb.TxnStatus{StartTs: startTs, CommitTs: commitTs})
	proposal := pb.Proposal{Delta: delta}
	data := make([]byte, 8+proposal.Size())
	sz, err := proposal.MarshalToSizedBuffer(data)
	x.Check(err)
	data = data[:8+sz]
	return raftpb.Entry{Index: index, Term: 1, Type: raftpb.EntryNormal, Data: data}
}

func TestCalculateSnapshot(t *testing.T) {
	dir := t.TempDir()
	ds := raftwal.Init(dir)
	defer ds.Close()

	n := newNode(ds, 1, 1, "")
	var entries []raftpb.Entry
	// Txn: 1 -> 5 // 5 should be the ReadTs.
	// Txn: 2 // Should correspond to the index. Subtract 1 from the index.
	// Txn: 3 -> 4
	entries = append(entries, getEntryForMutation(1, 1), getEntryForMutation(2, 3),
		getEntryForMutation(3, 2), getEntryForCommit(4, 3, 4), getEntryForCommit(5, 1, 5))
	require.NoError(t, n.Store.Save(&raftpb.HardState{}, entries, &raftpb.Snapshot{}))
	n.Applied.SetDoneUntil(5)
	posting.Oracle().RegisterStartTs(2)
	snap, err := n.calculateSnapshot(0, n.Applied.DoneUntil(), posting.Oracle().MinPendingStartTs())
	require.NoError(t, err)
	require.Equal(t, uint64(5), snap.ReadTs)
	require.Equal(t, uint64(1), snap.Index)

	// Check state of Raft store.
	var cs raftpb.ConfState
	require.NoError(t, n.Store.CreateSnapshot(snap.Index, &cs, nil))

	first, err := n.Store.FirstIndex()
	require.NoError(t, err)
	require.Equal(t, uint64(2), first)

	last, err := n.Store.LastIndex()
	require.NoError(t, err)
	require.Equal(t, uint64(5), last)

	// This time commit all txns.
	// Txn: 7 -> 8
	// Txn: 2 -> 9
	entries = entries[:0]
	entries = append(entries, getEntryForMutation(6, 7), getEntryForCommit(7, 7, 8),
		getEntryForCommit(8, 2, 9))
	require.NoError(t, n.Store.Save(&raftpb.HardState{}, entries, &raftpb.Snapshot{}))
	n.Applied.SetDoneUntil(8)
	posting.Oracle().ResetTxns()
	snap, err = n.calculateSnapshot(0, n.Applied.DoneUntil(), posting.Oracle().MinPendingStartTs())
	require.NoError(t, err)
	require.Equal(t, uint64(9), snap.ReadTs)
	require.Equal(t, uint64(8), snap.Index)

	// Check state of Raft store.
	require.NoError(t, n.Store.CreateSnapshot(snap.Index, &cs, nil))
	first, err = n.Store.FirstIndex()
	require.NoError(t, err)
	require.Equal(t, uint64(9), first)

	entries = entries[:0]
	entries = append(entries, getEntryForMutation(9, 11))
	require.NoError(t, n.Store.Save(&raftpb.HardState{}, entries, &raftpb.Snapshot{}))
	n.Applied.SetDoneUntil(9)
	snap, err = n.calculateSnapshot(0, n.Applied.DoneUntil(), posting.Oracle().MinPendingStartTs())
	require.NoError(t, err)
	require.Nil(t, snap)
}

func TestMain(m *testing.M) {
	posting.Config.CommitFraction = 0.10
	gr = new(groupi)
	gr.gid = 1
	gr.tablets = make(map[string]*pb.Tablet)
	addTablets := func(attrs []string, gid uint32, namespace uint64) {
		for _, attr := range attrs {
			gr.tablets[x.NamespaceAttr(namespace, attr)] = &pb.Tablet{GroupId: gid}
		}
	}

	addTablets([]string{"name", "name2", "age", "http://www.w3.org/2000/01/rdf-schema#range", "",
		"friend", "dgraph.type", "dgraph.graphql.xid", "dgraph.graphql.schema"},
		1, x.GalaxyNamespace)
	addTablets([]string{"friend_not_served"}, 2, x.GalaxyNamespace)
	addTablets([]string{"name"}, 1, 0x2)

	dir, err := os.MkdirTemp("", "storetest_")
	x.Check(err)
	defer os.RemoveAll(dir)

	opt := badger.DefaultOptions(dir)
	ps, err := badger.OpenManaged(opt)
	x.Check(err)
	pstore = ps
	// Not using posting list cache
	posting.Init(ps, 0)
	Init(ps)

	m.Run()
}
