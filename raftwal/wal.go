/*
 * Copyright (C) 2017 Dgraph Labs, Inc. and Contributors
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 */

package raftwal

import (
	"bytes"
	"encoding/binary"

	"github.com/coreos/etcd/raft"
	"github.com/coreos/etcd/raft/raftpb"
	"github.com/dgraph-io/badger"

	"github.com/dgraph-io/dgraph/x"
)

type Wal struct {
	wals *badger.KV
	id   uint64
}

func Init(walStore *badger.KV, id uint64) *Wal {
	return &Wal{wals: walStore, id: id}
}

func (w *Wal) snapshotKey(gid uint32) []byte {
	b := make([]byte, 14)
	binary.BigEndian.PutUint64(b[0:8], w.id)
	copy(b[8:10], []byte("ss"))
	binary.BigEndian.PutUint32(b[10:14], gid)
	return b
}

func (w *Wal) hardStateKey(gid uint32) []byte {
	b := make([]byte, 14)
	binary.BigEndian.PutUint64(b[0:8], w.id)
	copy(b[8:10], []byte("hs"))
	binary.BigEndian.PutUint32(b[10:14], gid)
	return b
}

func (w *Wal) entryKey(gid uint32, term, idx uint64) []byte {
	b := make([]byte, 28)
	binary.BigEndian.PutUint64(b[0:8], w.id)
	binary.BigEndian.PutUint32(b[8:12], gid)
	binary.BigEndian.PutUint64(b[12:20], term)
	binary.BigEndian.PutUint64(b[20:28], idx)
	return b
}

func (w *Wal) prefix(gid uint32) []byte {
	b := make([]byte, 12)
	binary.BigEndian.PutUint64(b[0:8], w.id)
	binary.BigEndian.PutUint32(b[8:12], gid)
	return b
}

func (w *Wal) StoreSnapshot(gid uint32, s raftpb.Snapshot) error {
	wb := make([]*badger.Entry, 0, 100)
	if raft.IsEmptySnap(s) {
		return nil
	}
	data, err := s.Marshal()
	if err != nil {
		return x.Wrapf(err, "wal.Store: While marshal snapshot")
	}
	if err := w.wals.Set(w.snapshotKey(gid), data, 0x00); err != nil {
		return err
	}
	x.Printf("Writing snapshot to WAL: %+v\n", s)

	go func(term uint64, index uint64) {
		// Delete all entries before this snapshot to save disk space.
		start := w.entryKey(gid, 0, 0)
		last := w.entryKey(gid, term, index)
		opt := badger.DefaultIteratorOptions
		opt.FetchValues = false
		itr := w.wals.NewIterator(opt)
		defer itr.Close()

		for itr.Seek(start); itr.Valid(); itr.Next() {
			key := itr.Item().Key()
			if bytes.Compare(key, last) > 0 {
				break
			}
			newk := make([]byte, len(key))
			copy(newk, key)
			wb = badger.EntriesDelete(wb, newk)
		}

		// Failure to delete entries is not a fatal error, so should be
		// ok to ignore
		if err := w.wals.BatchSet(wb); err != nil {
			x.Printf("Error while deleting entries %v\n", err)
		}
		for _, wbe := range wb {
			if err := wbe.Error; err != nil {
				x.Printf("Error while deleting entries %v\n", err)
			}
		}
	}(s.Metadata.Term, s.Metadata.Index)
	return nil
}

// Store stores the snapshot, hardstate and entries for a given RAFT group.
func (w *Wal) Store(gid uint32, h raftpb.HardState, es []raftpb.Entry) error {
	wb := make([]*badger.Entry, 0, 100)

	if !raft.IsEmptyHardState(h) {
		data, err := h.Marshal()
		if err != nil {
			return x.Wrapf(err, "wal.Store: While marshal hardstate")
		}
		wb = badger.EntriesSet(wb, w.hardStateKey(gid), data)
	}

	var t, i uint64
	for _, e := range es {
		t, i = e.Term, e.Index
		data, err := e.Marshal()
		if err != nil {
			return x.Wrapf(err, "wal.Store: While marshal entry")
		}
		k := w.entryKey(gid, e.Term, e.Index)
		wb = badger.EntriesSet(wb, k, data)
	}

	// If we get no entries, then the default value of t and i would be zero. That would
	// end up deleting all the previous valid raft entry logs. This check avoids that.
	if t > 0 || i > 0 {
		// When writing an Entry with Index i, any previously-persisted entries
		// with Index >= i must be discarded.
		start := w.entryKey(gid, t, i+1)
		prefix := w.prefix(gid)
		opt := badger.DefaultIteratorOptions
		opt.FetchValues = false
		itr := w.wals.NewIterator(opt)
		defer itr.Close()

		for itr.Seek(start); itr.ValidForPrefix(prefix); itr.Next() {
			key := itr.Item().Key()
			newk := make([]byte, len(key))
			copy(newk, key)
			wb = badger.EntriesDelete(wb, newk)
		}
	}
	if err := w.wals.BatchSet(wb); err != nil {
		return err
	}
	for _, wbe := range wb {
		if err := wbe.Error; err != nil {
			return err
		}
	}
	return nil
}

func (w *Wal) Snapshot(gid uint32) (snap raftpb.Snapshot, rerr error) {
	var item badger.KVItem
	if err := w.wals.Get(w.snapshotKey(gid), &item); err != nil {
		rerr = x.Wrapf(err, "while fetching snapshot from wal")
		return
	}
	val := item.Value()
	// Originally, with RocksDB, this can return an error and a non-null rdb.Slice object with Data=nil.
	// And for this case, we do NOT return.
	rerr = x.Wrapf(snap.Unmarshal(val), "While unmarshal snapshot")
	return
}

func (w *Wal) HardState(gid uint32) (hd raftpb.HardState, rerr error) {
	var item badger.KVItem
	if err := w.wals.Get(w.hardStateKey(gid), &item); err != nil {
		rerr = x.Wrapf(err, "while fetching hardstate from wal")
		return
	}
	val := item.Value()
	// Originally, with RocksDB, this can return an error and a non-null rdb.Slice object with Data=nil.
	// And for this case, we do NOT return.
	rerr = x.Wrapf(hd.Unmarshal(val), "While unmarshal hardstate")
	return
}

func (w *Wal) Entries(gid uint32, fromTerm, fromIndex uint64) (es []raftpb.Entry, rerr error) {
	start := w.entryKey(gid, fromTerm, fromIndex)
	prefix := w.prefix(gid)
	itr := w.wals.NewIterator(badger.DefaultIteratorOptions)
	defer itr.Close()

	for itr.Seek(start); itr.ValidForPrefix(prefix); itr.Next() {
		item := itr.Item()
		var e raftpb.Entry
		if err := e.Unmarshal(item.Value()); err != nil {
			return es, x.Wrapf(err, "While unmarshal raftpb.Entry")
		}
		es = append(es, e)
	}
	return
}
