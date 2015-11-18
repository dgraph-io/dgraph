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

package concurrent

import (
	"log"
	"sync/atomic"
	"unsafe"
)

type kv struct {
	k uint64
	v unsafe.Pointer
}

type bucket struct {
	elems [8]kv
}

type container struct {
	active int32
	sz     uint64
	list   []*bucket
}

type Map struct {
	cs [2]container
}

func powOf2(sz int) bool {
	return sz > 0 && (sz&(sz-1)) == 0
}

func initContainer(cs *container, sz uint64) {
	cs.active = 1
	cs.sz = sz
	cs.list = make([]*bucket, sz)
	for i := range cs.list {
		cs.list[i] = new(bucket)
	}
}

func NewMap(sz int) *Map {
	if !powOf2(sz) {
		log.Fatal("Map can only be created for a power of 2.")
	}

	m := new(Map)
	initContainer(&m.cs[0], uint64(sz))
	return m
}

func (m *Map) Get(k uint64) unsafe.Pointer {
	for _, c := range m.cs {
		if atomic.LoadInt32(&c.active) == 0 {
			continue
		}
		bi := k & (c.sz - 1)
		b := c.list[bi]
		for i := range b.elems {
			e := &b.elems[i]
			ek := atomic.LoadUint64(&e.k)
			if ek == k {
				return e.v
			}
		}
	}
	return nil
}

func (m *Map) Put(k uint64, v unsafe.Pointer) bool {
	for _, c := range m.cs {
		if atomic.LoadInt32(&c.active) == 0 {
			continue
		}
		bi := k & (c.sz - 1)
		b := c.list[bi]
		for i := range b.elems {
			e := &b.elems[i]
			// Once allocated a valid key, it would never change. So, first check if
			// it's allocated. If not, then allocate it. If can't, or not allocated,
			// then check if it's k. If it is, then replace value. Otherwise continue.
			if atomic.CompareAndSwapUint64(&e.k, 0, k) {
				atomic.StorePointer(&e.v, v)
				return true
			}
			if atomic.LoadUint64(&e.k) == k {
				atomic.StorePointer(&e.v, v)
				return true
			}
		}
	}
	return false
}

/*
func (m *Map) StreamUntilCap(ch chan uint64) {
	for _, c := range m.cs {
		if atomic.LoadInt32(&c.active) == 0 {
			continue
		}
		for {
			bi := rand.Intn(int(c.sz))
			for len(ch) < cap(ch) {
			}
		}
	}
}
*/

func (m *Map) StreamAll(ch chan uint64) {
	for _, c := range m.cs {
		if atomic.LoadInt32(&c.active) == 0 {
			continue
		}
		for i := 0; i < int(c.sz); i++ {
			for _, e := range c.list[i].elems {
				if k := atomic.LoadUint64(&e.k); k > 0 {
					ch <- k
				}
			}
		}
	}
}
