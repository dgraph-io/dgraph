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

func InitNode(groupId uint32, nodeId uint64, publicAddr string) *node {
	gr.Lock()
	defer gr.Unlock()
	if gr.Map == nil {
		gr.Map = make(map[uint32]*node)
	}

	node := newNode(groupId, nodeId, publicAddr)
	if _, has := gr.Map[groupId]; has {
		x.Assertf(false, "Didn't expect a node in RAFT group mapping: %v", groupId)
	}
	gr.Map[groupId] = node
	return node
}
