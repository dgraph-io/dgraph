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

	"github.com/dgraph-io/dgraph/task"
	"github.com/stretchr/testify/require"
)

func newList(data []uint64) *task.List {
	return SortedListToBlock(data)
}

/*
func TestMergeSorted1(t *testing.T) {
	input := []*task.List{
		newList([]uint64{55}),
	}
	require.Equal(t, BlockToList(MergeSorted(input)), []uint64{55})
}

func TestMergeSorted2(t *testing.T) {
	input := []*task.List{
		newList([]uint64{1, 3, 6, 8, 10}),
		newList([]uint64{2, 4, 5, 7, 15}),
	}
	require.Equal(t, BlockToList(MergeSorted(input)),
		[]uint64{1, 2, 3, 4, 5, 6, 7, 8, 10, 15})
}

func TestMergeSorted3(t *testing.T) {
	input := []*task.List{
		newList([]uint64{1, 3, 6, 8, 10}),
		newList([]uint64{}),
	}
	require.Equal(t, BlockToList(MergeSorted(input)), []uint64{1, 3, 6, 8, 10})
}

func TestMergeSorted4(t *testing.T) {
	input := []*task.List{
		newList([]uint64{}),
		newList([]uint64{1, 3, 6, 8, 10}),
	}
	require.Equal(t, BlockToList(MergeSorted(input)), []uint64{1, 3, 6, 8, 10})
}

func TestMergeSorted5(t *testing.T) {
	input := []*task.List{
		newList([]uint64{}),
		newList([]uint64{}),
	}
	require.Empty(t, BlockToList(MergeSorted(input)))
}

func TestMergeSorted6(t *testing.T) {
	input := []*task.List{
		newList([]uint64{11, 13, 16, 18, 20}),
		newList([]uint64{12, 14, 15, 15, 16, 16, 17, 25}),
		newList([]uint64{1, 2}),
	}
	require.Equal(t, BlockToList(MergeSorted(input)),
		[]uint64{1, 2, 11, 12, 13, 14, 15, 16, 17, 18, 20, 25})
}

func TestMergeSorted7(t *testing.T) {
	input := []*task.List{
		newList([]uint64{5, 6, 7}),
		newList([]uint64{3, 4}),
		newList([]uint64{1, 2}),
		newList([]uint64{}),
	}
	require.Equal(t, BlockToList(MergeSorted(input)), []uint64{1, 2, 3, 4, 5, 6, 7})
}

func TestMergeSorted8(t *testing.T) {
	input := []*task.List{}
	require.Empty(t, BlockToList(MergeSorted(input)))
}

func TestMergeSorted9(t *testing.T) {
	input := []*task.List{
		newList([]uint64{1, 1, 1}),
	}
	require.Equal(t, BlockToList(MergeSorted(input)), []uint64{1})
}

func TestMergeSorted10(t *testing.T) {
	input := []*task.List{
		newList([]uint64{1, 2, 3, 3, 6}),
		newList([]uint64{4, 8, 9}),
	}
	require.Equal(t, BlockToList(MergeSorted(input)), []uint64{1, 2, 3, 4, 6, 8, 9})
}
*/
func TestIntersectSorted1(t *testing.T) {
	input := []*task.List{
		newList([]uint64{1, 2, 3}),
		newList([]uint64{2, 3, 4, 5}),
	}
	require.Equal(t, BlockToList(IntersectSorted(input)), []uint64{2, 3})
}

func TestIntersectSorted2(t *testing.T) {
	input := []*task.List{
		newList([]uint64{1, 2, 3}),
	}
	require.Equal(t, BlockToList(IntersectSorted(input)), []uint64{1, 2, 3})
}

func TestIntersectSorted3(t *testing.T) {
	input := []*task.List{}
	require.Empty(t, BlockToList(IntersectSorted(input)))
}

func TestIntersectSorted4(t *testing.T) {
	input := []*task.List{
		newList([]uint64{100, 101}),
	}
	require.Equal(t, BlockToList(IntersectSorted(input)), []uint64{100, 101})
}

func TestIntersectSorted5(t *testing.T) {
	input := []*task.List{
		newList([]uint64{1, 2, 3}),
		newList([]uint64{2, 3, 4, 5}),
		newList([]uint64{4, 5, 6}),
	}
	require.Empty(t, BlockToList(IntersectSorted(input)))
}

func TestIntersectSorted6(t *testing.T) {
	input := []*task.List{
		newList([]uint64{10, 12, 13}),
		newList([]uint64{2, 3, 4, 13}),
		newList([]uint64{4, 5, 6}),
	}
	require.Empty(t, BlockToList(IntersectSorted(input)))
}

/*
func TestSubSorted1(t *testing.T) {
	input := []*task.List{
		newList([]uint64{1, 2, 3}),
		newList([]uint64{2, 3, 4, 5}),
	}
	Difference(input[0], input[1])
	require.Equal(t, []uint64{1}, BlockToList(input[0]))
}

func TestSubSorted6(t *testing.T) {
	input := []*task.List{
		newList([]uint64{10, 12, 13}),
		newList([]uint64{2, 3, 4, 13}),
	}
	Difference(input[0], input[1])
	require.Equal(t, []uint64{10, 12}, BlockToList(input[0]))
}

func TestIterator1(t *testing.T) {
	u := newList([]uint64{1, 2, 3, 4, 43, 234, 2344})

	it := NewListIterator(u)
	var res []uint64
	for ; it.Valid(); it.Next() {
		res = append(res, it.Val())
	}

	require.Equal(t, []uint64{1, 2, 3, 4, 43, 234, 2344}, res)
}

func TestIterator2(t *testing.T) {
	var ls, res []uint64
	for i := 0; i < 1000; i++ {
		ls = append(ls, uint64(i*2))
	}
	u := newList(ls)
	it := NewListIterator(u)
	for ; it.Valid(); it.Next() {
		res = append(res, it.Val())
	}
	require.Equal(t, ls, res)
}
*/
func TestUIDListIntersect1(t *testing.T) {
	u := newList([]uint64{1, 2, 3})
	v := newList([]uint64{})
	IntersectWith(u, v)

	require.Empty(t, BlockToList(u))
}

func TestUIDListIntersect2(t *testing.T) {
	u := newList([]uint64{1, 2, 3})
	v := newList([]uint64{1, 2, 3, 4, 5})
	IntersectWith(u, v)
	require.Equal(t, []uint64{1, 2, 3}, BlockToList(u))
}

func TestUIDListIntersect3(t *testing.T) {
	u := newList([]uint64{1, 2, 3})
	v := newList([]uint64{2})
	IntersectWith(u, v)
	require.Equal(t, []uint64{2}, BlockToList(u))
}

func TestUIDListIntersect4(t *testing.T) {
	u := newList([]uint64{1, 2, 3})
	v := newList([]uint64{0, 5})
	IntersectWith(u, v)
	require.Empty(t, BlockToList(u))
}

func TestUIDListIntersect5(t *testing.T) {
	u := newList([]uint64{1, 2, 3})
	v := newList([]uint64{3, 5})
	IntersectWith(u, v)
	require.Equal(t, []uint64{3}, BlockToList(u))
}

func TestUIDListIntersectDupFirst(t *testing.T) {
	u := newList([]uint64{1, 1, 2, 3})
	v := newList([]uint64{1, 2})
	IntersectWith(u, v)
	require.Equal(t, []uint64{1, 2}, BlockToList(u))
}

func TestUIDListIntersectDupBoth(t *testing.T) {
	u := newList([]uint64{1, 1, 2, 3, 5})
	v := newList([]uint64{1, 1, 2, 4})
	IntersectWith(u, v)
	require.Equal(t, []uint64{1, 1, 2}, BlockToList(u))
}

func TestUIDListIntersectDupSecond(t *testing.T) {
	u := newList([]uint64{1, 2, 3, 5})
	v := newList([]uint64{1, 1, 2, 4})
	IntersectWith(u, v)
	require.Equal(t, []uint64{1, 2}, BlockToList(u))
}

func TestApplyFilterUint(t *testing.T) {
	l := []uint64{1, 2, 3, 4, 5}
	u := newList(l)
	ApplyFilter(u, func(a uint64, idx int) bool { return (l[idx] % 2) == 1 })
	require.Equal(t, []uint64{1, 3, 5}, BlockToList(u))
}

func TestWriteAppend1(t *testing.T) {
	l := []uint64{1, 2, 3}
	u := newList(l)
	w := NewWriteIterator(u, 1)
	w.Append(4)
	w.Append(5)
	w.Append(6)
	w.End()
	require.Equal(t, []uint64{1, 2, 3, 4, 5, 6}, BlockToList(u))
}

func TestWriteAppend2(t *testing.T) {
	l := []uint64{1, 2, 3}
	u := newList(l)
	w := NewWriteIterator(u, 0)
	w.Append(4)
	w.Append(5)
	w.Append(6)
	w.End()
	require.Equal(t, []uint64{4, 5, 6}, BlockToList(u))
}

// sort interface for []uint64
type uint64Slice []uint64

func (xs uint64Slice) Len() int {
	return len(xs)
}
func (xs uint64Slice) Less(i, j int) bool {
	return xs[i] < xs[j]
}
func (xs uint64Slice) Swap(i, j int) {
	xs[i], xs[j] = xs[j], xs[i]
}

/*
// Benchmarks for IntersectWith
// random data : u and v having data within range [0, limit)
// where limit = N * sizeof-list ; for different N
func runIntersectRandom(arrSz int, limit int64, b *testing.B) {
	u1, v1 := make([]uint64, arrSz, arrSz), make([]uint64, arrSz, arrSz)
	for i := 0; i < arrSz; i++ {
		u1[i] = uint64(rand.Int63n(limit))
		v1[i] = uint64(rand.Int63n(limit))
	}
	sort.Sort(uint64Slice(u1))
	sort.Sort(uint64Slice(v1))

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
*/
