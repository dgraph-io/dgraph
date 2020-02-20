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
	"context"
	"fmt"
	"io"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dgraph-io/badger/v2"
	badgerpb "github.com/dgraph-io/badger/v2/pb"
	"github.com/dgraph-io/badger/v2/y"
	"github.com/dgraph-io/dgo/v2/protos/api"
	"github.com/dgraph-io/dgraph/conn"
	"github.com/dgraph-io/dgraph/ee/enc"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/raftwal"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/x"
	"github.com/golang/glog"
	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
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

var gr = &groupi{
	blockDeletes: new(sync.Mutex),
	tablets:      make(map[string]*pb.Tablet),
}

func groups() *groupi {
	return gr
}

// StartRaftNodes will read the WAL dir, create the RAFT groups,
// and either start or restart RAFT nodes.
// This function triggers RAFT nodes to be created, and is the entrance to the RAFT
// world from main.go.
func StartRaftNodes(walStore *badger.DB, bindall bool) {
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

		// If the w directory already contains raft information, ignore the proposed
		// group ID stored inside the p directory.
		if id > 0 {
			x.WorkerConfig.ProposedGroupId = 0
		}
	}
	glog.Infof("Current Raft Id: %#x\n", x.WorkerConfig.RaftId)

	// Successfully connect with dgraphzero, before doing anything else.

	// Connect with Zero leader and figure out what group we should belong to.
	m := &pb.Member{Id: x.WorkerConfig.RaftId, GroupId: x.WorkerConfig.ProposedGroupId,
		Addr: x.WorkerConfig.MyAddr}
	if m.GroupId > 0 {
		m.ForceGroupId = true
	}
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
	raftServer.UpdateNode(gr.Node.Node)
	gr.Node.InitAndStartNode()
	x.UpdateHealthStatus(true)
	glog.Infof("Server is ready")

	gr.closer = y.NewCloser(3) // Match CLOSER:1 in this file.
	go gr.sendMembershipUpdates()
	go gr.receiveMembershipUpdates()
	go gr.processOracleDeltaStream()

	gr.informZeroAboutTablets()
	gr.proposeInitialSchema()
	gr.proposeInitialTypes()
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
			if tablet, err := g.Tablet(pred); err != nil {
				failed = true
				glog.Errorf("Error while getting tablet for pred %q: %v", pred, err)
			} else if tablet == nil {
				failed = true
			}
		}
		if !failed {
			glog.V(1).Infof("Done informing Zero about the %d tablets I have", len(preds))
			return
		}
	}
}

func (g *groupi) proposeInitialTypes() {
	initialTypes := schema.InitialTypes()
	for _, t := range initialTypes {
		if _, ok := schema.State().GetType(t.TypeName); ok {
			continue
		}
		g.upsertSchema(nil, t)
	}
}

func (g *groupi) proposeInitialSchema() {
	initialSchema := schema.InitialSchema()
	for _, s := range initialSchema {
		if gid, err := g.BelongsToReadOnly(s.Predicate, 0); err != nil {
			glog.Errorf("Error getting tablet for predicate %s. Will force schema proposal.",
				s.Predicate)
			g.upsertSchema(s, nil)
		} else if gid == 0 {
			g.upsertSchema(s, nil)
		} else if curr, _ := schema.State().Get(schema.ReadCtx, s.Predicate); gid == g.groupId() &&
			!proto.Equal(s, &curr) {
			// If this tablet is served to the group, do not upsert the schema unless the
			// stored schema and the proposed one are different.
			g.upsertSchema(s, nil)
		} else {
			// The schema for this predicate has already been proposed.
			glog.V(1).Infof("Skipping initial schema upsert for predicate %s", s.Predicate)
			continue
		}
	}
}

func (g *groupi) upsertSchema(sch *pb.SchemaUpdate, typ *pb.TypeUpdate) {
	// Propose schema mutation.
	var m pb.Mutations
	// schema for a reserved predicate is not changed once set.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	ts, err := Timestamps(ctx, &pb.Num{Val: 1})
	cancel()
	if err != nil {
		glog.Errorf("error while requesting timestamp for schema %v: %v", sch, err)
		return
	}

	m.StartTs = ts.StartId
	if sch != nil {
		m.Schema = append(m.Schema, sch)
	}
	if typ != nil {
		m.Types = append(m.Types, typ)
	}

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

// MaxLeaseId returns the maximum UID that has been leased.
func MaxLeaseId() uint64 {
	g := groups()
	g.RLock()
	defer g.RUnlock()
	if g.state == nil {
		return 0
	}
	return g.state.MaxLeaseId
}

// GetMembershipState returns the current membership state.
func GetMembershipState() *pb.MembershipState {
	g := groups()
	g.RLock()
	defer g.RUnlock()
	return proto.Clone(g.state).(*pb.MembershipState)
}

// UpdateMembershipState contacts zero for an update on membership state.
func UpdateMembershipState(ctx context.Context) error {
	g := groups()
	p := g.Leader(0)
	if p == nil {
		return errors.Errorf("don't have the address of any dgraph zero leader")
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
	oldState := g.state
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
				conn.GetPools().Connect(member.Addr)
			}
		}
		for _, tablet := range group.Tablets {
			g.tablets[tablet.Predicate] = tablet
		}
		if gid == g.groupId() {
			glog.V(3).Infof("group %d checksum: %d", g.groupId(), group.Checksum)
			atomic.StoreUint64(&g.membershipChecksum, group.Checksum)
		}
	}
	for _, member := range g.state.Zeros {
		if x.WorkerConfig.MyAddr != member.Addr {
			conn.GetPools().Connect(member.Addr)
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

		for _, member := range g.state.GetRemoved() {
			// TODO: This leader check can be done once instead of repeatedly.
			if member.GetGroupId() == g.Node.gid && g.Node.AmLeader() {
				go func() {
					// Don't try to remove a member if it's already marked as removed in
					// the membership state and is not a current peer of the node.
					_, isPeer := g.Node.Peer(member.GetId())
					// isPeer should only be true if the rmeoved node is not the same as this node.
					isPeer = isPeer && member.GetId() != g.Node.RaftContext.Id

					for _, oldMember := range oldState.GetRemoved() {
						if oldMember.GetId() == member.GetId() && !isPeer {
							return
						}
					}

					if err := g.Node.ProposePeerRemoval(
						context.Background(), member.GetId()); err != nil {
						glog.Errorf("Error while proposing node removal: %+v", err)
					}
				}()
			}
		}
		conn.GetPools().RemoveInvalid(g.state)
	}
}

func (g *groupi) ServesGroup(gid uint32) bool {
	return g.groupId() == gid
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
			return errors.Errorf("Group checksum mismatch for id: %d", g.groupId())
		}
	}
}

func (g *groupi) BelongsTo(key string) (uint32, error) {
	if tablet, err := g.Tablet(key); err != nil {
		return 0, err
	} else if tablet != nil {
		return tablet.GroupId, nil
	}
	return 0, nil
}

// BelongsToReadOnly acts like BelongsTo except it does not ask zero to serve
// the tablet for key if no group is currently serving it.
// The ts passed should be the start ts of the query, so this method can compare that against a
// tablet move timestamp. If the tablet was moved to this group after the start ts of the query, we
// should reject that query.
func (g *groupi) BelongsToReadOnly(key string, ts uint64) (uint32, error) {
	g.RLock()
	tablet := g.tablets[key]
	g.RUnlock()
	if tablet != nil {
		if ts > 0 && ts < tablet.MoveTs {
			return 0, errors.Errorf("StartTs: %d is from before MoveTs: %d for pred: %q",
				ts, tablet.MoveTs, key)
		}
		return tablet.GetGroupId(), nil
	}

	// We don't know about this tablet. Talk to dgraphzero to find out who is
	// serving this tablet.
	pl := g.connToZeroLeader()
	zc := pb.NewZeroClient(pl.Get())

	tablet = &pb.Tablet{
		Predicate: key,
		ReadOnly:  true,
	}
	out, err := zc.ShouldServe(context.Background(), tablet)
	if err != nil {
		glog.Errorf("Error while ShouldServe grpc call %v", err)
		return 0, err
	}
	if out.GetGroupId() == 0 {
		return 0, nil
	}

	g.Lock()
	defer g.Unlock()
	g.tablets[key] = out
	if out != nil && ts > 0 && ts < out.MoveTs {
		return 0, errors.Errorf("StartTs: %d is from before MoveTs: %d for pred: %q",
			ts, out.MoveTs, key)
	}
	return out.GetGroupId(), nil
}

func (g *groupi) ServesTablet(key string) (bool, error) {
	if tablet, err := g.Tablet(key); err != nil {
		return false, err
	} else if tablet != nil && tablet.GroupId == groups().groupId() {
		return true, nil
	}
	return false, nil
}

// Do not modify the returned Tablet
func (g *groupi) Tablet(key string) (*pb.Tablet, error) {
	emptyTablet := pb.Tablet{}

	// TODO: Remove all this later, create a membership state and apply it
	g.RLock()
	tablet, ok := g.tablets[key]
	g.RUnlock()
	if ok {
		return tablet, nil
	}

	// We don't know about this tablet.
	// Check with dgraphzero if we can serve it.
	pl := g.connToZeroLeader()
	zc := pb.NewZeroClient(pl.Get())

	tablet = &pb.Tablet{GroupId: g.groupId(), Predicate: key}
	out, err := zc.ShouldServe(context.Background(), tablet)
	if err != nil {
		glog.Errorf("Error while ShouldServe grpc call %v", err)
		return &emptyTablet, err
	}

	// Do not store tablets with group ID 0, as they are just dummy tablets for
	// predicates that do no exist.
	if out.GroupId > 0 {
		g.Lock()
		g.tablets[key] = out
		g.Unlock()
	}

	if out.GroupId == groups().groupId() {
		glog.Infof("Serving tablet for: %v\n", key)
	}
	return out, nil
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
	for _, m := range members {
		pl, err := conn.GetPools().Get(m.Addr)
		if err == nil {
			return pl
		}
	}
	return nil
}

func (g *groupi) MyPeer() (uint64, bool) {
	members := g.members(g.groupId())
	for _, m := range members {
		if m.Id != g.Node.Id {
			return m.Id, true
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
			if pl, err := conn.GetPools().Get(m.Addr); err == nil {
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

// KnownGroups returns the known groups using the global groupi instance.
func KnownGroups() []uint32 {
	return groups().KnownGroups()
}

// GroupId returns the group to which this worker belongs to.
func GroupId() uint32 {
	return groups().groupId()
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
		ctx, cancel := context.WithTimeout(g.ctx, 10*time.Second)
		defer cancel()

		connState, err := zc.Connect(ctx, &pb.Member{ClusterInfoOnly: true})
		if err != nil || connState == nil {
			glog.V(1).Infof("While retrieving Zero leader info. Error: %v. Retrying...", err)
			return nil
		}
		for _, mz := range connState.State.GetZeros() {
			if mz.Leader {
				return conn.GetPools().Connect(mz.GetAddr())
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
			pl = conn.GetPools().Connect(x.WorkerConfig.ZeroAddr)
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
	return errors.Errorf(string(reply.GetData()))
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
			if err := stream.CloseSend(); err != nil {
				glog.Errorf("Error closing send stream: %+v", err)
			}
			break OUTER
		case <-ctx.Done():
			if err := stream.CloseSend(); err != nil {
				glog.Errorf("Error closing send stream: %+v", err)
			}
			break OUTER
		case state := <-stateCh:
			lastRecv = time.Now()
			g.applyState(state)
		case <-ticker.C:
			if time.Since(lastRecv) > 10*time.Second {
				// Zero might have gone under partition. We should recreate our connection.
				glog.Warningf("No membership update for 10s. Closing connection to Zero.")
				if err := stream.CloseSend(); err != nil {
					glog.Errorf("Error closing send stream: %+v", err)
				}
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
			defer func() {
				if err := stream.CloseSend(); err != nil {
					glog.Errorf("Error closing send stream: %+v", err)
				}
			}()
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
				// Block forever trying to propose this. Also this proposal should not be counted
				// towards num pending proposals and be proposed right away.
				err := g.Node.proposeAndWait(context.Background(), &pb.Proposal{Delta: delta})
				if err == nil {
					break
				}
				glog.Errorf("While proposing delta with MaxAssigned: %d and num txns: %d."+
					" Error=%v. Retrying...\n", delta.MaxAssigned, len(delta.Txns), err)
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

// EnterpriseEnabled returns whether enterprise features can be used or not.
func EnterpriseEnabled() bool {
	if !enc.EeBuild {
		return false
	}
	g := groups()
	if g.state == nil {
		return askZeroForEE()
	}
	g.RLock()
	defer g.RUnlock()
	return g.state.GetLicense().GetEnabled()
}

func askZeroForEE() bool {
	var err error
	var connState *pb.ConnectionState

	grp := &groupi{}

	createConn := func() bool {
		grp.ctx, grp.cancel = context.WithCancel(context.Background())
		defer grp.cancel()

		pl := grp.connToZeroLeader()
		if pl == nil {
			return false
		}
		zc := pb.NewZeroClient(pl.Get())

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		connState, err = zc.Connect(ctx, &pb.Member{ClusterInfoOnly: true})
		if connState == nil ||
			connState.GetState() == nil ||
			connState.GetState().GetLicense() == nil {
			glog.Info("Retry Zero Connection")
			return false
		}
		if err == nil || x.ShouldCrash(err) {
			return true
		}
		return false
	}

	for {
		if createConn() {
			break
		}
		time.Sleep(time.Second)
	}
	return connState.GetState().GetLicense().GetEnabled()
}

// SubscribeForUpdates will listen for updates for the given group.
func SubscribeForUpdates(prefixes [][]byte, cb func(kvs *badgerpb.KVList), group uint32,
	closer *y.Closer) {
	defer closer.Done()

	for {
		select {
		case <-closer.HasBeenClosed():
			return
		default:

			// Connect to any of the group 1 nodes.
			members := groups().AnyTwoServers(group)
			// There may be a lag while starting so keep retrying.
			if len(members) == 0 {
				continue
			}
			pool := conn.GetPools().Connect(members[0])
			client := pb.NewWorkerClient(pool.Get())

			// Get Subscriber stream.
			stream, err := client.Subscribe(context.Background(),
				&pb.SubscriptionRequest{Prefixes: prefixes})
			if err != nil {
				glog.Errorf("Error from alpha client subscribe: %v", err)
				time.Sleep(100 * time.Millisecond)
				continue
			}
		receiver:
			for {
				// Listen for updates.
				kvs, err := stream.Recv()
				if err != nil {
					glog.Errorf("Error from worker subscribe stream: %v", err)
					break receiver
				}
				cb(kvs)
			}
		}
	}
}
