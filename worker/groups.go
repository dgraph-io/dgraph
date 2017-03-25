package worker

import (
	"flag"
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/net/context"

	"github.com/dgraph-io/dgraph/protos/taskp"
	"github.com/dgraph-io/dgraph/protos/workerp"
	"github.com/dgraph-io/dgraph/raftwal"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/store"
	"github.com/dgraph-io/dgraph/x"
)

var (
	groupIds = flag.String("groups", "0,1", "RAFT groups handled by this server.")
	myAddr   = flag.String("my", "",
		"addr:port of this server, so other Dgraph servers can talk to this.")
	peerAddr    = flag.String("peer", "", "IP_ADDRESS:PORT of any healthy peer.")
	raftId      = flag.Uint64("idx", 1, "RAFT ID that this server will use to join RAFT groups.")
	schemaFile  = flag.String("schema", "", "Path to schema file")
	healthCheck uint32

	emptyMembershipUpdate taskp.MembershipUpdate
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
	x.SafeMutex
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
// This function triggers RAFT nodes to be created, and is the entrace to the RAFT
// world from main.go.
func StartRaftNodes(walDir string) {
	gr = new(groupi)
	gr.ctx, gr.cancel = context.WithCancel(context.Background())

	// Successfully connect with the peer, before doing anything else.
	if len(*peerAddr) > 0 {
		pools().connect(*peerAddr)

		// Force run syncMemberships with this peer, so our nodes know if they have other
		// servers who are serving the same groups. That way, they can talk to them
		// and try to join their clusters. Otherwise, they'll start off as a single-node
		// cluster.
		// IMPORTANT: Don't run any nodes until we have done at least one full sync for membership
		// information with the cluster. If you start this node too quickly, just
		// after starting the leader of group zero, that leader might not have updated
		// itself in the memberships; and hence this node would think that no one is handling
		// group zero. Therefore, we MUST wait to get pass a last update raft index of zero.
		gr.syncMemberships()
		for gr.LastUpdate() == 0 {
			time.Sleep(time.Second)
			fmt.Println("Last update raft index for membership information is zero. Syncing...")
			gr.syncMemberships()
		}
		fmt.Printf("Last update is now: %d\n", gr.LastUpdate())
	}

	x.Checkf(os.MkdirAll(walDir, 0700), "Error while creating WAL dir.")
	wals, err := store.NewSyncStore(walDir)
	x.Checkf(err, "Error initializing wal store")
	gr.wal = raftwal.Init(wals, *raftId)

	if len(*myAddr) == 0 {
		*myAddr = fmt.Sprintf("localhost:%d", *workerPort)
	}

	var wg sync.WaitGroup
	for _, id := range strings.Split(*groupIds, ",") {
		gid, err := strconv.ParseUint(id, 0, 32)
		x.Checkf(err, "Unable to parse group id: %v", id)
		node := gr.newNode(uint32(gid), *raftId, *myAddr)
		schema.ReloadData(*schemaFile, uint32(gid))
		wg.Add(1)
		go node.InitAndStartNode(gr.wal, &wg)
	}
	wg.Wait()
	atomic.StoreUint32(&healthCheck, 1)
	go gr.periodicSyncMemberships() // Now set it to be run periodically.
}

// HealthCheck returns whether the server is ready to accept requests or not
// Load balancer would add the node to the endpoint once health check starts
// returning true
func HealthCheck() bool {
	return atomic.LoadUint32(&healthCheck) != 0
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

// Servers return addresses of all servers in group.
func (g *groupi) Servers(group uint32) []string {
	g.RLock()
	defer g.RUnlock()
	all := g.all[group]
	if all == nil {
		return nil
	}
	out := make([]string, 0, len(all.list))
	for _, s := range all.list {
		out = append(out, s.Addr)
	}
	return out
}

// Servers return addresses of all servers in group.
func (g *groupi) Peer(group uint32, nodeId uint64) (uint64, bool) {
	g.RLock()
	defer g.RUnlock()
	all := g.all[group]
	if all == nil {
		return 0, false
	}
	for _, s := range all.list {
		if s.NodeId != nodeId {
			return s.NodeId, true
		}
	}
	return 0, false
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
	// If we start a single node cluster without group zero
	if len(gids) == 0 {
		for gid := range g.local {
			gids = append(gids, gid)
		}
	}
	return
}

func (g *groupi) nodes() (nodes []*node) {
	g.RLock()
	defer g.RUnlock()
	for _, n := range g.local {
		nodes = append(nodes, n)
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
	g.AssertRLock()
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
		defer g.RUnlock()
		for _, n := range g.local {
			rc := n.raftContext
			if g.duplicate(rc.Group, rc.Id, rc.Addr, n.AmLeader()) {
				continue
			}

			go func(rc *taskp.RaftContext, amleader bool) {
				mm := &taskp.Membership{
					Leader:  amleader,
					Id:      rc.Id,
					GroupId: rc.Group,
					Addr:    rc.Addr,
				}
				zero := g.Node(0)
				x.AssertTruef(zero != nil, "Expected node 0")
				if err := zero.ProposeAndWait(zero.ctx, &taskp.Proposal{Membership: mm}); err != nil {
					x.TraceError(g.ctx, err)
				}
			}(rc, n.AmLeader())
		}
		return
	}

	// This server doesn't serve group zero.
	// Generate membership update of all local nodes.
	var mu taskp.MembershipUpdate
	{
		g.RLock()
		for _, n := range g.local {
			rc := n.raftContext
			mu.Members = append(mu.Members,
				&taskp.Membership{
					Leader:  n.AmLeader(),
					Id:      rc.Id,
					GroupId: rc.Group,
					Addr:    rc.Addr,
				})
		}
		mu.LastUpdate = g.lastUpdate
		g.RUnlock()
	}

	// Send an update to peer.
	var pl *pool
	_, addr := g.Leader(0)

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

	c := workerp.NewWorkerClient(conn)
	update, err := c.UpdateMembership(g.ctx, &mu)
	if err != nil {
		x.TraceError(g.ctx, err)
		return
	}

	// Check if we got a redirect.
	if update.Redirect {
		addr = update.RedirectAddr
		if len(addr) == 0 {
			return
		}
		fmt.Printf("Got redirect for: %q\n", addr)
		pools().connect(addr)
		goto UPDATEMEMBERSHIP
	}

	var lu uint64
	for _, mm := range update.Members {
		g.applyMembershipUpdate(update.LastUpdate, mm)
		if lu < update.LastUpdate {
			lu = update.LastUpdate
		}
	}
	g.TouchLastUpdate(lu)
}

func (g *groupi) periodicSyncMemberships() {
	t := time.NewTicker(10 * time.Second)
	for {
		select {
		case <-t.C:
			g.syncMemberships()
		case <-g.ctx.Done():
			return
		}
	}
}

// raftIdx is the RAFT index corresponding to the application of this
// membership update in group zero.
func (g *groupi) applyMembershipUpdate(raftIdx uint64, mm *taskp.Membership) {
	update := server{
		NodeId:  mm.Id,
		Addr:    mm.Addr,
		Leader:  mm.Leader,
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

	sl := g.all[mm.GroupId]
	if sl == nil {
		sl = new(servers)
		g.all[mm.GroupId] = sl
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
func (g *groupi) MembershipUpdateAfter(ridx uint64) *taskp.MembershipUpdate {
	g.RLock()
	defer g.RUnlock()

	maxIdx := ridx
	out := new(taskp.MembershipUpdate)

	for gid, peers := range g.all {
		for _, s := range peers.list {
			if s.RaftIdx <= ridx {
				continue
			}
			if s.RaftIdx > maxIdx {
				maxIdx = s.RaftIdx
			}
			out.Members = append(out.Members,
				&taskp.Membership{
					Leader:  s.Leader,
					Id:      s.NodeId,
					GroupId: gid,
					Addr:    s.Addr,
				})
		}
	}

	out.LastUpdate = maxIdx
	return out
}

// UpdateMembership is the RPC call for updating membership for servers
// which don't serve group zero.
func (w *grpcWorker) UpdateMembership(ctx context.Context,
	update *taskp.MembershipUpdate) (*taskp.MembershipUpdate, error) {
	if ctx.Err() != nil {
		return &emptyMembershipUpdate, ctx.Err()
	}
	if !groups().ServesGroup(0) {
		_, addr := groups().Leader(0)
		// fmt.Printf("I don't serve group zero. But, here's who does: %v\n", addr)

		return &taskp.MembershipUpdate{
			Redirect:     true,
			RedirectAddr: addr,
		}, nil
	}

	che := make(chan error, len(update.Members))
	for _, mm := range update.Members {
		if groups().isDuplicate(mm.GroupId, mm.Id, mm.Addr, mm.Leader) {
			che <- nil
			continue
		}

		mmNew := &taskp.Membership{
			Leader:  mm.Leader,
			Id:      mm.Id,
			GroupId: mm.GroupId,
			Addr:    mm.Addr,
		}

		go func(mmNew *taskp.Membership) {
			zero := groups().Node(0)
			che <- zero.ProposeAndWait(zero.ctx, &taskp.Proposal{Membership: mmNew})
		}(mmNew)
	}

	for range update.Members {
		select {
		case <-ctx.Done():
			return &emptyMembershipUpdate, ctx.Err()
		case err := <-che:
			if err != nil {
				return &emptyMembershipUpdate, err
			}
		}
	}

	// Find all membership updates since the provided lastUpdate. LastUpdate is
	// the last raft index that the caller has recorded an update for.
	reply := groups().MembershipUpdateAfter(update.LastUpdate)
	return reply, nil
}

// SyncAllMarks syncs marks of all nodes of the worker group.
func syncAllMarks(ctx context.Context) error {
	var wg sync.WaitGroup
	var err error
	for _, n := range groups().nodes() {
		wg.Add(1)
		go func(n *node) {
			defer wg.Done()
			if e := n.syncAllMarks(ctx); e != nil && err == nil {
				err = e
			}
		}(n)
	}
	wg.Wait()
	return err
}

// StopAllNodes stops all the nodes of the worker group.
func stopAllNodes() {
	for _, n := range groups().nodes() {
		n.Stop()
	}
}
