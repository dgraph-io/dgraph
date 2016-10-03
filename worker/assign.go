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
	"context"
	"fmt"
	"log"

	"github.com/google/flatbuffers/go"

	"github.com/dgraph-io/dgraph/task"
	"github.com/dgraph-io/dgraph/uid"
	"github.com/dgraph-io/dgraph/x"
)

func createQuery(group uint32, N int) []byte {
	b := flatbuffers.NewBuilder(0)
	task.NumStart(b)
	task.NumAddGroup(b, group)
	task.NumAddVal(b, int32(N))
	uo := task.NumEnd(b)
	b.Finish(uo)
	return b.Bytes[b.Head():]
}

// assignUids returns a byte slice containing uids.
func assignUids(ctx context.Context, num *task.Num) (uidList []byte, rerr error) {
	// This function is triggered by an RPC call. We ensure that only leader can assign new UIDs,
	// so we can tackle any collisions that might happen with the lockmanager.
	// In essence, we just want one server to be handing out new uids.
	if !GetNode().AmLeader() {
		return uidList, x.Errorf("Assigning UIDs is only allowed on leader.")
	}

	val := int(num.Val())
	if val == 0 {
		return uidList, fmt.Errorf("Empty xid list")
	}

	mutations := uid.AssignNew(val, 0, 1)
	data, err := mutations.Encode()
	x.Checkf(err, "While encoding mutation: %v", mutations)
	if err := GetNode().ProposeAndWait(ctx, mutationMsg, data); err != nil {
		return uidList, err
	}
	// Mutations successfully applied.

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

// GetOrAssignUidsOverNetwork gets or assigns uids corresponding to xids and
// writes them to the newUids map.
func AssignUidsOverNetwork(ctx context.Context, newUids map[string]uint64) (rerr error) {
	query := new(Payload)
	query.Data = createQuery(0, len(newUids)) // TODO: Fill in the right group.

	uo := flatbuffers.GetUOffsetT(query.Data)
	num := new(task.Num)
	num.Init(query.Data, uo)

	reply := new(Payload)
	// If instance number 0 called this and then it already has posting list for
	// xid <-> uid conversion, hence call getOrAssignUids locally else call
	// GetOrAssign using worker client.
	// if ws.groupId == 0 {
	// HACK HACK HACK
	if true {
		reply.Data, rerr = assignUids(ctx, num)
		if rerr != nil {
			return rerr
		}

	} else {
		// Get pool for worker on instance 0.
		pool := ws.GetPool(0)
		conn, err := pool.Get()
		if err != nil {
			x.TraceError(ctx, x.Wrapf(err, "Error while retrieving connection"))
			return err
		}
		c := NewWorkerClient(conn)

		reply, rerr = c.AssignUids(context.Background(), query)
		if rerr != nil {
			x.TraceError(ctx, x.Wrapf(rerr, "Error while getting uids"))
			return rerr
		}
	}

	uidList := new(task.UidList)
	uo = flatbuffers.GetUOffsetT(reply.Data)
	uidList.Init(reply.Data, uo)

	if uidList.UidsLength() != int(num.Val()) {
		log.Fatalf("Requested: %d != Retrieved Uids: %d", num.Val(), uidList.UidsLength())
	}

	i := 0
	for k := range newUids {
		uid := uidList.Uids(i)
		newUids[k] = uid // Write uids to map.
		i++
	}
	return nil
}
