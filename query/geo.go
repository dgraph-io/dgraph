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

/*
func generateGeo(ctx context.Context, attr string, qt geo.QueryType, g []byte, maxDist float64) (*algo.UIDList, error) {
	tokens, data, err := geo.QueryTokens(g, qt, maxDist)
	if err != nil {
		return nil, err
	}

	// Lookup the geo index first
	uids, err := fetchIndexEntries(ctx, attr, tokens)
	if err != nil {
		return nil, err
	}

	// Fetch the actual values from the predicate
	values, err := fetchValues(ctx, attr, uids)
	if err != nil {
		return nil, err
	}

	// Filter the values
	return filterUIDs(uids, values, data), nil
}

func fetchIndexEntries(ctx context.Context, attr string, tokens []string) (*algo.UIDList, error) {
	sg := &SubGraph{Attr: attr}
	sgChan := make(chan error, 1)

	// Query the index for the uids
	taskQuery := createTaskQuery(sg, tokens, nil)
	go ProcessGraph(ctx, sg, nil, sgChan)
	select {
	case <-ctx.Done():
		return nil, x.Wrap(ctx.Err())
	case err := <-sgChan:
		if err != nil {
			return nil, err
		}
	}

	x.AssertTrue(len(sg.UIDMatrix()) == len(tokens))
	return algo.MergeLists(sg.UIDMatrix()), nil
}

func fetchValues(ctx context.Context, attr string, uids *algo.UIDList) (*task.ValueList, error) {
	sg := &SubGraph{Attr: attr}
	sgChan := make(chan error, 1)

	// Query the index for the uids
	taskQuery := createTaskQuery(sg, uids, nil, nil)
	go ProcessGraph(ctx, sg, nil, sgChan)
	select {
	case <-ctx.Done():
		return nil, x.Wrap(ctx.Err())
	case err := <-sgChan:
		if err != nil {
			return nil, err
		}
	}

	values := sg.Values()
	x.AssertTrue(values.ValuesLength() == uids.Size())
	return values, nil
}

func filterUIDs(uids *algo.UIDList, values *task.ValueList, q *geo.QueryData) *algo.UIDList {
	x.AssertTrue(values.ValuesLength() == uids.Size())
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
*/
