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

package main

import (
	"errors"
	"fmt"
	"math"
	"sync"
	"time"

	"golang.org/x/net/context"

	"github.com/dgraph-io/dgraph/conn"
	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/raftwal"
	"github.com/dgraph-io/dgraph/x"
	"github.com/gogo/protobuf/proto"
)

var (
	emptyMembershipState protos.MembershipState
	errInvalidId         = errors.New("Invalid server id")
	errInvalidAddress    = errors.New("Invalid address")
	errInvalidQuery      = errors.New("Invalid query")
	errInternalError     = errors.New("Internal server error")
	errJoinCluster       = errors.New("Unable to join cluster")
	errUnknownMember     = errors.New("Unknown cluster member")
	errUpdatedMember     = errors.New("Cluster member has updated credentials.")
)

type Server struct {
	x.SafeMutex
	wal  *raftwal.Wal
	Node *node

	NumReplicas int
	state       *protos.MembershipState

	nextLeaseId uint64
	leaseLock   sync.Mutex // protects nextLeaseId and lease proposals.

	// groupMap    map[uint32]*Group
	nextGroup      uint32
	leaderChangeCh chan struct{}
}

func (s *Server) Init() {
	s.Lock()
	defer s.Unlock()

	s.state = &protos.MembershipState{
		Groups: make(map[uint32]*protos.Group),
		Zeros:  make(map[uint64]*protos.Member),
	}
	s.nextLeaseId = 1
	s.nextGroup = 1
	s.leaderChangeCh = make(chan struct{}, 1)
	go s.rebalanceTablets()
}

func (s *Server) Leader(gid uint32) *conn.Pool {
	s.RLock()
	defer s.RUnlock()
	if s.state == nil {
		return nil
	}
	group := s.state.Groups[gid]
	if group == nil {
		return nil
	}
	var healthyPool *conn.Pool
	for _, m := range group.Members {
		if pl, err := conn.Get().Get(m.Addr); err == nil {
			healthyPool = pl
			if m.Leader {
				return pl
			}
		}
	}
	return healthyPool
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

func (s *Server) SetMembershipState(state *protos.MembershipState) {
	s.Lock()
	defer s.Unlock()
	s.state = state
	if state.Zeros == nil {
		state.Zeros = make(map[uint64]*protos.Member)
	}
	if state.Groups == nil {
		state.Groups = make(map[uint32]*protos.Group)
	}
	s.nextGroup = uint32(len(state.Groups) + 1)
}

func (s *Server) MarshalMembershipState() ([]byte, error) {
	s.Lock()
	defer s.Unlock()
	return s.state.Marshal()
}

func (s *Server) membershipState() *protos.MembershipState {
	s.RLock()
	defer s.RUnlock()
	return proto.Clone(s.state).(*protos.MembershipState)
}

func (s *Server) storeZero(m *protos.Member) {
	s.Lock()
	defer s.Unlock()

	s.state.Zeros[m.Id] = m
}

func (s *Server) ServingTablet(dst string) *protos.Tablet {
	s.RLock()
	defer s.RUnlock()

	for _, group := range s.state.Groups {
		for key, tab := range group.Tablets {
			if key == dst {
				return tab
			}
		}
	}
	return nil
}

func (s *Server) servingTablet(dst string) *protos.Tablet {
	s.AssertRLock()

	for _, group := range s.state.Groups {
		for key, tab := range group.Tablets {
			if key == dst {
				return tab
			}
		}
	}
	return nil
}

func (s *Server) createProposals(dst *protos.Group) ([]*protos.ZeroProposal, error) {
	var res []*protos.ZeroProposal
	if len(dst.Members) > 1 {
		return res, errInvalidQuery
	}

	s.RLock()
	defer s.RUnlock()
	// There is only one member.
	for mid, dstMember := range dst.Members {
		group, has := s.state.Groups[dstMember.GroupId]
		if !has {
			return res, errUnknownMember
		}
		srcMember, has := group.Members[mid]
		if !has {
			return res, errUnknownMember
		}
		if srcMember.Addr != dstMember.Addr ||
			srcMember.Leader != dstMember.Leader ||
			srcMember.AmDead != dstMember.AmDead {

			proposal := &protos.ZeroProposal{
				Member: dstMember,
			}
			res = append(res, proposal)
		}
		if !dstMember.Leader {
			// Don't continue to tablets if request is not from the leader.
			return res, nil
		}
	}
	for key, dstTablet := range dst.Tablets {
		group, has := s.state.Groups[dstTablet.GroupId]
		if !has {
			return res, errUnknownMember
		}
		srcTablet, has := group.Tablets[key]
		if !has {
			// Tablet moved to new group
			continue
		}

		s := float64(srcTablet.Space)
		d := float64(dstTablet.Space)
		if (s == 0 && d > 0) || (s > 0 && math.Abs(d/s-1) > 0.1) {
			dstTablet.Force = false
			proposal := &protos.ZeroProposal{
				Tablet: dstTablet,
			}
			res = append(res, proposal)
		}
	}
	return res, nil
}

// Connect is used to connect the very first time with group zero.
func (s *Server) Connect(ctx context.Context,
	m *protos.Member) (resp *protos.MembershipState, err error) {
	x.Printf("Got connection request: %+v\n", m)
	if ctx.Err() != nil {
		return &emptyMembershipState, ctx.Err()
	}
	if m.Id == 0 {
		return &emptyMembershipState, errInvalidId
	}
	if len(m.Addr) == 0 {
		fmt.Println("No address provided.")
		return &emptyMembershipState, errInvalidAddress
	}
	// Create a connection and check validity of the address by doing an Echo.
	conn.Get().Connect(m.Addr)

	createProposal := func() *protos.ZeroProposal {
		s.Lock()
		defer s.Unlock()

		proposal := new(protos.ZeroProposal)
		// Check if we already have this member.
		for _, group := range s.state.Groups {
			if _, has := group.Members[m.Id]; has {
				return nil
			}
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
		s.nextGroup++
		proposal.Member = m
		return proposal
	}

	proposal := createProposal()
	if proposal != nil {
		if err := s.Node.proposeAndWait(ctx, proposal); err != nil {
			return &emptyMembershipState, err
		}
	}
	return s.membershipState(), nil
}

func (s *Server) ShouldServe(
	ctx context.Context, tablet *protos.Tablet) (resp *protos.Tablet, err error) {

	if len(tablet.Predicate) == 0 || tablet.GroupId == 0 {
		return resp, errInvalidQuery
	}

	// Check who is serving this tablet.
	tab := s.ServingTablet(tablet.Predicate)
	if tab != nil {
		// Someone is serving this tablet. Could be the caller as well.
		// The caller should compare the returned group against the group it holds to check who's
		// serving.
		return tab, nil
	}

	// Set the tablet to be served by this server's group.
	var proposal protos.ZeroProposal
	// Multiple Groups might be assigned to same tablet, so during proposal we will check again.
	tablet.Force = false
	proposal.Tablet = tablet
	if err := s.Node.proposeAndWait(ctx, &proposal); err != nil && err != errTabletAlreadyServed {
		return tablet, err
	}
	tab = s.ServingTablet(tablet.Predicate)
	return tab, nil
}

func (s *Server) receiveUpdates(stream protos.Zero_UpdateServer) error {
	for {
		group, err := stream.Recv()
		// Due to closeSend on client Side
		if group == nil {
			return nil
		}
		// Could be EOF also, but we don't care about error type.
		if err != nil {
			return err
		}
		proposals, err := s.createProposals(group)
		if err != nil {
			x.Printf("Error while creating proposals in stream %v\n", err)
			return err
		}

		errCh := make(chan error)
		for _, pr := range proposals {
			go func(pr *protos.ZeroProposal) {
				errCh <- s.Node.proposeAndWait(context.Background(), pr)
			}(pr)
		}

		for range proposals {
			// We Don't care about these errors
			// Ideally shouldn't error out.
			if err := <-errCh; err != nil {
				x.Printf("Error while applying proposal in update stream %v\n", err)
			}
		}
	}
}

func (s *Server) Update(stream protos.Zero_UpdateServer) error {
	che := make(chan error, 1)
	// Server side cancellation can only be done by existing the handler
	// since Recv is blocking we need to run it in a goroutine.
	go func() {
		che <- s.receiveUpdates(stream)
	}()

	// Check every minute that whether we caught upto read index or not.
	ticker := time.NewTicker(time.Minute)
	ctx := stream.Context()
	// node sends struct{} on this channel whenever membership state is updated
	changeCh := make(chan struct{}, 1)

	id := s.Node.RegisterForUpdates(changeCh)
	defer s.Node.Deregister(id)
	// Send MembershipState immediately after registering. (Or there could be race
	// condition between registering and change in membership state).
	ms, err := s.latestMembershipState(ctx)
	if err != nil {
		return err
	}
	if ms != nil {
		// grpc will error out during marshalling if we send nil.
		if err := stream.Send(ms); err != nil {
			return err
		}
	}

	for {
		select {
		case <-changeCh:
			ms, err := s.latestMembershipState(ctx)
			if err != nil {
				return err
			}
			// TODO: Don't send if only lease has changed.
			if err := stream.Send(ms); err != nil {
				return err
			}
		case err := <-che:
			// Error while receiving updates.
			return err
		case <-ticker.C:
			// Check Whether we caught upto read index or not.
			if _, err := s.latestMembershipState(ctx); err != nil {
				return err
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (s *Server) latestMembershipState(ctx context.Context) (*protos.MembershipState, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Second*10)
	defer cancel()
	if err := s.Node.WaitLinearizableRead(ctx); err != nil {
		return nil, err
	}
	return s.membershipState(), nil
}
