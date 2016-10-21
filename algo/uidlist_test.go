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

	"github.com/google/flatbuffers/go"
	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/task"
)

func toArray(u *UIDList) []uint64 {
	n := u.Size()
	out := make([]uint64, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, u.Get(i))
	}
	return out
}

func newListFromTask(a []uint64) *UIDList {
	b := flatbuffers.NewBuilder(0)
	task.UidListStartUidsVector(b, len(a))
	for i := len(a) - 1; i >= 0; i-- {
		b.PrependUint64(a[i])
	}
	ve := b.EndVector(len(a))
	task.UidListStart(b)
	task.UidListAddUids(b, ve)
	b.Finish(task.UidListEnd(b))
	data := b.FinishedBytes()

	ulist := task.GetRootAsUidList(data, 0)
	out := new(UIDList)
	out.FromTask(ulist)
	return out
}

func TestEqual(t *testing.T) {
	input1 := newListFromTask([]uint64{5, 8, 13})
	input2 := NewUIDList([]uint64{5, 8, 13})
	require.Equal(t, toArray(input1), toArray(input2))

	input3 := NewUIDList([]uint64{5, 8, 12})
	require.NotEqual(t, toArray(input1), toArray(input3))
}

func TestMergeSorted1(t *testing.T) {
	input := []*UIDList{
		NewUIDList([]uint64{55}),
	}
	require.Equal(t, toArray(MergeLists(input)), []uint64{55})
}

func TestMergeSorted2(t *testing.T) {
	input := []*UIDList{
		NewUIDList([]uint64{1, 3, 6, 8, 10}),
		NewUIDList([]uint64{2, 4, 5, 7, 15}),
	}
	require.Equal(t, toArray(MergeLists(input)),
		[]uint64{1, 2, 3, 4, 5, 6, 7, 8, 10, 15})
}

func TestMergeSorted3(t *testing.T) {
	input := []*UIDList{
		newListFromTask([]uint64{1, 3, 6, 8, 10}),
		NewUIDList([]uint64{}),
	}
	require.Equal(t, toArray(MergeLists(input)), []uint64{1, 3, 6, 8, 10})
}

func TestMergeSorted4(t *testing.T) {
	input := []*UIDList{
		newListFromTask([]uint64{}),
		newListFromTask([]uint64{1, 3, 6, 8, 10}),
	}
	require.Equal(t, toArray(MergeLists(input)), []uint64{1, 3, 6, 8, 10})
}

func TestMergeSorted5(t *testing.T) {
	input := []*UIDList{
		newListFromTask([]uint64{}),
		newListFromTask([]uint64{}),
	}
	require.Empty(t, toArray(MergeLists(input)))
}

func TestMergeSorted6(t *testing.T) {
	input := []*UIDList{
		newListFromTask([]uint64{11, 13, 16, 18, 20}),
		NewUIDList([]uint64{12, 14, 15, 15, 16, 16, 17, 25}),
		NewUIDList([]uint64{1, 2}),
	}
	require.Equal(t, toArray(MergeLists(input)),
		[]uint64{1, 2, 11, 12, 13, 14, 15, 16, 17, 18, 20, 25})
}

func TestMergeSorted7(t *testing.T) {
	input := []*UIDList{
		newListFromTask([]uint64{5, 6, 7}),
		NewUIDList([]uint64{3, 4}),
		newListFromTask([]uint64{1, 2}),
		NewUIDList([]uint64{}),
	}
	require.Equal(t, toArray(MergeLists(input)), []uint64{1, 2, 3, 4, 5, 6, 7})
}

func TestMergeSorted8(t *testing.T) {
	input := []*UIDList{}
	require.Empty(t, toArray(MergeLists(input)))
}

func TestMergeSorted9(t *testing.T) {
	input := []*UIDList{
		newListFromTask([]uint64{1, 1, 1}),
	}
	require.Equal(t, toArray(MergeLists(input)), []uint64{1})
}

func TestMergeSorted10(t *testing.T) {
	input := []*UIDList{
		newListFromTask([]uint64{1, 2, 3, 3, 6}),
		newListFromTask([]uint64{4, 8, 9}),
	}
	require.Equal(t, toArray(MergeLists(input)), []uint64{1, 2, 3, 4, 6, 8, 9})
}

func TestIntersectSorted1(t *testing.T) {
	input := []*UIDList{
		newListFromTask([]uint64{1, 2, 3}),
		NewUIDList([]uint64{2, 3, 4, 5}),
	}
	require.Equal(t, toArray(IntersectLists(input)), []uint64{2, 3})
}

func TestIntersectSorted2(t *testing.T) {
	input := []*UIDList{
		newListFromTask([]uint64{1, 1, 1, 1, 1, 1, 2, 2, 2, 2, 3, 3, 3}),
	}
	require.Equal(t, toArray(IntersectLists(input)), []uint64{1, 2, 3})
}

func TestIntersectSorted3(t *testing.T) {
	input := []*UIDList{}
	require.Empty(t, toArray(IntersectLists(input)))
}

func TestIntersectSorted4(t *testing.T) {
	input := []*UIDList{
		NewUIDList([]uint64{100, 101}),
	}
	require.Equal(t, toArray(IntersectLists(input)), []uint64{100, 101})
}

func TestIntersectSorted5(t *testing.T) {
	input := []*UIDList{
		NewUIDList([]uint64{1, 2, 3}),
		newListFromTask([]uint64{2, 3, 4, 5}),
		NewUIDList([]uint64{4, 5, 6}),
	}
	require.Empty(t, toArray(IntersectLists(input)))
}

func TestUIDListIntersect1(t *testing.T) {
	u := NewUIDList([]uint64{1, 2, 3})
	v := newListFromTask([]uint64{})
	u.Intersect(v)
	require.Empty(t, toArray(u))
}

func TestUIDListIntersect2(t *testing.T) {
	u := NewUIDList([]uint64{1, 2, 3})
	v := newListFromTask([]uint64{1, 2, 3, 4, 5})
	u.Intersect(v)
	require.Equal(t, toArray(u), []uint64{1, 2, 3})
}

func TestUIDListIntersect3(t *testing.T) {
	u := NewUIDList([]uint64{1, 2, 3})
	v := newListFromTask([]uint64{2})
	u.Intersect(v)
	require.Equal(t, toArray(u), []uint64{2})
}

func TestUIDListIntersect4(t *testing.T) {
	u := NewUIDList([]uint64{1, 2, 3})
	v := newListFromTask([]uint64{0, 5})
	u.Intersect(v)
	require.Empty(t, toArray(u))
}

func TestUIDListIntersect5(t *testing.T) {
	u := NewUIDList([]uint64{1, 2, 3})
	v := newListFromTask([]uint64{3, 5})
	u.Intersect(v)
	require.Equal(t, toArray(u), []uint64{3})
}

func TestApplyFilterUint(t *testing.T) {
	u := NewUIDList([]uint64{1, 2, 3, 4, 5})
	u.ApplyFilter(func(a uint64, idx int) bool { return (a % 2) == 1 })
	require.Equal(t, toArray(u), []uint64{1, 3, 5})
}

func TestApplyFilterList(t *testing.T) {
	u := newListFromTask([]uint64{1, 2, 3, 4, 5})
	u.ApplyFilter(func(a uint64, idx int) bool { return (a % 2) == 1 })
	require.Equal(t, toArray(u), []uint64{1, 3, 5})
}

func TestApplyFilterByIdx(t *testing.T) {
	u := newListFromTask([]uint64{1, 2, 3, 4, 5})
	u.ApplyFilter(func(a uint64, idx int) bool { return (idx % 2) == 1 })
	require.Equal(t, toArray(u), []uint64{2, 4})
}
