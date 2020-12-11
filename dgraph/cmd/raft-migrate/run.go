/*
 * Copyright 2020 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package raftmigrate

import (
	"encoding/binary"
	"fmt"
	"log"
	"math"
	"os"

	"github.com/dgraph-io/dgraph/ee/enc"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/raftwal"
	"github.com/dgraph-io/dgraph/x"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.etcd.io/etcd/raft/raftpb"
)

var (
	// RaftMigrate is the sub-command invoked when running "dgraph raft-migrate".
	RaftMigrate x.SubCommand
	quiet       bool // enabling quiet mode would suppress the warning logs
	encKey      x.SensitiveByteSlice
)

func init() {
	RaftMigrate.Cmd = &cobra.Command{
		Use:   "raftmigrate",
		Short: "Run the raft migrate tool",
		Run: func(cmd *cobra.Command, args []string) {
			if err := run(RaftMigrate.Conf); err != nil {
				log.Fatalf("%v\n", err)
			}
		},
	}
	RaftMigrate.EnvPrefix = "DGRAPH_RAFT_MIGRATE"

	flag := RaftMigrate.Cmd.Flags()
	flag.StringP("old-dir", "", "", "Path to the old (z)w directory.")
	flag.IntP("old-node-id", "", 1,
		"Node ID of the old node. This will be the node ID of the new node.")
	flag.IntP("old-group-id", "", 0, "Group ID of the old node. This is used to open the old wal.")
	flag.StringP("new-dir", "", "", "Path to the new (z)w directory.")
	flag.BoolP("is-alpha", "", true, "true if alpha directory false if zeros")
	enc.RegisterFlags(flag)
}

func parseAndConvertKey(key string) uint64 {
	keyFormat := "z%x-%d"
	var random uint64
	var parsedKey uint32
	fmt.Sscanf(key, keyFormat, &parsedKey, &random)
	return uint64(uint64(parsedKey)<<32 | random>>32)
}

func updateProposalData(entry raftpb.Entry) raftpb.Entry {

	var oldProposal Proposal
	oldProposal.Unmarshal(entry.Data)
	newKey := parseAndConvertKey(oldProposal.Key)
	var newProposal pb.Proposal
	newProposal.Mutations = oldProposal.Mutations
	newProposal.Kv = oldProposal.Kv
	newProposal.State = oldProposal.State
	newProposal.CleanPredicate = oldProposal.CleanPredicate
	newProposal.Delta = oldProposal.Delta
	newProposal.Snapshot = oldProposal.Snapshot
	newProposal.Index = oldProposal.Index
	newProposal.ExpectedChecksum = oldProposal.ExpectedChecksum
	newProposal.Restore = oldProposal.Restore
	data := make([]byte, 8+newProposal.Size())
	binary.BigEndian.PutUint64(data, newKey)
	sz, err := newProposal.MarshalToSizedBuffer(data[8:])
	data = data[:8+sz]

	x.Checkf(err, "Failed to marshal proposal to buffer")
	entry.Data = data
	return entry
}

func updateZeroProposalData(entry raftpb.Entry) raftpb.Entry {
	var oldProposal ZeroProposal
	oldProposal.Unmarshal(entry.Data)
	newKey := parseAndConvertKey(oldProposal.Key)
	var newProposal pb.ZeroProposal

	newProposal.SnapshotTs = oldProposal.SnapshotTs
	newProposal.Member = oldProposal.Member
	newProposal.Tablet = oldProposal.Tablet
	newProposal.MaxLeaseId = oldProposal.MaxLeaseId
	newProposal.MaxTxnTs = oldProposal.MaxTxnTs
	newProposal.MaxRaftId = oldProposal.MaxRaftId
	newProposal.Txn = oldProposal.Txn
	newProposal.Cid = oldProposal.Cid
	newProposal.License = oldProposal.License
	// Snapshot is a newly added field hence skipped
	data := make([]byte, 8+newProposal.Size())
	binary.BigEndian.PutUint64(data, newKey)
	sz, err := newProposal.MarshalToSizedBuffer(data[8:])
	data = data[:8+sz]

	x.Checkf(err, "Failed to marshal proposal to buffer")
	entry.Data = data
	return entry
}

func run(conf *viper.Viper) error {
	oldDir := conf.GetString("old-dir")
	newDir := conf.GetString("new-dir")
	isAlpha := conf.GetBool("is-alpha")
	if len(oldDir) == 0 {
		log.Fatal("--old-dir not specified.")
	}

	if len(newDir) == 0 {
		log.Fatal("--new-dir not specified.")
	}

	// TODO(rahul): Maybe uncomment following
	//nodeId := conf.GetInt("old-node-id")
	//groupId := conf.GetInt("old-group-id")

	oldWal := raftwal.Init(oldDir) //Init(kv, uint64(nodeId), uint32(groupId))
	defer oldWal.Close()

	// TODO(rahul): raftwal.RaftId doesn't seem to be set correctly.
	raftID := uint64(raftwal.RaftId)
	firstIndex, err := oldWal.FirstIndex()
	x.Checkf(err, "failed to read FirstIndex from old wal: %s", err)

	lastIndex, err := oldWal.LastIndex()
	x.Checkf(err, "failed to read LastIndex from the old wal: %s", err)

	// TODO(ibrahim): Do we need this??
	// The new directory has one less entry if we don't do this.
	lastIndex++

	fmt.Printf("Fetching entries from low: %d to high: %d\n", firstIndex, lastIndex)
	// Should we batch this up?
	oldEntries, err := oldWal.Entries(firstIndex, lastIndex, math.MaxUint64)

	if isAlpha {
		for i, entry := range oldEntries {
			oldEntries[i] = updateProposalData(entry)
		}
	} else {
		for i, entry := range oldEntries {
			oldEntries[i] = updateZeroProposalData(entry)
		}
	}

	x.Checkf(err, "failed to read entries from low:%d high:%d err:%s", firstIndex, lastIndex, err)

	snapshot, err := oldWal.Snapshot()
	x.Checkf(err, "failed to read snaphot %s", err)

	hs, err := oldWal.HardState()
	x.Checkf(err, "failed to read hardstate %s", err)

	if _, err := os.Stat(newDir); os.IsNotExist(err) {
		os.Mkdir(newDir, 0777)
	}

	newWal, err := raftwal.InitEncrypted(newDir, encKey)
	x.Check(err)

	fmt.Printf("Setting raftID to: %+v\n", raftID)
	// Set the raft ID
	newWal.SetUint(raftwal.RaftId, raftID)

	fmt.Printf("Saving num of oldEntries:%+v\nsnapshot %+v\nhardstate = %+v\n",
		len(oldEntries), snapshot, hs)
	if err := newWal.Save(&hs, oldEntries, &snapshot); err != nil {
		log.Fatalf("failed to save new state. hs: %+v, snapshot: %+v, oldEntries: %+v, err: %s",
			hs, oldEntries, snapshot, err)
	}
	if err := newWal.Close(); err != nil {
		log.Fatalf("Failed to close new wal: %s", err)
	}
	fmt.Println("Succesfully completed migrating.")
	return nil
}
