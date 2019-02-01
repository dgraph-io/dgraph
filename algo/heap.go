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

func removeHeap(h *uint64Heap, i int) elem {
	n := h.Len() - 1
	if n != i {
		h.Swap(i, n)
		if !downHeap(h, i, n) {
			upHeap(h, i)
		}
	}
	return h.Pop()
}

func fixHeap(h *uint64Heap, i int) {
	if !downHeap(h, i, h.Len()) {
		upHeap(h, i)
	}
}

func upHeap(h *uint64Heap, j int) {
	for {
		i := (j - 1) / 2 // parent
		if i == j || !h.Less(j, i) {
			break
		}
		h.Swap(i, j)
		j = i
	}
}

func downHeap(h *uint64Heap, i0, n int) bool {
	i := i0
	for {
		j1 := 2*i + 1
		if j1 >= n || j1 < 0 { // j1 < 0 after int overflow
			break
		}
		j := j1 // left child
		if j2 := j1 + 1; j2 < n && h.Less(j2, j1) {
			j = j2 // = 2*i + 2  // right child
		}
		if !h.Less(j, i) {
			break
		}
		h.Swap(i, j)
		i = j
	}
	return i > i0
}
