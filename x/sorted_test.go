package x

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

func TestIntersectSorted1(t *testing.T) {
	a1 := []uint64{1, 2, 3}
	a2 := []uint64{2, 3, 4, 5}
	input := [][]uint64{a1, a2}
	expectedOutput := []uint64{2, 3}
	if err := arrayCompare(IntersectSorted(input), expectedOutput); err != nil {
		t.Error(err)
	}
}

func TestIntersectSorted2(t *testing.T) {
	a1 := []uint64{1, 1, 1, 1, 1, 1, 2, 2, 2, 2, 3, 3, 3}
	input := [][]uint64{a1, a1, a1, a1, a1, a1, a1}
	expectedOutput := []uint64{1, 2, 3}
	if err := arrayCompare(IntersectSorted(input), expectedOutput); err != nil {
		t.Error(err)
	}
}

func TestIntersectSorted3(t *testing.T) {
	input := [][]uint64{}
	expectedOutput := []uint64{}
	if err := arrayCompare(IntersectSorted(input), expectedOutput); err != nil {
		t.Error(err)
	}
}

func TestIntersectSorted4(t *testing.T) {
	input := [][]uint64{[]uint64{100, 101}}
	expectedOutput := []uint64{100, 101}
	if err := arrayCompare(IntersectSorted(input), expectedOutput); err != nil {
		t.Error(err)
	}
}

func TestIntersectSorted5(t *testing.T) {
	a1 := []uint64{1, 2, 3}
	a2 := []uint64{2, 3, 4, 5}
	a3 := []uint64{4, 5, 6}
	input := [][]uint64{a1, a2, a3}
	expectedOutput := []uint64{}
	if err := arrayCompare(IntersectSorted(input), expectedOutput); err != nil {
		t.Error(err)
	}
}
