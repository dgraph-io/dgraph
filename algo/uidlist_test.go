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
	"fmt"
	"math/rand"
	"sort"
	"testing"

	"github.com/dgraph-io/dgraph/protos/taskp"
	"github.com/stretchr/testify/require"
)

func newList(data []uint64) *taskp.List {
	return &taskp.List{data}
}

func TestMergeSorted1(t *testing.T) {
	input := []*taskp.List{
		newList([]uint64{55}),
	}
	require.Equal(t, MergeSorted(input).Uids, []uint64{55})
}

func TestMergeSorted2(t *testing.T) {
	input := []*taskp.List{
		newList([]uint64{1, 3, 6, 8, 10}),
		newList([]uint64{2, 4, 5, 7, 15}),
	}
	require.Equal(t, MergeSorted(input).Uids,
		[]uint64{1, 2, 3, 4, 5, 6, 7, 8, 10, 15})
}

func TestMergeSorted3(t *testing.T) {
	input := []*taskp.List{
		newList([]uint64{1, 3, 6, 8, 10}),
		newList([]uint64{}),
	}
	require.Equal(t, MergeSorted(input).Uids, []uint64{1, 3, 6, 8, 10})
}

func TestMergeSorted4(t *testing.T) {
	input := []*taskp.List{
		newList([]uint64{}),
		newList([]uint64{1, 3, 6, 8, 10}),
	}
	require.Equal(t, MergeSorted(input).Uids, []uint64{1, 3, 6, 8, 10})
}

func TestMergeSorted5(t *testing.T) {
	input := []*taskp.List{
		newList([]uint64{}),
		newList([]uint64{}),
	}
	require.Empty(t, MergeSorted(input).Uids)
}

func TestMergeSorted6(t *testing.T) {
	input := []*taskp.List{
		newList([]uint64{11, 13, 16, 18, 20}),
		newList([]uint64{12, 14, 15, 15, 16, 16, 17, 25}),
		newList([]uint64{1, 2}),
	}
	require.Equal(t, MergeSorted(input).Uids,
		[]uint64{1, 2, 11, 12, 13, 14, 15, 16, 17, 18, 20, 25})
}

func TestMergeSorted7(t *testing.T) {
	input := []*taskp.List{
		newList([]uint64{5, 6, 7}),
		newList([]uint64{3, 4}),
		newList([]uint64{1, 2}),
		newList([]uint64{}),
	}
	require.Equal(t, MergeSorted(input).Uids, []uint64{1, 2, 3, 4, 5, 6, 7})
}

func TestMergeSorted8(t *testing.T) {
	input := []*taskp.List{}
	require.Empty(t, MergeSorted(input).Uids)
}

func TestMergeSorted9(t *testing.T) {
	input := []*taskp.List{
		newList([]uint64{1, 1, 1}),
	}
	require.Equal(t, MergeSorted(input).Uids, []uint64{1})
}

func TestMergeSorted10(t *testing.T) {
	input := []*taskp.List{
		newList([]uint64{1, 2, 3, 3, 6}),
		newList([]uint64{4, 8, 9}),
	}
	require.Equal(t, MergeSorted(input).Uids, []uint64{1, 2, 3, 4, 6, 8, 9})
}

func TestIntersectSorted1(t *testing.T) {
	input := []*taskp.List{
		newList([]uint64{1, 2, 3}),
		newList([]uint64{2, 3, 4, 5}),
	}
	require.Equal(t, []uint64{2, 3}, IntersectSorted(input).Uids)
}

func TestIntersectSorted2(t *testing.T) {
	input := []*taskp.List{
		newList([]uint64{1, 2, 3}),
	}
	require.Equal(t, IntersectSorted(input).Uids, []uint64{1, 2, 3})
}

func TestIntersectSorted3(t *testing.T) {
	input := []*taskp.List{}
	require.Empty(t, IntersectSorted(input).Uids)
}

func TestIntersectSorted4(t *testing.T) {
	input := []*taskp.List{
		newList([]uint64{100, 101}),
	}
	require.Equal(t, IntersectSorted(input).Uids, []uint64{100, 101})
}

func TestIntersectSorted5(t *testing.T) {
	input := []*taskp.List{
		newList([]uint64{1, 2, 3}),
		newList([]uint64{2, 3, 4, 5}),
		newList([]uint64{4, 5, 6}),
	}
	require.Empty(t, IntersectSorted(input).Uids)
}

func TestIntersectSorted6(t *testing.T) {
	input := []*taskp.List{
		newList([]uint64{10, 12, 13}),
		newList([]uint64{2, 3, 4, 13}),
		newList([]uint64{4, 5, 6}),
	}
	require.Empty(t, IntersectSorted(input).Uids)
}

func TestSubSorted1(t *testing.T) {
	input := []*taskp.List{
		newList([]uint64{1, 2, 3}),
		newList([]uint64{2, 3, 4, 5}),
	}
	Difference(input[0], input[1])
	require.Equal(t, []uint64{1}, input[0].Uids)
}

func TestSubSorted6(t *testing.T) {
	input := []*taskp.List{
		newList([]uint64{10, 12, 13}),
		newList([]uint64{2, 3, 4, 13}),
	}
	Difference(input[0], input[1])
	require.Equal(t, []uint64{10, 12}, input[0].Uids)
}

func TestUIDListIntersect1(t *testing.T) {
	u := newList([]uint64{1, 2, 3})
	v := newList([]uint64{})
	IntersectWith(u, v, u)
	require.Empty(t, u.Uids)
}

func TestUIDListIntersect2(t *testing.T) {
	u := newList([]uint64{1, 2, 3})
	v := newList([]uint64{1, 2, 3, 4, 5})
	IntersectWith(u, v, u)
	require.Equal(t, []uint64{1, 2, 3}, u.Uids)
	require.Equal(t, []uint64{1, 2, 3, 4, 5}, v.Uids)
}

func TestUIDListIntersect3(t *testing.T) {
	u := newList([]uint64{1, 2, 3})
	v := newList([]uint64{2})
	IntersectWith(u, v, u)
	require.Equal(t, []uint64{2}, u.Uids)
	require.Equal(t, []uint64{2}, v.Uids)
}

func TestUIDListIntersect4(t *testing.T) {
	u := newList([]uint64{1, 2, 3})
	v := newList([]uint64{0, 5})
	IntersectWith(u, v, u)
	require.Empty(t, u.Uids)
	require.Equal(t, []uint64{0, 5}, v.Uids)
}

func TestUIDListIntersect5(t *testing.T) {
	u := newList([]uint64{1, 2, 3})
	v := newList([]uint64{3, 5})
	IntersectWith(u, v, u)
	require.Equal(t, []uint64{3}, u.Uids)
}

func TestUIDListIntersectDupFirst(t *testing.T) {
	u := newList([]uint64{1, 1, 2, 3})
	v := newList([]uint64{1, 2})
	IntersectWith(u, v, u)
	require.Equal(t, []uint64{1, 2}, u.Uids)
}

func TestUIDListIntersectDupBoth(t *testing.T) {
	u := newList([]uint64{1, 1, 2, 3, 5})
	v := newList([]uint64{1, 1, 2, 4})
	IntersectWith(u, v, u)
	require.Equal(t, []uint64{1, 1, 2}, u.Uids)
}

func TestUIDListIntersectDupSecond(t *testing.T) {
	u := newList([]uint64{1, 2, 3, 5})
	v := newList([]uint64{1, 1, 2, 4})
	IntersectWith(u, v, u)
	require.Equal(t, []uint64{1, 2}, u.Uids)
}

func TestApplyFilterUint(t *testing.T) {
	l := []uint64{1, 2, 3, 4, 5}
	u := newList(l)
	ApplyFilter(u, func(a uint64, idx int) bool { return (l[idx] % 2) == 1 })
	require.Equal(t, []uint64{1, 3, 5}, u.Uids)
}

// Benchmarks for IntersectWith
// random data : u and v having data within range [0, limit)
// where limit = N * sizeof-list ; for different N
func runIntersectRandom(arrSz int, limit int64, b *testing.B) {
	u1, v1 := make([]uint64, arrSz, arrSz), make([]uint64, arrSz, arrSz)
	for i := 0; i < arrSz; i++ {
		u1[i] = uint64(rand.Int63n(limit))
		v1[i] = uint64(rand.Int63n(limit))
	}
	sort.Slice(u1, func(i, j int) bool { return u1[i] < u1[j] })
	sort.Slice(v1, func(i, j int) bool { return v1[i] < v1[j] })

	u := newList(u1)
	v := newList(v1)
	ucopy := make([]uint64, len(u1), len(u1))
	copy(ucopy, u1)

	b.ResetTimer()
	for k := 0; k < b.N; k++ {
		IntersectWith(u, v, u)
		u.Uids = u.Uids[:arrSz]
		copy(u.Uids, ucopy)
	}

}

func BenchmarkListIntersectRandom(b *testing.B) {
	randomTests := func(sz int, overlap float64) {
		b.Run(fmt.Sprintf(":random:size=%d:overlap=%.2f:", sz, overlap),
			func(b *testing.B) {
				runIntersectRandom(sz, int64(float64(sz)/overlap), b)
			})
	}

	randomTests(500, 0.3)
	randomTests(10000, 0.3)
	randomTests(1000000, 0.3)
	randomTests(500, 0.1)
	randomTests(10000, 0.1)
	randomTests(1000000, 0.1)
	randomTests(500, 0.01)
	randomTests(10000, 0.01)
	randomTests(1000000, 0.01)
}

func BenchmarkListIntersectRatio(b *testing.B) {
	randomTests := func(sz int, overlap float64) {
		sz1 := sz
		sz2 := sz
		rs := []int{1, 10, 50, 100, 500, 1000, 10000, 100000, 1000000}
		for _, r := range rs {
			sz1 = sz
			sz2 = sz * r
			if sz2 > 1000000 {
				break
			}

			u1, v1 := make([]uint64, sz1, sz1), make([]uint64, sz2, sz2)
			limit := int64(float64(sz) / overlap)
			for i := 0; i < sz1; i++ {
				u1[i] = uint64(rand.Int63n(limit))
			}
			for i := 0; i < sz2; i++ {
				v1[i] = uint64(rand.Int63n(limit))
			}
			sort.Slice(u1, func(i, j int) bool { return u1[i] < u1[j] })
			sort.Slice(v1, func(i, j int) bool { return v1[i] < v1[j] })

			u := &taskp.List{u1}
			v := &taskp.List{v1}
			ucopy := make([]uint64, len(u1), len(u1))
			copy(ucopy, u1)

			b.Run(fmt.Sprintf(":IntersectWith:ratio=%d:size=%d:overlap=%.2f:", r, sz, overlap),
				func(b *testing.B) {
					for k := 0; k < b.N; k++ {
						u.Uids = u.Uids[:sz1]
						copy(u.Uids, ucopy)
						IntersectWith(u, v, u)
					}
				})
		}
	}

	randomTests(10, 0.01)
	randomTests(100, 0.01)
	randomTests(1000, 0.01)
	randomTests(10000, 0.01)
	randomTests(100000, 0.01)
	randomTests(1000000, 0.01)

	randomTests(10, 0.1)
	randomTests(100, 0.1)
	randomTests(1000, 0.1)
	randomTests(10000, 0.1)
	randomTests(100000, 0.1)
	randomTests(1000000, 0.1)

	randomTests(10, 0.4)
	randomTests(100, 0.4)
	randomTests(1000, 0.4)
	randomTests(10000, 0.4)
	randomTests(100000, 0.4)
	randomTests(1000000, 0.4)

	randomTests(10, 0.8)
	randomTests(100, 0.8)
	randomTests(1000, 0.8)
	randomTests(10000, 0.8)
	randomTests(100000, 0.8)
	randomTests(1000000, 0.8)
}
