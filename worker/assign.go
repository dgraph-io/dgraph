/*
 * Copyright 2016 DGraph Labs, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * 		http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package worker

import (
	"golang.org/x/net/context"

	"github.com/dgraph-io/dgraph/group"
	"github.com/dgraph-io/dgraph/protos/taskp"
	"github.com/dgraph-io/dgraph/protos/workerp"
	"github.com/dgraph-io/dgraph/uid"
	"github.com/dgraph-io/dgraph/x"
)

var emptyNum taskp.Num

func createNumQuery(group uint32, umap map[string]uint64) *taskp.Num {
	out := &taskp.Num{Group: group}
	for _, v := range umap {
		if v != 0 {
			out.Uids = append(out.Uids, v)
		} else {
			out.Val++
		}
	}
	return out
}

// assignUids returns a byte slice containing uids.
// This function is triggered by an RPC call. We ensure that only leader can assign new UIDs,
// so we can tackle any collisions that might happen with the lockmanager.
// In essence, we just want one server to be handing out new uids.
func assignUids(ctx context.Context, num *taskp.Num) (*taskp.List, error) {
	node := groups().Node(num.Group)
	if !node.AmLeader() {
		return &emptyUIDList, x.Errorf("Assigning UIDs is only allowed on leader.")
	}

	val := int(num.Val)
	markNum := len(num.Uids)
	if val == 0 && markNum == 0 {
		return &emptyUIDList, x.Errorf("Nothing to be marked or assigned")
	}

	mutations := uid.AssignNew(val, num.Group)

	for _, uid := range num.Uids {
		mutations.Edges = append(mutations.Edges, &taskp.DirectedEdge{
			Entity: uid,
			Attr:   "_uid_",
			Value:  []byte("_"), // not txid
			Label:  "A",
			Op:     taskp.DirectedEdge_SET,
		})
	}

	proposal := &taskp.Proposal{Mutations: mutations}
	if err := node.ProposeAndWait(ctx, proposal); err != nil {
		return &emptyUIDList, err
	}
	// Mutations successfully applied.

	out := make([]uint64, 0, val)
	// Only the First N entities are newly assigned UIDs, so we collect them.
	for i := 0; i < val; i++ {
		out = append(out, mutations.Edges[i].Entity)
	}
	return &taskp.List{out}, nil
}

// AssignUidsOverNetwork assigns new uids and writes them to the umap.
func AssignUidsOverNetwork(ctx context.Context, umap map[string]uint64) error {
	gid := group.BelongsTo("_uid_")
	num := createNumQuery(gid, umap)

	var ul *taskp.List
	var err error
	n := groups().Node(gid)

	// This is useful for testing, when the membership information doesn't have chance
	// to propagate. Probably this is not needed since we block mutations until we do
	// at least one attempt of sync membership. It would fail in rare cases.
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

	x.AssertTruef(len(ul.Uids) == int(num.Val),
		"Requested: %d != Retrieved Uids: %d", num.Val, len(ul.Uids))

	i := 0
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
