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
	"sync"

	"github.com/dgraph-io/dgraph/task"
	"github.com/dgraph-io/dgraph/uid"
	"github.com/google/flatbuffers/go"
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

func getOrAssignUids(
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
	wg.Wait()
	close(che)
	for err := range che {
		glog.WithError(err).Error("Encountered errors while getOrAssignUids")
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

func GetOrAssignUidsOverNetwork(xidToUid *map[string]uint64) (rerr error) {
	query := new(conn.Query)
	query.Data = createXidListBuffer(*xidToUid)
	uo := flatbuffers.GetUOffsetT(query.Data)
	xidList := new(task.XidList)
	xidList.Init(query.Data, uo)

	reply := new(conn.Reply)
	if instanceIdx == 0 {
		uo := flatbuffers.GetUOffsetT(query.Data)
		xidList := new(task.XidList)
		xidList.Init(query.Data, uo)

		reply.Data, rerr = getOrAssignUids(xidList)
		if rerr != nil {
			return rerr
		}
	} else {
		pool := pools[0]
		if err := pool.Call("Worker.GetOrAssign", query, reply); err != nil {
			glog.WithField("method", "GetOrAssign").WithError(err).
				Error("While getting uids")
			return err
		}
	}

	uidList := new(task.UidList)
	uo = flatbuffers.GetUOffsetT(reply.Data)
	uidList.Init(reply.Data, uo)

	if xidList.XidsLength() != uidList.UidsLength() {
		glog.WithField("num_xids", xidList.XidsLength()).
			WithField("num_uids", uidList.UidsLength()).
			Fatal("Num xids don't match num uids")
	}
	for i := 0; i < xidList.XidsLength(); i++ {
		xid := string(xidList.Xids(i))
		uid := uidList.Uids(i)
		(*xidToUid)[xid] = uid
	}
	return nil
}
