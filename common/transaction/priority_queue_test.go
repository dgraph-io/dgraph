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
			Validity: &Validity{Priority: 1},
		},
		{
			Validity: &Validity{Priority: 3},
		},
		{
			Validity: &Validity{Priority: 2},
		},
		{
			Validity: &Validity{Priority: 17},
		},
		{
			Validity: &Validity{Priority: 2},
		},
	}

	pq := NewPriorityQueue()
	expected := []int{3, 1, 2, 4, 0}

	for _, node := range tests {
		pq.Insert(node)
	}

	for _, exp := range expected {
		n := pq.Pop()
		if !reflect.DeepEqual(n, tests[exp]) {
			t.Fatalf("Fail: got %v expected %v", n, tests[exp])
		}
	}
}

func TestPriorityQueueAgain(t *testing.T) {
	tests := []*ValidTransaction{
		{
			Validity: &Validity{Priority: 2},
		},
		{
			Validity: &Validity{Priority: 3},
		},
		{
			Validity: &Validity{Priority: 2},
		},
		{
			Validity: &Validity{Priority: 3},
		},
		{
			Validity: &Validity{Priority: 1},
		},
	}

	pq := NewPriorityQueue()
	expected := []int{1, 3, 0, 2, 4}

	for _, node := range tests {
		pq.Insert(node)
	}

	for _, exp := range expected {
		n := pq.Pop()
		if !reflect.DeepEqual(n, tests[exp]) {
			t.Fatalf("Fail: got %v expected %v", n, tests[exp])
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

func TestPeek(t *testing.T) {
	tests := []*ValidTransaction{
		{
			Validity: &Validity{Priority: 2},
		},
		{
			Validity: &Validity{Priority: 3},
		},
		{
			Validity: &Validity{Priority: 2},
		},
		{
			Validity: &Validity{Priority: 3},
		},
		{
			Validity: &Validity{Priority: 1},
		},
	}

	pq := NewPriorityQueue()
	expected := []int{1, 3, 0, 2, 4}

	for _, node := range tests {
		pq.Insert(node)
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
		pq.Insert(&ValidTransaction{Validity: &Validity{Priority: 1}})
	}()
	go func() {
		pq.Insert(&ValidTransaction{Validity: &Validity{Priority: 1}})
	}()

	go func() {
		pq.Pop()
	}()

	go func() {
		pq.Pop()
	}()

	go func() {
		pq.Peek()
	}()
	go func() {
		pq.Peek()
	}()
}
