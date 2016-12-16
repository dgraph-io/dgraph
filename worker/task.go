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
	"bytes"
	"strings"

	"golang.org/x/net/context"

	"github.com/dgraph-io/dgraph/algo"
	"github.com/dgraph-io/dgraph/geo"
	"github.com/dgraph-io/dgraph/group"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/task"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
)

var (
	emptyUIDList task.List
	emptyResult  task.Result
)

// ProcessTaskOverNetwork is used to process the query and get the result from
// the instance which stores posting list corresponding to the predicate in the
// query.
func ProcessTaskOverNetwork(ctx context.Context, q *task.Query) (*task.Result, error) {
	attr := q.Attr
	gid := group.BelongsTo(attr)
	x.Trace(ctx, "attr: %v groupId: %v", attr, gid)

	if groups().ServesGroup(gid) {
		// No need for a network call, as this should be run from within this instance.
		return processTask(q)
	}

	// Send this over the network.
	// TODO: Send the request to multiple servers as described in Jeff Dean's talk.
	addr := groups().AnyServer(gid)
	pl := pools().get(addr)

	conn, err := pl.Get()
	if err != nil {
		return &emptyResult, x.Wrapf(err, "ProcessTaskOverNetwork: while retrieving connection.")
	}
	defer pl.Put(conn)
	x.Trace(ctx, "Sending request to %v", addr)

	c := NewWorkerClient(conn)
	reply, err := c.ServeTask(ctx, q)
	if err != nil {
		x.TraceError(ctx, x.Wrapf(err, "Error while calling Worker.ServeTask"))
		return &emptyResult, err
	}

	x.Trace(ctx, "Reply from server. length: %v Addr: %v Attr: %v",
		len(reply.UidMatrix), addr, attr)
	return reply, nil
}

func getValue(attr, data string) (types.Value, error) {
	// Parse given value and get token. There should be only one token.
	t := schema.TypeOf(attr)
	if t == nil || !t.IsScalar() {
		return nil, x.Errorf("Attribute %s is not valid scalar type", attr)
	}

	schemaType := t.(types.Scalar)
	v := types.ValueForType(schemaType.ID())
	err := v.UnmarshalText([]byte(data))
	if err != nil {
		return nil, err
	}
	return v, nil
}

// processTask processes the query, accumulates and returns the result.
func processTask(q *task.Query) (*task.Result, error) {
	attr := q.Attr

	useFunc := len(q.SrcFunc) != 0
	var n int
	var tokens []string
	var geoQuery *geo.QueryData
	var err error
	var intersectDest bool
	var ineqValue types.Value
	var ineqValueToken string
	var isGeq, isLeq bool

	if useFunc {
		f := q.SrcFunc[0]
		isGeq = f == "geq"
		isLeq = f == "leq"
		switch {
		case isGeq:
			fallthrough
		case isLeq:
			if len(q.SrcFunc) != 2 {
				return nil, x.Errorf("Function requires 2 arguments, but got %d %v",
					len(q.SrcFunc), q.SrcFunc)
			}
			ineqValue, err = getValue(attr, q.SrcFunc[1])
			if err != nil {
				return nil, err
			}
			// Tokenizing RHS value of inequality.
			ineqTokens, err := posting.IndexTokens(attr, ineqValue)
			if err != nil {
				return nil, err
			}
			if len(ineqTokens) != 1 {
				return nil, x.Errorf("Expected only 1 token but got: %v", ineqTokens)
			}
			ineqValueToken = ineqTokens[0]
			// Get tokens geq / leq ineqValueToken.
			tokens, err = getInequalityTokens(attr, ineqValueToken, isGeq)
			if err != nil {
				return nil, err
			}

		case geo.IsGeoFunc(q.SrcFunc[0]):
			// For geo functions, we get extra information used for filtering.
			tokens, geoQuery, err = geo.GetTokens(q.SrcFunc)
			if err != nil {
				return nil, err
			}

		default:
			tokens, err = getTokens(q.SrcFunc)
			if err != nil {
				return nil, err
			}
			intersectDest = (strings.ToLower(q.SrcFunc[0]) == "allof")
		}
		n = len(tokens)
	} else {
		n = len(q.Uids)
	}

	var out task.Result
	for i := 0; i < n; i++ {
		var key []byte
		if useFunc {
			key = x.IndexKey(attr, tokens[i])
		} else {
			key = x.DataKey(attr, q.Uids[i])
		}
		// Get or create the posting list for an entity, attribute combination.
		pl, decr := posting.GetOrCreate(key)
		defer decr()

		// If a posting list contains a value, we store that or else we store a nil
		// byte so that processing is consistent later.
		vbytes, vtype, err := pl.Value()

		newValue := &task.Value{ValType: uint32(vtype)}
		if err == nil {
			newValue.Val = vbytes
		} else {
			newValue.Val = x.Nilbyte
		}
		out.Values = append(out.Values, newValue)

		if q.DoCount {
			out.Counts = append(out.Counts, uint32(pl.Length(0)))
			// Add an empty UID list to make later processing consistent
			out.UidMatrix = append(out.UidMatrix, &emptyUIDList)
			continue
		}

		// The more usual case: Getting the UIDs.
		opts := posting.ListOptions{
			AfterUID: uint64(q.AfterUid),
		}
		// If we have srcFunc and Uids, it means its a filter. So we intersect.
		if useFunc && len(q.Uids) > 0 {
			opts.Intersect = &task.List{Uids: q.Uids}
		}
		out.UidMatrix = append(out.UidMatrix, pl.Uids(opts))
	}

	if (isGeq || isLeq) && len(tokens) > 0 && ineqValueToken == tokens[0] {
		// Need to evaluate inequality for entries in the first bucket.
		typ := schema.TypeOf(attr)
		if typ == nil || !typ.IsScalar() {
			return nil, x.Errorf("Attribute not scalar: %s %v", attr, typ)
		}
		scalarType := typ.(types.Scalar)

		x.AssertTrue(len(out.UidMatrix) > 0)
		// Filter the first row of UidMatrix. Since ineqValue != nil, we may
		// assume that ineqValue is equal to the first token found in TokensTable.
		algo.ApplyFilter(out.UidMatrix[0], func(uid uint64, i int) bool {
			key := x.DataKey(attr, uid)
			sv := getPostingValue(key, scalarType)
			if sv == nil {
				return false
			}
			if isGeq {
				return !scalarType.Less(*sv, ineqValue)
			}
			return !scalarType.Less(ineqValue, *sv)
		})
	}

	// If geo filter, do value check for correctness.
	var values []*task.Value
	if geoQuery != nil {
		uids := algo.MergeSorted(out.UidMatrix)
		for _, uid := range uids.Uids {
			key := x.DataKey(attr, uid)
			pl, decr := posting.GetOrCreate(key)

			vbytes, vtype, err := pl.Value()
			newValue := &task.Value{ValType: uint32(vtype)}
			if err == nil {
				newValue.Val = vbytes
			} else {
				newValue.Val = x.Nilbyte
			}
			values = append(values, newValue)
			decr() // Decrement the reference count of the pl.
		}

		filtered := geo.FilterUids(uids, values, geoQuery)
		for i := 0; i < len(out.UidMatrix); i++ {
			out.UidMatrix[i] = algo.IntersectSorted([]*task.List{out.UidMatrix[i], filtered})
		}
	}
	out.IntersectDest = intersectDest
	return &out, nil
}

// getPostingValue looks up key, gets the value, converts it. If any error is
// encountered, we return nil. This is used in some filtering where we do not
// want to waste time creating errors.
func getPostingValue(key []byte, scalarType types.Scalar) *types.Value {
	pl, decr := posting.GetOrCreate(key)
	defer decr()

	valBytes, vType, err := pl.Value()
	if bytes.Equal(valBytes, x.Nilbyte) {
		return nil
	}
	val := types.ValueForType(types.TypeID(vType))
	if val == nil {
		return nil
	}
	if err := val.UnmarshalBinary(valBytes); err != nil {
		return nil
	}
	// Convert to schema type.
	sv, err := scalarType.Convert(val)
	if err != nil {
		return nil
	}
	return &sv
}

// ServeTask is used to respond to a query.
func (w *grpcWorker) ServeTask(ctx context.Context, q *task.Query) (*task.Result, error) {
	if ctx.Err() != nil {
		return &emptyResult, ctx.Err()
	}

	gid := group.BelongsTo(q.Attr)
	x.Trace(ctx, "Attribute: %q NumUids: %v groupId: %v ServeTask", q.Attr, len(q.Uids), gid)

	var reply *task.Result
	x.AssertTruef(groups().ServesGroup(gid),
		"attr: %q groupId: %v Request sent to wrong server.", q.Attr, gid)

	c := make(chan error, 1)
	go func() {
		var err error
		reply, err = processTask(q)
		c <- err
	}()

	select {
	case <-ctx.Done():
		return reply, ctx.Err()
	case err := <-c:
		return reply, err
	}
}
