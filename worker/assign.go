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

	"golang.org/x/net/context"

	"github.com/dgraph-io/dgraph/group"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/taskp"
	"github.com/dgraph-io/dgraph/protos/workerp"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/uid"
	"github.com/dgraph-io/dgraph/x"
)

var (
	emptyNum        taskp.Num
	emptyListOption posting.ListOptions
	UidNotFound     = errors.New("Uid not found for xid")
)

func createNumQuery(group uint32, umap map[string]uint64) *taskp.Num {
	out := &taskp.Num{Group: group}
	for k := range umap {
		if strings.HasPrefix(k, "_:") {
			//generate new for this xid
			out.Val = out.Val + 1
		} else {
			out.Xids = append(out.Xids, k)
		}
	}
	return out
}

func getUid(ctx context.Context, xid string, group uint32) (uint64, error) {
	tokens, err := posting.IndexTokens("_xid_", "", types.Val{Tid: types.StringID, Value: []byte(xid)})
	if err != nil {
		return 0, err
	}
	x.AssertTruef(len(tokens) == 1, "_xid_ data corrupted")
	key := x.IndexKey("_xid_", tokens[0])
	pl, decr := posting.GetOrCreate(key, group)
	defer decr()
	uids := pl.Uids(emptyListOption)
	if len(uids.Uids) == 0 {
		return 0, UidNotFound
	}
	x.AssertTruef(len(uids.Uids) == 1, "_xid_ data corrupted")
	if uids.Uids[0] > 0 {
		return uids.Uids[0], nil
	}
	return 0, UidNotFound
}

func getUids(ctx context.Context, query *taskp.Num) (*taskp.List, error) {
	ul := &taskp.List{}
	for _, xid := range query.Xids {
		uid, err := getUid(ctx, xid, query.Group)
		if err != nil {
			return &emptyUIDList, err
		}
		ul.Uids = append(ul.Uids, uid)
	}
	return ul, nil
}

func (w *grpcWorker) GetUids(ctx context.Context, num *taskp.Num) (*taskp.List, error) {
	if ctx.Err() != nil {
		return &emptyUIDList, ctx.Err()
	}

	if !groups().ServesGroup(num.Group) {
		return &emptyUIDList, x.Errorf("groupId: %v. GetOrAssign. We shouldn't be getting this req", num.Group)
	}

	reply := &emptyUIDList
	c := make(chan error, 1)
	go func() {
		var err error
		reply, err = getUids(ctx, num)
		c <- err
	}()

	select {
	case <-ctx.Done():
		return reply, ctx.Err()
	case err := <-c:
		return reply, err
	}
}

func GetUidsOverNetwork(ctx context.Context, xids []string) (map[string]uint64, error) {
	gid := group.BelongsTo("_xid_")
	num := &taskp.Num{Group: gid, Xids: xids}
	xidMap := make(map[string]uint64)

	var ul *taskp.List
	var err error

	if groups().ServesGroup(gid) {
		ul, err = getUids(ctx, num)
		if err != nil {
			return xidMap, err
		}
	} else {
		lid, addr := groups().Leader(gid)
		x.Trace(ctx, "Not leader of group: %d. Sending to: %d", gid, lid)
		p := pools().get(addr)
		conn, err := p.Get()
		if err != nil {
			x.TraceError(ctx, x.Wrapf(err, "Error while retrieving connection"))
			return xidMap, err
		}
		defer p.Put(conn)
		x.Trace(ctx, "Calling AssignUids for group: %d, addr: %s", gid, addr)

		c := workerp.NewWorkerClient(conn)
		ul, err = c.GetUids(ctx, num)
		if err != nil {
			x.TraceError(ctx, x.Wrapf(err, "Error while getting uids"))
			return xidMap, err
		}
	}

	x.AssertTruef(len(ul.Uids) == len(xids),
		"Requested: %d != Retrieved Uids: %d", len(xids), len(ul.Uids))

	for i, xid := range xids {
		xidMap[xid] = ul.Uids[i]
	}
	return xidMap, nil
}

// AllocateOrGetUniqueUids assigns a new uid for xid if not already assigned or returns
// already assigned uid
func AllocateOrGetUniqueUids(ctx context.Context, xids []string, group uint32) ([]uint64, error) {
	num := len(xids)
	ids := make([]uint64, num, num)
	xidMap := make(map[string]uint64)
	var generate, wait []string
	// assuing max size
	ch := make(chan uid.XidAndUid, len(xids))
	for _, xid := range xids {
		// Check if uid has already been allocated from this xid
		if uid, err := getUid(ctx, xid, group); err == nil {
			xidMap[xid] = uid
			continue
		}
		// take a lock on xid so that we don't propose different uid for same xid
		ok := uid.LockManager().CanProposeUid(xid, ch)
		if id, err := getUid(ctx, xid, group); err == nil {
			if !ok {
				wait = append(wait, xid)
			} else {
				xidMap[xid] = id
				uid.LockManager().Done(xid, id)
			}
			continue
		}
		if !ok {
			wait = append(wait, xid)
		} else {
			generate = append(generate, xid)
		}
	}
	// generate required number of uids
	startId, err := assignNew(ctx, uint64(len(generate)), group)
	if err != nil {
		return ids, err
	}
	// populate uid to xid mapping
	mutations := &taskp.Mutations{GroupId: group}
	for _, xid := range generate {
		xidMap[xid] = startId
		mutations.Edges = append(mutations.Edges, &taskp.DirectedEdge{
			Entity:    startId,
			Attr:      "_xid_",
			Value:     []byte(xid),
			ValueType: uint32(types.StringID),
			Op:        taskp.DirectedEdge_SET,
		})
		startId += 1
	}

	if len(generate) > 0 {
		// persist mutations, iterate over them and mark xid as done
		proposal := &taskp.Proposal{Mutations: mutations}
		node := groups().Node(group)
		x.AssertTruef(node != nil, "Node doesn't serve group %d", group)
		if err := node.ProposeAndWait(ctx, proposal); err != nil {
			for _, edge := range mutations.Edges {
				uid.LockManager().Done(string(edge.Value), 0)
			}
			return ids, err
		}
		for _, edge := range mutations.Edges {
			uid.LockManager().Done(string(edge.Value), edge.Entity)
		}
	}
	// wait at the end to avoid circular dependencies between mutations
	for i := 0; i < len(wait); i++ {
		x.Trace(ctx, "waiting for uid assignment %v", wait)
		select {
		case xu := <-ch:
			if xu.Uid == 0 {
				return ids, x.Errorf("Error while generating uid for %s", xu.Xid)
			}
			xidMap[xu.Xid] = xu.Uid
		case <-ctx.Done():
			return ids, ctx.Err()
		}
	}
	close(ch)

	for i, xid := range xids {
		v, found := xidMap[xid]
		x.AssertTrue(found)
		ids[i] = v
	}

	return ids, nil
}

// assignUids returns a byte slice containing uids.
// This function is triggered by an RPC call. We ensure that only leader can assign new UIDs,
// so we can tackle any collisions that might happen with the leasemanager
// In essence, we just want one server to be handing out new uids.
func assignUids(ctx context.Context, num *taskp.Num) (*taskp.List, error) {
	node := groups().Node(num.Group)
	if !node.AmLeader() {
		return &emptyUIDList, x.Errorf("Assigning UIDs is only allowed on leader.")
	}

	val := int(num.Val)
	numXids := len(num.Xids)
	if val == 0 && numXids == 0 {
		return &emptyUIDList, x.Errorf("Nothing to be marked or assigned")
	}

	out := make([]uint64, val+numXids, val+numXids)
	che := make(chan error, 2)

	go func() {
		startId, err := assignNew(ctx, num.Val, num.Group)
		if err != nil {
			che <- err
			return
		}
		// First N entities are newly assigned UIDs, so we collect them.
		for i := uint64(0); i < num.Val; i++ {
			out[i] = startId + i
		}
		che <- nil
	}()

	go func() {
		ids, err := AllocateOrGetUniqueUids(ctx, num.Xids, num.Group)
		if err != nil {
			che <- err
			return
		}
		for i, id := range ids {
			out[num.Val+uint64(i)] = id
		}
		che <- nil
	}()

	for i := 0; i < 2; i++ {
		select {
		case err := <-che:
			if err != nil {
				return &emptyUIDList, err
			}
		case <-ctx.Done():
			return &emptyUIDList, ctx.Err()
		}
	}
	return &taskp.List{out}, nil
}

// AssignUidsOverNetwork assigns new uids and writes them to the umap.
func AssignUidsOverNetwork(ctx context.Context, umap map[string]uint64) error {
	gid := group.BelongsTo("_xid_")
	num := createNumQuery(gid, umap)

	var ul *taskp.List
	var err error
	n := groups().Node(gid)

	// This is useful for testing, when the membership information doesn't
	// have chance to propagate
	if n != nil && n.AmLeader() {
		x.Trace(ctx, "Calling assignUids as I'm leader of group: %d", gid)
		ul, err = assignUids(ctx, num)
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

		c := workerp.NewWorkerClient(conn)
		ul, err = c.AssignUids(ctx, num)
		if err != nil {
			x.TraceError(ctx, x.Wrapf(err, "Error while getting uids"))
			return err
		}
	}

	x.AssertTruef(len(ul.Uids) == len(umap),
		"Requested: %d != Retrieved Uids: %d", len(umap), len(ul.Uids))

	i := 0
	xidStart := int(num.Val)
	for i, xid := range num.Xids {
		umap[xid] = ul.Uids[xidStart+i]
	}
	// assign generated ones now
	for k, v := range umap {
		if v == 0 {
			umap[k] = ul.Uids[i] // Write uids to map.
			i++
		}
	}
	return nil
}

// AssignUids is used to assign new uids by communicating with the leader of the RAFT group
// responsible for handing out uids.
func (w *grpcWorker) AssignUids(ctx context.Context, num *taskp.Num) (*taskp.List, error) {
	if ctx.Err() != nil {
		return &emptyUIDList, ctx.Err()
	}

	if !groups().ServesGroup(num.Group) {
		return &emptyUIDList, x.Errorf("groupId: %v. GetOrAssign. We shouldn't be getting this req", num.Group)
	}

	reply := &emptyUIDList
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
