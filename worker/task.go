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
	"strings"

	"golang.org/x/net/context"

	"github.com/dgraph-io/dgraph/algo"
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
		return processTask(q, gid)
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

// convertValue converts the data to the schema type of predicate.
func convertValue(attr, data string) (types.Val, error) {
	// Parse given value and get token. There should be only one token.
	t, err := schema.TypeOf(attr)
	if err != nil || !t.IsScalar() {
		return types.Val{}, x.Errorf("Attribute %s is not valid scalar type", attr)
	}

	src := types.Val{types.StringID, []byte(data)}
	dst, err := types.Convert(src, t)
	return dst, err
}

// processTask processes the query, accumulates and returns the result.
func processTask(q *task.Query, gid uint32) (*task.Result, error) {
	attr := q.Attr

	useFunc := len(q.SrcFunc) != 0
	var tokens []string
	var geoQuery *types.GeoQueryData
	var err error
	var intersectDest bool
	var ineqValue types.Val
	var ineqValueToken string
	var isIneq, isAgrtr bool
	var f string
	var n int

	if useFunc {
		f = q.SrcFunc[0]
		isIneq = f == "leq" || f == "geq" || f == "lt" || f == "gt" || f == "eq"
		isAgrtr = f == "min" || f == "max" || f == "sum"
		switch {
		case isAgrtr:
			// confirm agrregator could apply on the attributes
			typ, err := schema.TypeOf(attr)
			if err != nil {
				return nil, x.Errorf("Attribute %q is not scalar-type", attr)
			}
			if !CouldApplyAggregatorOn(f, typ) {
				return nil, x.Errorf("Aggregator %q could not apply on %v",
					f, attr)
			}

		case isIneq:
			if len(q.SrcFunc) != 2 {
				return nil, x.Errorf("Function requires 2 arguments, but got %d %v",
					len(q.SrcFunc), q.SrcFunc)
			}
			ineqValue, err = convertValue(attr, q.SrcFunc[1])
			if err != nil {
				return nil, err
			}
			// Tokenizing RHS value of inequality.
			// TODO(kg): more comments about why we convert to types.BinaryID, and
			// then convert it back to attr type in IndexTokens.
			// the point is IndexTokens need BinaryID type to be passed in
			v := types.ValueForType(types.BinaryID)
			err = types.Marshal(ineqValue, &v)
			if err != nil {
				return nil, err
			}
			ineqTokens, err := posting.IndexTokens(attr, types.Val{ineqValue.Tid, v.Value.([]byte)})
			if err != nil {
				return nil, err
			}
			if len(ineqTokens) != 1 {
				return nil, x.Errorf("Expected only 1 token but got: %v", ineqTokens)
			}
			ineqValueToken = ineqTokens[0]
			// Get tokens geq / leq ineqValueToken.
			tokens, err = getInequalityTokens(attr, ineqValueToken, f)
			if err != nil {
				return nil, err
			}

		case types.IsGeoFunc(q.SrcFunc[0]):
			// For geo functions, we get extra information used for filtering.
			tokens, geoQuery, err = types.GetGeoTokens(q.SrcFunc)
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
		if isAgrtr {
			n = algo.ListLen(q.Uids)
		} else {
			n = len(tokens)
		}
	} else {
		n = algo.ListLen(q.Uids)
	}

	var out task.Result
	it := algo.NewListIterator(q.Uids)
	for i := 0; i < n; i++ {
		var key []byte
		if isAgrtr {
			key = x.DataKey(attr, it.Val())
			it.Next()
		} else if useFunc {
			key = x.IndexKey(attr, tokens[i])
		} else if q.Reverse {
			key = x.ReverseKey(attr, it.Val())
			it.Next()
		} else {
			key = x.DataKey(attr, it.Val())
			it.Next()
		}
		// Get or create the posting list for an entity, attribute combination.
		pl, decr := posting.GetOrCreate(key, gid)
		defer decr()

		// If a posting list contains a value, we store that or else we store a nil
		// byte so that processing is consistent later.
		val, err := pl.Value()
		newValue := &task.Value{ValType: int32(val.Tid)}
		if err == nil {
			newValue.Val = val.Value.([]byte)
		} else {
			newValue.Val = x.Nilbyte
		}
		out.Values = append(out.Values, newValue)

		if q.DoCount || isAgrtr {
			if q.DoCount {
				out.Counts = append(out.Counts, uint32(pl.Length(0)))
			}
			// Add an empty UID list to make later processing consistent
			out.UidMatrix = append(out.UidMatrix, &emptyUIDList)
			continue
		}
		// The more usual case: Getting the UIDs.
		opts := posting.ListOptions{
			AfterUID: uint64(q.AfterUid),
		}
		// If we have srcFunc and Uids, it means its a filter. So we intersect.
		if useFunc && algo.ListLen(q.Uids) > 0 {
			opts.Intersect = q.Uids
		}
		out.UidMatrix = append(out.UidMatrix, pl.Uids(opts))
	}

	if isAgrtr && len(out.Values) > 0 {
		var err error
		typ, _ := schema.TypeOf(attr)
		out.Values[0], err = Aggregate(f, out.Values, typ)
		if err != nil {
			return nil, err
		}
		out.Values = out.Values[0:1] // trim length to 1
	}

	if isIneq && len(tokens) > 0 && ineqValueToken == tokens[0] {
		// Need to evaluate inequality for entries in the first bucket.
		typ, err := schema.TypeOf(attr)
		if err != nil || !typ.IsScalar() {
			return nil, x.Errorf("Attribute not scalar: %s %v", attr, typ)
		}

		x.AssertTrue(len(out.UidMatrix) > 0)
		// Filter the first row of UidMatrix. Since ineqValue != nil, we may
		// assume that ineqValue is equal to the first token found in TokensTable.
		algo.ApplyFilter(out.UidMatrix[0], func(uid uint64, i int) bool {
			sv, err := fetchValue(uid, attr, typ)
			if sv.Value == nil || err != nil {
				return false
			}
			switch q.SrcFunc[0] {
			case "geq":
				return !types.Less(sv, ineqValue)
			case "gt":
				return types.Less(ineqValue, sv)
			case "leq":
				return !types.Less(ineqValue, sv)
			case "lt":
				return types.Less(sv, ineqValue)
			case "eq":
				return !types.Less(sv, ineqValue) && !types.Less(ineqValue, sv)
			default:
				x.Fatalf("Unknown ineqType %v", q.SrcFunc[0])
			}
			return false
		})
	}

	// If geo filter, do value check for correctness.
	var values []*task.Value
	if geoQuery != nil {
		uids := algo.MergeSorted(out.UidMatrix)
		it := algo.NewListIterator(uids)
		for ; it.Valid(); it.Next() {
			uid := it.Val()
			key := x.DataKey(attr, uid)
			pl, decr := posting.GetOrCreate(key, gid)
			val, err := pl.Value()
			newValue := &task.Value{ValType: int32(val.Tid)}
			if err == nil {
				newValue.Val = val.Value.([]byte)
			} else {
				newValue.Val = x.Nilbyte
			}
			values = append(values, newValue)
			decr() // Decrement the reference count of the pl.
		}

		filtered := types.FilterGeoUids(uids, values, geoQuery)
		for i := 0; i < len(out.UidMatrix); i++ {
			algo.IntersectWith(out.UidMatrix[i], filtered)
		}
	}
	out.IntersectDest = intersectDest
	return &out, nil
}

// ServeTask is used to respond to a query.
func (w *grpcWorker) ServeTask(ctx context.Context, q *task.Query) (*task.Result, error) {
	if ctx.Err() != nil {
		return &emptyResult, ctx.Err()
	}

	gid := group.BelongsTo(q.Attr)
	x.Trace(ctx, "Attribute: %q NumUids: %v groupId: %v ServeTask", q.Attr, algo.ListLen(q.Uids), gid)

	var reply *task.Result
	x.AssertTruef(groups().ServesGroup(gid),
		"attr: %q groupId: %v Request sent to wrong server.", q.Attr, gid)

	c := make(chan error, 1)
	go func() {
		var err error
		reply, err = processTask(q, gid)
		c <- err
	}()

	select {
	case <-ctx.Done():
		return reply, ctx.Err()
	case err := <-c:
		return reply, err
	}
}
