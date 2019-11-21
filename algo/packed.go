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
