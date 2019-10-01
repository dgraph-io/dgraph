package transaction

// PriorityQueue implements a priority queue using a double linked list
type PriorityQueue struct {
	head *node
}

type node struct {
	data   *ValidTransaction
	parent *node
	child  *node
}

// Pop removes the head of the queue and returns it
func (q *PriorityQueue) Pop() *ValidTransaction {
	head := q.head
	q.head = head.child
	return head.data
}

func (q *PriorityQueue) Peek() *ValidTransaction {
	return q.head.data
}

// Insert traverses the list and places a valid transaction with priority p directly before the
// first node with priority p-1. If there are other nodes with priority p, the new node is placed
// behind them.
func (q *PriorityQueue) Insert(vt *ValidTransaction) {
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
