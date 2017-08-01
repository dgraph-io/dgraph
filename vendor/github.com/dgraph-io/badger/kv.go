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
	"log"
	"math"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/net/trace"

	"github.com/dgraph-io/badger/skl"
	"github.com/dgraph-io/badger/table"
	"github.com/dgraph-io/badger/y"
	"github.com/pkg/errors"
)

var (
	badgerPrefix = []byte("!badger!")     // Prefix for internal keys used by badger.
	head         = []byte("!badger!head") // For storing value offset for replay.
)

// Options are params for creating DB object.
type Options struct {
	Dir      string // Directory to store the data in. Should exist and be writable.
	ValueDir string // Directory to store the value log in. Can be the same as Dir.
	// Should exist and be writable.

	// The following affect all levels of LSM tree.
	MaxTableSize        int64 // Each table (or file) is at most this size.
	LevelSizeMultiplier int   // Equals SizeOf(Li+1)/SizeOf(Li).
	MaxLevels           int   // Maximum number of levels of compaction.
	ValueThreshold      int   // If value size >= this threshold, only store value offsets in tree.
	MapTablesTo         int   // How should LSM tree be accessed.

	NumMemtables int // Maximum number of tables to keep in memory, before stalling.

	// The following affect how we handle LSM tree L0.
	// Maximum number of Level 0 tables before we start compacting.
	NumLevelZeroTables int
	// If we hit this number of Level 0 tables, we will stall until L0 is compacted away.
	NumLevelZeroTablesStall int

	// Maximum total size for L1.
	LevelOneSize int64

	// Run value log garbage collection if we can reclaim at least this much space. This is a ratio.
	ValueGCThreshold float64
	// How often to run value log garbage collector.
	ValueGCRunInterval time.Duration

	// Size of single value log file.
	ValueLogFileSize int64

	// The following affect value compression in value log. Note that compression
	// can significantly slow down the loading and lookup time.
	ValueCompressionMinSize  int32   // Minimal size in bytes of KV pair to be compressed.
	ValueCompressionMinRatio float64 // Minimal compression ratio of KV pair to be compressed.

	// Sync all writes to disk. Setting this to true would slow down data loading significantly.
	SyncWrites bool

	// Number of compaction workers to run concurrently.
	NumCompactors int

	// Flags for testing purposes.
	DoNotCompact bool // Stops LSM tree from compactions.

	maxBatchSize int64 // max batch size in bytes
}

// DefaultOptions sets a list of recommended options for good performance.
// Feel free to modify these to suit your needs.
var DefaultOptions = Options{
	DoNotCompact:        false,
	LevelOneSize:        256 << 20,
	LevelSizeMultiplier: 10,
	MapTablesTo:         table.LoadToRAM,
	// table.MemoryMap to mmap() the tables.
	// table.Nothing to not preload the tables.
	MaxLevels:                7,
	MaxTableSize:             64 << 20,
	NumCompactors:            3,
	NumLevelZeroTables:       5,
	NumLevelZeroTablesStall:  10,
	NumMemtables:             5,
	SyncWrites:               false,
	ValueCompressionMinRatio: 2.0,
	ValueCompressionMinSize:  math.MaxInt32, // Turn off by default.
	ValueGCRunInterval:       10 * time.Minute,
	ValueGCThreshold:         0.5, // Set to zero to not run GC.
	ValueLogFileSize:         1 << 30,
	ValueThreshold:           20,
}

func (opt *Options) estimateSize(entry *Entry) int {
	if len(entry.Value) < opt.ValueThreshold {
		return len(entry.Key) + len(entry.Value) + y.MetaSize + y.UserMetaSize + y.CasSize
	}
	return len(entry.Key) + 16 + y.MetaSize + y.UserMetaSize + y.CasSize
}

// KV provides the various functions required to interact with Badger.
// KV is thread-safe.
type KV struct {
	sync.RWMutex // Guards list of inmemory tables, not individual reads and writes.

	dirLockGuard *y.DirectoryLockGuard
	// nil if Dir and ValueDir are the same
	valueDirGuard *y.DirectoryLockGuard

	closer    *y.Closer
	elog      trace.EventLog
	mt        *skl.Skiplist   // Our latest (actively written) in-memory table
	imm       []*skl.Skiplist // Add here only AFTER pushing to flushChan.
	opt       Options
	lc        *levelsController
	vlog      valueLog
	vptr      valuePointer // less than or equal to a pointer to the last vlog value put into mt
	writeCh   chan *request
	flushChan chan flushTask // For flushing memtables.

	// Incremented in the non-concurrently accessed write loop.  But also accessed outside. So
	// we use an atomic op.
	lastUsedCasCounter uint64
}

var ErrInvalidDir error = errors.New("Invalid Dir, directory does not exist")
var ErrValueLogSize error = errors.New("Invalid ValueLogFileSize, must be between 1MB and 1GB")

const (
	kvWriteChCapacity = 1000
)

// NewKV returns a new KV object.
func NewKV(optParam *Options) (out *KV, err error) {
	// Make a copy early and fill in maxBatchSize
	opt := *optParam
	opt.maxBatchSize = (15 * opt.MaxTableSize) / 100

	for _, path := range []string{opt.Dir, opt.ValueDir} {
		dirExists, err := exists(path)
		if err != nil {
			return nil, y.Wrapf(err, "Invalid Dir: %q", path)
		}
		if !dirExists {
			return nil, ErrInvalidDir
		}
	}
	absDir, err := filepath.Abs(opt.Dir)
	if err != nil {
		return nil, err
	}
	absValueDir, err := filepath.Abs(opt.ValueDir)
	if err != nil {
		return nil, err
	}

	dirLockGuard, err := y.AcquireDirectoryLock(opt.Dir, lockFile)
	if err != nil {
		return nil, err
	}
	defer func() {
		if dirLockGuard != nil {
			_ = dirLockGuard.Release()
		}
	}()
	var valueDirLockGuard *y.DirectoryLockGuard
	if absValueDir != absDir {
		valueDirLockGuard, err = y.AcquireDirectoryLock(opt.ValueDir, lockFile)
		if err != nil {
			return nil, err
		}
	}
	defer func() {
		if valueDirLockGuard != nil {
			_ = valueDirLockGuard.Release()
		}
	}()
	if !(opt.ValueLogFileSize <= 2<<30 && opt.ValueLogFileSize >= 1<<20) {
		return nil, ErrValueLogSize
	}
	out = &KV{
		imm:           make([]*skl.Skiplist, 0, opt.NumMemtables),
		flushChan:     make(chan flushTask, opt.NumMemtables),
		writeCh:       make(chan *request, kvWriteChCapacity),
		opt:           opt,
		closer:        y.NewCloser(),
		elog:          trace.NewEventLog("Badger", "KV"),
		dirLockGuard:  dirLockGuard,
		valueDirGuard: valueDirLockGuard,
	}
	out.mt = skl.NewSkiplist(arenaSize(&opt))

	// newLevelsController potentially loads files in directory.
	if out.lc, err = newLevelsController(out); err != nil {
		return nil, err
	}

	lc := out.closer.Register("compactors")
	out.lc.startCompact(lc)

	lc = out.closer.Register("memtable")
	go out.flushMemtable(lc) // Need levels controller to be up.

	if err = out.vlog.Open(out, &opt); err != nil {
		return out, err
	}

	var item KVItem
	if err := out.Get(head, &item); err != nil {
		return nil, errors.Wrap(err, "Retrieving head")
	}
	val := item.Value()
	// lastUsedCasCounter will either be the value stored in !badger!head, or some subsequently
	// written value log entry that we replay.  (Subsequent value log entries might be _less_
	// than lastUsedCasCounter, if there was value log gc so we have to max() values while
	// replaying.)
	out.lastUsedCasCounter = item.casCounter

	var vptr valuePointer
	if len(val) > 0 {
		vptr.Decode(val)
	}

	lc = out.closer.Register("replay")
	go out.doWrites(lc)

	first := true
	fn := func(e Entry, vp valuePointer) error { // Function for replaying.
		if first {
			out.elog.Printf("First key=%s\n", e.Key)
		}
		first = false
		if out.lastUsedCasCounter < e.casCounter {
			out.lastUsedCasCounter = e.casCounter
		}

		if e.CASCounterCheck != 0 {
			oldValue, err := out.get(e.Key)
			if err != nil {
				return err
			}
			if oldValue.CASCounter != e.CASCounterCheck {
				return nil
			}
		}
		nk := make([]byte, len(e.Key))
		copy(nk, e.Key)
		var nv []byte
		meta := e.Meta
		if out.shouldWriteValueToLSM(e) {
			nv = make([]byte, len(e.Value))
			copy(nv, e.Value)
		} else {
			nv = make([]byte, 16)
			vp.Encode(nv)
			meta = meta | BitValuePointer
		}

		v := y.ValueStruct{
			Value:      nv,
			Meta:       meta,
			UserMeta:   e.UserMeta,
			CASCounter: e.casCounter,
		}
		for err := out.ensureRoomForWrite(); err != nil; err = out.ensureRoomForWrite() {
			out.elog.Printf("Replay: Making room for writes")
			time.Sleep(10 * time.Millisecond)
		}
		out.mt.Put(nk, v)
		return nil
	}
	if err = out.vlog.Replay(vptr, fn); err != nil {
		return out, err
	}
	lc.SignalAndWait() // Wait for replay to be applied first.

	out.writeCh = make(chan *request, kvWriteChCapacity)
	lc = out.closer.Register("writes")
	go out.doWrites(lc)

	lc = out.closer.Register("value-gc")
	go out.vlog.runGCInLoop(lc)

	valueDirLockGuard = nil
	dirLockGuard = nil
	return out, nil
}

// Close closes a KV. It's crucial to call it to ensure all the pending updates
// make their way to disk.
func (s *KV) Close() (err error) {
	defer func() {
		if guardErr := s.dirLockGuard.Release(); err == nil {
			err = errors.Wrap(guardErr, "KV.Close")
		}
		if s.valueDirGuard != nil {
			if guardErr := s.valueDirGuard.Release(); err == nil {
				err = errors.Wrap(guardErr, "KV.Close")
			}
		}
		// Fsync directories to ensure that lock file, and any other removed files whose directory
		// we haven't specifically fsynced, are guaranteed to have their directory entry removal
		// persisted to disk.
		if syncErr := syncDir(s.opt.Dir); err == nil {
			err = errors.Wrap(syncErr, "KV.Close")
		}
		if syncErr := syncDir(s.opt.ValueDir); err == nil {
			err = errors.Wrap(syncErr, "KV.Close")
		}
	}()

	s.elog.Printf("Closing database")
	// Stop value GC first.
	lc := s.closer.Get("value-gc")
	lc.SignalAndWait()

	// Stop writes next.
	lc = s.closer.Get("writes")
	lc.SignalAndWait()

	// Now close the value log.
	if err := s.vlog.Close(); err != nil {
		return errors.Wrapf(err, "KV.Close")
	}

	// Make sure that block writer is done pushing stuff into memtable!
	// Otherwise, you will have a race condition: we are trying to flush memtables
	// and remove them completely, while the block / memtable writer is still
	// trying to push stuff into the memtable. This will also resolve the value
	// offset problem: as we push into memtable, we update value offsets there.
	if s.mt.Size() > 0 {
		s.elog.Printf("Flushing memtable")
		for {
			pushedFlushTask := func() bool {
				s.Lock()
				defer s.Unlock()
				y.AssertTrue(s.mt != nil)
				select {
				case s.flushChan <- flushTask{s.mt, s.vptr}:
					s.imm = append(s.imm, s.mt) // Flusher will attempt to remove this from s.imm.
					s.mt = nil                  // Will segfault if we try writing!
					s.elog.Printf("pushed to flush chan\n")
					return true
				default:
					// If we fail to push, we need to unlock and wait for a short while.
					// The flushing operation needs to update s.imm. Otherwise, we have a deadlock.
					// TODO: Think about how to do this more cleanly, maybe without any locks.
				}
				return false
			}()
			if pushedFlushTask {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	s.flushChan <- flushTask{nil, valuePointer{}} // Tell flusher to quit.

	lc = s.closer.Get("memtable")
	lc.Wait()
	s.elog.Printf("Memtable flushed")

	lc = s.closer.Get("compactors")
	lc.SignalAndWait()
	s.elog.Printf("Compaction finished")

	if err := s.lc.close(); err != nil {
		return errors.Wrap(err, "KV.Close")
	}
	s.elog.Printf("Waiting for closer")
	s.closer.SignalAll()
	s.closer.WaitForAll()
	s.elog.Finish()

	return nil
}

const (
	lockFile = "LOCK"
)

// When you create or delete a file, you have to ensure the directory entry for the file is synced
// in order to guarantee the file is visible (if the system crashes).  (See the man page for fsync,
// or see https://github.com/coreos/etcd/issues/6368 for an example.)
func syncDir(dir string) error {
	f, err := os.Open(dir)
	if err != nil {
		return err
	}
	err = f.Sync()
	closeErr := f.Close()
	if err != nil {
		return err
	}
	return closeErr
}

// getMemtables returns the current memtables and get references.
func (s *KV) getMemTables() ([]*skl.Skiplist, func()) {
	s.RLock()
	defer s.RUnlock()

	tables := make([]*skl.Skiplist, len(s.imm)+1)

	// Get mutable memtable.
	tables[0] = s.mt
	tables[0].IncrRef()

	// Get immutable memtables.
	last := len(s.imm) - 1
	for i := range s.imm {
		tables[i+1] = s.imm[last-i]
		tables[i+1].IncrRef()
	}
	return tables, func() {
		for _, tbl := range tables {
			tbl.DecrRef()
		}
	}
}

func (s *KV) fillItem(item *KVItem) error {
	if (item.meta & BitDelete) != 0 {
		// Tombstone encountered.
		item.val = nil
		return nil
	}

	if item.slice == nil {
		item.slice = new(y.Slice)
	}
	if (item.meta & BitValuePointer) == 0 {
		item.val = item.slice.Resize(len(item.vptr))
		copy(item.val, item.vptr)
		return nil
	}

	var vp valuePointer
	vp.Decode(item.vptr)
	entry, err := s.vlog.Read(vp, item.slice)
	if err != nil {
		return errors.Wrapf(err, "Unable to read from value log: %+v", vp)
	}
	if (entry.Meta & BitDelete) != 0 { // Is a tombstone.
		item.val = nil
		return nil
	}
	item.val = entry.Value
	return nil
}

// get returns the value in memtable or disk for given key.
// Note that value will include meta byte.
func (s *KV) get(key []byte) (y.ValueStruct, error) {
	tables, decr := s.getMemTables() // Lock should be released.
	defer decr()

	y.NumGets.Add(1)
	for i := 0; i < len(tables); i++ {
		vs := tables[i].Get(key)
		y.NumMemtableGets.Add(1)
		if vs.Meta != 0 || vs.Value != nil {
			return vs, nil
		}
	}
	return s.lc.get(key)
}

// Get looks for key and returns a KVItem.
// If key is not found, item.Value() is nil.
func (s *KV) Get(key []byte, item *KVItem) error {
	vs, err := s.get(key)
	if err != nil {
		return errors.Wrapf(err, "KV::Get key: %q", key)
	}
	if item.slice == nil {
		item.slice = new(y.Slice)
	}
	item.meta = vs.Meta
	item.userMeta = vs.UserMeta
	item.casCounter = vs.CASCounter
	item.key = key
	item.vptr = vs.Value

	if err := s.fillItem(item); err != nil {
		return errors.Wrapf(err, "KV::Get key: %q", key)
	}
	return nil
}

// Exists looks if a key exists. Returns true if the
// key exists otherwises return false. if err is not nil an error occurs during
// the key lookup and the existence of the key is unknown
func (s *KV) Exists(key []byte) (bool, error) {
	vs, err := s.get(key)
	if err != nil {
		return false, err
	}

	if vs.Value == nil && vs.Meta == 0 {
		return false, nil
	}

	if (vs.Meta & BitDelete) != 0 {
		// Tombstone encountered.
		return false, nil
	}

	return true, nil
}

func (s *KV) updateOffset(ptrs []valuePointer) {
	var ptr valuePointer
	for i := len(ptrs) - 1; i >= 0; i-- {
		p := ptrs[i]
		if !p.IsZero() {
			ptr = p
			break
		}
	}
	if ptr.IsZero() {
		return
	}

	s.Lock()
	defer s.Unlock()
	y.AssertTrue(!ptr.Less(s.vptr))
	s.vptr = ptr
}

var requestPool = sync.Pool{
	New: func() interface{} {
		return new(request)
	},
}

func (s *KV) shouldWriteValueToLSM(e Entry) bool {
	return len(e.Value) < s.opt.ValueThreshold
}

func (s *KV) writeToLSM(b *request) error {
	var offsetBuf [10]byte
	if len(b.Ptrs) != len(b.Entries) {
		return errors.Errorf("Ptrs and Entries don't match: %+v", b)
	}

	for i, entry := range b.Entries {
		entry.Error = nil
		if entry.CASCounterCheck != 0 {
			oldValue, err := s.get(entry.Key)
			if err != nil {
				return errors.Wrap(err, "writeToLSM")
			}
			// No need to decode existing value. Just need old CAS counter.
			if oldValue.CASCounter != entry.CASCounterCheck {
				entry.Error = CasMismatch
				continue
			}
		}

		if entry.Meta == BitSetIfAbsent {
			// Someone else might have written a value, so lets check again if key exists.
			exists, err := s.Exists(entry.Key)
			if err != nil {
				return err
			}
			// Value already exists, don't write.
			if exists {
				entry.Error = KeyExists
				continue
			}
		}

		if s.shouldWriteValueToLSM(*entry) { // Will include deletion / tombstone case.
			s.mt.Put(entry.Key,
				y.ValueStruct{
					Value:      entry.Value,
					Meta:       entry.Meta,
					UserMeta:   entry.UserMeta,
					CASCounter: entry.casCounter})
		} else {
			s.mt.Put(entry.Key,
				y.ValueStruct{
					Value:      b.Ptrs[i].Encode(offsetBuf[:]),
					Meta:       entry.Meta | BitValuePointer,
					UserMeta:   entry.UserMeta,
					CASCounter: entry.casCounter})
		}
	}
	return nil
}

// lastCASCounter returns the last-used cas counter.
func (s *KV) lastCASCounter() uint64 {
	return atomic.LoadUint64(&s.lastUsedCasCounter)
}

// newCASCounters generates a set of unique CAS counters -- the interval [x, x + howMany) where x
// is the return value.
func (s *KV) newCASCounters(howMany uint64) uint64 {
	last := atomic.AddUint64(&s.lastUsedCasCounter, howMany)
	return last - howMany + 1
}

// writeRequests is called serially by only one goroutine.
func (s *KV) writeRequests(reqs []*request) error {
	if len(reqs) == 0 {
		return nil
	}

	done := func(err error) {
		for _, r := range reqs {
			r.Err = err
			r.Wg.Done()
		}
	}

	s.elog.Printf("writeRequests called. Writing to value log")

	// CAS counter for all operations has to go onto value log. Otherwise, if it is just in
	// memtable for a long time, and following CAS operations use that as a check, when
	// replaying, we will think that these CAS operations should fail, when they are actually
	// valid.

	// There is code (in flushMemtable) whose correctness depends on us generating CAS Counter
	// values _before_ we modify s.vptr here.
	for _, req := range reqs {
		counterBase := s.newCASCounters(uint64(len(req.Entries)))
		for i, e := range req.Entries {
			e.casCounter = counterBase + uint64(i)
		}
	}
	err := s.vlog.write(reqs)
	if err != nil {
		done(err)
		return err
	}

	s.elog.Printf("Writing to memtable")
	var count int
	for _, b := range reqs {
		if len(b.Entries) == 0 {
			continue
		}
		count += len(b.Entries)
		for err := s.ensureRoomForWrite(); err != nil; err = s.ensureRoomForWrite() {
			s.elog.Printf("Making room for writes")
			// We need to poll a bit because both hasRoomForWrite and the flusher need access to s.imm.
			// When flushChan is full and you are blocked there, and the flusher is trying to update s.imm,
			// you will get a deadlock.
			time.Sleep(10 * time.Millisecond)
		}
		if err != nil {
			done(err)
			return errors.Wrap(err, "writeRequests")
		}
		if err := s.writeToLSM(b); err != nil {
			done(err)
			return errors.Wrap(err, "writeRequests")
		}
		s.updateOffset(b.Ptrs)
	}
	done(nil)
	s.elog.Printf("%d entries written", count)
	return nil
}

func writeRequestsOrLogError(s *KV, reqs []*request) {
	if err := s.writeRequests(reqs); err != nil {
		log.Printf("ERROR in Badger::writeRequests: %v", err)
	}
}

func (s *KV) doWrites(lc *y.LevelCloser) {
	defer lc.Done()

	reqs := make([]*request, 0, 10)
	for {
		var r *request
		select {
		case r = <-s.writeCh:
		case <-lc.HasBeenClosed():
			goto closedCase
		}

		for {
			reqs = append(reqs, r)
			if len(reqs) == kvWriteChCapacity {
				goto defaultCase
			}
			select {
			case r = <-s.writeCh:
			case <-lc.HasBeenClosed():
				goto closedCase
			default:
				goto defaultCase
			}
		}

	closedCase:
		close(s.writeCh)

		for r := range s.writeCh { // Flush the channel.
			reqs = append(reqs, r)
		}
		writeRequestsOrLogError(s, reqs)
		return

	defaultCase:
		writeRequestsOrLogError(s, reqs)
		reqs = reqs[:0]
	}
}

func (s *KV) sendToWriteCh(entries []*Entry) []*request {
	var reqs []*request
	var size int64
	var b *request
	for _, entry := range entries {
		if b == nil {
			b = requestPool.Get().(*request)
			b.Entries = b.Entries[:0]
			b.Wg = sync.WaitGroup{}
			b.Wg.Add(1)
		}
		size += int64(s.opt.estimateSize(entry))
		b.Entries = append(b.Entries, entry)
		if size >= s.opt.maxBatchSize {
			s.writeCh <- b
			y.NumPuts.Add(int64(len(b.Entries)))
			reqs = append(reqs, b)
			size = 0
			b = nil
		}
	}

	if size > 0 {
		s.writeCh <- b
		y.NumPuts.Add(int64(len(b.Entries)))
		reqs = append(reqs, b)
	}
	return reqs
}

// BatchSet applies a list of badger.Entry. Errors are set on each Entry individually.
//   for _, e := range entries {
//      Check(e.Error)
//   }
func (s *KV) BatchSet(entries []*Entry) error {
	reqs := s.sendToWriteCh(entries)

	var err error
	for _, req := range reqs {
		req.Wg.Wait()
		if req.Err != nil {
			err = req.Err
		}
		requestPool.Put(req)
	}
	return err
}

// BatchSetAsync is the asynchronous version of BatchSet. It accepts a callback function
// which is called when all the sets are complete. Any error during execution is passed as an
// argument to the callback function.
func (s *KV) BatchSetAsync(entries []*Entry, f func(error)) {
	reqs := s.sendToWriteCh(entries)

	go func() {
		var err error
		for _, req := range reqs {
			req.Wg.Wait()
			if req.Err != nil {
				err = req.Err
			}
			requestPool.Put(req)
		}
		// All writes complete, lets call the callback function now.
		f(err)
	}()
}

// Set sets the provided value for a given key. If key is not present, it is created.
// If it is present, the existing value is overwritten with the one provided.
func (s *KV) Set(key, val []byte, userMeta byte) error {
	e := &Entry{
		Key:      key,
		Value:    val,
		UserMeta: userMeta,
	}
	return s.BatchSet([]*Entry{e})
}

// SetAsync is the asynchronous version of Set. It accepts a callback function which is called
// when the set is complete. Any error encountered during execution is passed as an argument
// to the callback function.
func (s *KV) SetAsync(key, val []byte, userMeta byte, f func(error)) {
	e := &Entry{
		Key:      key,
		Value:    val,
		UserMeta: userMeta,
	}
	s.BatchSetAsync([]*Entry{e}, f)
}

// SetIfAbsent sets value of key if key is not present.
// If it is present, it returns the KeyExists error.
func (s *KV) SetIfAbsent(key, val []byte, userMeta byte) error {
	exists, err := s.Exists(key)
	if err != nil {
		return err
	}
	// Found the key, return KeyExists
	if exists {
		return KeyExists
	}

	e := &Entry{
		Key:      key,
		Meta:     BitSetIfAbsent,
		Value:    val,
		UserMeta: userMeta,
	}
	if err := s.BatchSet([]*Entry{e}); err != nil {
		return err
	}
	return e.Error
}

// EntriesSet adds a Set to the list of entries.
// Exposing this so that user does not have to specify the Entry directly.
func EntriesSet(s []*Entry, key, val []byte) []*Entry {
	return append(s, &Entry{
		Key:   key,
		Value: val,
	})
}

// CompareAndSet sets the given value, ensuring that the no other Set operation has happened,
// since last read. If the key has a different casCounter, this would not update the key
// and return an error.
func (s *KV) CompareAndSet(key []byte, val []byte, casCounter uint64) error {
	e := &Entry{
		Key:             key,
		Value:           val,
		CASCounterCheck: casCounter,
	}
	if err := s.BatchSet([]*Entry{e}); err != nil {
		return err
	}
	return e.Error
}

func (s *KV) compareAsync(e *Entry, f func(error)) {
	b := requestPool.Get().(*request)
	b.Wg = sync.WaitGroup{}
	b.Wg.Add(1)
	s.writeCh <- b

	go func() {
		b.Wg.Wait()
		if b.Err != nil {
			f(b.Err)
			return
		}
		f(e.Error)
	}()
}

// CompareAndSetAsync is the asynchronous version of CompareAndSet. It accepts a callback function
// which is called when the CompareAndSet completes. Any error encountered during execution is
// passed as an argument to the callback function.
func (s *KV) CompareAndSetAsync(key []byte, val []byte, casCounter uint64, f func(error)) {
	e := &Entry{
		Key:             key,
		Value:           val,
		CASCounterCheck: casCounter,
	}
	s.compareAsync(e, f)
}

// Delete deletes a key.
// Exposing this so that user does not have to specify the Entry directly.
// For example, BitDelete seems internal to badger.
func (s *KV) Delete(key []byte) error {
	e := &Entry{
		Key:  key,
		Meta: BitDelete,
	}

	return s.BatchSet([]*Entry{e})
}

// DeleteAsync is the asynchronous version of Delete. It calls the callback function after deletion
// is complete. Any error encountered during the execution is passed as an argument to the
// callback function.
func (s *KV) DeleteAsync(key []byte, f func(error)) {
	e := &Entry{
		Key:  key,
		Meta: BitDelete,
	}
	s.BatchSetAsync([]*Entry{e}, f)
}

// EntriesDelete adds a Del to the list of entries.
func EntriesDelete(s []*Entry, key []byte) []*Entry {
	return append(s, &Entry{
		Key:  key,
		Meta: BitDelete,
	})
}

// CompareAndDelete deletes a key ensuring that it has not been changed since last read.
// If existing key has different casCounter, this would not delete the key and return an error.
func (s *KV) CompareAndDelete(key []byte, casCounter uint64) error {
	e := &Entry{
		Key:             key,
		Meta:            BitDelete,
		CASCounterCheck: casCounter,
	}
	if err := s.BatchSet([]*Entry{e}); err != nil {
		return err
	}
	return e.Error
}

// CompareAndDeleteAsync is the asynchronous version of CompareAndDelete. It accepts a callback
// function which is called when the CompareAndDelete completes. Any error encountered during
// execution is passed as an argument to the callback function.
func (s *KV) CompareAndDeleteAsync(key []byte, casCounter uint64, f func(error)) {
	e := &Entry{
		Key:             key,
		Meta:            BitDelete,
		CASCounterCheck: casCounter,
	}
	s.compareAsync(e, f)
}

var ErrNoRoom = errors.New("No room for write")

// ensureRoomForWrite is always called serially.
func (s *KV) ensureRoomForWrite() error {
	var err error
	s.Lock()
	defer s.Unlock()
	if s.mt.Size() < s.opt.MaxTableSize {
		return nil
	}

	y.AssertTrue(s.mt != nil) // A nil mt indicates that KV is being closed.
	select {
	case s.flushChan <- flushTask{s.mt, s.vptr}:
		s.elog.Printf("Flushing value log to disk if async mode.")
		// Ensure value log is synced to disk so this memtable's contents wouldn't be lost.
		err = s.vlog.sync()
		if err != nil {
			return err
		}

		s.elog.Printf("Flushing memtable, mt.size=%d size of flushChan: %d\n",
			s.mt.Size(), len(s.flushChan))
		// We manage to push this task. Let's modify imm.
		s.imm = append(s.imm, s.mt)
		s.mt = skl.NewSkiplist(arenaSize(&s.opt))
		// New memtable is empty. We certainly have room.
		return nil
	default:
		// We need to do this to unlock and allow the flusher to modify imm.
		return ErrNoRoom
	}
}

func arenaSize(opt *Options) int64 {
	return opt.MaxTableSize + opt.maxBatchSize
}

// WriteLevel0Table flushes memtable. It drops deleteValues.
func writeLevel0Table(s *skl.Skiplist, f *os.File) error {
	iter := s.NewIterator()
	defer iter.Close()
	b := table.NewTableBuilder()
	defer b.Close()
	for iter.SeekToFirst(); iter.Valid(); iter.Next() {
		if err := b.Add(iter.Key(), iter.Value()); err != nil {
			return err
		}
	}
	var buf [2]byte // Level 0. Leave it initialized as 0.
	_, err := f.Write(b.Finish(buf[:]))
	return err
}

type flushTask struct {
	mt   *skl.Skiplist
	vptr valuePointer
}

func (s *KV) flushMemtable(lc *y.LevelCloser) error {
	defer lc.Done()

	for ft := range s.flushChan {
		if ft.mt == nil {
			return nil
		}

		if !ft.vptr.IsZero() {
			s.elog.Printf("Storing offset: %+v\n", ft.vptr)
			offset := make([]byte, 10)
			ft.vptr.Encode(offset)
			// CAS counter is needed and is desirable -- it's the first value log entry
			// we replay, so to speak, perhaps the only, and we use it to re-initialize
			// the CAS counter.
			//
			// The write loop generates CAS counter values _before_ it sets vptr.  It
			// is crucial that we read the cas counter here _after_ reading vptr.  That
			// way, our value here is guaranteed to be >= the CASCounter values written
			// before vptr (because they don't get replayed).
			ft.mt.Put(head, y.ValueStruct{Value: offset, CASCounter: s.lastCASCounter()})
		}
		fileID, _ := s.lc.reserveFileIDs(1)
		fd, err := y.OpenSyncedFile(table.NewFilename(fileID, s.opt.Dir), true)
		if err != nil {
			return y.Wrap(err)
		}

		// Don't block just to sync the directory entry.
		dirSyncCh := make(chan error)
		go func() { dirSyncCh <- syncDir(s.opt.Dir) }()

		err = writeLevel0Table(ft.mt, fd)
		dirSyncErr := <-dirSyncCh

		if err != nil {
			s.elog.Errorf("ERROR while writing to level 0: %v", err)
			return err
		}
		if dirSyncErr != nil {
			s.elog.Errorf("ERROR while syncing level directory: %v", dirSyncErr)
			return err
		}

		tbl, err := table.OpenTable(fd, s.opt.MapTablesTo)
		if err != nil {
			s.elog.Printf("ERROR while opening table: %v", err)
			return err
		}
		// We own a ref on tbl.
		s.lc.addLevel0Table(tbl) // This will incrRef.
		tbl.DecrRef()            // Releases our ref.

		// Update s.imm. Need a lock.
		s.Lock()
		y.AssertTrue(ft.mt == s.imm[0]) //For now, single threaded.
		s.imm = s.imm[1:]
		ft.mt.DecrRef() // Return memory.
		s.Unlock()
	}
	return nil
}

func exists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return true, err
}
