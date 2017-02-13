/*
 * Copyright 2015 DGraph Labs, Inc.
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
	"encoding/binary"
	"encoding/json"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"google.golang.org/grpc/metadata"

	farm "github.com/dgryski/go-farm"
	"github.com/gogo/protobuf/proto"
	"github.com/stretchr/testify/require"
	geom "github.com/twpayne/go-geom"

	"github.com/dgraph-io/dgraph/algo"
	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/group"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/query/graph"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/store"
	"github.com/dgraph-io/dgraph/task"
	"github.com/dgraph-io/dgraph/types"
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

func taskValues(t *testing.T, v []*task.Value) []string {
	out := make([]string, len(v))
	for i, tv := range v {
		out[i] = string(tv.Val)
	}
	return out
}

func addEdgeToValue(t *testing.T, attr string, src uint64,
	value string) {
	edge := &task.DirectedEdge{
		Value:  []byte(value),
		Label:  "testing",
		Attr:   attr,
		Entity: src,
		Op:     task.DirectedEdge_SET,
	}
	l, _ := posting.GetOrCreate(x.DataKey(attr, src), 0)
	require.NoError(t,
		l.AddMutationWithIndex(context.Background(), edge))
}

func addEdgeToTypedValue(t *testing.T, attr string, src uint64,
	typ types.TypeID, value []byte) {
	edge := &task.DirectedEdge{
		Value:     value,
		ValueType: uint32(typ),
		Label:     "testing",
		Attr:      attr,
		Entity:    src,
		Op:        task.DirectedEdge_SET,
	}
	l, _ := posting.GetOrCreate(x.DataKey(attr, src), 0)
	require.NoError(t,
		l.AddMutationWithIndex(context.Background(), edge))
}

func addEdgeToUID(t *testing.T, attr string, src uint64, dst uint64) {
	edge := &task.DirectedEdge{
		ValueId: dst,
		Label:   "testing",
		Attr:    attr,
		Entity:  src,
		Op:      task.DirectedEdge_SET,
	}
	l, _ := posting.GetOrCreate(x.DataKey(attr, src), 0)
	require.NoError(t,
		l.AddMutationWithIndex(context.Background(), edge))
}

func delEdgeToUID(t *testing.T, attr string, src uint64, dst uint64) {
	edge := &task.DirectedEdge{
		ValueId: dst,
		Label:   "testing",
		Attr:    attr,
		Entity:  src,
		Op:      task.DirectedEdge_DEL,
	}
	l, _ := posting.GetOrCreate(x.DataKey(attr, src), 0)
	require.NoError(t,
		l.AddMutationWithIndex(context.Background(), edge))
}

func addGeoData(t *testing.T, ps *store.Store, uid uint64, p geom.T, name string) {
	value := types.ValueForType(types.BinaryID)
	src := types.ValueForType(types.GeoID)
	src.Value = p
	err := types.Marshal(src, &value)
	require.NoError(t, err)
	addEdgeToTypedValue(t, "geometry", uid, types.GeoID, value.Value.([]byte))
	addEdgeToTypedValue(t, "name", uid, types.StringID, []byte(name))
}

var ps *store.Store

func populateGraph(t *testing.T) {
	x.AssertTrue(ps != nil)
	// logrus.SetLevel(logrus.DebugLevel)
	// So, user we're interested in has uid: 1.
	// She has 5 friends: 23, 24, 25, 31, and 101
	addEdgeToUID(t, "friend", 1, 23)
	addEdgeToUID(t, "friend", 1, 24)
	addEdgeToUID(t, "friend", 1, 25)
	addEdgeToUID(t, "friend", 1, 31)
	addEdgeToUID(t, "friend", 1, 101)
	addEdgeToUID(t, "friend", 31, 24)
	addEdgeToUID(t, "friend", 23, 1)

	// Now let's add a few properties for the main user.
	addEdgeToValue(t, "name", 1, "Michonne")
	addEdgeToValue(t, "gender", 1, "female")

	src := types.ValueForType(types.StringID)
	src.Value = []byte("{\"Type\":\"Point\", \"Coordinates\":[1.1,2.0]}")
	coord, err := types.Convert(src, types.GeoID)
	require.NoError(t, err)
	gData := types.ValueForType(types.BinaryID)
	err = types.Marshal(coord, &gData)
	require.NoError(t, err)
	addEdgeToTypedValue(t, "loc", 1, types.GeoID, gData.Value.([]byte))

	// Int32ID
	data := types.ValueForType(types.BinaryID)
	intD := types.Val{types.Int32ID, int32(15)}
	err = types.Marshal(intD, &data)
	require.NoError(t, err)
	addEdgeToTypedValue(t, "age", 1, types.Int32ID, data.Value.([]byte))

	// FloatID
	fdata := types.ValueForType(types.BinaryID)
	floatD := types.Val{types.FloatID, float64(13.25)}
	err = types.Marshal(floatD, &fdata)
	require.NoError(t, err)
	addEdgeToTypedValue(t, "power", 1, types.FloatID, fdata.Value.([]byte))

	addEdgeToValue(t, "address", 1, "31, 32 street, Jupiter")

	boolD := types.Val{types.BoolID, true}
	err = types.Marshal(boolD, &data)
	require.NoError(t, err)
	addEdgeToTypedValue(t, "alive", 1, types.BoolID, data.Value.([]byte))
	addEdgeToValue(t, "age", 1, "38")
	addEdgeToValue(t, "survival_rate", 1, "98.99")
	addEdgeToValue(t, "sword_present", 1, "true")
	addEdgeToValue(t, "_xid_", 1, "mich")

	// Now let's add a name for each of the friends, except 101.
	addEdgeToTypedValue(t, "name", 23, types.StringID, []byte("Rick Grimes"))
	addEdgeToValue(t, "age", 23, "15")

	src.Value = []byte(`{"Type":"Polygon", "Coordinates":[[[0.0,0.0], [2.0,0.0], [2.0, 2.0], [0.0, 2.0], [0.0, 0.0]]]}`)
	coord, err = types.Convert(src, types.GeoID)
	require.NoError(t, err)
	gData = types.ValueForType(types.BinaryID)
	err = types.Marshal(coord, &gData)
	require.NoError(t, err)
	addEdgeToTypedValue(t, "loc", 23, types.GeoID, gData.Value.([]byte))

	addEdgeToValue(t, "address", 23, "21, mark street, Mars")
	addEdgeToValue(t, "name", 24, "Glenn Rhee")
	src.Value = []byte(`{"Type":"Point", "Coordinates":[1.10001,2.000001]}`)
	coord, err = types.Convert(src, types.GeoID)
	require.NoError(t, err)
	gData = types.ValueForType(types.BinaryID)
	err = types.Marshal(coord, &gData)
	require.NoError(t, err)
	addEdgeToTypedValue(t, "loc", 24, types.GeoID, gData.Value.([]byte))

	addEdgeToValue(t, "name", farm.Fingerprint64([]byte("a.bc")), "Alice")
	addEdgeToValue(t, "name", 25, "Daryl Dixon")
	addEdgeToValue(t, "name", 31, "Andrea")
	src.Value = []byte(`{"Type":"Point", "Coordinates":[2.0, 2.0]}`)
	coord, err = types.Convert(src, types.GeoID)
	require.NoError(t, err)
	gData = types.ValueForType(types.BinaryID)
	err = types.Marshal(coord, &gData)
	require.NoError(t, err)
	addEdgeToTypedValue(t, "loc", 31, types.GeoID, gData.Value.([]byte))

	addEdgeToValue(t, "dob", 1, "1910-01-01")
	addEdgeToValue(t, "dob", 23, "1910-01-02")
	addEdgeToValue(t, "dob", 24, "1909-05-05")
	addEdgeToValue(t, "dob", 25, "1909-01-10")
	addEdgeToValue(t, "dob", 31, "1901-01-15")

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

	addEdgeToValue(t, "film.film.initial_release_date", 23, "1900-01-02")
	addEdgeToValue(t, "film.film.initial_release_date", 24, "1909-05-05")
	addEdgeToValue(t, "film.film.initial_release_date", 25, "1929-01-10")
	addEdgeToValue(t, "film.film.initial_release_date", 31, "1801-01-15")

	// for aggregator(sum) test
	{
		data := types.ValueForType(types.BinaryID)
		intD := types.Val{types.Int32ID, int32(4)}
		err = types.Marshal(intD, &data)
		require.NoError(t, err)
		addEdgeToTypedValue(t, "shadow_deep", 23, types.Int32ID, data.Value.([]byte))
	}
	{
		data := types.ValueForType(types.BinaryID)
		intD := types.Val{types.Int32ID, int32(14)}
		err = types.Marshal(intD, &data)
		require.NoError(t, err)
		addEdgeToTypedValue(t, "shadow_deep", 24, types.Int32ID, data.Value.([]byte))
	}

	time.Sleep(5 * time.Millisecond)
}

func processToFastJSON(t *testing.T, query string) string {
	res, err := gql.Parse(query)
	require.NoError(t, err)

	var l Latency
	ctx := context.Background()
	sgl, err := ProcessQuery(ctx, res, &l)
	require.NoError(t, err)

	var buf bytes.Buffer
	require.NoError(t, ToJson(&l, sgl, &buf, nil))
	return string(buf.Bytes())
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

func TestUseVarsMultiCascade(t *testing.T) {
	populateGraph(t)
	query := `
		{
			var(id:0x01) {
				L AS friend {
				 B AS friend
				}
			}

			me(var:[L, B]) {
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
				L AS friend(first:2, orderasc: dob)
			}

			var(id:0x01) {
				G AS friend(first:2, offset:2, orderasc: dob)
			}

			friend1(var:L) {
				name
			}

			friend2(var:G) {
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
						friend @filter(id(L)) {
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
			friend(func:anyof(name, "Michonne Andrea Glenn")) {
				friend {
				 L as friend {
					 name
					 friend @filter(id(L)) {
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
				fr AS friend(first:2, offset:2, orderasc: dob)
			}

			friend(id:0x01) {
				L as friend {
					friend {
						name
						friend @filter(id(L) and id(fr)) {
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

func TestUseVarsFilterMultiId(t *testing.T) {
	populateGraph(t)
	query := `
		{
			var(id:0x01) {
				L AS friend {
					friend
				}
			}

			var(id:31) {
				G AS friend
			}

			friend(func:anyof(name, "Michonne Andrea Glenn")) @filter(id(G, L)) {
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
				L AS friend
			}

			var(id:31) {
				G AS friend
			}

			friend(var:L) @filter(id(G)) {
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
				L AS friend {
				  friend
				}
			}

			me(var:L) {
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
				L AS friend
			}

			me(var : L) {
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

func TestMin(t *testing.T) {
	populateGraph(t)
	// Alright. Now we have everything set up. Let's create the query.
	query := `
                {
                        me(id:0x01) {
                                name
                                gender
                                alive
                                friend {
                                    min(dob)
                                }
                        }
                }
        `
	js := processToFastJSON(t, query)
	require.EqualValues(t,
		`{"me":[{"alive":true,"friend":[{"min(dob)":"1901-01-15"}],"gender":"female","name":"Michonne"}]}`,
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
	res, err := gql.Parse(query)
	require.NoError(t, err)

	var l Latency
	_, queryErr := ProcessQuery(context.Background(), res, &l)
	require.NotNil(t, queryErr)
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
                                    max(dob @filter(gt("dob", "1910-01-02")))
                                }
                        }
                }
        `
	_, err := gql.Parse(query)
	require.NotNil(t, err)
}

func TestSum(t *testing.T) {
	populateGraph(t)
	// Alright. Now we have everything set up. Let's create the query.
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
	require.EqualValues(t,
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
                        me(func:anyof(name, "some cool name")) {
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
                        me(func:anyof(name, "some cool name")) {
                                name
                                friend @filter(anyof(name, "Andrea") or
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
                        f AS var(func:invalidfn4("name", "Michonne Rick Glenn")) {
                                name
                        }
                        you(func:anyof(name, "Michonne")) {
                                friend @filter(id(f)) {
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
                                friend(disorderasc: dob) @filter(leq("dob", "1909-03-20")) {
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
                                friend(offset:1, invalidorderasc:1) @filter(anyof("name", "Andrea")) {
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
			[]uint64{23, 24, 25, 31, 101},
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
      str_val: "female"
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
		proto.MarshalTextString(pb))
}

func TestToFastJSONFilter(t *testing.T) {
	populateGraph(t)
	query := `
		{
			me(id:0x01) {
				name
				gender
				friend @filter(anyof(name, "Andrea SomethingElse")) {
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
				friend @filter(anyof(name, "Andrea SomethingElse") {
					name
				}
			}
		}
	`
	_, err := gql.Parse(query)
	require.Error(t, err)
}

func TestToFastJSONFilterAllOf(t *testing.T) {
	populateGraph(t)
	query := `
		{
			me(id:0x01) {
				name
				gender
				friend @filter(allof("name", "Andrea SomethingElse")) {
					name
				}
			}
		}
	`

	js := processToFastJSON(t, query)
	require.EqualValues(t,
		`{"me":[{"gender":"female","name":"Michonne"}]}`, js)
}

func TestToFastJSONFilterUID(t *testing.T) {
	populateGraph(t)
	query := `
		{
			me(id:0x01) {
				name
				gender
				friend @filter(anyof(name, "Andrea")) {
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
				friend @filter(anyof(name, "Andrea") or anyof(name, "Andrea Rhee")) {
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
				count(friend @filter(anyof(name, "Andrea") or anyof(name, "Andrea Rhee")))
				friend @filter(anyof(name, "Andrea")) {
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
				friend(first:2) @filter(anyof(name, "Andrea") or anyof(name, "Glenn SomethingElse") or anyof(name, "Daryl")) {
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
				friend(offset:1) @filter(anyof(name, "Andrea") or anyof("name", "Glenn Rhee") or anyof("name", "Daryl Dixon")) {
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

func TestToFastJSONFilterGeq(t *testing.T) {
	populateGraph(t)
	query := `
		{
			me(id:0x01) {
				name
				gender
				friend @filter(geq("dob", "1909-05-05")) {
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
				friend @filter(gt("dob", "1909-05-05")) {
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
				friend @filter(leq("dob", "1909-01-10")) {
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
				friend @filter(lt("dob", "1909-01-10")) {
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
				friend @filter(eq("dob", "1909-03-20")) {
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
				friend @filter(eq("dob", "1909-01-10")) {
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

func TestToFastJSONFilterLeqOrder(t *testing.T) {
	populateGraph(t)
	query := `
		{
			me(id:0x01) {
				name
				gender
				friend(orderasc: dob) @filter(leq("dob", "1909-03-20")) {
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
				friend @filter(geq("dob", "1999-03-20")) {
					name
				}
			}
		}
	`

	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"gender":"female","name":"Michonne"}]}`, js)
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
				friend(offset:1, first:1) @filter(anyof("name", "Andrea") or anyof("name", "SomethingElse Rhee") or anyof("name", "Daryl Dixon")) {
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
				friend(offset:1, first:1) @filter(leq("dob", "1909-03-20")) {
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
				count(friend(offset:1, first:1) @filter(anyof("name", "Andrea") or anyof("name", "SomethingElse Rhee") or anyof("name", "Daryl Dixon"))) 
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
				friend(first:-1, offset:0) @filter(anyof("name", "Andrea") or anyof("name", "Glenn Rhee") or anyof("name", "Daryl Dixon")) {
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
				friend @filter(not anyof("name", "Andrea rick")) {
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
				friend @filter(not anyof("name", "Andrea") and anyof(name, "Glenn Andrea")) {
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
				friend @filter(not (anyof("name", "Andrea") or anyof(name, "Glenn Rick Andrea"))) {
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
				friend @filter(anyof("name", "Andrea") and anyof("name", "SomethingElse Rhee")) {
					name
				}
			}
		}
	`

	js := processToFastJSON(t, query)
	require.EqualValues(t,
		`{"me":[{"gender":"female","name":"Michonne"}]}`, js)
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
		`{"me":[{"name":"Glenn Rhee","~friend":[{"alive":true,"gender":"female","name":"Michonne"},{"name":"Andrea"}]}]}`,
		js)
}

func TestToFastJSONReverseFilter(t *testing.T) {
	populateGraph(t)
	query := `
		{
			me(id:0x18) {
				name
				~friend @filter(allof("name", "Andrea")) {
					name
					gender
			  	alive
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
	delEdgeToUID(t, "friend", 1, 24)  // Delete Michonne.
	delEdgeToUID(t, "friend", 23, 24) // Ignored.
	addEdgeToUID(t, "friend", 25, 24) // Add Daryl.

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
		`{"me":[{"name":"Glenn Rhee","~friend":[{"name":"Daryl Dixon"},{"name":"Andrea"}]}]}`,
		js)
}

func TestToFastJSONReverseDelSetCount(t *testing.T) {
	populateGraph(t)
	delEdgeToUID(t, "friend", 1, 24)  // Delete Michonne.
	delEdgeToUID(t, "friend", 23, 24) // Ignored.
	addEdgeToUID(t, "friend", 25, 24) // Add Daryl.

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

func getProperty(properties []*graph.Property, prop string) *graph.Value {
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
				friend {
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
      str_val: "female"
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
`, proto.MarshalTextString(pb))

}

func TestToProtoFilter(t *testing.T) {
	populateGraph(t)
	// Alright. Now we have everything set up. Let's create the query.
	query := `
		{
			me(id:0x01) {
				name
				gender
				friend @filter(anyof("name", "Andrea")) {
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
      str_val: "female"
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
	require.EqualValues(t, expectedPb, proto.MarshalTextString(pb))
}

func TestToProtoFilterOr(t *testing.T) {
	populateGraph(t)
	// Alright. Now we have everything set up. Let's create the query.
	query := `
		{
			me(id:0x01) {
				name
				gender
				friend @filter(anyof("name", "Andrea") or anyof("name", "Glenn Rhee")) {
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
      str_val: "female"
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
	require.EqualValues(t, expectedPb, proto.MarshalTextString(pb))
}

func TestToProtoFilterAnd(t *testing.T) {
	populateGraph(t)
	// Alright. Now we have everything set up. Let's create the query.
	query := `
		{
			me(id:0x01) {
				name
				gender
				friend @filter(anyof("name", "Andrea") and anyof("name", "Glenn Rhee")) {
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
      str_val: "female"
    >
  >
>
`
	require.EqualValues(t, expectedPb, proto.MarshalTextString(pb))
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

// Test sorting / ordering by dob and count.
func TestToFastJSONOrderDescCount(t *testing.T) {
	populateGraph(t)
	query := `
		{
			me(id:0x01) {
				name
				gender
				count(friend @filter(anyof("name", "Rick")) (orderasc: dob)) 
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
func ageSg(uidMatrix []*task.List, srcUids *task.List, ages []uint32) *SubGraph {
	var as []*task.Value
	for _, a := range ages {
		bs := make([]byte, 4)
		binary.LittleEndian.PutUint32(bs, a)
		as = append(as, &task.Value{[]byte(bs), 2})
	}

	return &SubGraph{
		Attr:      "age",
		uidMatrix: uidMatrix,
		SrcUIDs:   srcUids,
		values:    as,
		Params:    params{isDebug: false, GetUID: true},
	}
}
func nameSg(uidMatrix []*task.List, srcUids *task.List, names []string) *SubGraph {
	var ns []*task.Value
	for _, n := range names {
		ns = append(ns, &task.Value{[]byte(n), 0})
	}
	return &SubGraph{
		Attr:      "name",
		uidMatrix: uidMatrix,
		SrcUIDs:   srcUids,
		values:    ns,
		Params:    params{isDebug: false, GetUID: true},
	}

}
func friendsSg(uidMatrix []*task.List, srcUids *task.List, friends []*SubGraph) *SubGraph {
	return &SubGraph{
		Attr:      "friend",
		uidMatrix: uidMatrix,
		SrcUIDs:   srcUids,
		Params:    params{isDebug: false, GetUID: true},
		Children:  friends,
	}
}
func rootSg(uidMatrix []*task.List, srcUids *task.List, names []string, ages []uint32) *SubGraph {
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
      str_val: "female"
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
	require.EqualValues(t, expectedPb, proto.MarshalTextString(pb))
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
      str_val: "female"
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
	require.EqualValues(t, expectedPb, proto.MarshalTextString(pb))
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
      str_val: "female"
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
	require.EqualValues(t, expectedPb, proto.MarshalTextString(pb))
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
		`{"person":[{"address":"31, 32 street, Jupiter","age":38,"alive":true,"friend":[{"address":"21, mark street, Mars","age":15,"name":"Rick Grimes"},{"name":"Glenn Rhee"},{"name":"Daryl Dixon"},{"name":"Andrea"}],"name":"Michonne","survival_rate":98.990000}]}`, js)
}

func TestMultiQuery(t *testing.T) {
	populateGraph(t)
	query := `
		{
			me(func:anyof("name", "Michonne")) {
				name
				gender
			}

			you(func:anyof("name", "Andrea")) {
				name
			}
		}
  `
	js := processToFastJSON(t, query)
	require.JSONEq(t, `{"me":[{"gender":"female","name":"Michonne"}], "you":[{"name":"Andrea"}]}`, js)
}

func TestMultiQueryError1(t *testing.T) {
	populateGraph(t)
	query := `
    {
			me(func:anyof("name", "Michonne")) {
        name
        gender

			you(func:anyof("name", "Andrea")) {
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
      me(anyof("name", "Michonne")) {
        name
        gender
			}
		}

      you(anyof("name", "Andrea")) {
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
			me(func:anyof("name", "Michonne")) {
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
			friend AS var(func:anyof("name", "Michonne Rick Glenn")) {
      	name
			}

			you(var:friend) {
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
			f AS var(func:anyof("name", "Michonne Rick Glenn")) {
      	name
			}

			you(func:anyof(name, "Michonne")) {
				friend @filter(id(f)) {
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
			friend AS var(func:anyof("name", "Michonne Rick Glenn")) {
			}

			you(func:anyof(name, "Michonne Andrea Glenn")) @filter(id(friend)) {
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
			me(func:anyof("name", "Michonne Rick Glenn")) {
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
			L as var(func:anyof("name", "Michonne Rick Glenn"), orderasc: dob, offset:2) {
        name
      }

			me(var:L) {
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
			me(func:anyof("name", "Michonne Rick Glenn"), orderasc: dob, offset:2) {
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
			L as var(func:anyof("name", "Michonne Rick Glenn")) {
        name
      }
			me(var: L, orderasc: dob, offset:2) {
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
			me(func:anyof("name", "Michonne Rick Glenn"), orderdesc: dob) {
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
			me(func:anyof("name", "Michonne Rick Glenn"), orderasc: dob) {
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
			me(func:anyof("name", "Michonne Rick Glenn"), offset: 1) {
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
			me(func:anyof("name", "Michonne Rick Glenn")) {
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

func TestGeneratorMultiRootFilter1(t *testing.T) {
	populateGraph(t)
	query := `
    {
			me(func:anyof("name", "Daryl Rick Glenn")) @filter(leq(dob, 1909-01-10)) {
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
			me(func:anyof("name", "Michonne Rick Glenn")) @filter(geq(dob, 1909-01-10)) {
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
			me(func:anyof("name", "Michonne Rick Glenn")) @filter(anyof(name, "Glenn") and geq(dob, 1909-01-10)) {
        name
      }
    }
  `
	js := processToFastJSON(t, query)
	require.JSONEq(t, `{"me":[{"name":"Glenn Rhee"}]}`, js)
}

func TestToProtoMultiRoot(t *testing.T) {
	populateGraph(t)
	query := `
    {
			me(func:anyof("name", "Michonne Rick Glenn")) {
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
	require.EqualValues(t, expectedPb, proto.MarshalTextString(pb))
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
		me(func:near(loc, [1.1,2.0], 5.001)) @filter(allof(name, "Michonne")) {
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
		proto.MarshalTextString(pb))
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
				friend {
				}
			}
		}
  `
	gr := processToPB(t, query, true)
	require.EqualValues(t, "debug", gr.Children[0].Attribute)
	require.EqualValues(t, 1, gr.Children[0].Uid)
	require.EqualValues(t, "mich", gr.Children[0].Xid)
	require.Len(t, gr.Children[0].Properties, 4)

	require.EqualValues(t, "Michonne",
		getProperty(gr.Children[0].Properties, "name").GetStrVal())

	g := types.ValueForType(types.GeoID)
	g.Value = getProperty(gr.Children[0].Properties, "loc").GetGeoVal()
	g1, err := types.Convert(g, types.StringID)
	x.Check(err)
	require.EqualValues(t, "{'type':'Point','coordinates':[1.1,2]}", string(g1.Value.(string)))

	require.Len(t, gr.Children[0].Children, 5)

	child := gr.Children[0].Children[0]
	require.EqualValues(t, 23, child.Uid)
	require.EqualValues(t, "friend", child.Attribute)

	require.Len(t, child.Properties, 2)
	require.EqualValues(t, "Rick Grimes",
		getProperty(child.Properties, "name").GetStrVal())
	dob := getProperty(child.Properties, "dob").GetDateVal()
	var date time.Time
	date.UnmarshalBinary(dob)
	require.EqualValues(t, "1910-01-02 00:00:00 +0000 UTC", date.String())
	require.Empty(t, child.Children)

	child = gr.Children[0].Children[4]
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
		Children: []*gql.GraphQuery{&gql.GraphQuery{Attr: "name"}},
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
		Children: []*gql.GraphQuery{&gql.GraphQuery{Attr: "name"}},
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
		Children: []*gql.GraphQuery{&gql.GraphQuery{Attr: "name"}},
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
		Children: []*gql.GraphQuery{&gql.GraphQuery{Attr: "name"}},
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
		Children: []*gql.GraphQuery{&gql.GraphQuery{Attr: "name"}},
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
		Children: []*gql.GraphQuery{&gql.GraphQuery{Attr: "name"}},
	}

	mp := runQuery(t, gq)
	expected := `{"me":[{"name":"Googleplex"},{"name":"Shoreline Amphitheater"},
			{"name":"San Carlos Airport"},{"name":"SF Bay area"},
			{"name":"Mountain View"},{"name":"San Carlos"}]}`
	require.JSONEq(t, expected, mp)
}

const schemaStr = `
scalar name:string @index
scalar dob:date @index
scalar film.film.initial_release_date:date @index
scalar loc:geo @index
scalar (
	survival_rate : float
	alive         : bool
	age           : int
        shadow_deep   : int
)
scalar (
  friend:uid @reverse
)
scalar geometry:geo @index
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

	schema.ParseBytes([]byte(schemaStr))
	posting.Init(ps)
	worker.Init(ps)

	group.ParseGroupConfig("")
	dir2, err := ioutil.TempDir("", "wal_")
	x.Check(err)

	worker.StartRaftNodes(dir2)
	defer os.RemoveAll(dir2)

	os.Exit(m.Run())
}
