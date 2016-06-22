/*
 * Copyright 2015 DGraph Labs, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * 		http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

// commit package provides commit logs for storing mutations, as they arrive
// at the server. Mutations also get stored in memory within posting.List.
// So, commit logs are useful to handle machine crashes, and re-init of a
// posting list.
// This package provides functionality to write to a rotating log, and a way
// to quickly filter relevant entries corresponding to an attribute.
package commit

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"golang.org/x/net/trace"

	"github.com/willf/bloom"
)

type logFile struct {
	sync.RWMutex
	endTs int64 // never modified after creation.
	path  string
	size  int64
	cache *Cache
	bf    *bloom.BloomFilter
}

func (lf *logFile) Cache() *Cache {
	lf.RLock()
	defer lf.RUnlock()
	return lf.cache
}

func (lf *logFile) FillIfEmpty(wg *sync.WaitGroup) {
	lf.Lock()
	defer lf.Unlock()
	defer wg.Done()
	if lf.cache != nil {
		return
	}
	cache := new(Cache)
	if err := FillCache(cache, lf.path); err != nil {
		log.Fatalf("Unable to fill cache for path: %v. Err: %v", lf.path, err)
	}
	// No need to acquire lock on cache, because it just
	// got created.
	createAndUpdateBloomFilter(cache)
	lf.cache = cache
}

// Lock must have been acquired.
func createAndUpdateBloomFilter(cache *Cache) {
	hashes := make([]uint32, 50000)
	hashes = hashes[:0]
	if err := streamEntries(cache, 0, 0, func(hdr Header, record []byte) {
		hashes = append(hashes, hdr.hash)
	}); err != nil {
		log.Fatalf("Unable to create bloom filters: %v", err)
	}

	n := 100000
	if len(hashes) > n {
		n = len(hashes)
	}
	cache.bf = bloom.NewWithEstimates(uint(n), 0.0001)
	for _, hash := range hashes {
		cache.bf.Add(toBytes(hash))
	}
}

type CurFile struct {
	sync.RWMutex
	f         *os.File
	size      int64
	dirtyLogs int
	cch       unsafe.Pointer // handled via atomics.
}

func (c *CurFile) cache() *Cache {
	if c == nil {
		debug.PrintStack()
		// This got triggered due to a premature cleanup in query_test.go
	}

	v := atomic.LoadPointer(&c.cch)
	if v == nil {
		return nil
	}
	return (*Cache)(v)
}

func (c *CurFile) Size() int64 {
	c.RLock()
	defer c.RUnlock()
	return c.size
}

type ByTimestamp []*logFile

func (b ByTimestamp) Len() int      { return len(b) }
func (b ByTimestamp) Swap(i, j int) { b[i], b[j] = b[j], b[i] }
func (b ByTimestamp) Less(i, j int) bool {
	return b[i].endTs < b[j].endTs
}

type Logger struct {
	// Directory to store logs into.
	dir string

	// Prefix all filenames with this.
	filePrefix string

	// MaxSize is the maximum size of commit log file in bytes,
	// before it gets rotated.
	maxSize int64

	// Sync every N logs. A value of zero or less would mean
	// sync every append to file.
	SyncEvery int

	// Sync every d duration.
	SyncDur time.Duration

	sync.RWMutex
	list      []*logFile
	cf        *CurFile
	lastLogTs int64 // handled via atomics.
	ticker    *time.Ticker

	events trace.EventLog
}

func (l *Logger) curFile() *CurFile {
	l.RLock()
	defer l.RUnlock()
	return l.cf
}

func (l *Logger) updateLastLogTs(val int64) {
	for {
		prev := atomic.LoadInt64(&l.lastLogTs)
		if val <= prev {
			return
		}
		if atomic.CompareAndSwapInt64(&l.lastLogTs, prev, val) {
			return
		}
	}
}

func (l *Logger) DeleteCacheOlderThan(v time.Duration) {
	l.RLock()
	defer l.RUnlock()
	s := int64(v.Seconds())
	for _, lf := range l.list {
		if lf.Cache().LastAccessedInSeconds() > s {
			lf.Lock()
			lf.cache = nil
			lf.Unlock()
		}
	}
}

func (l *Logger) periodicSync() {
	if l.SyncDur == 0 {
		l.events.Printf("No Periodic Sync for commit log.")
		return
	}
	l.events.Printf("Periodic Sync at duration: %v", l.SyncDur)

	l.ticker = time.NewTicker(l.SyncDur)
	for _ = range l.ticker.C {
		cf := l.curFile()
		if cf == nil {
			continue
		}

		{
			cf.Lock()
			if cf.dirtyLogs > 0 {
				if err := cf.f.Sync(); err != nil {
					l.events.Errorf("While periodically syncing: %v", err)
				} else {
					cf.dirtyLogs = 0
					l.events.Printf("Successful periodic sync.")
				}
			} else {
				l.events.Printf("Skipping periodic sync.")
			}
			cf.Unlock()
		}
	}
}

func (l *Logger) Close() {
	l.Lock()
	defer l.Unlock()

	if l.ticker != nil {
		l.ticker.Stop()
	}
	if l.cf != nil {
		if err := l.cf.f.Close(); err != nil {
			l.events.Errorf("Error while closing current file: %v", err)
		}
		l.cf = nil
	}
	l.events.Finish()
}

func NewLogger(dir string, fileprefix string, maxSize int64) *Logger {
	l := new(Logger)
	l.dir = dir
	l.filePrefix = fileprefix
	l.maxSize = maxSize
	l.events = trace.NewEventLog("commit", "Logger")
	return l
}

// A mutex lock should have already been acquired to call this function.
func (l *Logger) handleFile(path string, info os.FileInfo, err error) error {
	if info.IsDir() {
		return nil
	}
	if !strings.HasPrefix(info.Name(), l.filePrefix+"-") {
		return nil
	}
	if !strings.HasSuffix(info.Name(), ".log") {
		return nil
	}
	lidx := strings.LastIndex(info.Name(), ".log")
	tstring := info.Name()[len(l.filePrefix)+1 : lidx]
	l.events.Printf("Found log with ts: %v", tstring)

	// Handle if we find the current log file.
	if tstring == "current" {
		return nil
	}

	ts, err := strconv.ParseInt(tstring, 16, 64)
	if err != nil {
		return err
	}
	lf := new(logFile)
	lf.endTs = ts
	lf.path = path
	l.list = append(l.list, lf)

	l.updateLastLogTs(lf.endTs)
	return nil
}

func (l *Logger) Init() {
	l.Lock()
	defer l.Unlock()

	l.events.Printf("Logger init started")
	{
		// Checking if the directory exists.
		if _, err := os.Stat(l.dir); err != nil {
			if os.IsNotExist(err) {
				log.Fatalf("Unable to find dir: %v", err)
			}
		}
		// First check if we have a current file.
		path := filepath.Join(l.dir, fmt.Sprintf("%s-current.log", l.filePrefix))
		fi, err := os.Stat(path)
		if err == nil {
			// we have the file. Derive information for counters.
			l.cf = new(CurFile)
			l.cf.size = fi.Size()
			l.cf.dirtyLogs = 0
			cache := new(Cache)
			if ferr := FillCache(cache, path); ferr != nil {
				log.Fatalf("Unable to write to cache: %v", ferr)
			}
			createAndUpdateBloomFilter(cache)
			atomic.StorePointer(&l.cf.cch, unsafe.Pointer(cache))
			lastTs, err := lastTimestamp(cache)
			if err != nil {
				log.Fatalf("Unable to read last log ts: %v", err)
			}
			l.updateLastLogTs(lastTs)

			// Open file for append.
			l.cf.f, err = os.OpenFile(path, os.O_APPEND|os.O_WRONLY,
				os.FileMode(0644))
			if err != nil {
				log.Fatalf("Unable to open current file in append mode: %v", err)
			}
		}
	}

	if err := filepath.Walk(l.dir, l.handleFile); err != nil {
		log.Fatal("While walking over directory: %v", err)
	}
	sort.Sort(ByTimestamp(l.list))
	if l.cf == nil {
		l.createNew()
	}
	go l.periodicSync()
	l.events.Printf("Logger init finished.")
}

func (l *Logger) filepath(ts int64) string {
	return fmt.Sprintf("%s-%s.log", l.filePrefix, strconv.FormatInt(ts, 16))
}

type Header struct {
	ts   int64
	hash uint32
	size int32
}

func parseHeader(hdr []byte) (Header, error) {
	buf := bytes.NewBuffer(hdr)
	var h Header
	var err error
	setError(&err, binary.Read(buf, binary.LittleEndian, &h.ts))
	setError(&err, binary.Read(buf, binary.LittleEndian, &h.hash))
	setError(&err, binary.Read(buf, binary.LittleEndian, &h.size))
	if err != nil {
		return h, err
	}
	return h, nil
}

func lastTimestamp(c *Cache) (int64, error) {
	var maxTs int64
	reader := NewReader(c)
	header := make([]byte, 16)
	count := 0
	for {
		n, err := reader.Read(header)
		if err == io.EOF {
			break
		}
		if n < len(header) {
			log.Fatalf("Unable to read full 16 byte header. Read %v", n)
		}
		if err != nil {
			return 0, err
		}
		count += 1
		h, err := parseHeader(header)
		if err != nil {
			return 0, err
		}

		if h.ts > maxTs {
			maxTs = h.ts

		} else if h.ts < maxTs {
			log.Fatalf("Log file doesn't have monotonically increasing records."+
				" ts: %v. maxts: %v. numrecords: %v", h.ts, maxTs, count)
		}
		reader.Discard(int(h.size))
	}
	return maxTs, nil
}

func (l *Logger) rotateCurrent() error {
	l.Lock()
	defer l.Unlock()

	cf := l.cf
	cf.Lock()
	defer cf.Unlock()

	if len(l.list) > 0 {
		last := l.list[len(l.list)-1]
		if last.endTs > atomic.LoadInt64(&l.lastLogTs) {
			return fmt.Errorf("Maxtimestamp is lower than existing commit logs.")
		}
	}

	lastTs := atomic.LoadInt64(&l.lastLogTs)
	newpath := filepath.Join(l.dir, l.filepath(lastTs))
	if err := cf.f.Close(); err != nil {
		return err
	}
	if err := os.Rename(cf.f.Name(), newpath); err != nil {
		l.events.Errorf("Error while renaming: %v", err)
		return err
	}

	lf := new(logFile)
	lf.endTs = lastTs
	lf.path = newpath
	lf.size = cf.size
	lf.cache = cf.cache()
	createAndUpdateBloomFilter(lf.cache)
	l.list = append(l.list, lf)

	l.createNew()
	return nil
}

// Expects a lock has already been acquired.
func (l *Logger) createNew() {
	path := filepath.Join(l.dir, fmt.Sprintf("%s-current.log", l.filePrefix))
	if err := os.MkdirAll(l.dir, 0744); err != nil {
		log.Fatalf("Unable to create directory: %v", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC,
		os.FileMode(0644))
	if err != nil {
		log.Fatalf("Unable to create a new file: %v", err)
	}
	l.cf = new(CurFile)
	l.cf.f = f
	cache := new(Cache)
	createAndUpdateBloomFilter(cache)
	atomic.StorePointer(&l.cf.cch, unsafe.Pointer(cache))
}

func setError(prev *error, n error) {
	if prev == nil {
		prev = &n
	}
	return
}

func (l *Logger) AddLog(hash uint32, value []byte) (int64, error) {
	lbuf := int64(len(value)) + 16
	if l.curFile().Size()+lbuf > l.maxSize {
		if err := l.rotateCurrent(); err != nil {
			l.events.Errorf("Error while rotating current file out: %v", err)
			return 0, err
		}
	}

	cf := l.curFile()
	if cf == nil {
		log.Fatalf("Current file isn't initialized.")
	}

	cf.Lock()
	defer cf.Unlock()

	ts := time.Now().UnixNano()
	lts := atomic.LoadInt64(&l.lastLogTs)
	if ts < lts {
		ts = lts + 1
		// We don't have to do CompareAndSwap because we've a mutex lock.
	}

	buf := new(bytes.Buffer)
	var err error
	setError(&err, binary.Write(buf, binary.LittleEndian, ts))
	setError(&err, binary.Write(buf, binary.LittleEndian, hash))
	setError(&err, binary.Write(buf, binary.LittleEndian, int32(len(value))))
	_, nerr := buf.Write(value)
	setError(&err, nerr)
	if err != nil {
		return ts, err
	}

	if _, err = cf.f.Write(buf.Bytes()); err != nil {
		l.events.Errorf("Error while writing to current file: %v", err)
		return ts, err
	}
	if _, err = cf.cache().Write(hash, buf.Bytes()); err != nil {
		l.events.Errorf("Error while writing to current cache: %v", err)
		return ts, err
	}
	cf.dirtyLogs += 1
	cf.size += int64(buf.Len())
	l.updateLastLogTs(ts)
	if l.SyncEvery <= 0 || cf.dirtyLogs >= l.SyncEvery {
		cf.dirtyLogs = 0
		l.events.Printf("Syncing file")
		return ts, cf.f.Sync()
	}
	return ts, nil
}

// streamEntries allows for hash to be zero.
// This means iterate over all the entries.
func streamEntries(cache *Cache,
	afterTs int64, hash uint32, iter LogIterator) error {

	reader := NewReader(cache)
	header := make([]byte, 16)
	for {
		_, err := reader.Read(header)
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Fatalf("While reading header: %v", err)
			return err
		}

		hdr, err := parseHeader(header)
		if err != nil {
			return err
		}

		if (hash == 0 || hdr.hash == hash) && hdr.ts >= afterTs {
			// Iterator expects a copy of the buffer, so create one, instead of
			// creating a big buffer upfront and reusing it.
			data := make([]byte, hdr.size)
			_, err := reader.Read(data)
			if err != nil {
				log.Fatalf("While reading data: %v", err)
				return err
			}
			iter(hdr, data)

		} else {
			reader.Discard(int(hdr.size))
		}
	}
	return nil
}

type LogIterator func(hdr Header, record []byte)

func (l *Logger) StreamEntries(afterTs int64, hash uint32,
	iter LogIterator) error {

	if atomic.LoadInt64(&l.lastLogTs) < afterTs {
		return nil
	}

	var wg sync.WaitGroup
	l.RLock()
	for _, lf := range l.list {
		if afterTs < lf.endTs {
			wg.Add(1)
			go lf.FillIfEmpty(&wg)
		}
	}
	l.RUnlock()
	wg.Wait()

	l.RLock()
	var caches []*Cache
	for _, lf := range l.list {
		if afterTs < lf.endTs && lf.cache.Present(hash) {
			caches = append(caches, lf.Cache())
		}
	}
	l.RUnlock()

	// Add current cache.
	if l.curFile().cache().Present(hash) {
		caches = append(caches, l.curFile().cache())
	}
	for _, cache := range caches {
		if cache == nil {
			l.events.Errorf("Cache is nil")
			continue
		}
		if err := streamEntries(cache, afterTs, hash, iter); err != nil {
			return err
		}
	}
	return nil
}
