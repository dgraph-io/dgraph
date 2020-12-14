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
	flag.StringP("new-dir", "", "", "Path to the new (z)w directory.")
	flag.BoolP("is-alpha", "", false, "Set it to true if migrating alpha raftwal (default: false)")
	enc.RegisterFlags(flag)
}

func parseAndConvertKey(key, keyFormat string) uint64 {
	var random uint64
	var parsedKey uint32
	fmt.Sscanf(key, keyFormat, &parsedKey, &random)
	return uint64(uint64(parsedKey)<<32 | random>>32)
}

func parseAndConvertSnapshot(snap *raftpb.Snapshot) {
	var ms pb.MembershipState
	var zs pb.ZeroSnapshot
	var err error
	x.Check(ms.Unmarshal(snap.Data))
	zs.State = &ms
	zs.Index = snap.Metadata.Index
	// It is okay to not set zs.CheckpointTs as it is used for purgeBelow.
	snap.Data, err = zs.Marshal()
	x.Check(err)
}

func updateProposalData(entry raftpb.Entry) raftpb.Entry {
	if entry.Type == raftpb.EntryConfChange {
		return entry
	}
	var oldProposal Proposal
	oldProposal.Unmarshal(entry.Data)
	newKey := parseAndConvertKey(oldProposal.Key, "%02d-%d")

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
	if entry.Type == raftpb.EntryConfChange {
		return entry
	}
	var oldProposal ZeroProposal
	oldProposal.Unmarshal(entry.Data)
	newKey := parseAndConvertKey(oldProposal.Key, "z%x-%d")

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
	binary.BigEndian.PutUint64(data[:8], newKey)
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

	oldWal, err := raftwal.InitEncrypted(oldDir, encKey)
	x.Checkf(err, "failed to initialize old wal: %s", err)
	defer oldWal.Close()

	firstIndex, err := oldWal.FirstIndex()
	x.Checkf(err, "failed to read FirstIndex from old wal: %s", err)

	lastIndex, err := oldWal.LastIndex()
	x.Checkf(err, "failed to read LastIndex from the old wal: %s", err)

	fmt.Printf("Fetching entries from low: %d to high: %d\n", firstIndex, lastIndex)
	// Should we batch this up?
	oldEntries, err := oldWal.Entries(1, lastIndex+1, math.MaxUint64)
	x.AssertTrue(len(oldEntries) == oldWal.NumEntries())

	newEntries := make([]raftpb.Entry, len(oldEntries))
	if isAlpha {
		for i, entry := range oldEntries {
			newEntries[i] = updateProposalData(entry)
		}
	} else {
		for i, entry := range oldEntries {
			newEntries[i] = updateZeroProposalData(entry)
		}
	}

	x.Checkf(err, "failed to read entries from low:%d high:%d err:%s", firstIndex, lastIndex, err)

	snapshot, err := oldWal.Snapshot()
	x.Checkf(err, "failed to read snaphot %s", err)
	if !isAlpha {
		// We earlier used to store MembershipState in raftpb.Snapshot. Now we store ZeroSnapshot in
		// case of zero.
		fmt.Println("Parsing and converting zero-snapshot")
		parseAndConvertSnapshot(&snapshot)
	}

	hs, err := oldWal.HardState()
	x.Checkf(err, "failed to read hardstate %s", err)

	if _, err := os.Stat(newDir); os.IsNotExist(err) {
		os.Mkdir(newDir, 0777)
	}

	newWal, err := raftwal.InitEncrypted(newDir, encKey)
	x.Check(err)

	// Set the raft ID
	raftID := oldWal.Uint(raftwal.RaftId)
	fmt.Printf("Setting raftID to: %+v\n", raftID)
	newWal.SetUint(raftwal.RaftId, raftID)

	// Set the Group ID
	groupID := oldWal.Uint(raftwal.GroupId)
	fmt.Printf("Setting GroupID to: %+v\n", groupID)
	newWal.SetUint(raftwal.GroupId, groupID)

	// Set the checkpoint index
	checkPoint, err := oldWal.Checkpoint()
	x.Checkf(err, "failed to read checkpoint %s", err)
	newWal.SetUint(raftwal.CheckpointIndex, checkPoint)

	fmt.Printf("Saving num of oldEntries:%+v\nsnapshot %+v\nhardstate = %+v\n",
		len(newEntries), snapshot, hs)
	if err := newWal.Save(&hs, newEntries, &snapshot); err != nil {
		log.Fatalf("failed to save new state. hs: %+v, snapshot: %+v, oldEntries: %+v, err: %s",
			hs, oldEntries, snapshot, err)
	}
	if err := newWal.Close(); err != nil {
		log.Fatalf("Failed to close new wal: %s", err)
	}
	fmt.Println("Succesfully completed migrating.")
	return nil
}
