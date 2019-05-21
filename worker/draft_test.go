/*
 * Copyright 2018 Dgraph Labs, Inc. and Contributors
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
	"io/ioutil"
	"os"
	"testing"

	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/raftwal"
	"github.com/dgraph-io/dgraph/x"
	"github.com/stretchr/testify/require"
	"go.etcd.io/etcd/raft/raftpb"
)

func openBadger(dir string) (*badger.DB, error) {
	opt := badger.DefaultOptions
	opt.Dir = dir
	opt.ValueDir = dir

	return badger.Open(opt)
}

func getEntryForMutation(index, startTs uint64) raftpb.Entry {
	proposal := pb.Proposal{Mutations: &pb.Mutations{StartTs: startTs}}
	data, err := proposal.Marshal()
	x.Check(err)
	return raftpb.Entry{Index: index, Term: 1, Type: raftpb.EntryNormal, Data: data}
}

func getEntryForCommit(index, startTs, commitTs uint64) raftpb.Entry {
	delta := &pb.OracleDelta{}
	delta.Txns = append(delta.Txns, &pb.TxnStatus{StartTs: startTs, CommitTs: commitTs})
	proposal := pb.Proposal{Delta: delta}
	data, err := proposal.Marshal()
	x.Check(err)
	return raftpb.Entry{Index: index, Term: 1, Type: raftpb.EntryNormal, Data: data}
}

func TestCalculateSnapshot(t *testing.T) {
	dir, err := ioutil.TempDir("", "badger")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	db, err := openBadger(dir)
	require.NoError(t, err)
	ds := raftwal.Init(db, 0, 0)

	n := newNode(ds, 1, 1, "")
	var entries []raftpb.Entry
	// Txn: 1 -> 5 // 5 should be the ReadTs.
	// Txn: 2 // Should correspond to the index. Subtract 1 from the index.
	// Txn: 3 -> 4
	entries = append(entries, getEntryForMutation(1, 1))
	entries = append(entries, getEntryForMutation(2, 3))
	entries = append(entries, getEntryForMutation(3, 2))  // Start ts can be jumbled.
	entries = append(entries, getEntryForCommit(4, 3, 4)) // But commit ts would be serial.
	entries = append(entries, getEntryForCommit(5, 1, 5))
	require.NoError(t, n.Store.Save(raftpb.HardState{}, entries, raftpb.Snapshot{}))
	n.Applied.SetDoneUntil(5)
	posting.Oracle().RegisterStartTs(2)
	snap, err := n.calculateSnapshot(0, 1)
	require.NoError(t, err)
	require.Equal(t, uint64(5), snap.ReadTs)
	require.Equal(t, uint64(1), snap.Index)

	// Check state of Raft store.
	var cs raftpb.ConfState
	err = n.Store.CreateSnapshot(snap.Index, &cs, nil)
	require.NoError(t, err)

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
	entries = append(entries, getEntryForMutation(6, 7))
	entries = append(entries, getEntryForCommit(7, 7, 8))
	entries = append(entries, getEntryForCommit(8, 2, 9))
	require.NoError(t, n.Store.Save(raftpb.HardState{}, entries, raftpb.Snapshot{}))
	n.Applied.SetDoneUntil(8)
	posting.Oracle().ResetTxns()
	snap, err = n.calculateSnapshot(0, 1)
	require.NoError(t, err)
	require.Equal(t, uint64(9), snap.ReadTs)
	require.Equal(t, uint64(8), snap.Index)

	// Check state of Raft store.
	err = n.Store.CreateSnapshot(snap.Index, &cs, nil)
	require.NoError(t, err)
	first, err = n.Store.FirstIndex()
	require.NoError(t, err)
	require.Equal(t, uint64(9), first)

	entries = entries[:0]
	entries = append(entries, getEntryForMutation(9, 11))
	require.NoError(t, n.Store.Save(raftpb.HardState{}, entries, raftpb.Snapshot{}))
	n.Applied.SetDoneUntil(9)
	snap, err = n.calculateSnapshot(0, 0)
	require.NoError(t, err)
	require.Nil(t, snap)
}
