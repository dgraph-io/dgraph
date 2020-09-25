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
	"encoding/binary"
	"math"
	"os"
	"path"
	"path/filepath"
	"sync"

	"github.com/dgraph-io/dgraph/x"
	"github.com/dgraph-io/ristretto/z"
	"github.com/golang/glog"
	"github.com/pkg/errors"
	"go.etcd.io/etcd/raft"
	"go.etcd.io/etcd/raft/raftpb"
	"golang.org/x/net/trace"
	"golang.org/x/sys/unix"
)

// versionKey is hardcoded into the special key used to fetch the maximum version from the DB.
const versionKey = 1

// DiskStorage handles disk access and writing for the RAFT write-ahead log.
// Dir would contain wal.meta file.
// And <start idx zero padded>.ent file.
//
// --- meta.wal wal.meta file ---
// This file should only be 4KB, so it can fit nicely in one Linux page.
// Store the raft ID in the first 8 bytes.
// wal.meta file would have the Snapshot and the HardState. First put hard state, then put Snapshot.
// Leave extra bytes in between to ensure they never overlap.
// Hardstate allocate 1KB. Rest 3KB for Snapshot. So snapshot is always accessible from offset=1024.
// Also checkpoint key goes into meta.
//
// --- <0000i>.ent files ---
// This would contain the raftpb.Entry protos. It contains term, index, type and data. No need to do
// proto.Marshal here.
// Each file can contain 10K entries.
// Term takes 8 bytes, Index takes 8 bytes, Type takes 8 bytes and Data we should store an offset to
// the actual slice, which can be 8 bytes. Total = 32 bytes.
// First 30K entries would consume 960KB.
// Pre-allocate 1MB in each file just for these entries, and zero them out explicitly. Zeroing them
// out would ensure that you'd know when these entries end, in case of a restart. In that case, the
// index would be zero, so you know that's the end.
//
// And the data for these entries are laid out starting offset=1<<20. Those are the offsets you
// store in the Entry for Data field.
// After 30K entries, you rotate the file.
//
// --- clean up ---
// If snapshot idx = Idx_s. Find the first wal.ent whose first Entry is less than Idx_s. This file
// and anything above MUST be kept. All the wal.ent files lower than this file can be deleted.
//
// --- sync ---
// Just do msync calls to sync the mmapped buffer. It would sync that to the disk.
//
// --- crashes ---
// sync would have already flushed the mmap to disk. mmap deals with process crashes just fine. So,
// we're good there. In case of file system crashes or disk crashes, we might need to replace this
// node anyway. The new node would get a new WAL.
//
type DiskStorage struct {
	dir      string
	commitTs uint64
	id       uint64
	gid      uint32
	elog     trace.EventLog

	meta    *metaFile
	entries *entryLog
	lock    sync.Mutex
}

type indexRange struct {
	from, until uint64 // index range for deletion, until index is not deleted.
}

type MetaInfo int

const (
	RaftId MetaInfo = iota
	GroupId
	CheckpointIndex
	SnapshotIndex
	SnapshotTerm
)

// getOffset returns offsets in wal.meta file.
func getOffset(info MetaInfo) int {
	switch info {
	case RaftId:
		return 0
	case GroupId:
		return 8
	case CheckpointIndex:
		return 16
	case SnapshotIndex:
		return snapshotIndex
	case SnapshotTerm:
		return snapshotIndex + 8
	default:
		panic("Invalid info" + string(info))
	}
}

// Constants to use when writing to mmap'ed meta and entry files.
const (
	// metaName is the name of the file used to store metadata (e.g raft ID, checkpoint).
	metaName = "wal.meta"
	// metaFileSize is the size of the wal.meta file.
	metaFileSize = 4 << 30
	//hardStateOffset is the offset of the hard sate within the wal.meta file.
	hardStateOffset = 512
	// snapshotIndex stores the index and term corresponding to the snapshot.
	snapshotIndex = 1024
	// snapshotOffest is the offset of the snapshot within the wal.meta file.
	snapshotOffset = snapshotIndex + 16
	// maxNumEntries is maximum number of entries before rotating the file.
	maxNumEntries = 30000
)

type mmapFile struct {
	data []byte
	fd   *os.File
}

func (m *mmapFile) sync() error {
	// TODO: Switch this to z.Msync. And probably use MS_SYNC
	return unix.Msync(m.data, unix.MS_SYNC)
}

func (m *mmapFile) slice(offset int) []byte {
	sz := binary.BigEndian.Uint32(m.data[offset:])
	start := offset + 4
	next := start + int(sz)
	if next > len(m.data) {
		return []byte{}
	}
	res := m.data[start:next]
	return res
}

func (m *mmapFile) allocateSlice(sz, offset int) ([]byte, int) {
	binary.BigEndian.PutUint32(m.data[offset:], uint32(sz))
	return m.data[offset+4 : offset+4+sz], offset + 4 + sz
}

type metaFile struct {
	*mmapFile
}

func zeroOut(dst []byte, start, end int) {
	// fmt.Printf("ZEROING out: %d -> %d. len: %d\n", start, end, len(dst))
	buf := dst[start:end]
	buf[0] = 0x00
	for i := 1; i < len(buf); i *= 2 {
		copy(buf[i:], buf[:i])
	}
}

func newMetaFile(dir string) (*metaFile, error) {
	fname := filepath.Join(dir, metaName)
	mf, err := openMmapFile(fname, os.O_RDWR|os.O_CREATE, metaFileSize)
	if err == errNewFile {
		zeroOut(mf.data, 0, snapshotOffset+4)
	} else if err != nil {
		return nil, errors.Wrapf(err, "unable to open meta file")
	}
	return &metaFile{mmapFile: mf}, nil
}

var errNewFile = errors.New("new file")

func syncDir(dir string) error {
	glog.V(2).Infof("Syncing dir: %s\n", dir)
	df, err := os.Open(dir)
	if err != nil {
		return errors.Wrapf(err, "while opening %s", dir)
	}
	x.Check(err)
	if err := df.Sync(); err != nil {
		return errors.Wrapf(err, "while syncing %s", dir)
	}
	if err := df.Close(); err != nil {
		return errors.Wrapf(err, "while closing %s", dir)
	}
	return nil
}

func openMmapFile(filename string, flag int, maxSz int) (*mmapFile, error) {
	fd, err := os.OpenFile(filename, flag, 0666)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to open: %s", filename)
	}
	fi, err := fd.Stat()
	if err != nil {
		return nil, errors.Wrapf(err, "cannot stat file: %s", filename)
	}
	fileSize := fi.Size()
	if fileSize > int64(maxSz) {
		return nil, errors.Errorf("file size %d does not match zero or max size %d",
			fileSize, maxSz)
	}
	if err := fd.Truncate(int64(maxSz)); err != nil {
		return nil, errors.Wrapf(err, "error while truncation")
	}
	buf, err := z.Mmap(fd, true, int64(maxSz)) // Mmap up to max size.
	if err != nil {
		return nil, errors.Wrapf(err, "while mmapping %s with size: %d", fd.Name(), maxSz)
	}

	err = nil
	if fileSize == 0 {
		err = errNewFile
		dir, _ := path.Split(filename)
		go func() {
			if err := syncDir(dir); err != nil {
				glog.Errorf("Error during syncDir Err: %v\n", err)
			}
		}()
	}
	return &mmapFile{
		data: buf,
		fd:   fd,
	}, err
}

func writeSlice(dst []byte, src []byte) {
	binary.BigEndian.PutUint32(dst[:4], uint32(len(src)))
	copy(dst[4:], src)
}

func allocateSlice(dst []byte, sz int) []byte {
	binary.BigEndian.PutUint32(dst[:4], uint32(sz))
	return dst[4 : 4+sz]
}

func sliceSize(dst []byte, offset int) int {
	sz := binary.BigEndian.Uint32(dst[offset:])
	return 4 + int(sz)
}

func readSlice(dst []byte, offset int) []byte {
	b := dst[offset:]
	sz := binary.BigEndian.Uint32(b)
	return b[4 : 4+sz]
}

func (m *metaFile) bufAt(info MetaInfo) []byte {
	pos := getOffset(info)
	return m.data[pos : pos+8]
}
func (m *metaFile) Uint(info MetaInfo) uint64 {
	return binary.BigEndian.Uint64(m.bufAt(info))
}
func (m *metaFile) SetUint(info MetaInfo, id uint64) {
	binary.BigEndian.PutUint64(m.bufAt(info), id)
}

func (m *metaFile) StoreHardState(hs *raftpb.HardState) error {
	if hs == nil || raft.IsEmptyHardState(*hs) {
		return nil
	}
	buf, err := hs.Marshal()
	if err != nil {
		return errors.Wrapf(err, "cannot marshal hard state")
	}
	x.AssertTrue(len(buf) < snapshotIndex-hardStateOffset)
	writeSlice(m.data[hardStateOffset:], buf)
	return nil
}

func (m *metaFile) HardState() (raftpb.HardState, error) {
	val := readSlice(m.data, hardStateOffset)
	var hs raftpb.HardState

	if len(val) == 0 {
		return hs, nil
	}
	if err := hs.Unmarshal(val); err != nil {
		return hs, errors.Wrapf(err, "cannot parse hardState")
	}
	return hs, nil
}

func (m *metaFile) StoreSnapshot(snap *raftpb.Snapshot) error {
	if snap == nil || raft.IsEmptySnap(*snap) {
		return nil
	}
	m.SetUint(SnapshotIndex, snap.Metadata.Index)
	m.SetUint(SnapshotTerm, snap.Metadata.Term)

	buf, err := snap.Marshal()
	if err != nil {
		return errors.Wrapf(err, "cannot marshal snapshot")
	}
	glog.V(1).Infof("Got valid snapshot to store of length: %d\n", len(buf))

	if len(m.data)-snapshotOffset < len(buf) {
		return errors.Errorf("Unable to store snapshot of size: %d\n", len(buf))
	}
	writeSlice(m.data[snapshotOffset:], buf)
	return nil
}

func (m *metaFile) snapshot() (raftpb.Snapshot, error) {
	val := readSlice(m.data, snapshotOffset)

	var snap raftpb.Snapshot
	if len(val) == 0 {
		return snap, nil
	}

	if err := snap.Unmarshal(val); err != nil {
		return snap, errors.Wrapf(err, "cannot parse snapshot")
	}
	return snap, nil
}

// Init initializes returns a properly initialized instance of DiskStorage.
// To gracefully shutdown DiskStorage, store.Closer.SignalAndWait() should be called.
func Init(dir string) *DiskStorage {
	w := &DiskStorage{
		dir: dir,
	}

	var err error
	w.meta, err = newMetaFile(dir)
	x.Check(err)
	// fmt.Printf("meta: %s\n", hex.Dump(w.meta.data[1024:2048]))
	// fmt.Printf("found snapshot of size: %d\n", sliceSize(w.meta.data, snapshotOffset))

	w.entries, err = openEntryLog(dir)
	x.Check(err)

	w.elog = trace.NewEventLog("Badger", "RaftStorage")

	snap, err := w.meta.snapshot()
	x.Check(err)

	first, _ := w.FirstIndex()
	if !raft.IsEmptySnap(snap) {
		x.AssertTruef(snap.Metadata.Index+1 == first,
			"snap index: %d + 1 should be equal to first: %d\n", snap.Metadata.Index, first)
	}

	// If db is not closed properly, there might be index ranges for which delete entries are not
	// inserted. So insert delete entries for those ranges starting from 0 to (first-1).
	if err := w.entries.deleteBefore(first - 1); err != nil {
		glog.Errorf("while deleting before: %d, err: %v\n", first-1, err)
	}
	last := w.entries.LastIndex()

	glog.Infof("Init Raft Storage with snap: %d, first: %d, last: %d\n",
		snap.Metadata.Index, first, last)
	return w
}

func (w *DiskStorage) SetUint(info MetaInfo, id uint64) {
	w.meta.SetUint(info, id)
}
func (w *DiskStorage) Uint(info MetaInfo) uint64 {
	return w.meta.Uint(info)
}

var errNotFound = errors.New("Unable to find raft entry")

// reset resets the entries. Used for testing.
func (w *DiskStorage) reset(es []raftpb.Entry) error {
	// Clean out the state.
	if err := w.entries.reset(); err != nil {
		return err
	}
	return w.addEntries(es)
}

func (w *DiskStorage) HardState() (raftpb.HardState, error) {
	if w.meta == nil {
		return raftpb.HardState{}, errors.Errorf("uninitialized meta file")
	}
	return w.meta.HardState()
}
func (w *DiskStorage) Checkpoint() (uint64, error) {
	if w.meta == nil {
		return 0, errors.Errorf("uninitialized meta file")
	}
	return w.meta.Uint(CheckpointIndex), nil
}

// Implement the Raft.Storage interface.
// -------------------------------------

// InitialState returns the saved HardState and ConfState information.
func (w *DiskStorage) InitialState() (hs raftpb.HardState, cs raftpb.ConfState, err error) {
	w.lock.Lock()
	defer w.lock.Unlock()

	w.elog.Printf("InitialState")
	defer w.elog.Printf("Done")
	hs, err = w.meta.HardState()
	if err != nil {
		return
	}
	var snap raftpb.Snapshot
	snap, err = w.meta.snapshot()
	if err != nil {
		return
	}
	return hs, snap.Metadata.ConfState, nil
}

func (w *DiskStorage) NumEntries() int {
	w.lock.Lock()
	defer w.lock.Unlock()

	start := w.entries.firstIndex()

	var count int
	for {
		ents := w.entries.allEntries(start, math.MaxUint64, 64<<20)
		if len(ents) == 0 {
			return count
		}
		count += len(ents)
		start = ents[len(ents)-1].Index + 1
	}
}

// Entries returns a slice of log entries in the range [lo,hi).
// MaxSize limits the total size of the log entries returned, but
// Entries returns at least one entry if any.
func (w *DiskStorage) Entries(lo, hi, maxSize uint64) (es []raftpb.Entry, rerr error) {
	// glog.Infof("Entries: [%d, %d) maxSize:%d", lo, hi, maxSize)
	w.lock.Lock()
	defer w.lock.Unlock()

	// glog.Infof("Entries after lock: [%d, %d) maxSize:%d", lo, hi, maxSize)

	first := w.entries.firstIndex()
	if lo < first {
		glog.Errorf("lo: %d <first: %d\n", lo, first)
		return nil, raft.ErrCompacted
	}

	last := w.entries.LastIndex()
	if hi > last+1 {
		glog.Errorf("hi: %d > last+1: %d\n", hi, last+1)
		return nil, raft.ErrUnavailable
	}

	ents := w.entries.allEntries(lo, hi, maxSize)
	// glog.Infof("got entries [%d, %d): %+v\n", lo, hi, ents)
	return ents, nil
}

func (w *DiskStorage) Term(idx uint64) (uint64, error) {
	w.lock.Lock()
	defer w.lock.Unlock()

	first := w.firstIndex()
	if idx < first-1 {
		glog.Errorf("TERM for %d = %v\n", idx, raft.ErrCompacted)
		return 0, raft.ErrCompacted
	}

	term, err := w.entries.Term(idx)
	if err != nil {
		glog.Errorf("TERM for %d = %v\n", idx, err)
	}
	// glog.Errorf("Got term: %d for index: %d\n", term, idx)
	return term, err
}

func (w *DiskStorage) LastIndex() (uint64, error) {
	w.lock.Lock()
	defer w.lock.Unlock()

	li := w.entries.LastIndex()
	si := w.meta.Uint(SnapshotIndex)
	if li < si {
		return si, nil
	}
	return li, nil
}

func (w *DiskStorage) firstIndex() uint64 {
	// We are deleting index ranges in background after taking snapshot, so we should check for last
	// snapshot in WAL(Badger) if it is not found in cache. If no snapshot is found, then we can
	// check firstKey.
	if si := w.Uint(SnapshotIndex); si > 0 {
		return si + 1
	}
	return w.entries.firstIndex()
}

// FirstIndex returns the index of the first log entry that is
// possibly available via Entries (older entries have been incorporated
// into the latest Snapshot).
func (w *DiskStorage) FirstIndex() (uint64, error) {
	w.lock.Lock()
	defer w.lock.Unlock()

	return w.firstIndex(), nil
}

// Snapshot returns the most recent snapshot.
// If snapshot is temporarily unavailable, it should return ErrSnapshotTemporarilyUnavailable,
// so raft state machine could know that Storage needs some time to prepare
// snapshot and call Snapshot later.
func (w *DiskStorage) Snapshot() (raftpb.Snapshot, error) {
	w.lock.Lock()
	defer w.lock.Unlock()

	return w.meta.snapshot()
}

// ---------------- Raft.Storage interface complete.

// CreateSnapshot generates a snapshot with the given ConfState and data and writes it to disk.
func (w *DiskStorage) CreateSnapshot(i uint64, cs *raftpb.ConfState, data []byte) error {
	glog.V(2).Infof("CreateSnapshot i=%d, cs=%+v", i, cs)

	w.lock.Lock()
	defer w.lock.Unlock()

	first := w.firstIndex()
	if i < first {
		glog.Errorf("i=%d<first=%d, ErrSnapOutOfDate", i, first)
		return raft.ErrSnapOutOfDate
	}

	e, err := w.entries.seekEntry(i)
	if err != nil {
		return err
	}

	var snap raftpb.Snapshot
	snap.Metadata.Index = i
	snap.Metadata.Term = e.Term()
	x.AssertTrue(cs != nil)
	snap.Metadata.ConfState = *cs
	snap.Data = data

	if err := w.meta.StoreSnapshot(&snap); err != nil {
		return err
	}
	// Now we delete all the files which are below the snapshot index.
	return w.entries.deleteBefore(snap.Metadata.Index)
}

// Save would write Entries, HardState and Snapshot to persistent storage in order, i.e. Entries
// first, then HardState and Snapshot if they are not empty. If persistent storage supports atomic
// writes then all of them can be written together. Note that when writing an Entry with Index i,
// any previously-persisted entries with Index >= i must be discarded.
func (w *DiskStorage) Save(h *raftpb.HardState, es []raftpb.Entry, snap *raftpb.Snapshot) error {
	w.lock.Lock()
	defer w.lock.Unlock()

	if err := w.entries.AddEntries(es); err != nil {
		return err
	}
	if err := w.meta.StoreHardState(h); err != nil {
		return err
	}
	if err := w.meta.StoreSnapshot(snap); err != nil {
		return err
	}
	return nil
}

// Append the new entries to storage.
func (w *DiskStorage) addEntries(entries []raftpb.Entry) error {
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

	// AddEntries would zero out all the entries starting entries[0].Index before writing.
	if err := w.entries.AddEntries(entries); err != nil {
		return errors.Wrapf(err, "while adding entries")
	}
	return nil
}

// Sync calls the Sync method in the underlying badger instance to write all the contents to disk.
func (w *DiskStorage) Sync() error {
	if err := w.meta.sync(); err != nil {
		return errors.Wrapf(err, "while syncing meta")
	}
	if err := w.entries.current.sync(); err != nil {
		return errors.Wrapf(err, "while syncing current file")
	}
	return nil
}

func (w *DiskStorage) Close() error {
	return w.Sync()
}
