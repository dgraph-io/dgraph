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

// Used to store the current way of schema for all the
// predicates we are going to change.
// We would store this only after entries are commited via raft and
// we can have only one commited entry per index irrespective of term.
func (w *Wal) schemaKey(gid uint32, idx uint64) []byte {
	b := make([]byte, 30)
	binary.BigEndian.PutUint64(b[0:8], w.id)
	copy(b[8:10], []byte("sk"))
	binary.BigEndian.PutUint32(b[10:14], gid)
	binary.BigEndian.PutUint64(b[14:22], idx)
	return b
}

func (w *Wal) schemaPrefix(gid uint32) []byte {
	b := make([]byte, 14)
	binary.BigEndian.PutUint64(b[0:8], w.id)
	copy(b[8:10], []byte("sk"))
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

// TODO: This prefix might collide with hardState, when first two
// bytes in gid are equal to "ss" or "hs" or "sk" .
func (w *Wal) entryPrefix(gid uint32) []byte {
	b := make([]byte, 12)
	binary.BigEndian.PutUint64(b[0:8], w.id)
	binary.BigEndian.PutUint32(b[8:12], gid)
	return b
}

func (w *Wal) deleteHelper(start []byte, last []byte, wb []*badger.Entry) {
	opt := badger.DefaultIteratorOptions
	opt.PrefetchValues = false
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
}

func (w *Wal) StoreSchemaState(gid uint32, idx uint64, val []byte) error {
	key := w.schemaKey(gid, idx)
	return w.wals.Set(key, val, 0x00)
}

func (w *Wal) SchemaState(gid uint32, idx uint64) ([]byte, error) {
	var item badger.KVItem
	var data []byte
	if err := w.wals.Get(w.schemaKey(gid, idx), &item); err != nil {
		return nil, x.Wrapf(err, "while fetching snapshot from wal")
	}
	err := item.Value(func(val []byte) error {
		data := make([]byte, len(val))
		copy(data, val)
		return nil
	})
	return data, err
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

	// Delete all entries before this snapshot to save disk space.
	start := w.entryKey(gid, 0, 0)
	last := w.entryKey(gid, s.Metadata.Term, s.Metadata.Index)
	w.deleteHelper(start, last, wb)

	// Delete all schema keys
	start = w.schemaKey(gid, 0)
	last = w.schemaKey(gid, s.Metadata.Index)
	w.deleteHelper(start, last, wb)

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
	return nil
}

// Store stores the hardstate and entries for a given RAFT group.
func (w *Wal) Store(gid uint32, h raftpb.HardState, es []raftpb.Entry) error {
	wb := make([]*badger.Entry, 0, 100)

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

	if !raft.IsEmptyHardState(h) {
		data, err := h.Marshal()
		if err != nil {
			return x.Wrapf(err, "wal.Store: While marshal hardstate")
		}
		wb = badger.EntriesSet(wb, w.hardStateKey(gid), data)
	}

	// If we get no entries, then the default value of t and i would be zero. That would
	// end up deleting all the previous valid raft entry logs. This check avoids that.
	if t > 0 || i > 0 {
		// When writing an Entry with Index i, any previously-persisted entries
		// with Index >= i must be discarded.
		start := w.entryKey(gid, t, i+1)
		prefix := w.entryPrefix(gid)
		opt := badger.DefaultIteratorOptions
		opt.PrefetchValues = false
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
		return snap, x.Wrapf(err, "while fetching snapshot from wal")
	}
	err := item.Value(func(val []byte) error {
		return x.Wrapf(snap.Unmarshal(val), "While unmarshal snapshot")
	})
	return snap, err
}

func (w *Wal) HardState(gid uint32) (hd raftpb.HardState, rerr error) {
	var item badger.KVItem
	if err := w.wals.Get(w.hardStateKey(gid), &item); err != nil {
		return hd, x.Wrapf(err, "while fetching hardstate from wal")
	}
	err := item.Value(func(val []byte) error {
		return x.Wrapf(hd.Unmarshal(val), "While unmarshal hardstate")
	})
	return hd, err
}

func (w *Wal) Entries(gid uint32, fromTerm, fromIndex uint64) (es []raftpb.Entry, rerr error) {
	start := w.entryKey(gid, fromTerm, fromIndex)
	prefix := w.entryPrefix(gid)
	itr := w.wals.NewIterator(badger.DefaultIteratorOptions)
	defer itr.Close()

	for itr.Seek(start); itr.ValidForPrefix(prefix); itr.Next() {
		item := itr.Item()
		var e raftpb.Entry
		err := item.Value(func(val []byte) error {
			return x.Wrapf(e.Unmarshal(val), "While unmarshal raftpb.Entry")
		})
		if err != nil {
			return es, err
		}
		es = append(es, e)
	}
	return
}
