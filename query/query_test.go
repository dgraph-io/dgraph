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

	farm "github.com/dgryski/go-farm"
	"github.com/gogo/protobuf/proto"
	"github.com/stretchr/testify/require"
	geom "github.com/twpayne/go-geom"

	"github.com/dgraph-io/dgraph/algo"
	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/group"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/graphp"
	"github.com/dgraph-io/dgraph/protos/taskp"
	"github.com/dgraph-io/dgraph/protos/typesp"

	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/store"
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

var ps *store.Store

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
	addEdgeToValue(t, "_xid_", 24, "g\"lenn", nil)
	src.Value = []byte(`{"Type":"Point", "Coordinates":[1.10001,2.000001]}`)
	coord, err = types.Convert(src, types.GeoID)
	require.NoError(t, err)
	gData = types.ValueForType(types.BinaryID)
	err = types.Marshal(coord, &gData)
	require.NoError(t, err)
	addEdgeToTypedValue(t, "loc", 24, types.GeoID, gData.Value.([]byte), nil)

	addEdgeToValue(t, "name", farm.Fingerprint64([]byte("a.bc")), "Alice", nil)
	addEdgeToValue(t, "name", 25, "Daryl Dixon", nil)
	addEdgeToValue(t, "name", 31, "Andrea", nil)
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

	// language stuff
	// 0x1001 is uid of interest for language tests
	addEdgeToLangValue(t, "name", 0x1001, "Badger", "", nil)
	addEdgeToLangValue(t, "name", 0x1001, "European badger", "en", nil)
	addEdgeToLangValue(t, "name", 0x1001, "Borsuk europejski", "pl", nil)
	addEdgeToLangValue(t, "name", 0x1001, "Europäischer Dachs", "de", nil)
	addEdgeToLangValue(t, "name", 0x1001, "Барсук", "ru", nil)
	addEdgeToLangValue(t, "name", 0x1001, "Blaireau européen", "fr", nil)

	addEdgeToValue(t, "name", 240, "Andrea With no friends", nil)
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
	res, err := gql.Parse(query)
	require.NoError(t, err)

	var l Latency
	ctx := context.Background()
	sgl, err := ProcessQuery(ctx, res, &l)
	require.NoError(t, err)

	var buf bytes.Buffer
	mp := map[string]string{
		"a": "123",
	}
	require.NoError(t, ToJson(&l, sgl, &buf, mp))
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

func TestQueryVarValAggMinMaxSelf(t *testing.T) {
	populateGraph(t)
	query := `
		{
			f as var(func: anyofterms(name, "Michonne Andrea Rick")) {
				a as age
				friend {
					n as min(age)
					s as max(age)
					sum as sumvar(n, a, s)
				}
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
		`{"me":[{"name":"Andrea","var[s]":15,"var[sum]":49},{"name":"Michonne","var[s]":19,"var[sum]":72},{"name":"Rick Grimes","var[s]":38,"var[sum]":91}]}`,
		js)
}

func TestQueryVarValAggMinMax(t *testing.T) {
	populateGraph(t)
	query := `
		{
			f as var(func: anyofterms(name, "Michonne Andrea Rick")) {
				friend {
					n as min(age)
					s as max(age)
					sum as sumvar(n, s)
				}
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
		`{"me":[{"name":"Rick Grimes","var[n]":38,"var[s]":38},{"name":"Michonne","var[n]":15,"var[s]":19},{"name":"Andrea","var[n]":15,"var[s]":15}]}`,
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
					sum as sumvar(n, s)
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
		`{"info":[{"friend":[{"age":15,"friend":[{"count":1}],"sumvar[n s]":16},{"age":15,"friend":[{"count":0}],"sumvar[n s]":15},{"age":17,"friend":[{"count":0}],"sumvar[n s]":17},{"age":19,"friend":[{"count":1}],"sumvar[n s]":20},{"friend":[{"count":0}]}]}],"me":[{"age":19,"friend":[{"count":1}],"name":"Andrea"},{"age":17,"friend":[{"count":0}],"name":"Daryl Dixon"},{"age":15,"friend":[{"count":1}],"name":"Rick Grimes"},{"age":15,"friend":[{"count":0}],"name":"Glenn Rhee"}]}`,
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
					sum as sumvar(n, s)
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

func TestUseVarsFilterVarReuse3(t *testing.T) {
	populateGraph(t)
	query := `
		{
			var(id:0x01) {
				fr as friend(first:2, offset:2, orderasc: dob)
			}

			friend(id:0x01) {
				L as friend {
					friend {
						name
						friend @filter(var(L) and var(fr)) {
							name
						}
					}
				}
			}
		}
	`
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"friend":[{"friend":[{"friend":[{"name":"Michonne", "friend":[{"name":"Rick Grimes"},{"name":"Glenn Rhee"}]}]}, {"friend":[{"name":"Glenn Rhee"}]}]}]}`,
		js)
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
			me(func: geq(count(friend), 1)) {
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
	res, err := gql.Parse(query)
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

func TestShortestPath3(t *testing.T) {
	populateGraph(t)
	query := `
		{
			A as shortest(from:1, to:1003) {
				path 
			}

			me(id: var( A)) {
				name
			}
		}`
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"_path_":[{"_uid_":"0x1","path":[{"_uid_":"0x1f","path":[{"_uid_":"0x3e8","path":[{"_uid_":"0x3ea","path":[{"_uid_":"0x3eb"}]}]}]}]}],"me":[{"name":"Michonne"},{"name":"Andrea"},{"name":"Alice"},{"name":"Matt"},{"name":"John"}]}`,
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
		`{"me":[{"_uid_":"0x1","alive":true,"friend":[{"count":5}],"gender":"female","name":"Michonne"}]}`,
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
	res, err := gql.Parse(query)
	require.NoError(t, err)

	var l Latency
	ctx := context.Background()
	ctx = context.WithValue(ctx, "debug", "true")
	sgl, err := ProcessQuery(ctx, res, &l)
	require.NoError(t, err)

	var buf bytes.Buffer
	require.NoError(t, ToJson(&l, sgl, &buf, nil))

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
			me(id: [1, 24]) @filter(geq(dob, "1910-01-01")) {
				name
			}
		}
	`
	res, err := gql.Parse(query)
	require.NoError(t, err)

	var l Latency
	ctx := context.Background()
	ctx = context.WithValue(ctx, "debug", "true")
	sgl, err := ProcessQuery(ctx, res, &l)
	require.NoError(t, err)

	var buf bytes.Buffer
	require.NoError(t, ToJson(&l, sgl, &buf, nil))

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
		`{"me":[{"alive":true,"friend":[{"count":5}],"gender":"female","name":"Michonne"}]}`,
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
	_, err := gql.Parse(query)
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
	_, err := gql.Parse(query)
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
	_, err := gql.Parse(query)
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
		`{"countorder":[{"friend":[{"count":0}],"name":"Andrea With no friends"},{"friend":[{"count":1}],"name":"Rick Grimes"},{"friend":[{"count":1}],"name":"Andrea"},{"friend":[{"count":5}],"name":"Michonne"}]}`,
		js)
}

func TestMultiAggSort(t *testing.T) {
	populateGraph(t)
	// Alright. Now we have everything set up. Let's create the query.
	query := `
	{
		f as var(func: anyofterms(name, "michonne rick andrea")) {
			name
			friend {
				mindob as min(dob)
				maxdob as max(dob)
				dob
			}
		}

		maxorder(id: var(f), orderasc: var(maxdob)) {
			name
			friend {
				max(dob)
			}
		}

		minorder(id: var(f), orderasc: var(mindob)) {
			name
			friend {
				min(dob)
			}
		}
	}
`
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"maxorder":[{"friend":[{"max(dob)":"1909-05-05"}],"name":"Andrea"},{"friend":[{"max(dob)":"1910-01-01"}],"name":"Rick Grimes"},{"friend":[{"max(dob)":"1910-01-02"}],"name":"Michonne"}],"minorder":[{"friend":[{"min(dob)":"1901-01-15"}],"name":"Michonne"},{"friend":[{"min(dob)":"1909-05-05"}],"name":"Andrea"},{"friend":[{"min(dob)":"1910-01-01"}],"name":"Rick Grimes"}]}`,
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
				min(dob)
				max(dob)
				dob
			}
		}
	}
`
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"friend":[{"dob":"1910-01-02"},{"dob":"1909-05-05"},{"dob":"1909-01-10"},{"dob":"1901-01-15"},{"min(dob)":"1901-01-15"},{"max(dob)":"1910-01-02"}],"name":"Michonne"},{"friend":[{"dob":"1910-01-01"},{"min(dob)":"1910-01-01"},{"max(dob)":"1910-01-01"}],"name":"Rick Grimes"},{"friend":[{"dob":"1909-05-05"},{"min(dob)":"1909-05-05"},{"max(dob)":"1909-05-05"}],"name":"Andrea"},{"name":"Andrea With no friends"}]}`,
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
				min(dob)
				max(dob)
			}
		}
	}
`
	pb := processToPB(t, query, false)
	res := `attribute: "_root_"
children: <
  attribute: "me"
  properties: <
    prop: "name"
    value: <
      str_val: "Michonne"
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
      prop: "min(dob)"
      value: <
        str_val: "1901-01-15T00:00:00Z"
      >
    >
  >
  children: <
    attribute: "friend"
    properties: <
      prop: "max(dob)"
      value: <
        str_val: "1910-01-02T00:00:00Z"
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
  children: <
    attribute: "friend"
    properties: <
      prop: "name"
      value: <
        str_val: "Michonne"
      >
    >
  >
  children: <
    attribute: "friend"
    properties: <
      prop: "min(dob)"
      value: <
        str_val: "1910-01-01T00:00:00Z"
      >
    >
  >
  children: <
    attribute: "friend"
    properties: <
      prop: "max(dob)"
      value: <
        str_val: "1910-01-01T00:00:00Z"
      >
    >
  >
>
`
	require.Equal(t, res, proto.MarshalTextString(pb[0]))
}

func TestMin(t *testing.T) {
	populateGraph(t)
	// Alright. Now we have everything set up. Let's create the query.
	query := `
	{
		me(id:0x01) {
			name
			friend {
				min(dob)
			}
		}
	}
`
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"friend":[{"min(dob)":"1901-01-15"}],"name":"Michonne"}]}`,
		js)
}

func TestMinError1(t *testing.T) {
	populateGraph(t)
	// error: min could not performed on non-scalar-type
	query := `
                {
                        me(id:0x01) {
                                name
                                gender
                                alive
                                min(friend)
                        }
                }
        `
	res, err := gql.Parse(query)
	require.NoError(t, err)

	var l Latency
	_, queryErr := ProcessQuery(context.Background(), res, &l)
	require.NotNil(t, queryErr)
}

func TestMinError2(t *testing.T) {
	populateGraph(t)
	// error: min should not have children
	query := `
                {
                        me(id:0x01) {
                                name
                                gender
                                alive
                                min(friend) {
                                    name
                                }
                        }
                }
        `
	_, err := gql.Parse(query)
	require.Error(t, err)
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
                                    min(survival_rate)
                                }
                        }
                }
        `
	js := processToFastJSON(t, query)
	require.EqualValues(t,
		`{"me":[{"alive":true,"friend":[{"min(survival_rate)":1.600000}],"gender":"female","name":"Michonne"}]}`,
		js)

	schema.State().Set("survival_rate", typesp.Schema{ValueType: uint32(types.IntID)})
	js = processToFastJSON(t, query)
	require.EqualValues(t,
		`{"me":[{"alive":true,"friend":[{"min(survival_rate)":1}],"gender":"female","name":"Michonne"}]}`,
		js)
	schema.State().Set("survival_rate", typesp.Schema{ValueType: uint32(types.FloatID)})
}

func TestMax(t *testing.T) {
	populateGraph(t)
	// Alright. Now we have everything set up. Let's create the query.
	query := `
                {
                        me(id:0x01) {
                                name
                                gender
                                alive
                                friend {
                                    max(dob)
                                }
                        }
                }
        `
	js := processToFastJSON(t, query)
	require.EqualValues(t,
		`{"me":[{"alive":true,"friend":[{"max(dob)":"1910-01-02"}],"gender":"female","name":"Michonne"}]}`,
		js)
}

func TestMaxError1(t *testing.T) {
	populateGraph(t)
	// error: aggregator 'max' should not have filters on its own
	query := `
                {
                        me(id:0x01) {
                                name
                                gender
                                alive
                                friend {
                                    max(dob @filter(gt(dob, "1910-01-02")))
                                }
                        }
                }
        `
	_, err := gql.Parse(query)
	require.Error(t, err)
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
				avg(shadow_deep)
			}
		}
	}
`
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"alive":true,"friend":[{"avg(shadow_deep)":9}],"gender":"female","name":"Michonne"}]}`,
		js)
}

func TestAvgError(t *testing.T) {
	populateGraph(t)
	query := `
	{
		me(id:0x01) {
			name
			gender
			alive
			friend {
				avg(name)
			}
		}
	}
`

	_, err := processToFastJsonReq(t, query)
	require.Error(t, err)
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
                                    sum(shadow_deep)
                                }
                        }
                }
        `
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"alive":true,"friend":[{"sum(shadow_deep)":18}],"gender":"female","name":"Michonne"}]}`,
		js)
}

func TestSumError1(t *testing.T) {
	populateGraph(t)
	// error: sum could only be applied on int/float
	query := `
                {
                        me(id:0x01) {
                                name
                                gender
                                alive
                                friend {
                                    sum(name)
                                }
                        }
                }
        `
	res, err := gql.Parse(query)
	require.NoError(t, err)

	var l Latency
	_, queryErr := ProcessQuery(context.Background(), res, &l)
	require.NotNil(t, queryErr)
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
	res, err := gql.Parse(query)
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
	res, err := gql.Parse(query)
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
	res, err := gql.Parse(query)
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
	res, err := gql.Parse(query)
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
	res, err := gql.Parse(query)
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
                                friend(disorderasc: dob) @filter(leq(dob, "1909-03-20")) {
                                        name
                                }
                        }
                }
        `
	res, err := gql.Parse(query)
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
	res, err := gql.Parse(query)
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
	res, err := gql.Parse(query)
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
	pb := processToPB(t, query, false)
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
	_, err := gql.Parse(query)
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

func TestFilterRegexError(t *testing.T) {
	populateGraph(t)
	query := `
    {
      me(id:0x01) {
        name
        friend @filter(regexp(dob, "^[a-z A-Z]+$")) {
          name
        }
      }
    }
  `

	res, err := gql.Parse(query)
	require.NoError(t, err)

	ctx := context.Background()
	sg, err := ToSubGraph(ctx, res.Query[0])
	require.NoError(t, err)
	sg.DebugPrint("")

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
        friend @filter(regexp(name, "^[a-z A-Z]+$")) {
          name
        }
      }
    }
  `

	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"name":"Michonne", "friend":[{"name":"Rick Grimes"},{"name":"Glenn Rhee"},{"name":"Daryl Dixon"}, {"name":"Andrea"}]}]}`, js)
}

func TestFilterRegex2(t *testing.T) {
	populateGraph(t)
	query := `
    {
      me(id:0x01) {
        name
        friend @filter(regexp(name, "^[^ao]+$")) {
          name
        }
      }
    }
  `

	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"name":"Michonne", "friend":[{"name":"Rick Grimes"},{"name":"Glenn Rhee"}]}]}`, js)
}

func TestFilterRegex3(t *testing.T) {
	populateGraph(t)
	query := `
    {
      me(id:0x01) {
        name
        friend @filter(regexp(name, "^(Ri)")) {
          name
        }
      }
    }
  `

	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"name":"Michonne", "friend":[{"name":"Rick Grimes"}]}]}`, js)
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
		`{"me":[{"friend":[{"count":2}, {"name":"Andrea"}],"gender":"female","name":"Michonne"}]}`,
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

func TestToFastJSONFilterGeqName(t *testing.T) {
	populateGraph(t)
	query := `
		{
			me(id:0x01) {
				friend @filter(geq(name, "Rick")) {
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

func TestToFastJSONFilterGeq(t *testing.T) {
	populateGraph(t)
	query := `
		{
			me(id:0x01) {
				name
				gender
				friend @filter(geq(dob, "1909-05-05")) {
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

func TestToFastJSONFilterLeq(t *testing.T) {
	populateGraph(t)
	query := `
		{
			me(id:0x01) {
				name
				gender
				friend @filter(leq(dob, "1909-01-10")) {
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
	res, err := gql.Parse(query)
	require.NoError(t, err)

	ctx := context.Background()
	sg, err := ToSubGraph(ctx, res.Query[0])
	require.NoError(t, err)
	sg.DebugPrint("")

	ch := make(chan error)
	go ProcessGraph(ctx, sg, nil, ch)
	err = <-ch
	require.Error(t, err)
}

func TestToFastJSONFilterLeqOrder(t *testing.T) {
	populateGraph(t)
	query := `
		{
			me(id:0x01) {
				name
				gender
				friend(orderasc: dob) @filter(leq(dob, "1909-03-20")) {
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

func TestToFastJSONFilterGeqNoResult(t *testing.T) {
	populateGraph(t)
	query := `
		{
			me(id:0x01) {
				name
				gender
				friend @filter(geq(dob, "1999-03-20")) {
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

func TestToFastJSONFilterLeqFirstOffset(t *testing.T) {
	populateGraph(t)
	query := `
		{
			me(id:0x01) {
				name
				gender
				friend(offset:1, first:1) @filter(leq(dob, "1909-03-20")) {
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
		`{"me":[{"friend":[{"count":1}],"gender":"female","name":"Michonne"}]}`,
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
			me(func: geq(count(~friend), 2)) {
				name
				count(~friend)
			}
		}
	`
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"name":"Glenn Rhee","~friend":[{"count":2}]}]}`,
		js)
}

func TestCountReverseFilter(t *testing.T) {
	populateGraph(t)
	query := `
		{
			me(func: anyofterms(name, "Glenn Michonne Rick")) @filter(geq(count(~friend), 2)) {
				name
				count(~friend)
			}
		}
	`
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"name":"Glenn Rhee","~friend":[{"count":2}]}]}`,
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
		`{"me":[{"name":"Glenn Rhee","~friend":[{"count":2}]}]}`,
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
		`{"me":[{"name":"Glenn Rhee","~friend":[{"count":2}]}]}`,
		js)
}

func getProperty(properties []*graphp.Property, prop string) *graphp.Value {
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
	pb := processToPB(t, query, true)
	require.EqualValues(t,
		`attribute: "_root_"
children: <
  uid: 1
  xid: "mich"
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
  properties: <
    prop: "alive"
    value: <
      bool_val: true
    >
  >
  children: <
    uid: 23
    attribute: "friend"
    properties: <
      prop: "name"
      value: <
        str_val: "Rick Grimes"
      >
    >
  >
  children: <
    uid: 24
    attribute: "friend"
    properties: <
      prop: "name"
      value: <
        str_val: "Glenn Rhee"
      >
    >
  >
  children: <
    uid: 25
    attribute: "friend"
    properties: <
      prop: "name"
      value: <
        str_val: "Daryl Dixon"
      >
    >
  >
  children: <
    uid: 31
    attribute: "friend"
    properties: <
      prop: "name"
      value: <
        str_val: "Andrea"
      >
    >
  >
  children: <
    uid: 101
    attribute: "friend"
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

	pb := processToPB(t, query, false)
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

	pb := processToPB(t, query, false)
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

	pb := processToPB(t, query, false)
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
		`{"me":[{"friend":[{"count":1}],"gender":"female","name":"Michonne"}]}`,
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
func ageSg(uidMatrix []*taskp.List, srcUids *taskp.List, ages []uint64) *SubGraph {
	var as []*taskp.Value
	for _, a := range ages {
		bs := make([]byte, 4)
		binary.LittleEndian.PutUint64(bs, a)
		as = append(as, &taskp.Value{[]byte(bs), 2})
	}

	return &SubGraph{
		Attr:      "age",
		uidMatrix: uidMatrix,
		SrcUIDs:   srcUids,
		values:    as,
		Params:    params{isDebug: false, GetUID: true},
	}
}
func nameSg(uidMatrix []*taskp.List, srcUids *taskp.List, names []string) *SubGraph {
	var ns []*taskp.Value
	for _, n := range names {
		ns = append(ns, &taskp.Value{[]byte(n), 0})
	}
	return &SubGraph{
		Attr:      "name",
		uidMatrix: uidMatrix,
		SrcUIDs:   srcUids,
		values:    ns,
		Params:    params{isDebug: false, GetUID: true},
	}

}
func friendsSg(uidMatrix []*taskp.List, srcUids *taskp.List, friends []*SubGraph) *SubGraph {
	return &SubGraph{
		Attr:      "friend",
		uidMatrix: uidMatrix,
		SrcUIDs:   srcUids,
		Params:    params{isDebug: false, GetUID: true},
		Children:  friends,
	}
}
func rootSg(uidMatrix []*taskp.List, srcUids *taskp.List, names []string, ages []uint64) *SubGraph {
	nameSg := nameSg(uidMatrix, srcUids, names)
	ageSg := ageSg(uidMatrix, srcUids, ages)

	return &SubGraph{
		Children:  []*SubGraph{nameSg, ageSg},
		Params:    params{isDebug: false, GetUID: true},
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

	pb := processToPB(t, query, false)
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

	pb := processToPB(t, query, false)
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

	pb := processToPB(t, query, false)
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
	_, err := gql.Parse(query)
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
	_, err := gql.Parse(query)
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
			me(func:anyofterms(name, "Daryl Rick Glenn")) @filter(leq(dob, 1909-01-10)) {
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
			me(func:anyofterms(name, "Michonne Rick Glenn")) @filter(geq(dob, 1909-01-10)) {
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
			me(func:anyofterms(name, "Michonne Rick Glenn")) @filter(anyofterms(name, "Glenn") and geq(dob, 1909-01-10)) {
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
	_, err := gql.Parse(query)
	require.NoError(t, err)

	js := processToFastJSON(t, query)
	require.JSONEq(t, `{"me":[{"name":"Michonne"}]}`, js)
}

func TestGeneratorRootFilterOnCountLeq(t *testing.T) {
	populateGraph(t)
	query := `
                {
                        me(func:anyofterms(name, "Michonne Rick")) @filter(leq(count(friend), 2)) {
                                name
                        }
                }
        `
	_, err := gql.Parse(query)
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
	_, err := gql.Parse(query)
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
	_, err := gql.Parse(query)
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
	res, err := gql.Parse(query)
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
	res, err := gql.Parse(query)
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
	res, err := gql.Parse(query)
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

	pb := processToPB(t, query, false)
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

	res, err := gql.Parse(query)
	require.NoError(t, err)

	ctx := context.Background()
	sg, err := ToSubGraph(ctx, res.Query[0])
	require.NoError(t, err)
	sg.DebugPrint("")

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

	res, err := gql.Parse(query)
	require.NoError(t, err)

	ctx := context.Background()
	sg, err := ToSubGraph(ctx, res.Query[0])
	require.NoError(t, err)
	sg.DebugPrint("")

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

	res, err := gql.Parse(query)
	require.NoError(t, err)

	ctx := context.Background()
	sg, err := ToSubGraph(ctx, res.Query[0])
	require.NoError(t, err)
	sg.DebugPrint("")

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

	res, err := gql.Parse(query)
	require.NoError(t, err)

	ctx := context.Background()
	sg, err := ToSubGraph(ctx, res.Query[0])
	require.NoError(t, err)
	sg.DebugPrint("")

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
					dob
					friend {
						fn : name
					}
				}
			}
		}
	`
	pb := processToPB(t, query, false)
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
    prop: "fn"
    value: <
      str_val: "Michonne"
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
    prop: "fn"
    value: <
      str_val: "Glenn Rhee"
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
    prop: "fn"
    value: <
      str_val: "Glenn Rhee"
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
					dob
					friend {
						fn : name
					}
				}
			}
		}
	`

	js := processToFastJSON(t, query)
	require.EqualValues(t,
		`{"me":[{"fn":"Michonne","mn":"Michonne","n":"Rick Grimes"},{"mn":"Michonne","n":"Glenn Rhee"},{"fn":"Glenn Rhee","mn":"Michonne","n":"Daryl Dixon"},{"fn":"Glenn Rhee","mn":"Michonne","n":"Andrea"}]}`,
		js)
}

func TestSchema(t *testing.T) {
	populateGraph(t)
	query := `
		{
			debug(id:0x1) {
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
	gr := processToPB(t, query, true)
	require.EqualValues(t, "debug", gr[0].Children[0].Attribute)
	require.EqualValues(t, 1, gr[0].Children[0].Uid)
	require.EqualValues(t, "mich", gr[0].Children[0].Xid)
	require.Len(t, gr[0].Children[0].Properties, 4)

	require.EqualValues(t, "Michonne",
		getProperty(gr[0].Children[0].Properties, "name").GetStrVal())

	g := types.ValueForType(types.GeoID)
	g.Value = getProperty(gr[0].Children[0].Properties, "loc").GetGeoVal()
	g1, err := types.Convert(g, types.StringID)
	x.Check(err)
	require.EqualValues(t, "{'type':'Point','coordinates':[1.1,2]}", string(g1.Value.(string)))

	require.Len(t, gr[0].Children[0].Children, 5)

	child := gr[0].Children[0].Children[0]
	require.EqualValues(t, 23, child.Uid)
	require.EqualValues(t, "friend", child.Attribute)

	require.Len(t, child.Properties, 2)
	require.EqualValues(t, "Rick Grimes",
		getProperty(child.Properties, "name").GetStrVal())
	dob := getProperty(child.Properties, "dob").GetStrVal()
	date, err := time.Parse(time.RFC3339, dob)
	require.NoError(t, err)
	require.EqualValues(t, "1910-01-02 00:00:00 +0000 UTC", date.String())
	require.Empty(t, child.Children)

	child = gr[0].Children[0].Children[4]
	require.EqualValues(t, 101, child.Uid)
	require.EqualValues(t, "friend", child.Attribute)
	require.Empty(t, child.Properties)
	require.Empty(t, child.Children)
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
	err = sg.ToFastJSON(&l, &buf, nil)
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

func checkSchemaNodes(t *testing.T, expected []*graphp.SchemaNode, actual []*graphp.SchemaNode) {
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
	expected := []*graphp.SchemaNode{{Predicate: "genre", Type: "uid"},
		{Predicate: "age", Type: "int"}, {Predicate: "name", Type: "string"},
		{Predicate: "film.film.initial_release_date", Type: "date"},
		{Predicate: "loc", Type: "geo"}, {Predicate: "alive", Type: "bool"},
		{Predicate: "shadow_deep", Type: "int"}, {Predicate: "friend", Type: "uid"},
		{Predicate: "geometry", Type: "geo"}, {Predicate: "alias", Type: "string"},
		{Predicate: "dob", Type: "date"}, {Predicate: "survival_rate", Type: "float"}}
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
	expected := []*graphp.SchemaNode{
		{Predicate: "name", Type: "string", Index: true, Tokenizer: []string{"term", "exact"}}}
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
	expected := []*graphp.SchemaNode{{Predicate: "age", Type: "int"}}
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
	expected := []*graphp.SchemaNode{
		{Predicate: "genre", Type: "uid", Reverse: true}, {Predicate: "age", Type: "int"}}
	checkSchemaNodes(t, expected, actual)
}

func TestSchemaBlock5(t *testing.T) {
	query := `
		schema(pred: name) {
		}
	`
	actual := processSchemaQuery(t, query)
	expected := []*graphp.SchemaNode{
		{Predicate: "name", Type: "string", Index: true, Tokenizer: []string{"term", "exact"}}}
	checkSchemaNodes(t, expected, actual)
}

const schemaStr = `
name:string @index(term, exact) .
alias:string @index(exact, term, fulltext) .
dob:date @index .
film.film.initial_release_date:date @index .
loc:geo @index .
genre:uid @reverse .
survival_rate : float .
alive         : bool @index .
age           : int .
shadow_deep   : int .
friend:uid @reverse .
geometry:geo @index .
`

func TestMain(m *testing.M) {
	x.SetTestRun()
	x.Init()

	dir, err := ioutil.TempDir("", "storetest_")
	x.Check(err)
	defer os.RemoveAll(dir)

	ps, err = store.NewStore(dir)
	x.Check(err)
	defer ps.Close()

	group.ParseGroupConfig("")
	schema.Init(ps)
	posting.Init(ps)
	worker.Init(ps)

	dir2, err := ioutil.TempDir("", "wal_")
	x.Check(err)

	worker.StartRaftNodes(dir2)
	// Load schema after nodes have started
	schema.ParseBytes([]byte(schemaStr), 1)
	defer os.RemoveAll(dir2)

	os.Exit(m.Run())
}

func TestFilterNonIndexedPredicateFail(t *testing.T) {
	populateGraph(t)
	// filtering on non indexing predicate fails
	query := `
		{
			me(id:0x01) {
				friend @filter(leq(age, 30)) {
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
	pb := processToPB(t, query, false)
	expectedPb := `attribute: "_root_"
children: <
  xid: "mich"
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
    xid: "g\"lenn"
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

func TestBoolIndexGeqRoot(t *testing.T) {
	populateGraph(t)
	q := `
		{
			me(func: geq(alive, true)) {
				name
				alive
				friend {
					name
					alive
				}
			}
		}`
	res, _ := gql.Parse(q)
	var l Latency
	ctx := context.Background()
	_, err := ProcessQuery(ctx, res, &l)
	require.Equal(t, "Only eq operator defined for type bool. Got: geq", err.Error())
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
	res, _ := gql.Parse(q)
	var l Latency
	ctx := context.Background()
	_, err := ProcessQuery(ctx, res, &l)
	require.NotNil(t, err)
}
