package worker

import (
	"fmt"
	"math"
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

type groupi struct {
	sync.RWMutex
	local map[uint32]*node
	all   map[uint32]*servers
	num   uint32
}

var gr *groupi

// package level init.
func init() {
	gr = new(groupi)
}

func groups() *groupi {
	return gr
}

/*
func pools() *pooli {
	return nil
}
var pl Pools
*/

func StartRaftNodes(raftId uint64, my, cluster, peer string) {
	node := groups().InitNode(math.MaxUint32, raftId, my)
	node.StartNode(cluster)
	if len(peer) > 0 {
		go node.JoinCluster(peer, ws)
	}

	// Also create node for group zero, which would handle UID assignment.
	node = groups().InitNode(0, raftId, my)
	node.StartNode(cluster)
	if len(peer) > 0 {
		go node.JoinCluster(peer, ws)
	}
}

func (g *groupi) Node(groupId uint32) *node {
	g.RLock()
	defer g.RUnlock()
	n, has := g.local[groupId]
	x.Assertf(has, "Node should be present for group: %v", groupId)
	return n
}

func (g *groupi) ServesGroup(groupId uint32) bool {
	g.RLock()
	defer g.RUnlock()
	_, has := g.local[groupId]
	return has
}

func (g *groupi) InitNode(groupId uint32, nodeId uint64, publicAddr string) *node {
	g.Lock()
	defer g.Unlock()
	if g.local == nil {
		g.local = make(map[uint32]*node)
	}

	node := newNode(groupId, nodeId, publicAddr)
	if _, has := g.local[groupId]; has {
		x.Assertf(false, "Didn't expect a node in RAFT group mapping: %v", groupId)
	}
	g.local[groupId] = node
	return node
}

func (g *groupi) Server(id uint64, groupId uint32) (rs server, found bool) {
	g.RLock()
	defer g.RUnlock()
	if g.all == nil {
		return server{}, false
	}
	all := g.all[groupId]
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

func (g *groupi) UpdateServer(mm *task.Membership) {
	update := server{
		Id:     mm.Id(),
		Addr:   string(mm.Addr()),
		Leader: mm.Leader() == byte(1),
	}
	g.Lock()
	defer g.Unlock()

	if g.all == nil {
		g.all = make(map[uint32]*servers)
	}

	all := g.all[mm.Group()]
	if all == nil {
		all = new(servers)
		g.all[mm.Group()] = all
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

/*
func LeaderPool(groupId uint32) *Pool {
	pool := pl.get(addr)
	if pool != nil {
		return pool
	}
	pl.connect(addr)
	return pl.get(addr)
}
*/
