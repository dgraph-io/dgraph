/*
 * Copyright 2017 Dgraph Labs, Inc.
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
	"sort"
	"testing"

	"google.golang.org/grpc/metadata"

	"github.com/stretchr/testify/require"
	geom "github.com/twpayne/go-geom"

	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/query/graph"
	"github.com/dgraph-io/dgraph/store"
	"github.com/dgraph-io/dgraph/task"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/types/facets"
	"github.com/dgraph-io/dgraph/x"
)

func childAttrs(sg *SubGraph) []string {
	var out []string
	for _, c := range sg.Children {
		out = append(out, c.Attr)
	}
	return out
}

func taskValues(t *testing.T, v []*task.Value) []string {
	out := make([]string, len(v))
	for i, tv := range v {
		out[i] = string(tv.Val)
	}
	return out
}

func addEdge(t *testing.T, attr string, src uint64, edge *task.DirectedEdge) {
	l, _ := posting.GetOrCreate(x.DataKey(attr, src), 0)
	require.NoError(t,
		l.AddMutationWithIndex(context.Background(), edge))
}

func makeFacets(facetKVs map[string]string) (fs []*facets.Facet, err error) {
	if len(facetKVs) == 0 {
		return nil, nil
	}
	allKeys := make([]string, 0, len(facetKVs))
	for k := range facetKVs {
		allKeys = append(allKeys, k)
	}
	sort.Strings(allKeys)

	for _, k := range allKeys {
		v := facetKVs[k]
		typ, err := facets.ValType(v)
		if err != nil {
			return nil, err
		}
		fs = append(fs, &facets.Facet{
			k,
			[]byte(v),
			typ,
		})
	}
	return fs, nil
}

func addEdgeToValue(t *testing.T, attr string, src uint64,
	value string, facetKVs map[string]string) {
	fs, err := makeFacets(facetKVs)
	require.NoError(t, err)
	edge := &task.DirectedEdge{
		Value:  []byte(value),
		Label:  "testing",
		Attr:   attr,
		Entity: src,
		Op:     task.DirectedEdge_SET,
		Facets: fs,
	}
	addEdge(t, attr, src, edge)
}

func addEdgeToTypedValue(t *testing.T, attr string, src uint64,
	typ types.TypeID, value []byte, facetKVs map[string]string) {
	fs, err := makeFacets(facetKVs)
	require.NoError(t, err)
	edge := &task.DirectedEdge{
		Value:     value,
		ValueType: uint32(typ),
		Label:     "testing",
		Attr:      attr,
		Entity:    src,
		Op:        task.DirectedEdge_SET,
		Facets:    fs,
	}
	addEdge(t, attr, src, edge)
}

func addEdgeToUID(t *testing.T, attr string, src uint64,
	dst uint64, facetKVs map[string]string) {
	fs, err := makeFacets(facetKVs)
	require.NoError(t, err)
	edge := &task.DirectedEdge{
		ValueId: dst,
		Label:   "testing",
		Attr:    attr,
		Entity:  src,
		Op:      task.DirectedEdge_SET,
		Facets:  fs,
	}
	addEdge(t, attr, src, edge)
}

func delEdgeToUID(t *testing.T, attr string, src uint64, dst uint64) {
	edge := &task.DirectedEdge{
		ValueId: dst,
		Label:   "testing",
		Attr:    attr,
		Entity:  src,
		Op:      task.DirectedEdge_DEL,
	}
	addEdge(t, attr, src, edge)
}

func addGeoData(t *testing.T, ps *store.Store, uid uint64, p geom.T, name string) {
	value := types.ValueForType(types.BinaryID)
	src := types.ValueForType(types.GeoID)
	src.Value = p
	err := types.Marshal(src, &value)
	require.NoError(t, err)
	addEdgeToTypedValue(t, "geometry", uid, types.GeoID, value.Value.([]byte), nil)
	addEdgeToTypedValue(t, "name", uid, types.StringID, []byte(name), nil)
}

func processToFastJsonReq(t *testing.T, query string) (string, error) {
	res, err := gql.Parse(query)
	if err != nil {
		return "", err
	}
	var l Latency
	ctx := context.Background()
	sgl, err := ProcessQuery(ctx, res, &l)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	err = ToJson(&l, sgl, &buf, nil)
	return string(buf.Bytes()), err
}

func processToFastJSON(t *testing.T, query string) string {
	res, err := processToFastJsonReq(t, query)
	require.NoError(t, err)
	return res
}

func processToPB(t *testing.T, query string, debug bool) *graph.Node {
	res, err := gql.Parse(query)
	require.NoError(t, err)
	var ctx context.Context
	if debug {
		ctx = metadata.NewContext(context.Background(), metadata.Pairs("debug", "true"))
	} else {
		ctx = context.Background()
	}
	sg, err := ToSubGraph(ctx, res.Query[0])
	require.NoError(t, err)

	ch := make(chan error)
	go ProcessGraph(ctx, sg, nil, ch)
	err = <-ch
	require.NoError(t, err)

	var l Latency
	pb, err := sg.ToProtocolBuffer(&l)
	require.NoError(t, err)
	return pb
}
