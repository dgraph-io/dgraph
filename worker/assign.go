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
	"strings"

	"github.com/google/flatbuffers/go"

	"github.com/dgraph-io/dgraph/task"
	"github.com/dgraph-io/dgraph/uid"
	"github.com/dgraph-io/dgraph/x"
)

func createXidListBuffer(xids map[string]uint64) []byte {
	b := flatbuffers.NewBuilder(0)
	var offsets []flatbuffers.UOffsetT
	for xid := range xids {
		uo := b.CreateString(xid)
		offsets = append(offsets, uo)
	}

	task.XidListStartXidsVector(b, len(offsets))
	for _, uo := range offsets {
		b.PrependUOffsetT(uo)
	}
	ve := b.EndVector(len(offsets))

	task.XidListStart(b)
	task.XidListAddXids(b, ve)
	lo := task.XidListEnd(b)
	b.Finish(lo)
	return b.Bytes[b.Head():]
}

// getOrAssignUids returns a byte slice containing uids corresponding to the
// xids in the xidList.
func getOrAssignUids(ctx context.Context,
	xidList *task.XidList) (uidList []byte, rerr error) {
	// This function is triggered by an RPC call. We ensure that only leader can assign new UIDs,
	// so we can tackle any collisions that might happen with the lockmanager.
	// In essence, we just want one server to be handing out new uids.
	if !GetNode().AmLeader() {
		return uidList, x.Errorf("Assigning UIDs is only allowed on leader.")
	}

	if xidList.XidsLength() == 0 {
		return uidList, fmt.Errorf("Empty xid list")
	}

	// Just verify that we're only asking for _new_ ids.
	for i := 0; i < xidList.XidsLength(); i++ {
		xid := string(xidList.Xids(i))
		x.Assertf(strings.HasPrefix(xid, "_new_:"), "UIDs over network can only be assigned for _new_")
	}

	mutations := uid.AssignNew(xidList.XidsLength(), 0, 1)
	data, err := mutations.Encode()
	x.Checkf(err, "While encoding mutation: %v", mutations)
	if err := GetNode().ProposeAndWait(ctx, mutationMsg, data); err != nil {
		return uidList, err
	}
	// Mutations successfully applied.

	b := flatbuffers.NewBuilder(0)
	task.UidListStartUidsVector(b, xidList.XidsLength())
	for i := 0; i < xidList.XidsLength(); i++ {
		b.PrependUint64(mutations.Set[i].Entity)
	}
	ve := b.EndVector(xidList.XidsLength())

	task.UidListStart(b)
	task.UidListAddUids(b, ve)
	uend := task.UidListEnd(b)
	b.Finish(uend)
	return b.Bytes[b.Head():], nil
}

// GetOrAssignUidsOverNetwork gets or assigns uids corresponding to xids and
// writes them to the newUids map.
func GetOrAssignUidsOverNetwork(ctx context.Context, newUids map[string]uint64) (rerr error) {
	query := new(Payload)
	query.Data = createXidListBuffer(newUids)
	uo := flatbuffers.GetUOffsetT(query.Data)
	xidList := new(task.XidList)
	xidList.Init(query.Data, uo)

	reply := new(Payload)
	// If instance number 0 called this and then it already has posting list for
	// xid <-> uid conversion, hence call getOrAssignUids locally else call
	// GetOrAssign using worker client.
	// if ws.groupId == 0 {
	// HACK HACK HACK
	if true {
		reply.Data, rerr = getOrAssignUids(ctx, xidList)
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

		reply, rerr = c.GetOrAssign(context.Background(), query)
		if rerr != nil {
			x.TraceError(ctx, x.Wrapf(rerr, "Error while getting uids"))
			return rerr
		}
	}

	uidList := new(task.UidList)
	uo = flatbuffers.GetUOffsetT(reply.Data)
	uidList.Init(reply.Data, uo)

	if xidList.XidsLength() != uidList.UidsLength() {
		log.Fatalf("Xids: %d != Uids: %d", xidList.XidsLength(), uidList.UidsLength())
	}
	for i := 0; i < xidList.XidsLength(); i++ {
		xid := string(xidList.Xids(i))
		uid := uidList.Uids(i)
		// Writing uids to the map.
		newUids[xid] = uid
	}
	return nil
}
