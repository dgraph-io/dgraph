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
	"fmt"
	"math/rand"
	"sort"
	"testing"
	"time"

	"github.com/dgraph-io/dgraph/codec"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/stretchr/testify/require"
)

func newList(data []uint64) *pb.List {
	return &pb.List{Uids: data}
}

func TestMergeSorted1(t *testing.T) {
	input := []*pb.List{
		newList([]uint64{55}),
	}
	require.Equal(t, MergeSorted(input).Uids, []uint64{55})
}

func TestMergeSorted2(t *testing.T) {
	input := []*pb.List{
		newList([]uint64{1, 3, 6, 8, 10}),
		newList([]uint64{2, 4, 5, 7, 15}),
	}
	require.Equal(t, MergeSorted(input).Uids,
		[]uint64{1, 2, 3, 4, 5, 6, 7, 8, 10, 15})
}

func TestMergeSorted3(t *testing.T) {
	input := []*pb.List{
		newList([]uint64{1, 3, 6, 8, 10}),
		newList([]uint64{}),
	}
	require.Equal(t, MergeSorted(input).Uids, []uint64{1, 3, 6, 8, 10})
}

func TestMergeSorted4(t *testing.T) {
	input := []*pb.List{
		newList([]uint64{}),
		newList([]uint64{1, 3, 6, 8, 10}),
	}
	require.Equal(t, MergeSorted(input).Uids, []uint64{1, 3, 6, 8, 10})
}

func TestMergeSorted5(t *testing.T) {
	input := []*pb.List{
		newList([]uint64{}),
		newList([]uint64{}),
	}
	require.Empty(t, MergeSorted(input).Uids)
}

func TestMergeSorted6(t *testing.T) {
	input := []*pb.List{
		newList([]uint64{11, 13, 16, 18, 20}),
		newList([]uint64{12, 14, 15, 15, 16, 16, 17, 25}),
		newList([]uint64{1, 2}),
	}
	require.Equal(t, MergeSorted(input).Uids,
		[]uint64{1, 2, 11, 12, 13, 14, 15, 16, 17, 18, 20, 25})
}

func TestMergeSorted7(t *testing.T) {
	input := []*pb.List{
		newList([]uint64{5, 6, 7}),
		newList([]uint64{3, 4}),
		newList([]uint64{1, 2}),
		newList([]uint64{}),
	}
	require.Equal(t, MergeSorted(input).Uids, []uint64{1, 2, 3, 4, 5, 6, 7})
}

func TestMergeSorted8(t *testing.T) {
	input := []*pb.List{}
	require.Empty(t, MergeSorted(input).Uids)
}

func TestMergeSorted9(t *testing.T) {
	input := []*pb.List{
		newList([]uint64{1, 1, 1}),
	}
	require.Equal(t, MergeSorted(input).Uids, []uint64{1})
}

func TestMergeSorted10(t *testing.T) {
	input := []*pb.List{
		newList([]uint64{1, 2, 3, 3, 6}),
		newList([]uint64{4, 8, 9}),
	}
	require.Equal(t, MergeSorted(input).Uids, []uint64{1, 2, 3, 4, 6, 8, 9})
}

func TestIntersectSorted1(t *testing.T) {
	input := []*pb.List{
		newList([]uint64{1, 2, 3}),
		newList([]uint64{2, 3, 4, 5}),
	}
	require.Equal(t, []uint64{2, 3}, IntersectSorted(input).Uids)
}

func TestIntersectSorted2(t *testing.T) {
	input := []*pb.List{
		newList([]uint64{1, 2, 3}),
	}
	require.Equal(t, IntersectSorted(input).Uids, []uint64{1, 2, 3})
}

func TestIntersectSorted3(t *testing.T) {
	input := []*pb.List{}
	require.Empty(t, IntersectSorted(input).Uids)
}

func TestIntersectSorted4(t *testing.T) {
	input := []*pb.List{
		newList([]uint64{100, 101}),
	}
	require.Equal(t, IntersectSorted(input).Uids, []uint64{100, 101})
}

func TestIntersectSorted5(t *testing.T) {
	input := []*pb.List{
		newList([]uint64{1, 2, 3}),
		newList([]uint64{2, 3, 4, 5}),
		newList([]uint64{4, 5, 6}),
	}
	require.Empty(t, IntersectSorted(input).Uids)
}

func TestIntersectSorted6(t *testing.T) {
	input := []*pb.List{
		newList([]uint64{10, 12, 13}),
		newList([]uint64{2, 3, 4, 13}),
		newList([]uint64{4, 5, 6}),
	}
	require.Empty(t, IntersectSorted(input).Uids)
}

func TestDiffSorted1(t *testing.T) {
	input := []*pb.List{
		newList([]uint64{1, 2, 3}),
		newList([]uint64{1}),
	}
	output := Difference(input[0], input[1])
	require.Equal(t, []uint64{2, 3}, output.Uids)
}

func TestDiffSorted2(t *testing.T) {
	input := []*pb.List{
		newList([]uint64{1, 2, 3}),
		newList([]uint64{2}),
	}
	output := Difference(input[0], input[1])
	require.Equal(t, []uint64{1, 3}, output.Uids)
}

func TestDiffSorted3(t *testing.T) {
	input := []*pb.List{
		newList([]uint64{1, 2, 3}),
		newList([]uint64{3}),
	}
	output := Difference(input[0], input[1])
	require.Equal(t, []uint64{1, 2}, output.Uids)
}

func TestDiffSorted4(t *testing.T) {
	input := []*pb.List{
		newList([]uint64{1, 2, 3}),
		newList([]uint64{}),
	}
	output := Difference(input[0], input[1])
	require.Equal(t, []uint64{1, 2, 3}, output.Uids)
}

func TestDiffSorted5(t *testing.T) {
	input := []*pb.List{
		newList([]uint64{}),
		newList([]uint64{1, 2}),
	}
	output := Difference(input[0], input[1])
	require.Equal(t, []uint64{}, output.Uids)
}

func TestSubSorted1(t *testing.T) {
	input := []*pb.List{
		newList([]uint64{1, 2, 3}),
		newList([]uint64{2, 3, 4, 5}),
	}
	output := Difference(input[0], input[1])
	require.Equal(t, []uint64{1}, output.Uids)
}

func TestSubSorted6(t *testing.T) {
	input := []*pb.List{
		newList([]uint64{10, 12, 13}),
		newList([]uint64{2, 3, 4, 13}),
	}
	output := Difference(input[0], input[1])
	require.Equal(t, []uint64{10, 12}, output.Uids)
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
func BenchmarkListIntersectRandom(b *testing.B) {
	randomTests := func(arrSz int, overlap float64) {
		limit := int64(float64(arrSz) / overlap)
		u1, v1 := make([]uint64, arrSz), make([]uint64, arrSz)
		for i := 0; i < arrSz; i++ {
			u1[i] = uint64(rand.Int63n(limit))
			v1[i] = uint64(rand.Int63n(limit))
		}
		sort.Slice(u1, func(i, j int) bool { return u1[i] < u1[j] })
		sort.Slice(v1, func(i, j int) bool { return v1[i] < v1[j] })

		u := newList(u1)
		v := newList(v1)
		dst1 := &pb.List{}
		dst2 := &pb.List{}
		compressedUids := codec.Encode(u1)

		b.Run(fmt.Sprintf(":size=%d:overlap=%.2f:", arrSz, overlap),
			func(b *testing.B) {
				for k := 0; k < b.N; k++ {
					IntersectWith(u, v, dst1)
				}
			})

		b.Run(fmt.Sprintf(":compressed:size=%d:overlap=%.2f:", arrSz, overlap),
			func(b *testing.B) {
				for k := 0; k < b.N; k++ {
					IntersectCompressedWith(compressedUids, 0, v, dst2)
				}
			})
		i := 0
		j := 0
		for i < len(dst1.Uids) {
			if dst1.Uids[i] != dst2.Uids[j] {
				b.Errorf("Unexpected error in intersection")
			}
			// Behaviour of bin intersect is not defined when duplicates are present
			i = skipDuplicate(dst1.Uids, i)
			j = skipDuplicate(dst2.Uids, j)
		}
		if j < len(dst2.Uids) {
			b.Errorf("Unexpected error in intersection")
		}
	}

	randomTests(10240, 0.3)
	randomTests(1024000, 0.3)
	randomTests(10240, 0.1)
	randomTests(1024000, 0.1)
	randomTests(10240, 0.01)
	randomTests(1024000, 0.01)
}

func BenchmarkListIntersectRatio(b *testing.B) {
	randomTests := func(sz int, overlap float64) {
		rs := []int{1, 10, 50, 100, 500, 1000, 10000, 100000, 1000000}
		for _, r := range rs {
			sz1 := sz
			sz2 := sz * r
			if sz2 > 1000000 {
				break
			}

			u1, v1 := make([]uint64, sz1), make([]uint64, sz2)
			limit := int64(float64(sz) / overlap)
			for i := 0; i < sz1; i++ {
				u1[i] = uint64(rand.Int63n(limit))
			}
			for i := 0; i < sz2; i++ {
				v1[i] = uint64(rand.Int63n(limit))
			}
			sort.Slice(u1, func(i, j int) bool { return u1[i] < u1[j] })
			sort.Slice(v1, func(i, j int) bool { return v1[i] < v1[j] })

			u := &pb.List{Uids: u1}
			v := &pb.List{Uids: v1}
			dst1 := &pb.List{}
			dst2 := &pb.List{}
			compressedUids := codec.Encode(v1)

			fmt.Printf("len: %d, compressed: %d, bytes/int: %f\n",
				len(v1), compressedUids.Size(), float64(compressedUids.Size())/float64(len(v1)))
			b.Run(fmt.Sprintf(":IntersectWith:ratio=%d:size=%d:overlap=%.2f:", r, sz, overlap),
				func(b *testing.B) {
					for k := 0; k < b.N; k++ {
						IntersectWith(u, v, dst1)
					}
				})
			b.Run(fmt.Sprintf("compressed:IntersectWith:ratio=%d:size=%d:overlap=%.2f:", r, sz, overlap),
				func(b *testing.B) {
					for k := 0; k < b.N; k++ {
						IntersectCompressedWith(compressedUids, 0, u, dst2)
					}
				})
			fmt.Println()
			i := 0
			j := 0
			for i < len(dst1.Uids) {
				if dst1.Uids[i] != dst2.Uids[j] {
					b.Errorf("Unexpected error in intersection")
				}
				// Behaviour of bin intersect is not defined when duplicates are present
				i = skipDuplicate(dst1.Uids, i)
				j = skipDuplicate(dst2.Uids, j)
			}
			if j < len(dst2.Uids) {
				b.Errorf("Unexpected error in intersection")
			}
		}
	}

	randomTests(10, 0.01)
	randomTests(100, 0.01)
	randomTests(1000, 0.01)
	randomTests(10000, 0.01)
	randomTests(100000, 0.01)
	randomTests(1000000, 0.01)
}

func skipDuplicate(in []uint64, idx int) int {
	i := idx + 1
	for i < len(in) && in[i] == in[idx] {
		i++
	}
	return i
}

func sortUint64(nums []uint64) {
	sort.Slice(nums, func(i, j int) bool { return nums[i] < nums[j] })
}

func fillNums(N1, N2 int) ([]uint64, []uint64, []uint64) {
	rand.Seed(time.Now().UnixNano())

	commonNums := make([]uint64, N1)
	blockNums := make([]uint64, N1+N2)
	otherNums := make([]uint64, N1+N2)

	for i := 0; i < N1; i++ {
		val := rand.Uint64()
		commonNums[i] = val
		blockNums[i] = val
		otherNums[i] = val
	}

	for i := N1; i < N1+N2; i++ {
		blockNums[i] = rand.Uint64()
		otherNums[i] = rand.Uint64()
	}

	sortUint64(commonNums)
	sortUint64(blockNums)
	sortUint64(otherNums)

	return commonNums, blockNums, otherNums
}
