/*
 * Copyright 2018 Dgraph Labs, Inc. and Contributors
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

package raftwal

import (
	"bytes"
	"encoding/binary"
	"math"
	"sync"

	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/x"
	"github.com/gogo/protobuf/proto"
	"github.com/golang/glog"
	"github.com/pkg/errors"
	"go.etcd.io/etcd/raft"
	"go.etcd.io/etcd/raft/raftpb"
	"golang.org/x/net/trace"
)

type DiskStorage struct {
	db   *badger.DB
	id   uint64
	gid  uint32
	elog trace.EventLog

	cache *sync.Map
}

func Init(db *badger.DB, id uint64, gid uint32) *DiskStorage {
	w := &DiskStorage{db: db, id: id, gid: gid, cache: new(sync.Map)}
	if prev, err := RaftId(db); err != nil || prev != id {
		x.Check(w.StoreRaftId(id))
	}
	w.elog = trace.NewEventLog("Badger", "RaftStorage")

	snap, err := w.Snapshot()
	x.Check(err)
	if !raft.IsEmptySnap(snap) {
		return w
	}

	_, err = w.FirstIndex()
	if err == errNotFound {
		ents := make([]raftpb.Entry, 1)
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
		return item.Value(func(val []byte) error {
			id = binary.BigEndian.Uint64(val)
			return nil
		})
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

func (w *DiskStorage) HardStateKey() []byte {
	b := make([]byte, 14)
	binary.BigEndian.PutUint64(b[0:8], w.id)
	copy(b[8:10], []byte("hs"))
	binary.BigEndian.PutUint32(b[10:14], w.gid)
	return b
}

func (w *DiskStorage) CheckpointKey() []byte {
	b := make([]byte, 14)
	binary.BigEndian.PutUint64(b[0:8], w.id)
	copy(b[8:10], []byte("ck"))
	binary.BigEndian.PutUint32(b[10:14], w.gid)
	return b
}

func (w *DiskStorage) EntryKey(idx uint64) []byte {
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

func (w *DiskStorage) UpdateCheckpoint(snap *pb.Snapshot) error {
	return w.db.Update(func(txn *badger.Txn) error {
		data, err := snap.Marshal()
		if err != nil {
			return err
		}
		return txn.Set(w.CheckpointKey(), data)
	})
}

func (w *DiskStorage) Checkpoint() (uint64, error) {
	var applied uint64
	err := w.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(w.CheckpointKey())
		if err == badger.ErrKeyNotFound {
			return nil
		}
		if err != nil {
			return err
		}
		return item.Value(func(val []byte) error {
			var snap pb.Snapshot
			if err := snap.Unmarshal(val); err != nil {
				return err
			}
			applied = snap.Index
			return nil
		})
	})
	return applied, err
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

	var e raftpb.Entry
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

func (w *DiskStorage) seekEntry(e *raftpb.Entry, seekTo uint64, reverse bool) (uint64, error) {
	var index uint64
	err := w.db.View(func(txn *badger.Txn) error {
		opt := badger.DefaultIteratorOptions
		opt.PrefetchValues = false
		opt.Prefix = w.entryPrefix()
		opt.Reverse = reverse
		itr := txn.NewIterator(opt)
		defer itr.Close()

		itr.Seek(w.EntryKey(seekTo))
		if !itr.Valid() {
			return errNotFound
		}
		item := itr.Item()
		index = w.parseIndex(item.Key())
		if e == nil {
			return nil
		}
		return item.Value(func(val []byte) error {
			return e.Unmarshal(val)
		})
	})
	return index, err
}

var (
	snapshotKey = "snapshot"
	firstKey    = "first"
	lastKey     = "last"
)

// FirstIndex returns the index of the first log entry that is
// possibly available via Entries (older entries have been incorporated
// into the latest Snapshot).
func (w *DiskStorage) FirstIndex() (uint64, error) {
	if val, ok := w.cache.Load(snapshotKey); ok {
		snap, ok := val.(*raftpb.Snapshot)
		if ok && !raft.IsEmptySnap(*snap) {
			return snap.Metadata.Index + 1, nil
		}
	}
	if val, ok := w.cache.Load(firstKey); ok {
		if first, ok := val.(uint64); ok {
			return first, nil
		}
	}

	// Now look into Badger.
	index, err := w.seekEntry(nil, 0, false)
	if err == nil {
		glog.V(2).Infof("Setting first index: %d", index+1)
		w.cache.Store(firstKey, index+1)
	} else if glog.V(2) {
		glog.Errorf("While seekEntry. Error: %v", err)
	}
	return index + 1, err
}

// LastIndex returns the index of the last entry in the log.
func (w *DiskStorage) LastIndex() (uint64, error) {
	if val, ok := w.cache.Load(lastKey); ok {
		if last, ok := val.(uint64); ok {
			return last, nil
		}
	}
	return w.seekEntry(nil, math.MaxUint64, true)
}

// Delete all entries from [0, until), i.e. excluding until.
// Keep the entry at the snapshot index, for simplification of logic.
// It is the application's responsibility to not attempt to deleteUntil an index
// greater than raftLog.applied.
func (w *DiskStorage) deleteUntil(batch *badger.WriteBatch, until uint64) error {
	var keys []string
	err := w.db.View(func(txn *badger.Txn) error {
		opt := badger.DefaultIteratorOptions
		opt.PrefetchValues = false
		opt.Prefix = w.entryPrefix()
		itr := txn.NewIterator(opt)
		defer itr.Close()

		start := w.EntryKey(0)
		first := true
		var index uint64
		for itr.Seek(start); itr.Valid(); itr.Next() {
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
	return w.deleteKeys(batch, keys)
}

// Snapshot returns the most recent snapshot.
// If snapshot is temporarily unavailable, it should return ErrSnapshotTemporarilyUnavailable,
// so raft state machine could know that Storage needs some time to prepare
// snapshot and call Snapshot later.
func (w *DiskStorage) Snapshot() (snap raftpb.Snapshot, rerr error) {
	if val, ok := w.cache.Load(snapshotKey); ok {
		snap, ok := val.(*raftpb.Snapshot)
		if ok && !raft.IsEmptySnap(*snap) {
			return *snap, nil
		}
	}
	err := w.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(w.snapshotKey())
		if err != nil {
			return err
		}
		return item.Value(func(val []byte) error {
			return snap.Unmarshal(val)
		})
	})
	if err == badger.ErrKeyNotFound {
		return snap, nil
	}
	return snap, err
}

// setSnapshot would store the snapshot. We can delete all the entries up until the snapshot
// index. But, keep the raft entry at the snapshot index, to make it easier to build the logic; like
// the dummy entry in MemoryStorage.
func (w *DiskStorage) setSnapshot(batch *badger.WriteBatch, s raftpb.Snapshot) error {
	if raft.IsEmptySnap(s) {
		return nil
	}
	data, err := s.Marshal()
	if err != nil {
		return errors.Wrapf(err, "wal.Store: While marshal snapshot")
	}
	if err := batch.Set(w.snapshotKey(), data, 0); err != nil {
		return err
	}

	e := raftpb.Entry{Term: s.Metadata.Term, Index: s.Metadata.Index}
	data, err = e.Marshal()
	if err != nil {
		return err
	}
	if err := batch.Set(w.EntryKey(e.Index), data, 0); err != nil {
		return err
	}

	// Update the last index cache here. This is useful so when a follower gets a jump due to
	// receiving a snapshot and Save is called, addEntries wouldn't have much. So, the last index
	// cache would need to rely upon this update here.
	if val, ok := w.cache.Load(lastKey); ok {
		le := val.(uint64)
		if le < e.Index {
			w.cache.Store(lastKey, e.Index)
		}
	}
	// Cache snapshot.
	w.cache.Store(snapshotKey, proto.Clone(&s))
	return nil
}

// SetHardState saves the current HardState.
func (w *DiskStorage) setHardState(batch *badger.WriteBatch, st raftpb.HardState) error {
	if raft.IsEmptyHardState(st) {
		return nil
	}
	data, err := st.Marshal()
	if err != nil {
		return errors.Wrapf(err, "wal.Store: While marshal hardstate")
	}
	return batch.Set(w.HardStateKey(), data, 0)
}

// reset resets the entries. Used for testing.
func (w *DiskStorage) reset(es []raftpb.Entry) error {
	w.cache = new(sync.Map) // reset cache.

	// Clean out the state.
	batch := w.db.NewWriteBatch()
	defer batch.Cancel()

	if err := w.deleteFrom(batch, 0); err != nil {
		return err
	}

	for _, e := range es {
		data, err := e.Marshal()
		if err != nil {
			return errors.Wrapf(err, "wal.Store: While marshal entry")
		}
		k := w.EntryKey(e.Index)
		if err := batch.Set(k, data, 0); err != nil {
			return err
		}
	}
	return batch.Flush()
}

func (w *DiskStorage) deleteKeys(batch *badger.WriteBatch, keys []string) error {
	if len(keys) == 0 {
		return nil
	}

	for _, k := range keys {
		if err := batch.Delete([]byte(k)); err != nil {
			return err
		}
	}
	return nil
}

// Delete entries in the range of index [from, inf).
func (w *DiskStorage) deleteFrom(batch *badger.WriteBatch, from uint64) error {
	var keys []string
	err := w.db.View(func(txn *badger.Txn) error {
		start := w.EntryKey(from)
		opt := badger.DefaultIteratorOptions
		opt.PrefetchValues = false
		opt.Prefix = w.entryPrefix()
		itr := txn.NewIterator(opt)
		defer itr.Close()

		for itr.Seek(start); itr.Valid(); itr.Next() {
			key := itr.Item().Key()
			keys = append(keys, string(key))
		}
		return nil
	})
	if err != nil {
		return err
	}
	return w.deleteKeys(batch, keys)
}

func (w *DiskStorage) HardState() (hd raftpb.HardState, rerr error) {
	w.elog.Printf("HardState")
	defer w.elog.Printf("Done")
	err := w.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(w.HardStateKey())
		if err != nil {
			return err
		}
		return item.Value(func(val []byte) error {
			return hd.Unmarshal(val)
		})
	})
	if err == badger.ErrKeyNotFound {
		return hd, nil
	}
	return hd, err
}

// InitialState returns the saved HardState and ConfState information.
func (w *DiskStorage) InitialState() (hs raftpb.HardState, cs raftpb.ConfState, err error) {
	w.elog.Printf("InitialState")
	defer w.elog.Printf("Done")
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

func (w *DiskStorage) NumEntries() (int, error) {
	var count int
	err := w.db.View(func(txn *badger.Txn) error {
		opt := badger.DefaultIteratorOptions
		opt.PrefetchValues = false
		opt.Prefix = w.entryPrefix()
		itr := txn.NewIterator(opt)
		defer itr.Close()

		start := w.EntryKey(0)
		for itr.Seek(start); itr.Valid(); itr.Next() {
			count++
		}
		return nil
	})
	return count, err
}

func (w *DiskStorage) allEntries(lo, hi, maxSize uint64) (es []raftpb.Entry, rerr error) {
	err := w.db.View(func(txn *badger.Txn) error {
		if hi-lo == 1 { // We only need one entry.
			item, err := txn.Get(w.EntryKey(lo))
			if err != nil {
				return err
			}
			return item.Value(func(val []byte) error {
				var e raftpb.Entry
				if err = e.Unmarshal(val); err != nil {
					return err
				}
				es = append(es, e)
				return nil
			})
		}

		iopt := badger.DefaultIteratorOptions
		iopt.Prefix = w.entryPrefix()
		itr := txn.NewIterator(iopt)
		defer itr.Close()

		start := w.EntryKey(lo)
		end := w.EntryKey(hi) // Not included in results.

		var size, lastIndex uint64
		first := true
		for itr.Seek(start); itr.Valid(); itr.Next() {
			item := itr.Item()
			var e raftpb.Entry
			if err := item.Value(func(val []byte) error {
				return e.Unmarshal(val)
			}); err != nil {
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
func (w *DiskStorage) Entries(lo, hi, maxSize uint64) (es []raftpb.Entry, rerr error) {
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

func (w *DiskStorage) CreateSnapshot(i uint64, cs *raftpb.ConfState, data []byte) error {
	glog.V(2).Infof("CreateSnapshot i=%d, cs=%+v", i, cs)
	first, err := w.FirstIndex()
	if err != nil {
		return err
	}
	if i < first {
		glog.Errorf("i=%d<first=%d, ErrSnapOutOfDate", i, first)
		return raft.ErrSnapOutOfDate
	}

	var e raftpb.Entry
	if _, err := w.seekEntry(&e, i, false); err != nil {
		return err
	}
	if e.Index != i {
		return errNotFound
	}

	var snap raftpb.Snapshot
	snap.Metadata.Index = i
	snap.Metadata.Term = e.Term
	x.AssertTrue(cs != nil)
	snap.Metadata.ConfState = *cs
	snap.Data = data

	batch := w.db.NewWriteBatch()
	defer batch.Cancel()
	if err := w.setSnapshot(batch, snap); err != nil {
		return err
	}
	if err := w.deleteUntil(batch, snap.Metadata.Index); err != nil {
		return err
	}
	return batch.Flush()
}

// Save would write Entries, HardState and Snapshot to persistent storage in order, i.e. Entries
// first, then HardState and Snapshot if they are not empty. If persistent storage supports atomic
// writes then all of them can be written together. Note that when writing an Entry with Index i,
// any previously-persisted entries with Index >= i must be discarded.
func (w *DiskStorage) Save(h raftpb.HardState, es []raftpb.Entry, snap raftpb.Snapshot) error {
	batch := w.db.NewWriteBatch()
	defer batch.Cancel()

	if err := w.addEntries(batch, es); err != nil {
		return err
	}
	if err := w.setHardState(batch, h); err != nil {
		return err
	}
	if err := w.setSnapshot(batch, snap); err != nil {
		return err
	}
	return batch.Flush()
}

// Append the new entries to storage.
func (w *DiskStorage) addEntries(batch *badger.WriteBatch, entries []raftpb.Entry) error {
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
	// firste can exceed last if Raft makes a jump.

	for _, e := range entries {
		k := w.EntryKey(e.Index)
		data, err := e.Marshal()
		if err != nil {
			return errors.Wrapf(err, "wal.Append: While marshal entry")
		}
		if err := batch.Set(k, data, 0); err != nil {
			return err
		}
	}
	laste := entries[len(entries)-1].Index
	w.cache.Store(lastKey, laste) // Update the last index cache.
	if laste < last {
		return w.deleteFrom(batch, laste+1)
	}
	return nil
}

func (w *DiskStorage) Sync() error {
	return w.db.Sync()
}
