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
	"sync"

	"github.com/coreos/etcd/raft"
	pb "github.com/coreos/etcd/raft/raftpb"
	"github.com/dgraph-io/badger"
	"golang.org/x/net/trace"

	"github.com/dgraph-io/dgraph/x"
)

type txnUnifier struct {
	txn *badger.Txn
	db  *badger.DB
}

func (w *DiskStorage) newUnifier() *txnUnifier {
	return &txnUnifier{txn: w.db.NewTransaction(true), db: w.db}
}

func (u *txnUnifier) run(k, v []byte, delete bool) error {
	var err error
	if delete {
		err = u.txn.Delete(k)
	} else {
		err = u.txn.Set(k, v)
	}
	if err != badger.ErrTxnTooBig {
		// Error can be nil, and we can return here.
		return err
	}
	err = u.txn.Commit(nil)
	if err != nil {
		return err
	}
	u.txn = u.db.NewTransaction(true)
	if delete {
		return u.txn.Delete(k)
	} else {
		return u.txn.Set(k, v)
	}
	return nil
}

func (u *txnUnifier) Done() error {
	return u.txn.Commit(nil)
}

func (u *txnUnifier) Cancel() {
	u.txn.Discard()
}

type localCache struct {
	sync.RWMutex
	snap pb.Snapshot
}

func (c *localCache) setSnapshot(s pb.Snapshot) {
	c.Lock()
	defer c.Unlock()
	c.snap = s
}

func (c *localCache) snapshot() pb.Snapshot {
	c.RLock()
	defer c.RUnlock()
	return c.snap
}

type DiskStorage struct {
	db   *badger.DB
	id   uint64
	gid  uint32
	elog trace.EventLog

	cache localCache
}

func Init(db *badger.DB, id uint64, gid uint32) *DiskStorage {
	w := &DiskStorage{db: db, id: id, gid: gid}
	x.Check(w.StoreRaftId(id))
	w.elog = trace.NewEventLog("Badger", "RaftStorage")

	snap, err := w.Snapshot()
	x.Check(err)
	if !raft.IsEmptySnap(snap) {
		return w
	}

	_, err = w.FirstIndex()
	if err == errNotFound {
		ents := make([]pb.Entry, 1)
		x.Check(w.reset(ents))
	} else {
		x.Check(err)
	}
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
	if err == badger.ErrKeyNotFound {
		return 0, nil
	}
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
	w.elog.Printf("Term: %d", idx)
	defer w.elog.Printf("Done")
	first, err := w.FirstIndex()
	if err != nil {
		return 0, err
	}
	if idx < first-1 {
		return 0, raft.ErrCompacted
	}

	var e pb.Entry
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
	snap := w.cache.snapshot()
	if !raft.IsEmptySnap(snap) {
		return snap.Metadata.Index + 1, nil
	}
	index, err := w.seekEntry(nil, 0, false)
	return index + 1, err
}

// LastIndex returns the index of the last entry in the log.
func (w *DiskStorage) LastIndex() (uint64, error) {
	return w.seekEntry(nil, math.MaxUint64, true)
}

// Delete all entries from [0, until), i.e. excluding until.
// Keep the entry at the snapshot index, for simplification of logic.
// It is the application's responsibility to not attempt to deleteUntil an index
// greater than raftLog.applied.
func (w *DiskStorage) deleteUntil(u *txnUnifier, until uint64) error {
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
				if until <= index {
					return raft.ErrCompacted
				}
			}
			if index >= until {
				break
			}
			keys = append(keys, string(item.Key()))
		}
		return nil
	})
	if err != nil {
		return err
	}
	return w.deleteKeys(u, keys)
}

// Snapshot returns the most recent snapshot.
// If snapshot is temporarily unavailable, it should return ErrSnapshotTemporarilyUnavailable,
// so raft state machine could know that Storage needs some time to prepare
// snapshot and call Snapshot later.
func (w *DiskStorage) Snapshot() (snap pb.Snapshot, rerr error) {
	if s := w.cache.snapshot(); !raft.IsEmptySnap(s) {
		return s, nil
	}
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

// setSnapshot would store the snapshot. We can delete all the entries up until the snapshot
// index. But, keep the raft entry at the snapshot index, to make it easier to build the logic; like
// the dummy entry in MemoryStorage.
func (w *DiskStorage) setSnapshot(u *txnUnifier, s pb.Snapshot) error {
	if raft.IsEmptySnap(s) {
		return nil
	}
	data, err := s.Marshal()
	if err != nil {
		return x.Wrapf(err, "wal.Store: While marshal snapshot")
	}
	if err := u.run(w.snapshotKey(), data, false); err != nil {
		return err
	}

	e := pb.Entry{Term: s.Metadata.Term, Index: s.Metadata.Index}
	data, err = e.Marshal()
	if err != nil {
		return err
	}
	if err := u.run(w.entryKey(e.Index), data, false); err != nil {
		return err
	}

	// Cache it.
	w.cache.setSnapshot(s)
	return nil
}

// SetHardState saves the current HardState.
func (w *DiskStorage) setHardState(u *txnUnifier, st pb.HardState) error {
	if raft.IsEmptyHardState(st) {
		return nil
	}
	data, err := st.Marshal()
	if err != nil {
		return x.Wrapf(err, "wal.Store: While marshal hardstate")
	}
	return u.run(w.hardStateKey(), data, false)
}

// reset resets the entries. Used for testing.
func (w *DiskStorage) reset(es []pb.Entry) error {
	// Clean out the state.
	u := w.newUnifier()
	defer u.Cancel()

	if err := w.deleteFrom(u, 0); err != nil {
		return err
	}

	for _, e := range es {
		data, err := e.Marshal()
		if err != nil {
			return x.Wrapf(err, "wal.Store: While marshal entry")
		}
		k := w.entryKey(e.Index)
		if err := u.run(k, data, false); err != nil {
			return err
		}
	}
	return u.Done()
}

func (w *DiskStorage) deleteKeys(u *txnUnifier, keys []string) error {
	if len(keys) == 0 {
		return nil
	}

	for _, k := range keys {
		if err := u.run([]byte(k), nil, true); err != nil {
			return err
		}
	}
	return nil
}

// Delete entries in the range of index [from, inf).
func (w *DiskStorage) deleteFrom(u *txnUnifier, from uint64) error {
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
	return w.deleteKeys(u, keys)
}

func (w *DiskStorage) HardState() (hd pb.HardState, rerr error) {
	w.elog.Printf("HardState")
	defer w.elog.Printf("Done")
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
	w.elog.Printf("InitialState")
	defer w.elog.Printf("Done")
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

func (w *DiskStorage) NumEntries() (int, error) {
	var count int
	err := w.db.View(func(txn *badger.Txn) error {
		opt := badger.DefaultIteratorOptions
		opt.PrefetchValues = false
		itr := txn.NewIterator(opt)
		defer itr.Close()

		start := w.entryKey(0)
		prefix := w.entryPrefix()
		for itr.Seek(start); itr.ValidForPrefix(prefix); itr.Next() {
			count++
		}
		return nil
	})
	return count, err
}

func (w *DiskStorage) allEntries(lo, hi, maxSize uint64) (es []pb.Entry, rerr error) {
	err := w.db.View(func(txn *badger.Txn) error {
		if hi-lo == 1 { // We only need one entry.
			item, err := txn.Get(w.entryKey(lo))
			if err != nil {
				return err
			}
			val, err := item.Value()
			if err != nil {
				return err
			}
			var e pb.Entry
			if err = e.Unmarshal(val); err != nil {
				return err
			}
			es = append(es, e)
			return nil
		}

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
	w.elog.Printf("Entries: [%d, %d) maxSize:%d", lo, hi, maxSize)
	defer w.elog.Printf("Done")
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

func (w *DiskStorage) CreateSnapshot(i uint64, cs *pb.ConfState, data []byte) error {
	first, err := w.FirstIndex()
	if err != nil {
		return err
	}
	if i < first {
		return raft.ErrSnapOutOfDate
	}

	var e pb.Entry
	if _, err := w.seekEntry(&e, i, false); err != nil {
		return err
	}
	if e.Index != i {
		return errNotFound
	}

	var snap pb.Snapshot
	snap.Metadata.Index = i
	snap.Metadata.Term = e.Term
	if cs != nil {
		snap.Metadata.ConfState = *cs
	}
	snap.Data = data

	u := w.newUnifier()
	defer u.Cancel()
	if err := w.setSnapshot(u, snap); err != nil {
		return err
	}
	if err := w.deleteUntil(u, snap.Metadata.Index); err != nil {
		return err
	}
	return u.Done()
}

// Save would write Entries, HardState and Snapshot to persistent storage in order, i.e. Entries
// first, then HardState and Snapshot if they are not empty. If persistent storage supports atomic
// writes then all of them can be written together. Note that when writing an Entry with Index i,
// any previously-persisted entries with Index >= i must be discarded.
func (w *DiskStorage) Save(h pb.HardState, es []pb.Entry, snap pb.Snapshot) error {
	u := w.newUnifier()
	defer u.Cancel()

	if err := w.addEntries(u, es); err != nil {
		return err
	}
	if err := w.setHardState(u, h); err != nil {
		return err
	}
	if err := w.setSnapshot(u, snap); err != nil {
		return err
	}
	return u.Done()
}

// Append the new entries to storage.
func (w *DiskStorage) addEntries(u *txnUnifier, entries []pb.Entry) error {
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

	for _, e := range entries {
		k := w.entryKey(e.Index)
		data, err := e.Marshal()
		if err != nil {
			return x.Wrapf(err, "wal.Append: While marshal entry")
		}
		if err := u.run(k, data, false); err != nil {
			return err
		}
	}
	laste := entries[len(entries)-1].Index
	if laste < last {
		return w.deleteFrom(u, laste+1)
	}
	return nil
}
