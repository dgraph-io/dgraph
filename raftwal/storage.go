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
	"errors"
	"math"

	"github.com/coreos/etcd/raft"
	"github.com/coreos/etcd/raft/raftpb"
	"github.com/dgraph-io/badger"

	"github.com/dgraph-io/dgraph/x"
)

type DiskStorage struct {
	db  *badger.DB
	id  uint64
	gid uint32
}

func Init(db *badger.DB, id uint64, gid uint32) *DiskStorage {
	w := &DiskStorage{db: db, id: id, gid: gid}
	x.Check(w.StoreRaftId(id))
	return w
}

var idKey = []byte("raftid")

func RaftId(db *badger.DB) (uint64, error) {
	var id uint64
	err := db.View(func(txn *badger.Txn) error {
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

func (w *DiskStorage) snapshotKey() []byte {
	b := make([]byte, 14)
	binary.BigEndian.PutUint64(b[0:8], w.id)
	copy(b[8:10], []byte("ss"))
	binary.BigEndian.PutUint32(b[10:14], w.gid)
	return b
}

func (w *DiskStorage) hardStateKey() []byte {
	b := make([]byte, 14)
	binary.BigEndian.PutUint64(b[0:8], w.id)
	copy(b[8:10], []byte("hs"))
	binary.BigEndian.PutUint32(b[10:14], w.gid)
	return b
}

func (w *DiskStorage) entryKey(idx uint64) []byte {
	b := make([]byte, 20)
	binary.BigEndian.PutUint64(b[0:8], w.id)
	binary.BigEndian.PutUint32(b[8:12], w.gid)
	binary.BigEndian.PutUint64(b[12:20], idx)
	return b
}

func (w *DiskStorage) entryPrefix() []byte {
	b := make([]byte, 12)
	binary.BigEndian.PutUint64(b[0:8], w.id)
	binary.BigEndian.PutUint32(b[8:12], w.gid)
	return b
}

func (w *DiskStorage) StoreRaftId(id uint64) error {
	return w.db.Update(func(txn *badger.Txn) error {
		var b [8]byte
		binary.BigEndian.PutUint64(b[:], id)
		return txn.Set(idKey, b[:])
	})
}

// Term returns the term of entry i, which must be in the range
// [FirstIndex()-1, LastIndex()]. The term of the entry before
// FirstIndex is retained for matching purposes even though the
// rest of that entry may not be available.
func (w *DiskStorage) Term(idx uint64) (uint64, error) {
	snap, err := w.Snapshot()
	if err != nil {
		return 0, err
	}
	var e raftpb.Entry
	if idx <= snap.Metadata.Index {
		return 0, raft.ErrCompacted
	}

	if err := w.seekEntry(&e, idx, false); err == errNotFound {
		return 0, raft.ErrUnavailable
	} else if err != nil {
		return 0, err
	}
	if idx < e.Index {
		return 0, raft.ErrCompacted
	}
	return e.Term, nil
}

var errNotFound = errors.New("Unable to find raft entry")

// TODO: We could optimize this by not looking up value, and parsing the key only.
func (w *DiskStorage) seekEntry(e *raftpb.Entry, seekTo uint64, reverse bool) error {
	return w.db.View(func(txn *badger.Txn) error {
		opt := badger.DefaultIteratorOptions
		opt.PrefetchValues = false
		opt.Reverse = reverse
		itr := txn.NewIterator(opt)
		defer itr.Close()

		itr.Seek(w.entryKey(seekTo))
		if !itr.ValidForPrefix(w.entryPrefix()) {
			return errNotFound
		}
		item := itr.Item()
		val, err := item.Value()
		if err != nil {
			return err
		}
		return e.Unmarshal(val)
	})
}

// FirstIndex returns the index of the first log entry that is
// possibly available via Entries (older entries have been incorporated
// into the latest Snapshot).
func (w *DiskStorage) FirstIndex() (uint64, error) {
	var e raftpb.Entry
	if err := w.seekEntry(&e, 0, false); err != nil {
		return 0, err
	}
	return e.Index, nil
}

// LastIndex returns the index of the last entry in the log.
func (w *DiskStorage) LastIndex() (uint64, error) {
	var e raftpb.Entry
	if err := w.seekEntry(&e, math.MaxUint64, true); err != nil {
		return 0, err
	}
	return e.Index, nil
}

func (w *DiskStorage) StoreSnapshot(s raftpb.Snapshot) error {
	if raft.IsEmptySnap(s) {
		return nil
	}
	data, err := s.Marshal()
	if err != nil {
		return x.Wrapf(err, "wal.Store: While marshal snapshot")
	}
	txn := w.db.NewTransaction(true)
	defer txn.Discard()
	if err := txn.Set(w.snapshotKey(), data); err != nil {
		return err
	}

	// Delete all entries before this snapshot to save disk space.
	start := w.entryKey(0)
	last := w.entryKey(s.Metadata.Index)
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
			txn = w.db.NewTransaction(true)
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
func (w *DiskStorage) Store(h raftpb.HardState, es []raftpb.Entry) error {
	txn := w.db.NewTransaction(true)
	defer txn.Discard()

	var t, i uint64
	for _, e := range es {
		t, i = e.Term, e.Index
		data, err := e.Marshal()
		if err != nil {
			return x.Wrapf(err, "wal.Store: While marshal entry")
		}
		k := w.entryKey(e.Index)
		if err := txn.Set(k, data); err == badger.ErrTxnTooBig {
			if err := txn.Commit(nil); err != nil {
				return err
			}
			txn = w.db.NewTransaction(true)
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
		if err := txn.Set(w.hardStateKey(), data); err != nil {
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
		start := w.entryKey(i + 1)
		prefix := w.entryPrefix()
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
				txn = w.db.NewTransaction(true)
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

// Snapshot returns the most recent snapshot.
// If snapshot is temporarily unavailable, it should return ErrSnapshotTemporarilyUnavailable,
// so raft state machine could know that Storage needs some time to prepare
// snapshot and call Snapshot later.
func (w *DiskStorage) Snapshot() (snap raftpb.Snapshot, rerr error) {
	err := w.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(w.snapshotKey())
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

func (w *DiskStorage) HardState() (hd raftpb.HardState, rerr error) {
	err := w.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(w.hardStateKey())
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

// InitialState returns the saved HardState and ConfState information.
func (w *DiskStorage) InitialState() (hs raftpb.HardState, cs raftpb.ConfState, err error) {
	hs, err = w.HardState()
	if err != nil {
		return
	}
	var snap raftpb.Snapshot
	snap, err = w.Snapshot()
	if err != nil {
		return
	}
	return hs, snap.Metadata.ConfState, nil
}

// Entries returns a slice of log entries in the range [lo,hi).
// MaxSize limits the total size of the log entries returned, but
// Entries returns at least one entry if any.
func (w *DiskStorage) Entries(lo, hi, maxSize uint64) (es []raftpb.Entry, rerr error) {
	err := w.db.View(func(txn *badger.Txn) error {
		itr := txn.NewIterator(badger.DefaultIteratorOptions)
		defer itr.Close()

		start := w.entryKey(lo)
		end := w.entryKey(hi) // Not included in results.
		prefix := w.entryPrefix()

		var firstIndex, lastIndex uint64
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
			if firstIndex == 0 {
				firstIndex = e.Index
			}
			// If this Assert does not fail, then we can safely remove that strange append fix
			// below.
			x.AssertTrue(e.Index > lastIndex && e.Index >= lo)
			lastIndex = e.Index
			if bytes.Compare(item.Key(), end) >= 0 {
				break
			}
			// When you see entry with Index i, ignore all entries with index >= i
			//
			// TODO: Do we still need this weird fix? We're no longer storing
			// terms in the entry keys, so the entries would automatically get
			// removed when storing anyway.
			es = append(es[:e.Index-firstIndex], e)
		}
		if firstIndex == 0 || lo < firstIndex {
			return raft.ErrCompacted
		}
		if hi > lastIndex {
			return raft.ErrUnavailable
		}
		return nil
	})
	// TODO: This could be optimized to do a rough processing within.
	es = limitSize(es, maxSize)
	return es, err
}

func limitSize(ents []raftpb.Entry, maxSize uint64) []raftpb.Entry {
	if len(ents) == 0 {
		return ents
	}
	size := ents[0].Size()
	var limit int
	for limit = 1; limit < len(ents); limit++ {
		size += ents[limit].Size()
		if uint64(size) > maxSize {
			break
		}
	}
	return ents[:limit]
}
