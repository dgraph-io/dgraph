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

	"github.com/dgraph-io/dgraph/task"
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

// UIDList is a list of UIDs. We might consider moving this to another package
// that provides some wrapper around task.UidList.
type UIDList struct{ task.UidList }

// Get returns i-th element.
func (ul *UIDList) Get(i int) uint64 { return ul.Uids(i) }

// Size returns size of UID list.
func (ul *UIDList) Size() int { return ul.UidsLength() }

// UIDLists is a list of UIDList.
type UIDLists []*UIDList

// Get returns the i-th list.
func (ul UIDLists) Get(i int) Uint64List { return ul[i] }

// Size returns number of lists.
func (ul UIDLists) Size() int { return len(ul) }

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
				val:     l.Get(0),
				listIdx: i,
			})
		}
	}

	// Our final output. Give it some capacity.
	output := make([]uint64, 0, 100)
	// idx[i] is the element we are looking at for lists[i].
	idx := make([]int, n)
	var last uint64   // Last element added to sorted / final output.
	for h.Len() > 0 { // While heap is not empty.
		me := (*h)[0] // Peek at the top element in heap.
		if len(output) == 0 || me.val != last {
			output = append(output, me.val) // Add if unique.
			last = me.val
		}
		l := lists.Get(me.listIdx)
		if idx[me.listIdx] >= l.Size()-1 {
			heap.Pop(h)
		} else {
			idx[me.listIdx]++
			val := l.Get(idx[me.listIdx])
			(*h)[0].val = val
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
	// idx[i] is the element we are looking at for lists[i].
	idx := make([]int, n)
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
			k := idx[j]
			for ; k < l.Size() && l.Get(k) < val; k++ {
			}
			idx[j] = k
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
