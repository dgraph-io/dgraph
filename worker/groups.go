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
	"sync/atomic"
	"time"

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
	// TODO: Is this context being used?
	ctx     context.Context
	cancel  context.CancelFunc
	wal     *raftwal.Wal
	state   *protos.MembershipState
	Node    *node
	gid     uint32
	tablets map[string]uint32
}

var gr *groupi

func groups() *groupi {
	return gr
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

	if Config.InMemoryComm {
		gr.state = &protos.MembershipState{}
		atomic.StoreUint32(&gr.gid, 1)

	} else {
		x.AssertTruef(len(Config.PeerAddr) > 0, "Providing dgraphzero address is mandatory.")
		x.AssertTruef(Config.PeerAddr != Config.MyAddr,
			"Dgraphzero address and Dgraph address can't be the same.")

		// Successfully connect with dgraphzero, before doing anything else.
		p := conn.Get().Connect(Config.PeerAddr)

		// Connect with dgraphzero and figure out what group we should belong to.
		zc := protos.NewZeroClient(p.Get())
		var state *protos.MembershipState
		m := &protos.Member{Id: Config.RaftId, Addr: Config.MyAddr}
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
		x.Printf("Connected to group zero. State: %+v\n", state)
		gr.applyState(state)
	}

	gr.wal = raftwal.Init(walStore, Config.RaftId)
	gid := gr.groupId()
	gr.Node = newNode(gid, Config.RaftId, Config.MyAddr)
	x.Checkf(schema.LoadFromDb(), "Error while initilizating schema")
	gr.Node.InitAndStartNode(gr.wal)

	x.UpdateHealthStatus(true)
	if !Config.InMemoryComm {
		go gr.periodicSyncMemberships() // Now set it to be run periodically.
	}
}

// No locks are acquired while accessing this function.
// Don't acquire RW lock during this, otherwise we might deadlock.
func (g *groupi) groupId() uint32 {
	return atomic.LoadUint32(&g.gid)
}

func (g *groupi) calculateTabletSizes() map[string]*protos.Tablet {
	// Only calculate these, if I'm the leader.
	if !g.Node.AmLeader() {
		return nil
	}

	opt := badger.DefaultIteratorOptions
	opt.PrefetchValues = false
	itr := pstore.NewIterator(opt)
	defer itr.Close()

	gid := g.groupId()
	tablets := make(map[string]*protos.Tablet)

	for itr.Rewind(); itr.Valid(); {
		item := itr.Item()

		pk := x.Parse(item.Key())
		if pk.IsSchema() {
			itr.Seek(pk.SkipSchema())
			continue
		}

		tablet, has := tablets[pk.Attr]
		if !has {
			if !g.ServesTablet(pk.Attr) {
				itr.Seek(pk.SkipPredicate())
				continue
			}
			tablet = &protos.Tablet{GroupId: gid, Predicate: pk.Attr}
			tablets[pk.Attr] = tablet
		}
		tablet.Size_ += item.EstimatedSize()
		itr.Next()
	}
	return tablets
}

func (g *groupi) applyState(state *protos.MembershipState) {
	x.AssertTrue(state != nil)
	g.Lock()
	defer g.Unlock()

	g.state = state
	g.tablets = make(map[string]uint32)
	for gid, group := range g.state.Groups {
		for _, member := range group.Members {
			if Config.RaftId == member.Id {
				atomic.StoreUint32(&g.gid, gid)
			}
			if Config.MyAddr != member.Addr {
				go conn.Get().Connect(member.Addr)
			}
		}
		for _, tablet := range group.Tablets {
			g.tablets[tablet.Predicate] = tablet.GroupId
		}
	}
	for _, member := range g.state.Zeros {
		if Config.MyAddr != member.Addr {
			go conn.Get().Connect(member.Addr)
		}
	}
}

func (g *groupi) ServesGroup(gid uint32) bool {
	g.RLock()
	defer g.RUnlock()
	return g.gid == gid
}

func (g *groupi) BelongsTo(key string) uint32 {
	g.RLock()
	gid := g.tablets[key]
	g.RUnlock()

	if gid > 0 {
		return gid
	}
	if g.ServesTablet(key) {
		return g.groupId()
	}
	g.RLock()
	gid = g.tablets[key]
	g.RUnlock()
	return gid
}

func (g *groupi) ServesTablet(key string) bool {
	if Config.InMemoryComm {
		return true
	}
	g.RLock()
	gid := g.tablets[key]
	g.RUnlock()
	if gid > 0 {
		return gid == g.groupId()
	}

	fmt.Printf("Asking if I serve tablet: %v\n", key)
	// We don't know about this tablet.
	// Check with dgraphzero if we can serve it.
	pl := g.AnyServer(0)
	if pl == nil {
		return false
	}
	zc := protos.NewZeroClient(pl.Get())

	tablet := &protos.Tablet{GroupId: g.groupId(), Predicate: key}
	out, err := zc.ShouldServe(context.Background(), tablet)
	if err != nil {
		return false
	}
	g.Lock()
	g.tablets[key] = out.GroupId
	g.Unlock()
	return out.GroupId == g.groupId()
}

func (g *groupi) HasMeInState() bool {
	g.RLock()
	defer g.RUnlock()

	group, has := g.state.Groups[g.groupId()]
	if !has {
		return false
	}
	_, has = group.Members[g.Node.Id]
	return has
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

func (g *groupi) members(gid uint32) map[uint64]*protos.Member {
	g.RLock()
	defer g.RUnlock()

	if gid == 0 {
		return g.state.Zeros
	}
	group, has := g.state.Groups[gid]
	if !has {
		return nil
	}
	return group.Members
}

func (g *groupi) AnyServer(gid uint32) *conn.Pool {
	members := g.members(gid)
	if members != nil {
		for _, m := range members {
			pl, err := conn.Get().Get(m.Addr)
			if err == nil {
				return pl
			}
		}
	}
	return nil
}

func (g *groupi) MyPeer() (uint64, bool) {
	members := g.members(g.groupId())
	if members != nil {
		for _, m := range members {
			if m.Id != g.Node.Id {
				return m.Id, true
			}
		}
	}
	return 0, false
}

// Leader will try to return the leader of a given group, based on membership information.
// There is currently no guarantee that the returned server is the leader of the group.
func (g *groupi) Leader(gid uint32) *conn.Pool {
	members := g.members(gid)
	if members == nil {
		return nil
	}
	for _, m := range members {
		if m.Leader {
			if pl, err := conn.Get().Get(m.Addr); err == nil {
				return pl
			}
		}
	}
	// Unable to find a healthy connection to leader. Get connection to any other server in the
	// group.
	return g.AnyServer(gid)
}

func (g *groupi) KnownGroups() (gids []uint32) {
	g.RLock()
	defer g.RUnlock()
	for gid := range g.state.Groups {
		gids = append(gids, gid)
	}
	return
}

// TODO: This could be better done via a uni-directional or bi-directional stream, so it's
// instantenous.
// If node is the leader, every sync involves calculating tablet sizes and sending them to
// dgraphzero.
func (g *groupi) syncMembershipState() {
	// TODO: Instead of getting an address first, then finding a connection to that address,
	// we should pick up a healthy connection from any server in the provided group.
	// This way, if a server goes down, AnyServer can avoid giving a connection to that server.
	pl := g.AnyServer(0)
	// We should always have some connection to dgraphzero.
	if pl == nil {
		x.Printf("WARNING: We don't have address of any dgraphzero server.")
		return
	}

	member := &protos.Member{
		Id:      Config.RaftId,
		GroupId: g.groupId(),
		Addr:    Config.MyAddr,
		Leader:  g.Node.AmLeader(),
	}
	group := &protos.Group{
		Members: make(map[uint64]*protos.Member),
	}
	group.Members[member.Id] = member
	group.Tablets = g.calculateTabletSizes()

	c := protos.NewZeroClient(pl.Get())
	state, err := c.Update(context.Background(), group)

	if err != nil || state == nil {
		x.Printf("Unable to sync memberships. Error: %v", err)
		return
	}
	// fmt.Printf("Got a updated state: %v\n", state)
	g.applyState(state)
}

func (g *groupi) periodicSyncMemberships() {
	t := time.NewTicker(time.Minute)
	// TODO: We don't need to send membership information every 10 seconds, if we get a stream of
	// MembershipState from dgraphzero. That way, we'll have the latest state update.
	for {
		select {
		case <-t.C:
			g.syncMembershipState()
		case <-g.ctx.Done():
			return
		}
	}
}

// SyncAllMarks syncs marks of all nodes of the worker group.
func syncAllMarks(ctx context.Context) error {
	n := groups().Node
	lastIndex, err := n.Store.LastIndex()
	if err != nil {
		return err
	}
	n.syncAllMarks(ctx, lastIndex)
	return nil
}
