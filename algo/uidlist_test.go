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

func TestApplyFilterUint(t *testing.T) {
	u := newList([]uint64{1, 2, 3, 4, 5})
	ApplyFilter(u, func(a uint64, idx int) bool { return (a % 2) == 1 })
	require.Equal(t, u.Uids, []uint64{1, 3, 5})
}
