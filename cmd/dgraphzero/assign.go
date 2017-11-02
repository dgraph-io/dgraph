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

	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/x"
)

var (
	emptyNum         protos.Num
	emptyAssignedIds protos.AssignedIds
)

const (
	leaseBandwidth = uint64(10000)
)

func (s *Server) updateLeases() {
	s.Lock()
	defer s.Unlock()
	s.nextLeaseId = s.state.MaxLeaseId + 1
	s.nextTxnTs = s.state.MaxTxnTs + 1
}

func (s *Server) maxLeaseId() uint64 {
	s.RLock()
	defer s.RUnlock()
	return s.state.MaxLeaseId
}

func (s *Server) maxTxnTs() uint64 {
	s.RLock()
	defer s.RUnlock()
	return s.state.MaxTxnTs
}

// lease would either allocate ids or timestamps.
// This function is triggered by an RPC call. We ensure that only leader can assign new UIDs,
// so we can tackle any collisions that might happen with the leasemanager
// In essence, we just want one server to be handing out new uids.
func (s *Server) lease(ctx context.Context, num *protos.Num, txn bool) (*protos.AssignedIds, error) {
	node := s.Node
	// TODO: Fix when we move to linearizable reads, need to check if we are the leader, might be
	// based on leader leases. If this node gets partitioned and unless checkquorum is enabled, this
	// node would still think that it's the leader.
	if !node.AmLeader() {
		return &emptyAssignedIds, x.Errorf("Assigning IDs is only allowed on leader.")
	}

	val := int(num.Val)
	if val == 0 {
		return &emptyAssignedIds, x.Errorf("Nothing to be marked or assigned")
	}

	s.leaseLock.Lock()
	defer s.leaseLock.Unlock()

	howMany := leaseBandwidth
	if num.Val > leaseBandwidth {
		howMany = num.Val + leaseBandwidth
	}

	if s.nextLeaseId == 0 || s.nextTxnTs == 0 {
		return nil, errors.New("Server not initialized.")
	}

	var maxLease, available uint64
	var proposal protos.ZeroProposal

	if txn {
		maxLease = s.maxTxnTs()
		available = maxLease - s.nextTxnTs + 1
		proposal.MaxTxnTs = maxLease + howMany
	} else {
		maxLease = s.maxLeaseId()
		available = maxLease - s.nextLeaseId + 1
		proposal.MaxLeaseId = maxLease + howMany
	}

	if available < num.Val {
		// Blocking propose to get more ids or timestamps.
		if err := s.Node.proposeAndWait(ctx, &proposal); err != nil {
			return nil, err
		}
	}

	out := &protos.AssignedIds{}
	if txn {
		out.StartId = s.nextTxnTs
		out.EndId = out.StartId + num.Val - 1
		s.nextTxnTs = out.EndId + 1
		s.orc.doneUntil.Begin(out.EndId)
	} else {
		out.StartId = s.nextLeaseId
		out.EndId = out.StartId + num.Val - 1
		s.nextLeaseId = out.EndId + 1
	}
	return out, nil
}

// AssignUids is used to assign new uids by communicating with the leader of the RAFT group
// responsible for handing out uids.
func (s *Server) AssignUids(ctx context.Context, num *protos.Num) (*protos.AssignedIds, error) {
	if ctx.Err() != nil {
		return &emptyAssignedIds, ctx.Err()
	}

	// TODO: Forward it to the leader, if I'm not the leader.
	reply := &emptyAssignedIds
	c := make(chan error, 1)
	go func() {
		var err error
		reply, err = s.lease(ctx, num, false)
		c <- err
	}()

	select {
	case <-ctx.Done():
		return reply, ctx.Err()
	case err := <-c:
		return reply, err
	}
}
