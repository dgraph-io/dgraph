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
	wals *badger.ManagedDB
	id   uint64
}

func Init(walStore *badger.ManagedDB, id uint64) *Wal {
	w := &Wal{wals: walStore, id: id}
	x.Check(w.StoreRaftId(id))
	return w
}

var idKey = []byte("raftid")

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

func (w *Wal) StoreRaftId(id uint64) error {
	txn := w.wals.NewTransactionAt(1, true)
	defer txn.Discard()
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], id)
	if err := txn.Set(idKey, b[:]); err != nil {
		return err
	}
	return txn.CommitAt(1, nil)
}

func RaftId(wals *badger.ManagedDB) (uint64, error) {
	txn := wals.NewTransactionAt(1, false)
	defer txn.Discard()
	item, err := txn.Get(idKey)
	if err == badger.ErrKeyNotFound {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	val, err := item.Value()
	if err != nil {
		return 0, err
	}
	id := binary.BigEndian.Uint64(val)
	return id, nil
}

func (w *Wal) StoreSnapshot(gid uint32, s raftpb.Snapshot) error {
	txn := w.wals.NewTransactionAt(1, true)
	defer txn.Discard()
	if raft.IsEmptySnap(s) {
		return nil
	}
	data, err := s.Marshal()
	if err != nil {
		return x.Wrapf(err, "wal.Store: While marshal snapshot")
	}
	if err := txn.Set(w.snapshotKey(gid), data); err != nil {
		return err
	}
	x.Printf("Writing snapshot to WAL, metadata: %+v, len(data): %d\n", s.Metadata, len(s.Data))

	// Delete all entries before this snapshot to save disk space.
	start := w.entryKey(gid, 0, 0)
	last := w.entryKey(gid, s.Metadata.Term, s.Metadata.Index)
	opt := badger.DefaultIteratorOptions
	opt.PrefetchValues = false
	itr := txn.NewIterator(opt)
	defer itr.Close()

	for itr.Seek(start); itr.Valid(); itr.Next() {
		key := itr.Item().Key()
		if bytes.Compare(key, last) > 0 {
			break
		}
		newk := make([]byte, len(key))
		copy(newk, key)
		if err := txn.Delete(newk); err != nil {
			return err
		}
	}

	// Failure to delete entries is not a fatal error, so should be
	// ok to ignore
	if err := txn.CommitAt(1, nil); err != nil {
		x.Printf("Error while storing snapshot %v\n", err)
		return err
	}
	return nil
}

// Store stores the hardstate and entries for a given RAFT group.
func (w *Wal) Store(gid uint32, h raftpb.HardState, es []raftpb.Entry) error {
	txn := w.wals.NewTransactionAt(1, true)

	var t, i uint64
	for _, e := range es {
		t, i = e.Term, e.Index
		data, err := e.Marshal()
		if err != nil {
			return x.Wrapf(err, "wal.Store: While marshal entry")
		}
		k := w.entryKey(gid, e.Term, e.Index)
		if err := txn.Set(k, data); err != nil {
			return err
		}
	}

	if !raft.IsEmptyHardState(h) {
		data, err := h.Marshal()
		if err != nil {
			return x.Wrapf(err, "wal.Store: While marshal hardstate")
		}
		if err := txn.Set(w.hardStateKey(gid), data); err != nil {
			return err
		}
	}

	// If we get no entries, then the default value of t and i would be zero. That would
	// end up deleting all the previous valid raft entry logs. This check avoids that.
	if t > 0 || i > 0 {
		// When writing an Entry with Index i, any previously-persisted entries
		// with Index >= i must be discarded.
		start := w.entryKey(gid, t, i+1)
		prefix := w.prefix(gid)
		opt := badger.DefaultIteratorOptions
		opt.PrefetchValues = false
		itr := txn.NewIterator(opt)
		defer itr.Close()

		for itr.Seek(start); itr.ValidForPrefix(prefix); itr.Next() {
			key := itr.Item().Key()
			newk := make([]byte, len(key))
			copy(newk, key)
			if err := txn.Delete(newk); err != nil {
				return err
			}
		}
	}
	if err := txn.CommitAt(1, nil); err != nil {
		return err
	}
	return nil
}

func (w *Wal) Snapshot(gid uint32) (snap raftpb.Snapshot, rerr error) {
	txn := w.wals.NewTransactionAt(1, false)
	defer txn.Discard()
	item, err := txn.Get(w.snapshotKey(gid))
	if err == badger.ErrKeyNotFound {
		return
	}
	if err != nil {
		return snap, x.Wrapf(err, "while fetching snapshot from wal")
	}
	val, err := item.Value()
	if err != nil {
		return
	}
	rerr = x.Wrapf(snap.Unmarshal(val), "While unmarshal snapshot")
	return
}

func (w *Wal) HardState(gid uint32) (hd raftpb.HardState, rerr error) {
	txn := w.wals.NewTransactionAt(1, false)
	defer txn.Discard()
	item, err := txn.Get(w.hardStateKey(gid))
	if err == badger.ErrKeyNotFound {
		return
	}
	if err != nil {
		return hd, x.Wrapf(err, "while fetching hardstate from wal")
	}
	val, err := item.Value()
	if err != nil {
		return
	}
	rerr = x.Wrapf(hd.Unmarshal(val), "While unmarshal snapshot")
	return
}

func (w *Wal) Entries(gid uint32, fromTerm, fromIndex uint64) (es []raftpb.Entry, rerr error) {
	start := w.entryKey(gid, fromTerm, fromIndex)
	prefix := w.prefix(gid)
	txn := w.wals.NewTransactionAt(1, false)
	defer txn.Discard()
	itr := txn.NewIterator(badger.DefaultIteratorOptions)
	defer itr.Close()

	for itr.Seek(start); itr.ValidForPrefix(prefix); itr.Next() {
		item := itr.Item()
		var e raftpb.Entry
		val, err := item.Value()
		if err != nil {
			return es, err
		}
		if err = e.Unmarshal(val); err != nil {
			return es, err
		}
		es = append(es, e)
	}
	return
}
