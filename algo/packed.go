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

	"github.com/dgraph-io/dgraph/codec"
	"github.com/dgraph-io/dgraph/protos/pb"
)

// ApplyFilterPacked applies the filter to a list of packed uids.
func ApplyFilterPacked(u *pb.UidPack, f func(uint64, int) bool) *pb.UidPack {
	index := 0
	decoder := codec.NewDecoder(u)
	encoder := codec.Encoder{BlockSize: int(u.BlockSize)}

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

// IntersectWithLinPacked performs the liner intersection between two compressed uid lists.
func IntersectWithLinPacked(u, v *pb.UidPack) *pb.UidPack {
	if u == nil || v == nil {
		return nil
	}

	uDec := codec.NewDecoder(u)
	uUids := uDec.Uids()
	vDec := codec.NewDecoder(v)
	vUids := vDec.Uids()
	uIdx, vIdx := 0, 0
	encoder := codec.Encoder{BlockSize: int(u.BlockSize)}

	for {
		// Load the next block of the encoded lists if necessary.
		if len(uUids) == 0 || uIdx == len(uUids) {
			if uDec.Valid() {
				uUids = uDec.Next()
				uIdx = 0
			} else {
				break
			}

		}
		if len(vUids) == 0 || vIdx == len(vUids) {
			if vDec.Valid() {
				vUids = vDec.Next()
				vIdx = 0
			} else {
				break
			}
		}

		uLen := len(uUids)
		vLen := len(vUids)

		for uIdx < uLen && vIdx < vLen {
			uid := uUids[uIdx]
			vid := vUids[vIdx]
			switch {
			case uid > vid:
				for vIdx = vIdx + 1; vIdx < vLen && vUids[vIdx] < uid; vIdx++ {
				}
			case uid == vid:
				encoder.Add(uid)
				vIdx++
				uIdx++
			default:
				for uIdx = uIdx + 1; uIdx < uLen && uUids[uIdx] < vid; uIdx++ {
				}
			}
		}
	}
	return encoder.Done()
}

type listInfoPacked struct {
	l      *pb.UidPack
	length int
}

// IntersectSortedPacked calculates the intersection of multiple lists and performs
// the intersections from the smallest to the largest list.
func IntersectSortedPacked(lists []*pb.UidPack) *pb.UidPack {
	if len(lists) == 0 {
		encoder := codec.Encoder{BlockSize: 10}
		return encoder.Done()
	}
	ls := make([]listInfoPacked, 0, len(lists))
	for _, list := range lists {
		ls = append(ls, listInfoPacked{
			l:      list,
			length: codec.ExactLen(list),
		})
	}
	// Sort the lists based on length.
	sort.Slice(ls, func(i, j int) bool {
		return ls[i].length < ls[j].length
	})

	if len(ls) == 1 {
		// Return a copy of the UidPack.
		return codec.CopyUidPack(ls[0].l)
	}

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
func DifferencePacked(u, v *pb.UidPack) *pb.UidPack {
	if u == nil || v == nil {
		// If v == nil, then it's empty so the value of u - v is just u.
		// Return a copy of u.
		if v == nil {
			return codec.CopyUidPack(u)
		}

		return nil
	}

	encoder := codec.Encoder{BlockSize: int(u.BlockSize)}

	uDec := codec.NewDecoder(u)
	uUids := uDec.Uids()
	vDec := codec.NewDecoder(v)
	vUids := vDec.Uids()
	uIdx, vIdx := 0, 0

	for {
		// Load the next block of the encoded lists if necessary.
		if len(uUids) == 0 || uIdx == len(uUids) {
			if uDec.Valid() {
				uUids = uDec.Next()
				uIdx = 0
			} else {
				break
			}

		}

		if len(vUids) == 0 || vIdx == len(vUids) {
			if vDec.Valid() {
				vUids = vDec.Next()
				vIdx = 0
			} else {
				break
			}
		}

		uLen := len(uUids)
		vLen := len(vUids)

		for uIdx < uLen && vIdx < vLen {
			uid := uUids[uIdx]
			vid := vUids[vIdx]

			switch {
			case uid < vid:
				for uIdx < uLen && uUids[uIdx] < vid {
					encoder.Add(uUids[uIdx])
					uIdx++
				}
			case uid == vid:
				uIdx++
				vIdx++
			default:
				vIdx++
				for {
					if !(vIdx < vLen && vUids[vIdx] < uid) {
						break
					}
					vIdx++
				}
			}
		}

		for uIdx < uLen && vIdx >= vLen {
			encoder.Add(uUids[uIdx])
			uIdx++
		}
	}

	return encoder.Done()
}

// MergeSortedPacked merges already sorted UidPack objects into a single UidPack.
func MergeSortedPacked(lists []*pb.UidPack) *pb.UidPack {
	if len(lists) == 0 {
		return nil
	}

	h := &uint64Heap{}
	heap.Init(h)
	maxSz := 0
	// decoders stores the decoder for each corresponding, list.
	decoders := make([]*codec.Decoder, len(lists))
	// lenghts stores the length of each list so they are only computed once.
	lenghts := make([]int, len(lists))
	blockSize := 0

	for i, l := range lists {
		if l == nil {
			continue
		}

		if blockSize == 0 {
			blockSize = int(l.BlockSize)
		}

		decoders[i] = codec.NewDecoder(lists[i])
		if len(decoders[i].Uids()) == 0 {
			continue
		}

		lenList := codec.ExactLen(lists[i])
		lenghts[i] = lenList
		if lenList > 0 {
			heap.Push(h, elem{
				val:     decoders[i].Uids()[0],
				listIdx: i,
			})
			if lenList > maxSz {
				maxSz = lenList
			}
		}
	}

	// Our final output.
	output := codec.Encoder{BlockSize: blockSize}
	// empty is used to keep track of whether the encoder contains data since the
	// encoder does not have an equivalent of len.
	empty := true
	// idx[i] is the element we are looking at for lists[i].
	idx := make([]int, len(lists))
	// uidIdx is the element we are looking for within the smaller decoded array.
	uidIdx := make([]int, len(lists))
	var last uint64 // Last element added to sorted / final output.

	for h.Len() > 0 { // While heap is not empty.
		me := (*h)[0] // Peek at the top element in heap.
		if empty || me.val != last {
			output.Add(me.val) // Add if unique.
			last = me.val
			empty = false
		}

		if idx[me.listIdx] >= lenghts[me.listIdx]-1 {
			heap.Pop(h)
		} else {
			idx[me.listIdx]++
			uidIdx[me.listIdx]++
			if uidIdx[me.listIdx] >= len(decoders[me.listIdx].Uids()) {
				decoders[me.listIdx].Next()
				uidIdx[me.listIdx] = 0
			}

			val := decoders[me.listIdx].Uids()[uidIdx[me.listIdx]]
			(*h)[0].val = val
			heap.Fix(h, 0) // Faster than Pop() followed by Push().
		}
	}
	return output.Done()
}

// IndexOfPacked finds the index of the given uid in the UidPack. If it doesn't find it,
// it returns -1.
func IndexOfPacked(u *pb.UidPack, uid uint64) int {
	if u == nil {
		return -1
	}
	decoder := codec.Decoder{Pack: u}
	decoder.Seek(0, codec.SeekStart)

	for {
		if !decoder.Valid() {
			break
		}

		if decoder.PeekNextBase() < uid {
			decoder.Next()
			continue
		}

		uids := decoder.Uids()
		if len(uids) == 0 {
			break
		}

		i := sort.Search(len(uids), func(i int) bool { return uids[i] >= uid })
		if i < len(uids) && uids[i] == uid {
			return i + int(u.BlockSize)*decoder.BlockIdx()
		}

		decoder.Next()
	}

	return -1
}
