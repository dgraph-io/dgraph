/*
 * Copyright (C) 2017 Dgraph Labs, Inc. and Contributors
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 */

package algo

import (
	"container/heap"
	"sort"

	"github.com/dgraph-io/dgraph/protos/taskp"
)

const jump = 32 // Jump size in InsersectWithJump.

// ApplyFilter applies a filter to our UIDList.
func ApplyFilter(u *taskp.List, f func(uint64, int) bool) {
	out := u.Uids[:0]
	for i, uid := range u.Uids {
		if f(uid, i) {
			out = append(out, uid)
		}
	}
	u.Uids = out
}

// IntersectWith intersects u with v. The update is made to o.
// u, v should be sorted.
func IntersectWith(u, v, o *taskp.List) {
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
		n += 1
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

func IntersectWithLin(u, v []uint64, o *[]uint64) {
	n := len(u)
	m := len(v)
	for i, k := 0, 0; i < n && k < m; {
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
}

func IntersectWithJump(u, v []uint64, o *[]uint64) {
	n := len(u)
	m := len(v)
	for i, k := 0, 0; i < n && k < m; {
		uid := u[i]
		vid := v[k]
		if uid == vid {
			*o = append(*o, uid)
			k++
			i++
		} else if k+jump < m && uid > v[k+jump] {
			k = k + jump
		} else if i+jump < n && vid > u[i+jump] {
			i = i + jump
		} else if uid > vid {
			for k = k + 1; k < m && v[k] < uid; k++ {
			}
		} else {
			for i = i + 1; i < n && u[i] < vid; i++ {
			}
		}
	}
}

// IntersectWithBin is based on the paper
// "Fast Intersection Algorithms for Sorted Sequences"
// https://link.springer.com/chapter/10.1007/978-3-642-12476-1_3
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
	minq := sort.Search(len(q), func(i int) bool {
		return q[i] >= val
	})

	val = d[len(d)-1]
	maxq := sort.Search(len(q), func(i int) bool {
		return q[i] > val
	})

	binIntersect(d, q[minq:maxq], o)
}

// binIntersect is the recursive function used.
// NOTE: len(d) >= len(q) (Must hold)
func binIntersect(d, q []uint64, final *[]uint64) {
	if len(d) == 0 || len(q) == 0 {
		return
	}
	midq := len(q) / 2
	qval := q[midq]
	midd := sort.Search(len(d), func(i int) bool {
		return d[i] >= qval
	})

	dd := d[0:midd]
	qq := q[0:midq]
	if len(dd) > len(qq) { // D > Q
		binIntersect(dd, qq, final)
	} else {
		binIntersect(qq, dd, final)
	}

	if midd >= len(d) {
		return
	}
	if d[midd] == qval {
		*final = append(*final, qval)
	} else {
		midd -= 1
	}

	dd = d[midd+1:]
	qq = q[midq+1:]
	if len(dd) > len(qq) { // D > Q
		binIntersect(dd, qq, final)
	} else {
		binIntersect(qq, dd, final)
	}
}

type listInfo struct {
	l      *taskp.List
	length int
}

func IntersectSorted(lists []*taskp.List) *taskp.List {
	if len(lists) == 0 {
		return &taskp.List{}
	}
	ls := make([]listInfo, 0, len(lists))
	for _, list := range lists {
		ls = append(ls, listInfo{
			l:      list,
			length: len(list.Uids),
		})
	}
	// Sort the lists based on length.
	sort.Slice(ls, func(i, j int) bool {
		return ls[i].length < ls[j].length
	})
	out := &taskp.List{Uids: make([]uint64, ls[0].length)}
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

func Difference(u, v *taskp.List) {
	if u == nil || v == nil {
		return
	}
	out := u.Uids[:0]
	n := len(u.Uids)
	m := len(v.Uids)
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
	u.Uids = out
}

// MergeSorted merges sorted lists.
func MergeSorted(lists []*taskp.List) *taskp.List {
	if len(lists) == 0 {
		return new(taskp.List)
	}

	h := &uint64Heap{}
	heap.Init(h)
	maxSz := 0

	for i, l := range lists {
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
	return &taskp.List{Uids: output}
}

// IndexOf performs a binary search on the uids slice and returns the index at
// which it finds the uid, else returns -1
func IndexOf(u *taskp.List, uid uint64) int {
	i := sort.Search(len(u.Uids), func(i int) bool { return u.Uids[i] >= uid })
	if i < len(u.Uids) && u.Uids[i] == uid {
		return i
	}
	return -1
}

// ToUintsListForTest converts to list of uints for testing purpose only.
func ToUintsListForTest(ul []*taskp.List) [][]uint64 {
	out := make([][]uint64, 0, len(ul))
	for _, u := range ul {
		out = append(out, u.Uids)
	}
	return out
}
