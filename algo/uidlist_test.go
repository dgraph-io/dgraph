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
	"testing"
	"strconv"
	"math/rand"
	"sort"
	"time"
	"github.com/stretchr/testify/require"

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

// Benchmarks for IntersectWith
// different data :
// 1. random data : u and v having data within range [0,limit] where limit = N * sizeof-list
// 2. uniformly distributed : u : 0,1,2.. and v : 0,100,200..
// 3. non-uniformly distributed : u : 0,1,2,..100,1001,1002,..2000 and v : 0,1,2,3..1000

func uintsFromInts(ints []int) []uint64 {
	arrSz := len(ints)
	uints := make([]uint64, arrSz, arrSz)
	alwaysIncreasing := uint64(0)
	for k := 0; k < arrSz; k++ {
		uints[k] = uint64(ints[k])
		if uints[k] < alwaysIncreasing {
			panic("Bad data") // go test . -v
		}
		alwaysIncreasing = uints[k]
	}
	return uints
}

func benchmarkListIntersectRandom(arrSz int, limit int, r *rand.Rand, b *testing.B) {
	u1, v1 := make([]int, arrSz, arrSz), make([]int, arrSz, arrSz)
	abs := func(x int) int { if x < 0 { return -x } else { return x } }

	for i := 0; i < arrSz; i++ {
		u1[i] = abs(r.Int()) % limit
		v1[i] = abs(r.Int()) % limit
	}
	sort.Ints(u1)
	sort.Ints(v1)

	v := newList(uintsFromInts(v1))

	b.Run(":curr:" + strconv.Itoa(arrSz), func(b *testing.B) {
		u := newList(uintsFromInts(u1))
		ucopy := uintsFromInts(u1)
		b.ResetTimer()
		for k := 0; k < b.N; k++ {
			IntersectWith(u, v)
			u.Uids = u.Uids[:arrSz]
			copy(u.Uids, ucopy)
		}
	})
	b.Run(":curr:ordChg:" + strconv.Itoa(arrSz), func(b *testing.B) {
		vArg := newList(uintsFromInts(u1))
		uArg := newList(uintsFromInts(v1))
		ucopy := uintsFromInts(v1)
		b.ResetTimer()
		for k := 0; k < b.N; k++ {
			IntersectWith(uArg, vArg)
			uArg.Uids = uArg.Uids[:arrSz]
			copy(uArg.Uids, ucopy)
		}
	})
}

func BenchmarkListIntersectRandom(b *testing.B) {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))

	randomTests := func(factor int) {
		b.Run(strconv.Itoa(factor) + ":random:size=500", func (b *testing.B) { benchmarkListIntersectRandom(500, 500*factor, r, b) })
		b.Run(strconv.Itoa(factor) + ":random:size=10000", func (b *testing.B) { benchmarkListIntersectRandom(10000, 10000*factor, r, b) })
		b.Run(strconv.Itoa(factor) + ":random:size=1000000", func (b *testing.B) { benchmarkListIntersectRandom(1000000, 1000000*factor, r, b) })
	}

	randomTests(3);
	randomTests(10);
	randomTests(100);
}

func GenList(n int, f func(int, int) int) *task.List {
	var arr = make([]uint64, n, n)
	var incNos uint64
	for i := 0; i < n; i++ {
		arr[i] = uint64(f(i, n))
		if (incNos > arr[i]) {
			panic("Non increasing sequence of numbers")
		}
		incNos = arr[i]
	}
	return newList(arr)
}

func benchmarkListIntersectFn(sz int, f1 func(int, int) int, f2 func(int, int) int, intersectF func(u, v *task.List), b *testing.B) {
	u, v := GenList(sz, f1), GenList(sz, f2)
	ucopy := GenList(sz, f1)
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		intersectF(u, v)
		// ~ this copy adds ~10% overhead in benchmarks for all
		u.Uids = u.Uids[:sz]
		copy(u.Uids, ucopy.Uids)
	}
}

func benchmarkListIntersectAll(name string, ignoreBelowSz int, f1 func(int, int) int, f2 func(int, int) int, b *testing.B) {
	runBenchmarks := func(n int) {
		if ignoreBelowSz < n {
			b.Run(name + ":curr:size=" + strconv.Itoa(n), func(b *testing.B) {
				benchmarkListIntersectFn(n, f1, f2, IntersectWith, b)})
		}
	}
	runBenchmarks(500)
	runBenchmarks(10000)
	runBenchmarks(1000000)
}

func BenchmarkListIntersectUniDist(b *testing.B) {
	// All data generated is of uniform distribution
	naturalNums := func(i int, sz int) int { return i }
	// two lists match 0 %
	match0Per := func(i int, sz int) int { return i + sz }
	benchmarkListIntersectAll("nomatch:1", 0, match0Per, naturalNums, b)
	benchmarkListIntersectAll("1:nomatch", 0, naturalNums, match0Per, b)
	// two lists match 10 %
	match10Per := func(i int, sz int) int { return 10*i }
	benchmarkListIntersectAll("10:1", 0, match10Per, naturalNums, b)
	benchmarkListIntersectAll("1:10", 0, naturalNums, match10Per, b)
}

func BenchmarkListIntersectNonUniform(b *testing.B) {
	naturalNums := func(i int, sz int) int { return i }
	numsAfterSz := func(i int, sz int) int {
		return sz + i
	}

	type matchNpercfn func(int, int) int

	matchNPercAtStart := func(n int) matchNpercfn {
		return func(i int, sz int) int {
			// for sz : 100, n : 10
			// 0,1,2,3,..9,110,111,...
			if (i < int((sz * n) / 100)) {
				return i
			}
			return sz + i
		}
	}

	// 20 % of list matches at start
	benchmarkListIntersectAll("Same20%atStart:1", 0, matchNPercAtStart(20), naturalNums, b)
	benchmarkListIntersectAll("1:Same20%atStart", 0, naturalNums, matchNPercAtStart(20) b)
	// 50 % of list matches at start
	benchmarkListIntersectAll("Same50%atStart:1", 0, matchNPercAtStart(50), naturalNums, b)
	benchmarkListIntersectAll("1:Same50%atStart", 0, naturalNums, matchNPercAtStart(50), b)

	matchNPercAtEnd := func (n int) matchNpercfn {
		return func (i int, sz int) int {
			// for sz : 100 and n : 10
			// 0,1,2,....88,89,190,191,192..
			if (i >= int(sz * (100 - n) / 100)) {
				return sz + i
			}
			return i
		}
	}

	// 20 % of list matches at end
	benchmarkListIntersectAll("Same20%atEnd:1", 0, matchNPercAtEnd(20), numsAfterSz, b)
	benchmarkListIntersectAll("1:Same20%atEnd", 0, numsAfterSz, matchNPercAtEnd(20), b)
	// 50 % of list matches at end
	benchmarkListIntersectAll("Same50%atEnd:1", 0, matchNPercAtEnd(50), numsAfterSz, b)
	benchmarkListIntersectAll("1:Same50%atEnd", 0, numsAfterSz, matchNPercAtEnd(50), b)

	matchNPercAtMid := func(n int) matchNpercfn {
		nhalf := int(n / 2)
		// for n : 50, sz : 100
		// 0,1,..23,24,125,126,..173,174,276,277..299
		return func(i int, sz int) int {
			szhalf := int(sz / 2)
			if (i < (szhalf - nhalf)) {
				return i
			} else if (i < (szhalf + nhalf)) {
				return sz + i
			} else {
				return (2 * sz) + i
			}
		}
	}

	// 20 % of list matches at mid
	benchmarkListIntersectAll("Same20%atMid:1", 0, matchNPercAtMid(20), numsAfterSz, b)
	benchmarkListIntersectAll("1:Same20%atMid", 0, numsAfterSz, matchNPercAtMid(20), b)
	// 50 % of list matches at Mid
	benchmarkListIntersectAll("Same50%atMid:1", 0, matchNPercAtMid(50), numsAfterSz, b)
	benchmarkListIntersectAll("1:Same50%atMid", 0, numsAfterSz, matchNPercAtMid(50), b)
}
