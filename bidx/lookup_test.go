package bidx

import (
	"fmt"
	"testing"
)

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

func TestMergeResults1(t *testing.T) {
	l1 := &LookupResult{
		uid: []uint64{1, 3, 6, 8, 10},
	}
	l2 := &LookupResult{
		uid: []uint64{2, 4, 5, 7, 15},
	}
	lr := []*LookupResult{l1, l2}
	results := mergeResults(lr)
	arrayCompare(results.uid, []uint64{1, 2, 3, 4, 5, 6, 7, 8, 10, 15})
}

func TestMergeResults2(t *testing.T) {
	l1 := &LookupResult{
		uid: []uint64{1, 3, 6, 8, 10},
	}
	l2 := &LookupResult{
		uid: []uint64{},
	}
	lr := []*LookupResult{l1, l2}
	results := mergeResults(lr)
	arrayCompare(results.uid, []uint64{1, 3, 6, 8, 10})
}

func TestMergeResults3(t *testing.T) {
	l1 := &LookupResult{
		uid: []uint64{},
	}
	l2 := &LookupResult{
		uid: []uint64{1, 3, 6, 8, 10},
	}
	lr := []*LookupResult{l1, l2}
	results := mergeResults(lr)
	arrayCompare(results.uid, []uint64{1, 3, 6, 8, 10})
}

func TestMergeResults4(t *testing.T) {
	l1 := &LookupResult{
		uid: []uint64{},
	}
	l2 := &LookupResult{
		uid: []uint64{},
	}
	lr := []*LookupResult{l1, l2}
	results := mergeResults(lr)
	arrayCompare(results.uid, []uint64{})
}

func TestMergeResults5(t *testing.T) {
	l1 := &LookupResult{
		uid: []uint64{11, 13, 16, 18, 20},
	}
	l2 := &LookupResult{
		uid: []uint64{12, 14, 15, 17, 25},
	}
	l3 := &LookupResult{
		uid: []uint64{1, 2},
	}
	lr := []*LookupResult{l1, l2, l3}
	results := mergeResults(lr)
	arrayCompare(results.uid, []uint64{1, 2, 11, 12, 13, 14, 15, 16, 17, 18, 20, 25})
}

func TestMergeResults6(t *testing.T) {
	l1 := &LookupResult{
		uid: []uint64{5, 6, 7},
	}
	l2 := &LookupResult{
		uid: []uint64{3, 4},
	}
	l3 := &LookupResult{
		uid: []uint64{1, 2},
	}
	l4 := &LookupResult{
		uid: []uint64{},
	}
	lr := []*LookupResult{l1, l2, l3, l4}
	results := mergeResults(lr)
	arrayCompare(results.uid, []uint64{1, 2, 3, 4, 5, 6, 7})
}
