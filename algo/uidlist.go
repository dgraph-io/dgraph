/*
 * Copyright 2016-2018 Dgraph Labs, Inc. and Contributors
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

	"github.com/dgraph-io/dgraph/codec"
	"github.com/dgraph-io/dgraph/protos/pb"
)

const jump = 32 // Jump size in InsersectWithJump.

// ApplyFilter applies a filter to our UIDList.
func ApplyFilter(u *pb.List, f func(uint64, int) bool) {
	out := u.Uids[:0]
	for i, uid := range u.Uids {
		if f(uid, i) {
			out = append(out, uid)
		}
	}
	u.Uids = out
}

// IntersectCompressedWith intersects a packed list of UIDs with another list
// and writes the output to o.
func IntersectCompressedWith(pack *pb.UidPack, afterUID uint64, v, o *pb.List) {
	if pack == nil {
		return
	}
	dec := codec.Decoder{Pack: pack}
	dec.Seek(afterUID)
	n := dec.ApproxLen()
	m := len(v.Uids)

	if n > m {
		n, m = m, n
	}
	dst := o.Uids[:0]

	// If n equals 0, set it to 1 to avoid division by zero.
	if n == 0 {
		n = 1
	}

	// Select appropriate function based on heuristics.
	ratio := float64(m) / float64(n)
	if ratio < 500 {
		IntersectCompressedWithLinJump(&dec, v.Uids, &dst)
	} else {
		IntersectCompressedWithBin(&dec, v.Uids, &dst)
	}
	o.Uids = dst
}

// IntersectCompressedWithLinJump performs the intersection linearly.
func IntersectCompressedWithLinJump(dec *codec.Decoder, v []uint64, o *[]uint64) {
	m := len(v)
	k := 0
	_, off := IntersectWithLin(dec.Uids(), v[k:], o)
	k += off

	for k < m {
		u := dec.LinearSeek(v[k])
		if len(u) == 0 {
			break
		}
		_, off := IntersectWithLin(u, v[k:], o)
		k += off
	}
}

// IntersectCompressedWithBin is based on the paper
// "Fast Intersection Algorithms for Sorted Sequences"
// https://link.springer.com/chapter/10.1007/978-3-642-12476-1_3
func IntersectCompressedWithBin(dec *codec.Decoder, q []uint64, o *[]uint64) {
	ld := dec.ApproxLen()
	lq := len(q)

	if ld == 0 || lq == 0 {
		return
	}
	// Pick the shorter list and do binary search
	if ld < lq {
		uids := dec.Uids()
		for len(uids) > 0 {
			for _, u := range uids {
				n := len(q)
				i, j := 0, n
				for i < j {
					h := (i + j) >> 1
					if q[h] < u {
						i = h + 1
					} else {
						j = h
					}
				}
				qidx := i
				if qidx >= len(q) {
					return
				}
				if q[qidx] == u {
					*o = append(*o, u)
					qidx++
				}
				q = q[qidx:]
			}
			uids = dec.Next()
		}
		return
	}

	for _, u := range q {
		uids := dec.Seek(u)
		if len(uids) == 0 {
			return
		}
		if uids[0] == u {
			*o = append(*o, u)
		}
	}
}

// IntersectWith intersects u with v. The update is made to o.
// u, v should be sorted.
func IntersectWith(u, v, o *pb.List) {
	n := len(u.Uids)
	m := len(v.Uids)

	if n > m {
		n, m = m, n
	}
	if o.Uids == nil {
		o.Uids = make([]uint64, 0, n)
	}
	dst := o.Uids[:0]
	if n == 0 {
		n = 1
	}
	// Select appropriate function based on heuristics.
	ratio := float64(m) / float64(n)
	if ratio < 100 {
		IntersectWithLin(u.Uids, v.Uids, &dst)
	} else if ratio < 500 {
		IntersectWithJump(u.Uids, v.Uids, &dst)
	} else {
		IntersectWithBin(u.Uids, v.Uids, &dst)
	}
	o.Uids = dst
}

// IntersectWithLin performs the intersection linearly.
func IntersectWithLin(u, v []uint64, o *[]uint64) (int, int) {
	n := len(u)
	m := len(v)
	i, k := 0, 0
	for i < n && k < m {
		uid := u[i]
		vid := v[k]
		if uid > vid {
			for k = k + 1; k < m && v[k] < uid; k++ {
			}
		} else if uid == vid {
			*o = append(*o, uid)
			k++
			i++
		} else {
			for i = i + 1; i < n && u[i] < vid; i++ {
			}
		}
	}
	return i, k
}

// IntersectWithJump performs the intersection linearly but jumping jump steps
// between iterations.
func IntersectWithJump(u, v []uint64, o *[]uint64) (int, int) {
	n := len(u)
	m := len(v)
	i, k := 0, 0
	for i < n && k < m {
		uid := u[i]
		vid := v[k]
		if uid == vid {
			*o = append(*o, uid)
			k++
			i++
		} else if k+jump < m && uid > v[k+jump] {
			k += jump
		} else if i+jump < n && vid > u[i+jump] {
			i += jump
		} else if uid > vid {
			for k = k + 1; k < m && v[k] < uid; k++ {
			}
		} else {
			for i = i + 1; i < n && u[i] < vid; i++ {
			}
		}
	}
	return i, k
}

// IntersectWithBin is a binary search intersection algorithm
func IntersectWithBin(d, q []uint64, o *[]uint64) {
	ld := len(d)
	lq := len(q)

	if ld < lq {
		ld, lq = lq, ld
		d, q = q, d
	}
	if ld == 0 || lq == 0 || d[ld-1] < q[0] || q[lq-1] < d[0] {
		return
	}

	val := d[0]

	n := len(q)
	i, j := 0, n
	for i < j {
		h := (i + j) >> 1
		if q[h] < val {
			i = h + 1
		} else {
			j = h
		}
	}

	minq := i

	val = d[ld-1]

	i, j = 0, n
	for i < j {
		h := (i + j) >> 1
		if q[h] <= val {
			i = h + 1
		} else {
			j = h
		}
	}

	maxq := i
	q = q[minq:maxq]
	// d>=q
	for len(q) > 0 && len(d) > 0 {
		qval := q[0]

		n := len(d)
		i, j := 0, n
		for i < j {
			h := (i + j) >> 1
			if d[h] < qval {
				i = h + 1
			} else {
				j = h
			}
		}
		if i < n {
			dval := d[i]
			if dval == qval {
				*o = append(*o, qval)
				if len(q) == 1 {
					break
				}
				q = q[1:]
				d = d[i:]
			} else {
				if len(q) == 1 {
					break
				}
				q = q[1:]
				d = d[i:]

				n := len(q)
				i, j := 0, n
				for i < j {
					h := (i + j) >> 1
					if q[h] < dval {
						i = h + 1
					} else {
						j = h
					}
				}
				qval := q[i]
				if dval == qval {
					*o = append(*o, qval)
					if len(d) == 1 {
						break
					}
					q = q[i:]
					d = d[1:]
				}
			}
		}
	}
}

type listInfo struct {
	l      *pb.List
	length int
}

// IntersectSorted calculates the intersection of multiple lists and performs
// the intersections from the smallest to the largest list.
func IntersectSorted(lists []*pb.List) *pb.List {
	if len(lists) == 0 {
		return &pb.List{}
	}
	ls := make([]listInfo, 0, len(lists))
	for _, list := range lists {
		lnval := len(list.Uids)

		n := len(ls)
		i, j := 0, n
		for i < j {
			h := (i + j) >> 1
			if ls[h].length < lnval {
				i = h + 1
			} else {
				j = h
			}
		}
		ls = append(ls, listInfo{
			l:      list,
			length: lnval,
		})
		if i < n {
			copy(ls[i+1:], ls[i:])
			ls[i] = listInfo{
				l:      list,
				length: lnval,
			}
		}
	}

	out := &pb.List{Uids: make([]uint64, ls[0].length)}
	if len(ls) == 1 {
		copy(out.Uids, ls[0].l.Uids)
		return out
	}

	IntersectWith(ls[0].l, ls[1].l, out)
	// Intersect from smallest to largest.
	for i := 2; i < len(ls); i++ {
		IntersectWith(out, ls[i].l, out)
		// Break if we reach size 0 as we can no longer
		// add any element.
		if len(out.Uids) == 0 {
			break
		}
	}
	return out
}

// Difference returns the difference of two lists.
func Difference(u, v *pb.List) *pb.List {
	if u == nil || v == nil {
		return &pb.List{Uids: make([]uint64, 0)}
	}
	n := len(u.Uids)
	m := len(v.Uids)
	out := make([]uint64, 0, n/2)
	i, k := 0, 0
	for i < n && k < m {
		uid := u.Uids[i]
		vid := v.Uids[k]
		if uid < vid {
			for i < n && u.Uids[i] < vid {
				out = append(out, u.Uids[i])
				i++
			}
		} else if uid == vid {
			i++
			k++
		} else {
			for k = k + 1; k < m && v.Uids[k] < uid; k++ {
			}
		}
	}
	for i < n && k >= m {
		out = append(out, u.Uids[i])
		i++
	}
	return &pb.List{Uids: out}
}

// MergeSorted merges sorted lists.
func MergeSorted(lists []*pb.List) *pb.List {
	if len(lists) == 0 {
		return new(pb.List)
	}

	h := &uint64Heap{}
	heap.Init(h)
	maxSz := 0

	for i, l := range lists {
		if l == nil {
			continue
		}
		lenList := len(l.Uids)
		if lenList > 0 {
			heap.Push(h, elem{
				val:     l.Uids[0],
				listIdx: i,
			})
			if lenList > maxSz {
				maxSz = lenList
			}
		}
	}

	// Our final output. Give it an approximate capacity as copies are expensive.
	output := make([]uint64, 0, maxSz)
	// idx[i] is the element we are looking at for lists[i].
	idx := make([]int, len(lists))
	var last uint64   // Last element added to sorted / final output.
	for h.Len() > 0 { // While heap is not empty.
		me := (*h)[0] // Peek at the top element in heap.
		if len(output) == 0 || me.val != last {
			output = append(output, me.val) // Add if unique.
			last = me.val
		}
		l := lists[me.listIdx]
		if idx[me.listIdx] >= len(l.Uids)-1 {
			heap.Pop(h)
		} else {
			idx[me.listIdx]++
			val := l.Uids[idx[me.listIdx]]
			(*h)[0].val = val
			heap.Fix(h, 0) // Faster than Pop() followed by Push().
		}
	}
	return &pb.List{Uids: output}
}

// IndexOf performs a binary search on the uids slice and returns the index at
// which it finds the uid, else returns -1
func IndexOf(u *pb.List, uid uint64) int {
	n := len(u.Uids)
	i, j := 0, n
	for i < j {
		h := (i + j) >> 1
		if u.Uids[h] < uid {
			i = h + 1
		} else {
			j = h
		}
	}
	if i < len(u.Uids) && u.Uids[i] == uid {
		return i
	}
	return -1
}

// ToUintsListForTest converts to list of uints for testing purpose only.
func ToUintsListForTest(ul []*pb.List) [][]uint64 {
	out := make([][]uint64, 0, len(ul))
	for _, u := range ul {
		out = append(out, u.Uids)
	}
	return out
}
