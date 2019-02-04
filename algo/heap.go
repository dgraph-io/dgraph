/*
 * Copyright 2016-2018 Dgraph Labs, Inc. and Contributors
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
 */

package algo

type elem struct {
	val     uint64 // Value of this element.
	listIdx int    // Which list this element comes from.
}

type uint64Heap []elem

func (h uint64Heap) Len() int           { return len(h) }
func (h uint64Heap) Less(i, j int) bool { return h[i].val < h[j].val }
func (h uint64Heap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *uint64Heap) Push(x elem) {
	*h = append(*h, x)
}

func (h *uint64Heap) Pop() elem {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}

// the code below has been adapted from src/container/heap/heap.go
func initHeap(h *uint64Heap) {
	n := h.Len()
	for i := n/2 - 1; i >= 0; i-- {
		downHeap(h, i, n)
	}
}

func pushHeap(h *uint64Heap, x elem) {
	h.Push(x)
	upHeap(h, h.Len()-1)
}

func popHeap(h *uint64Heap) elem {
	n := h.Len() - 1
	h.Swap(0, n)
	downHeap(h, 0, n)
	return h.Pop()
}

func fixHeap(h *uint64Heap, i int) {
	if !downHeap(h, i, h.Len()) {
		upHeap(h, i)
	}
}

// upHeap refines the value level
// starting from the bottom level “pos” to the parent levels, in the binary tree “h”
func upHeap(h *uint64Heap, pos int) {
	for {
		parent := (pos - 1) / 2 // parent
		if parent == pos || !h.Less(pos, parent) {
			break
		}
		h.Swap(parent, pos)
		pos = parent
	}
}

// downHeap refines the value level
// starting from the upper level “pos” to the right levels, in the binary tree “h”
func downHeap(h *uint64Heap, pos, n int) bool {
	current := pos
	for {
		high := 2*current + 1
		if high >= n || high < 0 { // high < 0 after int overflow
			break
		}
		node := high // left child
		if next := high + 1; next < n && h.Less(next, high) {
			node = next // = 2*i + 2  right child
		}
		if !h.Less(node, current) {
			break
		}
		h.Swap(current, node)
		current = node
	}
	return current > pos
}
