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
	"fmt"
	"log"
	"math"
	"os"

	"github.com/dgraph-io/badger/v2"
	"github.com/dgraph-io/dgraph/raftwal"
	"github.com/dgraph-io/dgraph/x"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	// RaftMigrate is the sub-command invoked when running "dgraph raft-migrate".
	RaftMigrate x.SubCommand
	quiet       bool // enabling quiet mode would suppress the warning logs
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

	nodeId := conf.GetInt("old-node-id")
	groupId := conf.GetInt("old-group-id")
	// Copied over from zero/run.go
	kvOpt := badger.LSMOnlyOptions(oldDir).
		WithSyncWrites(false).
		WithValueLogFileSize(64 << 20)

	kv, err := badger.OpenManaged(kvOpt)
	x.Checkf(err, "Error while opening WAL store")
	defer kv.Close()

	raftID, err := RaftId(kv)
	x.Check(err)
	oldWal := Init(kv, uint64(nodeId), uint32(groupId))

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
	x.Checkf(err, "failed to read entries from low:%d high:%d err:%s", firstIndex, lastIndex, err)

	snapshot, err := oldWal.Snapshot()
	x.Checkf(err, "failed to read snaphot %s", err)

	hs, err := oldWal.HardState()
	x.Checkf(err, "failed to read hardstate %s", err)

	if _, err := os.Stat(newDir); os.IsNotExist(err) {
		os.Mkdir(newDir, 0777)
	}

	newWal := raftwal.Init(newDir)
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
