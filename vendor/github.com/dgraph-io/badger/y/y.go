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

package y

import (
	"hash/crc32"
	"log"
	"os"
	"sync"
	"sync/atomic"
)

// Constants used in serialization sizes, and in ValueStruct serialization
const (
	MetaSize     = 1
	UserMetaSize = 1
	CasSize      = 8
)

var (
	// This is O_DSYNC (datasync) on platforms that support it -- see file_unix.go
	datasyncFileFlag = 0x0

	// CastagnoliCrcTable is a CRC32 polynomial table
	CastagnoliCrcTable = crc32.MakeTable(crc32.Castagnoli)
)

// OpenExistingSyncedFile opens an existing file, errors if it doesn't exist.
func OpenExistingSyncedFile(filename string, sync bool) (*os.File, error) {
	flags := os.O_RDWR
	if sync {
		flags |= datasyncFileFlag
	}
	return os.OpenFile(filename, flags, 0)
}

// CreateSyncedFile creates a new file (using O_EXCL), errors if it already existed.
func CreateSyncedFile(filename string, sync bool) (*os.File, error) {
	flags := os.O_RDWR | os.O_CREATE | os.O_EXCL
	if sync {
		flags |= datasyncFileFlag
	}
	return os.OpenFile(filename, flags, 0666)
}

// OpenSyncedFile creates the file if one doesn't exist.
func OpenSyncedFile(filename string, sync bool) (*os.File, error) {
	flags := os.O_RDWR | os.O_CREATE
	if sync {
		flags |= datasyncFileFlag
	}
	return os.OpenFile(filename, flags, 0666)
}

func OpenTruncFile(filename string, sync bool) (*os.File, error) {
	flags := os.O_RDWR | os.O_CREATE | os.O_TRUNC
	if sync {
		flags |= datasyncFileFlag
	}
	return os.OpenFile(filename, flags, 0666)
}

func Safecopy(a []byte, src []byte) []byte {
	if cap(a) < len(src) {
		a = make([]byte, len(src))
	}
	a = a[:len(src)]
	copy(a, src)
	return a
}

type Slice struct {
	buf []byte
}

func (s *Slice) Resize(sz int) []byte {
	if cap(s.buf) < sz {
		s.buf = make([]byte, sz)
	}
	return s.buf[0:sz]
}

type LevelCloser struct {
	Name    string
	running int32
	nomore  int32
	closed  chan struct{}
	waiting sync.WaitGroup
}

type Closer struct {
	sync.RWMutex
	levels map[string]*LevelCloser
}

func NewCloser() *Closer {
	return &Closer{
		levels: make(map[string]*LevelCloser),
	}
}

func (c *Closer) Register(name string) *LevelCloser {
	c.Lock()
	defer c.Unlock()

	lc, has := c.levels[name]
	if !has {
		lc = &LevelCloser{Name: name, closed: make(chan struct{}, 10)}
		lc.waiting.Add(1)
		c.levels[name] = lc
	}

	AssertTruef(atomic.LoadInt32(&lc.nomore) == 0, "Can't register with closer after signal.")
	atomic.AddInt32(&lc.running, 1)
	return lc
}

func (c *Closer) Get(name string) *LevelCloser {
	c.RLock()
	defer c.RUnlock()

	lc, has := c.levels[name]
	if !has {
		log.Fatalf("%q not present in Closer", name)
		return nil
	}
	return lc
}

func (c *Closer) SignalAll() {
	c.RLock()
	defer c.RUnlock()

	for _, l := range c.levels {
		l.Signal()
	}
}

func (c *Closer) WaitForAll() {
	c.RLock()
	defer c.RUnlock()

	for _, l := range c.levels {
		l.Wait()
	}
}

func (lc *LevelCloser) AddRunning(delta int32) {
	atomic.AddInt32(&lc.running, delta)
}

func (lc *LevelCloser) Signal() {
	if !atomic.CompareAndSwapInt32(&lc.nomore, 0, 1) {
		return
	}
	close(lc.closed)
}

func (lc *LevelCloser) HasBeenClosed() <-chan struct{} {
	return lc.closed
}

func (lc *LevelCloser) GotSignal() bool {
	return atomic.LoadInt32(&lc.nomore) == 1
}

func (lc *LevelCloser) Done() {
	if atomic.LoadInt32(&lc.running) <= 0 {
		return
	}

	running := atomic.AddInt32(&lc.running, -1)
	if running == 0 {
		lc.waiting.Done()
	}
}

func (lc *LevelCloser) Wait() {
	lc.waiting.Wait()
}

func (lc *LevelCloser) SignalAndWait() {
	lc.Signal()
	lc.Wait()
}
