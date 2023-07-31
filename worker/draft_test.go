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
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"go.etcd.io/etcd/raft/raftpb"

	debug "github.com/dgraph-io/dgraph/dgraph/cmd/debug"
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

// Create a function to delete a file.
func deleteFile(fileName string) {
	// Check if the file exists.
	if _, err := os.Stat(fileName); err == nil {
		// Delete the file.
		fmt.Println("Deleting file:", fileName)
		err := os.Remove(fileName)
		if err != nil {
			fmt.Println(err)
		}
	}
}

func TestCalculateSnapshot(t *testing.T) {
	// dir := t.TempDir()

	dir := "/home/alvis/go/src/github.com/dgraph-io/dgraph/dgraph/testw"

	// Iterate through all the files in the directory.
	// files, err := ioutil.ReadDir(dir)
	// if err != nil {
	// 	fmt.Println(err)
	// }

	// // Delete all the files.
	// for _, file := range files {
	// 	deleteFile(fmt.Sprintf("%s/%s", dir, file.Name()))
	// }

	ds := raftwal.Init(dir)
	defer ds.Close()

	fmt.Println("####################### Raft Wal Begin ####################################")
	fmt.Println("Initial Raft Wal")
	debug.PrintWal(ds)
	fmt.Println("######################## Raft Wal End ###################################")

	n := newNode(ds, 1, 1, "")
	var entries []raftpb.Entry
	// Txn: 1 -> 5 // 5 should be the ReadTs.
	// Txn: 2 // Should correspond to the index. Subtract 1 from the index.
	// Txn: 3 -> 4
	entries = append(entries, getEntryForMutation(1, 10), getEntryForMutation(2, 13),
		getEntryForCommit(3, 13, 14), getEntryForMutation(4, 12), getEntryForCommit(5, 10, 15))

	// entries = append(entries, getEntryForMutation(1, 2), getEntryForMutation(2, 3), getEntryForMutation(3, 4), getEntryForMutation(4, 5),
	// 	getEntryForCommit(5, 2, 5), getEntryForCommit(6, 3, 7), getEntryForCommit(7, 4, 6))
	require.NoError(t, n.Store.Save(&raftpb.HardState{}, entries, &raftpb.Snapshot{}))
	n.Applied.SetDoneUntil(5)
	posting.Oracle().RegisterStartTs(12)
	// posting.Oracle().RegisterStartTs(3)

	fmt.Println("####################### Raft Wal Begin ####################################")
	fmt.Println("Set Done until")
	debug.PrintWal(ds)
	fmt.Println("######################## Raft Wal End ###################################")

	fmt.Println("Applied DomUntil: ", n.Applied.DoneUntil())
	fmt.Println("MinPendingStartTs: ", posting.Oracle().MinPendingStartTs())

	snap, err := n.calculateSnapshot(0, n.Applied.DoneUntil(), posting.Oracle().MinPendingStartTs())
	require.NoError(t, err)
	require.Equal(t, uint64(15), snap.ReadTs)
	require.Equal(t, uint64(1), snap.Index)

	fmt.Println("####################### Raft Wal Begin ####################################")
	fmt.Println("Calculated Snapshot")
	debug.PrintWal(ds)
	fmt.Println("######################## Raft Wal End ###################################")

	// Check state of Raft store.
	var cs raftpb.ConfState
	require.NoError(t, n.Store.CreateSnapshot(snap.Index, &cs, nil))

	first, err := n.Store.FirstIndex()
	require.NoError(t, err)
	require.Equal(t, uint64(2), first)

	last, err := n.Store.LastIndex()
	require.NoError(t, err)
	require.Equal(t, uint64(5), last)

	fmt.Println("####################### Raft Wal Begin ####################################")
	fmt.Println("Created Snapshot")
	debug.PrintWal(ds)
	fmt.Println("######################## Raft Wal End ###################################")

	// This time commit all txns.
	// Txn: 7 -> 8
	// Txn: 2 -> 9
	entries = entries[:0]
	entries = append(entries, getEntryForMutation(6, 17), getEntryForCommit(7, 17, 18),
		getEntryForCommit(8, 12, 19))
	require.NoError(t, n.Store.Save(&raftpb.HardState{}, entries, &raftpb.Snapshot{}))
	n.Applied.SetDoneUntil(8)
	fmt.Println("####################### Raft Wal Begin ####################################")
	debug.PrintWal(ds)
	fmt.Println("######################## Raft Wal End ###################################")
	posting.Oracle().ResetTxns()
	snap, err = n.calculateSnapshot(0, n.Applied.DoneUntil(), posting.Oracle().MinPendingStartTs())
	require.NoError(t, err)
	require.Equal(t, uint64(19), snap.ReadTs)
	require.Equal(t, uint64(8), snap.Index)

	// Check state of Raft store.
	require.NoError(t, n.Store.CreateSnapshot(snap.Index, &cs, nil))
	first, err = n.Store.FirstIndex()
	require.NoError(t, err)
	require.Equal(t, uint64(9), first)

	fmt.Println("####################### Raft Wal Begin ####################################")
	debug.PrintWal(ds)
	fmt.Println("######################## Raft Wal End ###################################")

	entries = entries[:0]
	entries = append(entries, getEntryForMutation(9, 11))
	require.NoError(t, n.Store.Save(&raftpb.HardState{}, entries, &raftpb.Snapshot{}))
	n.Applied.SetDoneUntil(9)
	snap, err = n.calculateSnapshot(0, n.Applied.DoneUntil(), posting.Oracle().MinPendingStartTs())
	require.NoError(t, err)
	require.Nil(t, snap)

	fmt.Println("####################### Raft Wal Begin ####################################")
	debug.PrintWal(ds)
	fmt.Println("######################## Raft Wal End ###################################")
}
