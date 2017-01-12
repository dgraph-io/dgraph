/*
 * Copyright 2017 DGraph Labs, Inc.
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
	"github.com/dgraph-io/dgraph/x"
)

const schemaStr = `
scalar name:string @index
scalar dob:date @index
scalar loc:geo @index
scalar (
  friend:uid @reverse
)
`

func addEdgeToValue(t *testing.T, ps *store.Store, attr string, src uint64,
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

func addEdgeToTypedValue(t *testing.T, ps *store.Store, attr string, src uint64,
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

func addEdgeToUID(t *testing.T, ps *store.Store, attr string, src uint64, dst uint64) {
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

func delEdgeToUID(t *testing.T, ps *store.Store, attr string, src uint64, dst uint64) {
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

func populateGraph(t *testing.T) (string, string, *store.Store) {
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

	data := types.ValueForType(types.BinaryID)
	intD := types.Val{types.Int32ID, int32(15)}
	err = types.Marshal(intD, &data)
	require.NoError(t, err)
	addEdgeToTypedValue(t, ps, "age", 1, types.Int32ID, data.Value.([]byte))
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
	dir, dir2, ps := populateGraph(t)
	defer ps.Close()
	defer os.RemoveAll(dir)
	defer os.RemoveAll(dir2)

	res, err := gql.Parse(query)
	require.NoError(t, err)

	ctx := context.Background()
	sg, err := ToSubGraph(ctx, res.Query)
	require.NoError(t, err)

	ch := make(chan error)
	go ProcessGraph(ctx, sg, nil, ch)
	err = <-ch
	require.NoError(t, err)

	var l Latency
	_, err = sg.ToJSON(&l)
	require.NoError(t, err)

	return sg
}

func TestFastToJSONSimpleQuery(t *testing.T) {
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
	js, _ := sg.FastToJSON(&l)

	require.JSONEq(t, `{"me":[{"_uid_":"0x1","name":"Michonne","gender":"female","alive":"true","friend":[{"_uid_":"0x17","name":"Rick Grimes"},{"_uid_":"0x18","name":"Glenn Rhee"},{"_uid_":"0x19","name":"Daryl Dixon"},{"_uid_":"0x1f","name":"Andrea"},{"_uid_":"0x65"}]}]}`,
		string(js))
}

func TestBenchmarkFastJsonNode(t *testing.T) {
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

	sgFastJson := makeSubgraph(query, t)
	var l Latency
	fastjs, _ := sgFastJson.FastToJSON(&l)
	fmt.Println("response fastjs:", string(fastjs))
	bFastJson := testing.Benchmark(func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			var l Latency
			if _, err := sgFastJson.FastToJSON(&l); err != nil {
				b.FailNow()
				break
			}
		}
	})
	fmt.Println("fastToJson: Benchmarks: times: ", bFastJson.N, " total time: ", bFastJson.T, " ns/op: ", bFastJson.NsPerOp())
}

func TestBenchmarkOutputJsonNode(t *testing.T) {
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

	sgJson := makeSubgraph(query, t)
	var l Latency
	js, _ := sgJson.ToJSON(&l)
	fmt.Println("response js:", string(js))

	bresJson := testing.Benchmark(func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			var l Latency
			if _, err := sgJson.ToJSON(&l); err != nil {
				b.FailNow()
				break
			}
		}
	})
	fmt.Println("tojson: Benchmarks: times: ", bresJson.N, " total time: ", bresJson.T, " ns/op: ", bresJson.NsPerOp())
}
