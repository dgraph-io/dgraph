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
	sync.RWMutex
	path   string
	fd     *os.File
	fid    uint16
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

func (lf *logFile) read(buf []byte, offset int64) error {
	lf.RLock()
	defer lf.RUnlock()

	nbr, err := lf.fd.ReadAt(buf, offset)
	y.NumReads.Add(1)
	y.NumBytesRead.Add(int64(nbr))
	return err
}

func (lf *logFile) doneWriting() error {
	lf.Lock()
	defer lf.Unlock()
	if err := lf.fd.Sync(); err != nil {
		return errors.Wrapf(err, "Unable to sync value log: %q", lf.path)
	}
	if err := lf.fd.Close(); err != nil {
		return errors.Wrapf(err, "Unable to close value log: %q", lf.path)
	}

	return lf.openReadOnly()
}

func (lf *logFile) sync() error {
	lf.RLock()
	defer lf.RUnlock()
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
		vlog.Lock()
		idx := sort.Search(len(vlog.files), func(idx int) bool {
			return vlog.files[idx].fid >= f.fid
		})
		if idx == len(vlog.files) || vlog.files[idx].fid != f.fid {
			return errors.Errorf("Unable to find fid: %d", f.fid)
		}
		vlog.files = append(vlog.files[:idx], vlog.files[idx+1:]...)
		vlog.Unlock()
	}

	rem := vlog.fpath(f.fid)
	f.fd.Close() // close file previous to remove it

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
	Fid    uint16
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

// Encode encodes Pointer into byte buffer.
func (p valuePointer) Encode(b []byte) []byte {
	y.AssertTrue(len(b) >= 10)
	binary.BigEndian.PutUint16(b[:2], p.Fid)
	binary.BigEndian.PutUint32(b[2:6], p.Len)
	binary.BigEndian.PutUint32(b[6:10], p.Offset)
	return b[:10]
}

func (p *valuePointer) Decode(b []byte) {
	y.AssertTrue(len(b) >= 10)
	p.Fid = binary.BigEndian.Uint16(b[:2])
	p.Len = binary.BigEndian.Uint32(b[2:6])
	p.Offset = binary.BigEndian.Uint32(b[6:10])
}

type valueLog struct {
	sync.RWMutex
	buf     bytes.Buffer
	dirPath string
	elog    trace.EventLog
	files   []*logFile
	kv      *KV
	maxFid  uint32
	offset  uint32
	opt     Options
}

func (vlog *valueLog) fpath(fid uint16) string {
	return fmt.Sprintf("%s%s%06d.vlog", vlog.dirPath, string(os.PathSeparator), fid)
}

func (vlog *valueLog) openOrCreateFiles() error {
	files, err := ioutil.ReadDir(vlog.dirPath)
	if err != nil {
		return errors.Wrapf(err, "Error while opening value log")
	}

	found := make(map[int]struct{})
	for _, file := range files {
		if !strings.HasSuffix(file.Name(), ".vlog") {
			continue
		}
		fsz := len(file.Name())
		fid, err := strconv.Atoi(file.Name()[:fsz-5])
		if err != nil {
			return errors.Wrapf(err, "Error while parsing value log id for file: %q", file.Name())
		}
		if _, ok := found[fid]; ok {
			return errors.Errorf("Found the same value log file twice: %d", fid)
		}
		found[fid] = struct{}{}

		lf := &logFile{fid: uint16(fid), path: vlog.fpath(uint16(fid))}
		vlog.files = append(vlog.files, lf)

	}

	sort.Slice(vlog.files, func(i, j int) bool {
		return vlog.files[i].fid < vlog.files[j].fid
	})

	// Open all previous log files as read only. Open the last log file
	// as read write.
	for i := range vlog.files {
		lf := vlog.files[i]
		if i == len(vlog.files)-1 {
			lf.fd, err = y.OpenExistingSyncedFile(vlog.fpath(lf.fid), vlog.opt.SyncWrites)
			if err != nil {
				return errors.Wrapf(err, "Unable to open value log file as RDWR")
			}
			vlog.maxFid = uint32(lf.fid)

		} else {
			if err := lf.openReadOnly(); err != nil {
				return err
			}
		}
	}

	// If no files are found, then create a new file.
	if len(vlog.files) == 0 {
		_, err := vlog.createVlogFile(0)
		if err != nil {
			return err
		}
	}
	return nil
}

func (vlog *valueLog) createVlogFile(fid uint16) (*logFile, error) {
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
	vlog.Lock()
	vlog.files = append(vlog.files, lf)
	vlog.Unlock()
	return lf, nil
}

func (vlog *valueLog) Open(kv *KV, opt *Options) error {
	vlog.dirPath = opt.ValueDir
	vlog.opt = *opt
	vlog.kv = kv
	if err := vlog.openOrCreateFiles(); err != nil {
		return errors.Wrapf(err, "Unable to open value log")
	}

	vlog.elog = trace.NewEventLog("Badger", "Valuelog")

	return nil
}

func (vlog *valueLog) Close() error {
	vlog.elog.Printf("Stopping garbage collection of values.")
	defer vlog.elog.Finish()

	for _, f := range vlog.files {
		if err := f.fd.Close(); err != nil {
			return err
		}
	}
	return nil
}

// Replay replays the value log. The kv provided is only valid for the lifetime of function call.
func (vlog *valueLog) Replay(ptr valuePointer, fn logEntry) error {
	fid := ptr.Fid
	offset := ptr.Offset + ptr.Len
	vlog.elog.Printf("Seeking at value pointer: %+v\n", ptr)

	for _, f := range vlog.files {
		if f.fid < fid {
			continue
		}
		of := offset
		if f.fid > fid {
			of = 0
		}
		err := f.iterate(of, fn)
		if err != nil {
			return errors.Wrapf(err, "Unable to replay value log: %q", f.path)
		}
	}

	// Seek to the end to start writing.
	var err error
	last := vlog.files[len(vlog.files)-1]
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

	vlog.RLock()
	if len(vlog.files) == 0 {
		vlog.RUnlock()
		return nil
	}
	curlf := vlog.files[len(vlog.files)-1]
	vlog.RUnlock()

	dirSyncCh := make(chan error)
	go func() { dirSyncCh <- syncDir(vlog.opt.ValueDir) }()
	err := curlf.sync()
	dirSyncErr := <-dirSyncCh
	if err != nil {
		err = dirSyncErr
	}
	return err
}

// write is thread-unsafe by design and should not be called concurrently.
func (vlog *valueLog) write(reqs []*request) error {
	vlog.RLock()
	curlf := vlog.files[len(vlog.files)-1]
	vlog.RUnlock()

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
			newlf, err := vlog.createVlogFile(uint16(newid))
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

func (vlog *valueLog) getFile(fid uint16) (*logFile, error) {
	vlog.RLock()
	defer vlog.RUnlock()

	idx := sort.Search(len(vlog.files), func(idx int) bool {
		return vlog.files[idx].fid >= fid
	})
	if idx == len(vlog.files) || vlog.files[idx].fid != fid {
		return nil, ErrCorrupt
	}
	return vlog.files[idx], nil
}

// Read reads the value log at a given location.
func (vlog *valueLog) Read(p valuePointer, s *y.Slice) (e Entry, err error) {
	lf, err := vlog.getFile(p.Fid)
	if err != nil {
		return e, err
	}

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
	vlog.RLock()
	defer vlog.RUnlock()
	if len(vlog.files) <= 1 {
		return nil
	}
	// This file shouldn't be being written to.
	lfi := rand.Intn(len(vlog.files))
	if lfi > 0 {
		lfi = rand.Intn(lfi) // Another level of rand to favor smaller fids.
	}
	return vlog.files[lfi]
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
