package worker

import (
	"fmt"
	"math"
	"math/rand"
	"sync"

	"github.com/dgraph-io/dgraph/raftwal"
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
	// local stores the groupId to node map for this server.
	local map[uint32]*node
	// all stores the groupId to servers map for the entire cluster.
	all map[uint32]*servers
	num uint32
}

var gr *groupi

// package level init.
func init() {
	gr = new(groupi)
}
func groups() *groupi {
	return gr
}

func StartRaftNodes(wal *raftwal.Wal, raftId uint64, my, peer string) {
	// ch := make(chan error, 2)

	node := groups().newNode(math.MaxUint32, raftId, my)
	go node.InitAndStartNode(wal, peer)

	// Also create node for group zero, which would handle UID assignment.
	node = groups().newNode(0, raftId, my)
	go node.InitAndStartNode(wal, peer)
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

func (g *groupi) newNode(groupId uint32, nodeId uint64, publicAddr string) *node {
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
	sl := g.all[groupId]
	if sl == nil {
		return server{}, false
	}
	for _, s := range sl.list {
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

	sl := g.all[mm.Group()]
	if sl == nil {
		sl = new(servers)
		g.all[mm.Group()] = sl
	}

	for {
		// Remove all instances of the provided node. There should only be one.
		found := false
		for i, s := range sl.list {
			if s.Id == update.Id {
				found = true
				sl.list[i] = sl.list[len(sl.list)-1]
				sl.list = sl.list[:len(sl.list)-1]
			}
		}
		if !found {
			break
		}
	}

	// Append update to the list. If it's a leader, move it to index zero.
	sl.list = append(sl.list, update)
	last := len(sl.list) - 1
	if update.Leader {
		sl.list[0], sl.list[last] = sl.list[last], sl.list[0]
	}

	// Update all servers upwards of index zero as followers.
	for i := 1; i < len(sl.list); i++ {
		sl.list[i].Leader = false
	}
	fmt.Printf("Group: %v. List: %+v\n", mm.Group(), sl.list)
}

func (g *groupi) AnyServer(group uint32) string {
	g.RLock()
	defer g.RUnlock()
	all := g.all[group]
	x.Assert(all != nil)
	sz := len(all.list)
	idx := rand.Intn(sz)
	return all.list[idx].Addr
}

func (g *groupi) Leader(group uint32) string {
	g.RLock()
	defer g.RUnlock()
	all := g.all[group]
	x.Assert(all != nil)
	return all.list[0].Addr
}
