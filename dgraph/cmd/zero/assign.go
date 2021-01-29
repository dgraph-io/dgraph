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
	"context"

	otrace "go.opencensus.io/trace"

	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/x"
	"github.com/golang/glog"
	"github.com/pkg/errors"
)

var emptyAssignedIds pb.AssignedIds

const (
	leaseBandwidth = uint64(10000)
)

func (s *Server) updateLeases() {
	var startTs uint64
	s.Lock()
	s.nextLease[pb.Num_UID] = s.state.MaxUID + 1
	s.nextLease[pb.Num_TXN_TS] = s.state.MaxTxnTs + 1
	s.nextLease[pb.Num_NS_ID] = s.state.MaxNsID + 1

	startTs = s.nextLease[pb.Num_TXN_TS]
	glog.Infof("Updated UID: %d. Txn Ts: %d. NsID: %d.",
		s.nextLease[pb.Num_UID], s.nextLease[pb.Num_TXN_TS], s.nextLease[pb.Num_NS_ID])
	s.Unlock()
	s.orc.updateStartTxnTs(startTs)
}

func (s *Server) maxLease(typ pb.NumLeaseType) uint64 {
	s.RLock()
	defer s.RUnlock()
	var maxlease uint64
	switch typ {
	case pb.Num_UID:
		maxlease = s.state.MaxUID
	case pb.Num_TXN_TS:
		maxlease = s.state.MaxTxnTs
	case pb.Num_NS_ID:
		maxlease = s.state.MaxNsID
	}
	return maxlease
}

var errServedFromMemory = errors.New("Lease was served from memory")

// lease would either allocate ids or timestamps.
// This function is triggered by an RPC call. We ensure that only leader can assign new UIDs,
// so we can tackle any collisions that might happen with the leasemanager
// In essence, we just want one server to be handing out new uids.
func (s *Server) lease(ctx context.Context, num *pb.Num) (*pb.AssignedIds, error) {
	typ := num.GetType()
	node := s.Node
	// TODO: Fix when we move to linearizable reads, need to check if we are the leader, might be
	// based on leader leases. If this node gets partitioned and unless checkquorum is enabled, this
	// node would still think that it's the leader.
	if !node.AmLeader() {
		return &emptyAssignedIds, errors.Errorf("Assigning IDs is only allowed on leader.")
	}

	if num.Val == 0 && !num.ReadOnly {
		return &emptyAssignedIds, errors.Errorf("Nothing to be leased")
	}
	if glog.V(3) {
		glog.Infof("Got lease request for Type: %v. Num: %+v\n", typ, num)
	}

	s.leaseLock.Lock()
	defer s.leaseLock.Unlock()

	if typ == pb.Num_TXN_TS {
		if num.Val == 0 && num.ReadOnly {
			// If we're only asking for a readonly timestamp, we can potentially
			// service it directly.
			if glog.V(3) {
				glog.Infof("Attempting to serve read only txn ts [%d, %d]",
					s.readOnlyTs, s.nextLease[pb.Num_TXN_TS])
			}
			if s.readOnlyTs > 0 && s.readOnlyTs == s.nextLease[pb.Num_TXN_TS]-1 {
				return &pb.AssignedIds{ReadOnly: s.readOnlyTs}, errServedFromMemory
			}
		}
		// We couldn't service it. So, let's request an extra timestamp for
		// readonly transactions, if needed.
	}

	// If we're asking for more ids than the standard lease bandwidth, then we
	// should set howMany generously, so we can service future requests from
	// memory, without asking for another lease. Only used if we need to renew
	// our lease.
	howMany := leaseBandwidth
	if num.Val > leaseBandwidth {
		howMany = num.Val + leaseBandwidth
	}

	if s.nextLease[pb.Num_UID] == 0 || s.nextLease[pb.Num_TXN_TS] == 0 ||
		s.nextLease[pb.Num_NS_ID] == 0 {
		return nil, errors.New("Server not initialized")
	}

	var proposal pb.ZeroProposal

	// Calculate how many ids do we have available in memory, before we need to
	// renew our lease.
	maxLease := s.maxLease(typ)
	available := maxLease - s.nextLease[typ] + 1
	switch typ {
	case pb.Num_TXN_TS:
		proposal.MaxTxnTs = maxLease + howMany
	case pb.Num_UID:
		proposal.MaxUID = maxLease + howMany
	case pb.Num_NS_ID:
		proposal.MaxNsID = maxLease + howMany
	}

	// If we have less available than what we need, we need to renew our lease.
	if available < num.Val+1 { // +1 for a potential readonly ts.
		// Blocking propose to get more ids or timestamps.
		if err := s.Node.proposeAndWait(ctx, &proposal); err != nil {
			return nil, err
		}
	}

	out := &pb.AssignedIds{}
	if typ == pb.Num_TXN_TS {
		if num.Val > 0 {
			out.StartId = s.nextLease[pb.Num_TXN_TS]
			out.EndId = out.StartId + num.Val - 1
			s.nextLease[pb.Num_TXN_TS] = out.EndId + 1
		}
		if num.ReadOnly {
			s.readOnlyTs = s.nextLease[pb.Num_TXN_TS]
			s.nextLease[pb.Num_TXN_TS]++
			out.ReadOnly = s.readOnlyTs
		}
		s.orc.doneUntil.Begin(x.Max(out.EndId, out.ReadOnly))
	} else if typ == pb.Num_UID {
		out.StartId = s.nextLease[pb.Num_UID]
		out.EndId = out.StartId + num.Val - 1
		s.nextLease[pb.Num_UID] = out.EndId + 1
	} else if typ == pb.Num_NS_ID {
		out.StartId = s.nextLease[pb.Num_NS_ID]
		out.EndId = out.StartId + num.Val - 1
		s.nextLease[pb.Num_NS_ID] = out.EndId + 1

	} else {
		return out, errors.Errorf("Unknown lease type: %v\n", typ)
	}
	return out, nil
}

// AssignIds is used to assign new ids (UIDs, NsIDs) by communicating with the leader of the
// RAFT group responsible for handing out ids.
func (s *Server) AssignIds(ctx context.Context, num *pb.Num) (*pb.AssignedIds, error) {
	if ctx.Err() != nil {
		return &emptyAssignedIds, ctx.Err()
	}
	ctx, span := otrace.StartSpan(ctx, "Zero.AssignIds")
	defer span.End()

	reply := &emptyAssignedIds
	lease := func() error {
		var err error
		if s.Node.AmLeader() {
			span.Annotatef(nil, "Zero leader leasing %d ids", num.GetVal())
			reply, err = s.lease(ctx, num)
			return err
		}
		span.Annotate(nil, "Not Zero leader")
		// I'm not the leader and this request was forwarded to me by a peer, who thought I'm the
		// leader.
		if num.Forwarded {
			return errors.Errorf("Invalid Zero received AssignIds request forward. Please retry")
		}
		// This is an original request. Forward it to the leader.
		pl := s.Leader(0)
		if pl == nil {
			return errors.Errorf("No healthy connection found to Leader of group zero")
		}
		span.Annotatef(nil, "Sending request to %v", pl.Addr)
		zc := pb.NewZeroClient(pl.Get())
		num.Forwarded = true
		reply, err = zc.AssignIds(ctx, num)
		return err
	}

	c := make(chan error, 1)
	go func() {
		c <- lease()
	}()

	select {
	case <-ctx.Done():
		return &emptyAssignedIds, ctx.Err()
	case err := <-c:
		span.Annotatef(nil, "Error while leasing %+v: %v", num, err)
		return reply, err
	}
}
