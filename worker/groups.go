package worker

import (
	"flag"
	"fmt"
	"math"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/dgraph-io/dgraph/raftwal"
	"github.com/dgraph-io/dgraph/store"
	"github.com/dgraph-io/dgraph/task"
	"github.com/dgraph-io/dgraph/x"
)

var (
	groupConf = flag.String("conf", "", "Path to config file with group <-> predicate mapping.")
	groupIds  = flag.String("groups", "0", "RAFT groups handled by this server.")
	myAddr    = flag.String("my", "",
		"addr:port of this server, so other Dgraph servers can talk to this.")
	peer   = flag.String("peer", "", "Address of any peer.")
	raftId = flag.Uint64("idx", 1, "RAFT ID that this server will use to join RAFT groups.")
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
	wal *raftwal.Wal
	// local stores the groupId to node map for this server.
	local map[uint32]*node
	// all stores the groupId to servers map for the entire cluster.
	all map[uint32]*servers
	num uint32
}

var gr *groupi

func groups() *groupi {
	return gr
}

// StartRaftNodes will read the WAL dir, create the RAFT groups,
// and either start or restart RAFT nodes.
func StartRaftNodes(walDir string) {
	gr = new(groupi)

	x.Check(ParseGroupConfig(*groupConf))
	x.Checkf(os.MkdirAll(walDir, 0700), "Error while creating WAL dir.")
	wals, err := store.NewSyncStore(walDir)
	x.Checkf(err, "Error initializing wal store")
	gr.wal = raftwal.Init(wals, *raftId)

	my := *myAddr
	if len(my) == 0 {
		my = fmt.Sprintf("localhost:%d", *workerPort)
	}

	for _, id := range strings.Split(*groupIds, ",") {
		gid, err := strconv.ParseUint(id, 0, 32)
		x.Checkf(err, "Unable to parse group id: %v", id)
		node := groups().newNode(uint32(gid), *raftId, my)
		go node.InitAndStartNode(gr.wal, *peer)
	}
	// TODO: Remove this group that every server is currently part of.
	// Instead, only some servers should contain information about everyone,
	// and everyone should ping those servers for updates.
	node := groups().newNode(math.MaxUint32, *raftId, my)
	go node.InitAndStartNode(gr.wal, *peer)
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
	x.AssertTrue(all != nil)
	sz := len(all.list)
	idx := rand.Intn(sz)
	return all.list[idx].Addr
}

func (g *groupi) Leader(group uint32) string {
	g.RLock()
	defer g.RUnlock()
	all := g.all[group]
	x.AssertTrue(all != nil)
	return all.list[0].Addr
}
