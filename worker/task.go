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

	"github.com/dgraph-io/dgraph/algo"
	"github.com/dgraph-io/dgraph/group"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/task"
	"github.com/dgraph-io/dgraph/taskpb"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
)

// ProcessTaskOverNetwork is used to process the query and get the result from
// the instance which stores posting list corresponding to the predicate in the
// query.
func ProcessTaskOverNetwork(ctx context.Context, qu []byte) (result []byte, rerr error) {
	q := task.GetRootAsQuery(qu, 0)
	attr := string(q.Attr())
	gid := group.BelongsTo(attr)
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

// processTask processes the query, accumulates and returns the result.
func processTask(query []byte) ([]byte, error) {
	q := task.GetRootAsQuery(query, 0)

	attr := string(q.Attr())
	x.AssertTruef(q.UidsLength() == 0 || q.TokensLength() == 0,
		"At least one of Uids and Term should be empty: %d vs %d", q.UidsLength(), q.TokensLength())

	useTerm := q.TokensLength() > 0
	var n int
	if useTerm {
		n = q.TokensLength()
	} else {
		n = q.UidsLength()
	}

	countsList := new(taskpb.CountList)
	valuesList := new(taskpb.ValueList)
	out := &taskpb.Result{
		Counts: countsList,
		Values: valuesList,
	}

	var emptyUIDList *taskpb.UIDList // For handling _count_ only.
	if q.GetCount() == 1 {
		emptyUIDList = new(taskpb.UIDList)
	}

	for i := 0; i < n; i++ {
		var key []byte
		if useTerm {
			key = types.IndexKey(attr, string(q.Tokens(i)))
		} else {
			key = posting.Key(q.Uids(i), attr)
		}
		// Get or create the posting list for an entity, attribute combination.
		pl, decr := posting.GetOrCreate(key)
		defer decr()

		// If a posting list contains a value, we store that or else we store a nil
		// byte so that processing is consistent later.
		vbytes, vtype, err := pl.Value()

		newValue := &taskpb.Value{ValType: uint32(vtype)}
		if err == nil {
			newValue.Val = vbytes
		} else {
			newValue.Val = x.Nilbyte
		}
		valuesList.Values = append(valuesList.Values, newValue)

		if q.GetCount() == 1 {
			countsList.Count = append(countsList.Count, uint32(pl.Length(0)))
			// Add an empty UID list to make later processing consistent
			out.UidMatrix = append(out.UidMatrix, emptyUIDList)
			continue
		}

		// The more usual case: Getting the UIDs.
		opts := posting.ListOptions{
			AfterUID: uint64(q.AfterUid()),
		}

		// Get taskQuery.ToIntersect field.
		taskList := new(task.UidList)
		if q.ToIntersect(taskList) != nil {
			opts.Intersect = new(algo.UIDList)
			opts.Intersect.FromTask(taskList)
		}
		out.UidMatrix = append(out.UidMatrix, pl.Uids(opts).ToProto())
	}

	outB, err := out.Marshal()
	if err != nil {
		return nil, err
	}
	return outB, nil
}

// ServeTask is used to respond to a query.
func (w *grpcWorker) ServeTask(ctx context.Context, query *Payload) (*Payload, error) {
	if ctx.Err() != nil {
		return &Payload{}, ctx.Err()
	}

	q := task.GetRootAsQuery(query.Data, 0)
	gid := group.BelongsTo(string(q.Attr()))
	x.Trace(ctx, "Attribute: %q NumUids: %v groupId: %v ServeTask", q.Attr(), q.UidsLength(), gid)

	reply := new(Payload)
	x.AssertTruef(groups().ServesGroup(gid),
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
