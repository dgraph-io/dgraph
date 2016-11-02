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
	"fmt"
	"log"
	"time"

	"github.com/google/flatbuffers/go"
	"golang.org/x/net/context"

	"github.com/dgraph-io/dgraph/task"
	"github.com/dgraph-io/dgraph/uid"
	"github.com/dgraph-io/dgraph/x"
)

func createNumQuery(group uint32, umap map[string]uint64) []byte {
	b := flatbuffers.NewBuilder(0)

	count1, count2 := 0, 0
	for _, v := range umap {
		if v == 0 {
			count1++
		} else {
			count2++
		}
	}

	task.NumStartUidsVector(b, count2)
	for _, v := range umap {
		if v != 0 {
			b.PrependUint64(v)
		}
	}
	mlist := b.EndVector(count2)

	task.NumStart(b)
	task.NumAddGroup(b, group)
	task.NumAddVal(b, int32(count1))
	task.NumAddUids(b, mlist)
	uo := task.NumEnd(b)
	b.Finish(uo)
	return b.Bytes[b.Head():]
}

// assignUids returns a byte slice containing uids.
// This function is triggered by an RPC call. We ensure that only leader can assign new UIDs,
// so we can tackle any collisions that might happen with the lockmanager.
// In essence, we just want one server to be handing out new uids.
func assignUids(ctx context.Context, num *task.Num) (uidList []byte, rerr error) {
	node := groups().Node(num.Group())
	if !node.AmLeader() {
		return uidList, x.Errorf("Assigning UIDs is only allowed on leader.")
	}

	val := int(num.Val())
	markNum := num.UidsLength()
	if val == 0 && markNum == 0 {
		return uidList, fmt.Errorf("Nothing to be marked or assigned")
	}

	mutations := uid.AssignNew(val, 0, 1)

	for i := 0; i < markNum; i++ {
		uid := num.Uids(i)
		mut := x.DirectedEdge{
			Entity:    uid,
			Attribute: "_uid_",
			Value:     []byte("_taken_"), // not txid
			Source:    "_XIDorUSER_",
			Timestamp: time.Now(),
		}
		mutations.Set = append(mutations.Set, mut)
	}

	data, err := mutations.Encode()
	x.Checkf(err, "While encoding mutation: %v", mutations)
	if err := node.ProposeAndWait(ctx, mutationMsg, data); err != nil {
		return uidList, err
	}
	// Mutations successfully applied.

	// First N entities are newly assigned UIDs.
	b := flatbuffers.NewBuilder(0)
	task.UidListStartUidsVector(b, val)
	for i := 0; i < val; i++ {
		b.PrependUint64(mutations.Set[i].Entity)
	}
	ve := b.EndVector(val)

	task.UidListStart(b)
	task.UidListAddUids(b, ve)
	uend := task.UidListEnd(b)
	b.Finish(uend)
	return b.Bytes[b.Head():], nil
}

// AssignUidsOverNetwork assigns new uids and writes them to the umap.
func AssignUidsOverNetwork(ctx context.Context, umap map[string]uint64) (rerr error) {
	query := new(Payload)
	gid := BelongsTo("_uid_")
	query.Data = createNumQuery(gid, umap)

	num := task.GetRootAsNum(query.Data, 0)
	reply := new(Payload)
	if groups().ServesGroup(gid) {
		reply.Data, rerr = assignUids(ctx, num)
		if rerr != nil {
			return rerr
		}

	} else {
		addr := groups().Leader(gid)
		p := pools().get(addr)
		conn, err := p.Get()
		if err != nil {
			x.TraceError(ctx, x.Wrapf(err, "Error while retrieving connection"))
			return err
		}
		defer p.Put(conn)

		c := NewWorkerClient(conn)
		reply, rerr = c.AssignUids(ctx, query)
		if rerr != nil {
			x.TraceError(ctx, x.Wrapf(rerr, "Error while getting uids"))
			return rerr
		}
	}

	ul := task.GetRootAsUidList(reply.Data, 0)
	x.Assertf(ul.UidsLength() == int(num.Val()),
		"Requested: %d != Retrieved Uids: %d", num.Val(), ul.UidsLength())

	i := 0
	for k, v := range umap {
		if v == 0 {
			uid := ul.Uids(i)
			umap[k] = uid // Write uids to map.
			i++
		}
	}
	return nil
}

// AssignUids is used to assign new uids by communicating with the leader of the RAFT group
// responsible for handing out uids.
func (w *grpcWorker) AssignUids(ctx context.Context, query *Payload) (*Payload, error) {
	if ctx.Err() != nil {
		return &Payload{}, ctx.Err()
	}

	num := task.GetRootAsNum(query.Data, 0)
	if !groups().ServesGroup(num.Group()) {
		log.Fatalf("groupId: %v. GetOrAssign. We shouldn't be getting this req", num.Group())
	}

	reply := new(Payload)
	c := make(chan error, 1)
	go func() {
		var err error
		reply.Data, err = assignUids(ctx, num)
		c <- err
	}()

	select {
	case <-ctx.Done():
		return reply, ctx.Err()
	case err := <-c:
		return reply, err
	}
}
