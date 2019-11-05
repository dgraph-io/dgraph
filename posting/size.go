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
	"fmt"
	"math"
	"unsafe"

	"github.com/dgraph-io/dgraph/protos/pb"
)

// DeepSize computes the memory taken by a Posting List
func (l *List) DeepSize() int {
	var size int
	// safe mutex is of 4 words.
	size = 4 * 8
	// plist pointer is of 1 word.
	size += 1 * 8
	// mutation map pointer is of 1 word.
	size += 1 * 8
	// minTs and maxTs takes 1 word each
	size += 2 * 8
	// So far 11 words, in order to round the slab we're adding one
	// more word
	size += 1 * 8
	// so far basic struct layout has been calculated.
	// A map bucket is 16 + 8*sizeof(key) + 8*sizeof(value) bytes.
	// size of each bucket is 2 words + (8*sizeof(key)) + (8*sizeof(value))
	sizeOfBucket := 16 + (8 * 8) + (int(unsafe.Sizeof(&pb.PostingList{})) * 8)

	if l == nil {
		return size
	}
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
	if l.mutationMap != nil {
		// struct size
		// 4 + 3 + 1 + 1 (map is just a pointer) + 2 = 11
		// TODO: Russ Cox says that List will take 12 words instead of 11.
		// "I count 11 words, which will round up to 12 in the allocator"
		// Ref: https://github.com/golang/go/issues/34561
		size += 11 * 8
		// refer this for calculating number of buckets.
		// https://groups.google.com/forum/#!topic/golang-nuts/L8GbX2co3dU
		// each bucket holds 8 keys. every times map grows. size of buckets doubles.
		nb := math.Pow(2, math.Log2(float64(len(l.mutationMap))/8)+1)
		fmt.Println(nb)
		size += int(nb) * sizeOfBucket
	}

	// memory consumed by keys and values
	size += len(l.mutationMap) * 16

	// memory taken in PostingList in Map
	for _, v := range l.mutationMap {
		size += v.DeepSize()
	}

	return size
}
