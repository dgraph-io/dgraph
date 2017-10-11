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

	"google.golang.org/grpc"

	"golang.org/x/net/context"

	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/dgraph/conn"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/raftwal"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
)

type server struct {
	NodeId    uint64     // Raft Id associated with the raft node.
	Addr      string     // The public address of the server serving this node.
	Leader    bool       // Set to true if the node is a leader of the group.
	RaftIdx   uint64     // The raft index which applied this membership update in group zero.
	PoolOrNil *conn.Pool // An owned reference to the server's Pool entry (nil if Addr is our own).
}

type groupi struct {
	x.SafeMutex
	// TODO: Is this context being used?
	ctx       context.Context
	cancel    context.CancelFunc
	wal       *raftwal.Wal
	state     *protos.MembershipState
	Node      *node
	gid       uint32
	tablets   map[string]*protos.Tablet
	triggerCh chan struct{} // Used to trigger membership sync
	delPred   chan struct{} // Ensures that predicate move doesn't happen when deletion is ongoing.
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
		inMemoryTablet = &protos.Tablet{GroupId: gr.groupId()}
	} else {
		x.AssertTruefNoTrace(len(Config.PeerAddr) > 0, "Providing dgraphzero address is mandatory.")
		x.AssertTruefNoTrace(Config.PeerAddr != Config.MyAddr,
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
	gr.triggerCh = make(chan struct{}, 1)
	gr.delPred = make(chan struct{}, 1)
	gid := gr.groupId()
	gr.Node = newNode(gid, Config.RaftId, Config.MyAddr)
	x.Checkf(schema.LoadFromDb(), "Error while initializing schema")
	raftServer.Node = gr.Node.Node
	gr.Node.InitAndStartNode(gr.wal)

	x.UpdateHealthStatus(true)
	if !Config.InMemoryComm {
		go gr.periodicMembershipUpdate() // Now set it to be run periodically.
		go gr.cleanupTablets()
	}
	gr.proposeInitialSchema()
}

func (g *groupi) proposeInitialSchema() {
	g.RLock()
	_, ok := g.tablets[x.PredicateListAttr]
	g.RUnlock()
	if ok {
		return
	}

	// Propose schema mutation.
	var m protos.Mutations
	m.Schema = append(m.Schema, &protos.SchemaUpdate{
		Predicate: x.PredicateListAttr,
		ValueType: uint32(types.StringID),
		List:      true,
	})

	// This would propose the schema mutation and make sure some node serves this predicate
	// and has the schema defined above.

	// Could get an error if some other node asked first and got to serve it.
	if err := MutateOverNetwork(gr.ctx, &m); err != nil {
		if errStr := grpc.ErrorDesc(err); len(errStr) > 0 &&
			errStr != x.ErrTabletAlreadyServed.Error() {
			x.Checkf(err, "Got error while proposing initial schema")
		}
	}
}

// No locks are acquired while accessing this function.
// Don't acquire RW lock during this, otherwise we might deadlock.
func (g *groupi) groupId() uint32 {
	return atomic.LoadUint32(&g.gid)
}

func (g *groupi) calculateTabletSizes() map[string]*protos.Tablet {
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
		tablet.Space += item.EstimatedSize()
		itr.Next()
	}
	return tablets
}

func (g *groupi) applyState(state *protos.MembershipState) {
	x.AssertTrue(state != nil)
	g.Lock()
	defer g.Unlock()
	if g.state != nil && g.state.Counter >= state.Counter {
		return
	}

	g.state = state
	g.tablets = make(map[string]*protos.Tablet)
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
			g.tablets[tablet.Predicate] = tablet
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
	if Config.InMemoryComm {
		return g.groupId()
	}
	g.RLock()
	tablet, ok := g.tablets[key]
	g.RUnlock()

	if ok {
		return tablet.GroupId
	}
	tablet = g.Tablet(key)
	if tablet != nil {
		return tablet.GroupId
	}
	return 0
}

var inMemoryTablet *protos.Tablet

func (g *groupi) ServesTablet(key string) bool {
	tablet := g.Tablet(key)
	if tablet != nil && tablet.GroupId == groups().groupId() {
		return true
	}
	return false
}

// Do not modify the returned Tablet
func (g *groupi) Tablet(key string) *protos.Tablet {
	// TODO: Remove all this later, create a membership state and apply it
	if Config.InMemoryComm {
		return inMemoryTablet
	}
	g.RLock()
	tablet, ok := g.tablets[key]
	g.RUnlock()
	if ok {
		return tablet
	}

	fmt.Printf("Asking if I serve tablet: %v\n", key)
	// We don't know about this tablet.
	// Check with dgraphzero if we can serve it.
	pl := g.AnyServer(0)
	if pl == nil {
		return nil
	}
	zc := protos.NewZeroClient(pl.Get())

	tablet = &protos.Tablet{GroupId: g.groupId(), Predicate: key}
	out, err := zc.ShouldServe(context.Background(), tablet)
	if err != nil {
		x.Printf("Error while ShouldServe grpc call %v", err)
		return nil
	}
	g.Lock()
	g.tablets[key] = out
	g.Unlock()
	return out
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

func (g *groupi) triggerMembershipSync() {
	// It's ok if we miss the trigger, periodic membership sync runs every minute.
	select {
	case g.triggerCh <- struct{}{}:
	// It's ok to ignore it, since we would be sending update of a later state
	default:
	}
}

func (g *groupi) periodicMembershipUpdate() {
	ticker := time.NewTicker(time.Minute * 5)
	// Node might not be the leader when we are calculating size.
	// We need to send immediately on start so no leader check inside calculatesize.
	tablets := g.calculateTabletSizes()

START:
	pl := g.AnyServer(0)
	// We should always have some connection to dgraphzero.
	if pl == nil {
		x.Printf("WARNING: We don't have address of any dgraphzero server.")
		time.Sleep(time.Second)
		goto START
	}

	c := protos.NewZeroClient(pl.Get())
	ctx, cancel := context.WithCancel(context.Background())
	stream, err := c.Update(ctx)
	if err != nil {
		x.Printf("Error while calling update %v\n", err)
		time.Sleep(time.Second)
		goto START
	}

	go func() {
		n := g.Node
		for {
			// Blocking, should return if sending on stream fails(Need to verify).
			state, err := stream.Recv()
			if err != nil || state == nil {
				x.Printf("Unable to sync memberships. Error: %v", err)
				// If zero server is lagging behind leader.
				if ctx.Err() == nil {
					cancel()
				}
				return
			}
			if n.AmLeader() {
				proposal := &protos.Proposal{State: state}
				go n.ProposeAndWait(context.Background(), proposal)
			}
		}
	}()

	g.triggerMembershipSync() // Ticker doesn't start immediately
OUTER:
	for {
		select {
		case <-g.triggerCh:
			// On start of node if it becomes a leader, we would send tablets size for sure.
			if err := g.sendMembership(tablets, stream); err != nil {
				stream.CloseSend()
				break OUTER
			}
		case <-ticker.C:
			// dgraphzero just adds to the map so we needn't worry about race condition
			// where a newly added tablet is not sent.
			if g.Node.AmLeader() {
				tablets = g.calculateTabletSizes()
			}
			// Let's send update even if not leader, zero will know that this node is still
			// active.
			if err := g.sendMembership(tablets, stream); err != nil {
				x.Printf("Error while updating tablets size %v\n", err)
				stream.CloseSend()
				break OUTER
			}
		case <-ctx.Done():
			stream.CloseSend()
			break OUTER
		}
	}
	goto START
}

func (g *groupi) waitForBackgroundDeletion() {
	// Waits for background cleanup if any to finish.
	// No new cleanup on any predicate would start until we finish moving
	// the predicate because read only flag would be set by now. We start deletion
	// only when no predicate is being moved.
	g.delPred <- struct{}{}
	<-g.delPred
}

func (g *groupi) hasReadOnlyTablets() bool {
	g.RLock()
	defer g.RUnlock()
	if g.state == nil {
		return false
	}
	for _, group := range g.state.Groups {
		for _, tab := range group.Tablets {
			if tab.ReadOnly {
				return true
			}
		}
	}
	return false
}

func (g *groupi) cleanupTablets() {
	ticker := time.NewTimer(time.Minute * 10)
	select {
	case <-ticker.C:
		func() {
			opt := badger.DefaultIteratorOptions
			opt.PrefetchValues = false
			itr := pstore.NewIterator(opt)
			defer itr.Close()

			for itr.Rewind(); itr.Valid(); {
				item := itr.Item()

				pk := x.Parse(item.Key())

				// Delete at most one predicate at a time.
				// Tablet is not being served by me and is not read only.
				// Don't use servesTablet function because it can return false even if
				// request made to group zero fails. We might end up deleting a predicate
				// on failure of network request even though no one else is serving this
				// tablet.
				if tablet := g.Tablet(pk.Attr); tablet != nil && tablet.GroupId != g.groupId() {
					if g.hasReadOnlyTablets() {
						return
					}
					g.delPred <- struct{}{}
					posting.DeletePredicate(context.Background(), pk.Attr)
					<-g.delPred
					return
				}
				if pk.IsSchema() {
					itr.Seek(pk.SkipSchema())
					continue
				}
				itr.Seek(pk.SkipPredicate())
			}
		}()
	}
}

func (g *groupi) sendMembership(tablets map[string]*protos.Tablet,
	stream protos.Zero_UpdateClient) error {
	leader := g.Node.AmLeader()
	member := &protos.Member{
		Id:         Config.RaftId,
		GroupId:    g.groupId(),
		Addr:       Config.MyAddr,
		Leader:     leader,
		LastUpdate: uint64(time.Now().Unix()),
	}
	group := &protos.Group{
		Members: make(map[uint64]*protos.Member),
	}
	group.Members[member.Id] = member
	if leader {
		group.Tablets = tablets
	}

	return stream.Send(group)
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
