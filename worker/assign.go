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
	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/x"
)

var (
	emptyNum        protos.Num
	emptyListOption posting.ListOptions
	UidNotFound     = errors.New("Uid not found for xid")
)

func createNumQuery(group uint32, umap map[string]uint64) *protos.Num {
	out := &protos.Num{Group: group}
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
		return result.UidMatrix[0].Uids[0], nil
	}
	return 0, UidNotFound
}

type resErr struct {
	xid string
	uid uint64
	err error
}

// AllocateOrGetUniqueUids assigns a new uid for xid if not already assigned or returns
// already assigned uid
func AllocateOrGetUniqueUids(ctx context.Context, xids []string) ([]uint64, error) {
	num := len(xids)
	ids := make([]uint64, num, num)
	xidMap := make(map[string]uint64)
	ch := make(chan resErr, num)

	for _, x := range xids {
		xid := x
		go func() {
			uid, err := LockManager().getUid(ctx, xid)
			ch <- resErr{xid: xid, uid: uid, err: err}
		}()
	}
	for i := 0; i < num; i++ {
		res := <-ch
		if res.err != nil {
			return ids, res.err
		}
		xidMap[res.xid] = res.uid
	}

	close(ch)
	for i, xid := range xids {
		v, found := xidMap[xid]
		x.AssertTruef(found, "uid not found/generated for xid")
		ids[i] = v
	}

	return ids, nil
}

// assignUids returns a byte slice containing uids.
// This function is triggered by an RPC call. We ensure that only leader can assign new UIDs,
// so we can tackle any collisions that might happen with the leasemanager
// In essence, we just want one server to be handing out new uids.
func assignUids(ctx context.Context, num *protos.Num) (*protos.List, error) {
	node := groups().Node(num.Group)
	x.AssertTrue(num.Group == gid)
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
		if num.Val == 0 {
			che <- nil
			return
		}
		startId, err := LeaseManager().assignNew(ctx, num.Val)
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
		ids, err := AllocateOrGetUniqueUids(ctx, num.Xids)
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
	return &protos.List{out}, nil
}

// AssignUidsOverNetwork assigns new uids and writes them to the umap.
func AssignUidsOverNetwork(ctx context.Context, umap map[string]uint64) error {
	gid := group.BelongsTo("_xid_")
	num := createNumQuery(gid, umap)

	var ul *protos.List
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

		c := protos.NewWorkerClient(conn)
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
func (w *grpcWorker) AssignUids(ctx context.Context, num *protos.Num) (*protos.List, error) {
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
