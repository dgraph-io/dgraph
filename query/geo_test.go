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
	"context"
	"encoding/json"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/twpayne/go-geom"

	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/group"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/store"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/worker"
)

func createTestStore(t *testing.T) (string, *store.Store) {
	dir, err := ioutil.TempDir("", "storetest_")
	require.NoError(t, err)

	ps, err := store.NewStore(dir)
	require.NoError(t, err)

	schema.ParseBytes([]byte(`scalar geometry:geo @index`))
	posting.Init(ps)
	worker.Init(ps)

	group.ParseGroupConfig("")
	createTestData(t, ps)
	return dir, ps
}

func addGeoData(t *testing.T, ps *store.Store, uid uint64, p geom.T, name string) {
	value := types.ValueForType(types.BinaryID)
	src := types.ValueForType(types.GeoID)
	src.Value = p
	err := types.Marshal(src, &value)
	require.NoError(t, err)
	addEdgeToTypedValue(t, ps, "geometry", uid, types.GeoID, value.Value.([]byte))
	addEdgeToTypedValue(t, ps, "name", uid, types.StringID, []byte(name))
}

func createTestData(t *testing.T, ps *store.Store) {
	dir, err := ioutil.TempDir("", "wal")
	require.NoError(t, err)
	worker.StartRaftNodes(dir)

	p := geom.NewPoint(geom.XY).MustSetCoords(geom.Coord{-122.082506, 37.4249518})
	addGeoData(t, ps, 1, p, "Googleplex")

	p = geom.NewPoint(geom.XY).MustSetCoords(geom.Coord{-122.080668, 37.426753})
	addGeoData(t, ps, 2, p, "Shoreline Amphitheater")

	p = geom.NewPoint(geom.XY).MustSetCoords(geom.Coord{-122.2527428, 37.513653})
	addGeoData(t, ps, 3, p, "San Carlos Airport")

	poly := geom.NewPolygon(geom.XY).MustSetCoords([][]geom.Coord{
		{{-121.6, 37.1}, {-122.4, 37.3}, {-122.6, 37.8}, {-122.5, 38.3}, {-121.9, 38},
			{-121.6, 37.1}},
	})
	addGeoData(t, ps, 4, poly, "SF Bay area")
	poly = geom.NewPolygon(geom.XY).MustSetCoords([][]geom.Coord{
		{{-122.06, 37.37}, {-122.1, 37.36}, {-122.12, 37.4}, {-122.11, 37.43},
			{-122.04, 37.43}, {-122.06, 37.37}},
	})
	addGeoData(t, ps, 5, poly, "Mountain View")
	poly = geom.NewPolygon(geom.XY).MustSetCoords([][]geom.Coord{
		{{-122.25, 37.49}, {-122.28, 37.49}, {-122.27, 37.51}, {-122.25, 37.52},
			{-122.24, 37.51}},
	})
	addGeoData(t, ps, 6, poly, "San Carlos")

	time.Sleep(200 * time.Millisecond) // Let the index process jobs from channel.
}

func runQuery(t *testing.T, gq *gql.GraphQuery) string {
	ctx := context.Background()
	ch := make(chan error)

	sg, err := ToSubGraph(ctx, gq)
	require.NoError(t, err)
	go ProcessGraph(ctx, sg, nil, ch)
	err = <-ch
	require.NoError(t, err)

	var l Latency
	js, err := sg.ToJSON(&l)
	require.NoError(t, err)
	j, err := json.Marshal(js)
	require.NoError(t, err)
	return string(j)
}

func TestWithinPoint(t *testing.T) {
	dir, ps := createTestStore(t)
	defer os.RemoveAll(dir)
	defer ps.Close()

	gq := &gql.GraphQuery{
		Alias: "me",
		Func: &gql.Function{
			Attr: "geometry",
			Name: "near",
			Args: []string{`[-122.082506, 37.4249518]`, "1"},
		},
		Children: []*gql.GraphQuery{&gql.GraphQuery{Attr: "name"}},
	}

	mp := runQuery(t, gq)
	expected := `{"me":[{"name":"Googleplex"}]}`
	require.JSONEq(t, expected, mp)
}

func TestWithinPolygon(t *testing.T) {
	dir, ps := createTestStore(t)
	defer os.RemoveAll(dir)
	defer ps.Close()

	gq := &gql.GraphQuery{
		Alias: "me",
		Func: &gql.Function{Attr: "geometry", Name: "within", Args: []string{
			`[[-122.06, 37.37], [-122.1, 37.36], [-122.12, 37.4], [-122.11, 37.43], [-122.04, 37.43], [-122.06, 37.37]]`},
		},
		Children: []*gql.GraphQuery{&gql.GraphQuery{Attr: "name"}},
	}

	mp := runQuery(t, gq)
	expected := `{"me":[{"name":"Googleplex"},{"name":"Shoreline Amphitheater"}]}`
	require.JSONEq(t, expected, mp)
}

func TestContainsPoint(t *testing.T) {
	dir, ps := createTestStore(t)
	defer os.RemoveAll(dir)
	defer ps.Close()

	gq := &gql.GraphQuery{
		Alias: "me",
		Func: &gql.Function{Attr: "geometry", Name: "contains", Args: []string{
			`[-122.082506, 37.4249518]`},
		},
		Children: []*gql.GraphQuery{&gql.GraphQuery{Attr: "name"}},
	}

	mp := runQuery(t, gq)
	expected := `{"me":[{"name":"SF Bay area"},{"name":"Mountain View"}]}`
	require.JSONEq(t, expected, mp)
}

func TestNearPoint(t *testing.T) {
	dir, ps := createTestStore(t)
	defer os.RemoveAll(dir)
	defer ps.Close()

	gq := &gql.GraphQuery{
		Alias: "me",
		Func: &gql.Function{
			Attr: "geometry",
			Name: "near",
			Args: []string{`[-122.082506, 37.4249518]`, "1000"},
		},
		Children: []*gql.GraphQuery{&gql.GraphQuery{Attr: "name"}},
	}

	mp := runQuery(t, gq)
	expected := `{"me":[{"name":"Googleplex"},{"name":"Shoreline Amphitheater"}]}`
	require.JSONEq(t, expected, mp)
}

func TestIntersectsPolygon1(t *testing.T) {
	dir, ps := createTestStore(t)
	defer os.RemoveAll(dir)
	defer ps.Close()

	gq := &gql.GraphQuery{
		Alias: "me",
		Func: &gql.Function{
			Attr: "geometry",
			Name: "intersects",
			Args: []string{
				`[[-122.06, 37.37], [-122.1, 37.36], 
					[-122.12, 37.4], [-122.11, 37.43], [-122.04, 37.43], [-122.06, 37.37]]`,
			},
		},
		Children: []*gql.GraphQuery{&gql.GraphQuery{Attr: "name"}},
	}

	mp := runQuery(t, gq)
	expected := `{"me":[{"name":"Googleplex"},{"name":"Shoreline Amphitheater"},
		{"name":"SF Bay area"},{"name":"Mountain View"}]}`
	require.JSONEq(t, expected, mp)
}

func TestIntersectsPolygon2(t *testing.T) {
	dir, ps := createTestStore(t)
	defer os.RemoveAll(dir)
	defer ps.Close()

	gq := &gql.GraphQuery{
		Alias: "me",
		Func: &gql.Function{
			Attr: "geometry",
			Name: "intersects",
			Args: []string{
				`[[-121.6, 37.1], [-122.4, 37.3], 
					[-122.6, 37.8], [-122.5, 38.3], [-121.9, 38], [-121.6, 37.1]]`,
			},
		},
		Children: []*gql.GraphQuery{&gql.GraphQuery{Attr: "name"}},
	}

	mp := runQuery(t, gq)
	expected := `{"me":[{"name":"Googleplex"},{"name":"Shoreline Amphitheater"},
			{"name":"San Carlos Airport"},{"name":"SF Bay area"},
			{"name":"Mountain View"},{"name":"San Carlos"}]}`
	require.JSONEq(t, expected, mp)
}
