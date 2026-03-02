/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package hnsw

import (
	"container/heap"

	c "github.com/dgraph-io/dgraph/v25/tok/constraints"
)

const notAUid uint64 = 0

type persistentHeapElement[T c.Float] struct {
	value T
	index uint64
	// An element that is "filteredOut" is one that should be removed
	// from final consideration due to it not matching the passed in
	// filter criteria.
	filteredOut bool
}

func initPersistentHeapElement[T c.Float](
	val T, i uint64, filteredOut bool) *persistentHeapElement[T] {
	return &persistentHeapElement[T]{
		value:       val,
		index:       i,
		filteredOut: filteredOut,
	}
}

type minPersistentTupleHeap[T c.Float] []persistentHeapElement[T]

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
	*h = append(*h, x.(persistentHeapElement[T]))
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

// buildMinPersistentHeapByInit will create a min-heap for distance metrics
// in time O(n), where n = length of array
func buildMinPersistentHeapByInit[T c.Float](array []persistentHeapElement[T]) *minPersistentTupleHeap[T] {
	// initialize the MinTupleHeap that has implement the heap.Interface
	minPersistentTupleHeap := &minPersistentTupleHeap[T]{}
	*minPersistentTupleHeap = array
	heap.Init(minPersistentTupleHeap)
	return minPersistentTupleHeap
}

// maxPersistentTupleHeap is a max-heap for similarity metrics (cosine, dot-product)
// where higher values indicate better matches.
type maxPersistentTupleHeap[T c.Float] []persistentHeapElement[T]

func (h maxPersistentTupleHeap[T]) Len() int {
	return len(h)
}

func (h maxPersistentTupleHeap[T]) Less(i, j int) bool {
	return h[i].value > h[j].value // reversed for max-heap
}

func (h maxPersistentTupleHeap[T]) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
}

func (h *maxPersistentTupleHeap[T]) Push(x interface{}) {
	*h = append(*h, x.(persistentHeapElement[T]))
}

func (h *maxPersistentTupleHeap[T]) PopLast() {
	heap.Remove(h, h.Len()-1)
}

func (h *maxPersistentTupleHeap[T]) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[:n-1]
	return x
}

// buildMaxPersistentHeapByInit will create a max-heap for similarity metrics
// in time O(n), where n = length of array
func buildMaxPersistentHeapByInit[T c.Float](array []persistentHeapElement[T]) *maxPersistentTupleHeap[T] {
	maxHeap := &maxPersistentTupleHeap[T]{}
	*maxHeap = array
	heap.Init(maxHeap)
	return maxHeap
}

// candidateHeap is an interface for the candidate heap used in HNSW search.
// It abstracts over min-heap (for distance metrics) and max-heap (for similarity metrics).
type candidateHeap[T c.Float] interface {
	Len() int
	Push(x persistentHeapElement[T])
	Pop() persistentHeapElement[T]
	PopLast()
}

// minHeapWrapper wraps minPersistentTupleHeap to implement candidateHeap interface
type minHeapWrapper[T c.Float] struct {
	h *minPersistentTupleHeap[T]
}

func (w *minHeapWrapper[T]) Len() int {
	return w.h.Len()
}

func (w *minHeapWrapper[T]) Push(x persistentHeapElement[T]) {
	heap.Push(w.h, x)
}

func (w *minHeapWrapper[T]) Pop() persistentHeapElement[T] {
	return heap.Pop(w.h).(persistentHeapElement[T])
}

func (w *minHeapWrapper[T]) PopLast() {
	w.h.PopLast()
}

// maxHeapWrapper wraps maxPersistentTupleHeap to implement candidateHeap interface
type maxHeapWrapper[T c.Float] struct {
	h *maxPersistentTupleHeap[T]
}

func (w *maxHeapWrapper[T]) Len() int {
	return w.h.Len()
}

func (w *maxHeapWrapper[T]) Push(x persistentHeapElement[T]) {
	heap.Push(w.h, x)
}

func (w *maxHeapWrapper[T]) Pop() persistentHeapElement[T] {
	return heap.Pop(w.h).(persistentHeapElement[T])
}

func (w *maxHeapWrapper[T]) PopLast() {
	w.h.PopLast()
}

// buildCandidateHeap creates the appropriate heap based on whether we're using
// a distance metric (lower is better) or similarity metric (higher is better).
func buildCandidateHeap[T c.Float](array []persistentHeapElement[T], isSimilarityMetric bool) candidateHeap[T] {
	if isSimilarityMetric {
		return &maxHeapWrapper[T]{h: buildMaxPersistentHeapByInit(array)}
	}
	return &minHeapWrapper[T]{h: buildMinPersistentHeapByInit(array)}
}
