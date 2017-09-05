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

func (s *Server) createMembershipState() protos.MembershipState {
	return protos.MembershipState{}
}

// Connect is used to connect the very first time with group zero.
func (s *Server) Connect(ctx context.Context,
	m *protos.Member) (u *protos.MembershipState, err error) {
	if ctx.Err() != nil {
		return &emptyMembershipState, ctx.Err()
	}
	if m.Id == 0 {
		return u, errInvalidId
	}
	if len(m.Addr) == 0 {
		return u, errInvalidAddress
	}
	// Create a connection and check validity of the address by doing an Echo.
	pl := conn.Get().Connect(m.Addr)
	defer conn.Get().Release(pl)

	if err := conn.TestConnection(pl); err != nil {
		// TODO: How do we delete this connection pool?
		return u, err
	}
	x.Printf("Connection successful to addr: %s", m.Addr)

	s.Lock()
	defer s.Unlock()
	if m.GroupId > 0 {
		group, has := s.state.Groups[m.GroupId]
		if !has {
			// We don't have this group. Add the server to this group.
			group = new(protos.Group)
			group.Members = make(map[uint64]*protos.Member)
			group.Members[m.Id] = m
			s.state.Groups[m.GroupId] = group
			// TODO: Propose these updates to Raft before applying. Here and everywhere.
			return
		}

		if _, has := group.Members[m.Id]; has {
			group.Members[m.Id] = m // Update in case some fields have changed, like address.
			return
		}

		// We don't have this server in the list.

		if len(group.Members) < s.NumReplicas {
			// We need more servers here, so let's add it.
			group.Members[m.Id] = m
			// TODO: Update the cluster about this.
			return
		}
		// Already have plenty of servers serving this group.
	}

	// Let's assign this server to a new group.
	for gid, group := range s.state.Groups {
		if len(group.Members) < s.NumReplicas {
			m.GroupId = gid
			group.Members[m.Id] = m
			// TODO: Update the cluster about this.
			return
		}
	}

	// We either don't have any groups, or don't have any groups which need another member.
	m.GroupId = s.nextGroup
	s.nextGroup++
	group := new(protos.Group)
	group.Members = make(map[uint64]*protos.Member)
	group.Tablets = make(map[string]*protos.Tablet)
	group.Members[m.Id] = m
	s.state.Groups[m.GroupId] = group
	// TODO: Propose this to the raft cluster.
	return
}

func (s *Server) ShouldServe(
	ctx context.Context, tablet *protos.Tablet) (resp *protos.Tablet, err error) {

	if len(tablet.Predicate) == 0 {
		return resp, errInvalidQuery
	}

	s.Lock()
	defer s.Unlock()

	// Check who is serving this tablet.
	var tgroup uint32
	// TODO: Bring this back.
	// for gid, group := range s.groupMap {
	// 	// Slightly slow, but happens infrequently enough that it's OK.
	// 	for _, t := range group.tablets {
	// 		if t.Predicate == tablet.Predicate {
	// 			tgroup = gid
	// 		}
	// 	}
	// }

	// if tgroup == 0 {
	// 	// Set the tablet to be served by this server's group.
	// 	*resp = *tablet
	// 	resp.GroupId = tablet.GroupId
	// 	group, has := s.groupMap[tgroup]
	// 	if !has {
	// 		return resp, errInternalError
	// 	}
	// 	group.tablets = append(group.tablets, *tablet)
	// 	// TODO: Also propose and tell cluster about this.
	// 	return resp, nil
	// }

	// Someone is serving this tablet. Could be the caller as well.
	// The caller should compare the returned group against the group it holds to check who's
	// serving.
	*resp = *tablet
	resp.GroupId = tgroup
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
