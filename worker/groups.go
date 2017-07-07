/*
 * Copyright (C) 2017 Dgraph Labs, Inc. and Contributors
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 */

package worker

import (
	"flag"
	"fmt"
	"math/rand"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/badger/table"
	"golang.org/x/net/context"
	"golang.org/x/net/trace"

	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/raftwal"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/x"
)

var (
	groupIds = flag.String("groups", "0,1", "RAFT groups handled by this server.")
	myAddr   = flag.String("my", "",
		"addr:port of this server, so other Dgraph servers can talk to this.")
	peerAddr = flag.String("peer", "", "IP_ADDRESS:PORT of any healthy peer.")
	raftId   = flag.Uint64("idx", 1, "RAFT ID that this server will use to join RAFT groups.")
	// In case of flaky network connectivity we would try to keep upto maxPendingEntries in wal
	// so that the nodes which have lagged behind leader can just replay entries instead of
	// fetching snapshot if network disconnectivity is greater than the interval at which snapshots
	// are taken
	maxPendingCount = flag.Uint64("sc", 1000, "Max number of pending entries in wal after which snapshot is taken")

	emptyMembershipUpdate protos.MembershipUpdate
)

type server struct {
	NodeId  uint64 // Raft Id associated with the raft node.
	Addr    string // The public address of the server serving this node.
	Leader  bool   // Set to true if the node is a leader of the group.
	RaftIdx uint64 // The raft index which applied this membership update in group zero.
}

type servers struct {
	byNodeID map[uint64]int
	list     []server
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

func swapServers(sl *servers, i int, j int) {
	tmp := sl.list[i]
	sl.list[i] = sl.list[j]
	sl.list[j] = tmp
	sl.byNodeID[sl.list[i].NodeId] = i
	sl.byNodeID[sl.list[j].NodeId] = j
}

func removeFromServersIfPresent(sl *servers, nodeID uint64) {
	i, has := sl.byNodeID[nodeID]
	if !has {
		return
	}
	back := len(sl.list) - 1
	swapServers(sl, i, back)
	sl.list = sl.list[:back]
	delete(sl.byNodeID, nodeID)
}

func addToServers(sl *servers, update server) {
	back := len(sl.list)
	sl.list = append(sl.list, update)
	if update.Leader && back != 0 {
		swapServers(sl, 0, back)
		sl.list[back].Leader = false
	}
}

// StartRaftNodes will read the WAL dir, create the RAFT groups,
// and either start or restart RAFT nodes.
// This function triggers RAFT nodes to be created, and is the entrace to the RAFT
// world from main.go.
func StartRaftNodes(walDir string) {
	gr = new(groupi)
	gr.ctx, gr.cancel = context.WithCancel(context.Background())
	gr.all = make(map[uint32]*servers)
	gr.local = make(map[uint32]*node)

	if len(*myAddr) == 0 {
		*myAddr = fmt.Sprintf("localhost:%d", workerPort())
	} else {
		// check if address is valid or not
		ok := x.ValidateAddress(*myAddr)
		x.AssertTruef(ok, "%s is not valid address", *myAddr)
	}

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
	kvOpt := badger.DefaultOptions
	kvOpt.SyncWrites = true
	kvOpt.Dir = walDir
	kvOpt.ValueDir = walDir
	kvOpt.MapTablesTo = table.Nothing
	wals, err := badger.NewKV(&kvOpt)
	x.Checkf(err, "Error while creating badger KV store")
	gr.wal = raftwal.Init(wals, *raftId)

	var wg sync.WaitGroup
	gids, err := getGroupIds(*groupIds)
	x.AssertTruef(err == nil && len(gids) > 0, "Unable to parse 'groups' configuration")

	for _, gid := range gids {
		node := gr.newNode(gid, *raftId, *myAddr)
		x.Checkf(schema.LoadFromDb(uint32(gid)), "Error while initilizating schema")
		wg.Add(1)
		go func() {
			defer wg.Done()
			node.InitAndStartNode(gr.wal)
		}()
	}
	wg.Wait()
	x.UpdateHealthStatus(true)
	go gr.periodicSyncMemberships() // Now set it to be run periodically.
}

func getGroupIds(groups string) ([]uint32, error) {
	parts := strings.Split(groups, ",")
	var gids []uint32
	for _, part := range parts {
		dashCount := strings.Count(part, "-")
		switch dashCount {
		case 0:
			gid, err := strconv.ParseUint(part, 0, 32)
			if err != nil {
				return nil, err
			}
			gids = append(gids, uint32(gid))
		case 1:
			bounds := strings.Split(part, "-")
			min, err := strconv.ParseUint(bounds[0], 0, 32)
			if err != nil {
				return nil, err
			}
			max, err := strconv.ParseUint(bounds[1], 0, 32)
			if err != nil {
				return nil, err
			}
			for i := uint32(min); i <= uint32(max); i++ {
				gids = append(gids, i)
			}
		default:
			return nil, x.Errorf("Invalid group configuration item: %v", part)
		}
	}

	// check for duplicates
	sort.Sort(gidSlice(gids))
	for i := 0; i < len(gids)-1; i++ {
		if gids[i] == gids[i+1] {
			return nil, x.Errorf("Duplicated group id: %v", gids[i])
		}
	}

	return gids, nil
}

// just for sorting
type gidSlice []uint32

func (a gidSlice) Len() int           { return len(a) }
func (a gidSlice) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a gidSlice) Less(i, j int) bool { return a[i] < a[j] }

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
	sl := g.all[groupId]
	if sl == nil {
		return server{}, false
	}
	idx, has := sl.byNodeID[id]
	if has {
		return sl.list[idx], true
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
	out := make([]string, len(all.list))
	for i, s := range all.list {
		out[i] = s.Addr
	}
	return out
}

// Peer returns node(raft) id of the peer of given nodeid of given group
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
	return len(all.list) > 1 || (len(all.list) == 1 && all.list[0].NodeId != *raftId)
}

// Leader will try to return the leader of a given group, based on membership information.
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
	idx, has := sl.byNodeID[nid]
	s := sl.list[idx]
	return has && s.Addr == addr && s.Leader == leader
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

			go func(rc *protos.RaftContext, amleader bool) {
				mm := &protos.Membership{
					Leader:  amleader,
					Id:      rc.Id,
					GroupId: rc.Group,
					Addr:    rc.Addr,
				}
				zero := g.Node(0)
				x.AssertTruef(zero != nil, "Expected node 0")
				if err := zero.ProposeAndWait(zero.ctx, &protos.Proposal{Membership: mm}); err != nil {
					if tr, ok := trace.FromContext(g.ctx); ok {
						tr.LazyPrintf(err.Error())
					}
				}
			}(rc, n.AmLeader())
		}
		return
	}

	// This server doesn't serve group zero.
	// Generate membership update of all local nodes.
	var mu protos.MembershipUpdate
	{
		g.RLock()
		for _, n := range g.local {
			rc := n.raftContext
			mu.Members = append(mu.Members,
				&protos.Membership{
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
	addr := g.AnyServer(0)

UPDATEMEMBERSHIP:
	var pl *pool
	var err error
	if len(addr) > 0 {
		pl, err = pools().get(addr)
	} else {
		pl, err = pools().any()
	}
	if err == errNoConnection {
		fmt.Println("Unable to sync memberships. No valid connection")
		return
	}
	x.Check(err)

	conn := pl.Get()

	c := protos.NewWorkerClient(conn)
	update, err := c.UpdateMembership(g.ctx, &mu)
	if err != nil {
		if tr, ok := trace.FromContext(g.ctx); ok {
			tr.LazyPrintf(err.Error())
		}
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
func (g *groupi) applyMembershipUpdate(raftIdx uint64, mm *protos.Membership) {
	update := server{
		NodeId:  mm.Id,
		Addr:    mm.Addr,
		Leader:  mm.Leader,
		RaftIdx: raftIdx,
	}
	if n := g.Node(mm.GroupId); n != nil {
		// update peer address on address change
		n.Connect(mm.Id, mm.Addr)
		// TODO: Clean up old pools
	} else if update.Addr != *myAddr && mm.Id != *raftId { // ignore previous addr
		go pools().connect(update.Addr)
	}

	fmt.Println("----------------------------")
	fmt.Printf("====== APPLYING MEMBERSHIP UPDATE: %+v\n", update)
	fmt.Println("----------------------------")
	g.Lock()
	defer g.Unlock()

	sl := g.all[mm.GroupId]
	if sl == nil {
		sl = new(servers)
		g.all[mm.GroupId] = sl
	}

	removeFromServersIfPresent(sl, update.NodeId)
	addToServers(sl, update)

	// Print out the entire list.
	for gid, sl := range g.all {
		fmt.Printf("Group: %v. List: %+v\n", gid, sl.list)
	}
}

// MembershipUpdateAfter generates the Flatbuffer response containing all the
// membership updates after the provided raft index.
func (g *groupi) MembershipUpdateAfter(ridx uint64) *protos.MembershipUpdate {
	g.RLock()
	defer g.RUnlock()

	maxIdx := ridx
	out := new(protos.MembershipUpdate)

	for gid, peers := range g.all {
		for _, s := range peers.list {
			if s.RaftIdx <= ridx {
				continue
			}
			if s.RaftIdx > maxIdx {
				maxIdx = s.RaftIdx
			}
			out.Members = append(out.Members,
				&protos.Membership{
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
	update *protos.MembershipUpdate) (*protos.MembershipUpdate, error) {
	if ctx.Err() != nil {
		return &emptyMembershipUpdate, ctx.Err()
	}
	if !groups().ServesGroup(0) {
		addr := groups().AnyServer(0)
		// fmt.Printf("I don't serve group zero. But, here's who does: %v\n", addr)

		return &protos.MembershipUpdate{
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

		mmNew := &protos.Membership{
			Leader:  mm.Leader,
			Id:      mm.Id,
			GroupId: mm.GroupId,
			Addr:    mm.Addr,
		}

		go func(mmNew *protos.Membership) {
			zero := groups().Node(0)
			che <- zero.ProposeAndWait(zero.ctx, &protos.Proposal{Membership: mmNew})
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
	numNodes := len(groups().nodes())
	che := make(chan error, numNodes)
	for _, n := range groups().nodes() {
		go func(n *node) {
			// Get index of last committed.
			lastIndex, err := n.store.LastIndex()
			if err != nil {
				che <- err
				return
			}
			err = n.syncAllMarks(ctx, lastIndex)
			che <- err
		}(n)
	}

	var finalErr error
	for i := 0; i < numNodes; i++ {
		if e := <-che; e != nil {
			finalErr = e
		}
	}
	return finalErr
}

// snapshotAll takes snapshot of all nodes of the worker group
func snapshotAll() {
	var wg sync.WaitGroup
	for _, n := range groups().nodes() {
		wg.Add(1)
		go func(n *node) {
			defer wg.Done()
			n.snapshot(0)
		}(n)
	}
	wg.Wait()
}

// StopAllNodes stops all the nodes of the worker group.
func stopAllNodes() {
	for _, n := range groups().nodes() {
		n.Stop()
	}
}
