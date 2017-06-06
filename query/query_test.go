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
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/dgraph-io/badger/badger"
	"github.com/gogo/protobuf/proto"
	"github.com/stretchr/testify/require"
	geom "github.com/twpayne/go-geom"

	"github.com/dgraph-io/dgraph/algo"
	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/group"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos"

	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/worker"
	"github.com/dgraph-io/dgraph/x"
)

func addPassword(t *testing.T, uid uint64, attr, password string) {
	value := types.ValueForType(types.BinaryID)
	src := types.ValueForType(types.PasswordID)
	src.Value, _ = types.Encrypt(password)
	err := types.Marshal(src, &value)
	require.NoError(t, err)
	addEdgeToTypedValue(t, attr, uid, types.PasswordID, value.Value.([]byte), nil)
}

var ps *badger.KV

func populateGraph(t *testing.T) {
	x.AssertTrue(ps != nil)
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

	// IntID
	data := types.ValueForType(types.BinaryID)
	intD := types.Val{types.IntID, int64(15)}
	err = types.Marshal(intD, &data)
	require.NoError(t, err)
	addEdgeToTypedValue(t, "age", 1, types.IntID, data.Value.([]byte), nil)

	// FloatID
	fdata := types.ValueForType(types.BinaryID)
	floatD := types.Val{types.FloatID, float64(13.25)}
	err = types.Marshal(floatD, &fdata)
	require.NoError(t, err)
	addEdgeToTypedValue(t, "power", 1, types.FloatID, fdata.Value.([]byte), nil)

	addEdgeToValue(t, "address", 1, "31, 32 street, Jupiter", nil)

	boolD := types.Val{types.BoolID, true}
	err = types.Marshal(boolD, &data)
	require.NoError(t, err)
	addEdgeToTypedValue(t, "alive", 1, types.BoolID, data.Value.([]byte), nil)
	addEdgeToTypedValue(t, "alive", 23, types.BoolID, data.Value.([]byte), nil)

	boolD = types.Val{types.BoolID, false}
	err = types.Marshal(boolD, &data)
	require.NoError(t, err)
	addEdgeToTypedValue(t, "alive", 25, types.BoolID, data.Value.([]byte), nil)
	addEdgeToTypedValue(t, "alive", 31, types.BoolID, data.Value.([]byte), nil)

	addEdgeToValue(t, "age", 1, "38", nil)
	addEdgeToValue(t, "survival_rate", 1, "98.99", nil)
	addEdgeToValue(t, "sword_present", 1, "true", nil)
	addEdgeToValue(t, "_xid_", 1, "mich", nil)

	addPassword(t, 1, "password", "123456")

	// Now let's add a name for each of the friends, except 101.
	addEdgeToTypedValue(t, "name", 23, types.StringID, []byte("Rick Grimes"), nil)
	addEdgeToValue(t, "age", 23, "15", nil)
	addPassword(t, 23, "pass", "654321")

	src.Value = []byte(`{"Type":"Polygon", "Coordinates":[[[0.0,0.0], [2.0,0.0], [2.0, 2.0], [0.0, 2.0], [0.0, 0.0]]]}`)
	coord, err = types.Convert(src, types.GeoID)
	require.NoError(t, err)
	gData = types.ValueForType(types.BinaryID)
	err = types.Marshal(coord, &gData)
	require.NoError(t, err)
	addEdgeToTypedValue(t, "loc", 23, types.GeoID, gData.Value.([]byte), nil)

	addEdgeToValue(t, "address", 23, "21, mark street, Mars", nil)
	addEdgeToValue(t, "name", 24, "Glenn Rhee", nil)
	addEdgeToValue(t, "_xid_", 24, `g\"lenn`, nil)
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
	src.Value = []byte(`{"Type":"Point", "Coordinates":[2.0, 2.0]}`)
	coord, err = types.Convert(src, types.GeoID)
	require.NoError(t, err)
	gData = types.ValueForType(types.BinaryID)
	err = types.Marshal(coord, &gData)
	require.NoError(t, err)
	addEdgeToTypedValue(t, "loc", 31, types.GeoID, gData.Value.([]byte), nil)

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
	addGeoData(t, ps, 5101, p, "Googleplex")

	p = geom.NewPoint(geom.XY).MustSetCoords(geom.Coord{-122.080668, 37.426753})
	addGeoData(t, ps, 5102, p, "Shoreline Amphitheater")

	p = geom.NewPoint(geom.XY).MustSetCoords(geom.Coord{-122.2527428, 37.513653})
	addGeoData(t, ps, 5103, p, "San Carlos Airport")

	poly := geom.NewPolygon(geom.XY).MustSetCoords([][]geom.Coord{
		{{-121.6, 37.1}, {-122.4, 37.3}, {-122.6, 37.8}, {-122.5, 38.3}, {-121.9, 38},
			{-121.6, 37.1}},
	})
	addGeoData(t, ps, 5104, poly, "SF Bay area")
	poly = geom.NewPolygon(geom.XY).MustSetCoords([][]geom.Coord{
		{{-122.06, 37.37}, {-122.1, 37.36}, {-122.12, 37.4}, {-122.11, 37.43},
			{-122.04, 37.43}, {-122.06, 37.37}},
	})
	addGeoData(t, ps, 5105, poly, "Mountain View")
	poly = geom.NewPolygon(geom.XY).MustSetCoords([][]geom.Coord{
		{{-122.25, 37.49}, {-122.28, 37.49}, {-122.27, 37.51}, {-122.25, 37.52},
			{-122.24, 37.51}},
	})
	addGeoData(t, ps, 5106, poly, "San Carlos")

	addEdgeToValue(t, "film.film.initial_release_date", 23, "1900-01-02", nil)
	addEdgeToValue(t, "film.film.initial_release_date", 24, "1909-05-05", nil)
	addEdgeToValue(t, "film.film.initial_release_date", 25, "1929-01-10", nil)
	addEdgeToValue(t, "film.film.initial_release_date", 31, "1801-01-15", nil)

	// for aggregator(sum) test
	{
		data := types.ValueForType(types.BinaryID)
		intD := types.Val{types.IntID, int64(4)}
		err = types.Marshal(intD, &data)
		require.NoError(t, err)
		addEdgeToTypedValue(t, "shadow_deep", 23, types.IntID, data.Value.([]byte), nil)
	}
	{
		data := types.ValueForType(types.BinaryID)
		intD := types.Val{types.IntID, int64(14)}
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
	// data for bug (#945)
	addEdgeToLangValue(t, "name", 0x1004, "Артём Ткаченко", "ru", nil)
	addEdgeToLangValue(t, "name", 0x1004, "Artem Tkachenko", "en", nil)

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

	addEdgeToValue(t, "name", 2301, `Alice\"`, nil)

	time.Sleep(5 * time.Millisecond)
}

func TestGetUID(t *testing.T) {
	populateGraph(t)
	query := `
		{
			me(id:0x01) {
				name
				_uid_
				gender
				alive
				friend {
					_uid_
					name
				}
			}
		}
	`
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"_uid_":"0x1","alive":true,"friend":[{"_uid_":"0x17","name":"Rick Grimes"},{"_uid_":"0x18","name":"Glenn Rhee"},{"_uid_":"0x19","name":"Daryl Dixon"},{"_uid_":"0x1f","name":"Andrea"},{"_uid_":"0x65"}],"gender":"female","name":"Michonne"}]}`,
		js)
}

func TestReturnUids(t *testing.T) {
	populateGraph(t)
	query := `
		{
			me(id:0x01) {
				name
				_uid_
				gender
				alive
				friend {
					_uid_
					name
				}
			}
		}
	`
	res, err := gql.Parse(gql.Request{Str: query})
	require.NoError(t, err)

	var l Latency
	ctx := context.Background()
	sgl, err := ProcessQuery(ctx, res, &l)
	require.NoError(t, err)

	var buf bytes.Buffer
	mp := map[string]string{
		"a": "123",
	}
	require.NoError(t, ToJson(&l, sgl, &buf, mp, false))
	js := buf.String()
	require.JSONEq(t,
		`{"uids":{"a":"123"},"me":[{"_uid_":"0x1","alive":true,"friend":[{"_uid_":"0x17","name":"Rick Grimes"},{"_uid_":"0x18","name":"Glenn Rhee"},{"_uid_":"0x19","name":"Daryl Dixon"},{"_uid_":"0x1f","name":"Andrea"},{"_uid_":"0x65"}],"gender":"female","name":"Michonne"}]}`,
		js)
}

func TestGetUIDNotInChild(t *testing.T) {
	populateGraph(t)
	query := `
		{
			me(id:0x01) {
				name
				_uid_
				gender
				alive
				friend {
					name
				}
			}
		}
	`
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"_uid_":"0x1","alive":true,"gender":"female","name":"Michonne", "friend":[{"name":"Rick Grimes"},{"name":"Glenn Rhee"},{"name":"Daryl Dixon"},{"name":"Andrea"}]}]}`,
		js)
}

func TestCascadeDirective(t *testing.T) {
	populateGraph(t)
	query := `
		{
			me(id:0x01) @cascade {
				name
				gender
				friend {
					name
					friend{
						name
						dob
						age
					}
				}
			}
		}
	`

	js := processToFastJSON(t, query)
	require.JSONEq(t, `{"me":[{"friend":[{"friend":[{"age":38,"dob":"1910-01-01","name":"Michonne"}],"name":"Rick Grimes"},{"friend":[{"age":15,"dob":"1909-05-05","name":"Glenn Rhee"}],"name":"Andrea"}],"gender":"female","name":"Michonne"}]}`,
		js)
}

func TestLevelBasedFacetVarSum(t *testing.T) {
	populateGraph(t)
	query := `
		{
			friend(id: 1000) {
				path @facets(L1 as weight) {
						path @facets(L2 as weight) {
							c as count(follow)
						}
						L4 as math(c+L2+L1)
				}
			}

			sum(id: var(L4), orderdesc: var(L4)) {
				name
				var(L4)
			}
		}
	`
	js := processToFastJSON(t, query)
	require.JSONEq(t, `{"friend":[{"path":[{"@facets":{"_":{"weight":0.100000}},"path":[{"@facets":{"_":{"weight":0.100000}},"count(follow)":1},{"@facets":{"_":{"weight":1.500000}},"count(follow)":1}]},{"@facets":{"_":{"weight":0.700000}},"path":[{"@facets":{"_":{"weight":0.600000}},"count(follow)":1}],"var(L4)":1.200000}]}],"sum":[{"name":"John","var(L4)":3.900000},{"name":"Matt","var(L4)":1.200000}]}`, js)
}

func TestLevelBasedFacetVarSumError(t *testing.T) {
	populateGraph(t)
	query := `
		{
			friend(id: 1000) {
				path @facets(L1 as weight)
				follow {
					path @facets(L2 as weight)
					L3 as math(L1+L2)
				}
			}

			sum(id: var(L3), orderdesc: var(L3)) {
				name
				var(L3)
			}
		}
	`
	res, err := gql.Parse(gql.Request{Str: query})
	require.NoError(t, err)

	var l Latency
	ctx := context.Background()
	_, err = ProcessQuery(ctx, res, &l)
	require.Error(t, err)
}

func TestLevelBasedSumMix1(t *testing.T) {
	populateGraph(t)
	query := `
		{
			friend(id: 1) {
				a as age
				path @facets(L1 as weight) {
					L2 as math(a+L1)
			 	}
			}
			sum(id: var(L2), orderdesc: var(L2)) {
				name
				var(L2)
			}
		}
	`
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"friend":[{"age":38,"path":[{"@facets":{"_":{"weight":0.200000}},"var(L2)":38.200000},{"@facets":{"_":{"weight":0.100000}},"var(L2)":38.100000}]}],"sum":[{"name":"Glenn Rhee","var(L2)":38.200000},{"name":"Andrea","var(L2)":38.100000}]}`,
		js)
}

func TestLevelBasedFacetVarSum1(t *testing.T) {
	populateGraph(t)
	query := `
		{
			friend(id: 1000) {
				path @facets(L1 as weight) {
					name
					path @facets(L2 as weight)
					L3 as math(L1+L2)
			 }
			}
			sum(id: var(L3), orderdesc: var(L3)) {
				name
				var(L3)
			}
		}
	`
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"friend":[{"path":[{"@facets":{"_":{"weight":0.100000}},"name":"Bob","path":[{"@facets":{"_":{"weight":0.100000}}},{"@facets":{"_":{"weight":1.500000}}}]},{"@facets":{"_":{"weight":0.700000}},"name":"Matt","path":[{"@facets":{"_":{"weight":0.600000}}}],"var(L3)":0.200000}]}],"sum":[{"name":"John","var(L3)":2.900000},{"name":"Matt","var(L3)":0.200000}]}`,
		js)
}

func TestLevelBasedFacetVarSum2(t *testing.T) {
	populateGraph(t)
	query := `
		{
			friend(id: 1000) {
				path @facets(L1 as weight) {
					path @facets(L2 as weight) {
						path @facets(L3 as weight)
						L4 as math(L1+L2+L3)
					}
				}
			}
			sum(id: var(L4), orderdesc: var(L4)) {
				name
				var(L4)
			}
		}
	`
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"friend":[{"path":[{"@facets":{"_":{"weight":0.100000}},"path":[{"@facets":{"_":{"weight":0.100000}},"path":[{"@facets":{"_":{"weight":0.600000}}}]},{"@facets":{"_":{"weight":1.500000}},"var(L4)":0.800000}]},{"@facets":{"_":{"weight":0.700000}},"path":[{"@facets":{"_":{"weight":0.600000}},"var(L4)":0.800000}]}]}],"sum":[{"name":"Bob","var(L4)":2.900000},{"name":"John","var(L4)":0.800000}]}`,
		js)
}

func TestQueryConstMathVal(t *testing.T) {
	populateGraph(t)
	query := `
		{
			f as var(func: anyofterms(name, "Rick Michonne Andrea")) {
				a as math(24/8 * 3)
			}

			AgeOrder(id: var(f)) {
				name
				var(a)
			}
		}
	`
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"AgeOrder":[{"name":"Michonne","var(a)":9.000000},{"name":"Rick Grimes","var(a)":9.000000},{"name":"Andrea","var(a)":9.000000},{"name":"Andrea With no friends","var(a)":9.000000}]}`,
		js)
}

func TestQueryVarValAggSince(t *testing.T) {
	populateGraph(t)
	query := `
		{
			f as var(func: anyofterms(name, "Michonne Andrea Rick")) {
				a as dob
				b as math(since(a)/(60*60*24*365))
			}

			AgeOrder(id: var(f), orderasc: var(b)) {
				name
				var(a)
			}
		}
	`
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"AgeOrder":[{"name":"Rick Grimes","var(a)":"1910-01-02"},{"name":"Michonne","var(a)":"1910-01-01"},{"name":"Andrea","var(a)":"1901-01-15"}]}`,
		js)
}

func TestQueryVarValAggNestedFuncConst(t *testing.T) {
	populateGraph(t)
	query := `
		{
			f as var(func: anyofterms(name, "Michonne Andrea Rick")) {
				a as age
				friend {
					x as age
				}
				n as min(var(x))
				s as max(var(x))
				p as math(a + s % n + 10)
				q as math(a * s * n * -1)
			}

			MaxMe(id: var(f), orderasc: var(p)) {
				name
				var(p)
				var(a)
				var(n)
				var(s)
			}

			MinMe(id: var(f), orderasc: var(q)) {
				name
				var(q)
				var(a)
				var(n)
				var(s)
			}
		}
	`
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"MaxMe":[{"name":"Rick Grimes","var(a)":15,"var(n)":38,"var(p)":25.000000,"var(s)":38},{"name":"Andrea","var(a)":19,"var(n)":15,"var(p)":29.000000,"var(s)":15},{"name":"Michonne","var(a)":38,"var(n)":15,"var(p)":52.000000,"var(s)":19}],"MinMe":[{"name":"Rick Grimes","var(a)":15,"var(n)":38,"var(q)":-21660.000000,"var(s)":38},{"name":"Michonne","var(a)":38,"var(n)":15,"var(q)":-10830.000000,"var(s)":19},{"name":"Andrea","var(a)":19,"var(n)":15,"var(q)":-4275.000000,"var(s)":15}]}`,
		js)
}

func TestQueryVarValAggNestedFuncMinMaxVars(t *testing.T) {
	populateGraph(t)
	query := `
		{
			f as var(func: anyofterms(name, "Michonne Andrea Rick")) {
				a as age
				friend {
					x as age
				}
				n as min(var(x))
				s as max(var(x))
				p as math(max(max(a, s), n))
				q as math(min(min(a, s), n))
			}

			MaxMe(id: var(f), orderasc: var(p)) {
				name
				var(p)
				var(a)
				var(n)
				var(s)
			}

			MinMe(id: var(f), orderasc: var(q)) {
				name
				var(q)
				var(a)
				var(n)
				var(s)
			}
		}
	`
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"MinMe":[{"name":"Michonne","var(a)":38,"var(n)":15,"var(q)":15,"var(s)":19},{"name":"Rick Grimes","var(a)":15,"var(n)":38,"var(q)":15,"var(s)":38},{"name":"Andrea","var(a)":19,"var(n)":15,"var(q)":15,"var(s)":15}],"MaxMe":[{"name":"Andrea","var(a)":19,"var(n)":15,"var(p)":19,"var(s)":15},{"name":"Michonne","var(a)":38,"var(n)":15,"var(p)":38,"var(s)":19},{"name":"Rick Grimes","var(a)":15,"var(n)":38,"var(p)":38,"var(s)":38}]}`,
		js)
}

func TestQueryVarValAggNestedFuncConditional(t *testing.T) {
	populateGraph(t)
	query := `
	{
			f as var(func: anyofterms(name, "Michonne Andrea Rick")) {
				a as age
				friend {
					x as age
				}
				n as min(var(x))
				condLog as math(cond(a > 10, logbase(n, 5), 1))
				condExp as math(cond(a < 40, 1, pow(2, n)))
			}

			LogMe(id: var(f), orderasc: var(condLog)) {
				name
				var(condLog)
				var(n)
				var(a)
			}

			ExpMe(id: var(f), orderasc: var(condExp)) {
				name
				var(condExp)
				var(n)
				var(a)
			}
		}
	`
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"ExpMe":[{"name":"Michonne","var(a)":38,"var(condExp)":1.000000,"var(n)":15},{"name":"Rick Grimes","var(a)":15,"var(condExp)":1.000000,"var(n)":38},{"name":"Andrea","var(a)":19,"var(condExp)":1.000000,"var(n)":15}],"LogMe":[{"name":"Michonne","var(a)":38,"var(condLog)":1.682606,"var(n)":15},{"name":"Andrea","var(a)":19,"var(condLog)":1.682606,"var(n)":15},{"name":"Rick Grimes","var(a)":15,"var(condLog)":2.260159,"var(n)":38}]}`,
		js)
}

func TestQueryVarValAggNestedFuncConditional2(t *testing.T) {
	populateGraph(t)
	query := `
		{
			f as var(func: anyofterms(name, "Michonne Andrea Rick")) {
				a as age
				friend {
					x as age
				}
				n as min(var(x))
				condLog as math(cond(a==38, n/2, 1))
				condExp as math(cond(a!=38, 1, sqrt(2*n)))
			}

			LogMe(id: var(f), orderasc: var(condLog)) {
				name
				var(condLog)
				var(n)
				var(a)
			}

			ExpMe(id: var(f), orderasc: var(condExp)) {
				name
				var(condExp)
				var(n)
				var(a)
			}
		}
	`
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"ExpMe":[{"name":"Rick Grimes","var(a)":15,"var(condExp)":1.000000,"var(n)":38},{"name":"Andrea","var(a)":19,"var(condExp)":1.000000,"var(n)":15},{"name":"Michonne","var(a)":38,"var(condExp)":5.477226,"var(n)":15}],"LogMe":[{"name":"Rick Grimes","var(a)":15,"var(condLog)":1.000000,"var(n)":38},{"name":"Andrea","var(a)":19,"var(condLog)":1.000000,"var(n)":15},{"name":"Michonne","var(a)":38,"var(condLog)":7.500000,"var(n)":15}]}`,
		js)
}

func TestQueryVarValAggNestedFuncUnary(t *testing.T) {
	populateGraph(t)
	query := `
		{
			f as var(func: anyofterms(name, "Michonne Andrea Rick")) {
				a as age
				friend {
					x as age
				}
				n as min(var(x))
				s as max(var(x))
				combiLog as math(a + ln(s - n))
				combiExp as math(a + exp(s - n))
			}

			LogMe(id: var(f), orderasc: var(combiLog)) {
				name
				var(combiLog)
				var(a)
				var(n)
				var(s)
			}

			ExpMe(id: var(f), orderasc: var(combiExp)) {
				name
				var(combiExp)
				var(a)
				var(n)
				var(s)
			}
		}
	`
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"ExpMe":[{"name":"Rick Grimes","var(a)":15,"var(combiExp)":16.000000,"var(n)":38,"var(s)":38},{"name":"Andrea","var(a)":19,"var(combiExp)":20.000000,"var(n)":15,"var(s)":15},{"name":"Michonne","var(a)":38,"var(combiExp)":92.598150,"var(n)":15,"var(s)":19}],"LogMe":[{"name":"Rick Grimes","var(a)":15,"var(combiLog)":-179769313486231570814527423731704356798070567525844996598917476803157260780028538760589558632766878171540458953514382464234321326889464182768467546703537516986049910576551282076245490090389328944075868508455133942304583236903222948165808559332123348274797826204144723168738177180919299881250404026184124858368.000000,"var(n)":38,"var(s)":38},{"name":"Andrea","var(a)":19,"var(combiLog)":-179769313486231570814527423731704356798070567525844996598917476803157260780028538760589558632766878171540458953514382464234321326889464182768467546703537516986049910576551282076245490090389328944075868508455133942304583236903222948165808559332123348274797826204144723168738177180919299881250404026184124858368.000000,"var(n)":15,"var(s)":15},{"name":"Michonne","var(a)":38,"var(combiLog)":39.386294,"var(n)":15,"var(s)":19}]}`,
		js)
}

func TestQueryVarValAggNestedFunc(t *testing.T) {
	populateGraph(t)
	query := `
		{
			f as var(func: anyofterms(name, "Michonne Andrea Rick")) {
				a as age
				friend {
					x as age
				}
				n as min(var(x))
				s as max(var(x))
				combi as math(a + n * s)
			}

			me(id: var(f), orderasc: var(combi)) {
				name
				var(combi)
				var(a)
				var(n)
				var(s)
			}
		}
	`
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"name":"Andrea","var(a)":19,"var(combi)":244,"var(n)":15,"var(s)":15},{"name":"Michonne","var(a)":38,"var(combi)":323,"var(n)":15,"var(s)":19},{"name":"Rick Grimes","var(a)":15,"var(combi)":1459,"var(n)":38,"var(s)":38}]}`,
		js)
}

func TestQueryVarValAggMinMaxSelf(t *testing.T) {
	populateGraph(t)
	query := `
		{
			f as var(func: anyofterms(name, "Michonne Andrea Rick")) {
				a as age
				friend {
					x as age
				}
				n as min(var(x))
				s as max(var(x))
				sum as math(n +  a + s)
			}

			me(id: var(f), orderasc: var(sum)) {
				name
				var(sum)
				var(s)
			}
		}
	`
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"name":"Andrea","var(s)":15,"var(sum)":49},{"name":"Michonne","var(s)":19,"var(sum)":72},{"name":"Rick Grimes","var(s)":38,"var(sum)":91}]}`,
		js)
}

func TestQueryVarValAggMinMax(t *testing.T) {
	populateGraph(t)
	query := `
		{
			f as var(func: anyofterms(name, "Michonne Andrea Rick")) {
				friend {
					x as age
				}
				n as min(var(x))
				s as max(var(x))
				sum as math(n + s)
			}

			me(id: var(f), orderdesc: var(sum)) {
				name
				var(n)
				var(s)
			}
		}
	`
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"name":"Rick Grimes","var(n)":38,"var(s)":38},{"name":"Michonne","var(n)":15,"var(s)":19},{"name":"Andrea","var(n)":15,"var(s)":15}]}`,
		js)
}

func TestQueryVarValAggMinMaxAlias(t *testing.T) {
	populateGraph(t)
	query := `
		{
			f as var(func: anyofterms(name, "Michonne Andrea Rick")) {
				friend {
					x as age
				}
				n as min(var(x))
				s as max(var(x))
				sum as math(n + s)
			}

			me(id: var(f), orderdesc: var(sum)) {
				name
				MinAge: var(n)
				MaxAge: var(s)
			}
		}
	`
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"name":"Rick Grimes","MinAge":38,"MaxAge":38},{"name":"Michonne","MinAge":15,"MaxAge":19},{"name":"Andrea","MinAge":15,"MaxAge":15}]}`,
		js)
}

func TestQueryVarValAggMul(t *testing.T) {
	populateGraph(t)
	query := `
		{
			var(id: 1) {
				f as friend {
					n as age
					s as count(friend)
					mul as math(n * s)
				}
			}

			me(id: var(f), orderdesc: var(mul)) {
				name
				var(s)
				var(n)
				var(mul)
			}
		}
	`
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"name":"Andrea","var(mul)":19.000000,"var(n)":19,"var(s)":1},{"name":"Rick Grimes","var(mul)":15.000000,"var(n)":15,"var(s)":1},{"name":"Glenn Rhee","var(mul)":0.000000,"var(n)":15,"var(s)":0},{"name":"Daryl Dixon","var(mul)":0.000000,"var(n)":17,"var(s)":0},{"var(mul)":0.000000,"var(s)":0}]}`,
		js)
}

func TestQueryVarValAggOrderDesc(t *testing.T) {
	populateGraph(t)
	query := `
		{
			info(id: 1) {
				f as friend {
					n as age
					s as count(friend)
					sum as math(n + s)
				}
			}

			me(id: var(f), orderdesc: var(sum)) {
				name
				age
				count(friend)
			}
		}
	`
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"info":[{"friend":[{"age":15,"count(friend)":1,"var(sum)":16.000000},{"age":15,"count(friend)":0,"var(sum)":15.000000},{"age":17,"count(friend)":0,"var(sum)":17.000000},{"age":19,"count(friend)":1,"var(sum)":20.000000},{"count(friend)":0,"var(sum)":0.000000}]}],"me":[{"age":19,"count(friend)":1,"name":"Andrea"},{"age":17,"count(friend)":0,"name":"Daryl Dixon"},{"age":15,"count(friend)":1,"name":"Rick Grimes"},{"age":15,"count(friend)":0,"name":"Glenn Rhee"},{"count(friend)":0}]}`,
		js)
}

func TestQueryVarValAggOrderAsc(t *testing.T) {
	populateGraph(t)
	query := `
		{
			var(id: 1) {
				f as friend {
					n as age
					s as survival_rate
					sum as math(n + s)
				}
			}

			me(id: var(f), orderasc: var(sum)) {
				name
				age
				survival_rate
			}
		}
	`
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"age":15,"name":"Rick Grimes","survival_rate":1.600000},{"age":15,"name":"Glenn Rhee","survival_rate":1.600000},{"age":17,"name":"Daryl Dixon","survival_rate":1.600000},{"age":19,"name":"Andrea","survival_rate":1.600000}]}`,
		js)
}

func TestQueryVarValOrderAsc(t *testing.T) {
	populateGraph(t)
	query := `
		{
			var(id: 1) {
				f as friend {
					n as name
				}
			}

			me(id: var(f), orderasc: var(n)) {
				name
			}
		}
	`
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"name":"Andrea"},{"name":"Daryl Dixon"},{"name":"Glenn Rhee"},{"name":"Rick Grimes"}]}`,
		js)
}

func TestQueryVarValOrderDob(t *testing.T) {
	populateGraph(t)
	query := `
		{
			var(id: 1) {
				f as friend {
					n as dob
				}
			}

			me(id: var(f), orderasc: var(n)) {
				name
				dob
			}
		}
	`
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"name":"Andrea", "dob":"1901-01-15"},{"name":"Daryl Dixon", "dob":"1909-01-10"},{"name":"Glenn Rhee", "dob":"1909-05-05"},{"name":"Rick Grimes", "dob":"1910-01-02"}]}`,
		js)
}

func TestQueryVarValOrderDesc(t *testing.T) {
	populateGraph(t)
	query := `
		{
			var(id: 1) {
				f as friend {
					n as name
				}
			}

			me(id: var(f), orderdesc: var(n)) {
				name
			}
		}
	`
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"name":"Rick Grimes"},{"name":"Glenn Rhee"},{"name":"Daryl Dixon"},{"name":"Andrea"}]}`,
		js)
}

func TestQueryVarValOrderDescMissing(t *testing.T) {
	populateGraph(t)
	query := `
		{
			var(id: 1034) {
				f As friend {
					n As name
				}
			}

			me(id: var(f), orderdesc: var(n)) {
				name
			}
		}
	`
	js := processToFastJSON(t, query)
	require.JSONEq(t, `{}`, js)
}

func TestGroupByRootProto(t *testing.T) {
	populateGraph(t)
	query := `
	{
		me(id: [1, 23, 24, 25, 31]) @groupby(age) {
				count(_uid_)
		}
	}
	`
	pb := processToPB(t, query, map[string]string{}, false)
	resreq := `attribute: "_root_"
children: <
  attribute: "me"
  children: <
    attribute: "@groupby"
    properties: <
      prop: "age"
      value: <
        int_val: 17
      >
    >
    properties: <
      prop: "count"
      value: <
        int_val: 1
      >
    >
  >
  children: <
    attribute: "@groupby"
    properties: <
      prop: "age"
      value: <
        int_val: 19
      >
    >
    properties: <
      prop: "count"
      value: <
        int_val: 1
      >
    >
  >
  children: <
    attribute: "@groupby"
    properties: <
      prop: "age"
      value: <
        int_val: 38
      >
    >
    properties: <
      prop: "count"
      value: <
        int_val: 1
      >
    >
  >
  children: <
    attribute: "@groupby"
    properties: <
      prop: "age"
      value: <
        int_val: 15
      >
    >
    properties: <
      prop: "count"
      value: <
        int_val: 2
      >
    >
  >
>
`
	res := proto.MarshalTextString(pb[0])
	require.EqualValues(t,
		resreq,
		res)
}

func TestGroupByRoot(t *testing.T) {
	populateGraph(t)
	query := `
	{
		me(id: [1, 23, 24, 25, 31]) @groupby(age) {
				count(_uid_)
		}
	}
	`
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"@groupby":[{"age":17,"count":1},{"age":19,"count":1},{"age":38,"count":1},{"age":15,"count":2}]}]}`,
		js)
}
func TestGroupBy_RepeatAttr(t *testing.T) {
	populateGraph(t)
	query := `
	{
		me(id: 1) {
			friend @groupby(age) {
				count(_uid_)
			}
			friend {
				name
				age
			}
			name
		}
	}
	`
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"friend":[{"@groupby":[{"age":17,"count":1},{"age":19,"count":1},{"age":15,"count":2}]},{"age":15,"name":"Rick Grimes"},{"age":15,"name":"Glenn Rhee"},{"age":17,"name":"Daryl Dixon"},{"age":19,"name":"Andrea"}],"name":"Michonne"}]}`,
		js)
}

func TestGroupBy(t *testing.T) {
	populateGraph(t)
	query := `
	{
		age(id: 1) {
			friend {
				age
				name
			}
		}

		me(id: 1) {
			friend @groupby(age) {
				count(_uid_)
			}
			name
		}
	}
	`
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"age":[{"friend":[{"age":15,"name":"Rick Grimes"},{"age":15,"name":"Glenn Rhee"},{"age":17,"name":"Daryl Dixon"},{"age":19,"name":"Andrea"}]}],"me":[{"friend":[{"@groupby":[{"age":17,"count":1},{"age":19,"count":1},{"age":15,"count":2}]}],"name":"Michonne"}]}`,
		js)
}

func TestGroupByCountVar(t *testing.T) {
	populateGraph(t)
	query := `
		{
			var(id: 1) {
				friend @groupby(school) {
					a as count(_uid_)
				}
			}

			order(id:var(a), orderdesc: var(a)) {
				name
				var(a)
			}
		}
	`
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"order":[{"name":"School B","var(a)":3},{"name":"School A","var(a)":2}]}`,
		js)
}
func TestGroupByAggVar(t *testing.T) {
	populateGraph(t)
	query := `
		{
			var(id: 1) {
				friend @groupby(school) {
					a as max(name)
					b as min(name)
				}
			}

			orderMax(id:var(a), orderdesc: var(a)) {
				name
				var(a)
			}

			orderMin(id:var(b), orderdesc: var(b)) {
				name
				var(b)
			}
		}
	`
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"orderMax":[{"name":"School B","var(a)":"Rick Grimes"},{"name":"School A","var(a)":"Glenn Rhee"}],"orderMin":[{"name":"School A","var(b)":"Daryl Dixon"},{"name":"School B","var(b)":"Andrea"}]}`,
		js)
}

func TestGroupByAgg(t *testing.T) {
	populateGraph(t)
	query := `
		{
			me(id: 1) {
				friend @groupby(age) {
					max(name)
				}
			}
		}
	`
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"friend":[{"@groupby":[{"age":17,"max(name)":"Daryl Dixon"},{"age":19,"max(name)":"Andrea"},{"age":15,"max(name)":"Rick Grimes"}]}]}]}`,
		js)
}

func TestGroupByMulti(t *testing.T) {
	populateGraph(t)
	query := `
		{
			me(id: 1) {
				friend @groupby(friend,name) {
					count(_uid_)
				}
			}
		}
	`
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"friend":[{"@groupby":[{"count":1,"friend":"0x1","name":"Rick Grimes"},{"count":1,"friend":"0x18","name":"Andrea"}]}]}]}`,
		js)
}

func TestMultiEmptyBlocks(t *testing.T) {
	populateGraph(t)
	query := `
		{
			you(id:0x01) {
			}

			me(id: 0x02) {
			}
		}
	`
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{}`,
		js)
}

func TestUseVarsMultiCascade1(t *testing.T) {
	populateGraph(t)
	query := `
		{
			him(id:0x01) {
				L as friend {
				 B as friend
					name
			 }
			}

			me(id: var(L, B)) {
				name
			}
		}
	`
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"him": [{"friend":[{"name":"Rick Grimes"}, {"name":"Andrea"}]}], "me":[{"name":"Michonne"},{"name":"Rick Grimes"},{"name":"Glenn Rhee"}, {"name":"Andrea"}]}`,
		js)
}

func TestUseVarsMultiCascade(t *testing.T) {
	populateGraph(t)
	query := `
		{
			var(id:0x01) {
				L as friend {
				 B as friend
				}
			}

			me(id: var(L, B)) {
				name
			}
		}
	`
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"name":"Michonne"},{"name":"Rick Grimes"},{"name":"Glenn Rhee"}, {"name":"Andrea"}]}`,
		js)
}

func TestUseVarsMultiOrder(t *testing.T) {
	populateGraph(t)
	query := `
		{
			var(id:0x01) {
				L as friend(first:2, orderasc: dob)
			}

			var(id:0x01) {
				G as friend(first:2, offset:2, orderasc: dob)
			}

			friend1(id: var(L)) {
				name
			}

			friend2(id: var(G)) {
				name
			}
		}
	`
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"friend1":[{"name":"Daryl Dixon"}, {"name":"Andrea"}],"friend2":[{"name":"Rick Grimes"},{"name":"Glenn Rhee"}]}`,
		js)
}

func TestFilterFacetVar(t *testing.T) {
	populateGraph(t)
	query := `
		{
			friend(id:0x01) {
				path @facets(L as weight) {
					name
				 	friend @filter(var(L)) {
						name
						var(L)
					}
				}
			}
		}
	`
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"friend":[{"path":[{"@facets":{"_":{"weight":0.200000}},"name":"Glenn Rhee"},{"@facets":{"_":{"weight":0.100000}},"friend":[{"name":"Glenn Rhee","var(L)":0.200000}],"name":"Andrea"}]}]}`,
		js)
}

func TestFilterFacetVar1(t *testing.T) {
	populateGraph(t)
	query := `
		{
			friend(id:0x01) {
				path @facets(L as weight1) {
					name
				 	friend @filter(var(L)){
						name
						var(L)
					}
				}
			}
		}
	`
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"friend":[{"path":[{"name":"Glenn Rhee"},{"@facets":{"_":{"weight1":0.200000}},"name":"Andrea"}]}]}`,
		js)
}

func TestUseVarsFilterVarReuse1(t *testing.T) {
	populateGraph(t)
	query := `
		{
			friend(id:0x01) {
				friend {
					L as friend {
						name
						friend @filter(var(L)) {
							name
						}
					}
				}
			}
		}
	`
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"friend":[{"friend":[{"friend":[{"name":"Michonne", "friend":[{"name":"Glenn Rhee"}]}]}, {"friend":[{"name":"Glenn Rhee"}]}]}]}`,
		js)
}

func TestUseVarsFilterVarReuse2(t *testing.T) {
	populateGraph(t)
	query := `
		{
			friend(func:anyofterms(name, "Michonne Andrea Glenn")) {
				friend {
				 L as friend {
					 name
					 friend @filter(var(L)) {
						name
					}
				}
			}
		}
	}
	`
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"friend":[{"friend":[{"friend":[{"name":"Michonne", "friend":[{"name":"Glenn Rhee"}]}]}, {"friend":[{"name":"Glenn Rhee"}]}]}]}`,
		js)
}

func TestVarInAggError(t *testing.T) {
	populateGraph(t)
	query := `
    {
			var(id: 1) {
				friend {
					a as age
				}
			}

			# var not allowed in min filter
			me(func: min(var(a))) {
				name
			}
		}
  `
	res, err := gql.Parse(gql.Request{Str: query})
	require.NoError(t, err)

	var l Latency
	ctx := context.Background()
	_, err = ProcessQuery(ctx, res, &l)
	require.Error(t, err)
}

func TestVarInIneqError(t *testing.T) {
	populateGraph(t)
	query := `
    {
			var(id: 1) {
				f as friend {
					a as age
				}
			}

			me(id: var(f)) @filter(gt(var(a), "alice")) {
				name
			}
		}
  `
	res, err := gql.Parse(gql.Request{Str: query})
	require.NoError(t, err)

	var l Latency
	ctx := context.Background()
	_, err = ProcessQuery(ctx, res, &l)
	require.Error(t, err)
}

func TestVarInIneqScore(t *testing.T) {
	populateGraph(t)
	query := `
    {
			var(id: 1) {
				friend {
					a as age
					s as count(friend)
					score as math(2*a + 3 * s + 1)
				}
			}

			me(func: ge(var(score), 35)) {
				name
				var(score)
				var(a)
				var(s)
			}
		}
  `
	js := processToFastJSON(t, query)
	require.JSONEq(t, `{"me":[{"name":"Daryl Dixon","var(a)":17,"var(s)":0,"var(score)":35.000000},{"name":"Andrea","var(a)":19,"var(s)":1,"var(score)":42.000000}]}`,
		js)
}

func TestVarInIneq(t *testing.T) {
	populateGraph(t)
	query := `
    {
			var(id: 1) {
				f as friend {
					a as age
				}
			}

			me(id: var(f)) @filter(gt(var(a), 18)) {
				name
			}
		}
  `
	js := processToFastJSON(t, query)
	require.JSONEq(t, `{"me":[{"name":"Andrea"}]}`, js)
}

func TestVarInIneq2(t *testing.T) {
	populateGraph(t)
	query := `
    {
			var(id: 1) {
				friend {
					a as age
				}
			}

			me(func: gt(var(a), 18)) {
				name
			}
		}
  `
	js := processToFastJSON(t, query)
	require.JSONEq(t, `{"me":[{"name":"Andrea"}]}`, js)
}

func TestNestedFuncRoot(t *testing.T) {
	populateGraph(t)
	posting.CommitLists(10, 1)
	time.Sleep(100 * time.Millisecond)
	query := `
    {
			me(func: gt(count(friend), 2)) {
				name
			}
		}
  `
	js := processToFastJSON(t, query)
	require.JSONEq(t, `{"me":[{"name":"Michonne"}]}`, js)
}

func TestNestedFuncRoot2(t *testing.T) {
	populateGraph(t)
	posting.CommitLists(10, 1)
	time.Sleep(100 * time.Millisecond)
	query := `
		{
			me(func: ge(count(friend), 1)) {
				name
			}
		}
  `
	js := processToFastJSON(t, query)
	require.JSONEq(t, `{"me":[{"name":"Michonne"},{"name":"Rick Grimes"},{"name":"Andrea"}]}`, js)
}

func TestRecurseQuery(t *testing.T) {
	populateGraph(t)
	query := `
		{
			recurse(id:0x01) {
				friend
				name
			}
		}`
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"recurse":[{"name":"Michonne", "friend":[{"name":"Rick Grimes", "friend":[{"name":"Michonne"}]},{"name":"Glenn Rhee"},{"name":"Daryl Dixon"},{"name":"Andrea", "friend":[{"name":"Glenn Rhee"}]}]}]}`, js)
}

func TestRecurseQueryLimitDepth(t *testing.T) {
	populateGraph(t)
	query := `
		{
			recurse(id:0x01, depth: 2) {
				friend
				name
			}
		}`
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"recurse":[{"name":"Michonne", "friend":[{"name":"Rick Grimes"},{"name":"Glenn Rhee"},{"name":"Daryl Dixon"},{"name":"Andrea"}]}]}`, js)
}

func TestShortestPath_ExpandError(t *testing.T) {
	populateGraph(t)
	query := `
		{
			A as shortest(from:0x01, to:101) {
				expand(_all_)
			}

			me(id: var( A)) {
				name
			}
		}`
	res, err := gql.Parse(gql.Request{Str: query})
	require.NoError(t, err)

	var l Latency
	ctx := context.Background()
	_, err = ProcessQuery(ctx, res, &l)
	require.Error(t, err)
}

func TestShortestPath_NoPath(t *testing.T) {
	populateGraph(t)
	query := `
		{
			A as shortest(from:0x01, to:101) {
				path
				follow
			}

			me(id: var( A)) {
				name
			}
		}`
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{}`,
		js)
}

func TestShortestPath(t *testing.T) {
	populateGraph(t)
	query := `
		{
			A as shortest(from:0x01, to:31) {
				friend
			}

			me(id: var( A)) {
				name
			}
		}`
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"_path_":[{"_uid_":"0x1","friend":[{"_uid_":"0x1f"}]}],"me":[{"name":"Michonne"},{"name":"Andrea"}]}`,
		js)
}

func TestShortestPathRev(t *testing.T) {
	populateGraph(t)
	query := `
		{
			A as shortest(from:23, to:1) {
				friend
			}

			me(id: var( A)) {
				name
			}
		}`
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"_path_":[{"_uid_":"0x17","friend":[{"_uid_":"0x1"}]}],"me":[{"name":"Rick Grimes"},{"name":"Michonne"}]}`,
		js)
}

func TestFacetVarRetrieval(t *testing.T) {
	populateGraph(t)
	query := `
		{
			var(id:1) {
				path @facets(f as weight)
			}

			me(id: 24) {
				var(f)
			}
		}`
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"var(f)":0.200000}]}`,
		js)
}

func TestFacetVarRetrieveOrder(t *testing.T) {
	populateGraph(t)
	query := `
		{
			var(id:1) {
				path @facets(f as weight)
			}

			me(id: var(f), orderasc: var(f)) {
				name
				var(f)
			}
		}`
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"name":"Andrea","var(f)":0.100000},{"name":"Glenn Rhee","var(f)":0.200000}]}`,
		js)
}

func TestShortestPathWeightsMultiFacet_Error(t *testing.T) {
	populateGraph(t)
	query := `
		{
			A as shortest(from:1, to:1002) {
				path @facets(weight, weight1)
			}

			me(id: var( A)) {
				name
			}
		}`
	res, err := gql.Parse(gql.Request{Str: query})
	require.NoError(t, err)

	var l Latency
	ctx := context.Background()
	_, err = ProcessQuery(ctx, res, &l)
	require.Error(t, err)
}

func TestShortestPathWeights(t *testing.T) {
	populateGraph(t)
	query := `
		{
			A as shortest(from:1, to:1002) {
				path @facets(weight)
			}

			me(id: var( A)) {
				name
			}
		}`
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"_path_":[{"_uid_":"0x1","path":[{"@facets":{"_":{"weight":0.100000}},"_uid_":"0x1f","path":[{"@facets":{"_":{"weight":0.100000}},"_uid_":"0x3e8","path":[{"@facets":{"_":{"weight":0.100000}},"_uid_":"0x3e9","path":[{"@facets":{"_":{"weight":0.100000}},"_uid_":"0x3ea"}]}]}]}]}],"me":[{"name":"Michonne"},{"name":"Andrea"},{"name":"Alice"},{"name":"Bob"},{"name":"Matt"}]}`,
		js)
}

func TestShortestPath2(t *testing.T) {
	populateGraph(t)
	query := `
		{
			A as shortest(from:0x01, to:1000) {
				path
			}

			me(id: var( A)) {
				name
			}
		}`
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"_path_":[{"_uid_":"0x1","path":[{"_uid_":"0x1f","path":[{"_uid_":"0x3e8"}]}]}],"me":[{"name":"Michonne"},{"name":"Andrea"},{"name":"Alice"}]}
`,
		js)
}

func TestShortestPath4(t *testing.T) {
	populateGraph(t)
	query := `
		{
			A as shortest(from:1, to:1003) {
				path
				follow
			}

			me(id: var( A)) {
				name
			}
		}`
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"_path_":[{"_uid_":"0x1","follow":[{"_uid_":"0x1f","follow":[{"_uid_":"0x3e9","follow":[{"_uid_":"0x3eb"}]}]}]}],"me":[{"name":"Michonne"},{"name":"Andrea"},{"name":"Bob"},{"name":"John"}]}`,
		js)
}

func TestShortestPath_filter(t *testing.T) {
	populateGraph(t)
	query := `
		{
			A as shortest(from:1, to:1002) {
				path @filter(not anyofterms(name, "alice"))
				follow
			}

			me(id: var( A)) {
				name
			}
		}`
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"_path_":[{"_uid_":"0x1","follow":[{"_uid_":"0x1f","follow":[{"_uid_":"0x3e9","path":[{"_uid_":"0x3ea"}]}]}]}],"me":[{"name":"Michonne"},{"name":"Andrea"},{"name":"Bob"},{"name":"Matt"}]}`,
		js)
}

func TestShortestPath_filter2(t *testing.T) {
	populateGraph(t)
	query := `
		{
			A as shortest(from:1, to:1002) {
				path @filter(not anyofterms(name, "alice"))
				follow @filter(not anyofterms(name, "bob"))
			}

			me(id: var(A)) {
				name
			}
		}`
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{}`,
		js)
}

func TestUseVarsFilterMultiId(t *testing.T) {
	populateGraph(t)
	query := `
		{
			var(id:0x01) {
				L as friend {
					friend
				}
			}

			var(id:31) {
				G as friend
			}

			friend(func:anyofterms(name, "Michonne Andrea Glenn")) @filter(var(G, L)) {
				name
			}
		}
	`
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"friend":[{"name":"Glenn Rhee"},{"name":"Andrea"}]}`,
		js)
}

func TestUseVarsMultiFilterId(t *testing.T) {
	populateGraph(t)
	query := `
		{
			var(id:0x01) {
				L as friend
			}

			var(id:31) {
				G as friend
			}

			friend(id: var(L)) @filter(var(G)) {
				name
			}
		}
	`
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"friend":[{"name":"Glenn Rhee"}]}`,
		js)
}

func TestUseVarsCascade(t *testing.T) {
	populateGraph(t)
	query := `
		{
			var(id:0x01) {
				L as friend {
				  friend
				}
			}

			me(id: var(L)) {
				name
			}
		}
	`
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"name":"Rick Grimes"}, {"name":"Andrea"} ]}`,
		js)
}

func TestUseVars(t *testing.T) {
	populateGraph(t)
	query := `
		{
			var(id:0x01) {
				L as friend
			}

			me(id: var(L)) {
				name
			}
		}
	`
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"name":"Rick Grimes"},{"name":"Glenn Rhee"},{"name":"Daryl Dixon"},{"name":"Andrea"}]}`,
		js)
}

func TestGetUIDCount(t *testing.T) {
	populateGraph(t)
	query := `
		{
			me(id:0x01) {
				name
				_uid_
				gender
				alive
				count(friend)
			}
		}
	`
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"_uid_":"0x1","alive":true,"count(friend)":5,"gender":"female","name":"Michonne"}]}`,
		js)
}

func TestDebug1(t *testing.T) {
	populateGraph(t)

	// Alright. Now we have everything set up. Let's create the query.
	query := `
		{
			me(id:0x01) {
				name
				gender
				alive
				count(friend)
			}
		}
	`
	res, err := gql.Parse(gql.Request{Str: query})
	require.NoError(t, err)

	var l Latency
	ctx := context.Background()
	ctx = context.WithValue(ctx, "debug", "true")
	sgl, err := ProcessQuery(ctx, res, &l)
	require.NoError(t, err)

	var buf bytes.Buffer
	require.NoError(t, ToJson(&l, sgl, &buf, nil, true))

	var mp map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(buf.Bytes()), &mp))

	resp := mp["me"]
	uid := resp.([]interface{})[0].(map[string]interface{})["_uid_"].(string)
	require.EqualValues(t, "0x1", uid)

	latency := mp["server_latency"]
	require.NotNil(t, latency)
	_, ok := latency.(map[string]interface{})
	require.True(t, ok)
}

func TestDebug2(t *testing.T) {
	populateGraph(t)

	query := `
		{
			me(id:0x01) {
				name
				gender
				alive
				count(friend)
			}
		}
	`

	js := processToFastJSON(t, query)
	var mp map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(js), &mp))

	resp := mp["me"]
	uid, ok := resp.([]interface{})[0].(map[string]interface{})["_uid_"].(string)
	require.False(t, ok, "No uid expected but got one %s", uid)
}

func TestDebug3(t *testing.T) {
	populateGraph(t)

	// Alright. Now we have everything set up. Let's create the query.
	query := `
		{
			me(id: [1, 24]) @filter(ge(dob, "1910-01-01")) {
				name
			}
		}
	`
	res, err := gql.Parse(gql.Request{Str: query})
	require.NoError(t, err)

	var l Latency
	ctx := context.Background()
	ctx = context.WithValue(ctx, "debug", "true")
	sgl, err := ProcessQuery(ctx, res, &l)
	require.NoError(t, err)

	var buf bytes.Buffer
	require.NoError(t, ToJson(&l, sgl, &buf, nil, true))

	var mp map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(buf.Bytes()), &mp))

	resp := mp["me"]
	require.EqualValues(t, 1, len(mp["me"].([]interface{})))
	uid := resp.([]interface{})[0].(map[string]interface{})["_uid_"].(string)
	require.EqualValues(t, "0x1", uid)

	latency := mp["server_latency"]
	require.NotNil(t, latency)
	_, ok := latency.(map[string]interface{})
	require.True(t, ok)
}

func TestCount(t *testing.T) {
	populateGraph(t)

	// Alright. Now we have everything set up. Let's create the query.
	query := `
		{
			me(id:0x01) {
				name
				gender
				alive
				count(friend)
			}
		}
	`

	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"alive":true,"count(friend)":5,"gender":"female","name":"Michonne"}]}`,
		js)
}
func TestCountAlias(t *testing.T) {
	populateGraph(t)

	// Alright. Now we have everything set up. Let's create the query.
	query := `
		{
			me(id:0x01) {
				name
				gender
				alive
				friendCount: count(friend)
			}
		}
	`

	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"alive":true,"friendCount":5,"gender":"female","name":"Michonne"}]}`,
		js)
}

func TestCountError1(t *testing.T) {
	// Alright. Now we have everything set up. Let's create the query.
	query := `
		{
			me(id: 0x01) {
				count(friend {
					name
				})
				name
				gender
				alive
			}
		}
	`
	_, err := gql.Parse(gql.Request{Str: query})
	require.Error(t, err)
}

func TestCountError2(t *testing.T) {
	// Alright. Now we have everything set up. Let's create the query.
	query := `
		{
			me(id: 0x01) {
				count(friend {
					c {
						friend
					}
				})
				name
				gender
				alive
			}
		}
	`
	_, err := gql.Parse(gql.Request{Str: query})
	require.Error(t, err)
}

func TestCountError3(t *testing.T) {
	// Alright. Now we have everything set up. Let's create the query.
	query := `
		{
			me(id: 0x01) {
				count(friend
				name
				gender
				alive
			}
		}
	`
	_, err := gql.Parse(gql.Request{Str: query})
	require.Error(t, err)
}

func TestMultiCountSort(t *testing.T) {
	populateGraph(t)
	// Alright. Now we have everything set up. Let's create the query.
	query := `
	{
		f as var(func: anyofterms(name, "michonne rick andrea")) {
		 	n as count(friend)
		}

		countorder(id: var(f), orderasc: var(n)) {
			name
			count(friend)
		}
	}
`
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"countorder":[{"count(friend)":0,"name":"Andrea With no friends"},{"count(friend)":1,"name":"Rick Grimes"},{"count(friend)":1,"name":"Andrea"},{"count(friend)":5,"name":"Michonne"}]}`,
		js)
}

func TestMultiLevelAgg(t *testing.T) {
	populateGraph(t)
	// Alright. Now we have everything set up. Let's create the query.
	query := `
	{
		sumorder(func: anyofterms(name, "michonne rick andrea")) {
			name
			friend {
				s as count(friend)
			}
			sum(var(s))
		}
	}
`
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"sumorder":[{"friend":[{"count(friend)":1},{"count(friend)":0},{"count(friend)":0},{"count(friend)":1},{"count(friend)":0}],"name":"Michonne","sum(var(s))":2},{"friend":[{"count(friend)":5}],"name":"Rick Grimes","sum(var(s))":5},{"friend":[{"count(friend)":0}],"name":"Andrea","sum(var(s))":0},{"name":"Andrea With no friends"}]}`,
		js)
}

func TestMultiLevelAgg1(t *testing.T) {
	populateGraph(t)
	// Alright. Now we have everything set up. Let's create the query.
	query := `
	{
		var(func: anyofterms(name, "michonne rick andrea")) @filter(gt(count(friend), 0)){
			friend {
				s as count(friend)
			}
			ss as sum(var(s))
		}

		sumorder(id: var(ss), orderasc: var(ss)) {
			name
			var(ss)
		}
	}
`
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"sumorder":[{"name":"Andrea","var(ss)":0},{"name":"Michonne","var(ss)":2},{"name":"Rick Grimes","var(ss)":5}]}`,
		js)
}

func TestMultiLevelAgg1Error(t *testing.T) {
	populateGraph(t)
	// Alright. Now we have everything set up. Let's create the query.
	query := `
	{
		var(func: anyofterms(name, "michonne rick andrea")) @filter(gt(count(friend), 0)){
			friend {
				s as count(friend)
				ss as sum(var(s))
			}
		}

		sumorder(id: var(ss), orderasc: var(ss)) {
			name
			var(ss)
		}
	}
`
	res, err := gql.Parse(gql.Request{Str: query})
	require.NoError(t, err)

	var l Latency
	_, queryErr := ProcessQuery(context.Background(), res, &l)
	require.Error(t, queryErr)

}

func TestMultiAggSort(t *testing.T) {
	populateGraph(t)
	// Alright. Now we have everything set up. Let's create the query.
	query := `
	{
		f as var(func: anyofterms(name, "michonne rick andrea")) {
			name
			friend {
				x as dob
			}
			mindob as min(var(x))
			maxdob as max(var(x))
		}

		maxorder(id: var(f), orderasc: var(maxdob)) {
			name
			var(maxdob)
		}

		minorder(id: var(f), orderasc: var(mindob)) {
			name
			var(mindob)
		}
	}
`
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"maxorder":[{"name":"Andrea","var(maxdob)":"1909-05-05"},{"name":"Rick Grimes","var(maxdob)":"1910-01-01"},{"name":"Michonne","var(maxdob)":"1910-01-02"}],"minorder":[{"name":"Michonne","var(mindob)":"1901-01-15"},{"name":"Andrea","var(mindob)":"1909-05-05"},{"name":"Rick Grimes","var(mindob)":"1910-01-01"}]}`,
		js)
}

func TestMinMulti(t *testing.T) {
	populateGraph(t)
	// Alright. Now we have everything set up. Let's create the query.
	query := `
	{
		me(func: anyofterms(name, "michonne rick andrea")) {
			name
			friend {
				x as dob
			}
			min(var(x))
			max(var(x))
		}
	}
`
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"friend":[{"dob":"1910-01-02"},{"dob":"1909-05-05"},{"dob":"1909-01-10"},{"dob":"1901-01-15"}],"max(var(x))":"1910-01-02","min(var(x))":"1901-01-15","name":"Michonne"},{"friend":[{"dob":"1910-01-01"}],"max(var(x))":"1910-01-01","min(var(x))":"1910-01-01","name":"Rick Grimes"},{"friend":[{"dob":"1909-05-05"}],"max(var(x))":"1909-05-05","min(var(x))":"1909-05-05","name":"Andrea"},{"name":"Andrea With no friends"}]}`,
		js)
}

func TestMinMultiAlias(t *testing.T) {
	populateGraph(t)
	// Alright. Now we have everything set up. Let's create the query.
	query := `
	{
		me(func: anyofterms(name, "michonne rick andrea")) {
			name
			friend {
				x as dob
			}
			mindob: min(var(x))
			maxdob: max(var(x))
		}
	}
`
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"friend":[{"dob":"1910-01-02"},{"dob":"1909-05-05"},{"dob":"1909-01-10"},{"dob":"1901-01-15"}],"maxdob":"1910-01-02","mindob":"1901-01-15","name":"Michonne"},{"friend":[{"dob":"1910-01-01"}],"maxdob":"1910-01-01","mindob":"1910-01-01","name":"Rick Grimes"},{"friend":[{"dob":"1909-05-05"}],"maxdob":"1909-05-05","mindob":"1909-05-05","name":"Andrea"},{"name":"Andrea With no friends"}]}`,
		js)
}

func TestMinMultiProto(t *testing.T) {
	populateGraph(t)
	// Alright. Now we have everything set up. Let's create the query.
	query := `
	{
		me(id: [1,23]) {
			name
			friend {
				name
				x as dob
			}
			min(var(x))
			max(var(x))
	}
}
`
	pb := processToPB(t, query, map[string]string{}, false)
	res := `attribute: "_root_"
children: <
  attribute: "me"
  properties: <
    prop: "name"
    value: <
      str_val: "Michonne"
    >
  >
  properties: <
    prop: "min(var(x))"
    value: <
      str_val: "1901-01-15T00:00:00Z"
    >
  >
  properties: <
    prop: "max(var(x))"
    value: <
      str_val: "1910-01-02T00:00:00Z"
    >
  >
  children: <
    attribute: "friend"
    properties: <
      prop: "name"
      value: <
        str_val: "Rick Grimes"
      >
    >
    properties: <
      prop: "dob"
      value: <
        str_val: "1910-01-02T00:00:00Z"
      >
    >
  >
  children: <
    attribute: "friend"
    properties: <
      prop: "name"
      value: <
        str_val: "Glenn Rhee"
      >
    >
    properties: <
      prop: "dob"
      value: <
        str_val: "1909-05-05T00:00:00Z"
      >
    >
  >
  children: <
    attribute: "friend"
    properties: <
      prop: "name"
      value: <
        str_val: "Daryl Dixon"
      >
    >
    properties: <
      prop: "dob"
      value: <
        str_val: "1909-01-10T00:00:00Z"
      >
    >
  >
  children: <
    attribute: "friend"
    properties: <
      prop: "name"
      value: <
        str_val: "Andrea"
      >
    >
    properties: <
      prop: "dob"
      value: <
        str_val: "1901-01-15T00:00:00Z"
      >
    >
  >
>
children: <
  attribute: "me"
  properties: <
    prop: "name"
    value: <
      str_val: "Rick Grimes"
    >
  >
  properties: <
    prop: "min(var(x))"
    value: <
      str_val: "1910-01-01T00:00:00Z"
    >
  >
  properties: <
    prop: "max(var(x))"
    value: <
      str_val: "1910-01-01T00:00:00Z"
    >
  >
  children: <
    attribute: "friend"
    properties: <
      prop: "name"
      value: <
        str_val: "Michonne"
      >
    >
    properties: <
      prop: "dob"
      value: <
        str_val: "1910-01-01T00:00:00Z"
      >
    >
  >
>
`
	require.Equal(t, res, proto.MarshalTextString(pb[0]))
}

func TestMinSchema(t *testing.T) {
	populateGraph(t)
	// Alright. Now we have everything set up. Let's create the query.
	query := `
                {
                        me(id:0x01) {
                                name
                                gender
                                alive
                                friend {
																	x as survival_rate
                                }
																min(var(x))
                        }
                }
        `
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"alive":true,"friend":[{"survival_rate":1.600000},{"survival_rate":1.600000},{"survival_rate":1.600000},{"survival_rate":1.600000}],"gender":"female","min(var(x))":1.600000,"name":"Michonne"}]}`,
		js)

	schema.State().Set("survival_rate", protos.SchemaUpdate{ValueType: uint32(types.IntID)})
	js = processToFastJSON(t, query)
	require.EqualValues(t,
		`{"me":[{"alive":true,"friend":[{"survival_rate":1},{"survival_rate":1},{"survival_rate":1},{"survival_rate":1}],"gender":"female","min(var(x))":1,"name":"Michonne"}]}`,
		js)
	schema.State().Set("survival_rate", protos.SchemaUpdate{ValueType: uint32(types.FloatID)})
}

func TestAvg(t *testing.T) {
	populateGraph(t)
	query := `
	{
		me(id:0x01) {
			name
			gender
			alive
			friend {
				x as shadow_deep
			}
			avg(var(x))
		}
	}
`
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"alive":true,"avg(var(x))":9.000000,"friend":[{"shadow_deep":4},{"shadow_deep":14}],"gender":"female","name":"Michonne"}]}`,
		js)
}

func TestSum(t *testing.T) {
	populateGraph(t)
	query := `
                {
                        me(id:0x01) {
                                name
                                gender
                                alive
                                friend {
                                    x as shadow_deep
                                }
																sum(var(x))
                        }
                }
        `
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"alive":true,"friend":[{"shadow_deep":4},{"shadow_deep":14}],"gender":"female","name":"Michonne","sum(var(x))":18}]}`,
		js)
}

func TestQueryPassword(t *testing.T) {
	populateGraph(t)
	// Password is not fetchable
	query := `
                {
                        me(id:0x01) {
                                name
                                password
                        }
                }
	`
	res, err := gql.Parse(gql.Request{Str: query})
	require.NoError(t, err)

	var l Latency
	_, queryErr := ProcessQuery(context.Background(), res, &l)
	require.NotNil(t, queryErr)
}

func TestCheckPassword(t *testing.T) {
	populateGraph(t)
	query := `
                {
                        me(id:0x01) {
                                name
                                checkpwd(password, "123456")
                        }
                }
	`
	js := processToFastJSON(t, query)
	require.EqualValues(t,
		`{"me":[{"name":"Michonne","password":[{"checkpwd":true}]}]}`,
		js)
}

func TestCheckPasswordIncorrect(t *testing.T) {
	populateGraph(t)
	query := `
                {
                        me(id:0x01) {
                                name
                                checkpwd(password, "654123")
                        }
                }
	`
	js := processToFastJSON(t, query)
	require.EqualValues(t,
		`{"me":[{"name":"Michonne","password":[{"checkpwd":false}]}]}`,
		js)
}

// ensure, that old and deprecated form is not allowed
func TestCheckPasswordParseError(t *testing.T) {
	populateGraph(t)
	query := `
                {
                        me(id:0x01) {
                                name
                                checkpwd("654123")
                        }
                }
	`
	_, err := processToFastJsonReq(t, query)
	require.Error(t, err)
}

func TestCheckPasswordDifferentAttr1(t *testing.T) {
	populateGraph(t)
	query := `
                {
                        me(id:23) {
                                name
                                checkpwd(pass, "654321")
                        }
                }
	`
	js := processToFastJSON(t, query)
	require.EqualValues(t, `{"me":[{"name":"Rick Grimes","pass":[{"checkpwd":true}]}]}`, js)
}

func TestCheckPasswordDifferentAttr2(t *testing.T) {
	populateGraph(t)
	query := `
                {
                        me(id:23) {
                                name
                                checkpwd(pass, "invalid")
                        }
                }
	`
	js := processToFastJSON(t, query)
	require.EqualValues(t, `{"me":[{"name":"Rick Grimes","pass":[{"checkpwd":false}]}]}`, js)
}

func TestCheckPasswordInvalidAttr(t *testing.T) {
	populateGraph(t)
	query := `
                {
                        me(id:0x1) {
                                name
                                checkpwd(pass, "123456")
                        }
                }
	`
	js := processToFastJSON(t, query)
	// for id:0x1 there is no pass attribute defined (there's only password attribute)
	require.EqualValues(t, `{"me":[{"name":"Michonne","pass":[{"checkpwd":false}]}]}`, js)
}

// test for old version of checkpwd with hardcoded attribute name
func TestCheckPasswordQuery1(t *testing.T) {
	populateGraph(t)
	query := `
                {
                        me(id:0x1) {
                                name
                                password
                        }
                }
	`
	_, err := processToFastJsonReq(t, query)
	require.Error(t, err)
	require.EqualValues(t, "Attribute `password` of type password cannot be fetched", err.Error())
}

// test for improved version of checkpwd with custom attribute name
func TestCheckPasswordQuery2(t *testing.T) {
	populateGraph(t)
	query := `
                {
                        me(id:23) {
                                name
                                pass
                        }
                }
	`
	_, err := processToFastJsonReq(t, query)
	require.Error(t, err)
	require.EqualValues(t, "Attribute `pass` of type password cannot be fetched", err.Error())
}

func TestToSubgraphInvalidFnName(t *testing.T) {
	query := `
                {
                        me(func:invalidfn1(name, "some cool name")) {
                                name
                                gender
                                alive
                        }
                }
        `
	res, err := gql.Parse(gql.Request{Str: query})
	require.NoError(t, err)

	ctx := context.Background()
	_, err = ToSubGraph(ctx, res.Query[0])
	require.Error(t, err)
}

func TestToSubgraphInvalidFnName2(t *testing.T) {
	query := `
                {
                        me(func:anyofterms(name, "some cool name")) {
                                name
                                friend @filter(invalidfn2(name, "some name")) {
                                       name
                                }
                        }
                }
        `
	res, err := gql.Parse(gql.Request{Str: query})
	require.NoError(t, err)

	ctx := context.Background()
	_, err = ToSubGraph(ctx, res.Query[0])
	require.Error(t, err)
}

func TestToSubgraphInvalidFnName3(t *testing.T) {
	query := `
                {
                        me(func:anyofterms(name, "some cool name")) {
                                name
                                friend @filter(anyofterms(name, "Andrea") or
                                               invalidfn3(name, "Andrea Rhee")){
                                        name
                                }
                        }
                }
        `
	res, err := gql.Parse(gql.Request{Str: query})
	require.NoError(t, err)

	ctx := context.Background()
	_, err = ToSubGraph(ctx, res.Query[0])
	require.Error(t, err)
}

func TestToSubgraphInvalidFnName4(t *testing.T) {
	query := `
                {
                        f as var(func:invalidfn4(name, "Michonne Rick Glenn")) {
                                name
                        }
                        you(func:anyofterms(name, "Michonne")) {
                                friend @filter(var(f)) {
                                        name
                                }
                        }
                }
        `
	res, err := gql.Parse(gql.Request{Str: query})
	require.NoError(t, err)

	ctx := context.Background()
	_, err = ToSubGraph(ctx, res.Query[0])
	require.Error(t, err)
}

func TestToSubgraphInvalidArgs1(t *testing.T) {
	query := `
                {
                        me(id:0x01) {
                                name
                                gender
                                friend(disorderasc: dob) @filter(le(dob, "1909-03-20")) {
                                        name
                                }
                        }
                }
        `
	res, err := gql.Parse(gql.Request{Str: query})
	require.NoError(t, err)

	ctx := context.Background()
	_, err = ToSubGraph(ctx, res.Query[0])
	require.Error(t, err)
}

func TestToSubgraphInvalidArgs2(t *testing.T) {
	query := `
                {
                        me(id:0x01) {
                                name
                                gender
                                friend(offset:1, invalidorderasc:1) @filter(anyofterms(name, "Andrea")) {
                                        name
                                }
                        }
                }
        `
	res, err := gql.Parse(gql.Request{Str: query})
	require.NoError(t, err)

	ctx := context.Background()
	_, err = ToSubGraph(ctx, res.Query[0])
	require.Error(t, err)
}

func TestProcessGraph(t *testing.T) {
	populateGraph(t)
	// Alright. Now we have everything set up. Let's create the query.
	query := `
		{
			me(id: 0x01) {
				friend {
					name
				}
				name
				gender
				alive
			}
		}
	`
	res, err := gql.Parse(gql.Request{Str: query})
	require.NoError(t, err)

	ctx := context.Background()
	sg, err := ToSubGraph(ctx, res.Query[0])
	require.NoError(t, err)

	ch := make(chan error)
	go ProcessGraph(ctx, sg, nil, ch)
	err = <-ch
	require.NoError(t, err)

	require.EqualValues(t, childAttrs(sg), []string{"friend", "name", "gender", "alive"})
	require.EqualValues(t, childAttrs(sg.Children[0]), []string{"name"})

	child := sg.Children[0]
	require.EqualValues(t,
		[][]uint64{
			{23, 24, 25, 31, 101},
		}, algo.ToUintsListForTest(child.uidMatrix))

	require.EqualValues(t, []string{"name"}, childAttrs(child))

	child = child.Children[0]
	require.EqualValues(t,
		[]string{"Rick Grimes", "Glenn Rhee", "Daryl Dixon", "Andrea", ""},
		taskValues(t, child.values))

	require.EqualValues(t, []string{"Michonne"},
		taskValues(t, sg.Children[1].values))
	require.EqualValues(t, []string{"female"},
		taskValues(t, sg.Children[2].values))
}

func TestToFastJSON(t *testing.T) {
	populateGraph(t)
	// Alright. Now we have everything set up. Let's create the query.
	query := `
		{
			me(id:0x01) {
				name
				gender
				alive
				friend {
					name
				}
			}
		}
	`

	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"alive":true,"friend":[{"name":"Rick Grimes"},{"name":"Glenn Rhee"},{"name":"Daryl Dixon"},{"name":"Andrea"}],"gender":"female","name":"Michonne"}]}`,
		js)
}

func TestFieldAlias(t *testing.T) {
	populateGraph(t)

	// Alright. Now we have everything set up. Let's create the query.
	query := `
		{
			me(id:0x01) {
				MyName:name
				gender
				alive
				Buddies:friend {
					BudName:name
				}
			}
		}
	`

	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"alive":true,"Buddies":[{"BudName":"Rick Grimes"},{"BudName":"Glenn Rhee"},{"BudName":"Daryl Dixon"},{"BudName":"Andrea"}],"gender":"female","MyName":"Michonne"}]}`,
		string(js))
}

func TestFieldAliasProto(t *testing.T) {
	populateGraph(t)

	// Alright. Now we have everything set up. Let's create the query.
	query := `
		{
			me(id:0x01) {
				MyName:name
				gender
				alive
				Buddies:friend {
					BudName:name
				}
			}
		}
	`
	pb := processToPB(t, query, map[string]string{}, false)
	expectedPb := `attribute: "_root_"
children: <
  attribute: "me"
  properties: <
    prop: "MyName"
    value: <
      str_val: "Michonne"
    >
  >
  properties: <
    prop: "gender"
    value: <
      default_val: "female"
    >
  >
  properties: <
    prop: "alive"
    value: <
      bool_val: true
    >
  >
  children: <
    attribute: "Buddies"
    properties: <
      prop: "BudName"
      value: <
        str_val: "Rick Grimes"
      >
    >
  >
  children: <
    attribute: "Buddies"
    properties: <
      prop: "BudName"
      value: <
        str_val: "Glenn Rhee"
      >
    >
  >
  children: <
    attribute: "Buddies"
    properties: <
      prop: "BudName"
      value: <
        str_val: "Daryl Dixon"
      >
    >
  >
  children: <
    attribute: "Buddies"
    properties: <
      prop: "BudName"
      value: <
        str_val: "Andrea"
      >
    >
  >
>
`
	require.EqualValues(t,
		expectedPb,
		proto.MarshalTextString(pb[0]))
}

func TestToFastJSONFilter(t *testing.T) {
	populateGraph(t)
	query := `
		{
			me(id:0x01) {
				name
				gender
				friend @filter(anyofterms(name, "Andrea SomethingElse")) {
					name
				}
			}
		}
	`

	js := processToFastJSON(t, query)
	require.EqualValues(t,
		`{"me":[{"friend":[{"name":"Andrea"}],"gender":"female","name":"Michonne"}]}`,
		js)
}

func TestToFastJSONFilterMissBrac(t *testing.T) {
	populateGraph(t)
	query := `
		{
			me(id:0x01) {
				name
				gender
				friend @filter(anyofterms(name, "Andrea SomethingElse") {
					name
				}
			}
		}
	`
	_, err := gql.Parse(gql.Request{Str: query})
	require.Error(t, err)
}

func TestToFastJSONFilterallofterms(t *testing.T) {
	populateGraph(t)
	query := `
		{
			me(id:0x01) {
				name
				gender
				friend @filter(allofterms(name, "Andrea SomethingElse")) {
					name
				}
			}
		}
	`

	js := processToFastJSON(t, query)
	require.EqualValues(t,
		`{"me":[{"gender":"female","name":"Michonne"}]}`, js)
}

func TestInvalidStringIndex(t *testing.T) {
	// no FTS index defined for name
	populateGraph(t)
	query := `
		{
			me(id:0x01) {
				name
				gender
				friend @filter(alloftext(name, "Andrea SomethingElse")) {
					name
				}
			}
		}
	`

	_, err := processToFastJsonReq(t, query)
	require.Error(t, err)
}

func TestValidFulltextIndex(t *testing.T) {
	// no FTS index defined for name
	populateGraph(t)
	query := `
		{
			me(id:0x01) {
				name
				friend @filter(alloftext(alias, "BOB")) {
					alias
				}
			}
		}
	`

	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"name":"Michonne", "friend":[{"alias":"Bob Joe"}]}]}`, js)
}

// dob (date of birth) is not a string
func TestFilterRegexError(t *testing.T) {
	populateGraph(t)
	query := `
    {
      me(id:0x01) {
        name
        friend @filter(regexp(dob, /^[a-z A-Z]+$/)) {
          name
        }
      }
    }
`

	res, err := gql.Parse(gql.Request{Str: query})
	require.NoError(t, err)

	ctx := context.Background()
	sg, err := ToSubGraph(ctx, res.Query[0])
	require.NoError(t, err)

	ch := make(chan error)
	go ProcessGraph(ctx, sg, nil, ch)
	err = <-ch
	require.Error(t, err)
}

func TestFilterRegex1(t *testing.T) {
	populateGraph(t)
	query := `
    {
      me(id:0x01) {
        name
        friend @filter(regexp(name, /^[a-z A-Z]+$/)) {
          name
        }
      }
    }
`

	_, err := processToFastJsonReq(t, query)
	require.Error(t, err)
}

func TestFilterRegex2(t *testing.T) {
	populateGraph(t)
	query := `
    {
      me(id:0x01) {
        name
        friend @filter(regexp(name, /^[^ao]+$/)) {
          name
        }
      }
    }
`

	_, err := processToFastJsonReq(t, query)
	require.Error(t, err)
}

func TestFilterRegex3(t *testing.T) {
	populateGraph(t)
	query := `
    {
      me(id:0x01) {
        name
        friend @filter(regexp(name, /^Rick/)) {
          name
        }
      }
    }
`

	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"name":"Michonne", "friend":[{"name":"Rick Grimes"}]}]}`, js)
}

func TestFilterRegex4(t *testing.T) {
	populateGraph(t)
	query := `
    {
      me(id:0x01) {
        name
        friend @filter(regexp(name, /((en)|(xo))n/)) {
          name
        }
      }
    }
`

	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"name":"Michonne", "friend":[{"name":"Glenn Rhee"},{"name":"Daryl Dixon"} ]}]}`, js)
}

func TestFilterRegex5(t *testing.T) {
	populateGraph(t)
	query := `
    {
      me(id:0x01) {
        name
        friend @filter(regexp(name, /^[a-zA-z]*[^Kk ]?[Nn]ight/)) {
          name
        }
      }
    }
`

	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"name":"Michonne"}]}`, js)
}

func TestFilterRegex6(t *testing.T) {
	populateGraph(t)
	query := `
    {
	  me(id:0x1234) {
		pattern @filter(regexp(value, /miss((issippi)|(ouri))/)) {
			value
		}
      }
    }
`

	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"pattern":[{"value":"mississippi"}, {"value":"missouri"}]}]}`, js)
}

func TestFilterRegex7(t *testing.T) {
	populateGraph(t)
	query := `
    {
	  me(id:0x1234) {
		pattern @filter(regexp(value, /[aeiou]mission/)) {
			value
		}
      }
    }
`

	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"pattern":[{"value":"omission"}, {"value":"dimission"}]}]}`, js)
}

func TestFilterRegex8(t *testing.T) {
	populateGraph(t)
	query := `
    {
	  me(id:0x1234) {
		pattern @filter(regexp(value, /^(trans)?mission/)) {
			value
		}
      }
    }
`

	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"pattern":[{"value":"mission"}, {"value":"missionary"}, {"value":"transmission"}]}]}`, js)
}

func TestFilterRegex9(t *testing.T) {
	populateGraph(t)
	query := `
    {
	  me(id:0x1234) {
		pattern @filter(regexp(value, /s.{2,5}mission/)) {
			value
		}
      }
    }
`

	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"pattern":[{"value":"submission"}, {"value":"subcommission"}, {"value":"discommission"}]}]}`, js)
}

func TestFilterRegex10(t *testing.T) {
	populateGraph(t)
	query := `
    {
	  me(id:0x1234) {
		pattern @filter(regexp(value, /[^m]iss/)) {
			value
		}
      }
    }
`

	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"pattern":[{"value":"mississippi"}, {"value":"whissle"}]}]}`, js)
}

func TestFilterRegex11(t *testing.T) {
	populateGraph(t)
	query := `
    {
	  me(id:0x1234) {
		pattern @filter(regexp(value, /SUB[cm]/i)) {
			value
		}
      }
    }
`

	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"pattern":[{"value":"submission"}, {"value":"subcommission"}]}]}`, js)
}

// case insensitive mode may be turned on with modifier:
// http://www.regular-expressions.info/modifiers.html - this is completely legal
func TestFilterRegex12(t *testing.T) {
	populateGraph(t)
	query := `
    {
	  me(id:0x1234) {
		pattern @filter(regexp(value, /(?i)SUB[cm]/)) {
			value
		}
      }
    }
`

	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"pattern":[{"value":"submission"}, {"value":"subcommission"}]}]}`, js)
}

// case insensitive mode may be turned on and off with modifier:
// http://www.regular-expressions.info/modifiers.html - this is completely legal
func TestFilterRegex13(t *testing.T) {
	populateGraph(t)
	query := `
    {
	  me(id:0x1234) {
		pattern @filter(regexp(value, /(?i)SUB[cm](?-i)ISSION/)) {
			value
		}
      }
    }
`

	// no results are returned, becaues case insensive mode is turned off before 'ISSION'
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{}`, js)
}

// invalid regexp modifier
func TestFilterRegex14(t *testing.T) {
	populateGraph(t)
	query := `
    {
	  me(id:0x1234) {
		pattern @filter(regexp(value, /pattern/x)) {
			value
		}
      }
    }
`

	_, err := processToFastJsonReq(t, query)
	require.Error(t, err)
}

// multi-lang - simple
func TestFilterRegex15(t *testing.T) {
	populateGraph(t)
	query := `
		{
			me(func:regexp(name@ru, /Барсук/)) {
				name@ru
			}
		}
	`
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"name@ru":"Барсук"}]}`,
		js)
}

// multi-lang - test for bug (#945) - multi-byte runes
func TestFilterRegex16(t *testing.T) {
	populateGraph(t)
	query := `
		{
			me(func:regexp(name@ru, /^артём/i)) {
				name@ru
			}
		}
	`
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"name@ru":"Артём Ткаченко"}]}`,
		js)
}

func TestToFastJSONFilterUID(t *testing.T) {
	populateGraph(t)
	query := `
		{
			me(id:0x01) {
				name
				gender
				friend @filter(anyofterms(name, "Andrea")) {
					_uid_
				}
			}
		}
	`

	js := processToFastJSON(t, query)
	require.EqualValues(t,
		`{"me":[{"friend":[{"_uid_":"0x1f"}],"gender":"female","name":"Michonne"}]}`,
		js)
}

func TestToFastJSONFilterOrUID(t *testing.T) {
	populateGraph(t)
	query := `
		{
			me(id:0x01) {
				name
				gender
				friend @filter(anyofterms(name, "Andrea") or anyofterms(name, "Andrea Rhee")) {
					_uid_
					name
				}
			}
		}
	`

	js := processToFastJSON(t, query)
	require.EqualValues(t,
		`{"me":[{"friend":[{"_uid_":"0x18","name":"Glenn Rhee"},{"_uid_":"0x1f","name":"Andrea"}],"gender":"female","name":"Michonne"}]}`,
		js)
}

func TestToFastJSONFilterOrCount(t *testing.T) {
	populateGraph(t)
	query := `
		{
			me(id:0x01) {
				name
				gender
				count(friend @filter(anyofterms(name, "Andrea") or anyofterms(name, "Andrea Rhee")))
				friend @filter(anyofterms(name, "Andrea")) {
					name
				}
			}
		}
	`

	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"count(friend)":2,"friend": [{"name":"Andrea"}],"gender":"female","name":"Michonne"}]}`,
		js)
}

func TestToFastJSONFilterOrFirst(t *testing.T) {
	populateGraph(t)
	query := `
		{
			me(id:0x01) {
				name
				gender
				friend(first:2) @filter(anyofterms(name, "Andrea") or anyofterms(name, "Glenn SomethingElse") or anyofterms(name, "Daryl")) {
					name
				}
			}
		}
	`

	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"friend":[{"name":"Glenn Rhee"},{"name":"Daryl Dixon"}],"gender":"female","name":"Michonne"}]}`,
		js)
}

func TestToFastJSONFilterOrOffset(t *testing.T) {
	populateGraph(t)
	query := `
		{
			me(id:0x01) {
				name
				gender
				friend(offset:1) @filter(anyofterms(name, "Andrea") or anyofterms(name, "Glenn Rhee") or anyofterms(name, "Daryl Dixon")) {
					name
				}
			}
		}
	`

	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"friend":[{"name":"Daryl Dixon"},{"name":"Andrea"}],"gender":"female","name":"Michonne"}]}`,
		js)
}

func TestToFastJSONFiltergeName(t *testing.T) {
	populateGraph(t)
	query := `
		{
			me(id:0x01) {
				friend @filter(ge(name, "Rick")) {
					name
				}
			}
		}
	`

	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"friend":[{"name":"Rick Grimes"}]}]}`,
		js)
}

func TestToFastJSONFilteLtAlias(t *testing.T) {
	populateGraph(t)
	// We shouldn't get Zambo Alice.
	query := `
		{
			me(id:0x01) {
				friend(orderasc: alias) @filter(lt(alias, "Pat")) {
					alias
				}
			}
		}
	`

	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"friend":[{"alias":"Allan Matt"},{"alias":"Bob Joe"},{"alias":"John Alice"},{"alias":"John Oliver"}]}]}`,
		js)
}

func TestToFastJSONFilterge(t *testing.T) {
	populateGraph(t)
	query := `
		{
			me(id:0x01) {
				name
				gender
				friend @filter(ge(dob, "1909-05-05")) {
					name
				}
			}
		}
	`

	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"friend":[{"name":"Rick Grimes"},{"name":"Glenn Rhee"}],"gender":"female","name":"Michonne"}]}`,
		js)
}

func TestToFastJSONFilterGt(t *testing.T) {
	populateGraph(t)
	query := `
		{
			me(id:0x01) {
				name
				gender
				friend @filter(gt(dob, "1909-05-05")) {
					name
				}
			}
		}
	`

	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"friend":[{"name":"Rick Grimes"}],"gender":"female","name":"Michonne"}]}`,
		js)
}

func TestToFastJSONFilterle(t *testing.T) {
	populateGraph(t)
	query := `
		{
			me(id:0x01) {
				name
				gender
				friend @filter(le(dob, "1909-01-10")) {
					name
				}
			}
		}
	`

	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"friend":[{"name":"Daryl Dixon"},{"name":"Andrea"}],"gender":"female","name":"Michonne"}]}`,
		js)
}

func TestToFastJSONFilterLt(t *testing.T) {
	populateGraph(t)
	query := `
		{
			me(id:0x01) {
				name
				gender
				friend @filter(lt(dob, "1909-01-10")) {
					name
				}
			}
		}
	`

	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"friend":[{"name":"Andrea"}],"gender":"female","name":"Michonne"}]}`,
		js)
}

func TestToFastJSONFilterEqualNoHit(t *testing.T) {
	populateGraph(t)
	query := `
		{
			me(id:0x01) {
				name
				gender
				friend @filter(eq(dob, "1909-03-20")) {
					name
				}
			}
		}
	`

	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"gender":"female","name":"Michonne"}]}`,
		js)
}
func TestToFastJSONFilterEqualName(t *testing.T) {
	populateGraph(t)
	query := `
		{
			me(id:0x01) {
				name
				gender
				friend @filter(eq(name, "Daryl Dixon")) {
					name
				}
			}
		}
	`

	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"friend":[{"name":"Daryl Dixon"}], "gender":"female","name":"Michonne"}]}`,
		js)
}

func TestToFastJSONFilterEqualNameNoHit(t *testing.T) {
	populateGraph(t)
	query := `
		{
			me(id:0x01) {
				name
				gender
				friend @filter(eq(name, "Daryl")) {
					name
				}
			}
		}
	`

	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"gender":"female","name":"Michonne"}]}`,
		js)
}

func TestToFastJSONFilterEqual(t *testing.T) {
	populateGraph(t)
	query := `
		{
			me(id:0x01) {
				name
				gender
				friend @filter(eq(dob, "1909-01-10")) {
					name
				}
			}
		}
	`

	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"friend":[{"name":"Daryl Dixon"}], "gender":"female","name":"Michonne"}]}`,
		js)
}

func TestToFastJSONOrderName(t *testing.T) {
	populateGraph(t)
	query := `
		{
			me(id:0x01) {
				name
				friend(orderasc: alias) {
					alias
				}
			}
		}
	`

	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"friend":[{"alias":"Allan Matt"},{"alias":"Bob Joe"},{"alias":"John Alice"},{"alias":"John Oliver"},{"alias":"Zambo Alice"}],"name":"Michonne"}]}`,
		js)
}

func TestToFastJSONOrderNameDesc(t *testing.T) {
	populateGraph(t)
	query := `
		{
			me(id:0x01) {
				name
				friend(orderdesc: alias) {
					alias
				}
			}
		}
	`

	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"friend":[{"alias":"Zambo Alice"},{"alias":"John Oliver"},{"alias":"John Alice"},{"alias":"Bob Joe"},{"alias":"Allan Matt"}],"name":"Michonne"}]}`,
		js)
}

func TestToFastJSONOrderName1(t *testing.T) {
	populateGraph(t)
	query := `
		{
			me(id:0x01) {
				name
				friend(orderasc: name ) {
					name
				}
			}
		}
	`

	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"friend":[{"name":"Andrea"},{"name":"Daryl Dixon"},{"name":"Glenn Rhee"},{"name":"Rick Grimes"}],"name":"Michonne"}]}`,
		js)
}

func TestToFastJSONOrderNameError(t *testing.T) {
	populateGraph(t)
	query := `
		{
			me(id:0x01) {
				name
				friend(orderasc: nonexistent) {
					name
				}
			}
		}
	`
	res, err := gql.Parse(gql.Request{Str: query})
	require.NoError(t, err)

	ctx := context.Background()
	sg, err := ToSubGraph(ctx, res.Query[0])
	require.NoError(t, err)

	ch := make(chan error)
	go ProcessGraph(ctx, sg, nil, ch)
	err = <-ch
	require.Error(t, err)
}

func TestToFastJSONFilterleOrder(t *testing.T) {
	populateGraph(t)
	query := `
		{
			me(id:0x01) {
				name
				gender
				friend(orderasc: dob) @filter(le(dob, "1909-03-20")) {
					name
				}
			}
		}
	`

	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"friend":[{"name":"Andrea"},{"name":"Daryl Dixon"}],"gender":"female","name":"Michonne"}]}`,
		js)
}

func TestToFastJSONFiltergeNoResult(t *testing.T) {
	populateGraph(t)
	query := `
		{
			me(id:0x01) {
				name
				gender
				friend @filter(ge(dob, "1999-03-20")) {
					name
				}
			}
		}
	`

	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"gender":"female","name":"Michonne"}]}`, js)
}

func TestToFastJSONFirstOffsetOutOfBound(t *testing.T) {
	populateGraph(t)
	query := `
		{
			me(id:0x01) {
				name
				gender
				friend(offset:100, first:1) {
					name
				}
			}
		}
	`

	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"gender":"female","name":"Michonne"}]}`,
		js)
}

// No filter. Just to test first and offset.
func TestToFastJSONFirstOffset(t *testing.T) {
	populateGraph(t)
	query := `
		{
			me(id:0x01) {
				name
				gender
				friend(offset:1, first:1) {
					name
				}
			}
		}
	`

	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"friend":[{"name":"Glenn Rhee"}],"gender":"female","name":"Michonne"}]}`,
		js)
}

func TestToFastJSONFilterOrFirstOffset(t *testing.T) {
	populateGraph(t)
	query := `
		{
			me(id:0x01) {
				name
				gender
				friend(offset:1, first:1) @filter(anyofterms(name, "Andrea") or anyofterms(name, "SomethingElse Rhee") or anyofterms(name, "Daryl Dixon")) {
					name
				}
			}
		}
	`

	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"friend":[{"name":"Daryl Dixon"}],"gender":"female","name":"Michonne"}]}`,
		js)
}

func TestToFastJSONFilterleFirstOffset(t *testing.T) {
	populateGraph(t)
	query := `
		{
			me(id:0x01) {
				name
				gender
				friend(offset:1, first:1) @filter(le(dob, "1909-03-20")) {
					name
				}
			}
		}
	`

	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"friend":[{"name":"Andrea"}],"gender":"female","name":"Michonne"}]}`,
		js)
}

func TestToFastJSONFilterOrFirstOffsetCount(t *testing.T) {
	populateGraph(t)
	query := `
		{
			me(id:0x01) {
				name
				gender
				count(friend(offset:1, first:1) @filter(anyofterms(name, "Andrea") or anyofterms(name, "SomethingElse Rhee") or anyofterms(name, "Daryl Dixon")))
			}
		}
	`

	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"count(friend)":1,"gender":"female","name":"Michonne"}]}`,
		js)
}

func TestToFastJSONFilterOrFirstNegative(t *testing.T) {
	populateGraph(t)
	// When negative first/count is specified, we ignore offset and returns the last
	// few number of items.
	query := `
		{
			me(id:0x01) {
				name
				gender
				friend(first:-1, offset:0) @filter(anyofterms(name, "Andrea") or anyofterms(name, "Glenn Rhee") or anyofterms(name, "Daryl Dixon")) {
					name
				}
			}
		}
	`

	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"friend":[{"name":"Andrea"}],"gender":"female","name":"Michonne"}]}`,
		js)
}

func TestToFastJSONFilterNot1(t *testing.T) {
	populateGraph(t)
	query := `
		{
			me(id:0x01) {
				name
				gender
				friend @filter(not anyofterms(name, "Andrea rick")) {
					name
				}
			}
		}
	`

	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"gender":"female","name":"Michonne","friend":[{"name":"Glenn Rhee"},{"name":"Daryl Dixon"}]}]}`, js)
}

func TestToFastJSONFilterNot2(t *testing.T) {
	populateGraph(t)
	query := `
		{
			me(id:0x01) {
				name
				gender
				friend @filter(not anyofterms(name, "Andrea") and anyofterms(name, "Glenn Andrea")) {
					name
				}
			}
		}
	`

	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"gender":"female","name":"Michonne","friend":[{"name":"Glenn Rhee"}]}]}`, js)
}

func TestToFastJSONFilterNot3(t *testing.T) {
	populateGraph(t)
	query := `
		{
			me(id:0x01) {
				name
				gender
				friend @filter(not (anyofterms(name, "Andrea") or anyofterms(name, "Glenn Rick Andrea"))) {
					name
				}
			}
		}
	`

	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"gender":"female","name":"Michonne","friend":[{"name":"Daryl Dixon"}]}]}`, js)
}

func TestToFastJSONFilterAnd(t *testing.T) {
	populateGraph(t)
	query := `
		{
			me(id:0x01) {
				name
				gender
				friend @filter(anyofterms(name, "Andrea") and anyofterms(name, "SomethingElse Rhee")) {
					name
				}
			}
		}
	`

	js := processToFastJSON(t, query)
	require.EqualValues(t,
		`{"me":[{"gender":"female","name":"Michonne"}]}`, js)
}

func TestCountReverseFunc(t *testing.T) {
	populateGraph(t)
	posting.CommitLists(10, 1)
	time.Sleep(100 * time.Millisecond)
	query := `
		{
			me(func: ge(count(~friend), 2)) {
				name
				count(~friend)
			}
		}
	`
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"name":"Glenn Rhee","count(~friend)":2}]}`,
		js)
}

func TestCountReverseFilter(t *testing.T) {
	populateGraph(t)
	query := `
		{
			me(func: anyofterms(name, "Glenn Michonne Rick")) @filter(ge(count(~friend), 2)) {
				name
				count(~friend)
			}
		}
	`
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"name":"Glenn Rhee","count(~friend)":2}]}`,
		js)
}

func TestCountReverse(t *testing.T) {
	populateGraph(t)
	query := `
		{
			me(id:0x18) {
				name
				count(~friend)
			}
		}
	`
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"name":"Glenn Rhee","count(~friend)":2}]}`,
		js)
}

func TestToFastJSONReverse(t *testing.T) {
	populateGraph(t)
	query := `
		{
			me(id:0x18) {
				name
				~friend {
					name
					gender
					alive
				}
			}
		}
	`
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"name":"Glenn Rhee","~friend":[{"alive":true,"gender":"female","name":"Michonne"},{"alive": false, "name":"Andrea"}]}]}`,
		js)
}

func TestToFastJSONReverseFilter(t *testing.T) {
	populateGraph(t)
	query := `
		{
			me(id:0x18) {
				name
				~friend @filter(allofterms(name, "Andrea")) {
					name
					gender
				}
			}
		}
	`
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"name":"Glenn Rhee","~friend":[{"name":"Andrea"}]}]}`,
		js)
}

func TestToFastJSONReverseDelSet(t *testing.T) {
	populateGraph(t)
	delEdgeToUID(t, "friend", 1, 24)       // Delete Michonne.
	delEdgeToUID(t, "friend", 23, 24)      // Ignored.
	addEdgeToUID(t, "friend", 25, 24, nil) // Add Daryl.

	query := `
		{
			me(id:0x18) {
				name
				~friend {
					name
					gender
				}
			}
		}
	`
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"name":"Glenn Rhee","~friend":[{"name":"Daryl Dixon"},{"name":"Andrea"}]}]}`,
		js)
}

func TestToFastJSONReverseDelSetCount(t *testing.T) {
	populateGraph(t)
	delEdgeToUID(t, "friend", 1, 24)       // Delete Michonne.
	delEdgeToUID(t, "friend", 23, 24)      // Ignored.
	addEdgeToUID(t, "friend", 25, 24, nil) // Add Daryl.

	query := `
		{
			me(id:0x18) {
				name
				count(~friend)
			}
		}
	`
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"name":"Glenn Rhee","count(~friend)":2}]}`,
		js)
}

func getProperty(properties []*protos.Property, prop string) *protos.Value {
	for _, p := range properties {
		if p.Prop == prop {
			return p.Value
		}
	}
	return nil
}

func TestToProto(t *testing.T) {
	populateGraph(t)
	query := `
		{
			me(id:0x1) {
				_xid_
				name
				gender
				alive
				friend {
					name
				}
			}
		}
  `
	pb := processToPB(t, query, map[string]string{}, true)
	require.EqualValues(t,
		`attribute: "_root_"
children: <
  attribute: "me"
  properties: <
    prop: "_uid_"
    value: <
      uid_val: 1
    >
  >
  properties: <
    prop: "_xid_"
    value: <
      str_val: "mich"
    >
  >
  properties: <
    prop: "name"
    value: <
      str_val: "Michonne"
    >
  >
  properties: <
    prop: "gender"
    value: <
      default_val: "female"
    >
  >
  properties: <
    prop: "alive"
    value: <
      bool_val: true
    >
  >
  children: <
    attribute: "friend"
    properties: <
      prop: "_uid_"
      value: <
        uid_val: 23
      >
    >
    properties: <
      prop: "name"
      value: <
        str_val: "Rick Grimes"
      >
    >
  >
  children: <
    attribute: "friend"
    properties: <
      prop: "_uid_"
      value: <
        uid_val: 24
      >
    >
    properties: <
      prop: "name"
      value: <
        str_val: "Glenn Rhee"
      >
    >
  >
  children: <
    attribute: "friend"
    properties: <
      prop: "_uid_"
      value: <
        uid_val: 25
      >
    >
    properties: <
      prop: "name"
      value: <
        str_val: "Daryl Dixon"
      >
    >
  >
  children: <
    attribute: "friend"
    properties: <
      prop: "_uid_"
      value: <
        uid_val: 31
      >
    >
    properties: <
      prop: "name"
      value: <
        str_val: "Andrea"
      >
    >
  >
  children: <
    attribute: "friend"
    properties: <
      prop: "_uid_"
      value: <
        uid_val: 101
      >
    >
  >
>
`, proto.MarshalTextString(pb[0]))

}

func TestToProtoFilter(t *testing.T) {
	populateGraph(t)
	// Alright. Now we have everything set up. Let's create the query.
	query := `
		{
			me(id:0x01) {
				name
				gender
				friend @filter(anyofterms(name, "Andrea")) {
					name
				}
			}
		}
	`

	pb := processToPB(t, query, map[string]string{}, false)
	expectedPb := `attribute: "_root_"
children: <
  attribute: "me"
  properties: <
    prop: "name"
    value: <
      str_val: "Michonne"
    >
  >
  properties: <
    prop: "gender"
    value: <
      default_val: "female"
    >
  >
  children: <
    attribute: "friend"
    properties: <
      prop: "name"
      value: <
        str_val: "Andrea"
      >
    >
  >
>
`
	require.EqualValues(t, expectedPb, proto.MarshalTextString(pb[0]))
}

func TestToProtoFilterOr(t *testing.T) {
	populateGraph(t)
	// Alright. Now we have everything set up. Let's create the query.
	query := `
		{
			me(id:0x01) {
				name
				gender
				friend @filter(anyofterms(name, "Andrea") or anyofterms(name, "Glenn Rhee")) {
					name
				}
			}
		}
	`

	pb := processToPB(t, query, map[string]string{}, false)
	expectedPb := `attribute: "_root_"
children: <
  attribute: "me"
  properties: <
    prop: "name"
    value: <
      str_val: "Michonne"
    >
  >
  properties: <
    prop: "gender"
    value: <
      default_val: "female"
    >
  >
  children: <
    attribute: "friend"
    properties: <
      prop: "name"
      value: <
        str_val: "Glenn Rhee"
      >
    >
  >
  children: <
    attribute: "friend"
    properties: <
      prop: "name"
      value: <
        str_val: "Andrea"
      >
    >
  >
>
`
	require.EqualValues(t, expectedPb, proto.MarshalTextString(pb[0]))
}

func TestToProtoFilterAnd(t *testing.T) {
	populateGraph(t)
	// Alright. Now we have everything set up. Let's create the query.
	query := `
		{
			me(id:0x01) {
				name
				gender
				friend @filter(anyofterms(name, "Andrea") and anyofterms(name, "Glenn Rhee")) {
					name
				}
			}
		}
	`

	pb := processToPB(t, query, map[string]string{}, false)
	expectedPb := `attribute: "_root_"
children: <
  attribute: "me"
  properties: <
    prop: "name"
    value: <
      str_val: "Michonne"
    >
  >
  properties: <
    prop: "gender"
    value: <
      default_val: "female"
    >
  >
>
`
	require.EqualValues(t, expectedPb, proto.MarshalTextString(pb[0]))
}

// Test sorting / ordering by dob.
func TestToFastJSONOrder(t *testing.T) {
	populateGraph(t)
	query := `
		{
			me(id:0x01) {
				name
				gender
				friend(orderasc: dob) {
					name
					dob
				}
			}
		}
	`

	js := processToFastJSON(t, query)
	require.EqualValues(t,
		`{"me":[{"friend":[{"dob":"1901-01-15","name":"Andrea"},{"dob":"1909-01-10","name":"Daryl Dixon"},{"dob":"1909-05-05","name":"Glenn Rhee"},{"dob":"1910-01-02","name":"Rick Grimes"}],"gender":"female","name":"Michonne"}]}`,
		js)
}

// Test sorting / ordering by dob.
func TestToFastJSONOrderDesc(t *testing.T) {
	populateGraph(t)
	query := `
		{
			me(id:0x01) {
				name
				gender
				friend(orderdesc: dob) {
					name
					dob
				}
			}
		}
	`

	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"friend":[{"dob":"1910-01-02","name":"Rick Grimes"},{"dob":"1909-05-05","name":"Glenn Rhee"},{"dob":"1909-01-10","name":"Daryl Dixon"},{"dob":"1901-01-15","name":"Andrea"}],"gender":"female","name":"Michonne"}]}`,
		string(js))
}

// Test sorting / ordering by dob.
func TestToFastJSONOrderDesc_pawan(t *testing.T) {
	populateGraph(t)
	query := `
		{
			me(id:0x01) {
				name
				gender
				friend(orderdesc: film.film.initial_release_date) {
					name
					film.film.initial_release_date
				}
			}
		}
	`

	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"friend":[{"film.film.initial_release_date":"1929-01-10","name":"Daryl Dixon"},{"film.film.initial_release_date":"1909-05-05","name":"Glenn Rhee"},{"film.film.initial_release_date":"1900-01-02","name":"Rick Grimes"},{"film.film.initial_release_date":"1801-01-15","name":"Andrea"}],"gender":"female","name":"Michonne"}]}`,
		string(js))
}

// Test sorting / ordering by dob.
func TestToFastJSONOrderDedup(t *testing.T) {
	populateGraph(t)
	query := `
		{
			me(id:0x01) {
				name
				gender
				friend(orderasc: name) {
					name
					dob
				}
			}
		}
	`

	js := processToFastJSON(t, query)
	require.EqualValues(t,
		`{"me":[{"friend":[{"dob":"1901-01-15","name":"Andrea"},{"dob":"1909-01-10","name":"Daryl Dixon"},{"dob":"1909-05-05","name":"Glenn Rhee"},{"dob":"1910-01-02","name":"Rick Grimes"}],"gender":"female","name":"Michonne"}]}`,
		js)
}

// Test sorting / ordering by dob and count.
func TestToFastJSONOrderDescCount(t *testing.T) {
	populateGraph(t)
	query := `
		{
			me(id:0x01) {
				name
				gender
				count(friend @filter(anyofterms(name, "Rick")) (orderasc: dob))
			}
		}
	`

	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"count(friend)":1,"gender":"female","name":"Michonne"}]}`,
		string(js))
}

// Test sorting / ordering by dob.
func TestToFastJSONOrderOffset(t *testing.T) {
	populateGraph(t)
	query := `
		{
			me(id:0x01) {
				name
				gender
				friend(orderasc: dob, offset: 2) {
					name
				}
			}
		}
	`

	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"friend":[{"name":"Glenn Rhee"},{"name":"Rick Grimes"}],"gender":"female","name":"Michonne"}]}`,
		js)
}

// Test sorting / ordering by dob.
func TestToFastJSONOrderOffsetCount(t *testing.T) {
	populateGraph(t)
	query := `
		{
			me(id:0x01) {
				name
				gender
				friend(orderasc: dob, offset: 2, first: 1) {
					name
				}
			}
		}
	`

	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"friend":[{"name":"Glenn Rhee"}],"gender":"female","name":"Michonne"}]}`,
		js)
}

// Mocking Subgraph and Testing fast-json with it.
func ageSg(uidMatrix []*protos.List, srcUids *protos.List, ages []uint64) *SubGraph {
	var as []*protos.TaskValue
	for _, a := range ages {
		bs := make([]byte, 4)
		binary.LittleEndian.PutUint64(bs, a)
		as = append(as, &protos.TaskValue{[]byte(bs), 2})
	}

	return &SubGraph{
		Attr:      "age",
		uidMatrix: uidMatrix,
		SrcUIDs:   srcUids,
		values:    as,
		Params:    params{GetUid: true},
	}
}
func nameSg(uidMatrix []*protos.List, srcUids *protos.List, names []string) *SubGraph {
	var ns []*protos.TaskValue
	for _, n := range names {
		ns = append(ns, &protos.TaskValue{[]byte(n), 0})
	}
	return &SubGraph{
		Attr:      "name",
		uidMatrix: uidMatrix,
		SrcUIDs:   srcUids,
		values:    ns,
		Params:    params{GetUid: true},
	}

}
func friendsSg(uidMatrix []*protos.List, srcUids *protos.List, friends []*SubGraph) *SubGraph {
	return &SubGraph{
		Attr:      "friend",
		uidMatrix: uidMatrix,
		SrcUIDs:   srcUids,
		Params:    params{GetUid: true},
		Children:  friends,
	}
}
func rootSg(uidMatrix []*protos.List, srcUids *protos.List, names []string, ages []uint64) *SubGraph {
	nameSg := nameSg(uidMatrix, srcUids, names)
	ageSg := ageSg(uidMatrix, srcUids, ages)

	return &SubGraph{
		Children:  []*SubGraph{nameSg, ageSg},
		Params:    params{GetUid: true},
		SrcUIDs:   srcUids,
		uidMatrix: uidMatrix,
	}
}

// Test sorting / ordering by dob.
func TestToProtoOrder(t *testing.T) {
	populateGraph(t)
	query := `
		{
			me(id:0x01) {
				name
				gender
				friend(orderasc: dob) {
					name
				}
			}
		}
	`

	pb := processToPB(t, query, map[string]string{}, false)
	expectedPb := `attribute: "_root_"
children: <
  attribute: "me"
  properties: <
    prop: "name"
    value: <
      str_val: "Michonne"
    >
  >
  properties: <
    prop: "gender"
    value: <
      default_val: "female"
    >
  >
  children: <
    attribute: "friend"
    properties: <
      prop: "name"
      value: <
        str_val: "Andrea"
      >
    >
  >
  children: <
    attribute: "friend"
    properties: <
      prop: "name"
      value: <
        str_val: "Daryl Dixon"
      >
    >
  >
  children: <
    attribute: "friend"
    properties: <
      prop: "name"
      value: <
        str_val: "Glenn Rhee"
      >
    >
  >
  children: <
    attribute: "friend"
    properties: <
      prop: "name"
      value: <
        str_val: "Rick Grimes"
      >
    >
  >
>
`
	require.EqualValues(t, expectedPb, proto.MarshalTextString(pb[0]))
}

// Test sorting / ordering by dob.
func TestToProtoOrderCount(t *testing.T) {
	populateGraph(t)
	query := `
		{
			me(id:0x01) {
				name
				gender
				friend(orderasc: dob, first: 2) {
					name
				}
			}
		}
	`

	pb := processToPB(t, query, map[string]string{}, false)
	expectedPb := `attribute: "_root_"
children: <
  attribute: "me"
  properties: <
    prop: "name"
    value: <
      str_val: "Michonne"
    >
  >
  properties: <
    prop: "gender"
    value: <
      default_val: "female"
    >
  >
  children: <
    attribute: "friend"
    properties: <
      prop: "name"
      value: <
        str_val: "Andrea"
      >
    >
  >
  children: <
    attribute: "friend"
    properties: <
      prop: "name"
      value: <
        str_val: "Daryl Dixon"
      >
    >
  >
>
`
	require.EqualValues(t, expectedPb, proto.MarshalTextString(pb[0]))
}

// Test sorting / ordering by dob.
func TestToProtoOrderOffsetCount(t *testing.T) {
	populateGraph(t)
	query := `
		{
			me(id:0x01) {
				name
				gender
				friend(orderasc: dob, first: 2, offset: 1) {
					name
				}
			}
		}
	`

	pb := processToPB(t, query, map[string]string{}, false)
	expectedPb := `attribute: "_root_"
children: <
  attribute: "me"
  properties: <
    prop: "name"
    value: <
      str_val: "Michonne"
    >
  >
  properties: <
    prop: "gender"
    value: <
      default_val: "female"
    >
  >
  children: <
    attribute: "friend"
    properties: <
      prop: "name"
      value: <
        str_val: "Daryl Dixon"
      >
    >
  >
  children: <
    attribute: "friend"
    properties: <
      prop: "name"
      value: <
        str_val: "Glenn Rhee"
      >
    >
  >
>
`
	require.EqualValues(t, expectedPb, proto.MarshalTextString(pb[0]))
}

func TestSchema1(t *testing.T) {
	populateGraph(t)
	// Alright. Now we have everything set up. Let's create the query.
	query := `
		{
			person(id:0x01) {
				name
				age
				address
				alive
				survival_rate
				friend {
					name
					address
					age
				}
			}
		}
	`
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"person":[{"address":"31, 32 street, Jupiter","age":38,"alive":true,"friend":[{"address":"21, mark street, Mars","age":15,"name":"Rick Grimes"},{"name":"Glenn Rhee","age":15},{"age":17,"name":"Daryl Dixon"},{"age":19,"name":"Andrea"}],"name":"Michonne","survival_rate":98.990000}]}`, js)
}

func TestMultiQuery(t *testing.T) {
	populateGraph(t)
	query := `
		{
			me(func:anyofterms(name, "Michonne")) {
				name
				gender
			}

			you(func:anyofterms(name, "Andrea")) {
				name
			}
		}
  `
	js := processToFastJSON(t, query)
	require.JSONEq(t, `{"me":[{"gender":"female","name":"Michonne"}],"you":[{"name":"Andrea"},{"name":"Andrea With no friends"}]}`, js)
}

func TestMultiQueryError1(t *testing.T) {
	populateGraph(t)
	query := `
    {
			me(func:anyofterms(name, "Michonne")) {
        name
        gender

			you(func:anyofterms(name, "Andrea")) {
        name
      }
    }
  `
	_, err := gql.Parse(gql.Request{Str: query})
	require.Error(t, err)
}

func TestMultiQueryError2(t *testing.T) {
	populateGraph(t)
	query := `
    {
      me(anyofterms(name, "Michonne")) {
        name
        gender
			}
		}

      you(anyofterms(name, "Andrea")) {
        name
      }
    }
  `
	_, err := gql.Parse(gql.Request{Str: query})
	require.Error(t, err)
}

func TestGenerator(t *testing.T) {
	populateGraph(t)
	query := `
    {
			me(func:anyofterms(name, "Michonne")) {
        name
        gender
      }
    }
  `
	js := processToFastJSON(t, query)
	require.JSONEq(t, `{"me":[{"gender":"female","name":"Michonne"}]}`, js)
}

func TestGeneratorMultiRootMultiQueryRootVar(t *testing.T) {
	populateGraph(t)
	query := `
    {
			friend as var(func:anyofterms(name, "Michonne Rick Glenn")) {
      	name
			}

			you(id: var(friend)) {
				name
			}
    }
  `
	js := processToFastJSON(t, query)
	require.JSONEq(t, `{"you":[{"name":"Michonne"},{"name":"Rick Grimes"},{"name":"Glenn Rhee"}]}`, js)
}

func TestGeneratorMultiRootMultiQueryVarFilter(t *testing.T) {
	populateGraph(t)
	query := `
    {
			f as var(func:anyofterms(name, "Michonne Rick Glenn")) {
      	name
			}

			you(func:anyofterms(name, "Michonne")) {
				friend @filter(var(f)) {
					name
				}
			}
    }
  `
	js := processToFastJSON(t, query)
	require.JSONEq(t, `{"you":[{"friend":[{"name":"Rick Grimes"}, {"name":"Glenn Rhee"}]}]}`, js)
}

func TestGeneratorMultiRootMultiQueryRootVarFilter(t *testing.T) {
	populateGraph(t)
	query := `
    {
			friend as var(func:anyofterms(name, "Michonne Rick Glenn")) {
			}

			you(func:anyofterms(name, "Michonne Andrea Glenn")) @filter(var(friend)) {
				name
			}
    }
  `
	js := processToFastJSON(t, query)
	require.JSONEq(t, `{"you":[{"name":"Michonne"}, {"name":"Glenn Rhee"}]}`, js)
}

func TestGeneratorMultiRootMultiQuery(t *testing.T) {
	populateGraph(t)
	query := `
    {
			me(func:anyofterms(name, "Michonne Rick Glenn")) {
        name
      }

			you(id:[1, 23, 24]) {
				name
			}
    }
  `
	js := processToFastJSON(t, query)
	require.JSONEq(t, `{"me":[{"name":"Michonne"},{"name":"Rick Grimes"},{"name":"Glenn Rhee"}], "you":[{"name":"Michonne"},{"name":"Rick Grimes"},{"name":"Glenn Rhee"}]}`, js)
}

func TestGeneratorMultiRootVarOrderOffset(t *testing.T) {
	populateGraph(t)
	query := `
    {
			L as var(func:anyofterms(name, "Michonne Rick Glenn"), orderasc: dob, offset:2) {
        name
      }

			me(id: var(L)) {
			 name
			}
    }
  `
	js := processToFastJSON(t, query)
	require.JSONEq(t, `{"me":[{"name":"Rick Grimes"}]}`, js)
}

func TestGeneratorMultiRootVarOrderOffset1(t *testing.T) {
	populateGraph(t)
	query := `
    {
			me(func:anyofterms(name, "Michonne Rick Glenn"), orderasc: dob, offset:2) {
        name
      }
    }
  `
	js := processToFastJSON(t, query)
	require.JSONEq(t, `{"me":[{"name":"Rick Grimes"}]}`, js)
}

func TestGeneratorMultiRootOrderOffset(t *testing.T) {
	populateGraph(t)
	query := `
    {
			L as var(func:anyofterms(name, "Michonne Rick Glenn")) {
        name
      }
			me(id: var(L), orderasc: dob, offset:2) {
        name
      }
    }
  `
	js := processToFastJSON(t, query)
	require.JSONEq(t, `{"me":[{"name":"Rick Grimes"}]}`, js)
}

func TestGeneratorMultiRootOrderdesc(t *testing.T) {
	populateGraph(t)
	query := `
    {
			me(func:anyofterms(name, "Michonne Rick Glenn"), orderdesc: dob) {
        name
      }
    }
  `
	js := processToFastJSON(t, query)
	require.JSONEq(t, `{"me":[{"name":"Rick Grimes"},{"name":"Michonne"},{"name":"Glenn Rhee"}]}`, js)
}

func TestGeneratorMultiRootOrder(t *testing.T) {
	populateGraph(t)
	query := `
    {
			me(func:anyofterms(name, "Michonne Rick Glenn"), orderasc: dob) {
        name
      }
    }
  `
	js := processToFastJSON(t, query)
	require.JSONEq(t, `{"me":[{"name":"Glenn Rhee"},{"name":"Michonne"},{"name":"Rick Grimes"}]}`, js)
}

func TestGeneratorMultiRootOffset(t *testing.T) {
	populateGraph(t)
	query := `
    {
			me(func:anyofterms(name, "Michonne Rick Glenn"), offset: 1) {
        name
      }
    }
  `
	js := processToFastJSON(t, query)
	require.JSONEq(t, `{"me":[{"name":"Rick Grimes"},{"name":"Glenn Rhee"}]}`, js)
}

func TestGeneratorMultiRoot(t *testing.T) {
	populateGraph(t)
	query := `
    {
			me(func:anyofterms(name, "Michonne Rick Glenn")) {
        name
      }
    }
  `
	js := processToFastJSON(t, query)
	require.JSONEq(t, `{"me":[{"name":"Michonne"},{"name":"Rick Grimes"},{"name":"Glenn Rhee"}]}`, js)
}

func TestRootList(t *testing.T) {
	populateGraph(t)
	query := `{
	me(id:[1, 23, 24]) {
		name
	}
}`
	js := processToFastJSON(t, query)
	require.JSONEq(t, `{"me":[{"name":"Michonne"},{"name":"Rick Grimes"},{"name":"Glenn Rhee"}]}`, js)
}

func TestRootList1(t *testing.T) {
	populateGraph(t)
	query := `{
	me(id:[0x01, 23, 24, a.bc]) {
		name
	}
}`
	js := processToFastJSON(t, query)
	require.JSONEq(t, `{"me":[{"name":"Michonne"},{"name":"Rick Grimes"},{"name":"Glenn Rhee"},{"name":"Alice"}]}`, js)
}

func TestRootList2(t *testing.T) {
	populateGraph(t)
	query := `{
	me(id:[0x01, 23, a.bc, 24]) {
		name
	}
}`
	js := processToFastJSON(t, query)
	require.JSONEq(t, `{"me":[{"name":"Michonne"},{"name":"Rick Grimes"},{"name":"Alice"},{"name":"Glenn Rhee"}]}`, js)
}

func TestGeneratorMultiRootFilter1(t *testing.T) {
	populateGraph(t)
	query := `
    {
			me(func:anyofterms(name, "Daryl Rick Glenn")) @filter(le(dob, "1909-01-10")) {
        name
      }
    }
  `
	js := processToFastJSON(t, query)
	require.JSONEq(t, `{"me":[{"name":"Daryl Dixon"}]}`, js)
}

func TestGeneratorMultiRootFilter2(t *testing.T) {
	populateGraph(t)
	query := `
    {
			me(func:anyofterms(name, "Michonne Rick Glenn")) @filter(ge(dob, "1909-01-10")) {
        name
      }
    }
  `
	js := processToFastJSON(t, query)
	require.JSONEq(t, `{"me":[{"name":"Michonne"},{"name":"Rick Grimes"},{"name":"Glenn Rhee"}]}`, js)
}

func TestGeneratorMultiRootFilter3(t *testing.T) {
	populateGraph(t)
	query := `
    {
			me(func:anyofterms(name, "Michonne Rick Glenn")) @filter(anyofterms(name, "Glenn") and ge(dob, "1909-01-10")) {
        name
      }
    }
  `
	js := processToFastJSON(t, query)
	require.JSONEq(t, `{"me":[{"name":"Glenn Rhee"}]}`, js)
}

func TestGeneratorRootFilterOnCountGt(t *testing.T) {
	populateGraph(t)
	query := `
                {
                        me(func:anyofterms(name, "Michonne Rick")) @filter(gt(count(friend), 2)) {
                                name
                        }
                }
        `
	_, err := gql.Parse(gql.Request{Str: query})
	require.NoError(t, err)

	js := processToFastJSON(t, query)
	require.JSONEq(t, `{"me":[{"name":"Michonne"}]}`, js)
}

func TestGeneratorRootFilterOnCountle(t *testing.T) {
	populateGraph(t)
	query := `
                {
                        me(func:anyofterms(name, "Michonne Rick")) @filter(le(count(friend), 2)) {
                                name
                        }
                }
        `
	_, err := gql.Parse(gql.Request{Str: query})
	require.NoError(t, err)

	js := processToFastJSON(t, query)
	require.JSONEq(t, `{"me":[{"name":"Rick Grimes"}]}`, js)
}

func TestGeneratorRootFilterOnCountChildLevel(t *testing.T) {
	populateGraph(t)
	query := `
                {
                        me(id:23) {
                                name
                                friend @filter(gt(count(friend), 2)) {
                                        name
                                }
                        }
                }
        `
	_, err := gql.Parse(gql.Request{Str: query})
	require.NoError(t, err)

	js := processToFastJSON(t, query)
	require.JSONEq(t, `{"me":[{"friend":[{"name":"Michonne"}],"name":"Rick Grimes"}]}`, js)
}

func TestGeneratorRootFilterOnCountWithAnd(t *testing.T) {
	populateGraph(t)
	query := `
                {
                        me(id:23) {
                                name
                                friend @filter(gt(count(friend), 4) and lt(count(friend), 100)) {
                                        name
                                }
                        }
                }
        `
	_, err := gql.Parse(gql.Request{Str: query})
	require.NoError(t, err)

	js := processToFastJSON(t, query)
	require.JSONEq(t, `{"me":[{"friend":[{"name":"Michonne"}],"name":"Rick Grimes"}]}`, js)
}

func TestGeneratorRootFilterOnCountError1(t *testing.T) {
	populateGraph(t)
	// only cmp(count(attr), int) is valid, 'max'/'min'/'sum' not supported
	query := `
                {
                        me(func:anyofterms(name, "Michonne Rick")) @filter(gt(count(friend), "invalid")) {
                                name
                        }
                }
        `
	res, err := gql.Parse(gql.Request{Str: query})
	require.NoError(t, err)

	var l Latency
	_, queryErr := ProcessQuery(context.Background(), res, &l)
	require.NotNil(t, queryErr)
}

func TestGeneratorRootFilterOnCountError2(t *testing.T) {
	populateGraph(t)
	// missing digits
	query := `
                {
                        me(func:anyofterms(name, "Michonne Rick")) @filter(gt(count(friend))) {
                                name
                        }
                }
        `
	res, err := gql.Parse(gql.Request{Str: query})
	require.NoError(t, err)

	var l Latency
	_, queryErr := ProcessQuery(context.Background(), res, &l)
	require.NotNil(t, queryErr)
}

func TestGeneratorRootFilterOnCountError3(t *testing.T) {
	populateGraph(t)
	// to much args
	query := `
                {
                        me(func:anyofterms(name, "Michonne Rick")) @filter(gt(count(friend), 2, 4)) {
                                name
                        }
                }
        `
	res, err := gql.Parse(gql.Request{Str: query})
	require.NoError(t, err)

	var l Latency
	_, queryErr := ProcessQuery(context.Background(), res, &l)
	require.Error(t, queryErr)
}

func TestToProtoMultiRoot(t *testing.T) {
	populateGraph(t)
	query := `
    {
			me(func:anyofterms(name, "Michonne Rick Glenn")) {
        name
      }
    }
  `

	pb := processToPB(t, query, map[string]string{}, false)
	expectedPb := `attribute: "_root_"
children: <
  attribute: "me"
  properties: <
    prop: "name"
    value: <
      str_val: "Michonne"
    >
  >
>
children: <
  attribute: "me"
  properties: <
    prop: "name"
    value: <
      str_val: "Rick Grimes"
    >
  >
>
children: <
  attribute: "me"
  properties: <
    prop: "name"
    value: <
      str_val: "Glenn Rhee"
    >
  >
>
`
	require.EqualValues(t, expectedPb, proto.MarshalTextString(pb[0]))
}

func TestNearGenerator(t *testing.T) {
	populateGraph(t)
	query := `{
		me(func:near(loc, [1.1,2.0], 5.001)) {
			name
			gender
		}
	}`

	js := processToFastJSON(t, query)
	require.JSONEq(t, `{"me":[{"gender":"female","name":"Michonne"},{"name":"Glenn Rhee"}]}`, string(js))
}

func TestNearGeneratorFilter(t *testing.T) {
	populateGraph(t)
	query := `{
		me(func:near(loc, [1.1,2.0], 5.001)) @filter(allofterms(name, "Michonne")) {
			name
			gender
		}
	}`

	js := processToFastJSON(t, query)
	require.JSONEq(t, `{"me":[{"gender":"female","name":"Michonne"}]}`, string(js))
}

func TestNearGeneratorError(t *testing.T) {
	populateGraph(t)
	query := `{
		me(func:near(loc, [1.1,2.0], -5.0)) {
			name
			gender
		}
	}`

	res, err := gql.Parse(gql.Request{Str: query})
	require.NoError(t, err)

	ctx := context.Background()
	sg, err := ToSubGraph(ctx, res.Query[0])
	require.NoError(t, err)

	ch := make(chan error)
	go ProcessGraph(ctx, sg, nil, ch)
	err = <-ch
	require.Error(t, err)
}

func TestNearGeneratorErrorMissDist(t *testing.T) {
	populateGraph(t)
	query := `{
		me(func:near(loc, [1.1,2.0])) {
			name
			gender
		}
	}`

	res, err := gql.Parse(gql.Request{Str: query})
	require.NoError(t, err)

	ctx := context.Background()
	sg, err := ToSubGraph(ctx, res.Query[0])
	require.NoError(t, err)

	ch := make(chan error)
	go ProcessGraph(ctx, sg, nil, ch)
	err = <-ch
	require.Error(t, err)
}

func TestWithinGeneratorError(t *testing.T) {
	populateGraph(t)
	query := `{
		me(func:within(loc, [[0.0,0.0], [2.0,0.0], [1.5, 3.0], [0.0, 2.0], [0.0, 0.0]], 12.2)) {
			name
			gender
		}
	}`

	res, err := gql.Parse(gql.Request{Str: query})
	require.NoError(t, err)

	ctx := context.Background()
	sg, err := ToSubGraph(ctx, res.Query[0])
	require.NoError(t, err)

	ch := make(chan error)
	go ProcessGraph(ctx, sg, nil, ch)
	err = <-ch
	require.Error(t, err)
}

func TestWithinGenerator(t *testing.T) {
	populateGraph(t)
	query := `{
		me(func:within(loc,  [[0.0,0.0], [2.0,0.0], [1.5, 3.0], [0.0, 2.0], [0.0, 0.0]])) {
			name
		}
	}`

	js := processToFastJSON(t, query)
	require.JSONEq(t, `{"me":[{"name":"Michonne"},{"name":"Glenn Rhee"}]}`, string(js))
}

func TestContainsGenerator(t *testing.T) {
	populateGraph(t)
	query := `{
		me(func:contains(loc, [2.0,0.0])) {
			name
		}
	}`

	js := processToFastJSON(t, query)
	require.JSONEq(t, `{"me":[{"name":"Rick Grimes"}]}`, string(js))
}

func TestContainsGenerator2(t *testing.T) {
	populateGraph(t)
	query := `{
		me(func:contains(loc,  [[1.0,1.0], [1.9,1.0], [1.9, 1.9], [1.0, 1.9], [1.0, 1.0]])) {
			name
		}
	}`

	js := processToFastJSON(t, query)
	require.JSONEq(t, `{"me":[{"name":"Rick Grimes"}]}`, string(js))
}

func TestIntersectsGeneratorError(t *testing.T) {
	populateGraph(t)
	query := `{
		me(func:intersects(loc, [0.0,0.0])) {
			name
		}
	}`

	res, err := gql.Parse(gql.Request{Str: query})
	require.NoError(t, err)

	ctx := context.Background()
	sg, err := ToSubGraph(ctx, res.Query[0])
	require.NoError(t, err)

	ch := make(chan error)
	go ProcessGraph(ctx, sg, nil, ch)
	err = <-ch
	require.Error(t, err)
}

func TestIntersectsGenerator(t *testing.T) {
	populateGraph(t)
	query := `{
		me(func:intersects(loc, [[0.0,0.0], [2.0,0.0], [1.5, 3.0], [0.0, 2.0], [0.0, 0.0]])) {
			name
		}
	}`

	js := processToFastJSON(t, query)
	require.JSONEq(t, `{"me":[{"name":"Michonne"}, {"name":"Rick Grimes"}, {"name":"Glenn Rhee"}]}`, string(js))
}

func TestToProtoNormalizeDirective(t *testing.T) {
	populateGraph(t)
	query := `
		{
			me(id:0x01) @normalize {
				mn: name
				gender
				friend {
					n: name
					d: dob
					friend {
						fn : name
					}
				}
				son {
					sn: name
				}
			}
		}
	`
	pb := processToPB(t, query, map[string]string{}, false)
	expectedPb := `attribute: "_root_"
children: <
  properties: <
    prop: "mn"
    value: <
      str_val: "Michonne"
    >
  >
  properties: <
    prop: "n"
    value: <
      str_val: "Rick Grimes"
    >
  >
  properties: <
    prop: "d"
    value: <
      str_val: "1910-01-02T00:00:00Z"
    >
  >
  properties: <
    prop: "fn"
    value: <
      str_val: "Michonne"
    >
  >
  properties: <
    prop: "sn"
    value: <
      str_val: "Andre"
    >
  >
>
children: <
  properties: <
    prop: "mn"
    value: <
      str_val: "Michonne"
    >
  >
  properties: <
    prop: "n"
    value: <
      str_val: "Glenn Rhee"
    >
  >
  properties: <
    prop: "d"
    value: <
      str_val: "1909-05-05T00:00:00Z"
    >
  >
  properties: <
    prop: "sn"
    value: <
      str_val: "Andre"
    >
  >
>
children: <
  properties: <
    prop: "mn"
    value: <
      str_val: "Michonne"
    >
  >
  properties: <
    prop: "n"
    value: <
      str_val: "Daryl Dixon"
    >
  >
  properties: <
    prop: "d"
    value: <
      str_val: "1909-01-10T00:00:00Z"
    >
  >
  properties: <
    prop: "fn"
    value: <
      str_val: "Glenn Rhee"
    >
  >
  properties: <
    prop: "sn"
    value: <
      str_val: "Andre"
    >
  >
>
children: <
  properties: <
    prop: "mn"
    value: <
      str_val: "Michonne"
    >
  >
  properties: <
    prop: "n"
    value: <
      str_val: "Andrea"
    >
  >
  properties: <
    prop: "d"
    value: <
      str_val: "1901-01-15T00:00:00Z"
    >
  >
  properties: <
    prop: "fn"
    value: <
      str_val: "Glenn Rhee"
    >
  >
  properties: <
    prop: "sn"
    value: <
      str_val: "Andre"
    >
  >
>
`
	require.EqualValues(t,
		expectedPb,
		proto.MarshalTextString(pb[0]))
}

func TestNormalizeDirective(t *testing.T) {
	populateGraph(t)
	query := `
		{
			me(id:0x01) @normalize {
				mn: name
				gender
				friend {
					n: name
					d: dob
					friend {
						fn : name
					}
				}
				son {
					sn: name
				}
			}
		}
	`

	js := processToFastJSON(t, query)
	require.EqualValues(t,
		`{"me":[{"d":"1910-01-02","fn":"Michonne","mn":"Michonne","n":"Rick Grimes","sn":"Andre"},{"d":"1909-05-05","mn":"Michonne","n":"Glenn Rhee","sn":"Andre"},{"d":"1909-01-10","fn":"Glenn Rhee","mn":"Michonne","n":"Daryl Dixon","sn":"Andre"},{"d":"1901-01-15","fn":"Glenn Rhee","mn":"Michonne","n":"Andrea","sn":"Andre"}]}`,
		js)
}

func TestSchema(t *testing.T) {
	populateGraph(t)
	query := `
		{
			debug(id: 0x1 ) {
				_xid_
				name
				gender
				alive
				loc
				friend {
					dob
					name
				}
			}
		}
  `
	gr := processToPB(t, query, map[string]string{}, true)
	require.Equal(t, `attribute: "_root_"
children: <
  attribute: "debug"
  properties: <
    prop: "_uid_"
    value: <
      uid_val: 1
    >
  >
  properties: <
    prop: "_xid_"
    value: <
      str_val: "mich"
    >
  >
  properties: <
    prop: "name"
    value: <
      str_val: "Michonne"
    >
  >
  properties: <
    prop: "gender"
    value: <
      default_val: "female"
    >
  >
  properties: <
    prop: "alive"
    value: <
      bool_val: true
    >
  >
  properties: <
    prop: "loc"
    value: <
      geo_val: "\001\001\000\000\000\232\231\231\231\231\231\361?\000\000\000\000\000\000\000@"
    >
  >
  children: <
    attribute: "friend"
    properties: <
      prop: "_uid_"
      value: <
        uid_val: 23
      >
    >
    properties: <
      prop: "dob"
      value: <
        str_val: "1910-01-02T00:00:00Z"
      >
    >
    properties: <
      prop: "name"
      value: <
        str_val: "Rick Grimes"
      >
    >
  >
  children: <
    attribute: "friend"
    properties: <
      prop: "_uid_"
      value: <
        uid_val: 24
      >
    >
    properties: <
      prop: "dob"
      value: <
        str_val: "1909-05-05T00:00:00Z"
      >
    >
    properties: <
      prop: "name"
      value: <
        str_val: "Glenn Rhee"
      >
    >
  >
  children: <
    attribute: "friend"
    properties: <
      prop: "_uid_"
      value: <
        uid_val: 25
      >
    >
    properties: <
      prop: "dob"
      value: <
        str_val: "1909-01-10T00:00:00Z"
      >
    >
    properties: <
      prop: "name"
      value: <
        str_val: "Daryl Dixon"
      >
    >
  >
  children: <
    attribute: "friend"
    properties: <
      prop: "_uid_"
      value: <
        uid_val: 31
      >
    >
    properties: <
      prop: "dob"
      value: <
        str_val: "1901-01-15T00:00:00Z"
      >
    >
    properties: <
      prop: "name"
      value: <
        str_val: "Andrea"
      >
    >
  >
  children: <
    attribute: "friend"
    properties: <
      prop: "_uid_"
      value: <
        uid_val: 101
      >
    >
  >
>
`, proto.MarshalTextString(gr[0]))
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
	var buf bytes.Buffer
	err = sg.ToFastJSON(&l, &buf, nil, false)
	require.NoError(t, err)
	return string(buf.Bytes())
}

func TestWithinPoint(t *testing.T) {
	populateGraph(t)
	gq := &gql.GraphQuery{
		Alias: "me",
		Func: &gql.Function{
			Attr: "geometry",
			Name: "near",
			Args: []string{`[-122.082506, 37.4249518]`, "1"},
		},
		Children: []*gql.GraphQuery{{Attr: "name"}},
	}

	mp := runQuery(t, gq)
	expected := `{"me":[{"name":"Googleplex"}]}`
	require.JSONEq(t, expected, mp)
}

func TestWithinPolygon(t *testing.T) {
	populateGraph(t)
	gq := &gql.GraphQuery{
		Alias: "me",
		Func: &gql.Function{Attr: "geometry", Name: "within", Args: []string{
			`[[-122.06, 37.37], [-122.1, 37.36], [-122.12, 37.4], [-122.11, 37.43], [-122.04, 37.43], [-122.06, 37.37]]`},
		},
		Children: []*gql.GraphQuery{{Attr: "name"}},
	}

	mp := runQuery(t, gq)
	expected := `{"me":[{"name":"Googleplex"},{"name":"Shoreline Amphitheater"}]}`
	require.JSONEq(t, expected, mp)
}

func TestContainsPoint(t *testing.T) {
	populateGraph(t)
	gq := &gql.GraphQuery{
		Alias: "me",
		Func: &gql.Function{Attr: "geometry", Name: "contains", Args: []string{
			`[-122.082506, 37.4249518]`},
		},
		Children: []*gql.GraphQuery{{Attr: "name"}},
	}

	mp := runQuery(t, gq)
	expected := `{"me":[{"name":"SF Bay area"},{"name":"Mountain View"}]}`
	require.JSONEq(t, expected, mp)
}

func TestNearPoint(t *testing.T) {
	populateGraph(t)
	gq := &gql.GraphQuery{
		Alias: "me",
		Func: &gql.Function{
			Attr: "geometry",
			Name: "near",
			Args: []string{`[-122.082506, 37.4249518]`, "1000"},
		},
		Children: []*gql.GraphQuery{{Attr: "name"}},
	}

	mp := runQuery(t, gq)
	expected := `{"me":[{"name":"Googleplex"},{"name":"Shoreline Amphitheater"}]}`
	require.JSONEq(t, expected, mp)
}

func TestIntersectsPolygon1(t *testing.T) {
	populateGraph(t)
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
		Children: []*gql.GraphQuery{{Attr: "name"}},
	}

	mp := runQuery(t, gq)
	expected := `{"me":[{"name":"Googleplex"},{"name":"Shoreline Amphitheater"},
		{"name":"SF Bay area"},{"name":"Mountain View"}]}`
	require.JSONEq(t, expected, mp)
}

func TestIntersectsPolygon2(t *testing.T) {
	populateGraph(t)
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
		Children: []*gql.GraphQuery{{Attr: "name"}},
	}

	mp := runQuery(t, gq)
	expected := `{"me":[{"name":"Googleplex"},{"name":"Shoreline Amphitheater"},
			{"name":"San Carlos Airport"},{"name":"SF Bay area"},
			{"name":"Mountain View"},{"name":"San Carlos"}]}`
	require.JSONEq(t, expected, mp)
}

func TestNotExistObject(t *testing.T) {
	populateGraph(t)
	// we haven't set genre(type:uid) for 0x01, should just be ignored
	query := `
                {
                        me(id:0x01) {
                                name
                                gender
                                alive
                                genre
                        }
                }
        `
	js := processToFastJSON(t, query)
	require.EqualValues(t,
		`{"me":[{"alive":true,"gender":"female","name":"Michonne"}]}`,
		js)
}

func TestLangDefault(t *testing.T) {
	populateGraph(t)
	query := `
		{
			me(id:0x1001) {
				name
			}
		}
	`
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"name":"Badger"}]}`,
		js)
}

func TestLangMultiple_Alias(t *testing.T) {
	populateGraph(t)
	query := `
		{
			me(id:0x1001) {
				a: name@pl
				b: name@cn
				c: name
			}
		}
	`
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"c":"Badger","b":"Badger","a":"Borsuk europejski"}]}`,
		js)
}

func TestLangMultiple(t *testing.T) {
	populateGraph(t)
	query := `
		{
			me(id:0x1001) {
				name@pl
				name
			}
		}
	`
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"name":"Badger","name@pl":"Borsuk europejski"}]}`,
		js)
}

func TestLangSingle(t *testing.T) {
	populateGraph(t)
	query := `
		{
			me(id:0x1001) {
				name@pl
			}
		}
	`
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"name@pl":"Borsuk europejski"}]}`,
		js)
}

func TestLangSingleFallback(t *testing.T) {
	populateGraph(t)
	query := `
		{
			me(id:0x1001) {
				name@cn
			}
		}
	`
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"name@cn":"Badger"}]}`,
		js)
}

func TestLangMany1(t *testing.T) {
	populateGraph(t)
	query := `
		{
			me(id:0x1001) {
				name@ru:en:fr
			}
		}
	`
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"name@ru:en:fr":"Барсук"}]}`,
		js)
}

func TestLangMany2(t *testing.T) {
	populateGraph(t)
	query := `
		{
			me(id:0x1001) {
				name@hu:fi:fr
			}
		}
	`
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"name@hu:fi:fr":"Blaireau européen"}]}`,
		js)
}

func TestLangMany3(t *testing.T) {
	populateGraph(t)
	query := `
		{
			me(id:0x1001) {
				name@hu:fr:fi
			}
		}
	`
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"name@hu:fr:fi":"Blaireau européen"}]}`,
		js)
}

func TestLangManyFallback(t *testing.T) {
	populateGraph(t)
	query := `
		{
			me(id:0x1001) {
				name@hu:fi:cn
			}
		}
	`
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"name@hu:fi:cn":"Badger"}]}`,
		js)
}

func TestLangFilterMatch1(t *testing.T) {
	populateGraph(t)
	posting.CommitLists(10, 1)
	query := `
		{
			me(func:allofterms(name@pl, "Europejski borsuk"))  {
				name@pl
			}
		}
	`
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"name@pl":"Borsuk europejski"}]}`,
		js)
}

func TestLangFilterMismatch1(t *testing.T) {
	populateGraph(t)
	posting.CommitLists(10, 1)
	query := `
		{
			me(func:allofterms(name@pl, "European Badger"))  {
				name@pl
			}
		}
	`
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{}`,
		js)
}

func TestLangFilterMismatch2(t *testing.T) {
	populateGraph(t)
	posting.CommitLists(10, 1)
	query := `
		{
			me(id: [0x1, 0x2, 0x3, 0x1001]) @filter(anyofterms(name@pl, "Badger is cool")) {
				name@pl
			}
		}
	`
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{}`,
		js)
}

func TestLangFilterMismatch3(t *testing.T) {
	populateGraph(t)
	posting.CommitLists(10, 1)
	query := `
		{
			me(id: [0x1, 0x2, 0x3, 0x1001]) @filter(allofterms(name@pl, "European borsuk")) {
				name@pl
			}
		}
	`
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{}`,
		js)
}

func TestLangFilterMismatch5(t *testing.T) {
	populateGraph(t)
	posting.CommitLists(10, 1)
	query := `
		{
			me(func:anyofterms(name@en, "european honey")) {
				name@en
			}
		}
	`
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"name@en":"European badger"},{"name@en":"Honey badger"},{"name@en":"Honey bee"}]}`,
		js)
}

func TestLangFilterMismatch6(t *testing.T) {
	populateGraph(t)
	posting.CommitLists(10, 1)
	query := `
		{
			me(id: [0x1001, 0x1002, 0x1003]) @filter(lt(name@en, "D"))  {
				name@en
			}
		}
	`
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{}`,
		js)
}

func checkSchemaNodes(t *testing.T, expected []*protos.SchemaNode, actual []*protos.SchemaNode) {
	sort.Slice(expected, func(i, j int) bool {
		return expected[i].Predicate >= expected[j].Predicate
	})
	sort.Slice(actual, func(i, j int) bool {
		return actual[i].Predicate >= actual[j].Predicate
	})
	require.True(t, reflect.DeepEqual(expected, actual),
		fmt.Sprintf("Expected: %+v, Received: %+v \n", expected, actual))
}

func TestSchemaBlock1(t *testing.T) {
	query := `
		schema {
			type
		}
	`
	actual := processSchemaQuery(t, query)
	expected := []*protos.SchemaNode{{Predicate: "genre", Type: "uid"},
		{Predicate: "age", Type: "int"}, {Predicate: "name", Type: "string"},
		{Predicate: "film.film.initial_release_date", Type: "date"},
		{Predicate: "loc", Type: "geo"}, {Predicate: "alive", Type: "bool"},
		{Predicate: "shadow_deep", Type: "int"}, {Predicate: "friend", Type: "uid"},
		{Predicate: "geometry", Type: "geo"}, {Predicate: "alias", Type: "string"},
		{Predicate: "dob", Type: "date"}, {Predicate: "survival_rate", Type: "float"},
		{Predicate: "value", Type: "string"}, {Predicate: "full_name", Type: "string"},
		{Predicate: "noindex_name", Type: "string"}, {Predicate: "_xid_", Type: "string"}}
	checkSchemaNodes(t, expected, actual)
}

func TestSchemaBlock2(t *testing.T) {
	query := `
		schema(pred: name) {
			index
			reverse
			type
			tokenizer
		}
	`
	actual := processSchemaQuery(t, query)
	expected := []*protos.SchemaNode{
		{Predicate: "name", Type: "string", Index: true, Tokenizer: []string{"term", "exact", "trigram"}}}
	checkSchemaNodes(t, expected, actual)
}

func TestSchemaBlock3(t *testing.T) {
	query := `
		schema(pred: age) {
			index
			reverse
			type
			tokenizer
		}
	`
	actual := processSchemaQuery(t, query)
	expected := []*protos.SchemaNode{{Predicate: "age", Type: "int", Index: true, Tokenizer: []string{"int"}}}
	checkSchemaNodes(t, expected, actual)
}

func TestSchemaBlock4(t *testing.T) {
	query := `
		schema(pred: [age, genre, random]) {
			index
			reverse
			type
			tokenizer
		}
	`
	actual := processSchemaQuery(t, query)
	expected := []*protos.SchemaNode{
		{Predicate: "genre", Type: "uid", Reverse: true}, {Predicate: "age", Type: "int", Index: true, Tokenizer: []string{"int"}}}
	checkSchemaNodes(t, expected, actual)
}

func TestSchemaBlock5(t *testing.T) {
	query := `
		schema(pred: name) {
		}
	`
	actual := processSchemaQuery(t, query)
	expected := []*protos.SchemaNode{
		{Predicate: "name", Type: "string", Index: true, Tokenizer: []string{"term", "exact", "trigram"}}}
	checkSchemaNodes(t, expected, actual)
}

const schemaStr = `
name                           : string @index(term, exact, trigram) .
alias                          : string @index(exact, term, fulltext) .
dob                            : date @index .
film.film.initial_release_date : date @index .
loc                            : geo @index .
genre                          : uid @reverse .
survival_rate                  : float .
alive                          : bool @index .
age                            : int @index .
shadow_deep                    : int .
friend                         : uid @reverse .
geometry                       : geo @index .
value                          : string @index(trigram) .
full_name                      : string @index(hash) .
noindex_name                   : string .
`

func TestMain(m *testing.M) {
	x.SetTestRun()
	x.Init()

	dir, err := ioutil.TempDir("", "storetest_")
	x.Check(err)
	defer os.RemoveAll(dir)

	opt := badger.DefaultOptions
	opt.Dir = dir
	ps, err = badger.NewKV(&opt)
	defer ps.Close()
	x.Check(err)
	time.Sleep(time.Second)

	group.ParseGroupConfig("")
	schema.Init(ps)
	posting.Init(ps)
	worker.Init(ps)

	dir2, err := ioutil.TempDir("", "wal_")
	x.Check(err)

	worker.StartRaftNodes(dir2)
	// Load schema after nodes have started
	err = schema.ParseBytes([]byte(schemaStr), 1)
	x.Check(err)
	defer os.RemoveAll(dir2)

	os.Exit(m.Run())
}

func TestFilterNonIndexedPredicateFail(t *testing.T) {
	populateGraph(t)
	// filtering on non indexing predicate fails
	query := `
		{
			me(id:0x01) {
				friend @filter(le(survival_rate, 30)) {
					_uid_
					name
					age
				}
			}
		}
	`
	_, err := processToFastJsonReq(t, query)
	require.Error(t, err)
}

func TestMultipleSamePredicateInBlockFail(t *testing.T) {
	populateGraph(t)
	// name is asked for two times..
	query := `
		{
			me(id:0x01) {
				name
				friend {
					age
				}
				name
			}
		}
	`
	_, err := processToFastJsonReq(t, query)
	require.Error(t, err)
}

func TestMultipleSamePredicateInBlockFail2(t *testing.T) {
	populateGraph(t)
	// age is asked for two times..
	query := `
		{
			me(id:0x01) {
				friend {
					age
					age
				}
				name
			}
		}
	`
	_, err := processToFastJsonReq(t, query)
	require.Error(t, err)
}

func TestMultipleSamePredicateInBlockFail3(t *testing.T) {
	populateGraph(t)
	// friend is asked for two times..
	query := `
		{
			me(id:0x01) {
				friend {
					age
				}
				friend {
					name
				}
				name
			}
		}
	`
	_, err := processToFastJsonReq(t, query)
	require.Error(t, err)
}

func TestXidInvalidJSON(t *testing.T) {
	populateGraph(t)
	query := `
		{
			me(id:0x01) {
				name
				_xid_
				gender
				alive
				friend {
					_xid_
					random
					name
				}
			}
		}
	`
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"_xid_":"mich","alive":true,"friend":[{"name":"Rick Grimes"},{"_xid_":"g\"lenn","name":"Glenn Rhee"},{"name":"Daryl Dixon"},{"name":"Andrea"}],"gender":"female","name":"Michonne"}]}`,
		js)
	m := make(map[string]interface{})
	err := json.Unmarshal([]byte(js), &m)
	require.NoError(t, err)
}

func TestXidInvalidProto(t *testing.T) {
	populateGraph(t)
	query := `
		{
			me(id:0x01) {
				name
				_xid_
				gender
				alive
				friend {
					_xid_
					random
					name
				}
			}
		}
	`
	pb := processToPB(t, query, map[string]string{}, false)
	expectedPb := `attribute: "_root_"
children: <
  attribute: "me"
  properties: <
    prop: "name"
    value: <
      str_val: "Michonne"
    >
  >
  properties: <
    prop: "_xid_"
    value: <
      str_val: "mich"
    >
  >
  properties: <
    prop: "gender"
    value: <
      default_val: "female"
    >
  >
  properties: <
    prop: "alive"
    value: <
      bool_val: true
    >
  >
  children: <
    attribute: "friend"
    properties: <
      prop: "name"
      value: <
        str_val: "Rick Grimes"
      >
    >
  >
  children: <
    attribute: "friend"
    properties: <
      prop: "_xid_"
      value: <
        str_val: "g\\\"lenn"
      >
    >
    properties: <
      prop: "name"
      value: <
        str_val: "Glenn Rhee"
      >
    >
  >
  children: <
    attribute: "friend"
    properties: <
      prop: "name"
      value: <
        str_val: "Daryl Dixon"
      >
    >
  >
  children: <
    attribute: "friend"
    properties: <
      prop: "name"
      value: <
        str_val: "Andrea"
      >
    >
  >
>
`
	require.EqualValues(t, expectedPb, proto.MarshalTextString(pb[0]))
}

func TestToJSONReverseNegativeFirst(t *testing.T) {
	populateGraph(t)
	query := `
		{
			me(func: allofterms(name, "Andrea")) {
				name
				~friend (first: -1) {
					name
					gender
				}
			}
		}
	`
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"name":"Andrea","~friend":[{"gender":"female","name":"Michonne"}]},{"name":"Andrea With no friends"}]}`,
		js)
}

func TestToFastJSONOrderLang(t *testing.T) {
	populateGraph(t)
	query := `
		{
			me(id:0x01) {
				friend(first:2, orderdesc: alias@en:de) {
					alias
				}
			}
		}
	`

	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"friend":[{"alias":"Zambo Alice"},{"alias":"John Oliver"}]}]}`,
		js)
}

func TestBoolIndexEqRoot1(t *testing.T) {
	populateGraph(t)
	query := `
		{
			me(func: eq(alive, true)) {
				name
				alive
			}
		}
	`
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"alive":true,"name":"Michonne"},{"alive":true,"name":"Rick Grimes"}]}`,
		js)
}

func TestBoolIndexEqRoot2(t *testing.T) {
	populateGraph(t)
	query := `
		{
			me(func: eq(alive, false)) {
				name
				alive
			}
		}
	`
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"alive":false,"name":"Daryl Dixon"},{"alive":false,"name":"Andrea"}]}`,
		js)
}

func TestBoolIndexgeRoot(t *testing.T) {
	populateGraph(t)
	q := `
		{
			me(func: ge(alive, true)) {
				name
				alive
				friend {
					name
					alive
				}
			}
		}`
	res, _ := gql.Parse(gql.Request{Str: q})
	var l Latency
	ctx := context.Background()
	_, err := ProcessQuery(ctx, res, &l)
	require.NotNil(t, err)
}

func TestBoolIndexEqChild(t *testing.T) {
	populateGraph(t)
	query := `
		{
			me(func: eq(alive, true)) {
				name
				alive
				friend @filter(eq(alive, false)) {
					name
					alive
				}
			}
		}
	`
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"alive":true,"friend":[{"alive":false,"name":"Daryl Dixon"},{"alive":false,"name":"Andrea"}],"name":"Michonne"},{"alive":true,"name":"Rick Grimes"}]}`,
		js)
}

func TestBoolSort(t *testing.T) {
	populateGraph(t)
	q := `
		{
			me(func: anyofterms(name, "Michonne Andrea Rick"), orderasc: alive) {
				name
				alive
			}
		}
	`
	res, _ := gql.Parse(gql.Request{Str: q, Http: true})
	var l Latency
	ctx := context.Background()
	_, err := ProcessQuery(ctx, res, &l)
	require.NotNil(t, err)
}

func TestJSONQueryVariables(t *testing.T) {
	populateGraph(t)
	q := `{"query": "query test ($a: int = 1) { me(id: 0x01) { name, gender, friend(first: $a) { name }}}",
	"variables" : { "$a": "2"}}`
	js := processToFastJSON(t, q)
	require.JSONEq(t, `{"me":[{"friend":[{"name":"Rick Grimes"},{"name":"Glenn Rhee"}],"gender":"female","name":"Michonne"}]}`, string(js))
}

func TestPBQueryVariables(t *testing.T) {
	populateGraph(t)
	q := `query test ($a: int = 1) { me(id: 0x01) { name, gender, friend(first: $a) { name }}}`

	variables := make(map[string]string)
	variables["$a"] = "3"
	pb := processToPB(t, q, variables, false)
	require.Equal(t, `attribute: "_root_"
children: <
  attribute: "me"
  properties: <
    prop: "name"
    value: <
      str_val: "Michonne"
    >
  >
  properties: <
    prop: "gender"
    value: <
      default_val: "female"
    >
  >
  children: <
    attribute: "friend"
    properties: <
      prop: "name"
      value: <
        str_val: "Rick Grimes"
      >
    >
  >
  children: <
    attribute: "friend"
    properties: <
      prop: "name"
      value: <
        str_val: "Glenn Rhee"
      >
    >
  >
  children: <
    attribute: "friend"
    properties: <
      prop: "name"
      value: <
        str_val: "Daryl Dixon"
      >
    >
  >
>
`, proto.MarshalTextString(pb[0]))
}

func TestStringEscape(t *testing.T) {
	populateGraph(t)
	query := `
		{
			me(id: 2301) {
				name
			}
		}
	`
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"name":"Alice\""}]}`,
		js)
}

func TestOrderDescFilterCount(t *testing.T) {
	populateGraph(t)
	query := `
		{
			me(id:0x01) {
				friend(first:2, orderdesc: age) @filter(eq(alias, "Zambo Alice")) {
					alias
				}
			}
		}
	`

	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"friend":[{"alias":"Zambo Alice"}]}]}`,
		js)
}

func TestHashTokEq(t *testing.T) {
	populateGraph(t)
	query := `
		{
			me(func: eq(full_name, "Michonne's large name for hashing")) {
				full_name
				alive
				friend {
					name
				}
			}
		}
	`
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"alive":true,"friend":[{"name":"Rick Grimes"},{"name":"Glenn Rhee"},{"name":"Daryl Dixon"},{"name":"Andrea"}],"full_name":"Michonne's large name for hashing"}]}`,
		js)
}

func TestHashTokGeqErr(t *testing.T) {
	populateGraph(t)
	query := `
		{
			me(func: ge(full_name, "Michonne's large name for hashing")) {
				full_name
				alive
				friend {
					name
				}
			}
		}
	`
	res, _ := gql.Parse(gql.Request{Str: query})
	var l Latency
	ctx := context.Background()
	_, err := ProcessQuery(ctx, res, &l)
	require.Error(t, err)
}

func TestNameNotIndexed(t *testing.T) {
	populateGraph(t)
	query := `
		{
			me(func: eq(noindex_name, "Michonne's name not indexed")) {
				full_name
				alive
				friend {
					name
				}
			}
		}
	`
	res, _ := gql.Parse(gql.Request{Str: query})
	var l Latency
	ctx := context.Background()
	_, err := ProcessQuery(ctx, res, &l)
	require.Error(t, err)
}

func TestMultipleMinMax(t *testing.T) {
	populateGraph(t)
	query := `
		{
			me(id: 0x01) {
				friend {
					x as age
					n as name
				}
				min(var(x))
				max(var(x))
				min(var(n))
				max(var(n))
			}
		}`
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"friend":[{"age":15,"name":"Rick Grimes"},{"age":15,"name":"Glenn Rhee"},{"age":17,"name":"Daryl Dixon"},{"age":19,"name":"Andrea"}],"max(var(n))":"Rick Grimes","max(var(x))":19,"min(var(n))":"Andrea","min(var(x))":15}]}`,
		js)
}

func TestDuplicateAlias(t *testing.T) {
	populateGraph(t)
	query := `
		{
			me(id: 0x01) {
				friend {
					x as age
				}
				a: min(var(x))
				a: max(var(x))
			}
		}`
	res, _ := gql.Parse(gql.Request{Str: query})
	var l Latency
	ctx := context.Background()
	_, err := ProcessQuery(ctx, res, &l)
	require.Error(t, err)
}

func TestMinSomething(t *testing.T) {
	populateGraph(t)
	query := `
		{
			me(id: 0x01) {
				friendCount: count(friend)
			}
		}`
	pb := processToPB(t, query, map[string]string{}, false)
	require.Equal(t, `attribute: "_root_"
children: <
  attribute: "me"
  properties: <
    prop: "friendCount"
    value: <
      int_val: 5
    >
  >
>
`, proto.MarshalTextString(pb[0]))
}

func TestGraphQLId(t *testing.T) {
	populateGraph(t)
	q := `{"query": "query test ($a: string = 1) { me(id: $a) { name, gender, friend(first: 1) { name }}}",
	"variables" : { "$a": "[1, 31]"}}`
	js := processToFastJSON(t, q)
	require.JSONEq(t, `{"me":[{"friend":[{"name":"Rick Grimes"}],"gender":"female","name":"Michonne"},{"friend":[{"name":"Glenn Rhee"}],"name":"Andrea"}]}`, string(js))
}

func TestGraphQLIdProto(t *testing.T) {
	populateGraph(t)
	q := `query test ($id: string) { me(id: $id) { name, gender, friend(first: 1) { name }}}`

	variables := make(map[string]string)
	variables["$id"] = "1"
	pb := processToPB(t, q, variables, false)
	require.Equal(t, `attribute: "_root_"
children: <
  attribute: "me"
  properties: <
    prop: "name"
    value: <
      str_val: "Michonne"
    >
  >
  properties: <
    prop: "gender"
    value: <
      default_val: "female"
    >
  >
  children: <
    attribute: "friend"
    properties: <
      prop: "name"
      value: <
        str_val: "Rick Grimes"
      >
    >
  >
>
`, proto.MarshalTextString(pb[0]))
}

func TestDebugUid(t *testing.T) {
	populateGraph(t)
	query := `
		{
			me(id: 0x01) {
				name
				friend {
				  friend
				}
			}
		}`
	res, err := gql.Parse(gql.Request{Str: query})
	require.NoError(t, err)

	var l Latency
	ctx := context.Background()
	ctx = context.WithValue(ctx, "debug", "true")
	sgl, err := ProcessQuery(ctx, res, &l)
	require.NoError(t, err)
	var buf bytes.Buffer
	err = ToJson(&l, sgl, &buf, nil, false)
	require.NoError(t, err)
	var mp map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(buf.Bytes()), &mp))
	resp := mp["me"]
	body, err := json.Marshal(resp)
	require.NoError(t, err)
	require.Equal(t, `[{"_uid_":"0x1","friend":[{"_uid_":"0x17","friend":[{"_uid_":"0x1"}]},{"_uid_":"0x18"},{"_uid_":"0x19","friend":[{"_uid_":"0x18"}]},{"_uid_":"0x1f","friend":[{"_uid_":"0x18"}]},{"_uid_":"0x65"}],"name":"Michonne"}]`, string(body))
}

func TestUidAlias(t *testing.T) {
	populateGraph(t)
	query := `
		{
			me(id: 0x1) {
				id: _uid_
				alive
				friend {
					uid: _uid_
					name
				}
			}
		}
	`
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"alive":true,"friend":[{"name":"Rick Grimes","uid":"0x17"},{"name":"Glenn Rhee","uid":"0x18"},{"name":"Daryl Dixon","uid":"0x19"},{"name":"Andrea","uid":"0x1f"},{"uid":"0x65"}],"id":"0x1"}]}`,
		js)
}

func TestUidAliasProto(t *testing.T) {
	populateGraph(t)
	query := `
		{
			me(id: 0x1) {
				id: _uid_
				alive
				friend {
					uid: _uid_
					name
				}
			}
		}
	`
	pb := processToPB(t, query, nil, false)
	require.Equal(t, `attribute: "_root_"
children: <
  attribute: "me"
  properties: <
    prop: "id"
    value: <
      uid_val: 1
    >
  >
  properties: <
    prop: "alive"
    value: <
      bool_val: true
    >
  >
  children: <
    attribute: "friend"
    properties: <
      prop: "uid"
      value: <
        uid_val: 23
      >
    >
    properties: <
      prop: "name"
      value: <
        str_val: "Rick Grimes"
      >
    >
  >
  children: <
    attribute: "friend"
    properties: <
      prop: "uid"
      value: <
        uid_val: 24
      >
    >
    properties: <
      prop: "name"
      value: <
        str_val: "Glenn Rhee"
      >
    >
  >
  children: <
    attribute: "friend"
    properties: <
      prop: "uid"
      value: <
        uid_val: 25
      >
    >
    properties: <
      prop: "name"
      value: <
        str_val: "Daryl Dixon"
      >
    >
  >
  children: <
    attribute: "friend"
    properties: <
      prop: "uid"
      value: <
        uid_val: 31
      >
    >
    properties: <
      prop: "name"
      value: <
        str_val: "Andrea"
      >
    >
  >
  children: <
    attribute: "friend"
    properties: <
      prop: "uid"
      value: <
        uid_val: 101
      >
    >
  >
>
`, proto.MarshalTextString(pb[0]))
}

func TestCountAtRoot(t *testing.T) {
	populateGraph(t)
	posting.CommitLists(10, 1)
	time.Sleep(100 * time.Millisecond)
	query := `
                {
                        me(func: ge(count(friend), 0)) {
				count()
			}
                }
        `
	js := processToFastJSON(t, query)
	require.JSONEq(t, `{"me":[{"count": 4}]}`, js)
}

func TestCountAtRoot2(t *testing.T) {
	populateGraph(t)
	posting.CommitLists(10, 1)
	time.Sleep(100 * time.Millisecond)
	query := `
        {
                me(func: anyofterms(name, "Michonne Rick Andrea")) {
			count()
		}
        }
        `
	js := processToFastJSON(t, query)
	require.JSONEq(t, `{"me":[{"count": 4}]}`, js)
}

func TestCountAtRoot2PB(t *testing.T) {
	populateGraph(t)
	posting.CommitLists(10, 1)
	time.Sleep(100 * time.Millisecond)
	query := `
        {
                me(func: anyofterms(name, "Michonne Rick Andrea")) {
			name
			count()
		}
        }
        `
	pb := processToPB(t, query, nil, false)
	require.Equal(t, `attribute: "_root_"
children: <
  attribute: "me"
  properties: <
    prop: "count"
    value: <
      int_val: 4
    >
  >
>
children: <
  attribute: "me"
  properties: <
    prop: "name"
    value: <
      str_val: "Michonne"
    >
  >
>
children: <
  attribute: "me"
  properties: <
    prop: "name"
    value: <
      str_val: "Rick Grimes"
    >
  >
>
children: <
  attribute: "me"
  properties: <
    prop: "name"
    value: <
      str_val: "Andrea"
    >
  >
>
children: <
  attribute: "me"
  properties: <
    prop: "name"
    value: <
      str_val: "Andrea With no friends"
    >
  >
>
`, proto.MarshalTextString(pb[0]))
}

func TestCountAtRoot3(t *testing.T) {
	populateGraph(t)
	posting.CommitLists(10, 1)
	time.Sleep(100 * time.Millisecond)
	query := `
        {
		me(func:anyofterms(name, "Michonne Rick Daryl")) {
			name
			count()
			count(friend)
			friend {
				name
				count()
			}
		}
        }
        `
	js := processToFastJSON(t, query)
	require.JSONEq(t, `{"me":[{"count":3},{"count(friend)":5,"friend":[{"name":"Rick Grimes"},{"name":"Glenn Rhee"},{"name":"Daryl Dixon"},{"name":"Andrea"},{"count":5}],"name":"Michonne"},{"count(friend)":1,"friend":[{"name":"Michonne"},{"count":1}],"name":"Rick Grimes"},{"count(friend)":1,"friend":[{"name":"Glenn Rhee"},{"count":1}],"name":"Daryl Dixon"}]}`, js)
}

func TestCountAtRootWithAlias4(t *testing.T) {
	populateGraph(t)
	posting.CommitLists(10, 1)
	time.Sleep(100 * time.Millisecond)
	query := `
	{
                me(func:anyofterms(name, "Michonne Rick Daryl")) @filter(le(count(friend), 2)) {
			personCount: count()
		}
        }
        `
	js := processToFastJSON(t, query)
	require.JSONEq(t, `{"me": [{"personCount": 2}]}`, js)
}

func TestCountAtRoot5(t *testing.T) {
	populateGraph(t)
	query := `
	{
		me(id: 1) {
			f as friend {
				name
			}
		}
		MichonneFriends(id: var(f)) {
			count()
		}
	}


        `
	js := processToFastJSON(t, query)
	require.JSONEq(t, `{"MichonneFriends":[{"count":4}],"me":[{"friend":[{"name":"Rick Grimes"},{"name":"Glenn Rhee"},{"name":"Daryl Dixon"},{"name":"Andrea"}]}]}`, js)
}

func TestHasFuncAtRoot(t *testing.T) {
	populateGraph(t)
	posting.CommitLists(10, 1)
	time.Sleep(100 * time.Millisecond)
	query := `
	{
		me(func: has(friend)) {
			name
			friend {
				count()
			}
		}
	}
	`

	js := processToFastJSON(t, query)
	require.JSONEq(t, `{"me":[{"friend":[{"count":5}],"name":"Michonne"},{"friend":[{"count":1}],"name":"Rick Grimes"},{"friend":[{"count":1}],"name":"Daryl Dixon"},{"friend":[{"count":1}],"name":"Andrea"}]}`, string(js))
}

func TestHasFuncAtRootFilter(t *testing.T) {
	populateGraph(t)
	query := `
	{
		me(func: anyofterms(name, "Michonne Rick Daryl")) @filter(has(friend)) {
			name
			friend {
				count()
			}
		}
	}
	`

	js := processToFastJSON(t, query)
	require.JSONEq(t, `{"me":[{"friend":[{"count":5}],"name":"Michonne"},{"friend":[{"count":1}],"name":"Rick Grimes"},{"friend":[{"count":1}],"name":"Daryl Dixon"}]}`, string(js))
}

func TestHasFuncAtChild1(t *testing.T) {
	populateGraph(t)
	posting.CommitLists(10, 1)
	time.Sleep(100 * time.Millisecond)
	query := `
	{
		me(func: has(school)) {
			name
			friend @filter(has(scooter)) {
				name
			}
		}
	}
	`

	js := processToFastJSON(t, query)
	require.JSONEq(t, `{"me":[{"name":"Michonne"},{"name":"Rick Grimes"},{"name":"Glenn Rhee"},{"name":"Daryl Dixon"},{"name":"Andrea"}]}`, string(js))
}

func TestHasFuncAtChild2(t *testing.T) {
	populateGraph(t)
	posting.CommitLists(10, 1)
	time.Sleep(100 * time.Millisecond)
	query := `
	{
		me(func: has(school)) {
			name
			friend @filter(has(alias)) {
				name
				alias
			}
		}
	}
	`

	js := processToFastJSON(t, query)
	require.JSONEq(t, `{"me":[{"friend":[{"alias":"Zambo Alice","name":"Rick Grimes"},{"alias":"John Alice","name":"Glenn Rhee"},{"alias":"Bob Joe","name":"Daryl Dixon"},{"alias":"Allan Matt","name":"Andrea"},{"alias":"John Oliver"}],"name":"Michonne"},{"name":"Rick Grimes"},{"name":"Glenn Rhee"},{"friend":[{"alias":"John Alice","name":"Glenn Rhee"}],"name":"Daryl Dixon"},{"friend":[{"alias":"John Alice","name":"Glenn Rhee"}],"name":"Andrea"}]}`, string(js))
}

func TestHasFuncAtRoot2(t *testing.T) {
	populateGraph(t)
	posting.CommitLists(10, 1)
	time.Sleep(100 * time.Millisecond)
	query := `
	{
		me(func: has(name@en)) {
			name@en
		}
	}
	`

	js := processToFastJSON(t, query)
	require.JSONEq(t, `{"me":[{"name@en":"Test facet"},{"name@en":"European badger"},{"name@en":"Honey badger"},{"name@en":"Honey bee"},{"name@en":"Artem Tkachenko"}]}`, string(js))
}

func getSubGraphs(t *testing.T, query string) (subGraphs []*SubGraph) {
	res, err := gql.Parse(gql.Request{Str: query})
	require.NoError(t, err)

	ctx := context.Background()
	for _, block := range res.Query {
		subGraph, err := ToSubGraph(ctx, block)
		require.NoError(t, err)
		require.NotNil(t, subGraph)

		subGraphs = append(subGraphs, subGraph)
	}

	return subGraphs
}

// simplest case
func TestGetAllPredicatesSimple(t *testing.T) {
	query := `
	{
		me(id: 0x1) {
			name
		}
	}
	`

	subGraphs := getSubGraphs(t, query)

	predicates := GetAllPredicates(subGraphs)
	require.NotNil(t, predicates)
	require.Equal(t, 1, len(predicates))
	require.Equal(t, "name", predicates[0])
}

// recursive SubGraph traversal; predicates should be unique
func TestGetAllPredicatesUnique(t *testing.T) {
	query := `
	{
		me(id: 0x1) {
			name
			friend {
				name
				age
			}
		}
	}
	`

	subGraphs := getSubGraphs(t, query)

	predicates := GetAllPredicates(subGraphs)
	require.NotNil(t, predicates)
	require.Equal(t, 3, len(predicates))
	require.Contains(t, predicates, "name")
	require.Contains(t, predicates, "friend")
	require.Contains(t, predicates, "age")
}

// gather predicates from functions and filters
func TestGetAllPredicatesFunctions(t *testing.T) {
	query := `
	{
		me(func:anyofterms(name, "Alice")) @filter(le(age, 30)) {
			alias
			friend @filter(eq(school, 5000)) {
				alias
				follow
			}
		}
	}
	`

	subGraphs := getSubGraphs(t, query)

	predicates := GetAllPredicates(subGraphs)
	require.NotNil(t, predicates)
	require.Equal(t, 6, len(predicates))
	require.Contains(t, predicates, "name")
	require.Contains(t, predicates, "age")
	require.Contains(t, predicates, "alias")
	require.Contains(t, predicates, "friend")
	require.Contains(t, predicates, "school")
	require.Contains(t, predicates, "follow")
}

// gather predicates from order
func TestGetAllPredicatesOrdering(t *testing.T) {
	query := `
	{
		me(func:anyofterms(name, "Alice"), orderasc: age) {
			name
			friend(orderdesc: alias) {
				name
			}
		}
	}
	`

	subGraphs := getSubGraphs(t, query)

	predicates := GetAllPredicates(subGraphs)
	require.NotNil(t, predicates)
	require.Equal(t, 4, len(predicates))
	require.Contains(t, predicates, "name")
	require.Contains(t, predicates, "age")
	require.Contains(t, predicates, "friend")
	require.Contains(t, predicates, "alias")
}

// gather predicates from multiple query blocks (and var)
func TestGetAllPredicatesVars(t *testing.T) {
	query := `
	{
		IDS as var(func:anyofterms(name, "Alice"), orderasc: age) {}

		me(id: var(IDS)) {
			alias
		}
	}
	`

	subGraphs := getSubGraphs(t, query)

	predicates := GetAllPredicates(subGraphs)
	require.NotNil(t, predicates)
	require.Equal(t, 3, len(predicates))
	require.Contains(t, predicates, "name")
	require.Contains(t, predicates, "age")
	require.Contains(t, predicates, "alias")
}

// gather predicates from groupby
func TestGetAllPredicatesGroupby(t *testing.T) {
	query := `
	{
		me(id: 1) {
			friend @groupby(age) {
				count(_uid_)
			}
			name
		}
	}
	`

	subGraphs := getSubGraphs(t, query)

	predicates := GetAllPredicates(subGraphs)
	require.NotNil(t, predicates)
	require.Equal(t, 4, len(predicates))
	require.Contains(t, predicates, "_uid_")
	require.Contains(t, predicates, "name")
	require.Contains(t, predicates, "age")
	require.Contains(t, predicates, "friend")
}

func TestMathVarCrash(t *testing.T) {
	populateGraph(t)
	query := `
		{
			f(func: anyofterms(name, "Rick Michonne Andrea")) {
				age as age
				a as math(age *2)
				var(a)
			}
		}
	`
	res, err := gql.Parse(gql.Request{Str: query})
	require.NoError(t, err)

	var l Latency
	ctx := context.Background()
	_, err = ProcessQuery(ctx, res, &l)
	require.Error(t, err)
}

func TestMathVarAlias(t *testing.T) {
	populateGraph(t)
	query := `
		{
			f(func: anyofterms(name, "Rick Michonne Andrea")) {
				ageVar as age
				a: math(ageVar *2)
			}
		}
	`
	js := processToFastJSON(t, query)
	require.JSONEq(t, `{"f":[{"a":76.000000,"age":38},{"a":30.000000,"age":15},{"a":38.000000,"age":19}]}`, string(js))
}

func TestMathVarAlias2(t *testing.T) {
	populateGraph(t)
	query := `
		{
			f as me(func: anyofterms(name, "Rick Michonne Andrea")) {
				ageVar as age
				doubleAge: a as math(ageVar *2)
			}

			me2(id: var(f)) {
				var(a)
			}
		}
	`
	js := processToFastJSON(t, query)
	require.JSONEq(t, `{"me":[{"age":38,"doubleAge":76.000000},{"age":15,"doubleAge":30.000000},{"age":19,"doubleAge":38.000000}],"me2":[{"var(a)":76.000000},{"var(a)":30.000000},{"var(a)":38.000000}]}`, string(js))
}

func TestMathVar3(t *testing.T) {
	populateGraph(t)
	query := `
		{
			f as me(func: anyofterms(name, "Rick Michonne Andrea")) {
				ageVar as age
				a as math(ageVar *2)
			}

			me2(id: var(f)) {
				var(a)
			}
		}
	`
	js := processToFastJSON(t, query)
	require.JSONEq(t, `{"me":[{"age":38,"var(a)":76.000000},{"age":15,"var(a)":30.000000},{"age":19,"var(a)":38.000000}],"me2":[{"var(a)":76.000000},{"var(a)":30.000000},{"var(a)":38.000000}]}`, string(js))
}

func TestMultipleEquality(t *testing.T) {
	populateGraph(t)
	posting.CommitLists(10, 1)
	time.Sleep(100 * time.Millisecond)
	query := `
	{
		me(func: eq(name, ["Rick Grimes"])) {
			name
			friend {
				name
			}
		}
	}


        `
	js := processToFastJSON(t, query)
	require.JSONEq(t, `{"me":[{"friend":[{"name":"Michonne"}],"name":"Rick Grimes"}]}`, js)
}

func TestMultipleEquality2(t *testing.T) {
	populateGraph(t)
	posting.CommitLists(10, 1)
	time.Sleep(100 * time.Millisecond)
	query := `
	{
		me(func: eq(name, ["Badger", "Bobby", "Matt"])) {
			name
			friend {
				name
			}
		}
	}

        `
	js := processToFastJSON(t, query)
	require.JSONEq(t, `{"me":[{"name":"Matt"},{"name":"Badger"}]}`, js)
}

func TestMultipleEquality3(t *testing.T) {
	populateGraph(t)
	posting.CommitLists(10, 1)
	time.Sleep(100 * time.Millisecond)
	query := `
	{
		me(func: eq(dob, ["1910-01-01", "1909-05-05"])) {
			name
			friend {
				name
			}
		}
	}

        `
	js := processToFastJSON(t, query)
	require.JSONEq(t, `{"me":[{"friend":[{"name":"Rick Grimes"},{"name":"Glenn Rhee"},{"name":"Daryl Dixon"},{"name":"Andrea"}],"name":"Michonne"},{"name":"Glenn Rhee"}]}`, js)
}

func TestMultipleEquality4(t *testing.T) {
	populateGraph(t)
	posting.CommitLists(10, 1)
	time.Sleep(100 * time.Millisecond)
	query := `
	{
		me(func: eq(dob, ["1910-01-01", "1909-05-05"])) {
			name
			friend @filter(eq(name, ["Rick Grimes", "Andrea"])) {
				name
			}
		}
	}

        `
	js := processToFastJSON(t, query)
	require.JSONEq(t, `{"me":[{"friend":[{"name":"Rick Grimes"},{"name":"Andrea"}],"name":"Michonne"},{"name":"Glenn Rhee"}]}`, js)
}

func TestMultipleGtError(t *testing.T) {
	populateGraph(t)
	query := `
	{
		me(func: gt(name, ["Badger", "Bobby"])) {
			name
			friend {
				name
			}
		}
	}

  `
	res, err := gql.Parse(gql.Request{Str: query})
	require.NoError(t, err)

	var l Latency
	ctx := context.Background()
	_, err = ProcessQuery(ctx, res, &l)
	require.Error(t, err)
}

func TestMultipleEqQuote(t *testing.T) {
	populateGraph(t)
	query := `
	{
		me(func: eq(name, ["Alice\"", "Michonne"])) {
			name
			friend {
				name
			}
		}
	}
`
	js := processToFastJSON(t, query)
	require.JSONEq(t, `{"me":[{"friend":[{"name":"Rick Grimes"},{"name":"Glenn Rhee"},{"name":"Daryl Dixon"},{"name":"Andrea"}],"name":"Michonne"},{"name":"Alice\""}]}`, js)
}

func TestMultipleEqInt(t *testing.T) {
	populateGraph(t)
	query := `
	{
		me(func: eq(age, [15, 17, 38])) {
			name
			friend {
				name
			}
		}
	}
`
	js := processToFastJSON(t, query)
	require.JSONEq(t, `{"me":[{"friend":[{"name":"Rick Grimes"},{"name":"Glenn Rhee"},{"name":"Daryl Dixon"},{"name":"Andrea"}],"name":"Michonne"},{"friend":[{"name":"Michonne"}],"name":"Rick Grimes"},{"name":"Glenn Rhee"},{"friend":[{"name":"Glenn Rhee"}],"name":"Daryl Dixon"}]}`, js)
}
