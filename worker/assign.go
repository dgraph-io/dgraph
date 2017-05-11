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
	"errors"
	"strings"
	"sync"

	"golang.org/x/net/context"

	"github.com/dgraph-io/dgraph/group"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/x"
)

var (
	emptyNum         protos.Num
	emptyListOption  posting.ListOptions
	emptyAssignedIds protos.AssignedIds
	UidNotFound      = errors.New("Uid not found for xid")
	pending          chan struct{}
)

func createNumQuery(group uint32, umap map[string]uint64) *protos.Num {
	out := &protos.Num{Group: group}
	for k := range umap {
		if strings.HasPrefix(k, "_:") {
			//generate new for this xid
			out.Val = out.Val + 1
			continue
		}
		out.Xids = append(out.Xids, k)
	}
	return out
}

func GetUid(ctx context.Context, xid string) (uint64, error) {
	out := &protos.Query{
		Attr:    "_xid_",
		SrcFunc: []string{"eq", "", xid},
	}
	var result *protos.Result
	var err error
	if groups().ServesGroup(gid) {
		result, err = processTask(ctx, out, gid)
	} else {
		result, err = ProcessTaskOverNetwork(ctx, out)
	}
	if err != nil {
		return 0, err
	}
	if len(result.UidMatrix) > 0 && len(result.UidMatrix[0].Uids) > 0 {
		x.AssertTrue(len(result.UidMatrix) == 1 &&
			len(result.UidMatrix[0].Uids) == 1)
		return result.UidMatrix[0].Uids[0], nil
	}
	return 0, UidNotFound
}

// assignUids returns a byte slice containing uids.
// This function is triggered by an RPC call. We ensure that only leader can assign new UIDs,
// so we can tackle any collisions that might happen with the leasemanager
// In essence, we just want one server to be handing out new uids.
func assignUids(ctx context.Context, num *protos.Num) (*protos.AssignedIds, error) {
	node := groups().Node(num.Group)
	x.AssertTrue(num.Group == gid)
	if !node.AmLeader() {
		return &emptyAssignedIds, x.Errorf("Assigning UIDs is only allowed on leader.")
	}

	val := int(num.Val)
	numXids := len(num.Xids)
	if val == 0 && numXids == 0 {
		return &emptyAssignedIds, x.Errorf("Nothing to be marked or assigned")
	}

	out := &protos.AssignedIds{}
	out.M = make(map[string]uint64)
	che := make(chan error, 1+numXids)
	var m sync.Mutex

	go func() {
		if num.Val == 0 {
			che <- nil
			return
		}
		startId, err := leaseMgr().assignNewUids(ctx, num.Val)
		if err != nil {
			che <- err
			return
		}
		out.StartId = startId
		out.EndId = startId + num.Val - 1
		che <- nil
	}()

	for _, xid := range num.Xids {
		go func(xid string) {
			// ensures that we don't launch too many goroutines
			pending <- struct{}{}
			defer func() { <-pending }()
			uid, err := GetUid(ctx, xid)
			if err == nil {
				m.Lock()
				defer m.Unlock()
				out.M[xid] = uid
				che <- nil
				return
			}
			uid, err = lockMgr().assignUidForXid(ctx, xid)
			if err != nil {
				che <- err
				return
			}
			m.Lock()
			defer m.Unlock()
			out.M[xid] = uid
			che <- nil
		}(xid)
	}

	for i := 0; i < 1+numXids; i++ {
		select {
		case err := <-che:
			if err != nil {
				return &emptyAssignedIds, err
			}
		case <-ctx.Done():
			return &emptyAssignedIds, ctx.Err()
		}
	}
	return out, nil
}

// AssignUidsOverNetwork assigns new uids and writes them to the umap.
func AssignUidsOverNetwork(ctx context.Context, umap map[string]uint64) error {
	gid := group.BelongsTo("_xid_")
	num := createNumQuery(gid, umap)

	var res *protos.AssignedIds
	var err error
	n := groups().Node(gid)

	// This is useful for testing, when the membership information doesn't
	// have chance to propagate
	if n != nil && n.AmLeader() {
		x.Trace(ctx, "Calling assignUids as I'm leader of group: %d", gid)
		res, err = assignUids(ctx, num)
		if err != nil {
			return x.Wrap(err)
		}

	} else {
		lid, addr := groups().Leader(gid)
		x.Trace(ctx, "Not leader of group: %d. Sending to: %d", gid, lid)
		p := pools().get(addr)
		conn, err := p.Get()
		if err != nil {
			x.TraceError(ctx, x.Wrapf(err, "Error while retrieving connection"))
			return err
		}
		defer p.Put(conn)
		x.Trace(ctx, "Calling AssignUids for group: %d, addr: %s", gid, addr)

		c := protos.NewWorkerClient(conn)
		res, err = c.AssignUids(ctx, num)
		if err != nil {
			x.TraceError(ctx, x.Wrapf(err, "Error while getting uids"))
			return err
		}
	}

	currId := res.StartId
	// assign generated ones now
	for k := range umap {
		if strings.HasPrefix(k, "_:") {
			x.AssertTrue(currId != 0 && currId <= res.EndId)
			umap[k] = currId
			currId++
			continue
		}
		uid, found := res.M[k]
		x.AssertTrue(found)
		umap[k] = uid
	}
	return nil
}

// AssignUids is used to assign new uids by communicating with the leader of the RAFT group
// responsible for handing out uids.
func (w *grpcWorker) AssignUids(ctx context.Context, num *protos.Num) (*protos.AssignedIds, error) {
	if ctx.Err() != nil {
		return &emptyAssignedIds, ctx.Err()
	}

	if !groups().ServesGroup(num.Group) {
		return &emptyAssignedIds, x.Errorf("groupId: %v. GetOrAssign. We shouldn't be getting this req", num.Group)
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
