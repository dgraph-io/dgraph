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

	"golang.org/x/net/context"

	"github.com/dgraph-io/dgraph/conn"
	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/raftwal"
	"github.com/dgraph-io/dgraph/x"
)

var (
	emptyMembershipState protos.MembershipState
	errInvalidId         = errors.New("Invalid server id")
	errInvalidAddress    = errors.New("Invalid address")
	errUnknownMember     = errors.New("Unknown cluster member")
	errInvalidQuery      = errors.New("Invalid query")
	errInternalError     = errors.New("Internal server error")
	errJoinCluster       = errors.New("Unable to join cluster")
)

type Server struct {
	x.SafeMutex
	wal  *raftwal.Wal
	Node *node

	NumReplicas int
	state       *protos.MembershipState
	// groupMap    map[uint32]*Group
	nextGroup uint32
}

// Do not modify the membership state out of this.
func (s *Server) membershipState() *protos.MembershipState {
	s.RLock()
	defer s.RUnlock()

	return s.state
}

func (s *Server) servingTablet(tablet *protos.Tablet) uint32 {
	s.RLock()
	s.RUnlock()

	for gid, group := range s.state.Groups {
		for key := range group.Tablets {
			if key == tablet.Predicate {
				return gid
			}
		}
	}
	return 0
}

// Connect is used to connect the very first time with group zero.
func (s *Server) Connect(ctx context.Context,
	m *protos.Member) (resp *protos.MembershipState, err error) {
	if ctx.Err() != nil {
		return &emptyMembershipState, ctx.Err()
	}
	if m.Id == 0 {
		return &emptyMembershipState, errInvalidId
	}
	if len(m.Addr) == 0 {
		return &emptyMembershipState, errInvalidAddress
	}
	// Create a connection and check validity of the address by doing an Echo.
	pl := conn.Get().Connect(m.Addr)
	defer conn.Get().Release(pl)

	createProposal := func() *protos.GroupProposal {
		s.Lock()
		defer s.Unlock()

		proposal := new(protos.GroupProposal)
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
	x.Printf("Proposing: %+v\n", proposal)
	if err := s.Node.proposeAndWait(ctx, proposal); err != nil {
		return &emptyMembershipState, err
	}
	return s.membershipState(), nil
}

func (s *Server) ShouldServe(
	ctx context.Context, tablet *protos.Tablet) (resp *protos.Tablet, err error) {

	if len(tablet.Predicate) == 0 || tablet.GroupId == 0 {
		return resp, errInvalidQuery
	}

	// Check who is serving this tablet.
	tgroup := s.servingTablet(tablet)

	if tgroup == 0 {
		// Set the tablet to be served by this server's group.
		var proposal protos.GroupProposal
		proposal.Tablet = tablet
		if err := s.Node.proposeAndWait(ctx, &proposal); err != nil {
			return nil, err
		}
	}

	// Someone is serving this tablet. Could be the caller as well.
	// The caller should compare the returned group against the group it holds to check who's
	// serving.
	gid := s.servingTablet(tablet)
	*resp = *tablet
	resp.GroupId = gid
	return resp, nil
}

func (s *Server) Update(
	ctx context.Context, member *protos.Member) (state *protos.MembershipState, err error) {
	if ctx.Err() != nil {
		return &emptyMembershipState, ctx.Err()
	}
	// groupMap[gid]*members
	// tablets[gid]*tablets
	return
}
