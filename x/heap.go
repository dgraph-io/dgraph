/*
 * Copyright 2015 DGraph Labs, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * 		http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package x

type Elem struct {
	Uid uint64
	Idx int // channel index
}

type Uint64Heap []Elem

func (h Uint64Heap) Len() int           { return len(h) }
func (h Uint64Heap) Less(i, j int) bool { return h[i].Uid < h[j].Uid }
func (h Uint64Heap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *Uint64Heap) Push(x interface{}) {
	*h = append(*h, x.(Elem))
}
func (h *Uint64Heap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}
