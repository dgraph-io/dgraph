/*
 * Copyright 2016 Dgraph Labs, Inc.
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

package query

import (
	"bytes"
	"context"

	"github.com/dgraph-io/dgraph/algo"
	"github.com/dgraph-io/dgraph/geo"
	"github.com/dgraph-io/dgraph/task"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
)

func (sg *SubGraph) applyGeoQuery(ctx context.Context) error {
	if sg.GeoFilter == nil { // no geo filter
		return nil
	}

	tokens, data, err := geo.QueryTokens(sg.GeoFilter)
	if err != nil {
		return err
	}

	// Lookup the geo index first
	uids, err := fetchIndexEntries(ctx, tokens)
	if err != nil {
		return err
	}

	// Fetch the actual values from the predicate
	values, err := fetchValues(ctx, sg.Attr, uids)
	if err != nil {
		return err
	}

	// Filter the values
	sg.srcUIDs = filterUIDs(uids, values, data)
	// Keep result and values consistent with srcUIDs

	for i := 0; i < sg.srcUIDs.Size(); i++ {
		uid := sg.srcUIDs.Get(i)
		ulist := algo.NewUIDList([]uint64{uid})
		sg.Result = append(sg.Result, ulist)
	}
	sg.Values = createNilValuesList(sg.srcUIDs.Size())
	return nil
}

func fetchIndexEntries(ctx context.Context, tokens [][]byte) (*algo.UIDList, error) {
	sg := &SubGraph{Attr: geo.IndexAttr}
	sgChan := make(chan error, 1)

	// Query the index for the uids
	taskQuery := createTaskQuery(sg, nil, tokens, nil)
	go ProcessGraph(ctx, sg, taskQuery, sgChan)
	select {
	case <-ctx.Done():
		return nil, x.Wrap(ctx.Err())
	case err := <-sgChan:
		if err != nil {
			return nil, err
		}
	}

	x.Assert(len(sg.Result) == len(tokens))
	return algo.MergeLists(sg.Result), nil
}

func fetchValues(ctx context.Context, attr string, uids *algo.UIDList) (*task.ValueList, error) {
	sg := &SubGraph{Attr: attr}
	sgChan := make(chan error, 1)

	// Query the index for the uids
	taskQuery := createTaskQuery(sg, uids, nil, nil)
	go ProcessGraph(ctx, sg, taskQuery, sgChan)
	select {
	case <-ctx.Done():
		return nil, x.Wrap(ctx.Err())
	case err := <-sgChan:
		if err != nil {
			return nil, err
		}
	}

	values := sg.Values
	x.Assert(values.ValuesLength() == uids.Size())
	return values, nil
}

func filterUIDs(uids *algo.UIDList, values *task.ValueList, q *geo.QueryData) *algo.UIDList {
	x.Assert(values.ValuesLength() == uids.Size())
	var rv []uint64
	for i := 0; i < values.ValuesLength(); i++ {
		var tv task.Value
		if ok := values.Values(&tv, i); !ok {
			continue
		}
		valBytes := tv.ValBytes()
		if bytes.Equal(valBytes, nil) {
			continue
		}
		vType := tv.ValType()
		if types.TypeID(vType) != types.GeoID {
			continue
		}
		var g types.Geo
		if err := g.UnmarshalBinary(valBytes); err != nil {
			continue
		}

		if !q.MatchesFilter(g) {
			continue
		}

		// we matched the geo filter, add the uid to the list
		rv = append(rv, uids.Get(i))
	}
	return algo.NewUIDList(rv)
}
