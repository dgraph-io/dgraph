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
	"sync"

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

	if xidList.XidsLength() == 0 {
		return uidList, fmt.Errorf("Empty xid list")
	}

	wg := new(sync.WaitGroup)
	uids := make([]uint64, xidList.XidsLength())
	che := make(chan error, xidList.XidsLength())
	for i := 0; i < xidList.XidsLength(); i++ {
		wg.Add(1)
		xid := string(xidList.Xids(i))

		// Start a goroutine to get uid for a xid.
		go func(idx int) {
			defer wg.Done()
			u, err := uid.GetOrAssign(xid, 0, 1)
			if err != nil {
				che <- err
				return
			}
			uids[idx] = u
		}(i)
	}
	// We want for goroutines to finish and then we create the uidlist vector.
	wg.Wait()
	close(che)
	for err := range che {
		x.Trace(ctx, "Error while getOrAssignUids: %v", err)
		return uidList, err
	}

	b := flatbuffers.NewBuilder(0)
	task.UidListStartUidsVector(b, xidList.XidsLength())
	for i := len(uids) - 1; i >= 0; i-- {
		b.PrependUint64(uids[i])
	}
	ve := b.EndVector(xidList.XidsLength())

	task.UidListStart(b)
	task.UidListAddUids(b, ve)
	uend := task.UidListEnd(b)
	b.Finish(uend)
	return b.Bytes[b.Head():], nil
}

// GetOrAssignUidsOverNetwork gets or assigns uids corresponding to xids and
// writes them to the xidToUid map.
func GetOrAssignUidsOverNetwork(ctx context.Context, xidToUid map[string]uint64) (rerr error) {
	query := new(Payload)
	query.Data = createXidListBuffer(xidToUid)
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
			x.Trace(ctx, "Error while retrieving connection: %v", err)
			return err
		}
		c := NewWorkerClient(conn)

		reply, rerr = c.GetOrAssign(context.Background(), query)
		if rerr != nil {
			x.Trace(ctx, "Error while getting uids: %v", rerr)
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
		xidToUid[xid] = uid
	}
	return nil
}
