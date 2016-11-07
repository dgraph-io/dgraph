package worker

import (
	"bytes"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

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
	peer           = flag.String("peer", "", "Address of any peer.")
	raftId         = flag.Uint64("idx", 1, "RAFT ID that this server will use to join RAFT groups.")
	redirectPrefix = []byte("REDIRECT:")
)

type server struct {
	NodeId  uint64 // Raft Id associated with the raft node.
	Addr    string // The public address of the server serving this node.
	Leader  bool   // Set to true if the node is a leader of the group.
	RaftIdx uint64 // The raft index which applied this membership update in group zero.
}

type servers struct {
	list []server
}

type groupi struct {
	sync.RWMutex
	ctx    context.Context
	cancel context.CancelFunc
	wal    *raftwal.Wal
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
// This function triggers RAFT nodes to be created.
func StartRaftNodes(walDir string) {
	gr = new(groupi)
	gr.ctx, gr.cancel = context.WithCancel(context.Background())

	// Successfully connect with the peer, before doing anything else.
	if len(*peer) > 0 {
		_, paddr := parsePeer(*peer)
		pools().connect(paddr)

		// Force run syncMemberships with this peer, so our nodes know if they have other
		// servers who are serving the same groups. That way, they can talk to them
		// and try to join their clusters. Otherwise, they'll start off as a single-node
		// cluster.
		// IMPORTANT: Don't run any nodes until we have done at least one full sync for membership
		// information with the cluster. If you start this node with peer too quickly, just
		// after starting the leader of group zero, that node might not have updated
		// itself in the memberships; and hence this node would think that noone is handling
		// group zero. Therefore, we MUST wait to get pass a last update raft index of zero.
		for gr.LastUpdate() == 0 {
			fmt.Println("Last update raft index for membership information is zero. Syncing...")
			gr.syncMemberships()
			time.Sleep(time.Second)
		}
		fmt.Printf("Last update is now: %d\n", gr.LastUpdate())
	}
	go gr.periodicSyncMemberships() // Now set it to be run periodically.

	x.Check(ParseGroupConfig(*groupConf))
	x.Checkf(os.MkdirAll(walDir, 0700), "Error while creating WAL dir.")
	wals, err := store.NewSyncStore(walDir)
	x.Checkf(err, "Error initializing wal store")
	gr.wal = raftwal.Init(wals, *raftId)

	if len(*myAddr) == 0 {
		*myAddr = fmt.Sprintf("localhost:%d", *workerPort)
	}

	for _, id := range strings.Split(*groupIds, ",") {
		gid, err := strconv.ParseUint(id, 0, 32)
		x.Checkf(err, "Unable to parse group id: %v", id)
		node := groups().newNode(uint32(gid), *raftId, *myAddr)
		go node.InitAndStartNode(gr.wal)
	}
}

func (g *groupi) Node(groupId uint32) *node {
	g.RLock()
	defer g.RUnlock()
	if n, has := g.local[groupId]; has {
		return n
	}
	return nil
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
		if s.NodeId == id {
			return s, true
		}
	}
	return server{}, false
}

func (g *groupi) AnyServer(group uint32) string {
	g.RLock()
	defer g.RUnlock()
	all := g.all[group]
	if all == nil {
		return ""
	}
	sz := len(all.list)
	idx := rand.Intn(sz)
	return all.list[idx].Addr
}

func (g *groupi) HasPeer(group uint32) bool {
	g.RLock()
	defer g.RUnlock()
	all := g.all[group]
	if all == nil {
		return false
	}
	return len(all.list) > 0
}

// Leader will try to retrun the leader of a given group, based on membership information.
// There is currently no guarantee that the returned server is the leader of the group.
func (g *groupi) Leader(group uint32) (uint64, string) {
	g.RLock()
	defer g.RUnlock()
	all := g.all[group]
	if all == nil {
		return 0, ""
	}
	return all.list[0].NodeId, all.list[0].Addr
}

func (g *groupi) KnownGroups() (gids []uint32) {
	g.RLock()
	defer g.RUnlock()
	for gid := range g.all {
		gids = append(gids, gid)
	}
	return
}

func (g *groupi) isDuplicate(gid uint32, nid uint64, addr string, leader bool) bool {
	g.RLock()
	defer g.RUnlock()
	return g.duplicate(gid, nid, addr, leader)
}

// duplicate requires at least a read mutex lock to be held by the caller.
// duplicate will return true if we already have a server which matches the arguments
// provided to the function exactly. This is used to avoid re-applying the same update.
func (g *groupi) duplicate(gid uint32, nid uint64, addr string, leader bool) bool {
	sl := g.all[gid]
	if sl == nil {
		return false
	}
	for _, s := range sl.list {
		if s.NodeId == nid && s.Addr == addr && s.Leader == leader {
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

func (g *groupi) TouchLastUpdate(u uint64) {
	g.Lock()
	defer g.Unlock()
	if g.lastUpdate < u {
		g.lastUpdate = u
	}
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

// syncMemberships needs to be called in an periodic loop.
// How syncMemberships works:
// - Each server iterates over all the nodes it's serving, present in local.
// - If serving group zero, propose membership status updates directly via RAFT.
// - Otherwise, generates a membership update, which includes status of all serving nodes.
// - Check if it has address of a server from group zero. If so, use that.
// - Otherwise, use the peer information passed down via flags.
// - Send update via UpdateMembership call to the peer.
// - If the peer doesn't serve group zero, it would return back a redirect with the right address.
// - Otherwise, it would iterate over the memberships, check for duplicates, and apply updates.
// - Once iteration is over without errors, it would return back all new updates.
// - These updates are then applied to groups().all state via applyMembershipUpdate.
func (g *groupi) syncMemberships() {
	if g.ServesGroup(0) {
		// This server serves group zero.
		g.RLock()
		for _, n := range g.local {
			rc := task.GetRootAsRaftContext(n.raftContext, 0)
			if g.duplicate(rc.Group(), rc.Id(), string(rc.Addr()), n.AmLeader()) {
				continue
			}

			go func(rc *task.RaftContext, amleader bool) {
				b := flatbuffers.NewBuilder(0)
				uo := buildTaskMembership(b, rc.Group(), rc.Id(), rc.Addr(), amleader)
				b.Finish(uo)
				data := b.FinishedBytes()

				zero := groups().Node(0)
				x.Check(zero.ProposeAndWait(zero.ctx, membershipMsg, data))

			}(rc, n.AmLeader())
		}
		g.RUnlock()
		return
	}

	// This server doesn't serve group zero.
	// Generate membership update of all local nodes.
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
	x.AssertTrue(len(data) > 0)

	// Send an update to peer.
	var pl *pool
	addr := g.AnyServer(0)

UPDATEMEMBERSHIP:
	if len(addr) > 0 {
		pl = pools().get(addr)
	} else {
		pl = pools().any()
	}
	conn, err := pl.Get()
	if err == errNoConnection {
		fmt.Println("Unable to sync memberships. No valid connection")
		return
	}
	x.Check(err)
	defer pl.Put(conn)

	c := NewWorkerClient(conn)
	p := &Payload{Data: data}
	out, err := c.UpdateMembership(g.ctx, p)
	if err != nil {
		x.TraceError(g.ctx, err)
		return
	}

	// Check if we got a redirect.
	if len(out.Data) >= len(redirectPrefix) &&
		bytes.Equal(out.Data[0:len(redirectPrefix)], redirectPrefix) {

		addr = string(out.Data[len(redirectPrefix):])
		if len(addr) == 0 {
			return
		}
		fmt.Printf("Got redirect for: %q\n", addr)
		pools().connect(addr)
		goto UPDATEMEMBERSHIP
	}

	var lu uint64
	update := task.GetRootAsMembershipUpdate(out.Data, 0)
	for i := 0; i < update.MembersLength(); i++ {
		mm := new(task.Membership)
		x.AssertTrue(update.Members(mm, i))
		g.applyMembershipUpdate(update.LastUpdate(), mm)
		if lu < update.LastUpdate() {
			lu = update.LastUpdate()
		}
	}
	g.TouchLastUpdate(lu)
}

func (g *groupi) periodicSyncMemberships() {
	t := time.NewTicker(10 * time.Second)
	for {
		select {
		case tm := <-t.C:
			fmt.Printf("%v: Syncing memberships\n", tm)
			g.syncMemberships()
		case <-g.ctx.Done():
			return
		}
	}
}

// raftIdx is the RAFT index corresponding to the application of this
// membership update in group zero.
func (g *groupi) applyMembershipUpdate(raftIdx uint64, mm *task.Membership) {
	update := server{
		NodeId:  mm.Id(),
		Addr:    string(mm.Addr()),
		Leader:  mm.Leader() == byte(1),
		RaftIdx: raftIdx,
	}
	if update.Addr != *myAddr {
		go pools().connect(update.Addr)
	}

	fmt.Println("----------------------------")
	fmt.Printf("====== APPLYING MEMBERSHIP UPDATE: %+v\n", update)
	fmt.Println("----------------------------")
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
			if s.NodeId == update.NodeId {
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

	// Print out the entire list.
	for gid, sl := range g.all {
		fmt.Printf("Group: %v. List: %+v\n", gid, sl.list)
	}
}

// MembershipUpdateAfter generates the Flatbuffer response containing all the
// membership updates after the provided raft index.
func (g *groupi) MembershipUpdateAfter(ridx uint64) []byte {
	g.RLock()
	defer g.RUnlock()

	maxIdx := ridx
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
			uo := buildTaskMembership(b, gid, s.NodeId, []byte(s.Addr), s.Leader)
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

// UpdateMembership is the RPC call for updating membership for servers
// which don't serve group zero.
func (w *grpcWorker) UpdateMembership(ctx context.Context, query *Payload) (*Payload, error) {
	if ctx.Err() != nil {
		return &Payload{}, ctx.Err()
	}
	if !groups().ServesGroup(0) {
		addr := groups().AnyServer(0)
		// fmt.Printf("I don't serve group zero. But, here's who does: %v\n", addr)
		p := new(Payload)
		p.Data = append(p.Data, redirectPrefix...)
		p.Data = append(p.Data, []byte(addr)...)
		return p, nil
	}

	update := task.GetRootAsMembershipUpdate(query.Data, 0)
	che := make(chan error, update.MembersLength())

	for i := 0; i < update.MembersLength(); i++ {
		mm := new(task.Membership)
		x.AssertTrue(update.Members(mm, i))
		if groups().isDuplicate(mm.Group(), mm.Id(), string(mm.Addr()), mm.Leader() == 1) {
			che <- nil
			continue
		}

		b := flatbuffers.NewBuilder(0)
		uo := buildTaskMembership(b, mm.Group(), mm.Id(), mm.Addr(), mm.Leader() == 1)
		b.Finish(uo)
		data := b.FinishedBytes()

		go func(data []byte) {
			zero := groups().Node(0)
			che <- zero.ProposeAndWait(zero.ctx, membershipMsg, data)
		}(data)
	}

	for i := 0; i < update.MembersLength(); i++ {
		select {
		case <-ctx.Done():
			return &Payload{}, ctx.Err()
		case err := <-che:
			if err != nil {
				return &Payload{}, err
			}
		}
	}

	// Find all membership updates since the provided lastUpdate. LastUpdate is
	// the last raft index that the caller has recorded an update for.
	reply := groups().MembershipUpdateAfter(update.LastUpdate())
	return &Payload{Data: reply}, nil
}
