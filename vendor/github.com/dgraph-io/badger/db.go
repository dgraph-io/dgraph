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
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"expvar"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dgraph-io/badger/options"
	"github.com/dgraph-io/badger/pb"
	"github.com/dgraph-io/badger/skl"
	"github.com/dgraph-io/badger/table"
	"github.com/dgraph-io/badger/y"
	humanize "github.com/dustin/go-humanize"
	"github.com/pkg/errors"
	"golang.org/x/net/trace"
)

var (
	badgerPrefix      = []byte("!badger!")        // Prefix for internal keys used by badger.
	head              = []byte("!badger!head")    // For storing value offset for replay.
	txnKey            = []byte("!badger!txn")     // For indicating end of entries in txn.
	badgerMove        = []byte("!badger!move")    // For key-value pairs which got moved during GC.
	lfDiscardStatsKey = []byte("!badger!discard") // For storing lfDiscardStats
)

type closers struct {
	updateSize *y.Closer
	compactors *y.Closer
	memtable   *y.Closer
	writes     *y.Closer
	valueGC    *y.Closer
	pub        *y.Closer
}

type callback func(kv *pb.KVList)

// DB provides the various functions required to interact with Badger.
// DB is thread-safe.
type DB struct {
	sync.RWMutex // Guards list of inmemory tables, not individual reads and writes.

	dirLockGuard *directoryLockGuard
	// nil if Dir and ValueDir are the same
	valueDirGuard *directoryLockGuard

	closers   closers
	elog      trace.EventLog
	mt        *skl.Skiplist   // Our latest (actively written) in-memory table
	imm       []*skl.Skiplist // Add here only AFTER pushing to flushChan.
	opt       Options
	manifest  *manifestFile
	lc        *levelsController
	vlog      valueLog
	vhead     valuePointer // less than or equal to a pointer to the last vlog value put into mt
	writeCh   chan *request
	flushChan chan flushTask // For flushing memtables.
	closeOnce sync.Once      // For closing DB only once.

	// Number of log rotates since the last memtable flush. We will access this field via atomic
	// functions. Since we are not going to use any 64bit atomic functions, there is no need for
	// 64 bit alignment of this struct(see #311).
	logRotates int32

	blockWrites int32

	orc *oracle

	pub *publisher
}

const (
	kvWriteChCapacity = 1000
)

func (db *DB) replayFunction() func(Entry, valuePointer) error {
	type txnEntry struct {
		nk []byte
		v  y.ValueStruct
	}

	var txn []txnEntry
	var lastCommit uint64

	toLSM := func(nk []byte, vs y.ValueStruct) {
		for err := db.ensureRoomForWrite(); err != nil; err = db.ensureRoomForWrite() {
			db.elog.Printf("Replay: Making room for writes")
			time.Sleep(10 * time.Millisecond)
		}
		db.mt.Put(nk, vs)
	}

	first := true
	return func(e Entry, vp valuePointer) error { // Function for replaying.
		if first {
			db.elog.Printf("First key=%q\n", e.Key)
		}
		first = false

		if db.orc.nextTxnTs < y.ParseTs(e.Key) {
			db.orc.nextTxnTs = y.ParseTs(e.Key)
		}

		nk := make([]byte, len(e.Key))
		copy(nk, e.Key)
		var nv []byte
		meta := e.meta
		if db.shouldWriteValueToLSM(e) {
			nv = make([]byte, len(e.Value))
			copy(nv, e.Value)
		} else {
			nv = make([]byte, vptrSize)
			vp.Encode(nv)
			meta = meta | bitValuePointer
		}

		v := y.ValueStruct{
			Value:     nv,
			Meta:      meta,
			UserMeta:  e.UserMeta,
			ExpiresAt: e.ExpiresAt,
		}

		if e.meta&bitFinTxn > 0 {
			txnTs, err := strconv.ParseUint(string(e.Value), 10, 64)
			if err != nil {
				return errors.Wrapf(err, "Unable to parse txn fin: %q", e.Value)
			}
			y.AssertTrue(lastCommit == txnTs)
			y.AssertTrue(len(txn) > 0)
			// Got the end of txn. Now we can store them.
			for _, t := range txn {
				toLSM(t.nk, t.v)
			}
			txn = txn[:0]
			lastCommit = 0

		} else if e.meta&bitTxn > 0 {
			txnTs := y.ParseTs(nk)
			if lastCommit == 0 {
				lastCommit = txnTs
			}
			if lastCommit != txnTs {
				db.opt.Warningf("Found an incomplete txn at timestamp %d. Discarding it.\n",
					lastCommit)
				txn = txn[:0]
				lastCommit = txnTs
			}
			te := txnEntry{nk: nk, v: v}
			txn = append(txn, te)

		} else {
			// This entry is from a rewrite.
			toLSM(nk, v)

			// We shouldn't get this entry in the middle of a transaction.
			y.AssertTrue(lastCommit == 0)
			y.AssertTrue(len(txn) == 0)
		}
		return nil
	}
}

// Open returns a new DB object.
func Open(opt Options) (db *DB, err error) {
	opt.maxBatchSize = (15 * opt.MaxTableSize) / 100
	opt.maxBatchCount = opt.maxBatchSize / int64(skl.MaxNodeSize)

	if opt.ValueThreshold > ValueThresholdLimit {
		return nil, ErrValueThreshold
	}

	if opt.ReadOnly {
		// Can't truncate if the DB is read only.
		opt.Truncate = false
		// Do not perform compaction in read only mode.
		opt.CompactL0OnClose = false
	}

	for _, path := range []string{opt.Dir, opt.ValueDir} {
		dirExists, err := exists(path)
		if err != nil {
			return nil, y.Wrapf(err, "Invalid Dir: %q", path)
		}
		if !dirExists {
			if opt.ReadOnly {
				return nil, errors.Errorf("Cannot find directory %q for read-only open", path)
			}
			// Try to create the directory
			err = os.Mkdir(path, 0700)
			if err != nil {
				return nil, y.Wrapf(err, "Error Creating Dir: %q", path)
			}
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
	var dirLockGuard, valueDirLockGuard *directoryLockGuard
	dirLockGuard, err = acquireDirectoryLock(opt.Dir, lockFile, opt.ReadOnly)
	if err != nil {
		return nil, err
	}
	defer func() {
		if dirLockGuard != nil {
			_ = dirLockGuard.release()
		}
	}()
	if absValueDir != absDir {
		valueDirLockGuard, err = acquireDirectoryLock(opt.ValueDir, lockFile, opt.ReadOnly)
		if err != nil {
			return nil, err
		}
		defer func() {
			if valueDirLockGuard != nil {
				_ = valueDirLockGuard.release()
			}
		}()
	}
	if !(opt.ValueLogFileSize <= 2<<30 && opt.ValueLogFileSize >= 1<<20) {
		return nil, ErrValueLogSize
	}
	if !(opt.ValueLogLoadingMode == options.FileIO ||
		opt.ValueLogLoadingMode == options.MemoryMap) {
		return nil, ErrInvalidLoadingMode
	}
	manifestFile, manifest, err := openOrCreateManifestFile(opt.Dir, opt.ReadOnly)
	if err != nil {
		return nil, err
	}
	defer func() {
		if manifestFile != nil {
			_ = manifestFile.close()
		}
	}()

	db = &DB{
		imm:           make([]*skl.Skiplist, 0, opt.NumMemtables),
		flushChan:     make(chan flushTask, opt.NumMemtables),
		writeCh:       make(chan *request, kvWriteChCapacity),
		opt:           opt,
		manifest:      manifestFile,
		elog:          trace.NewEventLog("Badger", "DB"),
		dirLockGuard:  dirLockGuard,
		valueDirGuard: valueDirLockGuard,
		orc:           newOracle(opt),
		pub:           newPublisher(),
	}

	// Calculate initial size.
	db.calculateSize()
	db.closers.updateSize = y.NewCloser(1)
	go db.updateSize(db.closers.updateSize)
	db.mt = skl.NewSkiplist(arenaSize(opt))

	// newLevelsController potentially loads files in directory.
	if db.lc, err = newLevelsController(db, &manifest); err != nil {
		return nil, err
	}

	if !opt.ReadOnly {
		db.closers.compactors = y.NewCloser(1)
		db.lc.startCompact(db.closers.compactors)

		db.closers.memtable = y.NewCloser(1)
		go func() {
			_ = db.flushMemtable(db.closers.memtable) // Need levels controller to be up.
		}()
	}

	headKey := y.KeyWithTs(head, math.MaxUint64)
	// Need to pass with timestamp, lsm get removes the last 8 bytes and compares key
	vs, err := db.get(headKey)
	if err != nil {
		return nil, errors.Wrap(err, "Retrieving head")
	}
	db.orc.nextTxnTs = vs.Version
	var vptr valuePointer
	if len(vs.Value) > 0 {
		vptr.Decode(vs.Value)
	}

	replayCloser := y.NewCloser(1)
	go db.doWrites(replayCloser)

	if err = db.vlog.open(db, vptr, db.replayFunction()); err != nil {
		return db, err
	}
	replayCloser.SignalAndWait() // Wait for replay to be applied first.

	// Let's advance nextTxnTs to one more than whatever we observed via
	// replaying the logs.
	db.orc.txnMark.Done(db.orc.nextTxnTs)
	// In normal mode, we must update readMark so older versions of keys can be removed during
	// compaction when run in offline mode via the flatten tool.
	db.orc.readMark.Done(db.orc.nextTxnTs)
	db.orc.incrementNextTs()

	db.writeCh = make(chan *request, kvWriteChCapacity)
	db.closers.writes = y.NewCloser(1)
	go db.doWrites(db.closers.writes)

	db.closers.valueGC = y.NewCloser(1)
	go db.vlog.waitOnGC(db.closers.valueGC)

	db.closers.pub = y.NewCloser(1)
	go db.pub.listenForUpdates(db.closers.pub)

	valueDirLockGuard = nil
	dirLockGuard = nil
	manifestFile = nil
	return db, nil
}

// Close closes a DB. It's crucial to call it to ensure all the pending updates make their way to
// disk. Calling DB.Close() multiple times would still only close the DB once.
func (db *DB) Close() error {
	var err error
	db.closeOnce.Do(func() {
		err = db.close()
	})
	return err
}

func (db *DB) close() (err error) {
	db.elog.Printf("Closing database")

	if err := db.vlog.flushDiscardStats(); err != nil {
		return errors.Wrap(err, "failed to flush discard stats")
	}

	atomic.StoreInt32(&db.blockWrites, 1)

	// Stop value GC first.
	db.closers.valueGC.SignalAndWait()

	// Stop writes next.
	db.closers.writes.SignalAndWait()

	db.closers.pub.SignalAndWait()

	// Now close the value log.
	if vlogErr := db.vlog.Close(); vlogErr != nil {
		err = errors.Wrap(vlogErr, "DB.Close")
	}

	// Make sure that block writer is done pushing stuff into memtable!
	// Otherwise, you will have a race condition: we are trying to flush memtables
	// and remove them completely, while the block / memtable writer is still
	// trying to push stuff into the memtable. This will also resolve the value
	// offset problem: as we push into memtable, we update value offsets there.
	if !db.mt.Empty() {
		db.elog.Printf("Flushing memtable")
		for {
			pushedFlushTask := func() bool {
				db.Lock()
				defer db.Unlock()
				y.AssertTrue(db.mt != nil)
				select {
				case db.flushChan <- flushTask{mt: db.mt, vptr: db.vhead}:
					db.imm = append(db.imm, db.mt) // Flusher will attempt to remove this from s.imm.
					db.mt = nil                    // Will segfault if we try writing!
					db.elog.Printf("pushed to flush chan\n")
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
	db.stopCompactions()

	// Force Compact L0
	// We don't need to care about cstatus since no parallel compaction is running.
	if db.opt.CompactL0OnClose {
		err := db.lc.doCompact(compactionPriority{level: 0, score: 1.73})
		switch err {
		case errFillTables:
			// This error only means that there might be enough tables to do a compaction. So, we
			// should not report it to the end user to avoid confusing them.
		case nil:
			db.opt.Infof("Force compaction on level 0 done")
		default:
			db.opt.Warningf("While forcing compaction on level 0: %v", err)
		}
	}

	if lcErr := db.lc.close(); err == nil {
		err = errors.Wrap(lcErr, "DB.Close")
	}
	db.elog.Printf("Waiting for closer")
	db.closers.updateSize.SignalAndWait()
	db.orc.Stop()

	db.elog.Finish()

	if db.dirLockGuard != nil {
		if guardErr := db.dirLockGuard.release(); err == nil {
			err = errors.Wrap(guardErr, "DB.Close")
		}
	}
	if db.valueDirGuard != nil {
		if guardErr := db.valueDirGuard.release(); err == nil {
			err = errors.Wrap(guardErr, "DB.Close")
		}
	}
	if manifestErr := db.manifest.close(); err == nil {
		err = errors.Wrap(manifestErr, "DB.Close")
	}

	// Fsync directories to ensure that lock file, and any other removed files whose directory
	// we haven't specifically fsynced, are guaranteed to have their directory entry removal
	// persisted to disk.
	if syncErr := syncDir(db.opt.Dir); err == nil {
		err = errors.Wrap(syncErr, "DB.Close")
	}
	if syncErr := syncDir(db.opt.ValueDir); err == nil {
		err = errors.Wrap(syncErr, "DB.Close")
	}

	return err
}

const (
	lockFile = "LOCK"
)

// Sync syncs database content to disk. This function provides
// more control to user to sync data whenever required.
func (db *DB) Sync() error {
	return db.vlog.sync(math.MaxUint32)
}

// getMemtables returns the current memtables and get references.
func (db *DB) getMemTables() ([]*skl.Skiplist, func()) {
	db.RLock()
	defer db.RUnlock()

	tables := make([]*skl.Skiplist, len(db.imm)+1)

	// Get mutable memtable.
	tables[0] = db.mt
	tables[0].IncrRef()

	// Get immutable memtables.
	last := len(db.imm) - 1
	for i := range db.imm {
		tables[i+1] = db.imm[last-i]
		tables[i+1].IncrRef()
	}
	return tables, func() {
		for _, tbl := range tables {
			tbl.DecrRef()
		}
	}
}

// get returns the value in memtable or disk for given key.
// Note that value will include meta byte.
//
// IMPORTANT: We should never write an entry with an older timestamp for the same key, We need to
// maintain this invariant to search for the latest value of a key, or else we need to search in all
// tables and find the max version among them.  To maintain this invariant, we also need to ensure
// that all versions of a key are always present in the same table from level 1, because compaction
// can push any table down.
//
// Update (Sep 22, 2018): To maintain the above invariant, and to allow keys to be moved from one
// value log to another (while reclaiming space during value log GC), we have logically moved this
// need to write "old versions after new versions" to the badgerMove keyspace. Thus, for normal
// gets, we can stop going down the LSM tree once we find any version of the key (note however that
// we will ALWAYS skip versions with ts greater than the key version).  However, if that key has
// been moved, then for the corresponding movekey, we'll look through all the levels of the tree
// to ensure that we pick the highest version of the movekey present.
func (db *DB) get(key []byte) (y.ValueStruct, error) {
	tables, decr := db.getMemTables() // Lock should be released.
	defer decr()

	var maxVs *y.ValueStruct
	var version uint64
	if bytes.HasPrefix(key, badgerMove) {
		// If we are checking badgerMove key, we should look into all the
		// levels, so we can pick up the newer versions, which might have been
		// compacted down the tree.
		maxVs = &y.ValueStruct{}
		version = y.ParseTs(key)
	}

	y.NumGets.Add(1)
	for i := 0; i < len(tables); i++ {
		vs := tables[i].Get(key)
		y.NumMemtableGets.Add(1)
		if vs.Meta == 0 && vs.Value == nil {
			continue
		}
		// Found a version of the key. For user keyspace, return immediately. For move keyspace,
		// continue iterating, unless we found a version == given key version.
		if maxVs == nil || vs.Version == version {
			return vs, nil
		}
		if maxVs.Version < vs.Version {
			*maxVs = vs
		}
	}
	return db.lc.get(key, maxVs)
}

func (db *DB) updateHead(ptrs []valuePointer) {
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

	db.Lock()
	defer db.Unlock()
	y.AssertTrue(!ptr.Less(db.vhead))
	db.vhead = ptr
}

var requestPool = sync.Pool{
	New: func() interface{} {
		return new(request)
	},
}

func (db *DB) shouldWriteValueToLSM(e Entry) bool {
	return len(e.Value) < db.opt.ValueThreshold
}

func (db *DB) writeToLSM(b *request) error {
	if len(b.Ptrs) != len(b.Entries) {
		return errors.Errorf("Ptrs and Entries don't match: %+v", b)
	}

	for i, entry := range b.Entries {
		if entry.meta&bitFinTxn != 0 {
			continue
		}
		if db.shouldWriteValueToLSM(*entry) { // Will include deletion / tombstone case.
			db.mt.Put(entry.Key,
				y.ValueStruct{
					Value:     entry.Value,
					Meta:      entry.meta,
					UserMeta:  entry.UserMeta,
					ExpiresAt: entry.ExpiresAt,
				})
		} else {
			var offsetBuf [vptrSize]byte
			db.mt.Put(entry.Key,
				y.ValueStruct{
					Value:     b.Ptrs[i].Encode(offsetBuf[:]),
					Meta:      entry.meta | bitValuePointer,
					UserMeta:  entry.UserMeta,
					ExpiresAt: entry.ExpiresAt,
				})
		}
	}
	return nil
}

// writeRequests is called serially by only one goroutine.
func (db *DB) writeRequests(reqs []*request) error {
	if len(reqs) == 0 {
		return nil
	}

	done := func(err error) {
		for _, r := range reqs {
			r.Err = err
			r.Wg.Done()
		}
	}
	db.elog.Printf("writeRequests called. Writing to value log")

	err := db.vlog.write(reqs)
	if err != nil {
		done(err)
		return err
	}

	db.elog.Printf("Sending updates to subscribers")
	db.pub.sendUpdates(reqs)
	db.elog.Printf("Writing to memtable")
	var count int
	for _, b := range reqs {
		if len(b.Entries) == 0 {
			continue
		}
		count += len(b.Entries)
		var i uint64
		for err = db.ensureRoomForWrite(); err == errNoRoom; err = db.ensureRoomForWrite() {
			i++
			if i%100 == 0 {
				db.elog.Printf("Making room for writes")
			}
			// We need to poll a bit because both hasRoomForWrite and the flusher need access to s.imm.
			// When flushChan is full and you are blocked there, and the flusher is trying to update s.imm,
			// you will get a deadlock.
			time.Sleep(10 * time.Millisecond)
		}
		if err != nil {
			done(err)
			return errors.Wrap(err, "writeRequests")
		}
		if err := db.writeToLSM(b); err != nil {
			done(err)
			return errors.Wrap(err, "writeRequests")
		}
		db.updateHead(b.Ptrs)
	}
	done(nil)
	db.elog.Printf("%d entries written", count)
	return nil
}

func (db *DB) sendToWriteCh(entries []*Entry) (*request, error) {
	if atomic.LoadInt32(&db.blockWrites) == 1 {
		return nil, ErrBlockedWrites
	}
	var count, size int64
	for _, e := range entries {
		size += int64(e.estimateSize(db.opt.ValueThreshold))
		count++
	}
	if count >= db.opt.maxBatchCount || size >= db.opt.maxBatchSize {
		return nil, ErrTxnTooBig
	}

	// We can only service one request because we need each txn to be stored in a contigous section.
	// Txns should not interleave among other txns or rewrites.
	req := requestPool.Get().(*request)
	req.Entries = entries
	req.Wg = sync.WaitGroup{}
	req.Wg.Add(1)
	req.IncrRef()     // for db write
	req.IncrRef()     // for publisher updates
	db.writeCh <- req // Handled in doWrites.
	y.NumPuts.Add(int64(len(entries)))

	return req, nil
}

func (db *DB) doWrites(lc *y.Closer) {
	defer lc.Done()
	pendingCh := make(chan struct{}, 1)

	writeRequests := func(reqs []*request) {
		if err := db.writeRequests(reqs); err != nil {
			db.opt.Errorf("writeRequests: %v", err)
		}
		<-pendingCh
	}

	// This variable tracks the number of pending writes.
	reqLen := new(expvar.Int)
	y.PendingWrites.Set(db.opt.Dir, reqLen)

	reqs := make([]*request, 0, 10)
	for {
		var r *request
		select {
		case r = <-db.writeCh:
		case <-lc.HasBeenClosed():
			goto closedCase
		}

		for {
			reqs = append(reqs, r)
			reqLen.Set(int64(len(reqs)))

			if len(reqs) >= 3*kvWriteChCapacity {
				pendingCh <- struct{}{} // blocking.
				goto writeCase
			}

			select {
			// Either push to pending, or continue to pick from writeCh.
			case r = <-db.writeCh:
			case pendingCh <- struct{}{}:
				goto writeCase
			case <-lc.HasBeenClosed():
				goto closedCase
			}
		}

	closedCase:
		close(db.writeCh)
		for r := range db.writeCh { // Flush the channel.
			reqs = append(reqs, r)
		}

		pendingCh <- struct{}{} // Push to pending before doing a write.
		writeRequests(reqs)
		return

	writeCase:
		go writeRequests(reqs)
		reqs = make([]*request, 0, 10)
		reqLen.Set(0)
	}
}

// batchSet applies a list of badger.Entry. If a request level error occurs it
// will be returned.
//   Check(kv.BatchSet(entries))
func (db *DB) batchSet(entries []*Entry) error {
	req, err := db.sendToWriteCh(entries)
	if err != nil {
		return err
	}

	return req.Wait()
}

// batchSetAsync is the asynchronous version of batchSet. It accepts a callback
// function which is called when all the sets are complete. If a request level
// error occurs, it will be passed back via the callback.
//   err := kv.BatchSetAsync(entries, func(err error)) {
//      Check(err)
//   }
func (db *DB) batchSetAsync(entries []*Entry, f func(error)) error {
	req, err := db.sendToWriteCh(entries)
	if err != nil {
		return err
	}
	go func() {
		err := req.Wait()
		// Write is complete. Let's call the callback function now.
		f(err)
	}()
	return nil
}

var errNoRoom = errors.New("No room for write")

// ensureRoomForWrite is always called serially.
func (db *DB) ensureRoomForWrite() error {
	var err error
	db.Lock()
	defer db.Unlock()

	// Here we determine if we need to force flush memtable. Given we rotated log file, it would
	// make sense to force flush a memtable, so the updated value head would have a chance to be
	// pushed to L0. Otherwise, it would not go to L0, until the memtable has been fully filled,
	// which can take a lot longer if the write load has fewer keys and larger values. This force
	// flush, thus avoids the need to read through a lot of log files on a crash and restart.
	// Above approach is quite simple with small drawback. We are calling ensureRoomForWrite before
	// inserting every entry in Memtable. We will get latest db.head after all entries for a request
	// are inserted in Memtable. If we have done >= db.logRotates rotations, then while inserting
	// first entry in Memtable, below condition will be true and we will endup flushing old value of
	// db.head. Hence we are limiting no of value log files to be read to db.logRotates only.
	forceFlush := atomic.LoadInt32(&db.logRotates) >= db.opt.LogRotatesToFlush

	if !forceFlush && db.mt.MemSize() < db.opt.MaxTableSize {
		return nil
	}

	y.AssertTrue(db.mt != nil) // A nil mt indicates that DB is being closed.
	select {
	case db.flushChan <- flushTask{mt: db.mt, vptr: db.vhead}:
		// After every memtable flush, let's reset the counter.
		atomic.StoreInt32(&db.logRotates, 0)

		// Ensure value log is synced to disk so this memtable's contents wouldn't be lost.
		err = db.vlog.sync(db.vhead.Fid)
		if err != nil {
			return err
		}

		db.opt.Debugf("Flushing memtable, mt.size=%d size of flushChan: %d\n",
			db.mt.MemSize(), len(db.flushChan))
		// We manage to push this task. Let's modify imm.
		db.imm = append(db.imm, db.mt)
		db.mt = skl.NewSkiplist(arenaSize(db.opt))
		// New memtable is empty. We certainly have room.
		return nil
	default:
		// We need to do this to unlock and allow the flusher to modify imm.
		return errNoRoom
	}
}

func arenaSize(opt Options) int64 {
	return opt.MaxTableSize + opt.maxBatchSize + opt.maxBatchCount*int64(skl.MaxNodeSize)
}

// WriteLevel0Table flushes memtable.
func writeLevel0Table(ft flushTask, f io.Writer) error {
	iter := ft.mt.NewIterator()
	defer iter.Close()
	b := table.NewTableBuilder()
	defer b.Close()
	for iter.SeekToFirst(); iter.Valid(); iter.Next() {
		if len(ft.dropPrefix) > 0 && bytes.HasPrefix(iter.Key(), ft.dropPrefix) {
			continue
		}
		if err := b.Add(iter.Key(), iter.Value()); err != nil {
			return err
		}
	}
	_, err := f.Write(b.Finish())
	return err
}

type flushTask struct {
	mt         *skl.Skiplist
	vptr       valuePointer
	dropPrefix []byte
}

// handleFlushTask must be run serially.
func (db *DB) handleFlushTask(ft flushTask) error {
	// There can be a scnerio, when empty memtable is flushed. For example, memtable is empty and
	// after writing request to value log, rotation count exceeds db.LogRotatesToFlush.
	if ft.mt.Empty() {
		return nil
	}

	// Store badger head even if vptr is zero, need it for readTs
	db.opt.Debugf("Storing value log head: %+v\n", ft.vptr)
	db.elog.Printf("Storing offset: %+v\n", ft.vptr)
	offset := make([]byte, vptrSize)
	ft.vptr.Encode(offset)

	// Pick the max commit ts, so in case of crash, our read ts would be higher than all the
	// commits.
	headTs := y.KeyWithTs(head, db.orc.nextTs())
	ft.mt.Put(headTs, y.ValueStruct{Value: offset})

	fileID := db.lc.reserveFileID()
	fd, err := y.CreateSyncedFile(table.NewFilename(fileID, db.opt.Dir), true)
	if err != nil {
		return y.Wrap(err)
	}

	// Don't block just to sync the directory entry.
	dirSyncCh := make(chan error)
	go func() { dirSyncCh <- syncDir(db.opt.Dir) }()

	err = writeLevel0Table(ft, fd)
	dirSyncErr := <-dirSyncCh

	if err != nil {
		db.elog.Errorf("ERROR while writing to level 0: %v", err)
		return err
	}
	if dirSyncErr != nil {
		// Do dir sync as best effort. No need to return due to an error there.
		db.elog.Errorf("ERROR while syncing level directory: %v", dirSyncErr)
	}

	tbl, err := table.OpenTable(fd, db.opt.TableLoadingMode, nil)
	if err != nil {
		db.elog.Printf("ERROR while opening table: %v", err)
		return err
	}
	// We own a ref on tbl.
	err = db.lc.addLevel0Table(tbl) // This will incrRef (if we don't error, sure)
	_ = tbl.DecrRef()               // Releases our ref.
	return err
}

// flushMemtable must keep running until we send it an empty flushTask. If there
// are errors during handling the flush task, we'll retry indefinitely.
func (db *DB) flushMemtable(lc *y.Closer) error {
	defer lc.Done()

	for ft := range db.flushChan {
		if ft.mt == nil {
			// We close db.flushChan now, instead of sending a nil ft.mt.
			continue
		}
		for {
			err := db.handleFlushTask(ft)
			if err == nil {
				// Update s.imm. Need a lock.
				db.Lock()
				// This is a single-threaded operation. ft.mt corresponds to the head of
				// db.imm list. Once we flush it, we advance db.imm. The next ft.mt
				// which would arrive here would match db.imm[0], because we acquire a
				// lock over DB when pushing to flushChan.
				// TODO: This logic is dirty AF. Any change and this could easily break.
				y.AssertTrue(ft.mt == db.imm[0])
				db.imm = db.imm[1:]
				ft.mt.DecrRef() // Return memory.
				db.Unlock()

				break
			}
			// Encountered error. Retry indefinitely.
			db.opt.Errorf("Failure while flushing memtable to disk: %v. Retrying...\n", err)
			time.Sleep(time.Second)
		}
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

// This function does a filewalk, calculates the size of vlog and sst files and stores it in
// y.LSMSize and y.VlogSize.
func (db *DB) calculateSize() {
	newInt := func(val int64) *expvar.Int {
		v := new(expvar.Int)
		v.Add(val)
		return v
	}

	totalSize := func(dir string) (int64, int64) {
		var lsmSize, vlogSize int64
		err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			ext := filepath.Ext(path)
			if ext == ".sst" {
				lsmSize += info.Size()
			} else if ext == ".vlog" {
				vlogSize += info.Size()
			}
			return nil
		})
		if err != nil {
			db.elog.Printf("Got error while calculating total size of directory: %s", dir)
		}
		return lsmSize, vlogSize
	}

	lsmSize, vlogSize := totalSize(db.opt.Dir)
	y.LSMSize.Set(db.opt.Dir, newInt(lsmSize))
	// If valueDir is different from dir, we'd have to do another walk.
	if db.opt.ValueDir != db.opt.Dir {
		_, vlogSize = totalSize(db.opt.ValueDir)
	}
	y.VlogSize.Set(db.opt.Dir, newInt(vlogSize))
}

func (db *DB) updateSize(lc *y.Closer) {
	defer lc.Done()

	metricsTicker := time.NewTicker(time.Minute)
	defer metricsTicker.Stop()

	for {
		select {
		case <-metricsTicker.C:
			db.calculateSize()
		case <-lc.HasBeenClosed():
			return
		}
	}
}

// RunValueLogGC triggers a value log garbage collection.
//
// It picks value log files to perform GC based on statistics that are collected
// duing compactions.  If no such statistics are available, then log files are
// picked in random order. The process stops as soon as the first log file is
// encountered which does not result in garbage collection.
//
// When a log file is picked, it is first sampled. If the sample shows that we
// can discard at least discardRatio space of that file, it would be rewritten.
//
// If a call to RunValueLogGC results in no rewrites, then an ErrNoRewrite is
// thrown indicating that the call resulted in no file rewrites.
//
// We recommend setting discardRatio to 0.5, thus indicating that a file be
// rewritten if half the space can be discarded.  This results in a lifetime
// value log write amplification of 2 (1 from original write + 0.5 rewrite +
// 0.25 + 0.125 + ... = 2). Setting it to higher value would result in fewer
// space reclaims, while setting it to a lower value would result in more space
// reclaims at the cost of increased activity on the LSM tree. discardRatio
// must be in the range (0.0, 1.0), both endpoints excluded, otherwise an
// ErrInvalidRequest is returned.
//
// Only one GC is allowed at a time. If another value log GC is running, or DB
// has been closed, this would return an ErrRejected.
//
// Note: Every time GC is run, it would produce a spike of activity on the LSM
// tree.
func (db *DB) RunValueLogGC(discardRatio float64) error {
	if discardRatio >= 1.0 || discardRatio <= 0.0 {
		return ErrInvalidRequest
	}

	// Find head on disk
	headKey := y.KeyWithTs(head, math.MaxUint64)
	// Need to pass with timestamp, lsm get removes the last 8 bytes and compares key
	val, err := db.lc.get(headKey, nil)
	if err != nil {
		return errors.Wrap(err, "Retrieving head from on-disk LSM")
	}

	var head valuePointer
	if len(val.Value) > 0 {
		head.Decode(val.Value)
	}

	// Pick a log file and run GC
	return db.vlog.runGC(discardRatio, head)
}

// Size returns the size of lsm and value log files in bytes. It can be used to decide how often to
// call RunValueLogGC.
func (db *DB) Size() (lsm, vlog int64) {
	if y.LSMSize.Get(db.opt.Dir) == nil {
		lsm, vlog = 0, 0
		return
	}
	lsm = y.LSMSize.Get(db.opt.Dir).(*expvar.Int).Value()
	vlog = y.VlogSize.Get(db.opt.Dir).(*expvar.Int).Value()
	return
}

// Sequence represents a Badger sequence.
type Sequence struct {
	sync.Mutex
	db        *DB
	key       []byte
	next      uint64
	leased    uint64
	bandwidth uint64
}

// Next would return the next integer in the sequence, updating the lease by running a transaction
// if needed.
func (seq *Sequence) Next() (uint64, error) {
	seq.Lock()
	defer seq.Unlock()
	if seq.next >= seq.leased {
		if err := seq.updateLease(); err != nil {
			return 0, err
		}
	}
	val := seq.next
	seq.next++
	return val, nil
}

// Release the leased sequence to avoid wasted integers. This should be done right
// before closing the associated DB. However it is valid to use the sequence after
// it was released, causing a new lease with full bandwidth.
func (seq *Sequence) Release() error {
	seq.Lock()
	defer seq.Unlock()
	err := seq.db.Update(func(txn *Txn) error {
		var buf [8]byte
		binary.BigEndian.PutUint64(buf[:], seq.next)
		return txn.SetEntry(NewEntry(seq.key, buf[:]))
	})
	if err != nil {
		return err
	}
	seq.leased = seq.next
	return nil
}

func (seq *Sequence) updateLease() error {
	return seq.db.Update(func(txn *Txn) error {
		item, err := txn.Get(seq.key)
		if err == ErrKeyNotFound {
			seq.next = 0
		} else if err != nil {
			return err
		} else {
			var num uint64
			if err := item.Value(func(v []byte) error {
				num = binary.BigEndian.Uint64(v)
				return nil
			}); err != nil {
				return err
			}
			seq.next = num
		}

		lease := seq.next + seq.bandwidth
		var buf [8]byte
		binary.BigEndian.PutUint64(buf[:], lease)
		if err = txn.SetEntry(NewEntry(seq.key, buf[:])); err != nil {
			return err
		}
		seq.leased = lease
		return nil
	})
}

// GetSequence would initiate a new sequence object, generating it from the stored lease, if
// available, in the database. Sequence can be used to get a list of monotonically increasing
// integers. Multiple sequences can be created by providing different keys. Bandwidth sets the
// size of the lease, determining how many Next() requests can be served from memory.
//
// GetSequence is not supported on ManagedDB. Calling this would result in a panic.
func (db *DB) GetSequence(key []byte, bandwidth uint64) (*Sequence, error) {
	if db.opt.managedTxns {
		panic("Cannot use GetSequence with managedDB=true.")
	}

	switch {
	case len(key) == 0:
		return nil, ErrEmptyKey
	case bandwidth == 0:
		return nil, ErrZeroBandwidth
	}
	seq := &Sequence{
		db:        db,
		key:       key,
		next:      0,
		leased:    0,
		bandwidth: bandwidth,
	}
	err := seq.updateLease()
	return seq, err
}

// Tables gets the TableInfo objects from the level controller. If withKeysCount
// is true, TableInfo objects also contain counts of keys for the tables.
func (db *DB) Tables(withKeysCount bool) []TableInfo {
	return db.lc.getTableInfo(withKeysCount)
}

// KeySplits can be used to get rough key ranges to divide up iteration over
// the DB.
func (db *DB) KeySplits(prefix []byte) []string {
	var splits []string
	// We just want table ranges here and not keys count.
	for _, ti := range db.Tables(false) {
		// We don't use ti.Left, because that has a tendency to store !badger
		// keys.
		if bytes.HasPrefix(ti.Right, prefix) {
			splits = append(splits, string(ti.Right))
		}
	}
	sort.Strings(splits)
	return splits
}

// MaxBatchCount returns max possible entries in batch
func (db *DB) MaxBatchCount() int64 {
	return db.opt.maxBatchCount
}

// MaxBatchSize returns max possible batch size
func (db *DB) MaxBatchSize() int64 {
	return db.opt.maxBatchSize
}

func (db *DB) stopCompactions() {
	// Stop memtable flushes.
	if db.closers.memtable != nil {
		close(db.flushChan)
		db.closers.memtable.SignalAndWait()
	}
	// Stop compactions.
	if db.closers.compactors != nil {
		db.closers.compactors.SignalAndWait()
	}
}

func (db *DB) startCompactions() {
	// Resume compactions.
	if db.closers.compactors != nil {
		db.closers.compactors = y.NewCloser(1)
		db.lc.startCompact(db.closers.compactors)
	}
	if db.closers.memtable != nil {
		db.flushChan = make(chan flushTask, db.opt.NumMemtables)
		db.closers.memtable = y.NewCloser(1)
		go func() {
			_ = db.flushMemtable(db.closers.memtable)
		}()
	}
}

// Flatten can be used to force compactions on the LSM tree so all the tables fall on the same
// level. This ensures that all the versions of keys are colocated and not split across multiple
// levels, which is necessary after a restore from backup. During Flatten, live compactions are
// stopped. Ideally, no writes are going on during Flatten. Otherwise, it would create competition
// between flattening the tree and new tables being created at level zero.
func (db *DB) Flatten(workers int) error {
	db.stopCompactions()
	defer db.startCompactions()

	compactAway := func(cp compactionPriority) error {
		db.opt.Infof("Attempting to compact with %+v\n", cp)
		errCh := make(chan error, 1)
		for i := 0; i < workers; i++ {
			go func() {
				errCh <- db.lc.doCompact(cp)
			}()
		}
		var success int
		var rerr error
		for i := 0; i < workers; i++ {
			err := <-errCh
			if err != nil {
				rerr = err
				db.opt.Warningf("While running doCompact with %+v. Error: %v\n", cp, err)
			} else {
				success++
			}
		}
		if success == 0 {
			return rerr
		}
		// We could do at least one successful compaction. So, we'll consider this a success.
		db.opt.Infof("%d compactor(s) succeeded. One or more tables from level %d compacted.\n",
			success, cp.level)
		return nil
	}

	hbytes := func(sz int64) string {
		return humanize.Bytes(uint64(sz))
	}

	for {
		db.opt.Infof("\n")
		var levels []int
		for i, l := range db.lc.levels {
			sz := l.getTotalSize()
			db.opt.Infof("Level: %d. %8s Size. %8s Max.\n",
				i, hbytes(l.getTotalSize()), hbytes(l.maxTotalSize))
			if sz > 0 {
				levels = append(levels, i)
			}
		}
		if len(levels) <= 1 {
			prios := db.lc.pickCompactLevels()
			if len(prios) == 0 || prios[0].score <= 1.0 {
				db.opt.Infof("All tables consolidated into one level. Flattening done.\n")
				return nil
			}
			if err := compactAway(prios[0]); err != nil {
				return err
			}
			continue
		}
		// Create an artificial compaction priority, to ensure that we compact the level.
		cp := compactionPriority{level: levels[0], score: 1.71}
		if err := compactAway(cp); err != nil {
			return err
		}
	}
}

func (db *DB) prepareToDrop() func() {
	if db.opt.ReadOnly {
		panic("Attempting to drop data in read-only mode.")
	}
	// Stop accepting new writes.
	atomic.StoreInt32(&db.blockWrites, 1)

	// Make all pending writes finish. The following will also close writeCh.
	db.closers.writes.SignalAndWait()
	db.opt.Infof("Writes flushed. Stopping compactions now...")

	// Stop all compactions.
	db.stopCompactions()
	return func() {
		db.opt.Infof("Resuming writes")
		db.startCompactions()

		db.writeCh = make(chan *request, kvWriteChCapacity)
		db.closers.writes = y.NewCloser(1)
		go db.doWrites(db.closers.writes)

		// Resume writes.
		atomic.StoreInt32(&db.blockWrites, 0)
	}
}

// DropAll would drop all the data stored in Badger. It does this in the following way.
// - Stop accepting new writes.
// - Pause memtable flushes and compactions.
// - Pick all tables from all levels, create a changeset to delete all these
// tables and apply it to manifest.
// - Pick all log files from value log, and delete all of them. Restart value log files from zero.
// - Resume memtable flushes and compactions.
//
// NOTE: DropAll is resilient to concurrent writes, but not to reads. It is up to the user to not do
// any reads while DropAll is going on, otherwise they may result in panics. Ideally, both reads and
// writes are paused before running DropAll, and resumed after it is finished.
func (db *DB) DropAll() error {
	f, err := db.dropAll()
	if err != nil {
		return err
	}
	if f == nil {
		panic("both error and returned function cannot be nil in DropAll")
	}
	f()
	return nil
}

func (db *DB) dropAll() (func(), error) {
	db.opt.Infof("DropAll called. Blocking writes...")
	f := db.prepareToDrop()

	// Block all foreign interactions with memory tables.
	db.Lock()
	defer db.Unlock()

	// Remove inmemory tables. Calling DecrRef for safety. Not sure if they're absolutely needed.
	db.mt.DecrRef()
	for _, mt := range db.imm {
		mt.DecrRef()
	}
	db.imm = db.imm[:0]
	db.mt = skl.NewSkiplist(arenaSize(db.opt)) // Set it up for future writes.

	num, err := db.lc.dropTree()
	if err != nil {
		return nil, err
	}
	db.opt.Infof("Deleted %d SSTables. Now deleting value logs...\n", num)

	num, err = db.vlog.dropAll()
	if err != nil {
		return nil, err
	}
	db.vhead = valuePointer{} // Zero it out.
	db.lc.nextFileID = 1
	db.opt.Infof("Deleted %d value log files. DropAll done.\n", num)
	return f, nil
}

// DropPrefix would drop all the keys with the provided prefix. It does this in the following way:
// - Stop accepting new writes.
// - Stop memtable flushes and compactions.
// - Flush out all memtables, skipping over keys with the given prefix, Kp.
// - Write out the value log header to memtables when flushing, so we don't accidentally bring Kp
//   back after a restart.
// - Compact L0->L1, skipping over Kp.
// - Compact rest of the levels, Li->Li, picking tables which have Kp.
// - Resume memtable flushes, compactions and writes.
func (db *DB) DropPrefix(prefix []byte) error {
	db.opt.Infof("DropPrefix called on %s. Blocking writes...", hex.Dump(prefix))
	f := db.prepareToDrop()
	defer f()

	// Block all foreign interactions with memory tables.
	db.Lock()
	defer db.Unlock()

	db.imm = append(db.imm, db.mt)
	for _, memtable := range db.imm {
		if memtable.Empty() {
			memtable.DecrRef()
			continue
		}
		task := flushTask{
			mt: memtable,
			// Ensure that the head of value log gets persisted to disk.
			vptr:       db.vhead,
			dropPrefix: prefix,
		}
		db.opt.Debugf("Flushing memtable")
		if err := db.handleFlushTask(task); err != nil {
			db.opt.Errorf("While trying to flush memtable: %v", err)
			return err
		}
		memtable.DecrRef()
	}
	db.imm = db.imm[:0]
	db.mt = skl.NewSkiplist(arenaSize(db.opt))

	// Drop prefixes from the levels.
	if err := db.lc.dropPrefix(prefix); err != nil {
		return err
	}
	db.opt.Infof("DropPrefix done")
	return nil
}

// Subscribe can be used watch key changes for the given key prefix.
func (db *DB) Subscribe(ctx context.Context, cb callback, prefix []byte, prefixes ...[]byte) error {
	if cb == nil {
		return ErrNilCallback
	}
	prefixes = append(prefixes, prefix)
	c := y.NewCloser(1)
	recvCh, id := db.pub.newSubscriber(c, prefixes...)
	slurp := func(batch *pb.KVList) {
		defer func() {
			if len(batch.GetKv()) > 0 {
				cb(batch)
			}
		}()
		for {
			select {
			case kvs := <-recvCh:
				batch.Kv = append(batch.Kv, kvs.Kv...)
			default:
				return
			}
		}
	}
	for {
		select {
		case <-c.HasBeenClosed():
			slurp(new(pb.KVList))
			// Drain if any pending updates.
			c.Done()
			// No need to delete here. Closer will be called only while
			// closing DB. Subscriber will be deleted by cleanSubscribers.
			return nil
		case <-ctx.Done():
			c.Done()
			db.pub.deleteSubscriber(id)
			// Delete the subscriber to avoid further updates.
			return ctx.Err()
		case batch := <-recvCh:
			slurp(batch)
		}
	}
}
