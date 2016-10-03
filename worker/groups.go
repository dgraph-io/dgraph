package worker

import (
	"fmt"
	"sync"

	"github.com/dgraph-io/dgraph/task"
	"github.com/dgraph-io/dgraph/x"
)

type server struct {
	Id     uint64
	Addr   string
	Leader bool
}

type servers struct {
	list []server
}

type groups struct {
	sync.RWMutex
	Local map[uint32]*node
	All   map[uint32]*servers
}

var gr groups

func Node(groupId uint32) *node {
	gr.RLock()
	defer gr.RUnlock()
	n, has := gr.Local[groupId]
	x.Assertf(has, "Node should be present for group: %v", groupId)
	return n
}

func InitNode(groupId uint32, nodeId uint64, publicAddr string) *node {
	gr.Lock()
	defer gr.Unlock()
	if gr.Local == nil {
		gr.Local = make(map[uint32]*node)
	}

	node := newNode(groupId, nodeId, publicAddr)
	if _, has := gr.Local[groupId]; has {
		x.Assertf(false, "Didn't expect a node in RAFT group mapping: %v", groupId)
	}
	gr.Local[groupId] = node
	return node
}

func Server(id uint64, groupId uint32) (rs server, found bool) {
	gr.RLock()
	defer gr.RUnlock()
	if gr.All == nil {
		return server{}, false
	}
	all := gr.All[groupId]
	if all == nil {
		return server{}, false
	}
	for _, s := range all.list {
		if s.Id == id {
			return s, true
		}
	}
	return server{}, false
}

func UpdateServer(mm *task.Membership) {
	update := server{
		Id:     mm.Id(),
		Addr:   string(mm.Addr()),
		Leader: mm.Leader() == byte(1),
	}
	gr.Lock()
	defer gr.Unlock()

	if gr.All == nil {
		gr.All = make(map[uint32]*servers)
	}

	all := gr.All[mm.Group()]
	if all == nil {
		all = new(servers)
		gr.All[mm.Group()] = all
	}

	for {
		// Remove all instances of the provided node. There should only be one.
		found := false
		for i, s := range all.list {
			if s.Id == update.Id {
				found = true
				all.list[i] = all.list[len(all.list)-1]
				all.list = all.list[:len(all.list)-1]
			}
		}
		if !found {
			break
		}
	}

	// Append update to the list. If it's a leader, move it to index zero.
	all.list = append(all.list, update)
	last := len(all.list) - 1
	if update.Leader {
		all.list[0], all.list[last] = all.list[last], all.list[0]
	}

	// Update all servers upwards of index zero as followers.
	for i := 1; i < len(all.list); i++ {
		all.list[i].Leader = false
	}
	fmt.Printf("Group: %v. List: %+v\n", mm.Group(), all.list)
}
