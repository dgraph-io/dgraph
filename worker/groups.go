/*
 * Copyright 2016-2018 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package worker

import (
	"fmt"
	"io"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/net/context"

	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/badger/y"
	"github.com/dgraph-io/dgo/protos/api"
	"github.com/dgraph-io/dgraph/conn"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/raftwal"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/x"
	"github.com/golang/glog"
)

type groupi struct {
	x.SafeMutex
	// TODO: Is this context being used?
	ctx          context.Context
	cancel       context.CancelFunc
	state        *pb.MembershipState
	Node         *node
	gid          uint32
	tablets      map[string]*pb.Tablet
	triggerCh    chan struct{} // Used to trigger membership sync
	blockDeletes *sync.Mutex   // Ensure that deletion won't happen when move is going on.
	closer       *y.Closer

	// Group checksum is used to determine if the tablets served by the groups have changed from
	// the membership information that the Alpha has. If so, Alpha cannot service a read.
	deltaChecksum      uint64 // Checksum received by OracleDelta.
	membershipChecksum uint64 // Checksum received by MembershipState.
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
	gr = &groupi{
		blockDeletes: new(sync.Mutex),
	}
	gr.ctx, gr.cancel = context.WithCancel(context.Background())

	if len(x.WorkerConfig.MyAddr) == 0 {
		x.WorkerConfig.MyAddr = fmt.Sprintf("localhost:%d", workerPort())
	} else {
		// check if address is valid or not
		ok := x.ValidateAddress(x.WorkerConfig.MyAddr)
		x.AssertTruef(ok, "%s is not valid address", x.WorkerConfig.MyAddr)
		if !bindall {
			glog.Errorln("--my flag is provided without bindall, Did you forget to specify bindall?")
		}
	}

	x.AssertTruef(len(x.WorkerConfig.ZeroAddr) > 0, "Providing dgraphzero address is mandatory.")
	x.AssertTruef(x.WorkerConfig.ZeroAddr != x.WorkerConfig.MyAddr,
		"Dgraph Zero address and Dgraph address (IP:Port) can't be the same.")

	if x.WorkerConfig.RaftId == 0 {
		id, err := raftwal.RaftId(walStore)
		x.Check(err)
		x.WorkerConfig.RaftId = id
	}
	glog.Infof("Current Raft Id: %#x\n", x.WorkerConfig.RaftId)

	// Successfully connect with dgraphzero, before doing anything else.

	// Connect with Zero leader and figure out what group we should belong to.
	m := &pb.Member{Id: x.WorkerConfig.RaftId, Addr: x.WorkerConfig.MyAddr}
	var connState *pb.ConnectionState
	var err error
	for { // Keep on retrying. See: https://github.com/dgraph-io/dgraph/issues/2289
		pl := gr.connToZeroLeader()
		if pl == nil {
			continue
		}
		zc := pb.NewZeroClient(pl.Get())
		connState, err = zc.Connect(gr.ctx, m)
		if err == nil || x.ShouldCrash(err) {
			break
		}
	}
	x.CheckfNoTrace(err)
	if connState.GetMember() == nil || connState.GetState() == nil {
		x.Fatalf("Unable to join cluster via dgraphzero")
	}
	glog.Infof("Connected to group zero. Assigned group: %+v\n", connState.GetMember().GetGroupId())
	x.WorkerConfig.RaftId = connState.GetMember().GetId()
	glog.Infof("Raft Id after connection to Zero: %#x\n", x.WorkerConfig.RaftId)

	// This timestamp would be used for reading during snapshot after bulk load.
	// The stream is async, we need this information before we start or else replica might
	// not get any data.
	gr.applyState(connState.GetState())

	gid := gr.groupId()
	gr.triggerCh = make(chan struct{}, 1)

	// Initialize DiskStorage and pass it along.
	store := raftwal.Init(walStore, x.WorkerConfig.RaftId, gid)
	gr.Node = newNode(store, gid, x.WorkerConfig.RaftId, x.WorkerConfig.MyAddr)

	x.Checkf(schema.LoadFromDb(), "Error while initializing schema")
	raftServer.Node = gr.Node.Node
	gr.Node.InitAndStartNode()
	x.UpdateHealthStatus(true)
	glog.Infof("Server is ready")

	gr.closer = y.NewCloser(3) // Match CLOSER:1 in this file.
	go gr.sendMembershipUpdates()
	go gr.receiveMembershipUpdates()
	go gr.processOracleDeltaStream()

	go gr.informZeroAboutTablets()
	gr.proposeInitialSchema()
}

func (g *groupi) informZeroAboutTablets() {
	// Before we start this Alpha, let's pick up all the predicates we have in our postings
	// directory, and ask Zero if we are allowed to serve it. Do this irrespective of whether
	// this node is the leader or the follower, because this early on, we might not have
	// figured that out.
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for range ticker.C {
		failed := false
		preds := schema.State().Predicates()
		for _, pred := range preds {
			if tablet := g.Tablet(pred); tablet == nil {
				failed = true
			}
		}
		if !failed {
			glog.V(1).Infof("Done informing Zero about the %d tablets I have", len(preds))
			return
		}
	}
}

func (g *groupi) proposeInitialSchema() {
	initialSchema := schema.InitialSchema()
	for _, s := range initialSchema {
		g.upsertSchema(s)
	}
}

func (g *groupi) upsertSchema(schema *pb.SchemaUpdate) {
	// Propose schema mutation.
	var m pb.Mutations
	// schema for _predicate_ is not changed once set.
	m.StartTs = 1
	m.Schema = append(m.Schema, schema)

	// This would propose the schema mutation and make sure some node serves this predicate
	// and has the schema defined above.
	for {
		_, err := MutateOverNetwork(gr.ctx, &m)
		if err == nil {
			break
		}
		glog.Errorf("Error while proposing initial schema: %v\n", err)
		time.Sleep(100 * time.Millisecond)
	}
}

// No locks are acquired while accessing this function.
// Don't acquire RW lock during this, otherwise we might deadlock.
func (g *groupi) groupId() uint32 {
	return atomic.LoadUint32(&g.gid)
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
		return x.Errorf("Don't have the address of any dgraphzero server")
	}

	c := pb.NewZeroClient(p.Get())
	state, err := c.Connect(ctx, &pb.Member{ClusterInfoOnly: true})
	if err != nil {
		return err
	}
	g.applyState(state.GetState())
	return nil
}

func (g *groupi) applyState(state *pb.MembershipState) {
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

	// Sometimes this can cause us to lose latest tablet info, but that shouldn't cause any issues.
	var foundSelf bool
	g.tablets = make(map[string]*pb.Tablet)
	for gid, group := range g.state.Groups {
		for _, member := range group.Members {
			if x.WorkerConfig.RaftId == member.Id {
				foundSelf = true
				atomic.StoreUint32(&g.gid, gid)
			}
			if x.WorkerConfig.MyAddr != member.Addr {
				conn.Get().Connect(member.Addr)
			}
		}
		for _, tablet := range group.Tablets {
			g.tablets[tablet.Predicate] = tablet
		}
		if gid == g.gid {
			glog.V(3).Infof("group %d checksum: %d", g.gid, group.Checksum)
			atomic.StoreUint64(&g.membershipChecksum, group.Checksum)
		}
	}
	for _, member := range g.state.Zeros {
		if x.WorkerConfig.MyAddr != member.Addr {
			conn.Get().Connect(member.Addr)
		}
	}
	if !foundSelf {
		// I'm not part of this cluster. I should crash myself.
		glog.Fatalf("Unable to find myself [id:%d group:%d] in membership state: %+v. Goodbye!",
			g.Node.Id, g.groupId(), state)
	}

	// While restarting we fill Node information after retrieving initial state.
	if g.Node != nil {
		// Lets have this block before the one that adds the new members, else we may end up
		// removing a freshly added node.
		for _, member := range g.state.Removed {
			if member.GroupId == g.Node.gid && g.Node.AmLeader() {
				go g.Node.ProposePeerRemoval(context.Background(), member.Id)
			}
		}
		conn.Get().RemoveInvalid(g.state)
	}
}

func (g *groupi) ServesGroup(gid uint32) bool {
	g.RLock()
	defer g.RUnlock()
	return g.gid == gid
}

func (g *groupi) ChecksumsMatch(ctx context.Context) error {
	if atomic.LoadUint64(&g.deltaChecksum) == atomic.LoadUint64(&g.membershipChecksum) {
		return nil
	}
	t := time.NewTicker(100 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			if atomic.LoadUint64(&g.deltaChecksum) == atomic.LoadUint64(&g.membershipChecksum) {
				return nil
			}
		case <-ctx.Done():
			return fmt.Errorf("Group checksum mismatch for id: %d", g.gid)
		}
	}
}

func (g *groupi) BelongsTo(key string) uint32 {
	tablet := g.Tablet(key)
	if tablet != nil {
		return tablet.GroupId
	}
	return 0
}

func (g *groupi) ServesTablet(key string) bool {
	tablet := g.Tablet(key)
	if tablet != nil && tablet.GroupId == groups().groupId() {
		return true
	}
	return false
}

// Do not modify the returned Tablet
// TODO: This should return error.
func (g *groupi) Tablet(key string) *pb.Tablet {
	// TODO: Remove all this later, create a membership state and apply it
	g.RLock()
	tablet, ok := g.tablets[key]
	g.RUnlock()
	if ok {
		return tablet
	}

	// We don't know about this tablet.
	// Check with dgraphzero if we can serve it.
	pl := g.connToZeroLeader()
	zc := pb.NewZeroClient(pl.Get())

	tablet = &pb.Tablet{GroupId: g.groupId(), Predicate: key}
	out, err := zc.ShouldServe(context.Background(), tablet)
	if err != nil {
		glog.Errorf("Error while ShouldServe grpc call %v", err)
		return nil
	}
	g.Lock()
	g.tablets[key] = out
	g.Unlock()

	if out.GroupId == groups().groupId() {
		glog.Infof("Serving tablet for: %v\n", key)
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

func (g *groupi) members(gid uint32) map[uint64]*pb.Member {
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

const connBaseDelay = 100 * time.Millisecond

func (g *groupi) connToZeroLeader() *conn.Pool {
	pl := g.Leader(0)
	if pl != nil {
		return pl
	}
	glog.V(1).Infof("No healthy Zero leader found. Trying to find a Zero leader...")

	getLeaderConn := func(zc pb.ZeroClient) *conn.Pool {
		ctx, cancel := context.WithTimeout(gr.ctx, 10*time.Second)
		defer cancel()

		connState, err := zc.Connect(ctx, &pb.Member{ClusterInfoOnly: true})
		if err != nil || connState == nil {
			glog.V(1).Infof("While retrieving Zero leader info. Error: %v. Retrying...", err)
			return nil
		}
		for _, mz := range connState.State.GetZeros() {
			if mz.Leader {
				return conn.Get().Connect(mz.GetAddr())
			}
		}
		return nil
	}

	// No leader found. Let's get the latest membership state from Zero.
	delay := connBaseDelay
	maxHalfDelay := time.Second
	for { // Keep on retrying. See: https://github.com/dgraph-io/dgraph/issues/2289
		time.Sleep(delay)
		if delay <= maxHalfDelay {
			delay *= 2
		}
		pl := g.AnyServer(0)
		if pl == nil {
			pl = conn.Get().Connect(x.WorkerConfig.ZeroAddr)
		}
		if pl == nil {
			glog.V(1).Infof("No healthy Zero server found. Retrying...")
			continue
		}
		zc := pb.NewZeroClient(pl.Get())
		if pl := getLeaderConn(zc); pl != nil {
			glog.V(1).Infof("Found connection to leader: %s", pl.Addr)
			return pl
		}
		glog.V(1).Infof("Unable to connect to a healthy Zero leader. Retrying...")
	}
}

func (g *groupi) doSendMembership(tablets map[string]*pb.Tablet) error {
	leader := g.Node.AmLeader()
	member := &pb.Member{
		Id:         x.WorkerConfig.RaftId,
		GroupId:    g.groupId(),
		Addr:       x.WorkerConfig.MyAddr,
		Leader:     leader,
		LastUpdate: uint64(time.Now().Unix()),
	}
	group := &pb.Group{
		Members: make(map[uint64]*pb.Member),
	}
	group.Members[member.Id] = member
	if leader {
		// Do not send tablet information, if I'm not the leader.
		group.Tablets = tablets
		if snap, err := g.Node.Snapshot(); err == nil {
			group.SnapshotTs = snap.ReadTs
		}
	}

	pl := g.connToZeroLeader()
	if pl == nil {
		return errNoConnection
	}
	c := pb.NewZeroClient(pl.Get())
	ctx, cancel := context.WithTimeout(g.ctx, 10*time.Second)
	defer cancel()
	reply, err := c.UpdateMembership(ctx, group)
	if err != nil {
		return err
	}
	if string(reply.GetData()) == "OK" {
		return nil
	}
	return x.Errorf(string(reply.GetData()))
}

// sendMembershipUpdates sends the membership update to Zero leader. If this Alpha is the leader, it
// would also calculate the tablet sizes and send them to Zero.
func (g *groupi) sendMembershipUpdates() {
	defer g.closer.Done() // CLOSER:1

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	consumeTriggers := func() {
		for {
			select {
			case <-g.triggerCh:
			default:
				return
			}
		}
	}

	g.triggerMembershipSync() // Ticker doesn't start immediately
	var lastSent time.Time
	for {
		select {
		case <-g.closer.HasBeenClosed():
			return
		case <-ticker.C:
			if time.Since(lastSent) > 10*time.Second {
				// On start of node if it becomes a leader, we would send tablets size for sure.
				g.triggerMembershipSync()
			}
		case <-g.triggerCh:
			// Let's send update even if not leader, zero will know that this node is still active.
			// We don't need to send tablet information everytime. So, let's only send it when we
			// calculate it.
			consumeTriggers()
			if err := g.doSendMembership(nil); err != nil {
				glog.Errorf("While sending membership update: %v", err)
			} else {
				lastSent = time.Now()
			}
		}
	}
}

// receiveMembershipUpdates receives membership updates from ANY Zero server. This is the main
// connection which tells Alpha about the state of the cluster, including the latest Zero leader.
// All the other connections to Zero, are only made only to the leader.
func (g *groupi) receiveMembershipUpdates() {
	defer g.closer.Done() // CLOSER:1

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

START:
	select {
	case <-g.closer.HasBeenClosed():
		return
	default:
	}

	pl := g.connToZeroLeader()
	// We should always have some connection to dgraphzero.
	if pl == nil {
		glog.Warningln("Membership update: No Zero server known.")
		time.Sleep(time.Second)
		goto START
	}
	glog.Infof("Got address of a Zero leader: %s", pl.Addr)

	c := pb.NewZeroClient(pl.Get())
	ctx, cancel := context.WithCancel(context.Background())
	stream, err := c.StreamMembership(ctx, &api.Payload{})
	if err != nil {
		glog.Errorf("Error while calling update %v\n", err)
		time.Sleep(time.Second)
		goto START
	}

	stateCh := make(chan *pb.MembershipState, 10)
	go func() {
		glog.Infof("Starting a new membership stream receive from %s.", pl.Addr)
		for i := 0; ; i++ {
			// Blocking, should return if sending on stream fails(Need to verify).
			state, err := stream.Recv()
			if err != nil || state == nil {
				if err == io.EOF {
					glog.Infoln("Membership sync stream closed.")
				} else {
					glog.Errorf("Unable to sync memberships. Error: %v. State: %v", err, state)
				}
				// If zero server is lagging behind leader.
				if ctx.Err() == nil {
					cancel()
				}
				return
			}
			if i == 0 {
				glog.Infof("Received first state update from Zero: %+v", state)
			}
			select {
			case stateCh <- state:
			case <-ctx.Done():
				return
			}
		}
	}()

	lastRecv := time.Now()
OUTER:
	for {
		select {
		case <-g.closer.HasBeenClosed():
			stream.CloseSend()
			break OUTER
		case <-ctx.Done():
			stream.CloseSend()
			break OUTER
		case state := <-stateCh:
			lastRecv = time.Now()
			g.applyState(state)
		case <-ticker.C:
			if time.Since(lastRecv) > 10*time.Second {
				// Zero might have gone under partition. We should recreate our connection.
				glog.Warningf("No membership update for 10s. Closing connection to Zero.")
				stream.CloseSend()
				break OUTER
			}
		}
	}
	cancel()
	goto START
}

// processOracleDeltaStream is used to process oracle delta stream from Zero.
// Zero sends information about aborted/committed transactions and maxPending.
func (g *groupi) processOracleDeltaStream() {
	defer g.closer.Done() // CLOSER:1

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	blockingReceiveAndPropose := func() {
		glog.Infof("Leader idx=%#x of group=%d is connecting to Zero for txn updates\n",
			g.Node.Id, g.groupId())

		pl := g.connToZeroLeader()
		if pl == nil {
			glog.Warningln("Oracle delta stream: No Zero leader known.")
			time.Sleep(time.Second)
			return
		}
		glog.Infof("Got Zero leader: %s", pl.Addr)

		// The following code creates a stream. Then runs a goroutine to pick up events from the
		// stream and pushes them to a channel. The main loop loops over the channel, doing smart
		// batching. Once a batch is created, it gets proposed. Thus, we can reduce the number of
		// times proposals happen, which is a great optimization to have (and a common one in our
		// code base).
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		c := pb.NewZeroClient(pl.Get())
		// The first entry send by Zero contains the entire state of transactions. Zero periodically
		// confirms receipt from the group, and truncates its state. This 2-way acknowledgement is a
		// safe way to get the status of all the transactions.
		stream, err := c.Oracle(ctx, &api.Payload{})
		if err != nil {
			glog.Errorf("Error while calling Oracle %v\n", err)
			time.Sleep(time.Second)
			return
		}

		deltaCh := make(chan *pb.OracleDelta, 100)
		go func() {
			// This would exit when either a Recv() returns error. Or, cancel() is called by
			// something outside of this goroutine.
			defer stream.CloseSend()
			defer close(deltaCh)

			for {
				delta, err := stream.Recv()
				if err != nil || delta == nil {
					glog.Errorf("Error in oracle delta stream. Error: %v", err)
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
			var delta *pb.OracleDelta
			var batch int
			select {
			case delta = <-deltaCh:
				if delta == nil {
					return
				}
				batch++
			case <-ticker.C:
				newLead := g.Leader(0)
				if newLead == nil || newLead.Addr != pl.Addr {
					glog.Infof("Zero leadership changed. Renewing oracle delta stream.")
					return
				}
				continue

			case <-ctx.Done():
				return
			case <-g.closer.HasBeenClosed():
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
					delta.Txns = append(delta.Txns, more.Txns...)
					delta.MaxAssigned = x.Max(delta.MaxAssigned, more.MaxAssigned)
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
				glog.Errorf("No longer the leader of group %d. Exiting", g.groupId())
				return
			}

			// We should always sort the txns before applying. Otherwise, we might lose some of
			// these updates, because we never write over a new version.
			sort.Slice(delta.Txns, func(i, j int) bool {
				return delta.Txns[i].CommitTs < delta.Txns[j].CommitTs
			})
			if len(delta.Txns) > 0 {
				last := delta.Txns[len(delta.Txns)-1]
				// Update MaxAssigned on commit so best effort queries can get back latest data.
				delta.MaxAssigned = x.Max(delta.MaxAssigned, last.CommitTs)
			}
			if glog.V(3) {
				glog.Infof("Batched %d updates. Max Assigned: %d. Proposing Deltas:",
					batch, delta.MaxAssigned)
				for _, txn := range delta.Txns {
					if txn.CommitTs == 0 {
						glog.Infof("Aborted: %d", txn.StartTs)
					} else {
						glog.Infof("Committed: %d -> %d", txn.StartTs, txn.CommitTs)
					}
				}
			}
			for {
				// Block forever trying to propose this.
				err := g.Node.proposeAndWait(context.Background(), &pb.Proposal{Delta: delta})
				if err == nil {
					break
				}
				glog.Errorf("While proposing delta: %v. Error=%v. Retrying...\n", delta, err)
			}
		}
	}

	for {
		select {
		case <-g.closer.HasBeenClosed():
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
