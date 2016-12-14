/*
 * Copyright 2016 Dgraph Labs, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * 		http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package algo

import (
	"github.com/stretchr/testify/require"
	"math/rand"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/dgraph-io/dgraph/task"
)

func newList(data []uint64) *task.List {
	return &task.List{Uids: data}
}

func TestMergeSorted1(t *testing.T) {
	input := []*task.List{
		newList([]uint64{55}),
	}
	require.Equal(t, MergeSorted(input).Uids, []uint64{55})
}

func TestMergeSorted2(t *testing.T) {
	input := []*task.List{
		newList([]uint64{1, 3, 6, 8, 10}),
		newList([]uint64{2, 4, 5, 7, 15}),
	}
	require.Equal(t, MergeSorted(input).Uids,
		[]uint64{1, 2, 3, 4, 5, 6, 7, 8, 10, 15})
}

func TestMergeSorted3(t *testing.T) {
	input := []*task.List{
		newList([]uint64{1, 3, 6, 8, 10}),
		newList([]uint64{}),
	}
	require.Equal(t, MergeSorted(input).Uids, []uint64{1, 3, 6, 8, 10})
}

func TestMergeSorted4(t *testing.T) {
	input := []*task.List{
		newList([]uint64{}),
		newList([]uint64{1, 3, 6, 8, 10}),
	}
	require.Equal(t, MergeSorted(input).Uids, []uint64{1, 3, 6, 8, 10})
}

func TestMergeSorted5(t *testing.T) {
	input := []*task.List{
		newList([]uint64{}),
		newList([]uint64{}),
	}
	require.Empty(t, MergeSorted(input).Uids)
}

func TestMergeSorted6(t *testing.T) {
	input := []*task.List{
		newList([]uint64{11, 13, 16, 18, 20}),
		newList([]uint64{12, 14, 15, 15, 16, 16, 17, 25}),
		newList([]uint64{1, 2}),
	}
	require.Equal(t, MergeSorted(input).Uids,
		[]uint64{1, 2, 11, 12, 13, 14, 15, 16, 17, 18, 20, 25})
}

func TestMergeSorted7(t *testing.T) {
	input := []*task.List{
		newList([]uint64{5, 6, 7}),
		newList([]uint64{3, 4}),
		newList([]uint64{1, 2}),
		newList([]uint64{}),
	}
	require.Equal(t, MergeSorted(input).Uids, []uint64{1, 2, 3, 4, 5, 6, 7})
}

func TestMergeSorted8(t *testing.T) {
	input := []*task.List{}
	require.Empty(t, MergeSorted(input).Uids)
}

func TestMergeSorted9(t *testing.T) {
	input := []*task.List{
		newList([]uint64{1, 1, 1}),
	}
	require.Equal(t, MergeSorted(input).Uids, []uint64{1})
}

func TestMergeSorted10(t *testing.T) {
	input := []*task.List{
		newList([]uint64{1, 2, 3, 3, 6}),
		newList([]uint64{4, 8, 9}),
	}
	require.Equal(t, MergeSorted(input).Uids, []uint64{1, 2, 3, 4, 6, 8, 9})
}

func TestIntersectSorted1(t *testing.T) {
	input := []*task.List{
		newList([]uint64{1, 2, 3}),
		newList([]uint64{2, 3, 4, 5}),
	}
	require.Equal(t, IntersectSorted(input).Uids, []uint64{2, 3})
}

func TestIntersectSorted2(t *testing.T) {
	input := []*task.List{
		newList([]uint64{1, 2, 3}),
	}
	require.Equal(t, IntersectSorted(input).Uids, []uint64{1, 2, 3})
}

func TestIntersectSorted3(t *testing.T) {
	input := []*task.List{}
	require.Empty(t, IntersectSorted(input).Uids)
}

func TestIntersectSorted4(t *testing.T) {
	input := []*task.List{
		newList([]uint64{100, 101}),
	}
	require.Equal(t, IntersectSorted(input).Uids, []uint64{100, 101})
}

func TestIntersectSorted5(t *testing.T) {
	input := []*task.List{
		newList([]uint64{1, 2, 3}),
		newList([]uint64{2, 3, 4, 5}),
		newList([]uint64{4, 5, 6}),
	}
	require.Empty(t, IntersectSorted(input).Uids)
}

func TestIntersectSorted6(t *testing.T) {
	input := []*task.List{
		newList([]uint64{10, 12, 13}),
		newList([]uint64{2, 3, 4, 13}),
		newList([]uint64{4, 5, 6}),
	}
	require.Empty(t, IntersectSorted(input).Uids)
}

func TestUIDListIntersect1(t *testing.T) {
	u := newList([]uint64{1, 2, 3})
	v := newList([]uint64{})
	IntersectWith(u, v)
	require.Empty(t, u.Uids)
}

func TestUIDListIntersect2(t *testing.T) {
	u := newList([]uint64{1, 2, 3})
	v := newList([]uint64{1, 2, 3, 4, 5})
	IntersectWith(u, v)
	require.Equal(t, u.Uids, []uint64{1, 2, 3})
}

func TestUIDListIntersect3(t *testing.T) {
	u := newList([]uint64{1, 2, 3})
	v := newList([]uint64{2})
	IntersectWith(u, v)
	require.Equal(t, u.Uids, []uint64{2})
}

func TestUIDListIntersect4(t *testing.T) {
	u := newList([]uint64{1, 2, 3})
	v := newList([]uint64{0, 5})
	IntersectWith(u, v)
	require.Empty(t, u.Uids)
}

func TestUIDListIntersect5(t *testing.T) {
	u := newList([]uint64{1, 2, 3})
	v := newList([]uint64{3, 5})
	IntersectWith(u, v)
	require.Equal(t, u.Uids, []uint64{3})
}

func TestUIDListIntersectDupFirst(t *testing.T) {
	u := newList([]uint64{1, 1, 2, 3})
	v := newList([]uint64{1, 2})
	IntersectWith(u, v)
	require.Equal(t, []uint64{1, 2}, u.Uids)
}

func TestUIDListIntersectDupBoth(t *testing.T) {
	u := newList([]uint64{1, 1, 2, 3, 5})
	v := newList([]uint64{1, 1, 2, 4})
	IntersectWith(u, v)
	require.Equal(t, []uint64{1, 1, 2}, u.Uids)
}

func TestUIDListIntersectDupSecond(t *testing.T) {
	u := newList([]uint64{1, 2, 3, 5})
	v := newList([]uint64{1, 1, 2, 4})
	IntersectWith(u, v)
	require.Equal(t, []uint64{1, 2}, u.Uids)
}

func TestApplyFilterUint(t *testing.T) {
	u := newList([]uint64{1, 2, 3, 4, 5})
	ApplyFilter(u, func(a uint64, idx int) bool { return (a % 2) == 1 })
	require.Equal(t, u.Uids, []uint64{1, 3, 5})
}

// sort interface for []uint64
type Uint64Slice []uint64

func (xs Uint64Slice) Len() int {
	return len(xs)
}
func (xs Uint64Slice) Less(i, j int) bool {
	return xs[i] < xs[j]
}
func (xs Uint64Slice) Swap(i, j int) {
	xs[i], xs[j] = xs[j], xs[i]
}

// Benchmarks for IntersectWith
// random data : u and v having data within range [0,limit] where limit = N * sizeof-list
func benchmarkListIntersectRandom(arrSz int, limit int64, r *rand.Rand, b *testing.B) {
	u1, v1 := make([]uint64, arrSz, arrSz), make([]uint64, arrSz, arrSz)
	for i := 0; i < arrSz; i++ {
		u1[i] = uint64(r.Int63n(limit))
		v1[i] = uint64(r.Int63n(limit))
	}
	sort.Sort(Uint64Slice(u1))
	sort.Sort(Uint64Slice(v1))

	b.Run(":2forLoops:"+strconv.Itoa(arrSz), func(b *testing.B) {
		u := newList(u1)
		v := newList(v1)
		ucopy := make([]uint64, len(u1), len(u1))
		copy(ucopy, u1)
		b.ResetTimer()
		for k := 0; k < b.N; k++ {
			IntersectWith(u, v)
			u.Uids = u.Uids[:arrSz]
			copy(u.Uids, ucopy)
		}
	})
}

func BenchmarkListIntersectRandom(b *testing.B) {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	randomTests := func(sz int, factor int) {
		sizeStr := strconv.Itoa(sz)
		facStr := strconv.Itoa(factor)
		b.Run(":random:factor="+facStr+":size="+sizeStr, func(b *testing.B) { benchmarkListIntersectRandom(sz, int64(sz*factor), r, b) })
	}

	randomTests(500, 3)
	randomTests(10000, 3)
	randomTests(1000000, 3)
	randomTests(500, 10)
	randomTests(10000, 10)
	randomTests(1000000, 10)
	randomTests(500, 100)
	randomTests(10000, 100)
	randomTests(1000000, 100)
}
