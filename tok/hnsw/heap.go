/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package hnsw

import (
	"container/heap"

	c "github.com/hypermodeinc/dgraph/v25/tok/constraints"
)

const notAUid uint64 = 0

type minPersistentHeapElement[T c.Float] struct {
	value T
	index uint64
	// An element that is "filteredOut" is one that should be removed
	// from final consideration due to it not matching the passed in
	// filter criteria.
	filteredOut bool
}

func initPersistentHeapElement[T c.Float](
	val T, i uint64, filteredOut bool) *minPersistentHeapElement[T] {
	return &minPersistentHeapElement[T]{
		value:       val,
		index:       i,
		filteredOut: filteredOut,
	}
}

type minPersistentTupleHeap[T c.Float] []minPersistentHeapElement[T]

func (h minPersistentTupleHeap[T]) Len() int {
	return len(h)
}

func (h minPersistentTupleHeap[T]) Less(i, j int) bool {
	return h[i].value < h[j].value
}

func (h minPersistentTupleHeap[T]) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
}

func (h *minPersistentTupleHeap[T]) Push(x interface{}) {
	*h = append(*h, x.(minPersistentHeapElement[T]))
}

func (h *minPersistentTupleHeap[T]) PopLast() {
	heap.Remove(h, h.Len()-1)
}

func (h *minPersistentTupleHeap[T]) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[:n-1]
	return x
}

// buildPersistentHeapByInit will create a tuple heap using the array of minPersistentHeapElements
// in time O(n), where n = length of array
func buildPersistentHeapByInit[T c.Float](array []minPersistentHeapElement[T]) *minPersistentTupleHeap[T] {
	// initialize the MinTupleHeap that has implement the heap.Interface
	minPersistentTupleHeap := &minPersistentTupleHeap[T]{}
	*minPersistentTupleHeap = array
	heap.Init(minPersistentTupleHeap)
	return minPersistentTupleHeap
}
