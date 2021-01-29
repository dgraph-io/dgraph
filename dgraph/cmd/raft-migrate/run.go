/*
 * Copyright 2021 Dgraph Labs, Inc. and Contributors
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
		Short: "Run the Raft migration tool",
		Run: func(cmd *cobra.Command, args []string) {
			if err := run(RaftMigrate.Conf); err != nil {
				log.Fatalf("%v\n", err)
			}
		},
		Annotations: map[string]string{"group": "tool"},
	}
	RaftMigrate.EnvPrefix = "DGRAPH_RAFT_MIGRATE"
	RaftMigrate.Cmd.SetHelpTemplate(x.NonRootTemplate)

	flag := RaftMigrate.Cmd.Flags()
	flag.StringP("old-dir", "", "", "Path to the old (z)w directory.")
	flag.StringP("new-dir", "", "", "Path to the new (z)w directory.")
	enc.RegisterFlags(flag)
}

func updateEntry(entry raftpb.Entry) raftpb.Entry {
	// Raft commits an empty entry on becoming leader.
	if entry.Type == raftpb.EntryConfChange || len(entry.Data) == 0 {
		return entry
	}

	data := make([]byte, 8+len(entry.Data))
	// First 8 bytes are used to store key in current raftwal format. We don't need the key in
	// migrate tool. In zero, we don't use the key and in alpha the key used for not applying the
	// same proposal twice.
	copy(data[8:], entry.Data)
	entry.Data = data
	return entry
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

func run(conf *viper.Viper) error {
	oldDir := conf.GetString("old-dir")
	newDir := conf.GetString("new-dir")
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
	oldEntries, err := oldWal.Entries(firstIndex, lastIndex+1, math.MaxUint64)

	newEntries := make([]raftpb.Entry, len(oldEntries))
	for i, entry := range oldEntries {
		newEntries[i] = updateEntry(entry)
	}

	x.Checkf(err, "failed to read entries from low:%d high:%d err:%s", firstIndex, lastIndex, err)

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

	snapshot, err := oldWal.Snapshot()
	x.Checkf(err, "failed to read snaphot %s", err)
	if groupID == 0 {
		// We earlier used to store MembershipState in raftpb.Snapshot. Now we store ZeroSnapshot in
		// case of zero.
		fmt.Println("Parsing and converting zero-snapshot")
		parseAndConvertSnapshot(&snapshot)
	}

	hs, err := oldWal.HardState()
	x.Checkf(err, "failed to read hardstate %s", err)

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
