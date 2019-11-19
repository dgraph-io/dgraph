// Copyright 2019 ChainSafe Systems (ON) Corp.
// This file is part of gossamer.
//
// The gossamer library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The gossamer library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the gossamer library. If not, see <http://www.gnu.org/licenses/>.

package transaction

import "sync"

// PriorityQueue implements a priority queue using a double linked list
type PriorityQueue struct {
	head  *node
	mutex sync.Mutex
}

type node struct {
	data   *ValidTransaction
	parent *node
	child  *node
}

func NewPriorityQueue() *PriorityQueue {
	pq := PriorityQueue{
		head:  nil,
		mutex: sync.Mutex{},
	}

	return &pq
}

// Pop removes the head of the queue and returns it
func (q *PriorityQueue) Pop() *ValidTransaction {
	q.mutex.Lock()
	defer q.mutex.Unlock()
	if q.head == nil {
		return nil
	}
	head := q.head
	q.head = head.child
	return head.data
}

// Peek returns the next item without removing it from the queue
func (q *PriorityQueue) Peek() *ValidTransaction {
	q.mutex.Lock()
	defer q.mutex.Unlock()
	if q.head == nil {
		return nil
	}
	return q.head.data
}

// Insert traverses the list and places a valid transaction with priority p directly before the
// first node with priority p-1. If there are other nodes with priority p, the new node is placed
// behind them.
func (q *PriorityQueue) Insert(vt *ValidTransaction) {
	q.mutex.Lock()
	defer q.mutex.Unlock()
	curr := q.head
	if curr == nil {
		q.head = &node{data: vt}
		return
	}

	for ; curr != nil; curr = curr.child {
		currPriority := curr.data.Validity.Priority
		if vt.Validity.Priority > currPriority {
			newNode := &node{
				data:   vt,
				parent: curr.parent,
				child:  curr,
			}

			if curr.parent == nil {
				q.head = newNode
			} else {
				curr.parent.child = newNode
			}
			curr.parent = newNode

			return
		} else if curr.child == nil {
			newNode := &node{
				data:   vt,
				parent: curr,
			}
			curr.child = newNode
			return
		}
	}
}
