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
	"sort"

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
	dec.Seek(afterUID, codec.SeekStart)
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
		if off == 0 {
			off = 1 // If v[k] isn't in u, move forward.
		}

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
				qidx := sort.Search(len(q), func(idx int) bool {
					return q[idx] >= u
				})
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
		uids := dec.Seek(u, codec.SeekStart)
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
	switch {
	case ratio < 100:
		IntersectWithLin(u.Uids, v.Uids, &dst)
	case ratio < 500:
		IntersectWithJump(u.Uids, v.Uids, &dst)
	default:
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
		switch {
		case uid > vid:
			for k = k + 1; k < m && v[k] < uid; k++ {
			}
		case uid == vid:
			*o = append(*o, uid)
			k++
			i++
		default:
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
		switch {
		case uid == vid:
			*o = append(*o, uid)
			k++
			i++
		case k+jump < m && uid > v[k+jump]:
			k += jump
		case i+jump < n && vid > u[i+jump]:
			i += jump
		case uid > vid:
			for k = k + 1; k < m && v[k] < uid; k++ {
			}
		default:
			for i = i + 1; i < n && u[i] < vid; i++ {
			}
		}
	}
	return i, k
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
		midd--
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
	l      *pb.List
	length int
}

// IntersectSorted calculates the intersection of multiple lists and performs
// the intersections from the smallest to the largest list.
func IntersectSorted(lists []*pb.UidPack) *codec.ListMap {
	if len(lists) == 0 {
		return nil
	}
	lm := codec.NewListMap(lists[0])
	for i := 1; i < len(lists); i++ {
		dst := codec.NewListMap(lists[i])
		lm.Intersect(dst)
	}
	return lm
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
		switch {
		case uid < vid:
			for i < n && u.Uids[i] < vid {
				out = append(out, u.Uids[i])
				i++
			}
		case uid == vid:
			i++
			k++
		default:
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
func MergeSorted(lists []*pb.UidPack) *codec.ListMap {
	if len(lists) == 0 {
		return nil
	}
	lm := codec.NewListMap(lists[0])
	for i := 1; i < len(lists); i++ {
		dst := codec.NewListMap(lists[i])
		lm.Merge(dst)
	}
	return lm
}

// IndexOf performs a binary search on the uids slice and returns the index at
// which it finds the uid, else returns -1
func IndexOf(u *pb.List, uid uint64) int {
	i := sort.Search(len(u.Uids), func(i int) bool { return u.Uids[i] >= uid })
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
