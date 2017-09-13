/*
 * Copyright 2017 Dgraph Labs, Inc. and Contributors
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

package badger

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"
	"io/ioutil"
	"math/rand"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dgraph-io/badger/y"
	"github.com/pkg/errors"
	"golang.org/x/net/trace"
)

// Values have their first byte being byteData or byteDelete. This helps us distinguish between
// a key that has never been seen and a key that has been explicitly deleted.
const (
	BitDelete       byte  = 1 // Set if the key has been deleted.
	BitValuePointer byte  = 2 // Set if the value is NOT stored directly next to key.
	BitUnused       byte  = 4
	BitSetIfAbsent  byte  = 8 // Set if the key is set using SetIfAbsent.
	M               int64 = 1 << 20
)

// ErrCorrupt is returned when a value log file is corrupted.
var ErrCorrupt = errors.New("Unable to find log. Potential data corruption")

// ErrCasMismatch is returned when a CompareAndSet operation has failed due
// to a counter mismatch.
var ErrCasMismatch = errors.New("CompareAndSet failed due to counter mismatch")

// ErrKeyExists is returned by SetIfAbsent metadata bit is set, but the
// key already exists in the store.
var ErrKeyExists = errors.New("SetIfAbsent failed since key already exists")

const (
	maxKeySize   = 1 << 20
	maxValueSize = 1 << 30
)

type logFile struct {
	path string
	// This is a lock on the file descriptor's value and the file's existence.  Use shared
	// ownership when reading/writing the file, use exclusive ownership to open/close the
	// descriptor or remove the file.
	fdLock sync.RWMutex
	fd     *os.File
	fid    uint32
	offset uint32
	size   uint32
}

// openReadOnly assumes that we have a write lock on logFile.
func (lf *logFile) openReadOnly() error {
	var err error
	lf.fd, err = os.OpenFile(lf.path, os.O_RDONLY, 0666)
	if err != nil {
		return errors.Wrapf(err, "Unable to open %q as RDONLY.", lf.path)
	}

	fi, err := lf.fd.Stat()
	if err != nil {
		return errors.Wrapf(err, "Unable to check stat for %q", lf.path)
	}
	lf.size = uint32(fi.Size())
	return nil
}

// lf must be RLocked (or exclusively locked) if you call this.
func (lf *logFile) read(buf []byte, offset int64) error {
	nbr, err := lf.fd.ReadAt(buf, offset)
	y.NumReads.Add(1)
	y.NumBytesRead.Add(int64(nbr))
	return err
}

func (lf *logFile) doneWriting() error {
	// Sync before acquiring lock.  (We call this from write() and thus know we have shared access
	// to the fd.)
	if err := lf.fd.Sync(); err != nil {
		return errors.Wrapf(err, "Unable to sync value log: %q", lf.path)
	}
	// Close and reopen the file read-only.  Acquire lock because fd will become invalid for a bit.
	// Acquiring the lock is bad because, while we don't hold the lock for a long time, it forces
	// one batch of readers wait for the preceding batch of readers to finish.
	//
	// If there's a benefit to reopening the file read-only, it might be on Windows.  I don't know
	// what the benefit is.  Consider keeping the file read-write, or use fcntl to change permissions.
	lf.fdLock.Lock()
	defer lf.fdLock.Unlock()
	if err := lf.fd.Close(); err != nil {
		return errors.Wrapf(err, "Unable to close value log: %q", lf.path)
	}

	return lf.openReadOnly()
}

// You must hold lf.lock to sync()
func (lf *logFile) sync() error {
	return lf.fd.Sync()
}

var errStop = errors.New("Stop iteration")

type logEntry func(e Entry, vp valuePointer) error

// iterate iterates over log file. It doesn't not allocate new memory for every kv pair.
// Therefore, the kv pair is only valid for the duration of fn call.
func (lf *logFile) iterate(offset uint32, fn logEntry) error {
	_, err := lf.fd.Seek(int64(offset), io.SeekStart)
	if err != nil {
		return y.Wrap(err)
	}

	reader := bufio.NewReader(lf.fd)
	var hbuf [headerBufSize]byte
	var h header
	k := make([]byte, 1<<10)
	v := make([]byte, 1<<20)

	truncate := false
	recordOffset := offset
	for {
		hash := crc32.New(y.CastagnoliCrcTable)
		tee := io.TeeReader(reader, hash)

		if _, err = io.ReadFull(tee, hbuf[:]); err != nil {
			if err == io.EOF {
				break
			} else if err == io.ErrUnexpectedEOF {
				truncate = true
				break
			}
			return err
		}

		var e Entry
		e.offset = recordOffset
		h.Decode(hbuf[:])
		if h.klen > maxKeySize || h.vlen > maxValueSize {
			truncate = true
			break
		}
		vl := int(h.vlen)
		if cap(v) < vl {
			v = make([]byte, 2*vl)
		}

		kl := int(h.klen)
		if cap(k) < kl {
			k = make([]byte, 2*kl)
		}
		e.Key = k[:kl]
		e.Value = v[:vl]

		if _, err = io.ReadFull(tee, e.Key); err != nil {
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				truncate = true
				break
			}
			return err
		}
		e.Meta = h.meta
		e.UserMeta = h.userMeta
		e.casCounter = h.casCounter
		e.CASCounterCheck = h.casCounterCheck
		if _, err = io.ReadFull(tee, e.Value); err != nil {
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				truncate = true
				break
			}
			return err
		}

		var crcBuf [4]byte
		if _, err = io.ReadFull(reader, crcBuf[:]); err != nil {
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				truncate = true
				break
			}
			return err
		}
		crc := binary.BigEndian.Uint32(crcBuf[:])
		if crc != hash.Sum32() {
			truncate = true
			break
		}

		var vp valuePointer

		vp.Len = headerBufSize + h.klen + h.vlen + uint32(len(crcBuf))
		recordOffset += vp.Len

		vp.Offset = e.offset
		vp.Fid = lf.fid

		if err := fn(e, vp); err != nil {
			if err == errStop {
				break
			}
			return y.Wrap(err)
		}
	}

	if truncate {
		if err := lf.fd.Truncate(int64(recordOffset)); err != nil {
			return err
		}
	}

	return nil
}

func (vlog *valueLog) rewrite(f *logFile) error {
	maxFid := atomic.LoadUint32(&vlog.maxFid)
	y.AssertTruef(uint32(f.fid) < maxFid, "fid to move: %d. Current max fid: %d", f.fid, maxFid)

	elog := trace.NewEventLog("badger", "vlog-rewrite")
	defer elog.Finish()
	elog.Printf("Rewriting fid: %d", f.fid)

	wb := make([]*Entry, 0, 1000)
	var size int64

	y.AssertTrue(vlog.kv != nil)
	var count int
	fe := func(e Entry) error {
		count++
		if count%10000 == 0 {
			elog.Printf("Processing entry %d", count)
		}

		vs, err := vlog.kv.get(e.Key)
		if err != nil {
			return err
		}

		if (vs.Meta & BitDelete) > 0 {
			return nil
		}
		if (vs.Meta & BitValuePointer) == 0 {
			return nil
		}

		// Value is still present in value log.
		if len(vs.Value) == 0 {
			return errors.Errorf("Empty value: %+v", vs)
		}
		var vp valuePointer
		vp.Decode(vs.Value)

		if vp.Fid > f.fid {
			return nil
		}
		if vp.Offset > e.offset {
			return nil
		}
		if vp.Fid == f.fid && vp.Offset == e.offset {
			// This new entry only contains the key, and a pointer to the value.
			ne := new(Entry)
			if e.Meta == BitSetIfAbsent {
				// If we rewrite this entry without removing BitSetIfAbsent, lsm would see that
				// the key is already present, which would be this same entry and won't update
				// the vptr to point to the new file.
				e.Meta = 0
			}
			y.AssertTruef(e.Meta == 0, "Got meta: 0")
			ne.Meta = e.Meta
			ne.UserMeta = e.UserMeta
			ne.Key = make([]byte, len(e.Key))
			copy(ne.Key, e.Key)
			ne.Value = make([]byte, len(e.Value))
			copy(ne.Value, e.Value)
			ne.CASCounterCheck = vs.CASCounter // CAS counter check. Do not rewrite if key has a newer value.
			wb = append(wb, ne)
			size += int64(vlog.opt.estimateSize(ne))
			if size >= 64*M {
				elog.Printf("request has %d entries, size %d", len(wb), size)
				if err := vlog.kv.BatchSet(wb); err != nil {
					return err
				}
				size = 0
				wb = wb[:0]
			}
		} else {
			// This can now happen because we can move some entries forward, but then not write
			// them to LSM tree due to CAS check failure.
		}
		return nil
	}

	err := f.iterate(0, func(e Entry, vp valuePointer) error {
		return fe(e)
	})
	if err != nil {
		return err
	}

	if len(wb) > 0 {
		elog.Printf("request has %d entries, size %d", len(wb), size)
		if err := vlog.kv.BatchSet(wb); err != nil {
			return err
		}
	}
	elog.Printf("Processed %d entries in total", count)

	elog.Printf("Removing fid: %d", f.fid)
	// Entries written to LSM. Remove the older file now.
	{
		vlog.filesLock.Lock()
		// Just a sanity-check.
		if _, ok := vlog.filesMap[f.fid]; !ok {
			vlog.filesLock.Unlock()
			return errors.Errorf("Unable to find fid: %d", f.fid)
		}
		delete(vlog.filesMap, f.fid)
		vlog.filesLock.Unlock()
	}

	// Exclusively lock the file so that there are no readers before closing/destroying it
	f.fdLock.Lock()
	rem := vlog.fpath(f.fid)
	f.fd.Close() // close file previous to remove it
	f.fdLock.Unlock()

	elog.Printf("Removing %s", rem)
	return os.Remove(rem)
}

// Entry provides Key, Value and if required, CASCounterCheck to kv.BatchSet() API.
// If CASCounterCheck is provided, it would be compared against the current casCounter
// assigned to this key-value. Set be done on this key only if the counters match.
type Entry struct {
	Key             []byte
	Meta            byte
	UserMeta        byte
	Value           []byte
	CASCounterCheck uint64 // If nonzero, we will check if existing casCounter matches.
	Error           error  // Error if any.

	// Fields maintained internally.
	offset     uint32
	casCounter uint64
}

// Encodes e to buf. Returns number of bytes written.
func encodeEntry(e *Entry, buf *bytes.Buffer) (int, error) {
	var h header
	h.klen = uint32(len(e.Key))
	h.vlen = uint32(len(e.Value))
	h.meta = e.Meta
	h.userMeta = e.UserMeta
	h.casCounter = e.casCounter
	h.casCounterCheck = e.CASCounterCheck

	var headerEnc [headerBufSize]byte
	h.Encode(headerEnc[:])

	hash := crc32.New(y.CastagnoliCrcTable)

	buf.Write(headerEnc[:])
	hash.Write(headerEnc[:])

	buf.Write(e.Key)
	hash.Write(e.Key)

	buf.Write(e.Value)
	hash.Write(e.Value)

	var crcBuf [4]byte
	binary.BigEndian.PutUint32(crcBuf[:], hash.Sum32())
	buf.Write(crcBuf[:])

	return len(headerEnc) + len(e.Key) + len(e.Value) + len(crcBuf), nil
}

func (e Entry) print(prefix string) {
	fmt.Printf("%s Key: %s Meta: %d UserMeta: %d Offset: %d len(val)=%d cas=%d check=%d\n",
		prefix, e.Key, e.Meta, e.UserMeta, e.offset, len(e.Value), e.casCounter, e.CASCounterCheck)
}

type header struct {
	klen            uint32
	vlen            uint32
	meta            byte
	userMeta        byte
	casCounter      uint64
	casCounterCheck uint64
}

const (
	headerBufSize = 26
)

func (h header) Encode(out []byte) {
	y.AssertTrue(len(out) >= headerBufSize)
	binary.BigEndian.PutUint32(out[0:4], h.klen)
	binary.BigEndian.PutUint32(out[4:8], h.vlen)
	out[8] = h.meta
	out[9] = h.userMeta
	binary.BigEndian.PutUint64(out[10:18], h.casCounter)
	binary.BigEndian.PutUint64(out[18:26], h.casCounterCheck)
}

// Decodes h from buf.
func (h *header) Decode(buf []byte) {
	h.klen = binary.BigEndian.Uint32(buf[0:4])
	h.vlen = binary.BigEndian.Uint32(buf[4:8])
	h.meta = buf[8]
	h.userMeta = buf[9]
	h.casCounter = binary.BigEndian.Uint64(buf[10:18])
	h.casCounterCheck = binary.BigEndian.Uint64(buf[18:26])
}

type valuePointer struct {
	Fid    uint32
	Len    uint32
	Offset uint32
}

func (p valuePointer) Less(o valuePointer) bool {
	if p.Fid != o.Fid {
		return p.Fid < o.Fid
	}
	if p.Offset != o.Offset {
		return p.Offset < o.Offset
	}
	return p.Len < o.Len
}

func (p valuePointer) IsZero() bool {
	return p.Fid == 0 && p.Offset == 0 && p.Len == 0
}

const valuePointerEncodedSize = 12

// Encode encodes Pointer into byte buffer.
func (p valuePointer) Encode(b []byte) []byte {
	binary.BigEndian.PutUint32(b[:4], p.Fid)
	binary.BigEndian.PutUint32(b[4:8], p.Len)
	binary.BigEndian.PutUint32(b[8:valuePointerEncodedSize], p.Offset)
	return b[:valuePointerEncodedSize]
}

func (p *valuePointer) Decode(b []byte) {
	p.Fid = binary.BigEndian.Uint32(b[:4])
	p.Len = binary.BigEndian.Uint32(b[4:8])
	p.Offset = binary.BigEndian.Uint32(b[8:valuePointerEncodedSize])
}

type valueLog struct {
	buf     bytes.Buffer
	dirPath string
	elog    trace.EventLog

	// guards our view of which files exist
	filesLock sync.RWMutex
	filesMap  map[uint32]*logFile

	kv     *KV
	maxFid uint32
	offset uint32
	opt    Options
}

func vlogFilePath(dirPath string, fid uint32) string {
	return fmt.Sprintf("%s%s%06d.vlog", dirPath, string(os.PathSeparator), fid)
}

func (vlog *valueLog) fpath(fid uint32) string {
	return vlogFilePath(vlog.dirPath, fid)
}

func (vlog *valueLog) openOrCreateFiles() error {
	files, err := ioutil.ReadDir(vlog.dirPath)
	if err != nil {
		return errors.Wrapf(err, "Error while opening value log")
	}

	found := make(map[uint64]struct{})
	var maxFid uint32 // Beware len(files) == 0 case, this starts at 0.
	for _, file := range files {
		if !strings.HasSuffix(file.Name(), ".vlog") {
			continue
		}
		fsz := len(file.Name())
		fid, err := strconv.ParseUint(file.Name()[:fsz-5], 10, 32)
		if err != nil {
			return errors.Wrapf(err, "Error while parsing value log id for file: %q", file.Name())
		}
		if _, ok := found[fid]; ok {
			return errors.Errorf("Found the same value log file twice: %d", fid)
		}
		found[fid] = struct{}{}

		lf := &logFile{fid: uint32(fid), path: vlog.fpath(uint32(fid))}
		vlog.filesMap[uint32(fid)] = lf
		if uint32(fid) > maxFid {
			maxFid = uint32(fid)
		}
	}
	vlog.maxFid = uint32(maxFid)

	// Open all previous log files as read only. Open the last log file
	// as read write.
	for fid, lf := range vlog.filesMap {
		if fid == maxFid {
			lf.fd, err = y.OpenExistingSyncedFile(vlog.fpath(fid), vlog.opt.SyncWrites)
			if err != nil {
				return errors.Wrapf(err, "Unable to open value log file as RDWR")
			}
		} else {
			if err := lf.openReadOnly(); err != nil {
				return err
			}
		}
	}

	// If no files are found, then create a new file.
	if len(vlog.filesMap) == 0 {
		// We already set vlog.maxFid above
		_, err := vlog.createVlogFile(0)
		if err != nil {
			return err
		}
	}
	return nil
}

func (vlog *valueLog) createVlogFile(fid uint32) (*logFile, error) {
	path := vlog.fpath(fid)
	lf := &logFile{fid: fid, offset: 0, path: path}
	var err error
	lf.fd, err = y.CreateSyncedFile(path, vlog.opt.SyncWrites)
	if err != nil {
		return nil, errors.Wrapf(err, "Unable to create value log file")
	}
	err = syncDir(vlog.dirPath)
	if err != nil {
		return nil, errors.Wrapf(err, "Unable to sync value log file dir")
	}
	vlog.filesLock.Lock()
	vlog.filesMap[fid] = lf
	vlog.filesLock.Unlock()
	return lf, nil
}

func (vlog *valueLog) Open(kv *KV, opt *Options) error {
	vlog.dirPath = opt.ValueDir
	vlog.opt = *opt
	vlog.kv = kv
	vlog.filesMap = make(map[uint32]*logFile)
	if err := vlog.openOrCreateFiles(); err != nil {
		return errors.Wrapf(err, "Unable to open value log")
	}

	vlog.elog = trace.NewEventLog("Badger", "Valuelog")

	return nil
}

func (vlog *valueLog) Close() error {
	vlog.elog.Printf("Stopping garbage collection of values.")
	defer vlog.elog.Finish()

	var err error
	for _, f := range vlog.filesMap {
		if closeErr := f.fd.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
	}
	return err
}

// sortedFids returns the file id's, sorted.  Assumes we have shared access to filesMap.
func (vlog *valueLog) sortedFids() []uint32 {
	ret := make([]uint32, 0, len(vlog.filesMap))
	for fid := range vlog.filesMap {
		ret = append(ret, fid)
	}
	sort.Slice(ret, func(i, j int) bool {
		return ret[i] < ret[j]
	})
	return ret
}

// Replay replays the value log. The kv provided is only valid for the lifetime of function call.
func (vlog *valueLog) Replay(ptr valuePointer, fn logEntry) error {
	fid := ptr.Fid
	offset := ptr.Offset + ptr.Len
	vlog.elog.Printf("Seeking at value pointer: %+v\n", ptr)

	fids := vlog.sortedFids()

	for _, id := range fids {
		if id < fid {
			continue
		}
		of := offset
		if id > fid {
			of = 0
		}
		f := vlog.filesMap[id]
		err := f.iterate(of, fn)
		if err != nil {
			return errors.Wrapf(err, "Unable to replay value log: %q", f.path)
		}
	}

	// Seek to the end to start writing.
	var err error
	last := vlog.filesMap[vlog.maxFid]
	lastOffset, err := last.fd.Seek(0, io.SeekEnd)
	last.offset = uint32(lastOffset)
	return errors.Wrapf(err, "Unable to seek to end of value log: %q", last.path)
}

type request struct {
	// Input values
	Entries []*Entry
	// Output values and wait group stuff below
	Ptrs []valuePointer
	Wg   sync.WaitGroup
	Err  error
}

// sync is thread-unsafe and should not be called concurrently with write.
func (vlog *valueLog) sync() error {
	if vlog.opt.SyncWrites {
		return nil
	}

	vlog.filesLock.RLock()
	if len(vlog.filesMap) == 0 {
		vlog.filesLock.RUnlock()
		return nil
	}
	curlf := vlog.filesMap[vlog.maxFid]
	curlf.fdLock.RLock()
	vlog.filesLock.RUnlock()

	dirSyncCh := make(chan error)
	go func() { dirSyncCh <- syncDir(vlog.opt.ValueDir) }()
	err := curlf.sync()
	curlf.fdLock.RUnlock()
	dirSyncErr := <-dirSyncCh
	if err != nil {
		err = dirSyncErr
	}
	return err
}

// write is thread-unsafe by design and should not be called concurrently.
func (vlog *valueLog) write(reqs []*request) error {
	vlog.filesLock.RLock()
	curlf := vlog.filesMap[vlog.maxFid]
	vlog.filesLock.RUnlock()

	toDisk := func() error {
		if vlog.buf.Len() == 0 {
			return nil
		}
		vlog.elog.Printf("Flushing %d blocks of total size: %d", len(reqs), vlog.buf.Len())
		n, err := curlf.fd.Write(vlog.buf.Bytes())
		if err != nil {
			return errors.Wrapf(err, "Unable to write to value log file: %q", curlf.path)
		}
		y.NumWrites.Add(1)
		y.NumBytesWritten.Add(int64(n))
		vlog.elog.Printf("Done")
		curlf.offset += uint32(n)
		vlog.buf.Reset()

		if curlf.offset > uint32(vlog.opt.ValueLogFileSize) {
			var err error
			if err = curlf.doneWriting(); err != nil {
				return err
			}

			newid := atomic.AddUint32(&vlog.maxFid, 1)
			y.AssertTruef(newid < 1<<16, "newid will overflow uint16: %v", newid)
			newlf, err := vlog.createVlogFile(newid)
			if err != nil {
				return err
			}

			curlf = newlf
		}
		return nil
	}

	for i := range reqs {
		b := reqs[i]
		b.Ptrs = b.Ptrs[:0]
		for j := range b.Entries {
			e := b.Entries[j]
			var p valuePointer

			if !vlog.opt.SyncWrites && len(e.Value) < vlog.opt.ValueThreshold {
				// No need to write to value log.
				b.Ptrs = append(b.Ptrs, p)
				continue
			}

			p.Fid = curlf.fid
			p.Offset = curlf.offset + uint32(vlog.buf.Len()) // Use the offset including buffer length so far.
			plen, err := encodeEntry(e, &vlog.buf)           // Now encode the entry into buffer.
			if err != nil {
				return err
			}
			p.Len = uint32(plen)
			b.Ptrs = append(b.Ptrs, p)

			if p.Offset > uint32(vlog.opt.ValueLogFileSize) {
				if err := toDisk(); err != nil {
					return err
				}
			}
		}
	}
	return toDisk()

	// Acquire mutex locks around this manipulation, so that the reads don't try to use
	// an invalid file descriptor.
}

// Gets the logFile and acquires an RLock() on it too.  You must call RUnlock on the file (if
// non-nil).
func (vlog *valueLog) getFileRLocked(fid uint32) (*logFile, error) {
	vlog.filesLock.RLock()
	defer vlog.filesLock.RUnlock()
	ret, ok := vlog.filesMap[fid]
	if !ok {
		return nil, ErrCorrupt
	}
	ret.fdLock.RLock()
	return ret, nil
}

// Read reads the value log at a given location.
func (vlog *valueLog) Read(p valuePointer, s *y.Slice) (e Entry, err error) {
	lf, err := vlog.getFileRLocked(p.Fid)
	if err != nil {
		return e, err
	}
	defer lf.fdLock.RUnlock()

	if s == nil {
		s = new(y.Slice)
	}
	buf := s.Resize(int(p.Len))
	if err := lf.read(buf, int64(p.Offset)); err != nil {
		return e, err
	}
	var h header
	h.Decode(buf)
	n := uint32(headerBufSize)

	e.Key = buf[n : n+h.klen]
	n += h.klen
	e.Meta = h.meta
	e.UserMeta = h.userMeta
	e.casCounter = h.casCounter
	e.CASCounterCheck = h.casCounterCheck
	e.Value = buf[n : n+h.vlen]

	return e, nil
}

func (vlog *valueLog) runGCInLoop(lc *y.LevelCloser) {
	defer lc.Done()
	if vlog.opt.ValueGCThreshold == 0.0 {
		return
	}

	tick := time.NewTicker(vlog.opt.ValueGCRunInterval)
	for {
		select {
		case <-lc.HasBeenClosed():
			return
		case <-tick.C:
			vlog.doRunGC()
		}
	}
}

func (vlog *valueLog) pickLog() *logFile {
	vlog.filesLock.RLock()
	defer vlog.filesLock.RUnlock()
	if len(vlog.filesMap) <= 1 {
		return nil
	}
	fids := vlog.sortedFids()
	// This file shouldn't be being written to.
	idx := rand.Intn(len(fids))
	if idx > 0 {
		idx = rand.Intn(idx) // Another level of rand to favor smaller fids.
	}
	return vlog.filesMap[fids[idx]]
}

func (vlog *valueLog) doRunGC() error {
	lf := vlog.pickLog()
	if lf == nil {
		return nil
	}

	type reason struct {
		total   float64
		keep    float64
		discard float64
	}

	var r reason
	var window = 100.0
	count := 0

	// Pick a random start point for the log.
	skipFirstM := float64(rand.Intn(int(vlog.opt.ValueLogFileSize/M))) - window
	var skipped float64

	start := time.Now()
	y.AssertTrue(vlog.kv != nil)
	err := lf.iterate(0, func(e Entry, vp valuePointer) error {
		esz := float64(vp.Len) / (1 << 20) // in MBs. +4 for the CAS stuff.
		skipped += esz
		if skipped < skipFirstM {
			return nil
		}

		count++
		if count%100 == 0 {
			time.Sleep(time.Millisecond)
		}
		r.total += esz
		if r.total > window {
			return errStop
		}
		if time.Since(start) > 10*time.Second {
			return errStop
		}

		vs, err := vlog.kv.get(e.Key)
		if err != nil {
			return err
		}
		if (vs.Meta & BitDelete) > 0 {
			// Key has been deleted. Discard.
			r.discard += esz
			return nil
		}
		if (vs.Meta & BitValuePointer) == 0 {
			// Value is stored alongside key. Discard.
			r.discard += esz
			return nil
		}

		// Value is still present in value log.
		y.AssertTrue(len(vs.Value) > 0)

		vp.Decode(vs.Value)

		if vp.Fid > lf.fid {
			// Value is present in a later log. Discard.
			r.discard += esz
			return nil
		}
		if vp.Offset > e.offset {
			// Value is present in a later offset, but in the same log.
			r.discard += esz
			return nil
		}
		if vp.Fid == lf.fid && vp.Offset == e.offset {
			// This is still the active entry. This would need to be rewritten.
			r.keep += esz

		} else {
			vlog.elog.Printf("Reason=%+v\n", r)
			ne, err := vlog.Read(vp, nil)
			if err != nil {
				return errStop
			}
			ne.offset = vp.Offset
			if ne.casCounter == e.casCounter {
				ne.print("Latest Entry in LSM")
				e.print("Latest Entry in Log")
				return errors.Errorf(
					"This shouldn't happen. Latest Pointer:%+v. Meta:%v.", vp, vs.Meta)
			}
		}
		return nil
	})

	if err != nil {
		vlog.elog.Errorf("Error while iterating for RunGC: %v", err)
		return err
	}
	vlog.elog.Printf("Fid: %d Data status=%+v\n", lf.fid, r)

	if r.total < 10.0 || r.discard < vlog.opt.ValueGCThreshold*r.total {
		vlog.elog.Printf("Skipping GC on fid: %d\n\n", lf.fid)
		return nil
	}

	vlog.elog.Printf("REWRITING VLOG %d\n", lf.fid)
	if err = vlog.rewrite(lf); err != nil {
		return err
	}
	vlog.elog.Printf("Done rewriting.")
	return nil
}
