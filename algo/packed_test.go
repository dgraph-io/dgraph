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
	"github.com/stretchr/testify/require"
	"testing"

	"github.com/dgraph-io/dgraph/codec"
	"github.com/dgraph-io/dgraph/protos/pb"
)

func newUidPack(data []uint64) *pb.UidPack {
	encoder := codec.Encoder{BlockSize: 10}
	for _, uid := range data {
		encoder.Add(uid)
	}
	return encoder.Done()
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
		newUidPack([]uint64{1, 2, 3}),
		newUidPack([]uint64{2, 3, 4, 5}),
	}
	require.Equal(t, []uint64{2, 3}, codec.Decode(IntersectSortedPacked(input), 0))
}
