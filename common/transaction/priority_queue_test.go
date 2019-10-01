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

	pq := new(PriorityQueue)
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

	pq := new(PriorityQueue)
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
