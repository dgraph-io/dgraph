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
	"fmt"
	"testing"

	"github.com/dgraph-io/dgraph/task"
	"github.com/google/flatbuffers/go"
)

// TODO(jchiu): Use some test lib or build our own in future.
func arrayCompare(a []uint64, b []uint64) error {
	if len(a) != len(b) {
		return fmt.Errorf("Size mismatch %d vs %d", len(a), len(b))
	}
	for i := 0; i < len(a); i++ {
		if a[i] != b[i] {
			return fmt.Errorf("Element mismatch at index %d", i)
		}
	}
	return nil
}

func TestMergeSorted1(t *testing.T) {
	input := PlainUintLists{
		PlainUintList{1, 3, 6, 8, 10},
		PlainUintList{2, 4, 5, 7, 15},
	}
	expected := []uint64{1, 2, 3, 4, 5, 6, 7, 8, 10, 15}
	if err := arrayCompare(MergeSorted(input), expected); err != nil {
		t.Fatal(err)
	}
}

func TestMergeSorted2(t *testing.T) {
	input := PlainUintLists{
		PlainUintList{1, 3, 6, 8, 10},
		PlainUintList{},
	}
	expected := []uint64{1, 3, 6, 8, 10}
	if err := arrayCompare(MergeSorted(input), expected); err != nil {
		t.Fatal(err)
	}
}

func TestMergeSorted3(t *testing.T) {
	input := PlainUintLists{
		PlainUintList{},
		PlainUintList{1, 3, 6, 8, 10},
	}
	expected := []uint64{1, 3, 6, 8, 10}
	if err := arrayCompare(MergeSorted(input), expected); err != nil {
		t.Fatal(err)
	}
}

func TestMergeSorted4(t *testing.T) {
	input := PlainUintLists{
		PlainUintList{},
		PlainUintList{},
	}
	expected := []uint64{}
	if err := arrayCompare(MergeSorted(input), expected); err != nil {
		t.Fatal(err)
	}
}

func TestMergeSorted5(t *testing.T) {
	input := PlainUintLists{
		PlainUintList{11, 13, 16, 18, 20},
		PlainUintList{12, 14, 15, 15, 16, 16, 17, 25},
		PlainUintList{1, 2},
	}
	expected := []uint64{1, 2, 11, 12, 13, 14, 15, 16, 17, 18, 20, 25}
	if err := arrayCompare(MergeSorted(input), expected); err != nil {
		t.Fatal(err)
	}
}

func TestMergeSorted6(t *testing.T) {
	input := PlainUintLists{
		PlainUintList{5, 6, 7},
		PlainUintList{3, 4},
		PlainUintList{1, 2},
		PlainUintList{},
	}
	expected := []uint64{1, 2, 3, 4, 5, 6, 7}
	if err := arrayCompare(MergeSorted(input), expected); err != nil {
		t.Fatal(err)
	}
}

func TestMergeSorted7(t *testing.T) {
	input := PlainUintLists{}
	expected := []uint64{}
	if err := arrayCompare(MergeSorted(input), expected); err != nil {
		t.Fatal(err)
	}
}

func TestMergeSorted8(t *testing.T) {
	input := PlainUintLists{PlainUintList{1, 1, 1}}
	expected := []uint64{1}
	if err := arrayCompare(MergeSorted(input), expected); err != nil {
		t.Fatal(err)
	}
}

func TestIntersectSorted1(t *testing.T) {
	input := PlainUintLists{
		PlainUintList{1, 2, 3},
		PlainUintList{2, 3, 4, 5},
	}
	expected := []uint64{2, 3}
	if err := arrayCompare(IntersectSorted(input), expected); err != nil {
		t.Error(err)
	}
}

func TestIntersectSorted2(t *testing.T) {
	input := PlainUintLists{
		PlainUintList{1, 1, 1, 1, 1, 1, 2, 2, 2, 2, 3, 3, 3},
	}
	expected := []uint64{1, 2, 3}
	if err := arrayCompare(IntersectSorted(input), expected); err != nil {
		t.Error(err)
	}
}

func TestIntersectSorted3(t *testing.T) {
	input := PlainUintLists{}
	expected := []uint64{}
	if err := arrayCompare(IntersectSorted(input), expected); err != nil {
		t.Error(err)
	}
}

func TestIntersectSorted4(t *testing.T) {
	input := PlainUintLists{PlainUintList{100, 101}}
	expected := []uint64{100, 101}
	if err := arrayCompare(IntersectSorted(input), expected); err != nil {
		t.Error(err)
	}
}

func TestIntersectSorted5(t *testing.T) {
	input := PlainUintLists{
		PlainUintList{1, 2, 3},
		PlainUintList{2, 3, 4, 5},
		PlainUintList{4, 5, 6},
	}
	expected := []uint64{}
	if err := arrayCompare(IntersectSorted(input), expected); err != nil {
		t.Error(err)
	}
}

type uidList struct {
	task.UidList
}

// Get returns i-th element.
func (ul *uidList) Get(i int) uint64 {
	return ul.Uids(i)
}

// Size returns size of UID list.
func (ul *uidList) Size() int {
	return ul.UidsLength()
}

// UidLists is a list of UidList.
type uidLists []*uidList

// Get returns the i-th list.
func (ul uidLists) Get(i int) Uint64List {
	return ul[i]
}

// Size returns number of lists.
func (ul uidLists) Size() int {
	return len(ul)
}

func newUidList(a []uint64) *uidList {
	b := flatbuffers.NewBuilder(0)
	task.UidListStartUidsVector(b, len(a))
	for i := len(a) - 1; i >= 0; i-- {
		b.PrependUint64(a[i])
	}
	ve := b.EndVector(len(a))
	task.UidListStart(b)
	task.UidListAddUids(b, ve)
	uend := task.UidListEnd(b)
	b.Finish(uend)

	ulist := new(uidList)
	data := b.FinishedBytes()
	uo := flatbuffers.GetUOffsetT(data)
	ulist.Init(data, uo)
	return ulist
}

func TestTaskListMerge(t *testing.T) {
	u1 := newUidList([]uint64{1, 2, 3, 3, 6})
	u2 := newUidList([]uint64{4, 8, 9})
	input := uidLists{u1, u2}
	expected := []uint64{1, 2, 3, 4, 6, 8, 9}
	if err := arrayCompare(MergeSorted(input), expected); err != nil {
		t.Fatal(err)
	}
}
