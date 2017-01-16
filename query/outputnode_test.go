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
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	farm "github.com/dgryski/go-farm"
	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/group"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/store"
	"github.com/dgraph-io/dgraph/task"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/worker"
)

func populateGraph2(t *testing.T) (string, string, *store.Store) {
	// logrus.SetLevel(logrus.DebugLevel)
	dir, err := ioutil.TempDir("", "storetest_")
	require.NoError(t, err)

	ps, err := store.NewStore(dir)
	require.NoError(t, err)

	schema.ParseBytes([]byte(schemaStr))
	posting.Init(ps)
	worker.Init(ps)

	group.ParseGroupConfig("")
	dir2, err := ioutil.TempDir("", "wal_")
	require.NoError(t, err)
	worker.StartRaftNodes(dir2)

	// So, user we're interested in has uid: 1.
	// She has 5 friends: 23, 24, 25, 31, and 101
	addEdgeToUID(t, ps, "friend", 1, 23)
	addEdgeToUID(t, ps, "friend", 1, 24)
	addEdgeToUID(t, ps, "friend", 1, 25)
	addEdgeToUID(t, ps, "friend", 1, 31)
	addEdgeToUID(t, ps, "friend", 1, 101)
	addEdgeToUID(t, ps, "friend", 31, 24)

	// Now let's add a few properties for the main user.
	addEdgeToValue(t, ps, "name", 1, "Michonne")
	addEdgeToValue(t, ps, "gender", 1, "female")

	coord := types.ValueForType(types.GeoID)
	src := types.ValueForType(types.StringID)
	src.Value = []byte("{\"Type\":\"Point\", \"Coordinates\":[1.1,2.0]}")
	err = types.Convert(src, &coord)
	require.NoError(t, err)
	gData := types.ValueForType(types.BinaryID)
	err = types.Marshal(coord, &gData)
	require.NoError(t, err)
	addEdgeToTypedValue(t, ps, "loc", 1, types.GeoID, gData.Value.([]byte))

	// Int32ID
	data := types.ValueForType(types.BinaryID)
	intD := types.Val{types.Int32ID, int32(15)}
	err = types.Marshal(intD, &data)
	require.NoError(t, err)
	addEdgeToTypedValue(t, ps, "age", 1, types.Int32ID, data.Value.([]byte))

	// FloatID
	fdata := types.ValueForType(types.BinaryID)
	floatD := types.Val{types.FloatID, float64(13.25)}
	err = types.Marshal(floatD, &fdata)
	require.NoError(t, err)
	addEdgeToTypedValue(t, ps, "power", 1, types.FloatID, fdata.Value.([]byte))

	addEdgeToValue(t, ps, "address", 1, "31, 32 street, Jupiter")

	boolD := types.Val{types.BoolID, true}
	err = types.Marshal(boolD, &data)
	require.NoError(t, err)
	addEdgeToTypedValue(t, ps, "alive", 1, types.BoolID, data.Value.([]byte))
	addEdgeToValue(t, ps, "age", 1, "38")
	addEdgeToValue(t, ps, "survival_rate", 1, "98.99")
	addEdgeToValue(t, ps, "sword_present", 1, "true")
	addEdgeToValue(t, ps, "_xid_", 1, "mich")

	// Now let's add a name for each of the friends, except 101.
	addEdgeToTypedValue(t, ps, "name", 23, types.StringID, []byte("Rick Grimes"))
	addEdgeToValue(t, ps, "age", 23, "15")

	src.Value = []byte(`{"Type":"Polygon", "Coordinates":[[[0.0,0.0], [2.0,0.0], [2.0, 2.0], [0.0, 2.0], [0.0, 0.0]]]}`)
	err = types.Convert(src, &coord)
	require.NoError(t, err)
	gData = types.ValueForType(types.BinaryID)
	err = types.Marshal(coord, &gData)
	require.NoError(t, err)
	addEdgeToTypedValue(t, ps, "loc", 23, types.GeoID, gData.Value.([]byte))

	addEdgeToValue(t, ps, "address", 23, "21, mark street, Mars")
	addEdgeToValue(t, ps, "name", 24, "Glenn Rhee")
	src.Value = []byte(`{"Type":"Point", "Coordinates":[1.10001,2.000001]}`)
	err = types.Convert(src, &coord)
	require.NoError(t, err)
	gData = types.ValueForType(types.BinaryID)
	err = types.Marshal(coord, &gData)
	require.NoError(t, err)
	addEdgeToTypedValue(t, ps, "loc", 24, types.GeoID, gData.Value.([]byte))

	addEdgeToValue(t, ps, "name", farm.Fingerprint64([]byte("a.bc")), "Alice")
	addEdgeToValue(t, ps, "name", 25, "Daryl Dixon")
	addEdgeToValue(t, ps, "name", 31, "Andrea")
	src.Value = []byte(`{"Type":"Point", "Coordinates":[2.0, 2.0]}`)
	err = types.Convert(src, &coord)
	require.NoError(t, err)
	gData = types.ValueForType(types.BinaryID)
	err = types.Marshal(coord, &gData)
	require.NoError(t, err)
	addEdgeToTypedValue(t, ps, "loc", 31, types.GeoID, gData.Value.([]byte))

	addEdgeToValue(t, ps, "dob", 23, "1910-01-02")
	addEdgeToValue(t, ps, "dob", 24, "1909-05-05")
	addEdgeToValue(t, ps, "dob", 25, "1909-01-10")
	addEdgeToValue(t, ps, "dob", 31, "1901-01-15")

	return dir, dir2, ps
}

func makeSubgraph(query string, t *testing.T) *SubGraph {
	res, err := gql.Parse(query)
	require.NoError(t, err)

	ctx := context.Background()
	sg, err := ToSubGraph(ctx, res.Query)
	require.NoError(t, err)

	ch := make(chan error)
	go ProcessGraph(ctx, sg, nil, ch)
	err = <-ch
	require.NoError(t, err)

	return sg
}

func TestEnvToFastJSONSimpleQuery(t *testing.T) {
	dir, dir2, ps := populateGraph2(t)
	defer ps.Close()
	defer os.RemoveAll(dir)
	defer os.RemoveAll(dir2)

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

	sg := makeSubgraph(query, t)
	var l Latency
	jsFast, err := sg.ToFastJSON(&l)
	require.NoError(t, err)

	// check validity of json
	var unmarshalJs map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(jsFast), &unmarshalJs))

	require.JSONEq(t, `{"me":[{"_uid_":"0x1","name":"Michonne","gender":"female","alive":"true","friend":[{"_uid_":"0x17","name":"Rick Grimes"},{"_uid_":"0x18","name":"Glenn Rhee"},{"_uid_":"0x19","name":"Daryl Dixon"},{"_uid_":"0x1f","name":"Andrea"},{"_uid_":"0x65"}]}]}`,
		string(jsFast))

	jsCurr, err := sg.ToJSON(&l)
	require.NoError(t, err)
	require.JSONEq(t, string(jsFast), string(jsCurr))
}

func TestEnvToFastJSONNestedQuery(t *testing.T) {
	dir, dir2, ps := populateGraph2(t)
	defer ps.Close()
	defer os.RemoveAll(dir)
	defer os.RemoveAll(dir2)

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
                                        friend {
                                           alive
                                           name
                                           friend { name }
                                        }
				}
			}
		}
	`

	sg := makeSubgraph(query, t)
	var l Latency
	jsFast, err := sg.ToFastJSON(&l)
	require.NoError(t, err)

	// check validity of json
	var unmarshalJs map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(jsFast), &unmarshalJs))

	require.JSONEq(t, `{"me":[{"_uid_":"0x1","alive":"true","friend":[{"_uid_":"0x17","name":"Rick Grimes"},{"_uid_":"0x18","name":"Glenn Rhee"},{"_uid_":"0x19","name":"Daryl Dixon"},{"_uid_":"0x1f","friend":[{"name":"Glenn Rhee"}],"name":"Andrea"},{"_uid_":"0x65"}],"gender":"female","name":"Michonne"}]}`,
		string(jsFast))

	jsCurr, err := sg.ToJSON(&l)
	require.NoError(t, err)
	require.JSONEq(t, string(jsFast), string(jsCurr))
}

func TestEnvToFastJSONComplexQuery(t *testing.T) {
	dir, dir2, ps := populateGraph2(t)
	defer ps.Close()
	defer os.RemoveAll(dir)
	defer os.RemoveAll(dir2)

	query := `
		{
			me(id:0x01) {
				name
				_uid_
				gender
				alive
				friend @filter(anyof("name", "Rick Glenn Andrea")) {
					_uid_
					name
                                        friend @filter(anyof("name", "Rhee")) {
                                           alive
                                           name
                                           friend { name }
                                        }
				}
			}
		}
	`

	sg := makeSubgraph(query, t)
	var l Latency
	jsFast, err := sg.ToFastJSON(&l)
	require.NoError(t, err)

	// check validity of json
	var unmarshalJs map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(jsFast), &unmarshalJs))

	require.JSONEq(t, `{"me":[{"_uid_":"0x1","alive":"true","friend":[{"_uid_":"0x17","name":"Rick Grimes"},{"_uid_":"0x18","name":"Glenn Rhee"},{"_uid_":"0x1f","friend":[{"name":"Glenn Rhee"}],"name":"Andrea"}],"gender":"female","name":"Michonne"}]}`,
		string(jsFast))

	jsCurr, err := sg.ToJSON(&l)
	require.NoError(t, err)
	require.JSONEq(t, string(jsFast), string(jsCurr))
}

func TestEnvToFastJSONDataTypesQuery(t *testing.T) {
	dir, dir2, ps := populateGraph2(t)
	defer ps.Close()
	defer os.RemoveAll(dir)
	defer os.RemoveAll(dir2)

	query := `
		{
			me(id:0x01) {
				name
				_uid_
				gender
                                loc
                                dob
                                address
                                power
				friend {
		                        loc
                                        name
                                        friend {
                                           alive
                                           name
                                           dob
                                        }
				}
			}
		}
	`

	sg := makeSubgraph(query, t)
	var l Latency
	jsFast, err := sg.ToFastJSON(&l)
	require.NoError(t, err)
	// check validity of json
	var unmarshalJs map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(jsFast), &unmarshalJs))

	require.JSONEq(t, `{"me":[{"_uid_":"0x1","address":"31, 32 street, Jupiter","friend":[{"loc":{"type":"Polygon","coordinates":[[[0,0],[2,0],[2,2],[0,2],[0,0]]]},"name":"Rick Grimes"},{"loc":{"type":"Point","coordinates":[1.10001,2.000001]},"name":"Glenn Rhee"},{"name":"Daryl Dixon"},{"friend":[{"dob":"1909-05-05","name":"Glenn Rhee"}],"loc":{"type":"Point","coordinates":[2,2]},"name":"Andrea"}],"gender":"female","loc":{"type":"Point","coordinates":[1.1,2]},"name":"Michonne","power":"1.325E+01"}]}`,
		string(jsFast))

	jsCurr, err := sg.ToJSON(&l)
	require.NoError(t, err)
	require.JSONEq(t, string(jsFast), string(jsCurr))
}

func TestBenchmarkFastJsonNode(t *testing.T) {
	dir, dir2, ps := populateGraph2(t)
	defer ps.Close()
	defer os.RemoveAll(dir)
	defer os.RemoveAll(dir2)

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
				        friend {
					   _uid_
					   name
				        }
				}
			}
		}
	`

	sg := makeSubgraph(query, t)
	bFastJson := testing.Benchmark(func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			var l Latency
			if _, err := sg.ToFastJSON(&l); err != nil {
				b.FailNow()
				break
			}
		}
	})
	fmt.Println("fastToJson: Benchmarks: ", bFastJson.N, " times ; ", bFastJson.T, " total time ; ", bFastJson.NsPerOp(), " ns/op")
}

func TestBenchmarkOutputJsonNode(t *testing.T) {
	dir, dir2, ps := populateGraph2(t)
	defer ps.Close()
	defer os.RemoveAll(dir)
	defer os.RemoveAll(dir2)

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
				        friend {
					   _uid_
					   name
				        }
				}
			}
		}
	`

	sg := makeSubgraph(query, t)
	bresJson := testing.Benchmark(func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			var l Latency
			if _, err := sg.ToJSON(&l); err != nil {
				b.FailNow()
				break
			}
		}
	})
	fmt.Println("tojson: Benchmarks: ", bresJson.N, " times ; ", bresJson.T, " total time ; ", bresJson.NsPerOp(), " ns/op")
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

func mockSubGraph() *SubGraph {
	emptyUids := []uint64{}
	uidMatrix := []*task.List{&task.List{Uids: emptyUids}, &task.List{Uids: emptyUids}, &task.List{Uids: emptyUids}, &task.List{Uids: emptyUids}}
	srcUids := &task.List{Uids: []uint64{2, 3, 4, 5}}

	names := []string{"lincon", "messi", "martin", "aishwarya"}
	ages := []uint32{56, 29, 45, 36}
	namesSg := nameSg(uidMatrix, srcUids, names)
	agesSg := ageSg(uidMatrix, srcUids, ages)

	sgSrcUids := &task.List{Uids: []uint64{1}}
	sgUidMatrix := []*task.List{&task.List{Uids: emptyUids}}

	friendUidMatrix := []*task.List{&task.List{Uids: []uint64{2, 3, 4, 5}}}
	friendsSg1 := friendsSg(friendUidMatrix, sgSrcUids, []*SubGraph{namesSg, agesSg})

	sg := rootSg(sgUidMatrix, sgSrcUids, []string{"unknown"}, []uint32{39})
	sg.Children = append(sg.Children, friendsSg1)
	sg.DestUIDs = &task.List{Uids: []uint64{1}}
	sg.Params.Alias = "me"
	return sg
}

func TestMockSubGraphFastJson(t *testing.T) {
	sg := mockSubGraph()
	var l Latency
	js, _ := sg.ToFastJSON(&l)
	// check validity of json
	var unmarshalJs map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(js), &unmarshalJs))

	require.JSONEq(t, `{"me":[{"_uid_":"0x1","age":"39","friend":[{"_uid_":"0x2","age":"56","name":"lincon"},{"_uid_":"0x3","age":"29","name":"messi"},{"_uid_":"0x4","age":"45","name":"martin"},{"_uid_":"0x5","age":"36","name":"aishwarya"}],"name":"unknown"}]}`,
		string(js))
	js2, err := sg.ToJSON(&l)
	require.NoError(t, err)
	require.JSONEq(t, string(js), string(js2))
}

func BenchmarkMockSubGraphFastJson(b *testing.B) {
	sg := mockSubGraph()
	var l Latency
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := sg.ToFastJSON(&l); err != nil {
			b.FailNow()
			break
		}
	}
}

func BenchmarkMockSubGraphToJSON(b *testing.B) {
	sg := mockSubGraph()
	var l Latency
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := sg.ToJSON(&l); err != nil {
			b.FailNow()
			break
		}
	}
}

// run : go test -memprofile memmock.out -run=TestMemoryUsageMockSGFastJSON
func TestMemoryUsageMockSGFastJSON(t *testing.T) {
	sg := mockSubGraph()
	var l Latency
	for i := 0; i < 100000; i++ {
		if _, err := sg.ToFastJSON(&l); err != nil {
			t.FailNow()
			break
		}
	}
}

// run : go test -memprofile memenv.out -run=TestMemoryUsageMockSGFastJSON
func TestMemoryUsageEnvSGFastJSON(t *testing.T) {
	dir, dir2, ps := populateGraph2(t)
	defer ps.Close()
	defer os.RemoveAll(dir)
	defer os.RemoveAll(dir2)
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
				        friend {
					   _uid_
					   name
				        }
				}
			}
		}
	`
	sg := makeSubgraph(query, t)

	var l Latency
	for i := 0; i < 100000; i++ {
		if _, err := sg.ToFastJSON(&l); err != nil {
			t.FailNow()
			break
		}
	}
}
