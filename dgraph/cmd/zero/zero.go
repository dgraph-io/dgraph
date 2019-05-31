/*
 * Copyright 2017-2018 Dgraph Labs, Inc. and Contributors
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

package zero

import (
	"math"
	"sync"
	"time"

	otrace "go.opencensus.io/trace"
	"golang.org/x/net/context"

	"github.com/dgraph-io/badger/y"
	"github.com/dgraph-io/dgo/protos/api"
	"github.com/dgraph-io/dgraph/conn"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/x"
	"github.com/gogo/protobuf/proto"
	"github.com/golang/glog"
	"github.com/pkg/errors"
)

var (
	emptyConnectionState pb.ConnectionState
	errServerShutDown    = errors.New("Server is being shut down")
)

type Server struct {
	x.SafeMutex
	Node *node
	orc  *Oracle

	NumReplicas int
	state       *pb.MembershipState

	nextLeaseId uint64
	nextTxnTs   uint64
	readOnlyTs  uint64
	leaseLock   sync.Mutex // protects nextLeaseId, nextTxnTs and corresponding proposals.

	// groupMap    map[uint32]*Group
	nextGroup      uint32
	leaderChangeCh chan struct{}
	closer         *y.Closer  // Used to tell stream to close.
	connectLock    sync.Mutex // Used to serialize connect requests from servers.

	moveOngoing    chan struct{}
	blockCommitsOn *sync.Map
}

func (s *Server) Init() {
	s.Lock()
	defer s.Unlock()

	s.orc = &Oracle{}
	s.orc.Init()
	s.state = &pb.MembershipState{
		Groups: make(map[uint32]*pb.Group),
		Zeros:  make(map[uint64]*pb.Member),
	}
	s.nextLeaseId = 1
	s.nextTxnTs = 1
	s.nextGroup = 1
	s.leaderChangeCh = make(chan struct{}, 1)
	s.closer = y.NewCloser(2) // grpc and http
	s.blockCommitsOn = new(sync.Map)
	s.moveOngoing = make(chan struct{}, 1)
	go s.rebalanceTablets()
}

func (s *Server) periodicallyPostTelemetry() {
	glog.V(2).Infof("Starting telemetry data collection...")
	start := time.Now()

	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	var lastPostedAt time.Time
	for range ticker.C {
		if !s.Node.AmLeader() {
			continue
		}
		if time.Since(lastPostedAt) < time.Hour {
			continue
		}
		ms := s.membershipState()
		t := newTelemetry(ms)
		if t == nil {
			continue
		}
		t.SinceHours = int(time.Since(start).Hours())
		glog.V(2).Infof("Posting Telemetry data: %+v", t)

		err := t.post()
		glog.V(2).Infof("Telemetry data posted with error: %v", err)
		if err == nil {
			lastPostedAt = time.Now()
		}
	}
}

func (s *Server) triggerLeaderChange() {
	s.Lock()
	defer s.Unlock()
	close(s.leaderChangeCh)
	s.leaderChangeCh = make(chan struct{}, 1)
}

func (s *Server) leaderChangeChannel() chan struct{} {
	s.RLock()
	defer s.RUnlock()
	return s.leaderChangeCh
}

func (s *Server) member(addr string) *pb.Member {
	s.AssertRLock()
	for _, m := range s.state.Zeros {
		if m.Addr == addr {
			return m
		}
	}
	for _, g := range s.state.Groups {
		for _, m := range g.Members {
			if m.Addr == addr {
				return m
			}
		}
	}
	return nil
}

func (s *Server) Leader(gid uint32) *conn.Pool {
	s.RLock()
	defer s.RUnlock()
	if s.state == nil {
		return nil
	}
	var members map[uint64]*pb.Member
	if gid == 0 {
		members = s.state.Zeros
	} else {
		group := s.state.Groups[gid]
		if group == nil {
			return nil
		}
		members = group.Members
	}
	var healthyPool *conn.Pool
	for _, m := range members {
		if pl, err := conn.GetPools().Get(m.Addr); err == nil {
			healthyPool = pl
			if m.Leader {
				return pl
			}
		}
	}
	return healthyPool
}

func (s *Server) KnownGroups() []uint32 {
	var groups []uint32
	s.RLock()
	defer s.RUnlock()
	for group := range s.state.Groups {
		groups = append(groups, group)
	}
	return groups
}

func (s *Server) hasLeader(gid uint32) bool {
	s.AssertRLock()
	if s.state == nil {
		return false
	}
	group := s.state.Groups[gid]
	if group == nil {
		return false
	}
	for _, m := range group.Members {
		if m.Leader {
			return true
		}
	}
	return false
}

func (s *Server) SetMembershipState(state *pb.MembershipState) {
	s.Lock()
	defer s.Unlock()
	s.state = state
	if state.Zeros == nil {
		state.Zeros = make(map[uint64]*pb.Member)
	}
	if state.Groups == nil {
		state.Groups = make(map[uint32]*pb.Group)
	}
	// Create connections to all members.
	for _, g := range state.Groups {
		for _, m := range g.Members {
			conn.GetPools().Connect(m.Addr)
		}
		if g.Tablets == nil {
			g.Tablets = make(map[string]*pb.Tablet)
		}
	}
	s.nextGroup = uint32(len(state.Groups) + 1)
}

func (s *Server) MarshalMembershipState() ([]byte, error) {
	s.Lock()
	defer s.Unlock()
	return s.state.Marshal()
}

func (s *Server) membershipState() *pb.MembershipState {
	s.RLock()
	defer s.RUnlock()
	return proto.Clone(s.state).(*pb.MembershipState)
}

func (s *Server) groupChecksums() map[uint32]uint64 {
	s.RLock()
	defer s.RUnlock()
	m := make(map[uint32]uint64)
	for gid, g := range s.state.GetGroups() {
		m[gid] = g.Checksum
	}
	return m
}

func (s *Server) storeZero(m *pb.Member) {
	s.Lock()
	defer s.Unlock()

	s.state.Zeros[m.Id] = m
}

func (s *Server) updateZeroLeader() {
	s.Lock()
	defer s.Unlock()
	leader := s.Node.Raft().Status().Lead
	for _, m := range s.state.Zeros {
		m.Leader = m.Id == leader
	}
}

func (s *Server) removeZero(nodeId uint64) {
	s.Lock()
	defer s.Unlock()
	m, has := s.state.Zeros[nodeId]
	if !has {
		return
	}
	delete(s.state.Zeros, nodeId)
	s.state.Removed = append(s.state.Removed, m)
}

// ServingTablet returns the Tablet called tablet.
func (s *Server) ServingTablet(tablet string) *pb.Tablet {
	s.RLock()
	defer s.RUnlock()

	for _, group := range s.state.Groups {
		for key, tab := range group.Tablets {
			if key == tablet {
				return tab
			}
		}
	}
	return nil
}

func (s *Server) blockTablet(pred string) func() {
	s.blockCommitsOn.Store(pred, struct{}{})
	return func() {
		s.blockCommitsOn.Delete(pred)
	}
}

func (s *Server) isBlocked(pred string) bool {
	_, blocked := s.blockCommitsOn.Load(pred)
	return blocked
}

func (s *Server) servingTablet(tablet string) *pb.Tablet {
	s.AssertRLock()

	for _, group := range s.state.Groups {
		for key, tab := range group.Tablets {
			if key == tablet {
				return tab
			}
		}
	}
	return nil
}

func (s *Server) createProposals(dst *pb.Group) ([]*pb.ZeroProposal, error) {
	var res []*pb.ZeroProposal
	if len(dst.Members) > 1 {
		return res, errors.Errorf("Create Proposal: Invalid group: %+v", dst)
	}

	s.RLock()
	defer s.RUnlock()
	// There is only one member.
	for mid, dstMember := range dst.Members {
		group, has := s.state.Groups[dstMember.GroupId]
		if !has {
			return res, errors.Errorf("Unknown group for member: %+v", dstMember)
		}
		srcMember, has := group.Members[mid]
		if !has {
			return res, errors.Errorf("Unknown member: %+v", dstMember)
		}
		if srcMember.Addr != dstMember.Addr ||
			srcMember.Leader != dstMember.Leader {

			proposal := &pb.ZeroProposal{
				Member: dstMember,
			}
			res = append(res, proposal)
		}
		if !dstMember.Leader {
			// Don't continue to tablets if request is not from the leader.
			return res, nil
		}
		if dst.SnapshotTs > group.SnapshotTs {
			res = append(res, &pb.ZeroProposal{
				SnapshotTs: map[uint32]uint64{dstMember.GroupId: dst.SnapshotTs},
			})
		}
	}
	for key, dstTablet := range dst.Tablets {
		group, has := s.state.Groups[dstTablet.GroupId]
		if !has {
			return res, errors.Errorf("Unknown group for tablet: %+v", dstTablet)
		}
		srcTablet, has := group.Tablets[key]
		if !has {
			// Tablet moved to new group
			continue
		}

		s := float64(srcTablet.Space)
		d := float64(dstTablet.Space)
		if dstTablet.Remove || (s == 0 && d > 0) || (s > 0 && math.Abs(d/s-1) > 0.1) {
			dstTablet.Force = false
			proposal := &pb.ZeroProposal{
				Tablet: dstTablet,
			}
			res = append(res, proposal)
		}
	}
	return res, nil
}

// Its users responsibility to ensure that node doesn't come back again before calling the api.
func (s *Server) removeNode(ctx context.Context, nodeId uint64, groupId uint32) error {
	if groupId == 0 {
		return s.Node.ProposePeerRemoval(ctx, nodeId)
	}
	zp := &pb.ZeroProposal{}
	zp.Member = &pb.Member{Id: nodeId, GroupId: groupId, AmDead: true}
	if _, ok := s.state.Groups[groupId]; !ok {
		return errors.Errorf("No group with groupId %d found", groupId)
	}
	if _, ok := s.state.Groups[groupId].Members[nodeId]; !ok {
		return errors.Errorf("No node with nodeId %d found in group %d", nodeId, groupId)
	}
	if len(s.state.Groups[groupId].Members) == 1 && len(s.state.Groups[groupId].Tablets) > 0 {
		return errors.Errorf("Move all tablets from group %d before removing the last node", groupId)
	}

	return s.Node.proposeAndWait(ctx, zp)
}

// Connect is used to connect the very first time with group zero.
func (s *Server) Connect(ctx context.Context,
	m *pb.Member) (resp *pb.ConnectionState, err error) {
	// Ensures that connect requests are always serialized
	s.connectLock.Lock()
	defer s.connectLock.Unlock()
	glog.Infof("Got connection request: %+v\n", m)
	defer glog.Infof("Connected: %+v\n", m)

	if ctx.Err() != nil {
		err := errors.Errorf("Context has error: %v\n", ctx.Err())
		return &emptyConnectionState, err
	}
	ms, err := s.latestMembershipState(ctx)
	if err != nil {
		return nil, err
	}
	if m.ClusterInfoOnly {
		// This request only wants to access the membership state, and nothing else. Most likely
		// from our clients.
		cs := &pb.ConnectionState{
			State:      ms,
			MaxPending: s.orc.MaxPending(),
		}
		return cs, err
	}
	if len(m.Addr) == 0 {
		return &emptyConnectionState, errors.Errorf("NO_ADDR: No address provided: %+v", m)
	}

	for _, member := range ms.Removed {
		// It is not recommended to reuse RAFT ids.
		if member.GroupId != 0 && m.Id == member.Id {
			return &emptyConnectionState, errors.Errorf(
				"REUSE_RAFTID: Duplicate Raft ID %d to removed member: %+v", m.Id, member)
		}
	}

	for _, group := range ms.Groups {
		for _, member := range group.Members {
			switch {
			case member.Addr == m.Addr && m.Id == 0:
				glog.Infof("Found a member with the same address. Returning: %+v", member)
				conn.GetPools().Connect(m.Addr)
				return &pb.ConnectionState{
					State:  ms,
					Member: member,
				}, nil

			case member.Addr == m.Addr && member.Id != m.Id:
				// Same address. Different Id. If Id is zero, then it might be trying to connect for
				// the first time. We can just directly return the membership information.
				return nil, errors.Errorf("REUSE_ADDR: Duplicate address to existing member: %+v."+
					" Self: +%v", member, m)

			case member.Addr != m.Addr && member.Id == m.Id:
				// Same Id. Different address.
				if pl, err := conn.GetPools().Get(member.Addr); err == nil && pl.IsHealthy() {
					// Found a healthy connection.
					return nil, errors.Errorf("REUSE_RAFTID: Healthy connection to a member"+
						" with same ID: %+v", member)
				}
			}
		}
	}

	// Create a connection and check validity of the address by doing an Echo.
	conn.GetPools().Connect(m.Addr)

	createProposal := func() *pb.ZeroProposal {
		s.Lock()
		defer s.Unlock()

		proposal := new(pb.ZeroProposal)
		// Check if we already have this member.
		for _, group := range s.state.Groups {
			if _, has := group.Members[m.Id]; has {
				return nil
			}
		}
		if m.Id == 0 {
			m.Id = s.state.MaxRaftId + 1
			proposal.MaxRaftId = m.Id
		}

		// We don't have this member. So, let's see if it has preference for a group.
		if m.GroupId > 0 {
			group, has := s.state.Groups[m.GroupId]
			if !has {
				// We don't have this group. Add the server to this group.
				proposal.Member = m
				return proposal
			}

			if _, has := group.Members[m.Id]; has {
				proposal.Member = m // Update in case some fields have changed, like address.
				return proposal
			}

			// We don't have this server in the list.
			if len(group.Members) < s.NumReplicas {
				// We need more servers here, so let's add it.
				proposal.Member = m
				return proposal
			}
			// Already have plenty of servers serving this group.
		}
		// Let's assign this server to a new group.
		for gid, group := range s.state.Groups {
			if len(group.Members) < s.NumReplicas {
				m.GroupId = gid
				proposal.Member = m
				return proposal
			}
		}
		// We either don't have any groups, or don't have any groups which need another member.
		m.GroupId = s.nextGroup
		// We shouldn't increase nextGroup here as we don't know whether we have enough
		// replicas until proposal is committed and can cause issues due to race.
		proposal.Member = m
		return proposal
	}

	proposal := createProposal()
	if proposal != nil {
		if err := s.Node.proposeAndWait(ctx, proposal); err != nil {
			return &emptyConnectionState, err
		}
	}
	resp = &pb.ConnectionState{
		State:  s.membershipState(),
		Member: m,
	}
	return resp, nil
}

func (s *Server) ShouldServe(
	ctx context.Context, tablet *pb.Tablet) (resp *pb.Tablet, err error) {
	ctx, span := otrace.StartSpan(ctx, "Zero.ShouldServe")
	defer span.End()

	if len(tablet.Predicate) == 0 {
		return resp, errors.Errorf("Tablet predicate is empty in %+v", tablet)
	}
	if tablet.GroupId == 0 && !tablet.ReadOnly {
		return resp, errors.Errorf("Group ID is Zero in %+v", tablet)
	}

	// Check who is serving this tablet.
	tab := s.ServingTablet(tablet.Predicate)
	span.Annotatef(nil, "Tablet for %s: %+v", tablet.Predicate, tab)
	if tab != nil {
		// Someone is serving this tablet. Could be the caller as well.
		// The caller should compare the returned group against the group it holds to check who's
		// serving.
		return tab, nil
	}
	if tab == nil && tablet.ReadOnly {
		// Read-only requests should return an empty tablet instead of asking zero to serve
		// the predicate.
		return &pb.Tablet{}, nil
	}

	// Set the tablet to be served by this server's group.
	var proposal pb.ZeroProposal
	// Multiple Groups might be assigned to same tablet, so during proposal we will check again.
	tablet.Force = false
	if x.IsReservedPredicate(tablet.Predicate) {
		// Force all the reserved predicates to be allocated to group 1.
		// This is to make it easier to stream ACL updates to all alpha servers
		// since they only need to open one pipeline to receive updates for all
		// ACL predicates.
		// This will also make it easier to restore the reserved predicates after
		// a DropAll operation.
		tablet.GroupId = 1
	}
	proposal.Tablet = tablet
	if err := s.Node.proposeAndWait(ctx, &proposal); err != nil && err != errTabletAlreadyServed {
		span.Annotatef(nil, "While proposing tablet: %v", err)
		return tablet, err
	}
	tab = s.ServingTablet(tablet.Predicate)
	x.AssertTrue(tab != nil)
	span.Annotatef(nil, "Now serving tablet for %s: %+v", tablet.Predicate, tab)
	return tab, nil
}

func (s *Server) UpdateMembership(ctx context.Context, group *pb.Group) (*api.Payload, error) {
	proposals, err := s.createProposals(group)
	if err != nil {
		// Sleep here so the caller doesn't keep on retrying indefinitely, creating a busy
		// wait.
		time.Sleep(time.Second)
		glog.Errorf("Error while creating proposals in Update: %v\n", err)
		return nil, err
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	errCh := make(chan error, len(proposals))
	for _, pr := range proposals {
		go func(pr *pb.ZeroProposal) {
			errCh <- s.Node.proposeAndWait(ctx, pr)
		}(pr)
	}

	for range proposals {
		// We Don't care about these errors
		// Ideally shouldn't error out.
		if err := <-errCh; err != nil {
			glog.Errorf("Error while applying proposal in Update stream: %v\n", err)
			return nil, err
		}
	}

	if len(group.Members) == 0 {
		return &api.Payload{Data: []byte("OK")}, nil
	}
	select {
	case s.moveOngoing <- struct{}{}:
	default:
		// If a move is going on, don't do the next steps of deleting predicates.
		return &api.Payload{Data: []byte("OK")}, nil
	}
	defer func() {
		<-s.moveOngoing
	}()

	if err := s.deletePredicates(ctx, group); err != nil {
		glog.Warningf("While deleting predicates: %v", err)
	}
	return &api.Payload{Data: []byte("OK")}, nil
}

func (s *Server) deletePredicates(ctx context.Context, group *pb.Group) error {
	if group == nil || group.Tablets == nil {
		return nil
	}
	var gid uint32
	for _, tablet := range group.Tablets {
		gid = tablet.GroupId
		break
	}
	if gid == 0 {
		return errors.Errorf("Unable to find group")
	}
	state, err := s.latestMembershipState(ctx)
	if err != nil {
		return err
	}
	sg, ok := state.Groups[gid]
	if !ok {
		return errors.Errorf("Unable to find group: %d", gid)
	}

	pl := s.Leader(gid)
	if pl == nil {
		return errors.Errorf("Unable to reach leader of group: %d", gid)
	}
	wc := pb.NewWorkerClient(pl.Get())

	for pred := range group.Tablets {
		if _, found := sg.Tablets[pred]; found {
			continue
		}
		glog.Infof("Tablet: %v does not belong to group: %d. Sending delete instruction.",
			pred, gid)
		in := &pb.MovePredicatePayload{
			Predicate: pred,
			SourceGid: gid,
			DestGid:   0,
		}
		if _, err := wc.MovePredicate(ctx, in); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) StreamMembership(_ *api.Payload, stream pb.Zero_StreamMembershipServer) error {
	// Send MembershipState right away. So, the connection is correctly established.
	ctx := stream.Context()
	ms, err := s.latestMembershipState(ctx)
	if err != nil {
		return err
	}
	if err := stream.Send(ms); err != nil {
		return err
	}

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			// Send an update every second.
			ms, err := s.latestMembershipState(ctx)
			if err != nil {
				return err
			}
			if err := stream.Send(ms); err != nil {
				return err
			}
		case <-ctx.Done():
			return ctx.Err()
		case <-s.closer.HasBeenClosed():
			return errServerShutDown
		}
	}
}

func (s *Server) latestMembershipState(ctx context.Context) (*pb.MembershipState, error) {
	if err := s.Node.WaitLinearizableRead(ctx); err != nil {
		return nil, err
	}
	ms := s.membershipState()
	if ms == nil {
		return &pb.MembershipState{}, nil
	}
	return ms, nil
}
