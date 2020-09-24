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
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unsafe"

	"github.com/dgraph-io/badger/v2"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/x"
	"github.com/dgraph-io/ristretto/z"
	"github.com/pkg/errors"
	"go.etcd.io/etcd/raft"
	"go.etcd.io/etcd/raft/raftpb"
	"golang.org/x/net/trace"
	"google.golang.org/protobuf/proto"
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
	db       *badger.DB
	dir      string
	commitTs uint64
	id       uint64
	gid      uint32
	elog     trace.EventLog

	meta    *metaFile
	entries *entryLog

	cache          *sync.Map
	Closer         *z.Closer
	indexRangeChan chan indexRange
}

type indexRange struct {
	from, until uint64 // index range for deletion, until index is not deleted.
}

// Constants to use when writing to mmap'ed meta and entry files.
const (
	// metaName is the name of the file used to store metadata (e.g raft ID, checkpoint).
	metaName = "wal.meta"
	// metaFileSize is the size of the wal.meta file (4KB).
	metaFileSize = 4 << 30
	// raftIdOffset is the offset of the raft ID within the wal.meta file.
	raftIdOffset = 0
	// checkpointOffset is the offset of the checkpoint within the wal.meta file.
	checkpointOffset = 8
	//hardStateOffset is the offset of the hard sate within the wal.meta file.
	hardStateOffset = 512
	// snapshotOffest is the offset of the snapshot within the wal.meta file.
	snapshotOffset = 1024
	// maxNumEntries is maximum number of entries before rotating the file.
	maxNumEntries = 30000
	// entryFileOffset
	entryFileOffset = 1 << 20 // 1MB
	// entryFileSize is the initial size of the entry file.
	entryFileSize = 4 * entryFileOffset // 4MB
	// entryFileMaxSize is the maximum size allowed for an entry file.
	entryFileMaxSize = 1 << 30 // 1GB

)

var (
	// entrySize is the size in bytes of a single entry.
	entrySize  = int(unsafe.Sizeof(entry{}))
	emptyEntry = entry(make([]byte, entrySize))
)

type mmapFile struct {
	data   []byte
	fd     *os.File
	offset int64
}

type metaFile struct {
	*mmapFile
}

func zeroOut(buf []byte, start, end int) {
	buf[start] = 0x00
	for i := start + 1; i < end; i *= 2 {
		copy(buf[i:], buf[:i])
	}
}

func newMetaFile(dir string) (*metaFile, error) {
	fname := filepath.Join(dir, metaName)
	mf, err := openMmapFile(fname, os.O_RDWR|os.O_CREATE, metaFileSize)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to open meta file")
	}
	if mf.offset == 0 {
		zeroOut(mf.data, 0, snapshotOffset+4)
	}
	return &metaFile{mmapFile: mf}, nil
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
	return &mmapFile{
		data:   buf,
		fd:     fd,
		offset: fileSize,
	}, nil
}

func writeSlice(dst []byte, src []byte) {
	binary.BigEndian.PutUint32(dst[:4], uint32(len(src)))
	copy(dst[4:], src)
}

func allocateSlice(dst []byte, sz int) {
	binary.BigEndian.PutUint32(dst[:4], uint32(sz))
	return dst[4 : 4+sz]
}

func readSlice(dst []byte, offset int) []byte {
	b := dst[offset:]
	sz := binary.BigEndian.Uint32(b)
	return b[4 : 4+sz]
}

func (m *metaFile) raftBuf() []byte {
	return m.data[raftIdOffset : raftIdOffset+8]
}

func (m *metaFile) RaftId() uint64 {
	return binary.BigEndian.Uint64(m.raftBuf())
}

func (m *metaFile) StoreRaftId(id uint64) {
	binary.BigEndian.PutUint64(m.raftBuf(), id)
}

func (m *metaFile) UpdateCheckpoint(index uint64) {
	binary.BigEndian.PutUint64(m.data[checkpointOffset:], index)
}

func (m *metaFile) Checkpoint() uint64 {
	return binary.BigEndian.Uint64(m.data[checkpointOffset:])
}

func (m *metaFile) StoreHardState(hs *raftpb.HardState) error {
	if hs == nil || raft.IsEmptyHardState(*hs) {
		return nil
	}
	buf, err := hs.Marshal()
	if err != nil {
		return errors.Wrapf(err, "cannot marshal hard state")
	}
	x.AssertTrue(len(buf) < snapshotOffset-hardStateOffset)
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
	buf, err := snap.Marshal()
	if err != nil {
		return errors.Wrapf(err, "cannot marshal snapshot")
	}
	if len(m.data)-snapshotOffset < len(buf) {
		return fmt.Errorf("Unable to store snapshot of size: %d\n", len(buf))
	}
	writeSlice(m.data[snapshotOffset:], buf)
	return nil
}

func (m *metaFile) Snapshot() (raftpb.Snapshot, error) {
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

type entry []byte

// type entry struct {
// 	Term       uint64
// 	Index      uint64
// 	DataOffset uint64
// 	Type       raftpb.EntryType
// }

func (e entry) Term() uint64 {
	return binary.BigEndian.Uint64(e)
}
func (e entry) Index() uint64 {
	return binary.BigEndian.Uint64(e[8:])
}
func (e entry) DataOffset() uint64 {
	return binary.BigEndian.Uint64(e[16:])
}
func (e entry) Type() uint64 {
	return binary.BigEndian.Uint64(e[24:])
}

func marshalEntry(b []byte, term, index, do, typ uint64) {
	if e == nil {
		return
	}
	x.AssertTrue(len(b) == entrySize)

	binary.BigEndian.PutUint64(b, term)
	binary.BigEndian.PutUint64(b[8:], index)
	binary.BigEndian.PutUint64(b[16:], do)
	binary.BigEndian.PutUint64(b[24:], typ)
}

// entryFile represents a single log file.
type entryFile struct {
	*mmapFile
	fid int64
}

// func (ef *entryFile) readAt(offset uint32) []byte {
// 	return ef.mmap[offset:]
// }
func getEntryFile(path string) (*entryFile, error) {
	mf, err := openMmapFile(path, os.O_RDWR|os.O_CREATE, 16<<30)
	if err != nil {
		return nil, err
	}
	if mf.offset == 0 {
		zeroOut(mf.data, 0, entryFileOffset)
	}
	ef := &entryFile{
		mmapFile: mf,
	}
	return ef, nil
}

func getEntryFiles(dir string) ([]*entryFile, error) {
	entryFiles := x.WalkPathFunc(dir, func(path string, isDir bool) bool {
		if isDir {
			return false
		}
		if strings.HasSuffix(path, ".ent") {
			return true
		}
		return false
	})

	files := make([]*entryFile, 0)
	for _, path := range entryFiles {
		fid, err := strconv.ParseInt(strings.Split(".ent", path)[0], 10, 64)
		if err != nil {
			return nil, errors.Wrapf(err, "while parsing: %s", path)
		}
		f, err := getEntryFile(path)
		if err != nil {
			return nil, err
		}
		f.fid = fid
		files = append(files, f)
	}

	// Sort files by the first index they store.
	sort.Slice(files, func(i, j int) bool {
		return files[i].firstEntry().Index() < files[j].firstEntry().Index()
	})
	return files, nil
}

// get entry from a file.
func (ef *entryFile) getEntry(idx int) entry {
	offset := idx * entrySize
	return entry(ef.data[offset : offset+entrySize])
}

func (ef *entryFile) firstEntry() entry {
	return ef.getEntry(0)
}
func (ef *entryFile) firstIndex() uint64 {
	return ef.getEntry(0).Index()
}

func (ef *entryFile) lastEntry() entry {
	for i := maxNumEntries - 1; i >= 0; i-- {
		e := ef.getEntry(i)
		if e.Index() > 0 {
			return e
		}
	}
	return emptyEntry
}

func (ef *entryFile) Term(entryIndex uint64) uint64 {
	fi := ef.firstIndex()
	if entryIndex < fi {
		return 0
	}
	offset := entryIndex - fi
	if offset > maxNumEntries {
		return 0
	}
	e := ef.getEntry(int(offset))
	if e.Index() == entryIndex {
		return e.Term()
	}
	return 0
}

// entryLog represents the entire entry log. It consists of one or more
// entryFile objects.
type entryLog struct {
	// need lock for files and current ?

	// files is the list of all log files ordered in ascending order by the first
	// index in the file. The current file being written should always be accessible
	// by looking at the last element of this slice.
	files   []*entryFile
	current *entryFile
	// entryIdx is the index of the next entry to write to. When this value exceeds
	// maxNumEntries the file will be rotated.
	entryIdx int
	// lastIndex is the value of last index written to the log.
	lastIndex uint64
	// dir is the directory to use to store files.
	dir string
}

func openEntryLog(dir string) (*entryLog, error) {
	e := &entryLog{
		dir: dir,
	}
	files, err := getEntryFiles(dir)
	if err != nil {
		return nil, err
	}
	e.files = files

	var nextFid int64
	for _, ef := range e.files {
		if nextFid < ef.fid {
			nextFid = ef.fid
		}
	}
	nextFid += 1
	ef, err := getEntryFile(path.Join(dir, fmt.Sprintf("%05d.ent", nextFid)))
	if err != nil {
		return nil, errors.Wrapf(err, "while creating a new entry file")
	}
	nextFid += 1

	// Won't append current file to list of files.
	e.current = ef
	return e, nil
}

// firstEmtpy finds the first empty index.
func (l *entryLog) firstEmpty() (int, error) {
	for i := 0; i < maxNumEntries; i++ {
		entry, err := l.getEntry(i)
		if err != nil {
			return 0, err
		}

		if entry.Index == 0 {
			return i, nil
		}
	}
	return maxNumEntries, nil
}

func (l *entryLog) lastFile() *entryFile {
	return l.files[len(l.files)-1]
}

// getEntry gets the nth entry in the CURRENT log file.
func (l *entryLog) getEntry(n int) (*entry, error) {
	if n >= maxNumEntries {
		return nil, errors.Errorf("there cannot be more than %d in a single file",
			maxNumEntries)
	}

	start := n * entrySize
	buf := l.lastFile().mmap[start : start+entrySize]
	return entryFromBytes(buf)
}

func (l *entryLog) rotate(firstIndex uint64) error {
	// TODO: this file should not exist already. Should try a new name.
	path := filepath.Join(l.dir, fmt.Sprintf("%d", firstIndex))
	// Buf will start writing at 1<<20
	buf, err := z.NewMmapFile(entryFileSize, entryFileMaxSize, entryFileOffset, path)
	if err != nil {
		return errors.Wrapf(err, "cannot open file")
	}
	l.current = buf
	l.entryIdx = 0

	// Append the new file to the list of files.
	l.files = append(l.files, &entryFile{path: path, firstIndex: firstIndex})

	if err := l.zeroOut(l.entryIdx); err != nil {
		return errors.Wrapf(err, "cannot zero out log file")
	}
	return nil
}

func (l *entryLog) numEntries() int {
	if len(l.files) == 0 {
		return 0
	}
	total := 0
	if len(l.files) >= 1 {
		// all files except the last one.
		total += (len(l.files) - 1) * maxNumEntries
	}
	return total + l.entryIdx
}

func (l *entryLog) allEntries(lo, hi, maxSize uint64) ([]raftpb.Entry, error) {
	if lo < l.FirstIndex() {
		return nil, errors.Errorf("low(%d) less then firstIndex (%d", lo, l.FirstIndex())
	}

	// Find lo entry.
	fileID := sort.Search(len(l.files), func(i int) bool {
		return l.files[i].firstIndex >= lo
	})

	var es []raftpb.Entry
	// Start with what we found.
	file := l.files[fileID]

	sz := uint64(0)
	// We couldn't find what we were looking for. Go one file back and look for the entry.
	if l.files[fileID].firstIndex != lo {
		x.AssertTrue(fileID != 0)

		// Go one file back.
		fileID--
		file = l.files[fileID]
		diff := lo - file.firstIndex
		// TODO - overflow check.
		e, err := file.getEntry(int(diff))
		if err != nil {
			return nil, err
		}
		x.AssertTrue(e.Index == lo)
		re := raftpb.Entry{
			Index: e.Index,
			Term:  e.Term,
			Type:  e.Type,
			Data:  file.mmap[e.DataOffset:], // full length of the file.
		}
		// This isn't the last entry so get next entry and fix the data len.
		if diff != 0 {
			nextEntry, err := file.getEntry(int(diff + 1))
			if err != nil {
				return es, err
			}
			re.Data = file.mmap[e.DataOffset:nextEntry.DataOffset]
		}
		es = append(es, re)
		sz += uint64(es[0].Size())
		lo++
	}

	if sz > maxSize {
		return es, nil
	}

	for lo < hi && fileID < len(l.files) {
		file = l.files[fileID]
		// Finished with this file. Move head
		if lo-file.firstIndex == maxNumEntries {
			fileID++
			continue
		}

		diff := lo - file.firstIndex

		if diff > l.lastIndex {
			break
		}

		// TODO - Overflow check.
		e, err := file.getEntry(int(diff))
		if err != nil {
			return es, err
		}

		rEntry := raftpb.Entry{
			Index: e.Index,
			Term:  e.Term,
			Type:  e.Type,
			// This wouldn't work. We need end of the data.!!!
			// TODO(ibrahim): Find the end of current data block.
			Data: file.mmap[e.DataOffset : l.current.Len()-entryFileOffset],
		}

		if diff != 0 {
			// TODO - e could be the last entry.
			nextEntry, err := file.getEntry(int(diff + 1))
			if err != nil {
				return es, err
			}
			rEntry.Data = file.mmap[e.DataOffset:nextEntry.DataOffset]
		}
		sz += uint64(rEntry.Size())
		if sz > maxSize {
			break
		}
		es = append(es, rEntry)
		lo++
	}

	return es, nil
}

func (l *entryLog) AddEntries(entries []raftpb.Entry) error {
	for _, re := range entries {
		if l.entryIdx >= maxNumEntries {
			if err := l.rotate(re.Index); err != nil {
				return err
			}
		}
		e := entry{
			Term:  re.Term,
			Index: re.Index,
			Type:  re.Type,
		}

		if len(re.Data) > 0 {
			destBuf, offset := l.current.SliceAllocateOffset(len(re.Data))
			e.DataOffset = uint64(offset)
			x.AssertTrue(copy(destBuf, re.Data) == len(re.Data))
		}

		entryBuf := e.Bytes()
		// TODO(ibrahim): Is this correct? Shouldn't it be number of entries multiply by entryIndex?
		destBuf, err := l.current.ReadAt(entrySize, l.entryIdx)
		if err != nil {
			return err
		}
		copy(destBuf, entryBuf)

		l.entryIdx++
		l.lastIndex = e.Index
	}
	return nil
}

func (l *entryLog) DiscardFiles(snapshotIndex uint64) error {
	// TODO: delete all the files below the first file with a first index
	// less than or equal to snapshotIndex.
	return nil
}

func (l *entryLog) FirstIndex() uint64 {
	if l == nil || len(l.files) == 0 {
		return 0
	}
	return l.files[0].firstEntry().Index()
}

func (l *entryLog) LastIndex() uint64 {
	if l.entryIdx-1 >= 0 {
		e := l.current.getEntry(l.entryIdx - 1)
		return e.Index()
	}
	if len(l.files) == 0 {
		return 0
	}
	for i := len(l.files) - 1; i >= 0; i-- {
		ef := l.files[i]
		e := ef.lastEntry()
		if e.Index() > 0 {
			return e.Index()
		}
	}
	return l.lastIndex
}

func (l *entryLog) Term(idx uint64) (uint64, error) {
	// Look at the entry files and find the entry file with entry bigger than idx.
	// Read file before that idx.
	if t := l.current.Term(idx); t > 0 {
		return t, nil
	}
	if len(l.files) == 0 {
		return 0, nil
	}

	fileIdx := sort.Search(len(l.files), func(i int) bool {
		return l.files[i].firstIndex() >= idx
	})
	if fileIdx >= len(l.files) {
		fileIdx = len(l.files) - 1
	}
	for fileIdx > 0 {
		fi := l.files[fileIdx].firstIndex()
		if fi <= idx {
			break
		}
		fileIdx--
	}
	return l.files[fileIdx].Term(idx), nil
}

// Init initializes returns a properly initialized instance of DiskStorage.
// To gracefully shutdown DiskStorage, store.Closer.SignalAndWait() should be called.
func Init(dir string, id uint64, gid uint32) *DiskStorage {
	// TODO: Init should take a dir.
	w := &DiskStorage{
		dir:            dir,
		id:             id,
		gid:            gid,
		cache:          new(sync.Map),
		Closer:         z.NewCloser(1),
		indexRangeChan: make(chan indexRange, 16),
	}

	var err error
	w.meta, err = newMetaFile(dir)
	x.Check(err)

	w.entries, err = openEntryLog(dir)
	x.Check(err)

	if prev := w.meta.RaftId(); prev != id || prev == 0 {
		w.meta.StoreRaftId(id)
	}
	// TODO: Do we need this?
	go w.processIndexRange()

	w.elog = trace.NewEventLog("Badger", "RaftStorage")

	snap, err := w.meta.Snapshot()
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

func (w *DiskStorage) Term(i uint64) (uint64, error) {
	return w.entries.Term(i)
}

// // fetchMaxVersion fetches the commitTs to be used in the raftwal. The version is
// // fetched from the special key "maxVersion-id" or from db.MaxVersion
// // API which uses the stream framework.
// func (w *DiskStorage) fetchMaxVersion() {
// 	// This is a special key that is used to fetch the latest version.
// 	key := []byte(fmt.Sprintf("maxVersion-%d", versionKey))

// 	txn := w.db.NewTransactionAt(math.MaxUint64, true)
// 	defer txn.Discard()

// 	item, err := txn.Get(key)
// 	if err == nil {
// 		w.commitTs = item.Version()
// 		return
// 	}
// 	if err == badger.ErrKeyNotFound {
// 		// We don't have the special key so get it using the MaxVersion API.
// 		version, err := w.db.MaxVersion()
// 		x.Check(err)

// 		w.commitTs = version + 1
// 		// Insert the same key back into badger for reuse.
// 		x.Check(txn.Set(key, nil))
// 		x.Check(txn.CommitAt(w.commitTs, nil))
// 	} else {
// 		x.Check(err)
// 	}
// }

func (w *DiskStorage) processIndexRange() {
	defer w.Closer.Done()

	processSingleRange := func(r indexRange) {
		if r.from == r.until {
			return
		}
		// TODO(ibrahim): Fix this. We don't have a way to delete entries right now.

		// batch := w.db.NewWriteBatchAt(w.commitTs)
		// if err := w.deleteRange(batch, r.from, r.until); err != nil {
		// 	glog.Errorf("deleteRange failed with error: %v, from: %d, until: %d\n",
		// 		err, r.from, r.until)
		// }
		// if err := batch.Flush(); err != nil {
		// 	glog.Errorf("processDeleteRange batch flush failed with error: %v,\n", err)
		// }
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

// EntryKey returns the key where the entry with the given ID is stored.
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

// // Term returns the term of entry i, which must be in the range
// // [FirstIndex()-1, LastIndex()]. The term of the entry before
// // FirstIndex is retained for matching purposes even though the
// // rest of that entry may not be available.
// func (w *DiskStorage) Term(idx uint64) (uint64, error) {
// 	w.elog.Printf("Term: %d", idx)
// 	defer w.elog.Printf("Done")
// 	first, err := w.FirstIndex()
// 	if err != nil {
// 		return 0, err
// 	}
// 	if idx < first-1 {
// 		return 0, raft.ErrCompacted
// 	}

// 	var e raftpb.Entry
// 	if _, err := w.seekEntry(&e, idx, false); err == errNotFound {
// 		return 0, raft.ErrUnavailable
// 	} else if err != nil {
// 		return 0, err
// 	}
// 	if idx < e.Index {
// 		return 0, raft.ErrCompacted
// 	}
// 	return e.Term, nil
// }

var errNotFound = errors.New("Unable to find raft entry")

// func (w *DiskStorage) seekEntry(e *raftpb.Entry, seekTo uint64, reverse bool) (uint64, error) {
// 	var index uint64
// 	err := w.db.View(func(txn *badger.Txn) error {
// 		opt := badger.DefaultIteratorOptions
// 		opt.PrefetchValues = false
// 		opt.Prefix = w.entryPrefix()
// 		opt.Reverse = reverse
// 		itr := txn.NewIterator(opt)
// 		defer itr.Close()

// 		itr.Seek(w.EntryKey(seekTo))
// 		if !itr.Valid() {
// 			return errNotFound
// 		}
// 		item := itr.Item()
// 		index = w.parseIndex(item.Key())
// 		if e == nil {
// 			return nil
// 		}
// 		return item.Value(func(val []byte) error {
// 			return e.Unmarshal(val)
// 		})
// 	})
// 	return index, err
// }

var (
	snapshotKey = "snapshot"
	firstKey    = "first"
	lastKey     = "last"
)

func (w *DiskStorage) LastIndex() (uint64, error) {
	return w.entries.LastIndex(), nil
}

// FirstIndex returns the index of the first log entry that is
// possibly available via Entries (older entries have been incorporated
// into the latest Snapshot).
func (w *DiskStorage) FirstIndex() (uint64, error) {
	// We are deleting index ranges in background after taking snapshot, so we should check for last
	// snapshot in WAL(Badger) if it is not found in cache. If no snapshot is found, then we can
	// check firstKey.
	if snap, err := w.Snapshot(); err == nil && !raft.IsEmptySnap(snap) {
		return snap.Metadata.Index + 1, nil
	}

	return w.entries.FirstIndex(), nil
	// if val, ok := w.cache.Load(firstKey); ok {
	// 	if first, ok := val.(uint64); ok {
	// 		return first, nil
	// 	}
	// }

	// // Now look into the mmap WAL.
	// index, err := w.seekEntry(nil, 0, false)
	// if err == nil {
	// 	glog.V(2).Infof("Setting first index: %d", index+1)
	// 	w.cache.Store(firstKey, index+1)
	// } else if glog.V(2) {
	// 	glog.Errorf("While seekEntry. Error: %v", err)
	// }
	// return index + 1, err
}

// // LastIndex returns the index of the last entry in the log.
// func (w *DiskStorage) LastIndex() (uint64, error) {
// 	if val, ok := w.cache.Load(lastKey); ok {
// 		if last, ok := val.(uint64); ok {
// 			return last, nil
// 		}
// 	}
// 	return w.seekEntry(nil, math.MaxUint64, true)
// }

// Delete all entries from [from, until), i.e. excluding until.
// Keep the entry at the snapshot index, for simplification of logic.
// It is the application's responsibility to not attempt to deleteRange an index
// greater than raftLog.applied.
func (w *DiskStorage) deleteRange(batch *badger.WriteBatch, from, until uint64) error {
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
func (w *DiskStorage) Snapshot() (raftpb.Snapshot, error) {
	if val, ok := w.cache.Load(snapshotKey); ok {
		snap, ok := val.(*raftpb.Snapshot)
		if ok && !raft.IsEmptySnap(*snap) {
			return *snap, nil
		}
	}

	return w.meta.Snapshot()
}

// setSnapshot would store the snapshot. We can delete all the entries up until the snapshot
// index. But, keep the raft entry at the snapshot index, to make it easier to build the logic; like
// the dummy entry in MemoryStorage.
func (w *DiskStorage) setSnapshot(batch *badger.WriteBatch, s *raftpb.Snapshot) error {
	if s == nil || raft.IsEmptySnap(*s) {
		return nil
	}

	if err := w.meta.StoreSnapshot(s); err != nil {
		return err
	}

	e := raftpb.Entry{Term: s.Metadata.Term, Index: s.Metadata.Index}
	data, err := e.Marshal()
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

// reset resets the entries. Used for testing.
func (w *DiskStorage) reset(es []raftpb.Entry) error {
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
	return w.meta.Checkpoint()
}

func (w *DiskStorage) UpdateCheckpoint(snap *pb.Snapshot) error {
	if w.meta == nil {
		return errors.Errorf("uninitialized meta file")
	}
	return w.meta.UpdateCheckpoint(snap)
}

// InitialState returns the saved HardState and ConfState information.
func (w *DiskStorage) InitialState() (hs raftpb.HardState, cs raftpb.ConfState, err error) {
	w.elog.Printf("InitialState")
	defer w.elog.Printf("Done")
	hs, err = w.meta.HardState()
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
	return w.entries.numEntries(), nil
}

// // NumEntries returns the number of entries in the write-ahead log.
// func (w *DiskStorage) NumEntries() (int, error) {
// 	first, err := w.FirstIndex()
// 	if err != nil {
// 		return 0, err
// 	}
// 	var count int
// 	err = w.db.View(func(txn *badger.Txn) error {
// 		opt := badger.DefaultIteratorOptions
// 		opt.PrefetchValues = false
// 		opt.Prefix = w.entryPrefix()
// 		itr := txn.NewIterator(opt)
// 		defer itr.Close()

// 		start := w.EntryKey(first)
// 		for itr.Seek(start); itr.Valid(); itr.Next() {
// 			count++
// 		}
// 		return nil
// 	})
// 	return count, err
// }

// return low to high, excluding the high.
func (w *DiskStorage) allEntriesNew(lo, hi, maxSize uint64) (es []raftpb.Entry, rerr error) {
	// fetch all the entry item from the entryLog

	ents, err := w.entries.allEntries(lo, hi, maxSize)
	if err != nil {
		return nil, err
	}

	_ = ents
	// unmarshal them into raft.pb.Entry
	return nil, nil

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
func (w *DiskStorage) Entries(lo, hi, maxSize uint64) (es []raftpb.Entry, rerr error) {
	w.elog.Printf("Entries: [%d, %d) maxSize:%d", lo, hi, maxSize)
	defer w.elog.Printf("Done")
	first := w.entries.FirstIndex()
	if lo < first {
		return nil, raft.ErrCompacted
	}

	last := w.entries.LastIndex()
	if hi > last+1 {
		return nil, raft.ErrUnavailable
	}

	return w.allEntries(lo, hi, maxSize)
}

// func (w *DiskStorage) Entries(lo, hi, maxSize uint64) (es []raftpb.Entry, rerr error) {
// 	w.elog.Printf("Entries: [%d, %d) maxSize:%d", lo, hi, maxSize)
// 	defer w.elog.Printf("Done")
// 	first, err := w.FirstIndex()
// 	if err != nil {
// 		return es, err
// 	}
// 	if lo < first {
// 		return nil, raft.ErrCompacted
// 	}

// 	last, err := w.LastIndex()
// 	if err != nil {
// 		return es, err
// 	}
// 	if hi > last+1 {
// 		return nil, raft.ErrUnavailable
// 	}

// 	return w.allEntries(lo, hi, maxSize)
// }

// CreateSnapshot generates a snapshot with the given ConfState and data and writes it to disk.
func (w *DiskStorage) CreateSnapshot(i uint64, cs *raftpb.ConfState, data []byte) error {
	panic("not implemented")
	// glog.V(2).Infof("CreateSnapshot i=%d, cs=%+v", i, cs)
	// first, err := w.FirstIndex()
	// if err != nil {
	// 	return err
	// }
	// if i < first {
	// 	glog.Errorf("i=%d<first=%d, ErrSnapOutOfDate", i, first)
	// 	return raft.ErrSnapOutOfDate
	// }

	// var e raftpb.Entry
	// if _, err := w.seekEntry(&e, i, false); err != nil {
	// 	return err
	// }
	// if e.Index != i {
	// 	return errNotFound
	// }

	// var snap raftpb.Snapshot
	// snap.Metadata.Index = i
	// snap.Metadata.Term = e.Term
	// x.AssertTrue(cs != nil)
	// snap.Metadata.ConfState = *cs
	// snap.Data = data

	// batch := w.db.NewWriteBatchAt(w.commitTs)
	// defer batch.Cancel()
	// if err := w.setSnapshot(batch, &snap); err != nil {
	// 	return err
	// }

	// if err := batch.Flush(); err != nil {
	// 	return err
	// }

	// // deleteRange deletes all entries in the range except the last one(which is SnapshotIndex) and
	// // first index is last snapshotIndex+1, hence start index for indexRange should be (first-1).
	// // TODO: If deleteRangeChan is full, it might block mutations.
	// w.indexRangeChan <- indexRange{first - 1, snap.Metadata.Index}
	return nil
}

// Save would write Entries, HardState and Snapshot to persistent storage in order, i.e. Entries
// first, then HardState and Snapshot if they are not empty. If persistent storage supports atomic
// writes then all of them can be written together. Note that when writing an Entry with Index i,
// any previously-persisted entries with Index >= i must be discarded.
func (w *DiskStorage) Save(h *raftpb.HardState, es []raftpb.Entry, snap *raftpb.Snapshot) error {
	batch := w.db.NewWriteBatchAt(w.commitTs)
	defer batch.Cancel()

	if err := w.entries.AddEntries(es); err != nil {
		return err
	}
	if err := w.meta.StoreHardState(h); err != nil {
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

	last := w.entries.LastIndex()
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
func (w *DiskStorage) Sync() error {
	return w.db.Sync()
}
