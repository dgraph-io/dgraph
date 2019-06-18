/*
 * Copyright 2017-2018 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
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
	"encoding/json"
	"io"
	"os"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
	geom "github.com/twpayne/go-geom"
	"github.com/twpayne/go-geom/encoding/geojson"

	"github.com/dgraph-io/dgo/protos/api"
	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/pb"
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

func taskValues(t *testing.T, v []*pb.ValueList) []string {
	out := make([]string, len(v))
	for i, tv := range v {
		out[i] = string(tv.Values[0].Val)
	}
	return out
}

var index uint64

func addEdge(t *testing.T, attr string, src uint64, edge *pb.DirectedEdge) {
	// Mutations don't go through normal flow, so default schema for predicate won't be present.
	// Lets add it.
	if _, ok := schema.State().Get(attr); !ok {
		schema.State().Set(attr, pb.SchemaUpdate{
			Predicate: attr,
			ValueType: edge.ValueType,
		})
	}
	startTs := timestamp()
	txn := posting.Oracle().RegisterStartTs(startTs)
	l, err := txn.Get(x.DataKey(attr, src))
	require.NoError(t, err)
	require.NoError(t,
		l.AddMutationWithIndex(context.Background(), edge, txn))

	commit := timestamp()
	// The following logic is based on node.commitOrAbort in worker/draft.go.
	// We need to commit to disk, so secondary indices, particularly the ones
	// which iterate over Badger, would work correctly.
	txn.Update()
	writer := posting.NewTxnWriter(ps)
	require.NoError(t, txn.CommitToDisk(writer, commit))
	require.NoError(t, writer.Flush())

	// require.NoError(t, txn.CommitToMemory(commit))
	// TODO: Switch this package to use normal Dgraph cluster.
	delta := &pb.OracleDelta{MaxAssigned: commit}
	delta.Txns = append(delta.Txns, &pb.TxnStatus{StartTs: startTs, CommitTs: commit})
	posting.Oracle().ProcessDelta(delta)
}

func makeFacets(facetKVs map[string]string) (fs []*api.Facet, err error) {
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
		edge := &pb.DirectedEdge{
			Value: []byte(attr),
			Attr:  "_predicate_",
			Op:    pb.DirectedEdge_SET,
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
	edge := &pb.DirectedEdge{
		Value:  []byte(value),
		Lang:   lang,
		Label:  "testing",
		Attr:   attr,
		Entity: src,
		Op:     pb.DirectedEdge_SET,
		Facets: fs,
	}
	addEdge(t, attr, src, edge)
	addPredicateEdge(t, attr, src)
}

func addEdgeToTypedValue(t *testing.T, attr string, src uint64,
	typ types.TypeID, value []byte, facetKVs map[string]string) {
	fs, err := makeFacets(facetKVs)
	require.NoError(t, err)
	edge := &pb.DirectedEdge{
		Value:     value,
		ValueType: pb.Posting_ValType(typ),
		Label:     "testing",
		Attr:      attr,
		Entity:    src,
		Op:        pb.DirectedEdge_SET,
		Facets:    fs,
	}
	addEdge(t, attr, src, edge)
	addPredicateEdge(t, attr, src)
}

func addEdgeToUID(t *testing.T, attr string, src uint64,
	dst uint64, facetKVs map[string]string) {
	fs, err := makeFacets(facetKVs)
	require.NoError(t, err)
	edge := &pb.DirectedEdge{
		ValueId: dst,
		// This is used to set uid schema type for pred for the purpose of tests. Actual mutation
		// won't set ValueType to types.UidID.
		ValueType: pb.Posting_ValType(types.UidID),
		Label:     "testing",
		Attr:      attr,
		Entity:    src,
		Op:        pb.DirectedEdge_SET,
		Facets:    fs,
	}
	addEdge(t, attr, src, edge)
	addPredicateEdge(t, attr, src)
}

func delEdgeToUID(t *testing.T, attr string, src uint64, dst uint64) {
	edge := &pb.DirectedEdge{
		ValueType: pb.Posting_ValType(types.UidID),
		ValueId:   dst,
		Label:     "testing",
		Attr:      attr,
		Entity:    src,
		Op:        pb.DirectedEdge_DEL,
	}
	addEdge(t, attr, src, edge)
}

func delEdgeToLangValue(t *testing.T, attr string, src uint64, value, lang string) {
	edge := &pb.DirectedEdge{
		Value:  []byte(value),
		Lang:   lang,
		Label:  "testing",
		Attr:   attr,
		Entity: src,
		Op:     pb.DirectedEdge_DEL,
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

func processToFastJson(t *testing.T, query string) (string, error) {
	return processToFastJsonCtxVars(t, query, defaultContext(), nil)
}

func processToFastJsonCtxVars(t *testing.T, query string, ctx context.Context,
	vars map[string]string) (string, error) {
	res, err := gql.Parse(gql.Request{Str: query, Variables: vars})
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

func processToFastJsonNoErr(t *testing.T, query string) string {
	res, err := processToFastJson(t, query)
	require.NoError(t, err)
	return res
}

func processSchemaQuery(t *testing.T, q string) []*api.SchemaNode {
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

func addPassword(t *testing.T, uid uint64, attr, password string) {
	value := types.ValueForType(types.BinaryID)
	src := types.ValueForType(types.PasswordID)
	encrypted, ok := passwordCache[password]
	if !ok {
		encrypted, _ = types.Encrypt(password)
		passwordCache[password] = encrypted
	}
	src.Value = encrypted
	err := types.Marshal(src, &value)
	require.NoError(t, err)
	addEdgeToTypedValue(t, attr, uid, types.PasswordID, value.Value.([]byte), nil)
}

func populateGraph(t *testing.T) {
	x.AssertTrue(ps != nil)

	const schemaStr = `
name                           : string @index(term, exact, trigram) @count @lang .
alias                          : string @index(exact, term, fulltext) .
dob                            : dateTime @index(year) .
dob_day                        : dateTime @index(day) .
film.film.initial_release_date : dateTime @index(year) .
loc                            : geo @index(geo) .
genre                          : uid @reverse .
survival_rate                  : float .
alive                          : bool @index(bool) .
age                            : int @index(int) .
shadow_deep                    : int .
friend                         : uid @reverse @count .
geometry                       : geo @index(geo) .
value                          : string @index(trigram) .
full_name                      : string @index(hash) .
nick_name                      : string @index(term) .
royal_title                    : string @index(hash, term, fulltext) @lang .
noindex_name                   : string .
school                         : uid @count .
lossy                          : string @index(term) @lang .
occupations                    : [string] @index(term) .
graduation                     : [dateTime] @index(year) @count .
salary                         : float @index(float) .
password                       : password .
symbol                         : string @index(exact) .
room                           : string @index(term) .
office.room                    : uid .
number                         : int @index(int) .
`

	err := schema.ParseBytes([]byte(schemaStr), 1)
	x.Check(err)
	addPassword(t, 1, "password", "123456")
	addPassword(t, 23, "pass", "654321")

	addEdgeToUID(t, "school", 32, 33, nil)
	addEdgeToUID(t, "district", 33, 34, nil)
	addEdgeToUID(t, "county", 34, 35, nil)
	addEdgeToUID(t, "state", 35, 36, nil)

	addEdgeToValue(t, "name", 33, "San Mateo High School", nil)
	addEdgeToValue(t, "name", 34, "San Mateo School District", nil)
	addEdgeToValue(t, "name", 35, "San Mateo County", nil)
	addEdgeToValue(t, "name", 36, "California", nil)
	addEdgeToValue(t, "abbr", 36, "CA", nil)

	// So, user we're interested in has uid: 1.
	// She has 5 friends: 23, 24, 25, 31, and 101
	addEdgeToUID(t, "friend", 1, 23, nil)
	addEdgeToUID(t, "friend", 1, 24, nil)
	addEdgeToUID(t, "friend", 1, 25, nil)
	addEdgeToUID(t, "friend", 1, 31, nil)
	addEdgeToUID(t, "friend", 1, 101, nil)
	addEdgeToUID(t, "friend", 31, 24, nil)
	addEdgeToUID(t, "friend", 23, 1, nil)

	addEdgeToUID(t, "school", 1, 5000, nil)
	addEdgeToUID(t, "school", 23, 5001, nil)
	addEdgeToUID(t, "school", 24, 5000, nil)
	addEdgeToUID(t, "school", 25, 5000, nil)
	addEdgeToUID(t, "school", 31, 5001, nil)
	addEdgeToUID(t, "school", 101, 5001, nil)

	addEdgeToValue(t, "name", 5000, "School A", nil)
	addEdgeToValue(t, "name", 5001, "School B", nil)

	addEdgeToUID(t, "follow", 1, 31, nil)
	addEdgeToUID(t, "follow", 1, 24, nil)
	addEdgeToUID(t, "follow", 31, 1001, nil)
	addEdgeToUID(t, "follow", 1001, 1000, nil)
	addEdgeToUID(t, "follow", 1002, 1000, nil)
	addEdgeToUID(t, "follow", 1001, 1003, nil)
	addEdgeToUID(t, "follow", 1001, 1003, nil)
	addEdgeToUID(t, "follow", 1003, 1002, nil)

	addEdgeToUID(t, "path", 1, 31, map[string]string{"weight": "0.1", "weight1": "0.2"})
	addEdgeToUID(t, "path", 1, 24, map[string]string{"weight": "0.2"})
	addEdgeToUID(t, "path", 31, 1000, map[string]string{"weight": "0.1"})
	addEdgeToUID(t, "path", 1000, 1001, map[string]string{"weight": "0.1"})
	addEdgeToUID(t, "path", 1000, 1002, map[string]string{"weight": "0.7"})
	addEdgeToUID(t, "path", 1001, 1002, map[string]string{"weight": "0.1"})
	addEdgeToUID(t, "path", 1002, 1003, map[string]string{"weight": "0.6"})
	addEdgeToUID(t, "path", 1001, 1003, map[string]string{"weight": "1.5"})
	addEdgeToUID(t, "path", 1003, 1001, map[string]string{})

	addEdgeToValue(t, "name", 1000, "Alice", nil)
	addEdgeToValue(t, "name", 1001, "Bob", nil)
	addEdgeToValue(t, "name", 1002, "Matt", nil)
	addEdgeToValue(t, "name", 1003, "John", nil)
	addEdgeToValue(t, "nick_name", 5010, "Two Terms", nil)

	addEdgeToValue(t, "alias", 23, "Zambo Alice", nil)
	addEdgeToValue(t, "alias", 24, "John Alice", nil)
	addEdgeToValue(t, "alias", 25, "Bob Joe", nil)
	addEdgeToValue(t, "alias", 31, "Allan Matt", nil)
	addEdgeToValue(t, "alias", 101, "John Oliver", nil)

	// Now let's add a few properties for the main user.
	addEdgeToValue(t, "name", 1, "Michonne", nil)
	addEdgeToValue(t, "gender", 1, "female", nil)
	addEdgeToValue(t, "full_name", 1, "Michonne's large name for hashing", nil)
	addEdgeToValue(t, "noindex_name", 1, "Michonne's name not indexed", nil)

	src := types.ValueForType(types.StringID)
	src.Value = []byte("{\"Type\":\"Point\", \"Coordinates\":[1.1,2.0]}")
	coord, err := types.Convert(src, types.GeoID)
	require.NoError(t, err)
	gData := types.ValueForType(types.BinaryID)
	err = types.Marshal(coord, &gData)
	require.NoError(t, err)
	addEdgeToTypedValue(t, "loc", 1, types.GeoID, gData.Value.([]byte), nil)
	addEdgeToTypedValue(t, "loc", 25, types.GeoID, gData.Value.([]byte), nil)

	// IntID
	data := types.ValueForType(types.BinaryID)
	intD := types.Val{Tid: types.IntID, Value: int64(15)}
	err = types.Marshal(intD, &data)
	require.NoError(t, err)

	// FloatID
	fdata := types.ValueForType(types.BinaryID)
	floatD := types.Val{Tid: types.FloatID, Value: float64(13.25)}
	err = types.Marshal(floatD, &fdata)
	require.NoError(t, err)
	addEdgeToTypedValue(t, "power", 1, types.FloatID, fdata.Value.([]byte), nil)

	addEdgeToValue(t, "address", 1, "31, 32 street, Jupiter", nil)

	boolD := types.Val{Tid: types.BoolID, Value: true}
	err = types.Marshal(boolD, &data)
	require.NoError(t, err)
	addEdgeToTypedValue(t, "alive", 1, types.BoolID, data.Value.([]byte), nil)
	addEdgeToTypedValue(t, "alive", 23, types.BoolID, data.Value.([]byte), nil)

	boolD = types.Val{Tid: types.BoolID, Value: false}
	err = types.Marshal(boolD, &data)
	require.NoError(t, err)
	addEdgeToTypedValue(t, "alive", 25, types.BoolID, data.Value.([]byte), nil)
	addEdgeToTypedValue(t, "alive", 31, types.BoolID, data.Value.([]byte), nil)

	addEdgeToValue(t, "age", 1, "38", nil)
	addEdgeToValue(t, "survival_rate", 1, "98.99", nil)
	addEdgeToValue(t, "sword_present", 1, "true", nil)
	addEdgeToValue(t, "_xid_", 1, "mich", nil)

	// Now let's add a name for each of the friends, except 101.
	addEdgeToTypedValue(t, "name", 23, types.StringID, []byte("Rick Grimes"), nil)
	addEdgeToValue(t, "gender", 23, "male", nil)
	addEdgeToValue(t, "age", 23, "15", nil)

	src.Value = []byte(`{"Type":"Polygon", "Coordinates":[[[0.0,0.0], [2.0,0.0], [2.0, 2.0], [0.0, 2.0], [0.0, 0.0]]]}`)
	coord, err = types.Convert(src, types.GeoID)
	require.NoError(t, err)
	gData = types.ValueForType(types.BinaryID)
	err = types.Marshal(coord, &gData)
	require.NoError(t, err)
	addEdgeToTypedValue(t, "loc", 23, types.GeoID, gData.Value.([]byte), nil)

	addEdgeToValue(t, "address", 23, "21, mark street, Mars", nil)
	addEdgeToValue(t, "name", 24, "Glenn Rhee", nil)
	addEdgeToValue(t, "_xid_", 24, `g"lenn`, nil)
	src.Value = []byte(`{"Type":"Point", "Coordinates":[1.10001,2.000001]}`)
	coord, err = types.Convert(src, types.GeoID)
	require.NoError(t, err)
	gData = types.ValueForType(types.BinaryID)
	err = types.Marshal(coord, &gData)
	require.NoError(t, err)
	addEdgeToTypedValue(t, "loc", 24, types.GeoID, gData.Value.([]byte), nil)

	addEdgeToValue(t, "name", 110, "Alice", nil)
	addEdgeToValue(t, "_xid_", 110, "a.bc", nil)
	addEdgeToValue(t, "name", 25, "Daryl Dixon", nil)
	addEdgeToValue(t, "name", 31, "Andrea", nil)
	addEdgeToValue(t, "name", 2300, "Andre", nil)
	addEdgeToValue(t, "name", 2333, "Helmut", nil)
	src.Value = []byte(`{"Type":"Point", "Coordinates":[2.0, 2.0]}`)
	coord, err = types.Convert(src, types.GeoID)
	require.NoError(t, err)
	gData = types.ValueForType(types.BinaryID)
	err = types.Marshal(coord, &gData)
	require.NoError(t, err)
	addEdgeToTypedValue(t, "loc", 31, types.GeoID, gData.Value.([]byte), nil)

	addEdgeToValue(t, "dob_day", 1, "1910-01-01", nil)

	// Note - Though graduation is of [dateTime] type. Don't add another graduation for Michonne.
	// There is a test to check that JSON should return an array even if there is only one value
	// for attribute whose type is a list type.
	addEdgeToValue(t, "graduation", 1, "1932-01-01", nil)
	addEdgeToValue(t, "dob_day", 23, "1910-01-02", nil)
	addEdgeToValue(t, "dob_day", 24, "1909-05-05", nil)
	addEdgeToValue(t, "dob_day", 25, "1909-01-10", nil)
	addEdgeToValue(t, "dob_day", 31, "1901-01-15", nil)
	addEdgeToValue(t, "graduation", 31, "1933-01-01", nil)
	addEdgeToValue(t, "graduation", 31, "1935-01-01", nil)

	addEdgeToValue(t, "dob", 1, "1910-01-01", nil)
	addEdgeToValue(t, "dob", 23, "1910-01-02", nil)
	addEdgeToValue(t, "dob", 24, "1909-05-05", nil)
	addEdgeToValue(t, "dob", 25, "1909-01-10", nil)
	addEdgeToValue(t, "dob", 31, "1901-01-15", nil)

	addEdgeToValue(t, "age", 24, "15", nil)
	addEdgeToValue(t, "age", 25, "17", nil)
	addEdgeToValue(t, "age", 31, "19", nil)

	f1 := types.Val{Tid: types.FloatID, Value: 1.6}
	fData := types.ValueForType(types.BinaryID)
	err = types.Marshal(f1, &fData)
	require.NoError(t, err)
	addEdgeToTypedValue(t, "survival_rate", 23, types.FloatID, fData.Value.([]byte), nil)
	addEdgeToTypedValue(t, "survival_rate", 24, types.FloatID, fData.Value.([]byte), nil)
	addEdgeToTypedValue(t, "survival_rate", 25, types.FloatID, fData.Value.([]byte), nil)
	addEdgeToTypedValue(t, "survival_rate", 31, types.FloatID, fData.Value.([]byte), nil)

	// GEO stuff
	p := geom.NewPoint(geom.XY).MustSetCoords(geom.Coord{-122.082506, 37.4249518})
	addGeoData(t, 5101, p, "Googleplex")

	p = geom.NewPoint(geom.XY).MustSetCoords(geom.Coord{-122.080668, 37.426753})
	addGeoData(t, 5102, p, "Shoreline Amphitheater")

	p = geom.NewPoint(geom.XY).MustSetCoords(geom.Coord{-122.2527428, 37.513653})
	addGeoData(t, 5103, p, "San Carlos Airport")

	poly := geom.NewPolygon(geom.XY).MustSetCoords([][]geom.Coord{
		{{-121.6, 37.1}, {-122.4, 37.3}, {-122.6, 37.8}, {-122.5, 38.3}, {-121.9, 38},
			{-121.6, 37.1}},
	})
	addGeoData(t, 5104, poly, "SF Bay area")
	poly = geom.NewPolygon(geom.XY).MustSetCoords([][]geom.Coord{
		{{-122.06, 37.37}, {-122.1, 37.36}, {-122.12, 37.4}, {-122.11, 37.43},
			{-122.04, 37.43}, {-122.06, 37.37}},
	})
	addGeoData(t, 5105, poly, "Mountain View")
	poly = geom.NewPolygon(geom.XY).MustSetCoords([][]geom.Coord{
		{{-122.25, 37.49}, {-122.28, 37.49}, {-122.27, 37.51}, {-122.25, 37.52},
			{-122.25, 37.49}},
	})
	addGeoData(t, 5106, poly, "San Carlos")

	multipoly, err := loadPolygon("testdata/nyc-coordinates.txt")
	require.NoError(t, err)
	addGeoData(t, 5107, multipoly, "New York")

	// We should get this back as a result as it should contain our Denver polygon.
	// multipoly, err := loadPolygon("testdata/us-coordinates.txt")
	// require.NoError(t, err)
	// addGeoData(t, 5108, multipoly, "USA")

	addEdgeToValue(t, "film.film.initial_release_date", 23, "1900-01-02", nil)
	addEdgeToValue(t, "film.film.initial_release_date", 24, "1909-05-05", nil)
	addEdgeToValue(t, "film.film.initial_release_date", 25, "1929-01-10", nil)
	addEdgeToValue(t, "film.film.initial_release_date", 31, "1801-01-15", nil)

	// for aggregator(sum) test
	{
		data := types.ValueForType(types.BinaryID)
		intD := types.Val{Tid: types.IntID, Value: int64(4)}
		err = types.Marshal(intD, &data)
		require.NoError(t, err)
		addEdgeToTypedValue(t, "shadow_deep", 23, types.IntID, data.Value.([]byte), nil)
	}
	{
		data := types.ValueForType(types.BinaryID)
		intD := types.Val{Tid: types.IntID, Value: int64(14)}
		err = types.Marshal(intD, &data)
		require.NoError(t, err)
		addEdgeToTypedValue(t, "shadow_deep", 24, types.IntID, data.Value.([]byte), nil)
	}

	// Natural Language Processing test data
	// 0x1001 is uid of interest for language tests
	addEdgeToLangValue(t, "name", 0x1001, "Badger", "", nil)
	addEdgeToLangValue(t, "name", 0x1001, "European badger", "en", nil)
	addEdgeToLangValue(t, "name", 0x1001, "European badger barger European", "xx", nil)
	addEdgeToLangValue(t, "name", 0x1001, "Borsuk europejski", "pl", nil)
	addEdgeToLangValue(t, "name", 0x1001, "Europäischer Dachs", "de", nil)
	addEdgeToLangValue(t, "name", 0x1001, "Барсук", "ru", nil)
	addEdgeToLangValue(t, "name", 0x1001, "Blaireau européen", "fr", nil)
	addEdgeToLangValue(t, "name", 0x1002, "Honey badger", "en", nil)
	addEdgeToLangValue(t, "name", 0x1003, "Honey bee", "en", nil)
	// data for bug (#945), also used by test for #1010
	addEdgeToLangValue(t, "name", 0x1004, "Артём Ткаченко", "ru", nil)
	addEdgeToLangValue(t, "name", 0x1004, "Artem Tkachenko", "en", nil)
	// data for bug (#1118)
	addEdgeToLangValue(t, "lossy", 0x1001, "Badger", "", nil)
	addEdgeToLangValue(t, "lossy", 0x1001, "European badger", "en", nil)
	addEdgeToLangValue(t, "lossy", 0x1001, "European badger barger European", "xx", nil)
	addEdgeToLangValue(t, "lossy", 0x1001, "Borsuk europejski", "pl", nil)
	addEdgeToLangValue(t, "lossy", 0x1001, "Europäischer Dachs", "de", nil)
	addEdgeToLangValue(t, "lossy", 0x1001, "Барсук", "ru", nil)
	addEdgeToLangValue(t, "lossy", 0x1001, "Blaireau européen", "fr", nil)
	addEdgeToLangValue(t, "lossy", 0x1002, "Honey badger", "en", nil)
	addEdgeToLangValue(t, "lossy", 0x1003, "Honey bee", "en", nil)

	// full_name has hash index, we need following data for bug with eq (#1295)
	addEdgeToLangValue(t, "royal_title", 0x10000, "Her Majesty Elizabeth the Second, by the Grace of God of the United Kingdom of Great Britain and Northern Ireland and of Her other Realms and Territories Queen, Head of the Commonwealth, Defender of the Faith", "en", nil)
	addEdgeToLangValue(t, "royal_title", 0x10000, "Sa Majesté Elizabeth Deux, par la grâce de Dieu Reine du Royaume-Uni, du Canada et de ses autres royaumes et territoires, Chef du Commonwealth, Défenseur de la Foi", "fr", nil)

	// regex test data
	// 0x1234 is uid of interest for regex testing
	addEdgeToValue(t, "name", 0x1234, "Regex Master", nil)
	nextId := uint64(0x2000)
	patterns := []string{"mississippi", "missouri", "mission", "missionary",
		"whissle", "transmission", "zipped", "monosiphonic", "vasopressin", "vapoured",
		"virtuously", "zurich", "synopsis", "subsensuously",
		"admission", "commission", "submission", "subcommission", "retransmission", "omission",
		"permission", "intermission", "dimission", "discommission",
	}

	for _, p := range patterns {
		addEdgeToValue(t, "value", nextId, p, nil)
		addEdgeToUID(t, "pattern", 0x1234, nextId, nil)
		nextId++
	}

	addEdgeToValue(t, "name", 240, "Andrea With no friends", nil)
	addEdgeToUID(t, "son", 1, 2300, nil)
	addEdgeToUID(t, "son", 1, 2333, nil)

	addEdgeToValue(t, "name", 2301, `Alice"`, nil)

	// Add some base64 encoded data
	addEdgeToTypedValue(t, "bin_data", 0x1, types.BinaryID, []byte("YmluLWRhdGE="), nil)

	// Data to check multi-sort.
	addEdgeToValue(t, "name", 10000, "Alice", nil)
	addEdgeToValue(t, "age", 10000, "25", nil)
	addEdgeToValue(t, "salary", 10000, "10000", nil)
	addEdgeToValue(t, "name", 10001, "Elizabeth", nil)
	addEdgeToValue(t, "age", 10001, "75", nil)
	addEdgeToValue(t, "name", 10002, "Alice", nil)
	addEdgeToValue(t, "age", 10002, "75", nil)
	addEdgeToValue(t, "salary", 10002, "10002", nil)
	addEdgeToValue(t, "name", 10003, "Bob", nil)
	addEdgeToValue(t, "age", 10003, "75", nil)
	addEdgeToValue(t, "name", 10004, "Alice", nil)
	addEdgeToValue(t, "age", 10004, "75", nil)
	addEdgeToValue(t, "name", 10005, "Bob", nil)
	addEdgeToValue(t, "age", 10005, "25", nil)
	addEdgeToValue(t, "name", 10006, "Colin", nil)
	addEdgeToValue(t, "age", 10006, "25", nil)
	addEdgeToValue(t, "name", 10007, "Elizabeth", nil)
	addEdgeToValue(t, "age", 10007, "25", nil)

	// Data to test inequality (specifically gt, lt) on exact tokenizer
	addEdgeToValue(t, "name", 3000, "mystocks", nil)
	addEdgeToValue(t, "symbol", 3001, "AAPL", nil)
	addEdgeToValue(t, "symbol", 3002, "AMZN", nil)
	addEdgeToValue(t, "symbol", 3003, "AMD", nil)
	addEdgeToValue(t, "symbol", 3004, "FB", nil)
	addEdgeToValue(t, "symbol", 3005, "GOOG", nil)
	addEdgeToValue(t, "symbol", 3006, "MSFT", nil)

	addEdgeToValue(t, "name", 3500, "", nil) // empty default name
	addEdgeToLangValue(t, "name", 3500, "상현", "ko", nil)
	addEdgeToValue(t, "name", 3501, "Alex", nil)
	addEdgeToLangValue(t, "name", 3501, "Alex", "en", nil)
	addEdgeToValue(t, "name", 3502, "", nil) // empty default name
	addEdgeToLangValue(t, "name", 3502, "Amit", "en", nil)
	addEdgeToLangValue(t, "name", 3502, "अमित", "hi", nil)
	addEdgeToLangValue(t, "name", 3503, "Andrew", "en", nil) // no default name & empty hi name
	addEdgeToLangValue(t, "name", 3503, "", "hi", nil)

	addEdgeToValue(t, "office", 4001, "office 1", nil)
	addEdgeToValue(t, "room", 4002, "room 1", nil)
	addEdgeToValue(t, "room", 4003, "room 2", nil)
	addEdgeToValue(t, "room", 4004, "", nil)
	addEdgeToUID(t, "office.room", 4001, 4002, nil)
	addEdgeToUID(t, "office.room", 4001, 4003, nil)
	addEdgeToUID(t, "office.room", 4001, 4004, nil)
}
