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
	"golang.org/x/net/context"
	"golang.org/x/net/trace"

	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/x"
)

var (
	emptyNum         protos.Num
	emptyAssignedIds protos.AssignedIds
)

// assignUids returns a byte slice containing uids.
// This function is triggered by an RPC call. We ensure that only leader can assign new UIDs,
// so we can tackle any collisions that might happen with the leasemanager
// In essence, we just want one server to be handing out new uids.
func assignUids(ctx context.Context, num *protos.Num) (*protos.AssignedIds, error) {
	node := groups().Node(leaseGid)
	if !node.AmLeader() {
		return &emptyAssignedIds, x.Errorf("Assigning UIDs is only allowed on leader.")
	}

	val := int(num.Val)
	if val == 0 {
		return &emptyAssignedIds, x.Errorf("Nothing to be marked or assigned")
	}

	out := &protos.AssignedIds{}
	startId, err := leaseMgr().assignNewUids(ctx, num.Val)
	if err != nil {
		return out, err
	}
	out.StartId = startId
	out.EndId = startId + num.Val - 1
	return out, nil
}

// AssignUidsOverNetwork assigns new uids and writes them to the umap.
func AssignUidsOverNetwork(ctx context.Context, num *protos.Num) (*protos.AssignedIds, error) {
	n := groups().Node(leaseGid)

	// This is useful for testing, when the membership information doesn't
	// have chance to propagate
	if n != nil && n.AmLeader() {
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf("Calling assignUids as I'm leader of group: %d", leaseGid)
		}
		return assignUids(ctx, num)
	}
	lid, addr := groups().Leader(leaseGid)
	if tr, ok := trace.FromContext(ctx); ok {
		tr.LazyPrintf("Not leader of group: %d. Sending to: %d", leaseGid, lid)
	}
	p, err := pools().get(addr)
	if err != nil {
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf("Error while retrieving connection: %+v", err)
		}
		return &emptyAssignedIds, err
	}
	conn, err := p.Get()
	if err != nil {
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf("Error while retrieving connection: %+v", err)
		}
		return &emptyAssignedIds, err
	}
	defer p.Put(conn)
	if tr, ok := trace.FromContext(ctx); ok {
		tr.LazyPrintf("Calling AssignUids for group: %d, addr: %s", leaseGid, addr)
	}

	c := protos.NewWorkerClient(conn)
	return c.AssignUids(ctx, num)
}

// AssignUids is used to assign new uids by communicating with the leader of the RAFT group
// responsible for handing out uids.
func (w *grpcWorker) AssignUids(ctx context.Context, num *protos.Num) (*protos.AssignedIds, error) {
	if ctx.Err() != nil {
		return &emptyAssignedIds, ctx.Err()
	}

	if !groups().ServesGroup(leaseGid) {
		return &emptyAssignedIds, x.Errorf("groupId: %v. GetOrAssign. We shouldn't be getting this req", leaseGid)
	}

	reply := &emptyAssignedIds
	c := make(chan error, 1)
	go func() {
		var err error
		reply, err = assignUids(ctx, num)
		c <- err
	}()

	select {
	case <-ctx.Done():
		return reply, ctx.Err()
	case err := <-c:
		return reply, err
	}
}
