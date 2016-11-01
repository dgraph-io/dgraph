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

	flatbuffers "github.com/google/flatbuffers/go"
	"golang.org/x/net/context"
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
	Id      uint64
	Addr    string
	Leader  bool
	RaftIdx uint64
}

type servers struct {
	list []server
}

type groupi struct {
	sync.RWMutex
	ctx context.Context
	wal *raftwal.Wal
	// local stores the groupId to node map for this server.
	local map[uint32]*node
	// all stores the groupId to servers map for the entire cluster.
	all        map[uint32]*servers
	num        uint32
	lastUpdate uint64
}

var gr *groupi

func groups() *groupi {
	return gr
}

// StartRaftNodes will read the WAL dir, create the RAFT groups,
// and either start or restart RAFT nodes.
func StartRaftNodes(walDir string) {
	gr = new(groupi)
	gr.ctx = context.Background()

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
	x.AssertTruef(has, "Node should be present for group: %v", groupId)
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
		x.AssertTruef(false, "Didn't expect a node in RAFT group mapping: %v", groupId)
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

func (g *groupi) isDuplicate(gid uint32, nid uint64, addr string, leader bool) bool {
	g.RLock()
	defer g.RUnlock()
	return g.duplicate(gid, nid, addr, leader)
}

func (g *groupi) duplicate(gid uint32, nid uint64, addr string, leader bool) bool {
	sl := g.all[gid]
	if sl == nil {
		return false
	}
	for _, s := range sl.list {
		if s.Id == nid && s.Addr == addr && s.Leader == leader {
			return true
		}
	}
	return false
}

func (g *groupi) LastUpdate() uint64 {
	g.RLock()
	defer g.RUnlock()
	return g.lastUpdate
}

func buildTaskMembership(b *flatbuffers.Builder, gid uint32,
	id uint64, addr []byte, leader bool) flatbuffers.UOffsetT {

	var l byte
	if leader {
		l = 1
	}
	so := b.CreateByteString(addr)
	task.MembershipStart(b)
	task.MembershipAddId(b, id)
	task.MembershipAddGroup(b, gid)
	task.MembershipAddAddr(b, so)
	task.MembershipAddLeader(b, l)
	return task.MembershipEnd(b)
}

func (g *groupi) inform() {
	if g.ServesGroup(0) {
		// This server serves group zero.
		g.RLock()
		defer g.RUnlock()

		for _, n := range g.local {
			rc := task.GetRootAsRaftContext(n.raftContext, 0)
			if g.duplicate(rc.Group(), rc.Id(), string(rc.Addr()), n.AmLeader()) {
				continue
			}

			go func(rc *task.RaftContext, amleader bool) {
				b := flatbuffers.NewBuilder(0)
				uo := buildTaskMembership(b, rc.Group(), rc.Id(), rc.Addr(), amleader)
				b.Finish(uo)
				data := b.Bytes[b.Head():]

				zero := groups().Node(0)
				zero.ProposeAndWait(zero.ctx, membershipMsg, data)

			}(rc, n.AmLeader())
		}
		return
	}

	var data []byte
	{
		g.RLock()
		b := flatbuffers.NewBuilder(0)
		uoffsets := make([]flatbuffers.UOffsetT, 0, 10)
		for _, n := range g.local {
			rc := task.GetRootAsRaftContext(n.raftContext, 0)
			uo := buildTaskMembership(b, rc.Group(), rc.Id(), rc.Addr(), n.AmLeader())
			uoffsets = append(uoffsets, uo)
		}

		task.MembershipUpdateStartMembersVector(b, len(uoffsets))
		for _, uo := range uoffsets {
			b.PrependUOffsetT(uo)
		}
		me := b.EndVector(len(uoffsets))

		task.MembershipUpdateStart(b)
		task.MembershipUpdateAddLastUpdate(b, g.lastUpdate)
		task.MembershipUpdateAddMembers(b, me)
		b.Finish(task.MembershipUpdateEnd(b))
		data = b.FinishedBytes()
		g.RUnlock()
	}

	// This server isn't serving group zero. Send an update to peer.
	var pl *pool
	addr := g.AnyServer(0)

UPDATEMEMBERSHIP:
	if len(addr) > 0 {
		pl = pools().get(addr)
	} else {
		pl = pools().any()
	}
	conn, err := pl.Get()
	x.Check(err)
	defer pl.Put(conn)

	c := NewWorkerClient(conn)
	p := &Payload{Data: data}
	out, err := c.UpdateMembership(g.ctx, p)
	if err == errRedirect {
		x.TraceError(g.ctx, err)
		addr = string(out.Data)
		pools().connect(addr)
		goto UPDATEMEMBERSHIP

	} else if err != nil {
		x.TraceError(g.ctx, err)
		return
	}
	update := task.GetRootAsMembershipUpdate(out.Data, 0)
	for i := 0; i < update.MembersLength(); i++ {
		mm := new(task.Membership)
		x.AssertTrue(update.Members(mm, i))
		g.applyMembershipUpdate(0, mm)
	}
}

func (g *groupi) applyMembershipUpdate(raftIdx uint64, mm *task.Membership) {
	update := server{
		Id:      mm.Id(),
		Addr:    string(mm.Addr()),
		Leader:  mm.Leader() == byte(1),
		RaftIdx: raftIdx,
	}
	g.Lock()
	defer g.Unlock()

	if g.lastUpdate < raftIdx {
		g.lastUpdate = raftIdx
	}

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

func (g *groupi) MembershipUpdateAfter(ridx uint64) []byte {
	g.RLock()
	defer g.RUnlock()

	var maxIdx uint64
	b := flatbuffers.NewBuilder(0)
	uoffsets := make([]flatbuffers.UOffsetT, 0, 10)
	for gid, peers := range g.all {
		for _, s := range peers.list {
			if s.RaftIdx <= ridx {
				continue
			}
			if s.RaftIdx > maxIdx {
				maxIdx = s.RaftIdx
			}
			uo := buildTaskMembership(b, gid, s.Id, []byte(s.Addr), s.Leader)
			uoffsets = append(uoffsets, uo)
		}
	}

	task.MembershipUpdateStartMembersVector(b, len(uoffsets))
	for _, uo := range uoffsets {
		b.PrependUOffsetT(uo)
	}
	me := b.EndVector(len(uoffsets))

	task.MembershipUpdateStart(b)
	task.MembershipUpdateAddLastUpdate(b, maxIdx)
	task.MembershipUpdateAddMembers(b, me)
	b.Finish(task.MembershipUpdateEnd(b))

	return b.FinishedBytes()
}

func (w *grpcWorker) UpdateMembership(ctx context.Context, query *Payload) (*Payload, error) {
	if ctx.Err() != nil {
		return &Payload{}, ctx.Err()
	}
	if !groups().ServesGroup(0) {
		return &Payload{Data: []byte(groups().AnyServer(0))}, errRedirect
	}

	update := task.GetRootAsMembershipUpdate(query.Data, 0)
	zero := groups().Node(0)
	_ = zero
	che := make(chan error, 1)
	_ = che
	// TODO: Work on this.
	for i := 0; i < update.MembersLength(); i++ {
		mm := new(task.Membership)
		x.AssertTrue(update.Members(mm, i))
		if groups().isDuplicate(mm.Group(), mm.Id(), string(mm.Addr()), mm.Leader() == 1) {
			continue
		}
	}
	//if groups().isDuplicate(mm) {
	//che <- nil
	//} else {
	//che <- n.ProposeAndWait(ctx, membershipMsg, query.Data)
	//}

	select {
	case <-ctx.Done():
		return &Payload{}, ctx.Err()
	case err := <-che:
		if err != nil {
			return &Payload{}, err
		}
		//res := &Payload{Data: groups().MembershipUpdateAfter(mm.LastUpdate())}
		return &Payload{}, nil
	}
}
