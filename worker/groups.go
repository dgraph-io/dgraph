package worker

import (
	"sync"

	"github.com/dgraph-io/dgraph/x"
)

type groups struct {
	sync.RWMutex
	Map map[uint32]*node
}

var gr groups

func Node(groupId uint32) *node {
	gr.RLock()
	defer gr.RUnlock()
	n, has := gr.Map[groupId]
	x.Assertf(has, "Node should be present for group: %v", groupId)
	return n
}

func StoreNode(groupId uint32, newn *node) *node {
	gr.Lock()
	defer gr.Unlock()
	if n, has := gr.Map[groupId]; has {
		newn.Stop() // Stop the provided node.
		return n
	}
	gr.Map[groupId] = newn
	return newn
}
