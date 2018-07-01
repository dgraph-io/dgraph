/*
 * Copyright 2016-2018 Dgraph Labs, Inc.
 *
 * This file is available under the Apache License, Version 2.0,
 * with the Commons Clause restriction.
 */

package worker

import (
	"fmt"
	"math"
	"sync/atomic"
	"time"

	"google.golang.org/grpc"

	"golang.org/x/net/context"
	"golang.org/x/net/trace"

	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/dgo/protos/api"
	"github.com/dgraph-io/dgraph/conn"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/intern"
	"github.com/dgraph-io/dgraph/raftwal"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/x"
)

type groupi struct {
	x.SafeMutex
	// TODO: Is this context being used?
	ctx       context.Context
	cancel    context.CancelFunc
	state     *intern.MembershipState
	Node      *node
	gid       uint32
	tablets   map[string]*intern.Tablet
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
func StartRaftNodes(walStore *badger.DB, bindall bool) {
	gr = new(groupi)
	gr.ctx, gr.cancel = context.WithCancel(context.Background())

	if len(Config.MyAddr) == 0 {
		Config.MyAddr = fmt.Sprintf("localhost:%d", workerPort())
	} else {
		// check if address is valid or not
		ok := x.ValidateAddress(Config.MyAddr)
		x.AssertTruef(ok, "%s is not valid address", Config.MyAddr)
		if !bindall {
			x.Printf("--my flag is provided without bindall, Did you forget to specify bindall?\n")
		}
	}

	x.AssertTruefNoTrace(len(Config.ZeroAddr) > 0, "Providing dgraphzero address is mandatory.")
	x.AssertTruefNoTrace(Config.ZeroAddr != Config.MyAddr,
		"Dgraph Zero address and Dgraph address (IP:Port) can't be the same.")

	if Config.RaftId == 0 {
		id, err := raftwal.RaftId(walStore)
		x.Check(err)
		Config.RaftId = id
	}
	x.Printf("Current Raft Id: %d\n", Config.RaftId)

	// Successfully connect with dgraphzero, before doing anything else.
	p := conn.Get().Connect(Config.ZeroAddr)

	// Connect with dgraphzero and figure out what group we should belong to.
	zc := intern.NewZeroClient(p.Get())
	var connState *intern.ConnectionState
	m := &intern.Member{Id: Config.RaftId, Addr: Config.MyAddr}
	delay := 50 * time.Millisecond
	maxHalfDelay := 3 * time.Second
	var err error
	for { // Keep on retrying. See: https://github.com/dgraph-io/dgraph/issues/2289
		connState, err = zc.Connect(gr.ctx, m)
		if err == nil || grpc.ErrorDesc(err) == x.ErrReuseRemovedId.Error() {
			break
		}
		x.Printf("Error while connecting with group zero: %v", err)
		time.Sleep(delay)
		if delay <= maxHalfDelay {
			delay *= 2
		}
	}
	x.CheckfNoTrace(err)
	if connState.GetMember() == nil || connState.GetState() == nil {
		x.Fatalf("Unable to join cluster via dgraphzero")
	}
	x.Printf("Connected to group zero. Assigned group: %+v\n", connState.GetMember().GetGroupId())
	Config.RaftId = connState.GetMember().GetId()
	// This timestamp would be used for reading during snapshot after bulk load.
	// The stream is async, we need this information before we start or else replica might
	// not get any data.
	posting.Oracle().SetMaxPending(connState.MaxPending)
	gr.applyState(connState.GetState())

	gid := gr.groupId()
	gr.triggerCh = make(chan struct{}, 1)
	gr.delPred = make(chan struct{}, 1)

	// Initialize DiskStorage and pass it along.
	store := raftwal.Init(walStore, Config.RaftId, gid)
	gr.Node = newNode(store, gid, Config.RaftId, Config.MyAddr)

	x.Checkf(schema.LoadFromDb(), "Error while initializing schema")
	raftServer.Node = gr.Node.Node
	gr.Node.InitAndStartNode()

	x.UpdateHealthStatus(true)
	go gr.periodicMembershipUpdate() // Now set it to be run periodically.
	go gr.cleanupTablets()
	go gr.processOracleDeltaStream()
	gr.proposeInitialSchema()
}

func (g *groupi) proposeInitialSchema() {
	if !Config.ExpandEdge {
		return
	}
	g.RLock()
	_, ok := g.tablets[x.PredicateListAttr]
	g.RUnlock()
	if ok {
		return
	}

	// Propose schema mutation.
	var m intern.Mutations
	// schema for _predicate_ is not changed once set.
	m.StartTs = 1
	m.Schema = append(m.Schema, &intern.SchemaUpdate{
		Predicate: x.PredicateListAttr,
		ValueType: intern.Posting_STRING,
		List:      true,
	})

	// This would propose the schema mutation and make sure some node serves this predicate
	// and has the schema defined above.
	for {
		_, err := MutateOverNetwork(gr.ctx, &m)
		if err == nil {
			break
		}
		x.Println("Error while proposing initial schema: ", err)
		time.Sleep(100 * time.Millisecond)
	}
}

// No locks are acquired while accessing this function.
// Don't acquire RW lock during this, otherwise we might deadlock.
func (g *groupi) groupId() uint32 {
	return atomic.LoadUint32(&g.gid)
}

// calculateTabletSizes iterates through badger and gets a size of the space occupied by each
// predicate (including data and indexes). All data for a predicate forms a Tablet.
func (g *groupi) calculateTabletSizes() map[string]*intern.Tablet {
	opt := badger.DefaultIteratorOptions
	opt.PrefetchValues = false
	txn := pstore.NewTransactionAt(math.MaxUint64, false)
	defer txn.Discard()
	itr := txn.NewIterator(opt)
	defer itr.Close()

	gid := g.groupId()
	tablets := make(map[string]*intern.Tablet)

	for itr.Rewind(); itr.Valid(); {
		item := itr.Item()

		pk := x.Parse(item.Key())
		if pk == nil {
			itr.Next()
			continue
		}

		// We should not be skipping schema keys here, otherwise if there is no data for them, they
		// won't be added to the tablets map returned by this function and would ultimately be
		// removed from the membership state.
		tablet, has := tablets[pk.Attr]
		if !has {
			if !g.ServesTablet(pk.Attr) {
				if pk.IsSchema() {
					itr.Next()
				} else {
					// data key for predicate we don't serve, skip it.
					itr.Seek(pk.SkipPredicate())
				}
				continue
			}
			tablet = &intern.Tablet{GroupId: gid, Predicate: pk.Attr}
			tablets[pk.Attr] = tablet
		}
		tablet.Space += item.EstimatedSize()
		itr.Next()
	}
	return tablets
}

func MaxLeaseId() uint64 {
	g := groups()
	g.RLock()
	defer g.RUnlock()
	if g.state == nil {
		return 0
	}
	return g.state.MaxLeaseId
}

func UpdateMembershipState(ctx context.Context) error {
	g := groups()
	p := g.Leader(0)
	if p == nil {
		return x.Errorf("don't have the address of any dgraphzero server")
	}

	c := intern.NewZeroClient(p.Get())
	state, err := c.Connect(ctx, &intern.Member{ClusterInfoOnly: true})
	if err != nil {
		return err
	}
	g.applyState(state.GetState())
	return nil
}

func (g *groupi) applyState(state *intern.MembershipState) {
	x.AssertTrue(state != nil)
	g.Lock()
	defer g.Unlock()
	// We don't update state if we get any old state. Counter stores the raftindex of
	// last update. For leader changes at zero since we don't propose, state can get
	// updated at same counter value. So ignore only if counter is less.
	if g.state != nil && g.state.Counter > state.Counter {
		return
	}

	g.state = state

	// While restarting we fill Node information after retrieving initial state.
	if g.Node != nil {
		// Lets have this block before the one that adds the new members, else we may end up
		// removing a freshly added node.
		for _, member := range g.state.Removed {
			if member.GroupId == g.Node.gid && g.Node.AmLeader() {
				go g.Node.ProposePeerRemoval(context.Background(), member.Id)
			}
			// Each node should have different id and address.
			conn.Get().Remove(member.Addr)
		}
	}

	// Sometimes this can cause us to lose latest tablet info, but that shouldn't cause any issues.
	g.tablets = make(map[string]*intern.Tablet)
	for gid, group := range g.state.Groups {
		for _, member := range group.Members {
			if Config.RaftId == member.Id {
				atomic.StoreUint32(&g.gid, gid)
			}
			if Config.MyAddr != member.Addr {
				conn.Get().Connect(member.Addr)
			}
		}
		for _, tablet := range group.Tablets {
			g.tablets[tablet.Predicate] = tablet
		}
	}
	for _, member := range g.state.Zeros {
		if Config.MyAddr != member.Addr {
			conn.Get().Connect(member.Addr)
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

func (g *groupi) ServesTabletRW(key string) bool {
	tablet := g.Tablet(key)
	if tablet != nil && !tablet.ReadOnly && tablet.GroupId == groups().groupId() {
		return true
	}
	return false
}

func (g *groupi) ServesTablet(key string) bool {
	tablet := g.Tablet(key)
	if tablet != nil && tablet.GroupId == groups().groupId() {
		return true
	}
	return false
}

// Do not modify the returned Tablet
func (g *groupi) Tablet(key string) *intern.Tablet {
	// TODO: Remove all this later, create a membership state and apply it
	g.RLock()
	tablet, ok := g.tablets[key]
	g.RUnlock()
	if ok {
		return tablet
	}

	// We don't know about this tablet.
	// Check with dgraphzero if we can serve it.
	pl := g.AnyServer(0)
	if pl == nil {
		return nil
	}
	zc := intern.NewZeroClient(pl.Get())

	tablet = &intern.Tablet{GroupId: g.groupId(), Predicate: key}
	out, err := zc.ShouldServe(context.Background(), tablet)
	if err != nil {
		x.Printf("Error while ShouldServe grpc call %v", err)
		return nil
	}
	g.Lock()
	g.tablets[key] = out
	g.Unlock()

	if out.GroupId == groups().groupId() {
		x.Printf("Serving tablet for: %v\n", key)
	}
	return out
}

func (g *groupi) HasMeInState() bool {
	g.RLock()
	defer g.RUnlock()
	if g.state == nil {
		return false
	}

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

	if g.state == nil {
		return []string{}
	}
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

func (g *groupi) members(gid uint32) map[uint64]*intern.Member {
	g.RLock()
	defer g.RUnlock()

	if g.state == nil {
		return nil
	}
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
	return nil
}

func (g *groupi) KnownGroups() (gids []uint32) {
	g.RLock()
	defer g.RUnlock()
	if g.state == nil {
		return
	}
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
	// Calculating tablet sizes is expensive, hence we do it only every 5 mins.
	ticker := time.NewTicker(time.Minute * 5)
	// Node might not be the leader when we are calculating size.
	// We need to send immediately on start so no leader check inside calculatesize.
	tablets := g.calculateTabletSizes()

START:
	pl := g.AnyServer(0)
	// We should always have some connection to dgraphzero.
	if pl == nil {
		x.Printf("WARNING: We don't have address of any Zero server.")
		time.Sleep(time.Second)
		goto START
	}
	x.Printf("Got address of a Zero server: %s", pl.Addr)

	c := intern.NewZeroClient(pl.Get())
	ctx, cancel := context.WithCancel(context.Background())
	stream, err := c.Update(ctx)
	if err != nil {
		x.Printf("Error while calling update %v\n", err)
		time.Sleep(time.Second)
		goto START
	}

	go func() {
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
			g.applyState(state)
		}
	}()

	g.triggerMembershipSync() // Ticker doesn't start immediately
OUTER:
	for {
		select {
		case <-g.triggerCh:
			if !g.Node.AmLeader() {
				tablets = nil
			}
			// On start of node if it becomes a leader, we would send tablets size for sure.
			if err := g.sendMembership(tablets, stream); err != nil {
				stream.CloseSend()
				break OUTER
			}
		case <-ticker.C:
			// dgraphzero just adds to the map so check that no data is present for the tablet
			// before we remove it to avoid the race condition where a tablet is added recently
			// and mutation has not been persisted to disk.
			var allTablets map[string]*intern.Tablet
			if g.Node.AmLeader() {
				prevTablets := tablets
				tablets = g.calculateTabletSizes()
				if prevTablets != nil {
					allTablets = make(map[string]*intern.Tablet)
					g.RLock()
					for attr := range g.tablets {
						if tablets[attr] == nil && prevTablets[attr] == nil {
							allTablets[attr] = &intern.Tablet{
								GroupId:   g.gid,
								Predicate: attr,
								Remove:    true,
							}
						}
					}
					g.RUnlock()
					for attr, tab := range tablets {
						allTablets[attr] = tab
					}
				} else {
					allTablets = tablets
				}
			}
			// Let's send update even if not leader, zero will know that this node is still
			// active.
			if err := g.sendMembership(allTablets, stream); err != nil {
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
			txn := pstore.NewTransactionAt(math.MaxUint64, false)
			defer txn.Discard()
			itr := txn.NewIterator(opt)
			defer itr.Close()

			for itr.Rewind(); itr.Valid(); {
				item := itr.Item()

				// TODO: Investiage out of bounds.
				pk := x.Parse(item.Key())
				if pk == nil {
					itr.Next()
					continue
				}

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
					// Predicate moves are disabled during deletion, deletePredicate purges everything.
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

func (g *groupi) sendMembership(tablets map[string]*intern.Tablet,
	stream intern.Zero_UpdateClient) error {
	leader := g.Node.AmLeader()
	member := &intern.Member{
		Id:         Config.RaftId,
		GroupId:    g.groupId(),
		Addr:       Config.MyAddr,
		Leader:     leader,
		LastUpdate: uint64(time.Now().Unix()),
	}
	group := &intern.Group{
		Members: make(map[uint64]*intern.Member),
	}
	group.Members[member.Id] = member
	if leader {
		group.Tablets = tablets
	}

	return stream.Send(group)
}

// processOracleDeltaStream is used to process oracle delta stream from Zero.
// Zero sends information about aborted/committed transactions and maxPending.
func (g *groupi) processOracleDeltaStream() {
	blockingReceiveAndPropose := func() {
		elog := trace.NewEventLog("Dgraph", "ProcessOracleStream")
		defer elog.Finish()
		elog.Printf("Leader idx=%d of group=%d is connecting to Zero for txn updates\n",
			g.Node.Id, g.groupId())

		pl := g.Leader(0)
		if pl == nil {
			x.Printf("WARNING: We don't have address of any dgraphzero leader.")
			elog.Errorf("Dgraph zero leader address unknown")
			time.Sleep(time.Second)
			return
		}

		// The following code creates a stream. Then runs a goroutine to pick up events from the
		// stream and pushes them to a channel. The main loop loops over the channel, doing smart
		// batching. Once a batch is created, it gets proposed. Thus, we can reduce the number of
		// times proposals happen, which is a great optimization to have (and a common one in our
		// code base).
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		c := intern.NewZeroClient(pl.Get())
		// The first entry send by Zero contains the entire state of transactions. Zero periodically
		// confirms receipt from the group, and truncates its state. This 2-way acknowledgement is a
		// safe way to get the status of all the transactions.
		stream, err := c.Oracle(ctx, &api.Payload{})
		if err != nil {
			x.Printf("Error while calling Oracle %v\n", err)
			elog.Errorf("Error while calling Oracle %v", err)
			time.Sleep(time.Second)
			return
		}

		deltaCh := make(chan *intern.OracleDelta, 100)
		go func() {
			// This would exit when either a Recv() returns error. Or, cancel() is called by
			// something outside of this goroutine.
			defer stream.CloseSend()
			defer close(deltaCh)

			for {
				delta, err := stream.Recv()
				if err != nil || delta == nil {
					x.Printf("Error in oracle delta stream. Error: %v", err)
					return
				}

				select {
				case deltaCh <- delta:
				case <-ctx.Done():
					return
				}
			}
		}()

		for {
			var delta *intern.OracleDelta
			var batch int
			select {
			case delta = <-deltaCh:
				if delta == nil {
					return
				}
				batch++
			case <-ctx.Done():
				return
			}

		SLURP:
			for {
				select {
				case more := <-deltaCh:
					if more == nil {
						return
					}
					batch++
					if delta.Commits == nil {
						delta.Commits = make(map[uint64]uint64)
					}
					// Merge more with delta.
					for start, commit := range more.GetCommits() {
						delta.Commits[start] = commit
					}
					delta.Aborts = append(delta.Aborts, more.Aborts...)
					if delta.MaxPending < more.MaxPending {
						delta.MaxPending = more.MaxPending
					}
				default:
					break SLURP
				}
			}

			// Only the leader needs to propose the oracleDelta retrieved from Zero.
			// The leader and the followers would not directly apply or use the
			// oracleDelta streaming in from Zero. They would wait for the proposal to
			// go through and be applied via node.Run.  This saves us from many edge
			// cases around network partitions and race conditions between prewrites and
			// commits, etc.
			if !g.Node.AmLeader() {
				elog.Errorf("No longer the leader of group %d. Exiting", g.groupId())
				return
			}
			// Block forever trying to propose this.
			elog.Printf("Batched %d updates. Proposing Delta of size: %d.", batch, delta.Size())
			g.Node.proposeAndWait(context.Background(), &intern.Proposal{Delta: delta})
		}
	}

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-g.Node.stop:
			return
		case <-ticker.C:
			// Only the leader needs to connect to Zero and get transaction
			// updates.
			if g.Node.AmLeader() {
				blockingReceiveAndPropose()
			}
		}
	}
}
