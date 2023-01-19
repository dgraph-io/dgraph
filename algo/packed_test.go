/*
 * Copyright 2016-2022 Dgraph Labs, Inc. and Contributors
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
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/codec"
	"github.com/dgraph-io/dgraph/protos/pb"
)

func newUidPack(data []uint64) *pb.UidPack {
	// Using a small block size to make sure multiple blocks are used by the tests.
	encoder := codec.Encoder{BlockSize: 5}
	for _, uid := range data {
		encoder.Add(uid)
	}
	return encoder.Done()
}

func TestMergeSorted1Packed(t *testing.T) {
	input := []*pb.UidPack{
		newUidPack([]uint64{55}),
	}
	require.Equal(t, []uint64{55}, codec.Decode(MergeSortedPacked(input), 0))
}

func printPack(t *testing.T, pack *pb.UidPack) {
	for _, block := range pack.Blocks {
		t.Logf("[%x]Block base: %d. Num uids: %d. Deltas: %x\n",
			pack.AllocRef, block.Base, block.NumUids, block.Deltas)
	}
}

func TestMergeSorted2Packed(t *testing.T) {
	input := []*pb.UidPack{
		newUidPack([]uint64{1, 3, 6, 8, 10}),
		newUidPack([]uint64{2, 4, 5, 7, 15}),
	}
	require.Equal(t, []uint64{1, 3, 6, 8, 10}, codec.Decode(input[0], 0))
	require.Equal(t, []uint64{2, 4, 5, 7, 15}, codec.Decode(input[1], 0))

	// printPack(t, input[1])
	merged := MergeSortedPacked(input)
	// printPack(t, merged)
	require.Equal(t, []uint64{1, 2, 3, 4, 5, 6, 7, 8, 10, 15},
		codec.Decode(merged, 0))
}

func TestMergeSorted3Packed(t *testing.T) {
	input := []*pb.UidPack{
		newUidPack([]uint64{1, 3, 6, 8, 10}),
		newUidPack([]uint64{}),
	}
	require.Equal(t, []uint64{1, 3, 6, 8, 10}, codec.Decode(MergeSortedPacked(input), 0))
}

func TestMergeSorted4Packed(t *testing.T) {
	input := []*pb.UidPack{
		newUidPack([]uint64{}),
		newUidPack([]uint64{1, 3, 6, 8, 10}),
	}
	require.Equal(t, []uint64{1, 3, 6, 8, 10}, codec.Decode(MergeSortedPacked(input), 0))
}

func TestMergeSorted5Packed(t *testing.T) {
	input := []*pb.UidPack{
		newUidPack([]uint64{}),
		newUidPack([]uint64{}),
	}
	require.Empty(t, codec.Decode(MergeSortedPacked(input), 0))
}

func TestMergeSorted6Packed(t *testing.T) {
	input := []*pb.UidPack{
		newUidPack([]uint64{11, 13, 16, 18, 20}),
		newUidPack([]uint64{12, 14, 15, 15, 16, 16, 17, 25}),
		newUidPack([]uint64{1, 2}),
	}
	require.Equal(t,
		[]uint64{1, 2, 11, 12, 13, 14, 15, 16, 17, 18, 20, 25},
		codec.Decode(MergeSortedPacked(input), 0))
}

func TestMergeSorted7Packed(t *testing.T) {
	input := []*pb.UidPack{
		newUidPack([]uint64{5, 6, 7}),
		newUidPack([]uint64{3, 4}),
		newUidPack([]uint64{1, 2}),
		newUidPack([]uint64{}),
	}
	require.Equal(t, []uint64{1, 2, 3, 4, 5, 6, 7}, codec.Decode(MergeSortedPacked(input), 0))
}

func TestMergeSorted8Packed(t *testing.T) {
	input := []*pb.UidPack{}
	require.Empty(t, codec.Decode(MergeSortedPacked(input), 0))
}

func TestMergeSorted9Packed(t *testing.T) {
	input := []*pb.UidPack{
		newUidPack([]uint64{1, 1, 1}),
	}
	require.Equal(t, []uint64{1}, codec.Decode(MergeSortedPacked(input), 0))
}

func TestMergeSorted10Packed(t *testing.T) {
	input := []*pb.UidPack{
		newUidPack([]uint64{1, 2, 3, 3, 6}),
		newUidPack([]uint64{4, 8, 9}),
	}
	require.Equal(t, []uint64{1, 2, 3, 4, 6, 8, 9}, codec.Decode(MergeSortedPacked(input), 0))
}

func TestUIDListIntersect1Packed(t *testing.T) {
	u := newUidPack([]uint64{1, 2, 3})
	v := newUidPack([]uint64{})
	o := IntersectWithLinPacked(u, v)
	require.Empty(t, codec.Decode(o, 0))
}

func TestUIDListIntersect2Packed(t *testing.T) {
	u := newUidPack([]uint64{1, 2, 3})
	v := newUidPack([]uint64{1, 2, 3, 4, 5})
	o := IntersectWithLinPacked(u, v)
	require.Equal(t, []uint64{1, 2, 3}, codec.Decode(o, 0))
}

func TestUIDListIntersect3Packed(t *testing.T) {
	u := newUidPack([]uint64{1, 2, 3})
	v := newUidPack([]uint64{2})
	o := IntersectWithLinPacked(u, v)
	require.Equal(t, []uint64{2}, codec.Decode(o, 0))
}

func TestUIDListIntersect4Packed(t *testing.T) {
	u := newUidPack([]uint64{1, 2, 3})
	v := newUidPack([]uint64{0, 5})
	o := IntersectWithLinPacked(u, v)
	require.Empty(t, codec.Decode(o, 0))
}

func TestUIDListIntersect5Packed(t *testing.T) {
	u := newUidPack([]uint64{1, 2, 3})
	v := newUidPack([]uint64{3, 5})
	o := IntersectWithLinPacked(u, v)
	require.Equal(t, []uint64{3}, codec.Decode(o, 0))
}

func TestUIDListIntersect6Packed(t *testing.T) {
	u := newUidPack([]uint64{1, 2, 3, 4, 5, 6, 7, 9})
	v := newUidPack([]uint64{1, 3, 5, 7, 8, 9})
	o := IntersectWithLinPacked(u, v)
	require.Equal(t, []uint64{1, 3, 5, 7, 9}, codec.Decode(o, 0))
}

func TestUIDListIntersectDupFirstPacked(t *testing.T) {
	u := newUidPack([]uint64{1, 1, 2, 3})
	v := newUidPack([]uint64{1, 2})
	o := IntersectWithLinPacked(u, v)
	require.Equal(t, []uint64{1, 2}, codec.Decode(o, 0))
}

func TestUIDListIntersectDupBothPacked(t *testing.T) {
	u := newUidPack([]uint64{1, 1, 2, 3, 5})
	v := newUidPack([]uint64{1, 1, 2, 4})
	o := IntersectWithLinPacked(u, v)
	require.Equal(t, []uint64{1, 1, 2}, codec.Decode(o, 0))
}

func TestUIDListIntersectDupSecondPacked(t *testing.T) {
	u := newUidPack([]uint64{1, 2, 3, 5})
	v := newUidPack([]uint64{1, 1, 2, 4})
	o := IntersectWithLinPacked(u, v)
	require.Equal(t, []uint64{1, 2}, codec.Decode(o, 0))
}

func TestIntersectSorted1Packed(t *testing.T) {
	input := []*pb.UidPack{
		newUidPack([]uint64{1, 2, 3}),
		newUidPack([]uint64{2, 3, 4, 5}),
	}
	require.Equal(t, []uint64{2, 3}, codec.Decode(IntersectSortedPacked(input), 0))
}

func TestIntersectSorted2Packed(t *testing.T) {
	input := []*pb.UidPack{
		newUidPack([]uint64{1, 2, 3}),
	}
	require.Equal(t, []uint64{1, 2, 3}, codec.Decode(IntersectSortedPacked(input), 0))
}

func TestIntersectSorted3Packed(t *testing.T) {
	input := []*pb.UidPack{}
	require.Empty(t, codec.Decode(IntersectSortedPacked(input), 0))
}

func TestIntersectSorted4Packed(t *testing.T) {
	input := []*pb.UidPack{
		newUidPack([]uint64{100, 101}),
	}
	require.Equal(t, []uint64{100, 101}, codec.Decode(IntersectSortedPacked(input), 0))
}

func TestIntersectSorted5Packed(t *testing.T) {
	input := []*pb.UidPack{
		newUidPack([]uint64{1, 2, 3}),
		newUidPack([]uint64{2, 3, 4, 5}),
		newUidPack([]uint64{4, 5, 6}),
	}
	require.Empty(t, codec.Decode(IntersectSortedPacked(input), 0))
}

func TestIntersectSorted6Packed(t *testing.T) {
	input := []*pb.UidPack{
		newUidPack([]uint64{10, 12, 13}),
		newUidPack([]uint64{2, 3, 4, 13}),
		newUidPack([]uint64{4, 5, 6}),
	}
	require.Empty(t, codec.Decode(IntersectSortedPacked(input), 0))
}

func TestIntersectSorted7Packed(t *testing.T) {
	input := []*pb.UidPack{
		newUidPack([]uint64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11}),
		newUidPack([]uint64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}),
		newUidPack([]uint64{1, 2, 3, 4, 5, 6, 7, 8, 9}),
		newUidPack([]uint64{1, 2, 3, 4, 5, 6, 7, 8}),
		newUidPack([]uint64{1, 2, 3, 4, 5, 6, 7}),
		newUidPack([]uint64{1, 2, 3, 4, 5, 6}),
		newUidPack([]uint64{1, 2, 3, 4, 5}),
		newUidPack([]uint64{1, 2, 3, 4}),
		newUidPack([]uint64{1, 2, 3}),
		newUidPack([]uint64{1, 2}),
		newUidPack([]uint64{1}),
	}
	require.Equal(t, []uint64{1}, codec.Decode(IntersectSortedPacked(input), 0))
}

func TestDiffSorted1Packed(t *testing.T) {
	input := []*pb.UidPack{
		newUidPack([]uint64{1, 2, 3}),
		newUidPack([]uint64{1}),
	}
	output := DifferencePacked(input[0], input[1])
	require.Equal(t, []uint64{2, 3}, codec.Decode(output, 0))
}

func TestDiffSorted2Packed(t *testing.T) {
	input := []*pb.UidPack{
		newUidPack([]uint64{1, 2, 3}),
		newUidPack([]uint64{2}),
	}
	output := DifferencePacked(input[0], input[1])
	require.Equal(t, []uint64{1, 3}, codec.Decode(output, 0))
}

func TestDiffSorted3Packed(t *testing.T) {
	input := []*pb.UidPack{
		newUidPack([]uint64{1, 2, 3}),
		newUidPack([]uint64{3}),
	}
	output := DifferencePacked(input[0], input[1])
	require.Equal(t, []uint64{1, 2}, codec.Decode(output, 0))
}

func TestDiffSorted4Packed(t *testing.T) {
	input := []*pb.UidPack{
		newUidPack([]uint64{1, 2, 3}),
		newUidPack([]uint64{}),
	}
	output := DifferencePacked(input[0], input[1])
	require.Equal(t, []uint64{1, 2, 3}, codec.Decode(output, 0))
}

func TestDiffSorted5Packed(t *testing.T) {
	input := []*pb.UidPack{
		newUidPack([]uint64{}),
		newUidPack([]uint64{1, 2}),
	}
	output := DifferencePacked(input[0], input[1])
	require.Equal(t, []uint64{}, codec.Decode(output, 0))
}

func TestSubSorted1Packed(t *testing.T) {
	input := []*pb.UidPack{
		newUidPack([]uint64{1, 2, 3}),
		newUidPack([]uint64{2, 3, 4, 5}),
	}
	output := DifferencePacked(input[0], input[1])
	require.Equal(t, []uint64{1}, codec.Decode(output, 0))
}

func TestSubSorted6Packed(t *testing.T) {
	input := []*pb.UidPack{
		newUidPack([]uint64{10, 12, 13}),
		newUidPack([]uint64{2, 3, 4, 13}),
	}
	output := DifferencePacked(input[0], input[1])
	require.Equal(t, []uint64{10, 12}, codec.Decode(output, 0))
}

func TestIndexOfPacked1(t *testing.T) {
	encoder := codec.Encoder{BlockSize: 10}
	for i := 0; i < 1000; i++ {
		encoder.Add(uint64(i))
	}
	pack := encoder.Done()

	for i := 0; i < 1000; i++ {
		require.Equal(t, i, IndexOfPacked(pack, uint64(i)))
	}
	require.Equal(t, -1, IndexOfPacked(pack, 1000))
}

func TestIndexOfPacked2(t *testing.T) {
	encoder := codec.Encoder{BlockSize: 10}
	for i := 0; i < 100; i++ {
		encoder.Add(uint64(i))
	}
	pack := encoder.Done()

	require.Equal(t, -1, IndexOfPacked(pack, 100))
	require.Equal(t, -1, IndexOfPacked(pack, 101))
	require.Equal(t, -1, IndexOfPacked(pack, 1000))
	require.Equal(t, -1, IndexOfPacked(pack, math.MaxUint64))
}

func TestIndexOfPacked3(t *testing.T) {
	require.Equal(t, -1, IndexOfPacked(nil, 0))
	require.Equal(t, -1, IndexOfPacked(nil, math.MaxUint64))
}

func TestApplyFilterUintPacked(t *testing.T) {
	l := []uint64{1, 2, 3, 4, 5, 6, 7, 8, 9}
	u := newUidPack(l)
	res := ApplyFilterPacked(u, func(a uint64, idx int) bool { return (l[idx] % 2) == 1 })
	require.Equal(t, []uint64{1, 3, 5, 7, 9}, codec.Decode(res, 0))
}
