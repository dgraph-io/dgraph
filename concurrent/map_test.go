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
	"math/rand"
	"testing"
	"unsafe"

	"github.com/dgraph-io/dgraph/posting"
	"github.com/zond/gotomic"
)

func TestGetAndPut(t *testing.T) {
	m := NewMap(1024)
	var i uint64
	for i = 1; i < 100; i++ {
		v := new(uint64)
		*v = i
		b := unsafe.Pointer(v)
		if ok := m.Put(i, b); !ok {
			t.Errorf("Couldn't put key: %v", i)
		}
	}
	for i = 1; i < 100; i++ {
		p := m.Get(i)
		v := (*uint64)(p)
		if v == nil {
			t.Errorf("Didn't expect nil for i: %v", i)
			return
		}
		if *v != i {
			t.Errorf("Expected: %v. Got: %v", i, *v)
		}
	}
}

func BenchmarkGetAndPut(b *testing.B) {
	m := NewMap(1 << 16)
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			key := uint64(rand.Int63())
			p := m.Get(key)
			if p == nil {
				l := posting.NewList()
				m.Put(key, unsafe.Pointer(l))
			}
		}
	})
}

func BenchmarkGotomic(b *testing.B) {
	h := gotomic.NewHash()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			key := uint64(rand.Int63())
			_, has := h.Get(gotomic.IntKey(key))
			if !has {
				l := posting.NewList()
				h.Put(gotomic.IntKey(key), l)
			}
		}
	})
}
