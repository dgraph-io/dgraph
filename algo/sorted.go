/*
 * Copyright 2016 Dgraph Labs, Inc.
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

package algo

import (
	"container/heap"
)

// Uint64Lists is a list of Uint64List. We need this because []X is not equal to
// []interface{} and we do not want to incur a O(n) cost for a trivial conversion.
type Uint64Lists interface {
	Get(int) Uint64List // Returns i-th list.
	Size() int
}

// Uint64List is a list of uint64s.
type Uint64List interface {
	Get(int) uint64
	Size() int
}

// PlainUintLists is the simplest possible Uint64Lists.
type PlainUintLists []PlainUintList

// Size returns number of lists.
func (s PlainUintLists) Size() int { return len(s) }

// Get returns the i-th list.
func (s PlainUintLists) Get(i int) Uint64List { return s[i] }

// PlainUintList is the simplest possible Uint64List.
type PlainUintList []uint64

// Size returns size of list.
func (s PlainUintList) Size() int { return len(s) }

// Get returns i-th element of list.
func (s PlainUintList) Get(i int) uint64 { return (s)[i] }

// MergeSorted merges sorted uint64 lists. Only unique numbers are returned.
// In the future, we might have another interface for the output.
func MergeSorted(lists Uint64Lists) []uint64 {
	n := lists.Size()
	if n == 0 {
		return []uint64{}
	}

	h := &uint64Heap{}
	heap.Init(h)

	for i := 0; i < n; i++ {
		l := lists.Get(i)
		if l.Size() > 0 {
			heap.Push(h, elem{
				Val: l.Get(0),
				Idx: i,
			})
		}
	}

	// Our final output. Give it some capacity.
	output := make([]uint64, 0, 100)

	// ptr[i] is the element we are looking at for lists[i].
	ptr := make([]int, n)

	var last uint64   // Last element added to sorted / final output.
	for h.Len() > 0 { // While heap is not empty.
		me := (*h)[0] // Peek at the top element in heap.
		if len(output) == 0 || me.Val != last {
			output = append(output, me.Val) // Add if unique.
			last = me.Val
		}
		l := lists.Get(me.Idx)
		if ptr[me.Idx] >= l.Size()-1 {
			heap.Pop(h)
		} else {
			ptr[me.Idx]++
			val := l.Get(ptr[me.Idx])
			(*h)[0].Val = val
			heap.Fix(h, 0) // Faster than Pop() followed by Push().
		}
	}
	return output
}

// IntersectSorted returns intersection of uint64 lists.
func IntersectSorted(lists Uint64Lists) []uint64 {
	n := lists.Size()
	if n == 0 {
		return []uint64{}
	}

	// Scan through the smallest list. Denote as A.
	// For each x in A,
	//   For each other list B,
	//     Keep popping elements until we get a y >= x.
	//     If y > x, we want to skip x. Break out of loop for B.
	//   If we reach here, append our output by x.
	// We also remove all duplicates.
	var minLen, minLenIndex int
	// minLen array is lists[minLenIndex].
	for i := 0; i < n; i++ {
		l := lists.Get(i)
		if l.Size() < minLen {
			minLen = l.Size()
			minLenIndex = i
		}
	}

	// Our final output. Give it some capacity.
	output := make([]uint64, 0, minLenIndex)

	// ptr[i] is the element we are looking at for lists[i].
	ptr := make([]int, n)

	shortList := lists.Get(minLenIndex)
	for i := 0; i < shortList.Size(); i++ {
		val := shortList.Get(i)
		if i > 0 && val == shortList.Get(i-1) {
			continue // Avoid duplicates.
		}

		var skip bool            // Should we skip val in output?
		for j := 0; j < n; j++ { // For each other list in lists.
			if j == minLenIndex {
				// No point checking yourself.
				continue
			}
			l := lists.Get(j)
			k := ptr[j]
			for ; k < l.Size() && l.Get(k) < val; k++ {
			}
			ptr[j] = k
			if l.Get(k) > val {
				skip = true
				break
			}
			// Otherwise, l[k] = val and we continue checking other lists.
		}
		if !skip {
			output = append(output, val)
		}
	}
	return output
}
