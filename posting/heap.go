package posting

import (
	"container/heap"
)

type minBadgerHeapElement struct {
	value float64
	index uint64
}

func initBadgerHeapElement(val float64, i uint64) *minBadgerHeapElement {
	return &minBadgerHeapElement{
		value: val,
		index: i,
	}
}

type minBadgerTupleHeap []minBadgerHeapElement

func (h minBadgerTupleHeap) Len() int {
	return len(h)
}

func (h minBadgerTupleHeap) Less(i, j int) bool {
	return h[i].value < h[j].value
}

func (h minBadgerTupleHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
}

func (h *minBadgerTupleHeap) Push(x interface{}) {
	*h = append(*h, x.(minBadgerHeapElement))
}

func (h *minBadgerTupleHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[:n-1]
	return x
}

// Time: O(n)
func buildBadgerHeapByInit(array []minBadgerHeapElement) *minBadgerTupleHeap {
	// initialize the MinTupleHeap that has implement the heap.Interface
	minBadgerTupleHeap := &minBadgerTupleHeap{}
	*minBadgerTupleHeap = array
	heap.Init(minBadgerTupleHeap)
	return minBadgerTupleHeap
}
