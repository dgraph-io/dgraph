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
	"encoding/json"
	"io"
	"os"
	"sort"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	geom "github.com/twpayne/go-geom"
	"github.com/twpayne/go-geom/encoding/geojson"

	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/types/facets"
	"github.com/dgraph-io/dgraph/worker"
	"github.com/dgraph-io/dgraph/x"
)

func childAttrs(sg *SubGraph) []string {
	var out []string
	for _, c := range sg.Children {
		out = append(out, c.Attr)
	}
	return out
}

func taskValues(t *testing.T, v []*protos.ValueList) []string {
	out := make([]string, len(v))
	for i, tv := range v {
		out[i] = string(tv.Values[0].Val)
	}
	return out
}

var index uint64

func addEdge(t *testing.T, attr string, src uint64, edge *protos.DirectedEdge) {
	// Mutations don't go through normal flow, so default schema for predicate won't be present.
	// Lets add it.
	if _, ok := schema.State().Get(attr); !ok {
		schema.State().Set(attr, protos.SchemaUpdate{
			Predicate: attr,
			ValueType: edge.ValueType,
		})
	}
	l := posting.Get(x.DataKey(attr, src))
	startTs := timestamp()
	txn := &posting.Txn{
		StartTs: startTs,
		Indices: []uint64{atomic.AddUint64(&index, 1)},
	}
	txn = posting.Txns().PutOrMergeIndex(txn)
	require.NoError(t,
		l.AddMutationWithIndex(context.Background(), edge, txn))

	commit := commitTs(startTs)
	go func() {
		require.NoError(t, txn.CommitMutations(context.Background(), commit))
	}()
}

func makeFacets(facetKVs map[string]string) (fs []*protos.Facet, err error) {
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

func addPredicateEdge(t *testing.T, attr string, src uint64) {
	if worker.Config.ExpandEdge {
		edge := &protos.DirectedEdge{
			Value: []byte(attr),
			Attr:  "_predicate_",
			Op:    protos.DirectedEdge_SET,
		}
		addEdge(t, "_predicate_", src, edge)
	}
}

func addEdgeToValue(t *testing.T, attr string, src uint64,
	value string, facetKVs map[string]string) {
	addEdgeToLangValue(t, attr, src, value, "", facetKVs)
	addPredicateEdge(t, attr, src)
}

func addEdgeToLangValue(t *testing.T, attr string, src uint64,
	value, lang string, facetKVs map[string]string) {
	fs, err := makeFacets(facetKVs)
	require.NoError(t, err)
	edge := &protos.DirectedEdge{
		Value:  []byte(value),
		Lang:   lang,
		Label:  "testing",
		Attr:   attr,
		Entity: src,
		Op:     protos.DirectedEdge_SET,
		Facets: fs,
	}
	addEdge(t, attr, src, edge)
	addPredicateEdge(t, attr, src)
}

func addEdgeToTypedValue(t *testing.T, attr string, src uint64,
	typ types.TypeID, value []byte, facetKVs map[string]string) {
	fs, err := makeFacets(facetKVs)
	require.NoError(t, err)
	edge := &protos.DirectedEdge{
		Value:     value,
		ValueType: protos.Posting_ValType(typ),
		Label:     "testing",
		Attr:      attr,
		Entity:    src,
		Op:        protos.DirectedEdge_SET,
		Facets:    fs,
	}
	addEdge(t, attr, src, edge)
	addPredicateEdge(t, attr, src)
}

func addEdgeToUID(t *testing.T, attr string, src uint64,
	dst uint64, facetKVs map[string]string) {
	fs, err := makeFacets(facetKVs)
	require.NoError(t, err)
	edge := &protos.DirectedEdge{
		ValueId: dst,
		// This is used to set uid schema type for pred for the purpose of tests. Actual mutation
		// won't set ValueType to types.UidID.
		ValueType: protos.Posting_ValType(types.UidID),
		Label:     "testing",
		Attr:      attr,
		Entity:    src,
		Op:        protos.DirectedEdge_SET,
		Facets:    fs,
	}
	addEdge(t, attr, src, edge)
	addPredicateEdge(t, attr, src)
}

func delEdgeToUID(t *testing.T, attr string, src uint64, dst uint64) {
	edge := &protos.DirectedEdge{
		ValueType: protos.Posting_ValType(types.UidID),
		ValueId:   dst,
		Label:     "testing",
		Attr:      attr,
		Entity:    src,
		Op:        protos.DirectedEdge_DEL,
	}
	addEdge(t, attr, src, edge)
}

func delEdgeToLangValue(t *testing.T, attr string, src uint64, value, lang string) {
	edge := &protos.DirectedEdge{
		Value:  []byte(value),
		Lang:   lang,
		Label:  "testing",
		Attr:   attr,
		Entity: src,
		Op:     protos.DirectedEdge_DEL,
	}
	addEdge(t, attr, src, edge)
}

func addGeoData(t *testing.T, uid uint64, p geom.T, name string) {
	value := types.ValueForType(types.BinaryID)
	src := types.ValueForType(types.GeoID)
	src.Value = p
	err := types.Marshal(src, &value)
	require.NoError(t, err)
	addEdgeToTypedValue(t, "geometry", uid, types.GeoID, value.Value.([]byte), nil)
	addEdgeToTypedValue(t, "name", uid, types.StringID, []byte(name), nil)
}

func defaultContext() context.Context {
	return context.Background()
}

func processToFastJsonReq(t *testing.T, query string) (string, error) {
	return processToFastJsonReqCtx(t, query, defaultContext())
}

func processToFastJsonReqCtx(t *testing.T, query string, ctx context.Context) (string, error) {
	res, err := gql.Parse(gql.Request{Str: query, Http: true})
	if err != nil {
		return "", err
	}

	startTs := timestamp()
	maxPendingCh <- startTs
	queryRequest := QueryRequest{Latency: &Latency{}, GqlQuery: &res, ReadTs: startTs}
	err = queryRequest.ProcessQuery(ctx)
	if err != nil {
		return "", err
	}

	out, err := ToJson(queryRequest.Latency, queryRequest.Subgraphs)
	if err != nil {
		return "", err
	}
	response := map[string]interface{}{}
	response["data"] = json.RawMessage(string(out))
	resp, err := json.Marshal(response)
	require.NoError(t, err)
	return string(resp), err
}

func processToFastJSON(t *testing.T, query string) string {
	res, err := processToFastJsonReq(t, query)
	require.NoError(t, err)
	return res
}

func processSchemaQuery(t *testing.T, q string) []*protos.SchemaNode {
	res, err := gql.Parse(gql.Request{Str: q})
	require.NoError(t, err)

	ctx := context.Background()
	schema, err := worker.GetSchemaOverNetwork(ctx, res.Schema)
	require.NoError(t, err)
	return schema
}

func loadPolygon(name string) (geom.T, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var b bytes.Buffer
	_, err = io.Copy(&b, f)
	if err != nil {
		return nil, err
	}

	var g geojson.Geometry
	g.Type = "MultiPolygon"
	m := json.RawMessage(b.Bytes())
	g.Coordinates = &m
	return g.Decode()
}
