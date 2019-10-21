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
	if l.mutationMap != nil {
		size += mapDeepSize(l.mutationMap)
	}

	// memory taken in PostingList in Map
	// TODO: double calculation here? first in List.PostingList
	for _, v := range l.mutationMap {
		size += v.DeepSize()
	}

	return size
}

// MAP Usage calculation
type emptyInterface struct {
	typ unsafe.Pointer
	val unsafe.Pointer
}

type _type struct {
	size       uintptr
	ptrdata    uintptr // size of memory prefix holding all pointers
	hash       uint32
	tflag      uint8
	align      uint8
	fieldalign uint8
	kind       uint8
	alg        uintptr
	// gcdata stores the GC type data for the garbage collector.
	// If the KindGCProg bit is set in kind, gcdata is a GC program.
	// Otherwise it is a ptrmask bitmap. See mbitmap.go for details.
	gcdata    *byte
	str       int32
	ptrToThis int32
}

type maptype struct {
	typ        _type
	key        *_type
	elem       *_type
	bucket     *_type // internal type representing a hash bucket
	keysize    uint8  // size of key slot
	elemsize   uint8  // size of elem slot
	bucketsize uint16 // size of bucket
	flags      uint32
}

type hmap struct {
	count      int
	flags      uint8
	B          uint8
	noverflow  uint16
	hash0      uint32
	buckets    unsafe.Pointer
	oldbuckets unsafe.Pointer
	nevacuate  uintptr
	// it's *mapExtra but we are not interested in that for now.
	extra uintptr
}

// compute the space consumed by a map
func mapDeepSize(m interface{}) int {
	var size = 0

	// map has maptype and hmap, let's extract those first.
	ei := (*emptyInterface)(unsafe.Pointer(&m))
	mt, h := (*maptype)(ei.typ), (*hmap)(ei.val)
	if h == nil {
		return size
	}

	// maptype is defined at compile time and is hardcoded in the compiled code.
	// Hence, it doesn't consume any extra memory.
	// Ref: https://dave.cheney.net/2018/05/29/how-the-go-runtime-implements-maps-efficiently-without-generics
	size += 0

	// size of hmap struct
	// Ref: https://golang.org/src/runtime/map.go?#L114
	size += 6 * 8

	// hmap.buckets
	size += int(mt.bucket.size)

	// TODO: hmap.oldbuckets
	// TODO: hmap.extra

	return size
}
