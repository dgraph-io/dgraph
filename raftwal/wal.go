/*
 * Copyright 2016-2018 Dgraph Labs, Inc.
 *
 * This file is available under the Apache License, Version 2.0,
 * with the Commons Clause restriction.
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
	wals *badger.DB
	id   uint64
	gid  uint32
}

func Init(walStore *badger.DB, id uint64) *Wal {
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
	return w.wals.Update(func(txn *badger.Txn) error {
		var b [8]byte
		binary.BigEndian.PutUint64(b[:], id)
		return txn.Set(idKey, b[:])
	})
}

func RaftId(wals *badger.DB) (uint64, error) {
	var id uint64
	err := wals.View(func(txn *badger.Txn) error {
		item, err := txn.Get(idKey)
		if err != nil {
			return err
		}
		val, err := item.Value()
		if err != nil {
			return err
		}
		id = binary.BigEndian.Uint64(val)
		return nil
	})
	return id, err
}

// TODO: Move gid within WAL.
func (w *Wal) StoreSnapshot(gid uint32, s raftpb.Snapshot) error {
	if raft.IsEmptySnap(s) {
		return nil
	}
	data, err := s.Marshal()
	if err != nil {
		return x.Wrapf(err, "wal.Store: While marshal snapshot")
	}
	txn := w.wals.NewTransaction(true)
	defer txn.Discard()
	if err := txn.Set(w.snapshotKey(gid), data); err != nil {
		return err
	}

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
		if err := txn.Delete(newk); err == badger.ErrTxnTooBig {
			if err := txn.Commit(nil); err != nil {
				return err
			}
			txn = w.wals.NewTransaction(true)
			defer txn.Discard()
			if err := txn.Delete(newk); err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
	}

	return txn.Commit(nil)
}

// Store stores the hardstate and entries for a given RAFT group.
func (w *Wal) Store(gid uint32, h raftpb.HardState, es []raftpb.Entry) error {
	txn := w.wals.NewTransaction(true)
	defer txn.Discard()

	var t, i uint64
	for _, e := range es {
		t, i = e.Term, e.Index
		data, err := e.Marshal()
		if err != nil {
			return x.Wrapf(err, "wal.Store: While marshal entry")
		}
		k := w.entryKey(gid, e.Term, e.Index)
		if err := txn.Set(k, data); err == badger.ErrTxnTooBig {
			if err := txn.Commit(nil); err != nil {
				return err
			}
			txn = w.wals.NewTransaction(true)
			defer txn.Discard()
			if err := txn.Set(k, data); err != nil {
				return err
			}
		} else if err != nil {
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
		// Ideally we should be deleting entries from previous term with index >= i,
		// but to avoid complexity we remove them during reading from wal.
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
			if err := txn.Delete(newk); err == badger.ErrTxnTooBig {
				if err := txn.Commit(nil); err != nil {
					return err
				}
				txn = w.wals.NewTransaction(true)
				defer txn.Discard()
				if err := txn.Delete(newk); err != nil {
					return err
				}
			} else if err != nil {
				return err
			}
		}
	}
	return txn.Commit(nil)
}

func (w *Wal) Snapshot(gid uint32) (snap raftpb.Snapshot, rerr error) {
	err := w.wals.View(func(txn *badger.Txn) error {
		item, err := txn.Get(w.snapshotKey(gid))
		if err != nil {
			return err
		}
		val, err := item.Value()
		if err != nil {
			return err
		}
		return snap.Unmarshal(val)
	})
	if err == badger.ErrKeyNotFound {
		return snap, nil
	}
	return snap, err
}

func (w *Wal) HardState(gid uint32) (hd raftpb.HardState, rerr error) {
	err := w.wals.View(func(txn *badger.Txn) error {
		item, err := txn.Get(w.hardStateKey(gid))
		if err != nil {
			return err
		}
		val, err := item.Value()
		if err != nil {
			return err
		}
		return hd.Unmarshal(val)
	})
	if err == badger.ErrKeyNotFound {
		return hd, nil
	}
	return hd, err
}

func (w *Wal) Entries(gid uint32, fromTerm, fromIndex uint64) (es []raftpb.Entry, rerr error) {
	start := w.entryKey(gid, fromTerm, fromIndex)
	prefix := w.prefix(gid)
	err := w.wals.View(func(txn *badger.Txn) error {
		itr := txn.NewIterator(badger.DefaultIteratorOptions)
		defer itr.Close()

		var firstIndex uint64
		for itr.Seek(start); itr.ValidForPrefix(prefix); itr.Next() {
			item := itr.Item()
			var e raftpb.Entry
			val, err := item.Value()
			if err != nil {
				return err
			}
			if err = e.Unmarshal(val); err != nil {
				return err
			}
			if e.Index < fromIndex {
				continue
			}
			if firstIndex == 0 {
				firstIndex = e.Index
			}
			// When you see entry with Index i, ignore all entries with index >= i
			es = append(es[:e.Index-firstIndex], e)
		}
		return nil
	})
	return es, err
}
