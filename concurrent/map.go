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
	"math/rand"
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

const (
	MUTABLE = iota
	IMMUTABLE
)

type container struct {
	status   int32
	sz       uint64
	list     []*bucket
	numElems uint32
}

type Map struct {
	cs   [2]unsafe.Pointer
	size uint32
}

func powOf2(sz int) bool {
	return sz > 0 && (sz&(sz-1)) == 0
}

func initContainer(cs *container, sz uint64) {
	cs.status = MUTABLE
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

	c := new(container)
	initContainer(c, uint64(sz))

	m := new(Map)
	m.cs[MUTABLE] = unsafe.Pointer(c)
	m.cs[IMMUTABLE] = nil
	return m
}

func (c *container) get(k uint64) unsafe.Pointer {
	bi := k & (c.sz - 1)
	b := c.list[bi]
	for i := range b.elems {
		e := &b.elems[i]
		if ek := atomic.LoadUint64(&e.k); ek == k {
			return e.v
		}
	}
	return nil
}

func (c *container) getOrInsert(k uint64, v unsafe.Pointer) unsafe.Pointer {
	bi := k & (c.sz - 1)
	b := c.list[bi]
	for i := range b.elems {
		e := &b.elems[i]
		// Once allocated a valid key, it would never change. So, first check if
		// it's allocated. If not, then allocate it. If can't, or not allocated,
		// then check if it's k. If it is, then replace value. Otherwise continue.
		// This sequence could be problematic, if this happens:
		// Main thread runs Step 1. Check
		if atomic.CompareAndSwapUint64(&e.k, 0, k) { // Step 1.
			atomic.AddUint32(&c.numElems, 1)
			if atomic.CompareAndSwapPointer(&e.v, nil, v) {
				return v
			}
			return atomic.LoadPointer(&e.v)
		}

		if atomic.LoadUint64(&e.k) == k {
			// Swap if previous pointer is nil.
			if atomic.CompareAndSwapPointer(&e.v, nil, v) {
				return v
			}
			return atomic.LoadPointer(&e.v)
		}
	}
	return nil
}

func (m *Map) GetOrInsert(k uint64, v unsafe.Pointer) unsafe.Pointer {
	if v == nil {
		log.Fatal("GetOrInsert doesn't allow setting nil pointers.")
		return nil
	}

	// Check immutable first.
	cval := atomic.LoadPointer(&m.cs[IMMUTABLE])
	if cval != nil {
		c := (*container)(cval)
		if pv := c.get(k); pv != nil {
			return pv
		}
	}

	// Okay, deal with mutable container now.
	cval = atomic.LoadPointer(&m.cs[MUTABLE])
	if cval == nil {
		log.Fatal("This is disruptive in a bad way.")
	}
	c := (*container)(cval)
	if pv := c.getOrInsert(k, v); pv != nil {
		return pv
	}

	// We still couldn't insert the key. Time to grow.
	// TODO: Handle this case.
	return nil
}

func (m *Map) SetNilIfPresent(k uint64) bool {
	for _, c := range m.cs {
		if atomic.LoadInt32(&c.status) == 0 {
			continue
		}
		bi := k & (c.sz - 1)
		b := c.list[bi]
		for i := range b.elems {
			e := &b.elems[i]
			if atomic.LoadUint64(&e.k) == k {
				// Set to nil.
				atomic.StorePointer(&e.v, nil)
				return true
			}
		}
	}
	return false
}

func (m *Map) StreamUntilCap(ch chan uint64) {
	for {
		ci := rand.Intn(2)
		c := m.cs[ci]
		if atomic.LoadInt32(&c.status) == 0 {
			ci += 1
			c = m.cs[ci%2] // use the other.
		}
		bi := rand.Intn(int(c.sz))

		for _, e := range c.list[bi].elems {
			if len(ch) >= cap(ch) {
				return
			}
			if k := atomic.LoadUint64(&e.k); k > 0 {
				ch <- k
			}
		}
	}
}

func (m *Map) StreamAll(ch chan uint64) {
	for _, c := range m.cs {
		if atomic.LoadInt32(&c.status) == 0 {
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
