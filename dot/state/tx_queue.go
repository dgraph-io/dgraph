package state

import (
	"github.com/ChainSafe/gossamer/lib/transaction"
)

// TransactionQueue represents the queue of transactions
type TransactionQueue struct {
	queue *transaction.PriorityQueue
}

// NewTransactionQueue returns a new TransactionQueue
func NewTransactionQueue() *TransactionQueue {
	return &TransactionQueue{
		queue: transaction.NewPriorityQueue(),
	}
}

// Push pushes a transaction to the queue, ordered by priority
func (q *TransactionQueue) Push(vt *transaction.ValidTransaction) {
	q.queue.Push(vt)
}

// Pop removes and returns the head of the queue
func (q *TransactionQueue) Pop() *transaction.ValidTransaction {
	return q.queue.Pop()
}

// Peek returns the head of the queue without removing it
func (q *TransactionQueue) Peek() *transaction.ValidTransaction {
	return q.queue.Peek()
}

// Pending returns the current transactions in the queue
func (q *TransactionQueue) Pending() []*transaction.ValidTransaction {
	return q.queue.Pending()
}
