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
	pb "github.com/coreos/etcd/raft/raftpb"
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

func (w *DiskStorage) parseIndex(key []byte) uint64 {
	x.AssertTrue(len(key) == 20)
	return binary.BigEndian.Uint64(key[12:20])
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
	var e pb.Entry
	if idx <= snap.Metadata.Index {
		return 0, raft.ErrCompacted
	}

	if _, err := w.seekEntry(&e, idx, false); err == errNotFound {
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

func (w *DiskStorage) seekEntry(e *pb.Entry, seekTo uint64, reverse bool) (uint64, error) {
	var index uint64
	err := w.db.View(func(txn *badger.Txn) error {
		opt := badger.DefaultIteratorOptions
		opt.PrefetchValues = false
		opt.Reverse = reverse
		itr := txn.NewIterator(opt)
		defer itr.Close()

		itr.Seek(w.entryKey(seekTo))
		if !itr.ValidForPrefix(w.entryPrefix()) {
			return errNotFound
		}
		index = w.parseIndex(itr.Item().Key())
		if e == nil {
			return nil
		}
		item := itr.Item()
		val, err := item.Value()
		if err != nil {
			return err
		}
		return e.Unmarshal(val)
	})
	return index, err
}

// FirstIndex returns the index of the first log entry that is
// possibly available via Entries (older entries have been incorporated
// into the latest Snapshot).
func (w *DiskStorage) FirstIndex() (uint64, error) {
	index, err := w.seekEntry(nil, 0, false)
	return index + 1, err
}

// LastIndex returns the index of the last entry in the log.
func (w *DiskStorage) LastIndex() (uint64, error) {
	return w.seekEntry(nil, math.MaxUint64, true)
}

// Compact discards all log entries prior to compactIndex. It would keep the entry at the
// compactIndex.
// Delete all entries before this snapshot to save disk space.
// Keep the entry at the snapshot index, for simplification of logic.
// It is the application's responsibility to not attempt to compact an index
// greater than raftLog.applied.
func (w *DiskStorage) Compact(compactIndex uint64) error {
	var keys []string
	err := w.db.View(func(txn *badger.Txn) error {
		opt := badger.DefaultIteratorOptions
		opt.PrefetchValues = false
		itr := txn.NewIterator(opt)
		defer itr.Close()

		start := w.entryKey(0)
		prefix := w.entryPrefix()
		first := true
		var index uint64
		for itr.Seek(start); itr.ValidForPrefix(prefix); itr.Next() {
			item := itr.Item()
			index = w.parseIndex(item.Key())
			if first {
				first = false
				if compactIndex <= index {
					return raft.ErrCompacted
				}
			}
			if index >= compactIndex {
				break
			}
			keys = append(keys, string(item.Key()))
		}
		return nil
	})
	if err != nil {
		return err
	}
	return w.deleteKeys(keys)
}

// setSnapshot would store the snapshot. We can delete all the entries up until the snapshot
// index. But, keep the raft entry at the snapshot index, to make it easier to build the logic; like
// the dummy entry in MemoryStorage.
func (w *DiskStorage) setSnapshot(s pb.Snapshot) error {
	data, err := s.Marshal()
	if err != nil {
		return x.Wrapf(err, "wal.Store: While marshal snapshot")
	}
	return w.db.Update(func(txn *badger.Txn) error {
		return txn.Set(w.snapshotKey(), data)
	})
}

// ApplySnapshot overwrites the contents of this Storage object with
// those of the given snapshot.
func (w *DiskStorage) ApplySnapshot(snap pb.Snapshot) error {
	prev, err := w.Snapshot()
	if err != nil {
		return err
	}
	if prev.Metadata.Index >= snap.Metadata.Index {
		return raft.ErrSnapOutOfDate
	}
	if err := w.setSnapshot(snap); err != nil {
		return err
	}
	return w.deleteEntries(snap.Metadata.Index + 1)
}

// SetHardState saves the current HardState.
func (w *DiskStorage) SetHardState(st pb.HardState) error {
	return w.db.Update(func(txn *badger.Txn) error {
		data, err := st.Marshal()
		if err != nil {
			return x.Wrapf(err, "wal.Store: While marshal hardstate")
		}
		return txn.Set(w.hardStateKey(), data)
	})
}

// Store stores the hardstate and entries for a given RAFT group.
func (w *DiskStorage) Store(h pb.HardState, es []pb.Entry) error {
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

func (w *DiskStorage) deleteKeys(keys []string) error {
	if len(keys) == 0 {
		return nil
	}

	txn := w.db.NewTransaction(true)
	defer txn.Discard()
	for _, k := range keys {
		err := txn.Delete([]byte(k))
		if err == badger.ErrTxnTooBig {
			if err := txn.Commit(nil); err != nil {
				return err
			}
			txn = w.db.NewTransaction(true)
			defer txn.Discard()
			if err := txn.Delete([]byte(k)); err != nil {
				return err
			}
		}
		if err != nil {
			return err
		}
	}
	return txn.Commit(nil)
}

// Delete entries in the range of index [from, inf).
func (w *DiskStorage) deleteEntries(from uint64) error {
	var keys []string
	err := w.db.View(func(txn *badger.Txn) error {
		start := w.entryKey(from)
		prefix := w.entryPrefix()
		opt := badger.DefaultIteratorOptions
		opt.PrefetchValues = false
		itr := txn.NewIterator(opt)
		defer itr.Close()

		for itr.Seek(start); itr.ValidForPrefix(prefix); itr.Next() {
			key := itr.Item().Key()
			keys = append(keys, string(key))
		}
		return nil
	})
	if err != nil {
		return err
	}
	return w.deleteKeys(keys)
}

// Snapshot returns the most recent snapshot.
// If snapshot is temporarily unavailable, it should return ErrSnapshotTemporarilyUnavailable,
// so raft state machine could know that Storage needs some time to prepare
// snapshot and call Snapshot later.
func (w *DiskStorage) Snapshot() (snap pb.Snapshot, rerr error) {
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

func (w *DiskStorage) HardState() (hd pb.HardState, rerr error) {
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
func (w *DiskStorage) InitialState() (hs pb.HardState, cs pb.ConfState, err error) {
	hs, err = w.HardState()
	if err != nil {
		return
	}
	var snap pb.Snapshot
	snap, err = w.Snapshot()
	if err != nil {
		return
	}
	return hs, snap.Metadata.ConfState, nil
}

func (w *DiskStorage) allEntries(lo, hi, maxSize uint64) (es []pb.Entry, rerr error) {
	err := w.db.View(func(txn *badger.Txn) error {
		itr := txn.NewIterator(badger.DefaultIteratorOptions)
		defer itr.Close()

		start := w.entryKey(lo)
		end := w.entryKey(hi) // Not included in results.
		prefix := w.entryPrefix()

		var size, lastIndex uint64
		first := true
		for itr.Seek(start); itr.ValidForPrefix(prefix); itr.Next() {
			item := itr.Item()
			var e pb.Entry
			val, err := item.Value()
			if err != nil {
				return err
			}
			if err = e.Unmarshal(val); err != nil {
				return err
			}
			// If this Assert does not fail, then we can safely remove that strange append fix
			// below.
			x.AssertTrue(e.Index > lastIndex && e.Index >= lo)
			lastIndex = e.Index
			if bytes.Compare(item.Key(), end) >= 0 {
				break
			}
			size += uint64(e.Size())
			if size > maxSize && !first {
				break
			}
			es = append(es, e)
			first = false
		}
		return nil
	})
	return es, err
}

// Entries returns a slice of log entries in the range [lo,hi).
// MaxSize limits the total size of the log entries returned, but
// Entries returns at least one entry if any.
func (w *DiskStorage) Entries(lo, hi, maxSize uint64) (es []pb.Entry, rerr error) {
	first, err := w.FirstIndex()
	if err != nil {
		return es, err
	}
	if lo < first {
		return nil, raft.ErrCompacted
	}

	last, err := w.LastIndex()
	if err != nil {
		return es, err
	}
	if hi > last+1 {
		return nil, raft.ErrUnavailable
	}

	return w.allEntries(lo, hi, maxSize)
}

func limitSize(ents []pb.Entry, maxSize uint64) []pb.Entry {
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

func (w *DiskStorage) CreateSnapshot(i uint64, cs *pb.ConfState, data []byte) (pb.Snapshot, error) {
	first, err := w.FirstIndex()
	if err != nil {
		return pb.Snapshot{}, err
	}
	if i < first {
		return pb.Snapshot{}, raft.ErrSnapOutOfDate
	}

	var e pb.Entry
	if _, err := w.seekEntry(&e, i, false); err != nil {
		return pb.Snapshot{}, err
	}
	if e.Index != i {
		return pb.Snapshot{}, errNotFound
	}

	var snap pb.Snapshot
	snap.Metadata.Index = i
	snap.Metadata.Term = e.Term
	if cs != nil {
		snap.Metadata.ConfState = *cs
	}
	snap.Data = data
	return snap, w.setSnapshot(snap)
}

// Append the new entries to storage.
func (w *DiskStorage) Append(entries []pb.Entry) error {
	if len(entries) == 0 {
		return nil
	}

	first, err := w.FirstIndex()
	if err != nil {
		return err
	}
	firste := entries[0].Index
	if firste+uint64(len(entries))-1 < first {
		// All of these entries have already been compacted.
		return nil
	}
	if first > firste {
		// Truncate compacted entries
		entries = entries[first-firste:]
	}

	last, err := w.LastIndex()
	if err != nil {
		return err
	}
	x.AssertTruef(firste <= last+1, "firste: %d. last: %d", firste, last)

	txn := w.db.NewTransaction(true)
	defer txn.Discard()
	for _, e := range entries {
		k := w.entryKey(e.Index)
		data, err := e.Marshal()
		if err != nil {
			return x.Wrapf(err, "wal.Append: While marshal entry")
		}
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
	if err := txn.Commit(nil); err != nil {
		return err
	}
	laste := entries[len(entries)-1].Index
	if laste < last {
		return w.deleteEntries(laste + 1)
	}
	return nil
}
