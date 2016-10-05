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

	"github.com/dgraph-io/dgraph/task"
	"github.com/dgraph-io/dgraph/x"
)

func listEqual(u, v *UIDList, t *testing.T) {
	if u.Size() != v.Size() {
		t.Errorf("Size mismatch %d vs %d", u.Size(), v.Size())
		t.Fatal()
	}
	for i := 0; i < u.Size(); i++ {
		if u.Get(i) != v.Get(i) {
			t.Errorf("Element mismatch at index %d: %d vs %d", i, u.Get(i), v.Get(i))
			t.Fatal()
		}
	}
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

	ulist := new(task.UidList)
	x.ParseUidList(ulist, data)
	out := new(UIDList)
	out.FromTask(ulist)
	return out
}

func TestEqual(t *testing.T) {
	input1 := newListFromTask([]uint64{5, 8, 13})
	input2 := NewUIDList([]uint64{5, 8, 13})
	listEqual(input1, input2, t)
}

func TestMergeSorted1(t *testing.T) {
	input := []*UIDList{
		NewUIDList([]uint64{55}),
	}
	expected := NewUIDList([]uint64{55})
	listEqual(MergeLists(input), expected, t)
}

func TestMergeSorted2(t *testing.T) {
	input := []*UIDList{
		NewUIDList([]uint64{1, 3, 6, 8, 10}),
		NewUIDList([]uint64{2, 4, 5, 7, 15}),
	}
	expected := NewUIDList([]uint64{1, 2, 3, 4, 5, 6, 7, 8, 10, 15})
	listEqual(MergeLists(input), expected, t)
}

func TestMergeSorted3(t *testing.T) {
	input := []*UIDList{
		newListFromTask([]uint64{1, 3, 6, 8, 10}),
		NewUIDList([]uint64{}),
	}
	expected := NewUIDList([]uint64{1, 3, 6, 8, 10})
	listEqual(MergeLists(input), expected, t)
}

func TestMergeSorted4(t *testing.T) {
	input := []*UIDList{
		newListFromTask([]uint64{}),
		newListFromTask([]uint64{1, 3, 6, 8, 10}),
	}
	expected := NewUIDList([]uint64{1, 3, 6, 8, 10})
	listEqual(MergeLists(input), expected, t)
}

func TestMergeSorted5(t *testing.T) {
	input := []*UIDList{
		newListFromTask([]uint64{}),
		newListFromTask([]uint64{}),
	}
	expected := NewUIDList([]uint64{})
	listEqual(MergeLists(input), expected, t)
}

func TestMergeSorted6(t *testing.T) {
	input := []*UIDList{
		newListFromTask([]uint64{11, 13, 16, 18, 20}),
		NewUIDList([]uint64{12, 14, 15, 15, 16, 16, 17, 25}),
		NewUIDList([]uint64{1, 2}),
	}
	expected := NewUIDList([]uint64{1, 2, 11, 12, 13, 14, 15, 16, 17, 18, 20, 25})
	listEqual(MergeLists(input), expected, t)
}

func TestMergeSorted7(t *testing.T) {
	input := []*UIDList{
		newListFromTask([]uint64{5, 6, 7}),
		NewUIDList([]uint64{3, 4}),
		newListFromTask([]uint64{1, 2}),
		NewUIDList([]uint64{}),
	}
	expected := NewUIDList([]uint64{1, 2, 3, 4, 5, 6, 7})
	listEqual(MergeLists(input), expected, t)
}

func TestMergeSorted8(t *testing.T) {
	input := []*UIDList{}
	expected := NewUIDList([]uint64{})
	listEqual(MergeLists(input), expected, t)
}

func TestMergeSorted9(t *testing.T) {
	input := []*UIDList{
		newListFromTask([]uint64{1, 1, 1}),
	}
	expected := NewUIDList([]uint64{1})
	listEqual(MergeLists(input), expected, t)
}

func TestMergeSorted10(t *testing.T) {
	input := []*UIDList{
		newListFromTask([]uint64{1, 2, 3, 3, 6}),
		newListFromTask([]uint64{4, 8, 9}),
	}
	expected := NewUIDList([]uint64{1, 2, 3, 4, 6, 8, 9})
	listEqual(MergeLists(input), expected, t)
}

func TestIntersectSorted1(t *testing.T) {
	input := []*UIDList{
		newListFromTask([]uint64{1, 2, 3}),
		NewUIDList([]uint64{2, 3, 4, 5}),
	}
	expected := NewUIDList([]uint64{2, 3})
	listEqual(IntersectLists(input), expected, t)
}

func TestIntersectSorted2(t *testing.T) {
	input := []*UIDList{
		newListFromTask([]uint64{1, 1, 1, 1, 1, 1, 2, 2, 2, 2, 3, 3, 3}),
	}
	expected := NewUIDList([]uint64{1, 2, 3})
	listEqual(IntersectLists(input), expected, t)
}

func TestIntersectSorted3(t *testing.T) {
	input := []*UIDList{}
	expected := NewUIDList([]uint64{})
	listEqual(IntersectLists(input), expected, t)
}

func TestIntersectSorted4(t *testing.T) {
	input := []*UIDList{
		NewUIDList([]uint64{100, 101}),
	}
	expected := NewUIDList([]uint64{100, 101})
	listEqual(IntersectLists(input), expected, t)
}

func TestIntersectSorted5(t *testing.T) {
	input := []*UIDList{
		NewUIDList([]uint64{1, 2, 3}),
		newListFromTask([]uint64{2, 3, 4, 5}),
		NewUIDList([]uint64{4, 5, 6}),
	}
	expected := NewUIDList([]uint64{})
	listEqual(IntersectLists(input), expected, t)
}

func TestUIDListIntersect1(t *testing.T) {
	var u, v UIDList
	u.FromUints([]uint64{1, 2, 3})
	v.FromUints([]uint64{})
	u.Intersect(&v)
	listEqual(&u, NewUIDList([]uint64{}), t)
}

func TestUIDListIntersect2(t *testing.T) {
	var u, v UIDList
	u.FromUints([]uint64{1, 2, 3})
	v.FromUints([]uint64{1, 2, 3, 4, 5})
	u.Intersect(&v)
	listEqual(&u, NewUIDList([]uint64{1, 2, 3}), t)
}

func TestUIDListIntersect3(t *testing.T) {
	var u, v UIDList
	u.FromUints([]uint64{1, 2, 3})
	v.FromUints([]uint64{2})
	u.Intersect(&v)
	listEqual(&u, NewUIDList([]uint64{2}), t)
}

func TestUIDListIntersect4(t *testing.T) {
	var u, v UIDList
	u.FromUints([]uint64{1, 2, 3})
	v.FromUints([]uint64{0, 5})
	u.Intersect(&v)
	listEqual(&u, NewUIDList([]uint64{}), t)
}

func TestUIDListIntersect5(t *testing.T) {
	var u, v UIDList
	u.FromUints([]uint64{1, 2, 3})
	v.FromUints([]uint64{3, 5})
	u.Intersect(&v)
	listEqual(&u, NewUIDList([]uint64{3}), t)
}

func TestApplyFilterUint(t *testing.T) {
	var u UIDList
	u.FromUints([]uint64{1, 2, 3, 4, 5})
	u.ApplyFilter(func(a uint64) bool { return (a % 2) == 1 })
	listEqual(&u, NewUIDList([]uint64{1, 3, 5}), t)
}

func TestApplyFilterList(t *testing.T) {
	u := newListFromTask([]uint64{1, 2, 3, 4, 5})
	u.ApplyFilter(func(a uint64) bool { return (a % 2) == 1 })
	listEqual(u, NewUIDList([]uint64{1, 3, 5}), t)
}
