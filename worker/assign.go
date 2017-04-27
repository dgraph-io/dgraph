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

const (
	// 0 and 1 are not allowed as uid value, so safe to use
	waitForUid     = 0
	generateNewUid = 1
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
			// generate new for this xid
			out.Val = out.Val + 1
		} else {
			out.Xids = append(out.Xids, k)
		}
	}
	return out
}

func getUid(xid string, group uint32) (uint64, error) {
	tokens, _ := posting.IndexTokens("_xid_", "", types.Val{Tid: types.StringID, Value: []byte(xid)})
	x.AssertTrue(len(tokens) == 1)
	key := x.IndexKey("_xid_", tokens[0])
	pl, decr := posting.GetOrCreate(key, group)
	defer decr()
	ul := pl.Uids(emptyListOption)
	if len(ul.Uids) > 0 {
		// TODO: Ensure user can't change xid until we take care of corner cases
		x.AssertTrue(len(ul.Uids) == 1)
		if ul.Uids[0] > 1 {
			return ul.Uids[0], nil
		}
		// don't allow lease edges to be fetched.
		// since xid's are always non-integers(strings which can't be parsed to integer),
		// indexing of lease edges won't conflict since lease values are always integers
		return 0, UidNotFound
	}
	return 0, UidNotFound
}

// AllocateOrGetUniqueUids assigns a new uid for xid if not already assigned or returns
// already assigned uid
func AllocateOrGetUniqueUids(ctx context.Context, xids []string, group uint32) ([]uint64, error) {
	num := len(xids)
	ids := make([]uint64, num, num)
	xidMap := make(map[string]uint64)
	var numRequired, numWaiting uint64
	ch := make(chan struct{})
	for _, xid := range xids {
		// Check if uid has already been allocated from this xid
		if uid, err := getUid(xid, group); err == nil {
			xidMap[xid] = uid
			continue
		}
		// take a lock on xid so that we don't propose different uid for same xid
		if ok := uid.LockManager().CanProposeUid(xid, ch); !ok {
			xidMap[xid] = waitForUid
			numWaiting++
		} else {
			xidMap[xid] = generateNewUid
			numRequired++
		}
	}

	// generate required number of uids
	startId, err := assignNew(ctx, numRequired, group)
	if err != nil {
		return ids, err
	}

	// wait for assignment
	for i := uint64(0); i < numWaiting; i++ {
		select {
		case <-ch:
			//pass
		case <-ctx.Done():
			return ids, ctx.Err()
		}
	}
	close(ch)

	// populate ids
	mutations := &taskp.Mutations{GroupId: group}
	for i, xid := range xids {
		v, found := xidMap[xid]
		x.AssertTrue(found)
		if v == generateNewUid {
			ids[i] = startId
			mutations.Edges = append(mutations.Edges, &taskp.DirectedEdge{
				Entity:    startId,
				Attr:      "_xid_",
				Value:     []byte(xid),
				ValueType: uint32(types.StringID),
				Op:        taskp.DirectedEdge_SET,
			})
			startId += 1
		} else if v == waitForUid {
			if uid, err := getUid(xid, group); err != nil {
				return ids, err
			} else {
				ids[i] = uid
			}
		} else {
			ids[i] = v
		}
	}

	if numRequired > 0 {
		// persist mutations, iterate over them and mark xid as done
		proposal := &taskp.Proposal{Mutations: mutations}
		node := groups().Node(group)
		x.AssertTruef(node != nil, "Node doesn't serve group %d", group)
		if err := node.ProposeAndWait(ctx, proposal); err != nil {
			return ids, err
		}
		for _, edge := range mutations.Edges {
			uid.LockManager().Done(string(edge.Value))
		}
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
	che := make(chan error)

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
	for k := range umap {
		umap[k] = ul.Uids[i] // Write uids to map.
		i++
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
