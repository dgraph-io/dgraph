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

package algo

import (
	"container/heap"
	"sort"

	"github.com/RoaringBitmap/roaring"
	"github.com/dgraph-io/dgraph/codec"
	"github.com/dgraph-io/dgraph/protos/pb"
)

// ApplyFilterPacked applies the filter to a list of packed uids.
func ApplyFilterPacked(u *pb.UidPack, f func(uint64, int) bool) *pb.UidPack {
	index := 0
	decoder := codec.NewDecoder(u)
	encoder := codec.Encoder{}

	for ; decoder.Valid(); decoder.Next() {
		for _, uid := range decoder.Uids() {
			if f(uid, index) {
				encoder.Add(uid)
			}
			index++
		}
	}

	return encoder.Done()
}

// IntersectWithLinPacked performs the linear intersection between two compressed uid lists.
func IntersectWithLinPacked(uidPack1, uidPack2 *pb.UidPack) *pb.UidPack {
	if uidPack1 == nil || uidPack2 == nil {
		return nil
	}
	intersection := codec.Encoder{}
	if len(uidPack1.Blocks) == 0 || len(uidPack2.Blocks) == 0 {
		return intersection.Done()
	}
	dec1 := codec.NewDecoder(uidPack1)
	dec2 := codec.NewDecoder(uidPack2)
	dec2Idx := 0
	var uidPack1Msb uint64
	uidPack2Msb := dec2.Pack.Blocks[dec2Idx].Base

	for dec1Idx := 0; dec1Idx < len(dec1.Pack.Blocks); dec1Idx++ {
		uidPack1Msb = dec1.Pack.Blocks[dec1Idx].Base
		for uidPack1Msb > uidPack2Msb && dec2Idx < len(dec2.Pack.Blocks) {
			dec2Idx++
			uidPack2Msb = dec2.Pack.Blocks[dec2Idx].Base
		}
		if uidPack1Msb == uidPack2Msb {
			rb := roaring.And(dec1.RoaringBitmapForBlock(dec1Idx), dec2.RoaringBitmapForBlock(dec2Idx))
			intersection.AddBlockFromBitmap(uidPack1Msb, rb, uint32(rb.GetCardinality()))
		}
		if dec2Idx == len(dec2.Pack.Blocks)-1 {
			break
		}
	}
	return intersection.Done()
}

// listInfoPacked stores the packed list in a format that allows lists to be sorted by size.
type listInfoPacked struct {
	l      *pb.UidPack
	length int
}

// IntersectSortedPacked calculates the intersection of multiple lists and performs
// the intersections from the smallest to the largest list.
func IntersectSortedPacked(uidPacks []*pb.UidPack) *pb.UidPack {
	if len(uidPacks) == 0 {
		encoder := codec.Encoder{}
		return encoder.Done()
	}
	ls := make([]listInfoPacked, 0, len(uidPacks))
	for _, uidPack := range uidPacks {
		ls = append(ls, listInfoPacked{
			l:      uidPack,
			length: codec.ExactLen(uidPack),
		})
	}
	// Sort the uidPacks based on length.
	sort.Slice(ls, func(i, j int) bool {
		return ls[i].length < ls[j].length
	})

	if len(ls) == 1 {
		// Return a copy of the UidPack.
		return codec.CopyUidPack(ls[0].l)
	}

	// TODO(martinmr): Add the rest of the algorithms.
	out := IntersectWithLinPacked(ls[0].l, ls[1].l)
	// Intersect from smallest to largest.
	for i := 2; i < len(ls); i++ {
		out := IntersectWithLinPacked(out, ls[i].l)
		// Break if we reach size 0 as we can no longer
		// add any element.
		if codec.ExactLen(out) == 0 {
			break
		}
	}
	return out
}

// DifferencePacked performs the difference operation between two UidPack objects.
func DifferencePacked(uidPack1, uidPack2 *pb.UidPack) *pb.UidPack {
	if uidPack1 == nil || uidPack2 == nil {
		// If uidPack2 == nil, then it's empty so the value of uidPack1 - uidPack2 is just uidPack1.
		// Return a copy of uidPack1.
		if uidPack2 == nil {
			return codec.CopyUidPack(uidPack1)
		}
		return nil
	}

	difference := codec.Encoder{}
	dec1 := codec.NewDecoder(uidPack1)
	dec2 := codec.NewDecoder(uidPack2)
	dec2Idx := 0
	var uidPack1Msb uint64
	uidPack2Msb := dec2.Pack.Blocks[dec2Idx].Base

	for dec1Idx := 0; dec1Idx < len(dec1.Pack.Blocks); dec1Idx++ {
		uidPack1Msb = dec1.Pack.Blocks[dec1Idx].Base
		for uidPack1Msb > uidPack2Msb && dec2Idx < len(dec2.Pack.Blocks) {
			dec2Idx++
			uidPack2Msb = dec2.Pack.Blocks[dec2Idx].Base
		}
		if uidPack1Msb == uidPack2Msb {
			rb := roaring.AndNot(dec1.RoaringBitmapForBlock(dec1Idx), dec2.RoaringBitmapForBlock(dec2Idx))
			difference.AddBlockFromBitmap(uidPack1Msb, rb, uint32(rb.GetCardinality()))
		} else {
			difference.AddBlock(dec1.Pack.Blocks[dec1Idx])
		}
	}

	return difference.Done()
}

// MergeSortedPacked merges already sorted UidPack objects into a single UidPack.
func MergeSortedPacked(lists []*pb.UidPack) *pb.UidPack {
	if len(lists) == 0 {
		return nil
	}

	h := &uint64Heap{}
	heap.Init(h)
	maxSz := 0
	blockSize := 0

	for i, l := range lists {
		if l == nil {
			continue
		}
		if blockSize == 0 {
			blockSize = int(l.BlockSize)
		}
		decoder := codec.NewDecoder(lists[i])
		block := decoder.Uids()
		if len(block) == 0 {
			continue
		}

		listLen := codec.ExactLen(lists[i])
		if listLen > 0 {
			heap.Push(h, elem{
				val:       block[0],
				listIdx:   i,
				decoder:   decoder,
				blockIdx:  0,
				blockUids: block,
			})
			if listLen > maxSz {
				maxSz = listLen
			}
		}
	}

	// Our final result.
	result := codec.Encoder{}
	// emptyResult is used to keep track of whether the encoder contains data since the
	// encoder storing the final result does not have an equivalent of len.
	emptyResult := true
	var last uint64 // Last element added to sorted / final result.

	for h.Len() > 0 { // While heap is not empty.
		me := &(*h)[0] // Peek at the top element in heap.
		if emptyResult || me.val != last {
			result.Add(me.val) // Add if unique.
			last = me.val
			emptyResult = false
		}

		// Reached the end of this list. Remove it from the heap.
		lastBlock := me.decoder.BlockIdx() == len(me.decoder.Pack.GetBlocks())-1
		if me.blockIdx == len(me.blockUids)-1 && lastBlock {
			heap.Pop(h)
			continue
		}

		// Increment counters.
		me.blockIdx++
		if me.blockIdx >= len(me.blockUids) {
			// Reached the end of the current block. Decode the next block
			// and reset the block counter.
			me.blockUids = me.decoder.Next()
			me.blockIdx = 0
		}

		// Update current value and re-heapify.
		me.val = me.blockUids[me.blockIdx]
		heap.Fix(h, 0) // Faster than Pop() followed by Push().
	}

	return result.Done()
}

// IndexOfPacked finds the index of the given uid in the UidPack. If it doesn't find it,
// it returns -1.
// FIXME
func IndexOfPacked(u *pb.UidPack, uid uint64) int {
	if u == nil {
		return -1
	}

	index := 0
	decoder := codec.Decoder{Pack: u}
	decoder.Seek(uid, codec.SeekStart)
	// Need to re-unpack this block since Seek might make Uids return an array with missing
	// elements in this block. We need them to make the correct calculation.
	decoder.UnpackBlock()

	uids := decoder.Uids()
	if len(uids) == 0 {
		return -1
	}

	searchFunc := func(i int) bool { return uids[i] >= uid }
	uidx := sort.Search(len(uids), searchFunc)
	if uids[uidx] != uid {
		return -1
	}

	index += uidx
	for i := 0; i < decoder.BlockIdx(); i++ {
		index += int(u.Blocks[i].GetNumUids())
	}

	return index
}
