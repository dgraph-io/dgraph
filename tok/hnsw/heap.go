/*
 * Copyright 2016-2024 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 * Co-authored by: jairad26@gmail.com, sunil@hypermode.com, bill@hypdermode.com
 */

package hnsw

import (
	"container/heap"

	c "github.com/dgraph-io/dgraph/v24/tok/constraints"
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
