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
	"github.com/google/flatbuffers/go"
	"golang.org/x/net/context"

	"github.com/dgraph-io/dgraph/algo"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/task"
	"github.com/dgraph-io/dgraph/x"
)

// ProcessTaskOverNetwork is used to process the query and get the result from
// the instance which stores posting list corresponding to the predicate in the
// query.
func ProcessTaskOverNetwork(ctx context.Context, qu []byte) (result []byte, rerr error) {
	q := task.GetRootAsQuery(qu, 0)
	attr := string(q.Attr())
	gid := BelongsTo(attr)
	x.Trace(ctx, "attr: %v groupId: %v", attr, gid)

	if groups().ServesGroup(gid) {
		// No need for a network call, as this should be run from within this instance.
		return processTask(qu)
	}

	// Send this over the network.
	// TODO: Send the request to multiple servers as described in Jeff Dean's talk.
	addr := groups().AnyServer(gid)
	pl := pools().get(addr)

	conn, err := pl.Get()
	if err != nil {
		return result, x.Wrapf(err, "ProcessTaskOverNetwork: while retrieving connection.")
	}
	defer pl.Put(conn)
	x.Trace(ctx, "Sending request to %v", addr)

	c := NewWorkerClient(conn)
	reply, err := c.ServeTask(ctx, &Payload{Data: qu})
	if err != nil {
		x.TraceError(ctx, x.Wrapf(err, "Error while calling Worker.ServeTask"))
		return []byte(""), err
	}

	x.Trace(ctx, "Reply from server. length: %v Addr: %v Attr: %v",
		len(reply.Data), addr, attr)
	return reply.Data, nil
}

type keyList interface {
	// Length of the list
	Length() int
	// fetch the i'th key in the list
	Key(i int, attr string) []byte
	// A filter to be applied to the uid list
	PostFilter(attr string) func(u uint64) bool
}

// wrap the TermList to implement the keyList interface
type termKeyList struct {
	*task.TermList
}

func (t termKeyList) Length() int {
	return t.TermsLength()
}

func (t termKeyList) Key(i int, attr string) []byte {
	return posting.IndexKey(attr, t.Terms(i))
}

func (t termKeyList) PostFilter(attr string) func(u uint64) bool {
	return nil
}

// Wrap the UidsList to implement the keyList interface
type uidKeyList struct {
	*task.UidList
}

func (t uidKeyList) Length() int {
	return t.UidsLength()
}

func (t uidKeyList) Key(i int, attr string) []byte {
	return posting.Key(t.Uids(i), attr)
}

func (t uidKeyList) PostFilter(attr string) func(u uint64) bool {
	return nil
}

// processTask processes the query, accumulates and returns the result.
func processTask(query []byte) ([]byte, error) {
	q := task.GetRootAsQuery(query, 0)

	unionTable := new(flatbuffers.Table)
	if q.Filter(unionTable) {
		switch q.FilterType() {

		case task.QueryFilterUidList:
			uids := new(task.UidList)
			uids.Init(unionTable.Bytes, unionTable.Pos)
			return readPostingList(q, uidKeyList{uids}), nil

		case task.QueryFilterTermList:
			terms := new(task.TermList)
			terms.Init(unionTable.Bytes, unionTable.Pos)
			return readPostingList(q, termKeyList{terms}), nil

		case task.QueryFilterGeoFilter:
			filter := new(task.GeoFilter)
			filter.Init(unionTable.Bytes, unionTable.Pos)
			k, err := newQueryKeys(filter)
			if err != nil {
				return nil, err
			}
			return readPostingList(q, k), nil

		default:
			return nil, x.Errorf("Unknown filter type %v", q.FilterType())
		}
	}
	return nil, x.Errorf("No filter in query")
}

func readPostingList(q *task.Query, keys keyList) []byte {
	store := ws.dataStore
	attr := string(q.Attr())
	n := keys.Length()

	b := flatbuffers.NewBuilder(0)
	voffsets := make([]flatbuffers.UOffsetT, n)
	uoffsets := make([]flatbuffers.UOffsetT, n)
	var counts []uint64

	for i := 0; i < n; i++ {
		key := keys.Key(i, attr)
		// Get or create the posting list for an entity, attribute combination.
		pl, decr := posting.GetOrCreate(key, store)
		defer decr()

		var valoffset flatbuffers.UOffsetT
		// If a posting list contains a value, we store that or else we store a nil
		// byte so that processing is consistent later.
		val, t, err := pl.Value()
		if err != nil {
			valoffset = b.CreateByteVector(x.Nilbyte)
		} else {
			valoffset = b.CreateByteVector(val)
		}
		task.ValueStart(b)
		task.ValueAddVal(b, valoffset)
		task.ValueAddValType(b, t)
		voffsets[i] = task.ValueEnd(b)

		if q.GetCount() == 1 {
			count := uint64(pl.Length())
			counts = append(counts, count)
			// Add an empty UID list to make later processing consistent
			uoffsets[i] = algo.NewUIDList([]uint64{}).AddTo(b)
		} else {
			opts := posting.ListOptions{
				AfterUID: uint64(q.AfterUid()),
			}

			// Get taskQuery.Intersect field.
			taskList := new(task.UidList)
			if q.ToIntersect(taskList) != nil {
				opts.Intersect = new(algo.UIDList)
				opts.Intersect.FromTask(taskList)
			}

			ulist := pl.Uids(opts)
			pf := keys.PostFilter(attr)
			if pf != nil {
				ulist.ApplyFilter(pf)
			}
			uoffsets[i] = ulist.AddTo(b)
		}
	}
	return createResult(b, uoffsets, voffsets, counts)
}

func createResult(b *flatbuffers.Builder, uoffsets, voffsets []flatbuffers.UOffsetT,
	counts []uint64) []byte {

	// Create a ValueList's vector of Values.
	task.ValueListStartValuesVector(b, len(voffsets))
	for i := len(voffsets) - 1; i >= 0; i-- {
		b.PrependUOffsetT(voffsets[i])
	}
	valuesVecOffset := b.EndVector(len(voffsets))

	// Create a ValueList.
	task.ValueListStart(b)
	task.ValueListAddValues(b, valuesVecOffset)
	valuesVent := task.ValueListEnd(b)

	// Prepare UID matrix.
	task.ResultStartUidmatrixVector(b, len(uoffsets))
	for i := len(uoffsets) - 1; i >= 0; i-- {
		b.PrependUOffsetT(uoffsets[i])
	}
	matrixVent := b.EndVector(len(uoffsets))

	// Create a CountList's vector of ulong.
	task.CountListStartCountVector(b, len(counts))
	for i := len(counts) - 1; i >= 0; i-- {
		b.PrependUint64(counts[i])
	}
	countVecOffset := b.EndVector(len(counts))

	// Create a CountList.
	task.CountListStart(b)
	task.CountListAddCount(b, countVecOffset)
	countsVent := task.CountListEnd(b)

	task.ResultStart(b)
	task.ResultAddValues(b, valuesVent)
	task.ResultAddUidmatrix(b, matrixVent)
	task.ResultAddCount(b, countsVent)
	b.Finish(task.ResultEnd(b))
	return b.FinishedBytes()
}

// ServeTask is used to respond to a query.
func (w *grpcWorker) ServeTask(ctx context.Context, query *Payload) (*Payload, error) {
	if ctx.Err() != nil {
		return &Payload{}, ctx.Err()
	}

	q := task.GetRootAsQuery(query.Data, 0)
	gid := BelongsTo(string(q.Attr()))
	x.Trace(ctx, "Attribute: %q groupId: %v ServeTask", q.Attr(), gid)

	reply := new(Payload)
	x.Assertf(groups().ServesGroup(gid),
		"attr: %q groupId: %v Request sent to wrong server.", q.Attr(), gid)

	c := make(chan error, 1)
	go func() {
		var err error
		reply.Data, err = processTask(query.Data)
		c <- err
	}()

	select {
	case <-ctx.Done():
		return reply, ctx.Err()
	case err := <-c:
		return reply, err
	}
}
