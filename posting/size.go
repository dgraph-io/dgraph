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
	"math"
	"reflect"
	"unsafe"

	"github.com/dgraph-io/dgo/v2/protos/api"
	"github.com/dgraph-io/dgraph/protos/pb"
)

const sizeOfBucket = 144

// DeepSize computes the memory taken by a Posting List
func (l *List) DeepSize() int {
	var size int
	if l == nil {
		return size
	}
	// safe mutex is of 4 words.
	size = 4 * 8
	// plist pointer is of 1 word.
	size += 1 * 8
	// mutation map pointer is of 1 word.
	size += 1 * 8
	// minTs and maxTs takes 1 word each
	size += 2 * 8
	// key array
	size += 3 * 8
	// So far 11 words, in order to round the slab we're adding one
	// more word
	size += 1 * 8
	// so far basic struct layout has been calculated.
	// List.key
	size += int(cap(l.key))

	// List.PostingList
	size += calculatePostingListSize(l.plist)
	if l.mutationMap != nil {
		// List.mutationMap
		// map has maptype and hmap
		// maptype is defined at compile time and is hardcoded in the compiled code.
		// Hence, it doesn't consume any extra memory.
		// Ref: https://dave.cheney.net/2018/05/29/how-the-go-runtime-implements-maps-efficiently-without-generics
		// Now, let's look at hmap strcut.
		// size of hmap struct
		// Ref: https://golang.org/src/runtime/map.go?#L114
		size += 6 * 8
		// we'll calculate number of buckets based on pointer arithmetic in hmap struct.
		// reflect value give us access to the hmap struct.
		hmap := reflect.ValueOf(l.mutationMap)
		nb := int(math.Pow(2, float64((*(*uint8)(unsafe.Pointer(hmap.Pointer() + uintptr(9)))))))
		nob := (*(*uint16)(unsafe.Pointer(hmap.Pointer() + uintptr(10))))
		size += int(nob * sizeOfBucket)
		if len(l.mutationMap) > 0 || nb > 1 {
			size += int(nb * sizeOfBucket)
		}
	}
	// memory taken in PostingList in Map
	for _, v := range l.mutationMap {
		size += calculatePostingListSize(v)
	}

	return size
}

// calculatePostingListSize is used to calculate the size of posting list
func calculatePostingListSize(list *pb.PostingList) int {
	var size int

	if list == nil {
		return size
	}

	// 1+3+1+3+0+3+1
	size += 12 * 8

	// PostingList.Pack
	size += calculatePackSize(list.Pack)

	// PostingList.Postings
	size += int(cap(list.Postings)) * 8
	for _, p := range list.Postings {
		size += calculatePostingSize(p)
	}

	// PostingList.Splits
	size += int(cap(list.Splits)) * 8

	// PostingList.XXX_unrecognized
	size += int(cap(list.XXX_unrecognized))

	return size
}

// calculatePostingSize is used to calculate the size of posting
func calculatePostingSize(posting *pb.Posting) int {
	var size int

	if posting == nil {
		return size
	}

	// 1 + 3 + 1 + 1 + 3 + 1 + 3 + 1 + 1 + 1 + 0 + 3 + 1
	size += 20 * 8

	// Posting.Value
	size += int(cap(posting.Value))

	// Posting.LangTag
	size += int(cap(posting.LangTag))

	// Posting.Label, strings are immutable, hence cap = len
	size += int(len(posting.Label))

	for _, f := range posting.Facets {
		size += calcuateFacet(f)
	}

	// Posting.XXX_unrecognized
	size += int(cap(posting.XXX_unrecognized))

	return size
}

// calculatePackSize is used to calculate the size of uidpack
func calculatePackSize(pack *pb.UidPack) int {
	var size int

	if pack == nil {
		return size
	}

	// size of struct UidPack (1 + 3 + 0 + 3 + 1)
	// UidPack.BlockSize consumes a full word
	size += 8 * 8

	// UidPack.Blocks, each pointer takes 1 word
	size += int(cap(pack.Blocks)) * 8
	for _, block := range pack.Blocks {
		size += calculateUIDBlock(block)
	}
	// UidPack.XXX_unrecognized
	size += int(cap(pack.XXX_unrecognized))

	return size
}

// calculateUIDBlock is used to calculate UidBlock
func calculateUIDBlock(block *pb.UidBlock) int {
	var size int

	if block == nil {
		return size
	}

	// size of struct UidBlock (1 + 3 + 1 + 0 + 3 + 1)
	// Due to alignment, both NumUids and XXX_sizecache consume 8 bytes instead of 4 bytes
	// to round the word making it 10
	size += 10 * 8

	// UidBlock.Deltas
	size += int(cap(block.Deltas))

	// UidBlock.XXX_unrecognized
	size += int(cap(block.XXX_unrecognized))

	return size
}

// calcuateFacet is used to calculate size of facet.
func calcuateFacet(facet *api.Facet) int {
	var size int
	if facet == nil {
		return size
	}
	// size of struct 1 + 3 + 1 + 3 + 1 + 0 + 3 + 1
	// rounding to 16
	size += 16 * 8
	size += int(len(facet.Key))
	size += int(cap(facet.Value))
	for _, token := range facet.Tokens {
		size += int(len(token))
	}
	size += int(len(facet.Alias))
	size += int(len(facet.XXX_unrecognized))
	return size
}
