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
	it := codec.NewUidPackIterator(u)
	index := 0
	encoder := codec.Encoder{BlockSize: int(u.BlockSize)}

	for ; it.Valid(); it.Next() {
		uid := it.Get()
		if f(uid, index) {
			encoder.Add(uid)
		}
		index++
	}

	return encoder.Done()
}

func IntersectWithLinPacked(u, v *pb.UidPack) *pb.UidPack {
	if u == nil || v == nil {
		return nil
	}

	n := codec.ExactLen(u)
	m := codec.ExactLen(v)
	i, k := 0, 0
	uIt := codec.NewUidPackIterator(u)
	vIt := codec.NewUidPackIterator(v)
	encoder := codec.Encoder{BlockSize: int(u.BlockSize)}

	for i < n && k < m {
		uid := uIt.Get()
		vid := vIt.Get()

		switch {
		case uid > vid:
			k++
			vIt.Next()

			for {
				if !(k < m && vIt.Get() < uid) {
					break
				}
				k++
				vIt.Next()
			}
		case uid == vid:
			encoder.Add(uid)
			i++
			uIt.Next()
			k++
			vIt.Next()
		default:
			i++
			uIt.Next()

			for {
				if !(i < n && uIt.Get() < vid) {
					break
				}

				i++
				uIt.Next()
			}
		}
	}

	return encoder.Done()
}

type packedListInfo struct {
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
	ls := make([]packedListInfo, 0, len(lists))
	for _, list := range lists {
		ls = append(ls, packedListInfo{
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

func DifferencePacked(u, v *pb.UidPack) *pb.UidPack {
	if u == nil || v == nil {
		// If v == nil, then it's empty so the value of u - v is just u.
		// Return a copy of u.
		if v == nil {
			return codec.CopyUidPack(u)
		}

		return nil
	}

	n := codec.ExactLen(u)
	m := codec.ExactLen(v)
	encoder := codec.Encoder{BlockSize: int(u.BlockSize)}
	uIt := codec.NewUidPackIterator(u)
	vIt := codec.NewUidPackIterator(v)
	i, k := 0, 0

	for i < n && k < m {
		uid := uIt.Get()
		vid := vIt.Get()

		switch {
		case uid < vid:
			for i < n && uIt.Get() < vid {
				encoder.Add(uIt.Get())
				i++
				uIt.Next()
			}
		case uid == vid:
			i++
			uIt.Next()
			k++
			vIt.Next()
		default:
			k++
			vIt.Next()
			for {
				if !(k < m && vIt.Get() < uid) {
					break
				}
				k++
				vIt.Next()
			}
		}
	}

	for i < n && k >= m {
		encoder.Add(uIt.Get())
		i++
		uIt.Next()
	}

	return encoder.Done()
}

// MergeSorted merges sorted compressed lists of uids.
func MergeSortedPacked(lists []*pb.UidPack) *pb.UidPack {
	if len(lists) == 0 {
		return nil
	}

	h := &uint64Heap{}
	heap.Init(h)
	maxSz := 0
	// iters stores the iterator for each corresponding, list.
	iters := make([]*codec.UidPackIterator, len(lists))
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

		iters[i] = codec.NewUidPackIterator(lists[i])

		lenList := codec.ExactLen(lists[i])
		lenghts[i] = lenList
		if lenList > 0 {
			heap.Push(h, elem{
				val:     iters[i].Get(),
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
	var last uint64   // Last element added to sorted / final output.
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
			iters[me.listIdx].Next()
			val := iters[me.listIdx].Get()
			(*h)[0].val = val
			heap.Fix(h, 0) // Faster than Pop() followed by Push().
		}
	}
	return output.Done()
}
