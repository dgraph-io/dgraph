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

import "unsafe"

// DeepSize computes the memory taken by a Posting List
func (l *List) DeepSize() int {
	var size int

	if l == nil {
		return size
	}

	// struct size
	// 4 + 3 + 1 + 1 (map is just a pointer) + 2 = 11
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
	// Now, let's look at hmap strcut.
	// size of hmap struct
	// Ref: https://golang.org/src/runtime/map.go?#L114
	size += 6 * 8

	// A map bucket is 16 + 8*sizeof(key) + 8*sizeof(value) bytes.
	// size of each bucket is 16 + 1 + 1 = 18
	// Number of buckets is stored at 10th byte in the hmap struct.
	// The pointer stored in the struct is the pointer to hmap struct of the map.
	// For List struct, pointer is stored at the 64th position
	// 64 is the offset of the map in the struct
	// 9 is the offset of B (storing number of buckets in log_2) in hmap struct
	if l.mutationMap != nil {
		nb := 1 << (*(*uint8)(unsafe.Pointer(*(*uintptr)(unsafe.Pointer(uintptr(unsafe.Pointer(l)) + 64)) + 9)))
		size += 18 * 8 * nb
	}

	// memory consumed by keys and values
	size += len(l.mutationMap) * 16

	// memory taken in PostingList in Map
	for _, v := range l.mutationMap {
		size += v.DeepSize()
	}

	return size
}
