/*
 * Copyright 2019 Dgraph Labs, Inc. and Contributors
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

package debug

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/raftwal"
	"github.com/dgraph-io/dgraph/x"
	humanize "github.com/dustin/go-humanize"
	"go.etcd.io/etcd/raft/raftpb"
)

func printEntry(es raftpb.Entry, pending map[uint64]bool) {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "%d . %d . %v . %-6s .", es.Term, es.Index, es.Type,
		humanize.Bytes(uint64(es.Size())))
	if es.Type == raftpb.EntryConfChange {
		fmt.Printf("%s\n", buf.Bytes())
		return
	}
	var pr pb.Proposal
	var zpr pb.ZeroProposal
	if err := pr.Unmarshal(es.Data); err == nil {
		printAlphaProposal(&buf, pr, pending)
	} else if err := zpr.Unmarshal(es.Data); err == nil {
		printZeroProposal(&buf, zpr)
	} else {
		fmt.Printf("%s Unable to parse Proposal: %v\n", buf.Bytes(), err)
		return
	}
	fmt.Printf("%s\n", buf.Bytes())
}

func printRaft(db *badger.DB, store *raftwal.DiskStorage) {
	fmt.Println()
	snap, err := store.Snapshot()
	if err != nil {
		fmt.Printf("Got error while retrieving snapshot: %v\n", err)
	} else {
		fmt.Printf("Snapshot Metadata: %+v\n", snap.Metadata)
		var ds pb.Snapshot
		var ms pb.MembershipState
		if err := ds.Unmarshal(snap.Data); err == nil {
			fmt.Printf("Snapshot Alpha: %+v\n", ds)
		} else if err := ms.Unmarshal(snap.Data); err == nil {
			for gid, group := range ms.GetGroups() {
				fmt.Printf("\nGROUP: %d\n", gid)
				for _, member := range group.GetMembers() {
					fmt.Printf("Member: %+v .\n", member)
				}
				for _, tablet := range group.GetTablets() {
					fmt.Printf("Tablet: %+v .\n", tablet)
				}
				group.Members = nil
				group.Tablets = nil
				fmt.Printf("Group: %d %+v .\n", gid, group)
			}
			ms.Groups = nil
			fmt.Printf("\nSnapshot Zero: %+v\n", ms)
		} else {
			fmt.Printf("Unable to unmarshal Dgraph snapshot: %v", err)
		}
	}
	fmt.Println()

	if hs, err := store.HardState(); err != nil {
		fmt.Printf("Got error while retrieving hardstate: %v\n", err)
	} else {
		fmt.Printf("Hardstate: %+v\n", hs)
	}

	if chk, err := store.Checkpoint(); err != nil {
		fmt.Printf("Got error while retrieving checkpoint: %v\n", err)
	} else {
		fmt.Printf("Checkpoint: %d\n", chk)
	}

	lastIdx, err := store.LastIndex()
	if err != nil {
		fmt.Printf("Got error while retrieving last index: %v\n", err)
		return
	}
	startIdx := snap.Metadata.Index + 1
	fmt.Printf("Last Index: %d . Num Entries: %d .\n\n", lastIdx, lastIdx-startIdx)

	// In case we need to truncate raft entries.
	batch := db.NewWriteBatch()
	defer batch.Cancel()
	var numTruncates int

	pending := make(map[uint64]bool)
	for startIdx < lastIdx-1 {
		entries, err := store.Entries(startIdx, lastIdx+1, 64<<20 /* 64 MB Max Size */)
		if err != nil {
			fmt.Printf("Got error while retrieving entries: %v\n", err)
			return
		}
		for _, ent := range entries {
			switch {
			case ent.Type == raftpb.EntryNormal && ent.Index < opt.wtruncateUntil:
				if len(ent.Data) == 0 {
					continue
				}
				ent.Data = nil
				numTruncates++
				k := store.EntryKey(ent.Index)
				data, err := ent.Marshal()
				if err != nil {
					log.Fatalf("Unable to marshal entry: %+v. Error: %v", ent, err)
				}
				if err := batch.Set(k, data, 0); err != nil {
					log.Fatalf("Unable to set data: %+v", err)
				}
			default:
				printEntry(ent, pending)
			}
			startIdx = x.Max(startIdx, ent.Index)
		}
	}
	if err := batch.Flush(); err != nil {
		fmt.Printf("Got error while flushing batch: %v\n", err)
	}
	if numTruncates > 0 {
		fmt.Printf("==> Log entries truncated: %d\n\n", numTruncates)
		err := db.Flatten(1)
		fmt.Printf("Flatten done with error: %v\n", err)
	}
}

func overwriteSnapshot(db *badger.DB, store *raftwal.DiskStorage) error {
	snap, err := store.Snapshot()
	x.Checkf(err, "Unable to get snapshot")
	cs := snap.Metadata.ConfState
	fmt.Printf("Confstate: %+v\n", cs)

	var dsnap pb.Snapshot
	if len(snap.Data) > 0 {
		x.Check(dsnap.Unmarshal(snap.Data))
	}
	fmt.Printf("Previous snapshot: %+v\n", dsnap)

	splits := strings.Split(opt.wsetSnapshot, ",")
	x.AssertTruef(len(splits) == 3,
		"Expected term,index,readts in string. Got: %s", splits)
	term, err := strconv.Atoi(splits[0])
	x.Check(err)
	index, err := strconv.Atoi(splits[1])
	x.Check(err)
	readTs, err := strconv.Atoi(splits[2])
	x.Check(err)

	ent := raftpb.Entry{
		Term:  uint64(term),
		Index: uint64(index),
		Type:  raftpb.EntryNormal,
	}
	fmt.Printf("Using term: %d , index: %d , readTs : %d\n", term, index, readTs)
	if dsnap.Index >= ent.Index {
		fmt.Printf("Older snapshot is >= index %d", ent.Index)
		return nil
	}

	// We need to write the Raft entry first.
	fmt.Printf("Setting entry: %+v\n", ent)
	hs := raftpb.HardState{
		Term:   ent.Term,
		Commit: ent.Index,
	}
	fmt.Printf("Setting hard state: %+v\n", hs)
	err = db.Update(func(txn *badger.Txn) error {
		data, err := ent.Marshal()
		if err != nil {
			return err
		}
		if err = txn.Set(store.EntryKey(ent.Index), data); err != nil {
			return err
		}

		data, err = hs.Marshal()
		if err != nil {
			return err
		}
		return txn.Set(store.HardStateKey(), data)
	})
	x.Check(err)

	dsnap.Index = ent.Index
	dsnap.ReadTs = uint64(readTs)

	fmt.Printf("Setting snapshot to: %+v\n", dsnap)
	data, err := dsnap.Marshal()
	x.Check(err)
	if err = store.CreateSnapshot(dsnap.Index, &cs, data); err != nil {
		fmt.Printf("Created snapshot with error: %v\n", err)
	}
	return err
}

func handleWal(db *badger.DB) error {
	rids := make(map[uint64]bool)
	gids := make(map[uint32]bool)

	parseIds := func(item *badger.Item) {
		key := item.Key()
		switch {
		case len(key) == 14:
			// hard state and snapshot key.
			rid := binary.BigEndian.Uint64(key[0:8])
			rids[rid] = true

			gid := binary.BigEndian.Uint32(key[10:14])
			gids[gid] = true
		case len(key) == 20:
			// entry key.
			rid := binary.BigEndian.Uint64(key[0:8])
			rids[rid] = true

			gid := binary.BigEndian.Uint32(key[8:12])
			gids[gid] = true
		default:
			// Ignore other keys.
		}
	}

	err := db.View(func(txn *badger.Txn) error {
		opt := badger.DefaultIteratorOptions
		opt.PrefetchValues = false
		itr := txn.NewIterator(opt)
		defer itr.Close()

		for itr.Rewind(); itr.Valid(); itr.Next() {
			parseIds(itr.Item())
		}
		return nil
	})
	if err != nil {
		return err
	}
	fmt.Printf("rids: %v\n", rids)
	fmt.Printf("gids: %v\n", gids)

	for rid := range rids {
		for gid := range gids {
			fmt.Printf("Iterating with Raft Id = %d Groupd Id = %d\n", rid, gid)
			store := raftwal.Init(db, rid, gid)
			switch {
			case len(opt.wsetSnapshot) > 0:
				return overwriteSnapshot(db, store)

			default:
				printRaft(db, store)
			}
		}
	}
	return nil
}
