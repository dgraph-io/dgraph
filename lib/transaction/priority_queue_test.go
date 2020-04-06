// Copyright 2019 ChainSafe Systems (ON) Corp.
// This file is part of gossamer.
//
// The gossamer library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The gossamer library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the gossamer library. If not, see <http://www.gnu.org/licenses/>.

package transaction

import (
	"reflect"
	"testing"
)

func TestPriorityQueue(t *testing.T) {
	tests := []*ValidTransaction{
		{
			Extrinsic: []byte("a"),
			Validity:  &Validity{Priority: 1},
		},
		{
			Extrinsic: []byte("b"),
			Validity:  &Validity{Priority: 4},
		},
		{
			Extrinsic: []byte("c"),
			Validity:  &Validity{Priority: 2},
		},
		{
			Extrinsic: []byte("d"),
			Validity:  &Validity{Priority: 17},
		},
		{
			Extrinsic: []byte("e"),
			Validity:  &Validity{Priority: 2},
		},
	}

	pq := NewPriorityQueue()
	expected := []int{3, 1, 2, 4, 0}

	for _, node := range tests {
		pq.Push(node)
	}

	for i, exp := range expected {
		n := pq.Pop()
		if !reflect.DeepEqual(n, tests[exp]) {
			t.Log(n.Validity)
			t.Log(tests[exp].Validity)
			t.Fatalf("Fail: iteration %d got %v expected %v", i, n, tests[exp])
		}
	}
}

func TestPriorityQueueAgain(t *testing.T) {
	tests := []*ValidTransaction{
		{
			Extrinsic: []byte("a"),
			Validity:  &Validity{Priority: 2},
		},
		{
			Extrinsic: []byte("b"),
			Validity:  &Validity{Priority: 3},
		},
		{
			Extrinsic: []byte("c"),
			Validity:  &Validity{Priority: 2},
		},
		{
			Extrinsic: []byte("d"),
			Validity:  &Validity{Priority: 3},
		},
		{
			Extrinsic: []byte("e"),
			Validity:  &Validity{Priority: 1},
		},
	}

	pq := NewPriorityQueue()
	expected := []int{1, 3, 0, 2, 4}

	for _, node := range tests {
		pq.Push(node)
	}

	for i, exp := range expected {
		n := pq.Pop()
		if !reflect.DeepEqual(n, tests[exp]) {
			t.Fatalf("Fail: iteration %d got %v expected %v", i, n, tests[exp])
		}
	}
}

func TestPeek_Empty(t *testing.T) {
	pq := NewPriorityQueue()
	vt := pq.Peek()
	if vt != nil {
		t.Fatalf("Fail: expected nil for empty queue")
	}
}

func TestPriorityQueue_Pop(t *testing.T) {
	pq := NewPriorityQueue()

	val := pq.Pop()

	if val != nil {
		t.Errorf("pop on empty list should return nil")
	}
	val = pq.Peek()
	if val != nil {
		t.Errorf("pop on empty list should return nil")
	}

	pq.Push(&ValidTransaction{
		Extrinsic: []byte{},
		Validity:  new(Validity),
	})

	peek := pq.Peek()
	if peek == nil {
		t.Errorf("expected item, got nil Peek()")
	}

	pop := pq.Pop()
	if pop == nil {
		t.Errorf("expected item, got nil for Pop()")
	}

	if !reflect.DeepEqual(peek, pop) {
		t.Error("Peek() did not return the same value as Pop()")
	}
}

func TestPeek(t *testing.T) {
	tests := []*ValidTransaction{
		{
			Extrinsic: []byte("a"),
			Validity:  &Validity{Priority: 2},
		},
		{
			Extrinsic: []byte("b"),
			Validity:  &Validity{Priority: 3},
		},
		{
			Extrinsic: []byte("c"),
			Validity:  &Validity{Priority: 2},
		},
		{
			Extrinsic: []byte("d"),
			Validity:  &Validity{Priority: 3},
		},
		{
			Extrinsic: []byte("e"),
			Validity:  &Validity{Priority: 1},
		},
	}

	pq := NewPriorityQueue()
	expected := []int{1, 3, 0, 2, 4}

	for _, node := range tests {
		pq.Push(node)
	}

	for _, exp := range expected {
		n := pq.Peek()
		if !reflect.DeepEqual(n, tests[exp]) {
			t.Fatalf("Fail: got %v expected %v", n, tests[exp])
		}
		pq.Pop()
	}
}

func TestPriorityQueueConcurrentCalls(t *testing.T) {
	pq := NewPriorityQueue()

	go func() {
		pq.Push(&ValidTransaction{Validity: &Validity{Priority: 1}})
		pq.Peek()
		pq.Pop()
	}()
	go func() {
		pq.Push(&ValidTransaction{Validity: &Validity{Priority: 1}})
		pq.Peek()
		pq.Pop()
	}()

}

func TestPending(t *testing.T) {
	tests := []*ValidTransaction{
		{
			Extrinsic: []byte("a"),
			Validity:  &Validity{Priority: 5},
		},
		{
			Extrinsic: []byte("b"),
			Validity:  &Validity{Priority: 4},
		},
		{
			Extrinsic: []byte("c"),
			Validity:  &Validity{Priority: 3},
		},
		{
			Extrinsic: []byte("d"),
			Validity:  &Validity{Priority: 2},
		},
		{
			Extrinsic: []byte("e"),
			Validity:  &Validity{Priority: 1},
		},
	}

	pq := NewPriorityQueue()

	for _, node := range tests {
		pq.Push(node)
	}

	pending := pq.Pending()
	if !reflect.DeepEqual(pending, tests) {
		t.Fatalf("Fail: got %v expected %v", pending, tests)
	}
}

func TestRemoveExtrinsic(t *testing.T) {
	tests := []*ValidTransaction{
		{
			Extrinsic: []byte("rats"),
			Validity:  &Validity{Priority: 5},
		},
		{
			Extrinsic: []byte("arecool"),
			Validity:  &Validity{Priority: 4},
		},
	}

	pq := NewPriorityQueue()

	for _, node := range tests {
		pq.Push(node)
	}

	pq.RemoveExtrinsic(tests[0].Extrinsic)

	res := pq.Pop()
	if !reflect.DeepEqual(res, tests[1]) {
		t.Fatalf("Fail: got %v expected %v", res, tests[1])
	}
}
