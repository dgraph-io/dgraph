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
	"container/heap"
	"errors"
	"sync"

	"github.com/ChainSafe/gossamer/dot/types"
	"github.com/ChainSafe/gossamer/lib/common"
)

// ErrTransactionExists is returned when trying to add a transaction to the queue that already exists
var ErrTransactionExists = errors.New("transaction is already in queue")

// An Item is something we manage in a priority queue.
type Item struct {
	data *ValidTransaction
	hash common.Hash

	priority uint64 // The priority of the item in the queue.

	// The order is an monotonically increasing sequence and is used to differentiate between `Item`
	// having the same priority value.
	order uint64

	// The index is needed by update and is maintained by the heap.Interface methods.
	index int // The index of the item in the heap.
}

// A PriorityQueue implements heap.Interface and holds Items.
type priorityQueue []*Item

func (pq priorityQueue) Len() int { return len(pq) }

func (pq priorityQueue) Less(i, j int) bool {
	// For Item having same priority value we compare them based on their insertion order(FIFO).
	if pq[i].priority == pq[j].priority {
		return pq[i].order < pq[j].order
	}
	// We want Pop to give us the highest, not lowest, priority so we use greater than here.
	return pq[i].priority > pq[j].priority
}

func (pq priorityQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
	pq[i].index = i
	pq[j].index = j
}

func (pq *priorityQueue) Push(x interface{}) {
	n := len(*pq)
	item := x.(*Item)
	item.index = n
	*pq = append(*pq, item)
}

func (pq *priorityQueue) Pop() interface{} {
	old := *pq
	n := len(old)
	item := old[n-1]
	old[n-1] = nil  // avoid memory leak
	item.index = -1 // for safety
	*pq = old[0 : n-1]
	return item
}

// PriorityQueue is a thread safe wrapper over `priorityQueue`
type PriorityQueue struct {
	pq        priorityQueue
	currOrder uint64
	txs       map[common.Hash]*Item
	sync.Mutex
}

func NewPriorityQueue() *PriorityQueue {
	spq := &PriorityQueue{
		pq:  make(priorityQueue, 0),
		txs: make(map[common.Hash]*Item),
	}
	heap.Init(&spq.pq)
	return spq
}

func (spq *PriorityQueue) RemoveExtrinsic(ext types.Extrinsic) {
	spq.Lock()
	defer spq.Unlock()

	hash := ext.Hash()
	item, ok := spq.txs[hash]
	if !ok {
		return
	}

	heap.Remove(&spq.pq, item.index)
	delete(spq.txs, hash)
}

// Push inserts a valid transaction with priority p into the queue
func (spq *PriorityQueue) Push(txn *ValidTransaction) (common.Hash, error) {
	spq.Lock()
	defer spq.Unlock()

	hash := txn.Extrinsic.Hash()
	if spq.txs[hash] != nil {
		return hash, ErrTransactionExists
	}

	item := &Item{
		data:     txn,
		hash:     hash,
		order:    spq.currOrder,
		priority: txn.Validity.Priority,
	}
	spq.currOrder++
	heap.Push(&spq.pq, item)
	spq.txs[hash] = item

	return hash, nil
}

// Pop removes the transaction with has the highest priority value from the queue and returns it.
// If there are multiple transaction with same priority value then it return them in FIFO order.
func (spq *PriorityQueue) Pop() *ValidTransaction {
	spq.Lock()
	defer spq.Unlock()
	if spq.pq.Len() == 0 {
		return nil
	}

	item := heap.Pop(&spq.pq).(*Item)
	delete(spq.txs, item.hash)
	return item.data
}

// Peek returns the next item without removing it from the queue
func (spq *PriorityQueue) Peek() *ValidTransaction {
	spq.Lock()
	defer spq.Unlock()
	if spq.pq.Len() == 0 {
		return nil
	}
	return spq.pq[0].data
}

// Pending returns all the transactions currently in the queue
func (spq *PriorityQueue) Pending() []*ValidTransaction {
	spq.Lock()
	defer spq.Unlock()

	var txns []*ValidTransaction
	for idx := 0; idx < spq.pq.Len(); idx++ {
		txns = append(txns, spq.pq[idx].data)
	}
	return txns
}
