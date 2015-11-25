/*
 * Copyright 2015 Manish R Jain <manishrjain@gmail.com>
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

package commit

import (
	"encoding/binary"
	"errors"
	"io"
	"io/ioutil"
	"sync"
	"sync/atomic"
	"time"

	"github.com/willf/bloom"
)

var E_READ = errors.New("Unable to read")
var E_WRITE = errors.New("Unable to write")

type Cache struct {
	sync.RWMutex
	buf        []byte
	lastAccess int64
	bf         *bloom.BloomFilter
}

func toBytes(hash uint32) []byte {
	n := make([]byte, 8)
	nlen := binary.PutUvarint(n, uint64(hash))
	return n[:nlen]
}

func (c *Cache) Present(hash uint32) bool {
	c.RLock()
	defer c.RUnlock()

	if c.bf == nil {
		return true
	}
	return c.bf.Test(toBytes(hash))
}

func (c *Cache) Write(hash uint32, p []byte) (n int, err error) {
	atomic.StoreInt64(&c.lastAccess, time.Now().UnixNano())

	c.Lock()
	defer c.Unlock()
	c.bf.Add(toBytes(hash))
	c.buf = append(c.buf, p...)
	return len(p), nil
}

func (c *Cache) ReadAt(pos int, p []byte) (n int, err error) {
	atomic.StoreInt64(&c.lastAccess, time.Now().UnixNano())

	c.RLock()
	defer c.RUnlock()

	if len(c.buf[pos:]) == 0 {
		return 0, io.EOF
	}

	n = copy(p, c.buf[pos:])
	if n < len(p) {
		return n, E_READ
	}
	return n, nil
}

func (c *Cache) LastAccessedInSeconds() int64 {
	d := atomic.LoadInt64(&c.lastAccess)
	return (time.Now().UnixNano() - d) / 1000000000
}

// Reader isn't thread-safe. But multiple readers can be used to read the
// same cache.
type Reader struct {
	c   *Cache
	pos int
}

func NewReader(c *Cache) *Reader {
	r := new(Reader)
	r.c = c
	return r
}

func (r *Reader) Read(p []byte) (n int, err error) {
	n, err = r.c.ReadAt(r.pos, p)
	r.pos += n
	return
}

func (r *Reader) Discard(n int) {
	r.pos += n
}

func FillCache(c *Cache, path string) error {
	buf, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}
	c.Lock()
	c.buf = buf
	c.Unlock()
	return nil
}
