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
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/task"
	"github.com/dgraph-io/dgraph/x"
	"github.com/dgryski/go-farm"
	"github.com/google/flatbuffers/go"
	"golang.org/x/net/context"
)

func ProcessTaskOverNetwork(qu []byte) (result []byte, rerr error) {
	uo := flatbuffers.GetUOffsetT(qu)
	q := new(task.Query)
	q.Init(qu, uo)

	attr := string(q.Attr())
	idx := farm.Fingerprint64([]byte(attr)) % numInstances

	var runHere bool
	if attr == "_xid_" || attr == "_uid_" {
		idx = 0
		runHere = (instanceIdx == 0)
	} else {
		runHere = (instanceIdx == idx)
	}
	glog.WithField("runHere", runHere).WithField("attr", attr).
		WithField("instanceIdx", instanceIdx).
		WithField("numInstances", numInstances).Info("ProcessTaskOverNetwork")

	if runHere {
		// No need for a network call, as this should be run from within
		// this instance.
		return processTask(qu)
	}

	pool := pools[idx]
	addr := pool.Addr
	query := new(Payload)
	query.Data = qu

	conn, err := pool.Get()
	if err != nil {
		return []byte(""), err
	}
	defer pool.Put(conn)
	c := NewWorkerClient(conn)
	reply, err := c.ServeTask(context.Background(), query)
	if err != nil {
		glog.WithField("call", "Worker.ServeTask").Error(err)
		return []byte(""), err
	}

	glog.WithField("reply_len", len(reply.Data)).WithField("addr", addr).
		WithField("attr", attr).Info("Got reply from server")
	return reply.Data, nil
}

func processTask(query []byte) (result []byte, rerr error) {
	uo := flatbuffers.GetUOffsetT(query)
	q := new(task.Query)
	q.Init(query, uo)

	attr := string(q.Attr())
	store := dataStore
	if attr == "_xid_" {
		store = uidStore
	}

	b := flatbuffers.NewBuilder(0)
	voffsets := make([]flatbuffers.UOffsetT, q.UidsLength())
	uoffsets := make([]flatbuffers.UOffsetT, q.UidsLength())

	for i := 0; i < q.UidsLength(); i++ {
		uid := q.Uids(i)
		key := posting.Key(uid, attr)
		pl := posting.GetOrCreate(key, store)

		var valoffset flatbuffers.UOffsetT
		if val, err := pl.Value(); err != nil {
			valoffset = b.CreateByteVector(x.Nilbyte)
		} else {
			valoffset = b.CreateByteVector(val)
		}
		task.ValueStart(b)
		task.ValueAddVal(b, valoffset)
		voffsets[i] = task.ValueEnd(b)

		ulist := pl.GetUids(int(q.Offset()), int(q.Count()))
		uoffsets[i] = x.UidlistOffset(b, ulist)
	}
	task.ResultStartValuesVector(b, len(voffsets))
	for i := len(voffsets) - 1; i >= 0; i-- {
		b.PrependUOffsetT(voffsets[i])
	}
	valuesVent := b.EndVector(len(voffsets))

	task.ResultStartUidmatrixVector(b, len(uoffsets))
	for i := len(uoffsets) - 1; i >= 0; i-- {
		b.PrependUOffsetT(uoffsets[i])
	}
	matrixVent := b.EndVector(len(uoffsets))

	task.ResultStart(b)
	task.ResultAddValues(b, valuesVent)
	task.ResultAddUidmatrix(b, matrixVent)
	rend := task.ResultEnd(b)
	b.Finish(rend)
	return b.Bytes[b.Head():], nil
}
