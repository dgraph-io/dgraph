/*
 * Copyright 2019 Dgraph Labs, Inc. and Contributors
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

package posting

import (
	"unsafe"
)

type mapHeader struct {
	count      int
	flags      uint8
	B          uint8
	noverflow  uint16
	hash0      uint32
	buckets    unsafe.Pointer
	oldbuckets unsafe.Pointer
	nevacuate  uintptr
	extra      *int // just so that we don't have to define mapExtra
}

// DeepSize computes the memory taken by a Posting List
func (l *List) DeepSize() int {
	l.RLock()
	defer l.Unlock()

	var size int
	if l == nil {
		return size
	}

	// struct size
	// 4 + 3 + 1 + 1 (map is just a pointer) + 1 + 1 = 11 words
	// TODO: Russ Cox says that List will take 12 words instead of 11.
	// "I count 11 words, which will round up to 12 in the allocator"
	// Ref: https://github.com/golang/go/issues/34561
	size += 11 * 8

	// List.key
	size += cap(l.key)

	// List.PostingList
	size += l.plist.DeepSize()

	// List.mutationMap
	// map has maptype and hmap
	// maptype is defined at compile time and is hardcoded in the compiled code.
	// Hence, it doesn't consume any extra memory.
	// Ref: https://dave.cheney.net/2018/05/29/how-the-go-runtime-implements-maps-efficiently-without-generics
	// Now, let's look at hmap stuct.
	// size of hmap struct
	// Ref: https://golang.org/src/runtime/map.go?#L114
	size += 6 * 8

	// A map bucket is 16 + 8*sizeof(key) + 8*sizeof(value) bytes.
	// size of each bucket is 16 + 1 + 1 = 18
	// TODO: This doesn't seem accurate
	if l.mutationMap != nil {
		hh := (*mapHeader)(unsafe.Pointer(*(*uintptr)(unsafe.Pointer(&l.mutationMap))))
		size += 16 * ((1 << hh.B) + int(hh.noverflow))
		size += 2 * 8 * hh.count
	}

	// memory consumed by keys and values
	size += len(l.mutationMap) * 16

	// memory taken in PostingList in Map
	for _, v := range l.mutationMap {
		size += v.DeepSize()
	}

	return size
}
