/*
 * Copyright (C) 2017 Dgraph Labs, Inc. and Contributors
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
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
	"github.com/dgraph-io/dgraph/protos/facetsp"
	"github.com/dgraph-io/dgraph/protos/graphp"
	"github.com/dgraph-io/dgraph/protos/taskp"
	"github.com/dgraph-io/dgraph/types/facets"
	"github.com/dgraph-io/dgraph/worker"

	"github.com/dgraph-io/dgraph/store"
	"github.com/dgraph-io/dgraph/types"

	"github.com/dgraph-io/dgraph/x"
)

func childAttrs(sg *SubGraph) []string {
	var out []string
	for _, c := range sg.Children {
		out = append(out, c.Attr)
	}
	return out
}

func taskValues(t *testing.T, v []*taskp.Value) []string {
	out := make([]string, len(v))
	for i, tv := range v {
		out[i] = string(tv.Val)
	}
	return out
}

func addEdge(t *testing.T, attr string, src uint64, edge *taskp.DirectedEdge) {
	l, _ := posting.GetOrCreate(x.DataKey(attr, src), 1)
	require.NoError(t,
		l.AddMutationWithIndex(context.Background(), edge))
}

func makeFacets(facetKVs map[string]string) (fs []*facetsp.Facet, err error) {
	if len(facetKVs) == 0 {
		return nil, nil
	}
	allKeys := make([]string, 0, len(facetKVs))
	for k := range facetKVs {
		allKeys = append(allKeys, k)
	}
	sort.Strings(allKeys)

	for _, k := range allKeys {
		f, err := facets.FacetFor(k, facetKVs[k])
		if err != nil {
			return nil, err
		}
		fs = append(fs, f)
	}
	return fs, nil
}

func addEdgeToValue(t *testing.T, attr string, src uint64,
	value string, facetKVs map[string]string) {
	addEdgeToLangValue(t, attr, src, value, "", facetKVs)
}

func addEdgeToLangValue(t *testing.T, attr string, src uint64,
	value, lang string, facetKVs map[string]string) {
	fs, err := makeFacets(facetKVs)
	require.NoError(t, err)
	edge := &taskp.DirectedEdge{
		Value:  []byte(value),
		Lang:   lang,
		Label:  "testing",
		Attr:   attr,
		Entity: src,
		Op:     taskp.DirectedEdge_SET,
		Facets: fs,
	}
	addEdge(t, attr, src, edge)
}

func addEdgeToTypedValue(t *testing.T, attr string, src uint64,
	typ types.TypeID, value []byte, facetKVs map[string]string) {
	fs, err := makeFacets(facetKVs)
	require.NoError(t, err)
	edge := &taskp.DirectedEdge{
		Value:     value,
		ValueType: uint32(typ),
		Label:     "testing",
		Attr:      attr,
		Entity:    src,
		Op:        taskp.DirectedEdge_SET,
		Facets:    fs,
	}
	addEdge(t, attr, src, edge)
}

func addEdgeToUID(t *testing.T, attr string, src uint64,
	dst uint64, facetKVs map[string]string) {
	fs, err := makeFacets(facetKVs)
	require.NoError(t, err)
	edge := &taskp.DirectedEdge{
		ValueId: dst,
		Label:   "testing",
		Attr:    attr,
		Entity:  src,
		Op:      taskp.DirectedEdge_SET,
		Facets:  fs,
	}
	addEdge(t, attr, src, edge)
}

func delEdgeToUID(t *testing.T, attr string, src uint64, dst uint64) {
	edge := &taskp.DirectedEdge{
		ValueId: dst,
		Label:   "testing",
		Attr:    attr,
		Entity:  src,
		Op:      taskp.DirectedEdge_DEL,
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
	res, err := gql.Parse(gql.Request{Str: query, Http: true})
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

func processSchemaQuery(t *testing.T, q string) []*graphp.SchemaNode {
	res, err := gql.Parse(gql.Request{Str: q})
	require.NoError(t, err)

	ctx := context.Background()
	schema, err := worker.GetSchemaOverNetwork(ctx, res.Schema)
	require.NoError(t, err)
	return schema
}

func processToPB(t *testing.T, query string, variables map[string]string,
	debug bool) []*graphp.Node {
	res, err := gql.Parse(gql.Request{Str: query, Variables: variables})
	require.NoError(t, err)
	var ctx context.Context
	if debug {
		ctx = metadata.NewIncomingContext(context.Background(), metadata.Pairs("debug", "true"))
	} else {
		ctx = context.Background()
	}
	var l Latency
	sgl, err := ProcessQuery(ctx, res, &l)
	require.NoError(t, err)

	pb, err := ToProtocolBuf(&l, sgl)
	require.NoError(t, err)
	return pb
}
