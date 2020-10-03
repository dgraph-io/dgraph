/*
 * Copyright 2020 Dgraph Labs, Inc. and Contributors
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

package raftmigrate

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"sync"

	"github.com/dgraph-io/badger/v2"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/x"
	"github.com/dgraph-io/ristretto/z"
	"github.com/gogo/protobuf/proto"
	"github.com/golang/glog"
	"github.com/pkg/errors"
	"go.etcd.io/etcd/raft"
	"go.etcd.io/etcd/raft/raftpb"
	"golang.org/x/net/trace"
)

// versionKey is hardcoded into the special key used to fetch the maximum version from the DB.
const versionKey = 1

// OldDiskStorage handles disk access and writing for the RAFT write-ahead log.
type OldDiskStorage struct {
	db       *badger.DB
	commitTs uint64
	id       uint64
	gid      uint32
	elog     trace.EventLog

	cache          *sync.Map
	Closer         *z.Closer
	indexRangeChan chan indexRange
}

type indexRange struct {
	from, until uint64 // index range for deletion, until index is not deleted.
}

// Init initializes returns a properly initialized instance of oldDiskStorage.
// To gracefully shutdown oldDiskStorage, store.Closer.SignalAndWait() should be called.
func Init(db *badger.DB, id uint64, gid uint32) *OldDiskStorage {
	w := &OldDiskStorage{db: db,
		id:             id,
		gid:            gid,
		cache:          new(sync.Map),
		Closer:         z.NewCloser(1),
		indexRangeChan: make(chan indexRange, 16),
	}

	w.fetchMaxVersion()
	if prev, err := RaftId(db); err != nil || prev != id {
		x.Check(w.StoreRaftId(id))
	}
	go w.processIndexRange()

	w.elog = trace.NewEventLog("Badger", "RaftStorage")

	snap, err := w.Snapshot()
	x.Check(err)
	if !raft.IsEmptySnap(snap) {
		return w
	}

	first, err := w.FirstIndex()
	if err == errNotFound {
		ents := make([]raftpb.Entry, 1)
		x.Check(w.reset(ents))
	} else {
		x.Check(err)
	}

	// If db is not closed properly, there might be index ranges for which delete entries are not
	// inserted. So insert delete entries for those ranges starting from 0 to (first-1).
	w.indexRangeChan <- indexRange{0, first - 1}

	return w
}

// fetchMaxVersion fetches the commitTs to be used in the raftwal. The version is
// fetched from the special key "maxVersion-id" or from db.MaxVersion
// API which uses the stream framework.
func (w *OldDiskStorage) fetchMaxVersion() {
	// This is a special key that is used to fetch the latest version.
	key := []byte(fmt.Sprintf("maxVersion-%d", versionKey))

	txn := w.db.NewTransactionAt(math.MaxUint64, true)
	defer txn.Discard()

	item, err := txn.Get(key)
	if err == nil {
		w.commitTs = item.Version()
		return
	}
	if err == badger.ErrKeyNotFound {
		// We don't have the special key so get it using the MaxVersion API.
		version := w.db.MaxVersion()

		w.commitTs = version + 1
		// Insert the same key back into badger for reuse.
		x.Check(txn.Set(key, nil))
		x.Check(txn.CommitAt(w.commitTs, nil))
	} else {
		x.Check(err)
	}
}

func (w *OldDiskStorage) processIndexRange() {
	defer w.Closer.Done()

	processSingleRange := func(r indexRange) {
		if r.from == r.until {
			return
		}
		batch := w.db.NewWriteBatchAt(w.commitTs)
		if err := w.deleteRange(batch, r.from, r.until); err != nil {
			glog.Errorf("deleteRange failed with error: %v, from: %d, until: %d\n",
				err, r.from, r.until)
		}
		if err := batch.Flush(); err != nil {
			glog.Errorf("processDeleteRange batch flush failed with error: %v,\n", err)
		}
	}

loop:
	for {
		select {
		case r := <-w.indexRangeChan:
			processSingleRange(r)
		case <-w.Closer.HasBeenClosed():
			break loop
		}
	}

	// As we have already shutdown the node, it is safe to close indexRangeChan.
	// node.processApplyChan() calls CreateSnapshot, which internally sends values on this chan.
	close(w.indexRangeChan)

	for r := range w.indexRangeChan {
		processSingleRange(r)
	}
}

var idKey = []byte("raftid")

// RaftId reads the given badger store and returns the stored RAFT ID.
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

func (w *OldDiskStorage) snapshotKey() []byte {
	b := make([]byte, 14)
	binary.BigEndian.PutUint64(b[0:8], w.id)
	copy(b[8:10], "ss")
	binary.BigEndian.PutUint32(b[10:14], w.gid)
	return b
}

// HardStateKey generates the key where the hard state is stored.
func (w *OldDiskStorage) HardStateKey() []byte {
	b := make([]byte, 14)
	binary.BigEndian.PutUint64(b[0:8], w.id)
	copy(b[8:10], "hs")
	binary.BigEndian.PutUint32(b[10:14], w.gid)
	return b
}

// CheckpointKey generates the key where the checkpoint is stored.
func (w *OldDiskStorage) CheckpointKey() []byte {
	b := make([]byte, 14)
	binary.BigEndian.PutUint64(b[0:8], w.id)
	copy(b[8:10], "ck")
	binary.BigEndian.PutUint32(b[10:14], w.gid)
	return b
}

// EntryKey returns the key where the entry with the given ID is stored.
func (w *OldDiskStorage) EntryKey(idx uint64) []byte {
	b := make([]byte, 20)
	binary.BigEndian.PutUint64(b[0:8], w.id)
	binary.BigEndian.PutUint32(b[8:12], w.gid)
	binary.BigEndian.PutUint64(b[12:20], idx)
	return b
}

func (w *OldDiskStorage) parseIndex(key []byte) uint64 {
	x.AssertTrue(len(key) == 20)
	return binary.BigEndian.Uint64(key[12:20])
}

func (w *OldDiskStorage) entryPrefix() []byte {
	b := make([]byte, 12)
	binary.BigEndian.PutUint64(b[0:8], w.id)
	binary.BigEndian.PutUint32(b[8:12], w.gid)
	return b
}

func (w *OldDiskStorage) update(cb func(txn *badger.Txn) error) error {
	txn := w.db.NewTransactionAt(math.MaxUint64, true)
	defer txn.Discard()
	if err := cb(txn); err != nil {
		return err
	}
	return txn.CommitAt(w.commitTs, nil)
}

// StoreRaftId stores the given RAFT ID in disk.
func (w *OldDiskStorage) StoreRaftId(id uint64) error {
	return w.update(func(txn *badger.Txn) error {
		var b [8]byte
		binary.BigEndian.PutUint64(b[:], id)
		return txn.Set(idKey, b[:])
	})
}

// UpdateCheckpoint writes the given snapshot to disk.
func (w *OldDiskStorage) UpdateCheckpoint(snap *pb.Snapshot) error {
	return w.update(func(txn *badger.Txn) error {
		data, err := snap.Marshal()
		if err != nil {
			return err
		}
		return txn.Set(w.CheckpointKey(), data)
	})
}

// Checkpoint reads the checkpoint stored in disk and returns index stored in it.
func (w *OldDiskStorage) Checkpoint() (uint64, error) {
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
func (w *OldDiskStorage) Term(idx uint64) (uint64, error) {
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

func (w *OldDiskStorage) seekEntry(e *raftpb.Entry, seekTo uint64, reverse bool) (uint64, error) {
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
func (w *OldDiskStorage) FirstIndex() (uint64, error) {
	// We are deleting index ranges in background after taking snapshot, so we should check for last
	// snapshot in WAL(Badger) if it is not found in cache. If no snapshot is found, then we can
	// check firstKey.
	if snap, err := w.Snapshot(); err == nil && !raft.IsEmptySnap(snap) {
		return snap.Metadata.Index + 1, nil
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
func (w *OldDiskStorage) LastIndex() (uint64, error) {
	if val, ok := w.cache.Load(lastKey); ok {
		if last, ok := val.(uint64); ok {
			return last, nil
		}
	}
	return w.seekEntry(nil, math.MaxUint64, true)
}

// Delete all entries from [from, until), i.e. excluding until.
// Keep the entry at the snapshot index, for simplification of logic.
// It is the application's responsibility to not attempt to deleteRange an index
// greater than raftLog.applied.
func (w *OldDiskStorage) deleteRange(batch *badger.WriteBatch, from, until uint64) error {
	var keys []string
	err := w.db.View(func(txn *badger.Txn) error {
		opt := badger.DefaultIteratorOptions
		opt.PrefetchValues = false
		opt.Prefix = w.entryPrefix()
		itr := txn.NewIterator(opt)
		defer itr.Close()

		start := w.EntryKey(from)
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
func (w *OldDiskStorage) Snapshot() (snap raftpb.Snapshot, rerr error) {
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
func (w *OldDiskStorage) setSnapshot(batch *badger.WriteBatch, s *raftpb.Snapshot) error {
	if s == nil || raft.IsEmptySnap(*s) {
		return nil
	}
	data, err := s.Marshal()
	if err != nil {
		return errors.Wrapf(err, "wal.Store: While marshal snapshot")
	}
	if err := batch.Set(w.snapshotKey(), data); err != nil {
		return err
	}

	e := raftpb.Entry{Term: s.Metadata.Term, Index: s.Metadata.Index}
	data, err = e.Marshal()
	if err != nil {
		return err
	}
	if err := batch.Set(w.EntryKey(e.Index), data); err != nil {
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
	w.cache.Store(snapshotKey, proto.Clone(s))
	return nil
}

// SetHardState saves the current HardState.
func (w *OldDiskStorage) setHardState(batch *badger.WriteBatch, st *raftpb.HardState) error {
	if st == nil || raft.IsEmptyHardState(*st) {
		return nil
	}
	data, err := st.Marshal()
	if err != nil {
		return errors.Wrapf(err, "wal.Store: While marshal hardstate")
	}
	return batch.Set(w.HardStateKey(), data)
}

// reset resets the entries. Used for testing.
func (w *OldDiskStorage) reset(es []raftpb.Entry) error {
	w.cache = new(sync.Map) // reset cache.

	// Clean out the state.
	batch := w.db.NewWriteBatchAt(w.commitTs)
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
		if err := batch.Set(k, data); err != nil {
			return err
		}
	}
	return batch.Flush()
}

func (w *OldDiskStorage) deleteKeys(batch *badger.WriteBatch, keys []string) error {
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
func (w *OldDiskStorage) deleteFrom(batch *badger.WriteBatch, from uint64) error {
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

// HardState reads the RAFT hard state from disk and returns it.
func (w *OldDiskStorage) HardState() (hd raftpb.HardState, rerr error) {
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
func (w *OldDiskStorage) InitialState() (hs raftpb.HardState, cs raftpb.ConfState, err error) {
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

// NumEntries returns the number of entries in the write-ahead log.
func (w *OldDiskStorage) NumEntries() (int, error) {
	first, err := w.FirstIndex()
	if err != nil {
		return 0, err
	}
	var count int
	err = w.db.View(func(txn *badger.Txn) error {
		opt := badger.DefaultIteratorOptions
		opt.PrefetchValues = false
		opt.Prefix = w.entryPrefix()
		itr := txn.NewIterator(opt)
		defer itr.Close()

		start := w.EntryKey(first)
		for itr.Seek(start); itr.Valid(); itr.Next() {
			count++
		}
		return nil
	})
	return count, err
}

func (w *OldDiskStorage) allEntries(lo, hi, maxSize uint64) (es []raftpb.Entry, rerr error) {
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

		// We are opening badger in LSM only mode. In that mode the values are
		// colocated with the keys. Hence, there is no need to prefetch values.
		// Also, if Prefetch is set to true, then it causes latency issue with
		// random spikes inbetween.

		iopt := badger.DefaultIteratorOptions
		iopt.PrefetchValues = false
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
func (w *OldDiskStorage) Entries(lo, hi, maxSize uint64) (es []raftpb.Entry, rerr error) {
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

// CreateSnapshot generates a snapshot with the given ConfState and data and writes it to disk.
func (w *OldDiskStorage) CreateSnapshot(i uint64, cs *raftpb.ConfState, data []byte) error {
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

	batch := w.db.NewWriteBatchAt(w.commitTs)
	defer batch.Cancel()
	if err := w.setSnapshot(batch, &snap); err != nil {
		return err
	}

	if err := batch.Flush(); err != nil {
		return err
	}

	// deleteRange deletes all entries in the range except the last one(which is SnapshotIndex) and
	// first index is last snapshotIndex+1, hence start index for indexRange should be (first-1).
	// TODO: If deleteRangeChan is full, it might block mutations.
	w.indexRangeChan <- indexRange{first - 1, snap.Metadata.Index}
	return nil
}

// Save would write Entries, HardState and Snapshot to persistent storage in order, i.e. Entries
// first, then HardState and Snapshot if they are not empty. If persistent storage supports atomic
// writes then all of them can be written together. Note that when writing an Entry with Index i,
// any previously-persisted entries with Index >= i must be discarded.
func (w *OldDiskStorage) Save(h *raftpb.HardState, es []raftpb.Entry, snap *raftpb.Snapshot) error {
	batch := w.db.NewWriteBatchAt(w.commitTs)
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
func (w *OldDiskStorage) addEntries(batch *badger.WriteBatch, entries []raftpb.Entry) error {
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
		if err := batch.Set(k, data); err != nil {
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

// Sync calls the Sync method in the underlying badger instance to write all the contents to disk.
func (w *OldDiskStorage) Sync() error {
	return w.db.Sync()
}
