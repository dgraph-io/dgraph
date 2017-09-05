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
	"fmt"
	"sync"
	"sync/atomic"

	"golang.org/x/net/context"

	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/dgraph/conn"
	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/raftwal"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/x"
)

type server struct {
	NodeId    uint64     // Raft Id associated with the raft node.
	Addr      string     // The public address of the server serving this node.
	Leader    bool       // Set to true if the node is a leader of the group.
	RaftIdx   uint64     // The raft index which applied this membership update in group zero.
	PoolOrNil *conn.Pool // An owned reference to the server's Pool entry (nil if Addr is our own).
}

type servers struct {
	// A map of indices into list, allowing for random access by their NodeId field.
	byNodeID map[uint64]int
	// Servers for the group, as determined by Raft group zero.
	// list[0] is the (last-known) leader of that group.
	list []server
}

type groupi struct {
	x.SafeMutex
	ctx    context.Context
	cancel context.CancelFunc
	wal    *raftwal.Wal
	// local stores the groupId to node map for this server.
	// TODO: Remove the local map. Instead store the RaftServer variable here.
	local map[uint32]*node
	// all stores the groupId to servers map for the entire cluster.
	all        map[uint32]*servers
	num        uint32
	lastUpdate uint64
	// TODO: Also store the tablet -> group mapping.
	// In fact, we could have a common struct to deal with this membership info, so it's shared.
	state *protos.MembershipState
	Node  *node
	gid   uint32
}

var gr *groupi

func groups() *groupi {
	return gr
}

func swapServers(sl *servers, i int, j int) {
	sl.list[i], sl.list[j] = sl.list[j], sl.list[i]
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
	pool := sl.list[back].PoolOrNil
	sl.list = sl.list[:back]
	delete(sl.byNodeID, nodeID)
	if pool != nil {
		conn.Get().Release(pool)
	}
}

func addToServers(sl *servers, update server) {
	back := len(sl.list)
	sl.list = append(sl.list, update)
	sl.byNodeID[update.NodeId] = back
	if update.Leader && back != 0 {
		swapServers(sl, 0, back)
		sl.list[back].Leader = false
	}
}

// StartRaftNodes will read the WAL dir, create the RAFT groups,
// and either start or restart RAFT nodes.
// This function triggers RAFT nodes to be created, and is the entrace to the RAFT
// world from main.go.
func StartRaftNodes(walStore *badger.KV, bindall bool) {
	gr = new(groupi)
	gr.ctx, gr.cancel = context.WithCancel(context.Background())

	// gr.all = make(map[uint32]*servers)
	// gr.local = make(map[uint32]*node)

	if Config.InMemoryComm {
		Config.MyAddr = "inmemory"
	}

	if len(Config.MyAddr) == 0 {
		Config.MyAddr = fmt.Sprintf("localhost:%d", workerPort())
	} else if !Config.InMemoryComm {
		// check if address is valid or not
		ok := x.ValidateAddress(Config.MyAddr)
		x.AssertTruef(ok, "%s is not valid address", Config.MyAddr)
		if !bindall {
			x.Printf("--my flag is provided without bindall, Did you forget to specify bindall?\n")
		}
	}

	// Successfully connect with the peer, before doing anything else.
	if len(Config.PeerAddr) > 0 {
		func() {
			if Config.PeerAddr == Config.MyAddr {
				return
			}
			p := conn.Get().Connect(Config.PeerAddr)
			defer conn.Get().Release(p)

			// Connect with dgraphzero and figure out what group we should belong to.
			zc := protos.NewZeroClient(p.Get())
			var state *protos.MembershipState
			m := &protos.Member{Id: Config.RaftId}
			for i := 0; i < 100; i++ { // Generous number of attempts.
				var err error
				state, err = zc.Connect(gr.ctx, m)
				if err == nil {
					break
				}
				x.Printf("Error while connecting with group zero: %v", err)
			}
			if state == nil {
				x.Fatalf("Unable to join cluster via dgraphzero")
			}
			gr.state = state
		}()
	}

	gr.wal = raftwal.Init(walStore, Config.RaftId)

	gid := gr.groupId()
	gr.Node = newNode(gid, Config.RaftId, Config.MyAddr)
	x.Checkf(schema.LoadFromDb(uint32(gid)), "Error while initilizating schema")
	gr.Node.InitAndStartNode(gr.wal)

	x.UpdateHealthStatus(true)
	// TODO: Run this again.
	// go gr.periodicSyncMemberships() // Now set it to be run periodically.
}

func (g *groupi) groupId() uint32 {
	gid := atomic.LoadUint32(&g.gid)
	if gid > 0 {
		return gid
	}
	for gid, group := range g.state.Groups {
		for _, m := range group.Members {
			if m.Id == Config.RaftId {
				atomic.StoreUint32(&g.gid, gid)
				return gid
			}
		}
	}
	return 0
}

func (g *groupi) ServesGroup(gid uint32) bool {
	g.RLock()
	defer g.RUnlock()
	return g.gid == gid
}

func (g *groupi) ServesTablet(key string) bool {
	g.RLock()
	defer g.RUnlock()

	group := g.state.Groups[g.gid]
	_, has := group.Tablets[key]
	return has
}

// TODO: Don't need this.
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

func (g *groupi) Server(id uint64, groupId uint32) (rs string, found bool) {
	g.RLock()
	defer g.RUnlock()
	sl := g.all[groupId]
	if sl == nil {
		return "", false
	}
	idx, has := sl.byNodeID[id]
	if has {
		return sl.list[idx].Addr, true
	}
	return "", false
}

// Returns 0, 1, or 2 valid server addrs.
func (g *groupi) AnyTwoServers(gid uint32) []string {
	g.RLock()
	defer g.RUnlock()
	group, has := g.state.Groups[gid]
	if !has {
		return []string{}
	}
	var res []string
	for _, m := range group.Members {
		// map iteration gives us members in no particular order.
		res = append(res, m.Addr)
		if len(res) >= 2 {
			break
		}
	}
	return res
}

func (g *groupi) AnyServer(gid uint32) string {
	g.RLock()
	defer g.RUnlock()
	group, has := g.state.Groups[gid]
	if !has {
		return ""
	}
	for _, m := range group.Members {
		return m.Addr
	}
	return ""
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
	return len(all.list) > 1 || (len(all.list) == 1 && all.list[0].NodeId != Config.RaftId)
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

// TODO: Don't need this.
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

// TODO: Bring this back. We need to sync membership periodically.
// In fact, this could be better done via a uni-directional or bi-directional stream, so it's
// instantenous.

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
// func (g *groupi) syncMemberships() {
// 	// This server doesn't serve group zero.
// 	// Generate membership update of all local nodes.
// 	var mu protos.MembershipUpdate
// 	{
// 		g.RLock()
// 		for _, n := range g.local {
// 			rc := n.RaftContext
// 			mu.Members = append(mu.Members,
// 				&protos.Membership{
// 					Leader:  n.AmLeader(),
// 					Id:      rc.Id,
// 					GroupId: rc.Group,
// 					Addr:    rc.Addr,
// 				})
// 		}
// 		mu.LastUpdate = g.lastUpdate
// 		g.RUnlock()
// 	}

// 	// Send an update to peer.
// 	addr := g.AnyServer(0)

// 	var pl *conn.Pool
// 	var err error
// 	if len(addr) > 0 {
// 		pl, err = conn.Get().Get(addr)
// 	} else {
// 		pl, err = conn.Get().Any()
// 	}
// 	if err == conn.ErrNoConnection {
// 		x.Println("Unable to sync memberships. No valid connection")
// 		return
// 	}
// 	x.Check(err)

// 	var update *protos.MembershipUpdate
// 	for {
// 		gconn := pl.Get()
// 		c := protos.NewWorkerClient(gconn)
// 		update, err = c.UpdateMembership(g.ctx, &mu)
// 		conn.Get().Release(pl)

// 		if err != nil {
// 			if tr, ok := trace.FromContext(g.ctx); ok {
// 				tr.LazyPrintf(err.Error())
// 			}
// 			return
// 		}

// 		// Check if we got a redirect.
// 		// TODO: Don't need this redirect portion of logic.
// 		if !update.Redirect {
// 			break
// 		}

// 		addr = update.RedirectAddr
// 		if len(addr) == 0 {
// 			return
// 		}
// 		x.Printf("Got redirect for: %q\n", addr)
// 		if addr == Config.MyAddr {
// 			// We got redirected to ourselves.
// 			return
// 		}
// 		pl = conn.Get().Connect(addr)
// 	}

// 	var lu uint64
// 	for _, mm := range update.Members {
// 		g.applyMembershipUpdate(update.LastUpdate, mm)
// 		if lu < update.LastUpdate {
// 			lu = update.LastUpdate
// 		}
// 	}
// 	g.TouchLastUpdate(lu)
// }

// func (g *groupi) periodicSyncMemberships() {
// 	t := time.NewTicker(10 * time.Second)
// 	for {
// 		select {
// 		case <-t.C:
// 			g.syncMemberships()
// 		case <-g.ctx.Done():
// 			return
// 		}
// 	}
// }

// SyncAllMarks syncs marks of all nodes of the worker group.
// TODO: Don't need this.
func syncAllMarks(ctx context.Context) error {
	numNodes := len(groups().nodes())
	che := make(chan error, numNodes)
	for _, n := range groups().nodes() {
		go func(n *node) {
			// Get index of last committed.
			lastIndex, err := n.Store.LastIndex()
			if err != nil {
				che <- err
				return
			}
			n.syncAllMarks(ctx, lastIndex)
			che <- nil
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
// TODO: Don't need this.
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
// TODO: Don't need this.
func stopAllNodes() {
	for _, n := range groups().nodes() {
		n.Stop()
	}
}
