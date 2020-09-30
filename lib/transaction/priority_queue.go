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

import (
	"errors"
	"sync"

	"github.com/ChainSafe/gossamer/dot/types"
	"github.com/ChainSafe/gossamer/lib/common"
)

// ErrTransactionExists is returned when trying to add a transaction to the queue that already exists
var ErrTransactionExists = errors.New("transaction is already in queue")

// PriorityQueue implements a priority queue using a double linked list
type PriorityQueue struct {
	head  *node
	mutex sync.Mutex
	txs   map[common.Hash]*ValidTransaction
}

type node struct {
	data   *ValidTransaction
	parent *node
	child  *node
	hash   common.Hash
}

// NewPriorityQueue creates new instance of PriorityQueue
func NewPriorityQueue() *PriorityQueue {
	pq := PriorityQueue{
		head:  nil,
		mutex: sync.Mutex{},
		txs:   make(map[common.Hash]*ValidTransaction),
	}

	return &pq
}

// RemoveExtrinsic removes an extrinsic from the queue
func (q *PriorityQueue) RemoveExtrinsic(ext types.Extrinsic) {
	hash := ext.Hash()

	q.mutex.Lock()
	defer q.mutex.Unlock()

	if q.txs[hash] == nil {
		return
	}

	curr := q.head
	for ; curr != nil; curr = curr.child {
		if curr.data.Extrinsic.Hash() == hash {
			if curr.parent != nil {
				curr.parent.child = curr.child
			} else {
				// head of queue
				q.head = curr.child
			}

			if curr.child != nil {
				curr.child.parent = curr.parent
			}
		}
	}

	delete(q.txs, hash)
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

	delete(q.txs, head.hash)

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

// Pending returns all the transactions currently in the queue
func (q *PriorityQueue) Pending() []*ValidTransaction {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	txs := []*ValidTransaction{}

	curr := q.head
	for {
		if curr == nil {
			return txs
		}

		txs = append(txs, curr.data)
		curr = curr.child
	}
}

// Push traverses the list and places a valid transaction with priority p directly before the
// first node with priority p-1. If there are other nodes with priority p, the new node is placed
// behind them.
func (q *PriorityQueue) Push(vt *ValidTransaction) (common.Hash, error) {
	q.mutex.Lock()
	defer q.mutex.Unlock()
	curr := q.head

	hash := vt.Extrinsic.Hash()
	if q.txs[hash] != nil {
		return hash, ErrTransactionExists
	}

	if curr == nil {
		q.head = &node{data: vt, hash: hash}
		q.txs[hash] = vt
		return hash, nil
	}

	for ; curr != nil; curr = curr.child {
		currPriority := curr.data.Validity.Priority
		if vt.Validity.Priority > currPriority {
			newNode := &node{
				data:   vt,
				parent: curr.parent,
				child:  curr,
				hash:   hash,
			}

			if curr.parent == nil {
				q.head = newNode
			} else {
				curr.parent.child = newNode
			}
			curr.parent = newNode

			q.txs[hash] = vt
			return hash, nil
		} else if curr.child == nil {
			newNode := &node{
				data:   vt,
				parent: curr,
				hash:   hash,
			}
			curr.child = newNode

			q.txs[hash] = vt
			return hash, nil
		}
	}

	return common.Hash{}, nil
}
