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
	// TODO: Use UIDSet
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
func IntersectWithLinPacked(UIDPack1, UIDPack2 *pb.UidPack) *pb.UidPack {
	if UIDPack1 == nil || UIDPack2 == nil {
		return nil
	}

	return codec.Intersect(codec.UIDSetFromPack(UIDPack1), codec.UIDSetFromPack(UIDPack2)).ToPack()
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

	for ; dec1.Valid(); dec1.Next() {
		for dec2.Valid() && dec1.CurrentBase() > dec2.CurrentBase() {
			dec2.Next()
		}
		if dec2.Valid() && dec1.CurrentBase() == dec2.CurrentBase() {
			rb := roaring.AndNot(dec1.UnpackBlockRoaringBitmap(), dec2.UnpackBlockRoaringBitmap())
			difference.AddBlockFromBitmap(dec1.CurrentBase(), rb, uint32(rb.GetCardinality()))
		} else {
			difference.AddBlock(dec1.CurrentBlock())
		}
	}

	return difference.Done()
}

// MergeSortedPacked merges already sorted UidPack objects into a single UidPack.
func MergeSortedPacked(uidPacks []*pb.UidPack) *pb.UidPack {
	if len(uidPacks) == 0 {
		return nil
	}

	decoderHeap := &uint64Heap{}
	heap.Init(decoderHeap)

	for i, uidPack := range uidPacks {
		if uidPack == nil || len(uidPack.Blocks) == 0 {
			continue
		}
		decoder := codec.NewDecoder(uidPacks[i])

		listLen := codec.ExactLen(uidPacks[i])
		if listLen > 0 {
			heap.Push(decoderHeap, elem{
				val:      decoder.Pack.Blocks[0].Base,
				listIdx:  i,
				decoder:  decoder,
				blockIdx: 0,
			})
		}
	}

	result := codec.NewEncoder()
	for decoderHeap.Len() > 0 { // While heap is not empty.

		currentDecoder := heap.Pop(decoderHeap).(elem)
		var currentRb *roaring.Bitmap = nil

		for decoderHeap.Len() > 0 {
			// Peek the next block and check that it has the same base.
			peekedNextDecoder := &(*decoderHeap)[0]
			if currentDecoder.val != peekedNextDecoder.val {
				break
			}
			// Pop the next block.
			nextDecoder := heap.Pop(decoderHeap).(elem)
			// Add its successor to the heap.
			if len(nextDecoder.decoder.Pack.Blocks) > (nextDecoder.blockIdx + 1) {
				heap.Push(decoderHeap, elem{
					val:      nextDecoder.decoder.Pack.Blocks[nextDecoder.blockIdx+1].Base,
					listIdx:  nextDecoder.listIdx,
					decoder:  nextDecoder.decoder,
					blockIdx: nextDecoder.blockIdx + 1,
				})
			}
			// Merge both blocks into the current block.
			if currentRb == nil {
				currentRb = currentDecoder.decoder.RoaringBitmapForBlock(currentDecoder.blockIdx)
			}
			currentRb.Or(nextDecoder.decoder.RoaringBitmapForBlock(nextDecoder.blockIdx))
		}
		if currentRb == nil {
			result.AddBlock(currentDecoder.decoder.Pack.Blocks[currentDecoder.blockIdx])
		} else {
			result.AddBlockFromBitmap(currentDecoder.val, currentRb, uint32(currentRb.GetCardinality()))
		}

		if len(currentDecoder.decoder.Pack.Blocks) > (currentDecoder.blockIdx + 1) {
			heap.Push(decoderHeap, elem{
				val:      currentDecoder.decoder.Pack.Blocks[currentDecoder.blockIdx+1].Base,
				listIdx:  currentDecoder.listIdx,
				decoder:  currentDecoder.decoder,
				blockIdx: currentDecoder.blockIdx + 1,
			})
		}
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
	decoder := codec.NewDecoder(u)
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
	if uidx == len(uids) || uids[uidx] != uid {
		return -1
	}

	index += uidx
	for i := 0; i < decoder.BlockIdx(); i++ {
		index += int(u.Blocks[i].GetNumUids())
	}

	return index
}
