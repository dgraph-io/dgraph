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
	"github.com/dgryski/go-farm"
	"github.com/google/flatbuffers/go"
	"golang.org/x/net/context"

	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/task"
	"github.com/dgraph-io/dgraph/x"
)

const (
	_xid_ = "_xid_"
	_uid_ = "_uid_"
)

// ProcessTaskOverNetwork is used to process the query and get the result from
// the instance which stores posting list corresponding to the predicate in the
// query.
func ProcessTaskOverNetwork(ctx context.Context, qu []byte) (result []byte, rerr error) {
	uo := flatbuffers.GetUOffsetT(qu)
	q := new(task.Query)
	q.Init(qu, uo)

	attr := string(q.Attr())
	idx := farm.Fingerprint64([]byte(attr)) % numInstances

	var runHere bool
	// Posting list with xid -> uid and uid -> xid mapping is stored on instance 0.
	if attr == _xid_ || attr == _uid_ {
		idx = 0
		runHere = (instanceIdx == 0)
	} else {
		runHere = (instanceIdx == idx)
	}
	x.Trace(ctx, "runHere: %v attr: %v instanceIdx: %v numInstances: %v",
		runHere, attr, instanceIdx, numInstances)

	if runHere {
		// No need for a network call, as this should be run from within
		// this instance.
		return processTask(qu)
	}

	// Using a worker client for the instance idx, we get the result of the query.
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
		x.Trace(ctx, "Error while calling Worker.ServeTask: %v", err)
		return []byte(""), err
	}

	x.Trace(ctx, "Reply from server. length: %v Addr: %v Attr: %v",
		len(reply.Data), addr, attr)
	return reply.Data, nil
}

// processTask processes the query, accumulates and returns the result.
func processTask(query []byte) (result []byte, rerr error) {
	uo := flatbuffers.GetUOffsetT(query)
	q := new(task.Query)
	q.Init(query, uo)

	attr := string(q.Attr())
	store := dataStore
	if attr == _xid_ {
		store = uidStore
	}

	b := flatbuffers.NewBuilder(0)
	voffsets := make([]flatbuffers.UOffsetT, q.UidsLength())
	uoffsets := make([]flatbuffers.UOffsetT, q.UidsLength())
	var counts []uint64

	for i := 0; i < q.UidsLength(); i++ {
		uid := q.Uids(i)
		key := posting.Key(uid, attr)
		// Get or create the posting list for an entity, attribute combination.
		pl := posting.GetOrCreate(key, store)

		var valoffset flatbuffers.UOffsetT
		// If a posting list contains a value, we store that or else we store a nil
		// byte so that processing is consistent later.
		if val, err := pl.Value(); err != nil {
			valoffset = b.CreateByteVector(x.Nilbyte)
		} else {
			valoffset = b.CreateByteVector(val)
		}
		task.ValueStart(b)
		task.ValueAddVal(b, valoffset)
		voffsets[i] = task.ValueEnd(b)

		if q.GetCount() == 1 {
			count := uint64(pl.Length())
			counts = append(counts, count)
			// Add an empty UID list to make later processing consistent
			uoffsets[i] = x.UidlistOffset(b, []uint64{})
		} else {
			opts := posting.ListOptions{
				Offset:   int(q.Offset()),
				Count:    int(q.Count()),
				AfterUid: uint64(q.AfterUid()),
			}
			ulist := pl.Uids(opts)
			uoffsets[i] = x.UidlistOffset(b, ulist)
		}
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

	task.ResultStartCountVector(b, len(counts))
	for i := len(counts) - 1; i >= 0; i-- {
		b.PrependUint64(counts[i])
	}
	countsVent := b.EndVector(len(counts))

	task.ResultStart(b)
	task.ResultAddValues(b, valuesVent)
	task.ResultAddUidmatrix(b, matrixVent)
	task.ResultAddCount(b, countsVent)
	rend := task.ResultEnd(b)
	b.Finish(rend)
	return b.Bytes[b.Head():], nil
}
