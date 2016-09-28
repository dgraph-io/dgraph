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
	"encoding/gob"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gogo/protobuf/proto"

	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/query/graph"
	"github.com/dgraph-io/dgraph/store"
	"github.com/dgraph-io/dgraph/task"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/worker"
	"github.com/dgraph-io/dgraph/x"
)

func setErr(err *error, nerr error) {
	if err != nil {
		return
	}
	*err = nerr
}

func addEdge(t *testing.T, edge x.DirectedEdge, l *posting.List) {
	if _, err := l.AddMutation(context.Background(), edge, posting.Set); err != nil {
		t.Error(err)
	}
}

func checkName(t *testing.T, r *task.Result, idx int, expected string) {
	var tv task.Value
	if ok := r.Values(&tv, idx); !ok {
		t.Error("Unable to retrieve value")
	}
	name := tv.ValBytes()
	if string(name) != expected {
		t.Errorf("Expected: %v. Got: %v", expected, string(name))
	}
}

func checkSingleValue(t *testing.T, child *SubGraph,
	attr string, value string) {
	if child.Attr != attr || len(child.Result) == 0 {
		t.Error("Expected attr name with some.Result")
	}
	r := new(task.Result)
	x.ParseTaskResult(r, child.Result)

	if r.ValuesLength() != 1 {
		t.Errorf("Expected value length 1. Got: %v", r.ValuesLength())
	}
	if r.UidmatrixLength() != 1 {
		t.Errorf("Expected uidmatrix length 1. Got: %v", r.UidmatrixLength())
	}
	var ul task.UidList
	if ok := r.Uidmatrix(&ul, 0); !ok {
		t.Errorf("While parsing uidlist")
	}

	if ul.UidsLength() != 0 {
		t.Errorf("Expected uids length 0. Got: %v", ul.UidsLength())
	}
	checkName(t, r, 0, value)
}

func TestNewGraph(t *testing.T) {
	dir, err := ioutil.TempDir("", "storetest_")
	if err != nil {
		t.Error(err)
		return
	}

	gq := &gql.GraphQuery{
		UID:  101,
		Attr: "me",
	}
	ps, err := store.NewStore(dir)
	if err != nil {
		t.Error(err)
		return
	}
	ctx := context.Background()
	sg, err := newGraph(ctx, gq)
	if err != nil {
		t.Error(err)
	}

	worker.SetWorkerState(worker.NewState(ps, nil, 0, 1))

	r := new(task.Result)
	x.ParseTaskResult(r, sg.Result)
	if r.UidmatrixLength() != 1 {
		t.Errorf("Expected length 1. Got: %v", r.UidmatrixLength())
	}
	var ul task.UidList
	if ok := r.Uidmatrix(&ul, 0); !ok {
		t.Errorf("Unable to parse uidlist at index 0")
	}
	if ul.UidsLength() != 1 {
		t.Errorf("Expected length 1. Got: %v", ul.UidsLength())
	}
	if ul.Uids(0) != 101 {
		t.Errorf("Expected uid: %v. Got: %v", 101, ul.Uids(0))
	}
}

func getOrCreate(key []byte, ps *store.Store) *posting.List {
	l, _ := posting.GetOrCreate(key, ps)
	return l
}

func populateGraph(t *testing.T) (string, *store.Store) {
	// logrus.SetLevel(logrus.DebugLevel)
	dir, err := ioutil.TempDir("", "storetest_")
	if err != nil {
		t.Error(err)
		return "", nil
	}

	ps, err := store.NewStore(dir)
	if err != nil {
		t.Error(err)
		return "", nil
	}

	worker.SetWorkerState(worker.NewState(ps, nil, 0, 1))
	posting.Init()

	// So, user we're interested in has uid: 1.
	// She has 5 friends: 23, 24, 25, 31, and 101
	edge := x.DirectedEdge{
		ValueId:   23,
		Source:    "testing",
		Timestamp: time.Now(),
	}
	addEdge(t, edge, getOrCreate(posting.Key(1, "friend"), ps))

	edge.ValueId = 24
	addEdge(t, edge, getOrCreate(posting.Key(1, "friend"), ps))

	edge.ValueId = 25
	addEdge(t, edge, getOrCreate(posting.Key(1, "friend"), ps))

	edge.ValueId = 31
	addEdge(t, edge, getOrCreate(posting.Key(1, "friend"), ps))

	edge.ValueId = 101
	addEdge(t, edge, getOrCreate(posting.Key(1, "friend"), ps))

	// Now let's add a few properties for the main user.
	edge.Value = []byte("Michonne")
	addEdge(t, edge, getOrCreate(posting.Key(1, "name"), ps))

	edge.Value = []byte("female")
	addEdge(t, edge, getOrCreate(posting.Key(1, "gender"), ps))

	edge.Value = []byte("alive")
	addEdge(t, edge, getOrCreate(posting.Key(1, "status"), ps))

	edge.Value = []byte("38")
	addEdge(t, edge, getOrCreate(posting.Key(1, "age"), ps))

	edge.Value = []byte("98.99")
	addEdge(t, edge, getOrCreate(posting.Key(1, "survival_rate"), ps))

	edge.Value = []byte("true")
	addEdge(t, edge, getOrCreate(posting.Key(1, "sword_present"), ps))

	// Now let's add a name for each of the friends, except 101.
	edge.Value = []byte("Rick Grimes")
	addEdge(t, edge, getOrCreate(posting.Key(23, "name"), ps))

	edge.Value = []byte("Glenn Rhee")
	addEdge(t, edge, getOrCreate(posting.Key(24, "name"), ps))

	edge.Value = []byte("Daryl Dixon")
	addEdge(t, edge, getOrCreate(posting.Key(25, "name"), ps))

	edge.Value = []byte("Andrea")
	addEdge(t, edge, getOrCreate(posting.Key(31, "name"), ps))

	edge.Value = []byte("mich")
	addEdge(t, edge, getOrCreate(posting.Key(1, "_xid_"), ps))

	return dir, ps
}

func processToJson(t *testing.T, query string) map[string]interface{} {
	gq, _, err := gql.Parse(query)
	if err != nil {
		t.Error(err)
	}
	ctx := context.Background()
	sg, err := ToSubGraph(ctx, gq)
	if err != nil {
		t.Error(err)
	}

	ch := make(chan error)
	go ProcessGraph(ctx, sg, ch)
	err = <-ch
	if err != nil {
		t.Error(err)
	}

	var l Latency
	js, err := sg.ToJSON(&l)
	if err != nil {
		t.Error(err)
	}
	var mp map[string]interface{}
	err = json.Unmarshal(js, &mp)
	if err != nil {
		t.Error(err)
	}
	return mp
}

func TestGetUid(t *testing.T) {
	dir, _ := populateGraph(t)
	defer os.RemoveAll(dir)

	// Alright. Now we have everything set up. Let's create the query.
	query := `
		{
			me(_uid_:0x01) {
				name
				_uid_
				gender
				status
				friend {
					_count_
				}
			}
		}
	`
	mp := processToJson(t, query)
	fmt.Println(mp)
	resp := mp["me"]
	uid := resp.([]interface{})[0].(map[string]interface{})["_uid_"].(string)
	if uid != "0x1" {
		t.Errorf("Expected uid 0x01. Got %s", uid)
	}
}

func TestDebug1(t *testing.T) {
	dir, _ := populateGraph(t)
	defer os.RemoveAll(dir)

	// Alright. Now we have everything set up. Let's create the query.
	query := `
		{
			debug(_uid_:0x01) {
				name
				gender
				status
				friend {
					_count_
				}
			}
		}
	`

	mp := processToJson(t, query)
	resp := mp["debug"]
	uid := resp.([]interface{})[0].(map[string]interface{})["_uid_"].(string)
	if uid != "0x1" {
		t.Errorf("Expected uid 0x1. Got %s", uid)
	}
}

func TestDebug2(t *testing.T) {
	dir, _ := populateGraph(t)
	defer os.RemoveAll(dir)

	query := `
		{
			me(_uid_:0x01) {
				name
				gender
				status
				friend {
					_count_
				}
			}
		}
	`

	mp := processToJson(t, query)
	resp := mp["me"]
	uid, ok := resp.([]interface{})[0].(map[string]interface{})["_uid_"].(string)
	if ok {
		t.Errorf("No uid expected but got one %s", uid)
	}

}

func TestCount(t *testing.T) {
	dir, _ := populateGraph(t)
	defer os.RemoveAll(dir)

	// Alright. Now we have everything set up. Let's create the query.
	query := `
		{
			me(_uid_:0x01) {
				name
				gender
				status
				friend {
					_count_
				}
			}
		}
	`

	mp := processToJson(t, query)
	resp := mp["me"]
	friend := resp.([]interface{})[0].(map[string]interface{})["friend"]
	count := int(friend.(map[string]interface{})["_count_"].(float64))
	if count != 5 {
		t.Errorf("Expected count 1. Got %d", count)
	}
}

func TestCountError1(t *testing.T) {

	// Alright. Now we have everything set up. Let's create the query.
	query := `
		{
			me(_uid_: 0x01) {
				friend {
					name
					_count_
				}
				name
				gender
				status
			}
		}
	`
	gq, _, err := gql.Parse(query)
	if err != nil {
		t.Error(err)
	}
	ctx := context.Background()
	_, err = ToSubGraph(ctx, gq)
	if err == nil {
		t.Error("Expected error")
	}
}

func TestCountError2(t *testing.T) {

	// Alright. Now we have everything set up. Let's create the query.
	query := `
		{
			me(_uid_: 0x01) {
				friend {
					_count_ {
						friend
					}
				}
				name
				gender
				status
			}
		}
	`
	gq, _, err := gql.Parse(query)
	if err != nil {
		t.Error(err)
	}
	ctx := context.Background()
	_, err = ToSubGraph(ctx, gq)
	if err == nil {
		t.Error("Expected error")
	}
}

func TestProcessGraph(t *testing.T) {
	dir, _ := populateGraph(t)
	defer os.RemoveAll(dir)

	// Alright. Now we have everything set up. Let's create the query.
	query := `
		{
			me(_uid_: 0x01) {
				friend {
					name
				}
				name
				gender
				status
			}
		}
	`
	gq, _, err := gql.Parse(query)
	if err != nil {
		t.Error(err)
	}
	ctx := context.Background()
	sg, err := ToSubGraph(ctx, gq)
	if err != nil {
		t.Error(err)
	}

	ch := make(chan error)
	go ProcessGraph(ctx, sg, ch)
	err = <-ch
	if err != nil {
		t.Error(err)
	}

	if len(sg.Children) != 4 {
		t.Errorf("Expected len 4. Got: %v", len(sg.Children))
	}
	child := sg.Children[0]
	if child.Attr != "friend" {
		t.Errorf("Expected attr friend. Got: %v", child.Attr)
	}
	if len(child.Result) == 0 {
		t.Errorf("Expected some.Result.")
		return
	}

	r := new(task.Result)
	x.ParseTaskResult(r, child.Result)

	if r.UidmatrixLength() != 1 {
		t.Errorf("Expected 1 matrix. Got: %v", r.UidmatrixLength())
	}
	var ul task.UidList
	if ok := r.Uidmatrix(&ul, 0); !ok {
		t.Errorf("While parsing uidlist")
	}

	if ul.UidsLength() != 5 {
		t.Errorf("Expected 5 friends. Got: %v", ul.UidsLength())
	}
	if ul.Uids(0) != 23 || ul.Uids(1) != 24 || ul.Uids(2) != 25 ||
		ul.Uids(3) != 31 || ul.Uids(4) != 101 {
		t.Errorf("Friend ids don't match")
	}
	if len(child.Children) != 1 || child.Children[0].Attr != "name" {
		t.Errorf("Expected attr name")
	}
	child = child.Children[0]
	x.ParseTaskResult(r, child.Result)
	if r.ValuesLength() != 5 {
		t.Errorf("Expected 5 names of 5 friends")
	}
	checkName(t, r, 0, "Rick Grimes")
	checkName(t, r, 1, "Glenn Rhee")
	checkName(t, r, 2, "Daryl Dixon")
	checkName(t, r, 3, "Andrea")
	{
		var tv task.Value
		if ok := r.Values(&tv, 4); !ok {
			t.Error("Unable to retrieve value")
		}
		if !bytes.Equal(tv.ValBytes(), []byte{}) {
			t.Error("Expected a null byte slice")
		}
	}

	checkSingleValue(t, sg.Children[1], "name", "Michonne")
	checkSingleValue(t, sg.Children[2], "gender", "female")
	checkSingleValue(t, sg.Children[3], "status", "alive")
}

func TestToJson(t *testing.T) {
	dir, _ := populateGraph(t)
	defer os.RemoveAll(dir)

	// Alright. Now we have everything set up. Let's create the query.
	query := `
		{
			me(_uid_:0x01) {
				name
				gender
				status
				friend {
					name
				}
			}
		}
	`

	gq, _, err := gql.Parse(query)
	if err != nil {
		t.Error(err)
	}
	ctx := context.Background()
	sg, err := ToSubGraph(ctx, gq)
	if err != nil {
		t.Error(err)
	}

	ch := make(chan error)
	go ProcessGraph(ctx, sg, ch)
	err = <-ch
	if err != nil {
		t.Error(err)
	}

	var l Latency
	js, err := sg.ToJSON(&l)
	if err != nil {
		t.Error(err)
	}
	s := string(js)
	if !strings.Contains(s, "Michonne") {
		t.Errorf("Unable to find Michonne in this result: %v", s)
	}
}

// Checking for Type coercion errors.
// NOTE: Writing separate test since Marshal/Unmarshal process of ToJSON converts
// 'int' type to 'float64' and thus mucks up the tests.
func TestPostTraverse(t *testing.T) {
	dir, _ := populateGraph(t)
	defer os.RemoveAll(dir)

	file, err := os.Create("test_schema.json")
	if err != nil {
		t.Error(err)
	}
	s := `
		{
			"name": "string",
			"age": "int",
			"survival_rate": "float",
			"sword_present": "bool"
		}
	`
	file.WriteString(s)

	defer file.Close()
	defer os.Remove(file.Name())

	// load schema from json file
	err = gql.LoadSchema(file.Name())
	if err != nil {
		t.Error(err)
	}

	query := `
		{
			actor(_uid_:0x01) {
				name
				age
				sword_present
				survival_rate
				friend {
					name
				}
			}
		}
	`

	gq, _, err := gql.Parse(query)
	if err != nil {
		t.Error(err)
	}
	ctx := context.Background()
	sg, err := ToSubGraph(ctx, gq)
	if err != nil {
		t.Error(err)
	}

	ch := make(chan error)
	go ProcessGraph(ctx, sg, ch)
	err = <-ch
	if err != nil {
		t.Error(err)
	}

	r, err := postTraverse(sg)
	if err != nil {
		t.Error(err)
	}

	// iterating over the map (as in ToJSON) to get the required values
	for _, val := range r {
		var m map[string]interface{}
		if val != nil {
			m = val.(map[string]interface{})
		} else {
			m = make(map[string]interface{})
		}

		actorMap := m["actor"].([]interface{})[0].(map[string]interface{})

		if _, success := actorMap["name"].(types.StringType); !success {
			t.Errorf("Expected type coercion to string for: %v\n", actorMap["name"])
		}
		// Note: although, int and int32 have same size, they are treated as different types in go
		// GraphQL spec mentions integer type to be int32
		if _, success := actorMap["age"].(types.Int32Type); !success {
			t.Errorf("Expected type coercion to int32 for: %v\n", actorMap["age"])
		}
		if _, success := actorMap["sword_present"].(types.BoolType); !success {
			t.Errorf("Expected type coercion to bool for: %v\n", actorMap["sword_present"])
		}
		if _, success := actorMap["survival_rate"].(types.FloatType); !success {
			t.Errorf("Expected type coercion to float64 for: %v\n", actorMap["survival_rate"])
		}
		friendMap := actorMap["friend"].([]interface{})
		friend := friendMap[0].(map[string]interface{})
		if _, success := friend["name"].(types.StringType); !success {
			t.Errorf("Expected type coercion to string for: %v\n", friend["name"])
		}

		// Test that the types marshal to json correctly
		js, err := json.Marshal(val)
		if err != nil {
			t.Errorf("Error marshaling json: %v\n", err)
		}
		var mp map[string]interface{}
		err = json.Unmarshal(js, &mp)
		if err != nil {
			t.Error(err)
		}

		actorMap = mp["actor"].([]interface{})[0].(map[string]interface{})
		if _, success := actorMap["name"].(string); !success {
			t.Errorf("Expected json type string for: %v\n", actorMap["name"])
		}
		// json parses ints as floats
		if _, success := actorMap["age"].(float64); !success {
			t.Errorf("Expected json type int for: %v\n", actorMap["age"])
		}
		if _, success := actorMap["sword_present"].(bool); !success {
			t.Errorf("Expected json type bool for: %v\n", actorMap["sword_present"])
		}
		if _, success := actorMap["survival_rate"].(float64); !success {
			t.Errorf("Expected json type float64 for: %v\n", actorMap["survival_rate"])
		}
	}
}

func getProperty(properties []*graph.Property, prop string) []byte {
	for _, p := range properties {
		if p.Prop == prop {
			return p.Val
		}
	}
	return nil
}

func TestToPB(t *testing.T) {
	dir, _ := populateGraph(t)
	defer os.RemoveAll(dir)

	query := `
		{
			debug(_uid_:0x1) {
				_xid_
				name
				gender
				status
				friend {
					name
				}
				friend {
				}
			}
		}
  `

	gq, _, err := gql.Parse(query)
	if err != nil {
		t.Error(err)
	}
	ctx := context.Background()
	sg, err := ToSubGraph(ctx, gq)
	if err != nil {
		t.Error(err)
	}

	ch := make(chan error)
	go ProcessGraph(ctx, sg, ch)
	err = <-ch
	if err != nil {
		t.Error(err)
	}

	var l Latency
	gr, err := sg.ToProtocolBuffer(&l)
	if err != nil {
		t.Error(err)
	}

	if gr.Attribute != "debug" {
		t.Errorf("Expected attribute me, Got: %v", gr.Attribute)
	}
	if gr.Uid != 1 {
		t.Errorf("Expected uid 1, Got: %v", gr.Uid)
	}
	if gr.Xid != "mich" {
		t.Errorf("Expected xid mich, Got: %v", gr.Xid)
	}
	if len(gr.Properties) != 3 {
		t.Errorf("Expected values map to contain 3 properties, Got: %v",
			len(gr.Properties))
	}
	if string(getProperty(gr.Properties, "name")) != "Michonne" {
		t.Errorf("Expected property name to have value Michonne, Got: %v",
			string(getProperty(gr.Properties, "name")))
	}
	if len(gr.Children) != 10 {
		t.Errorf("Expected 10 children, Got: %v", len(gr.Children))
	}

	child := gr.Children[0]
	if child.Uid != 23 {
		t.Errorf("Expected uid 23, Got: %v", gr.Uid)
	}
	if child.Attribute != "friend" {
		t.Errorf("Expected attribute friend, Got: %v", child.Attribute)
	}
	if len(child.Properties) != 1 {
		t.Errorf("Expected values map to contain 1 property, Got: %v",
			len(child.Properties))
	}
	if string(getProperty(child.Properties, "name")) != "Rick Grimes" {
		t.Errorf("Expected property name to have value Rick Grimes, Got: %v",
			string(getProperty(child.Properties, "name")))
	}
	if len(child.Children) != 0 {
		t.Errorf("Expected 0 children, Got: %v", len(child.Children))
	}

	child = gr.Children[5]
	if child.Uid != 23 {
		t.Errorf("Expected uid 23, Got: %v", gr.Uid)
	}
	if child.Attribute != "friend" {
		t.Errorf("Expected attribute friend, Got: %v", child.Attribute)
	}
	if len(child.Properties) != 0 {
		t.Errorf("Expected values map to contain 0 properties, Got: %v",
			len(child.Properties))
	}
	if len(child.Children) != 0 {
		t.Errorf("Expected 0 children, Got: %v", len(child.Children))
	}
}

func benchmarkToJson(file string, b *testing.B) {
	b.ReportAllocs()
	var sg SubGraph
	var l Latency

	f, err := ioutil.ReadFile(file)
	if err != nil {
		b.Error(err)
	}

	buf := bytes.NewBuffer(f)
	dec := gob.NewDecoder(buf)
	err = dec.Decode(&sg)
	if err != nil {
		b.Error(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := sg.ToJSON(&l); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkToJSON_10_Actor(b *testing.B)      { benchmarkToJson("benchmark/actors10.bin", b) }
func BenchmarkToJSON_10_Director(b *testing.B)   { benchmarkToJson("benchmark/directors10.bin", b) }
func BenchmarkToJSON_100_Actor(b *testing.B)     { benchmarkToJson("benchmark/actors100.bin", b) }
func BenchmarkToJSON_100_Director(b *testing.B)  { benchmarkToJson("benchmark/directors100.bin", b) }
func BenchmarkToJSON_1000_Actor(b *testing.B)    { benchmarkToJson("benchmark/actors1000.bin", b) }
func BenchmarkToJSON_1000_Director(b *testing.B) { benchmarkToJson("benchmark/directors1000.bin", b) }

func benchmarkToPB(file string, b *testing.B) {
	b.ReportAllocs()
	var sg SubGraph
	var l Latency

	f, err := ioutil.ReadFile(file)
	if err != nil {
		b.Error(err)
	}

	buf := bytes.NewBuffer(f)
	dec := gob.NewDecoder(buf)
	err = dec.Decode(&sg)
	if err != nil {
		b.Error(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pb, err := sg.ToProtocolBuffer(&l)
		if err != nil {
			b.Fatal(err)
		}
		r := new(graph.Response)
		r.N = pb
		var c Codec
		if _, err = c.Marshal(r); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkToPB_10_Actor(b *testing.B)      { benchmarkToPB("benchmark/actors10.bin", b) }
func BenchmarkToPB_10_Director(b *testing.B)   { benchmarkToPB("benchmark/directors10.bin", b) }
func BenchmarkToPB_100_Actor(b *testing.B)     { benchmarkToPB("benchmark/actors100.bin", b) }
func BenchmarkToPB_100_Director(b *testing.B)  { benchmarkToPB("benchmark/directors100.bin", b) }
func BenchmarkToPB_1000_Actor(b *testing.B)    { benchmarkToPB("benchmark/actors1000.bin", b) }
func BenchmarkToPB_1000_Director(b *testing.B) { benchmarkToPB("benchmark/directors1000.bin", b) }

func benchmarkToPBMarshal(file string, b *testing.B) {
	b.ReportAllocs()
	var sg SubGraph
	var l Latency

	f, err := ioutil.ReadFile(file)
	if err != nil {
		b.Error(err)
	}

	buf := bytes.NewBuffer(f)
	dec := gob.NewDecoder(buf)
	err = dec.Decode(&sg)
	if err != nil {
		b.Error(err)
	}
	p, err := sg.ToProtocolBuffer(&l)
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err = proto.Marshal(p); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkToPBMarshal_10_Actor(b *testing.B) {
	benchmarkToPBMarshal("benchmark/actors10.bin", b)
}
func BenchmarkToPBMarshal_10_Director(b *testing.B) {
	benchmarkToPBMarshal("benchmark/directors10.bin", b)
}
func BenchmarkToPBMarshal_100_Actor(b *testing.B) {
	benchmarkToPBMarshal("benchmark/actors100.bin", b)
}
func BenchmarkToPBMarshal_100_Director(b *testing.B) {
	benchmarkToPBMarshal("benchmark/directors100.bin", b)
}
func BenchmarkToPBMarshal_1000_Actor(b *testing.B) {
	benchmarkToPBMarshal("benchmark/actors1000.bin", b)
}
func BenchmarkToPBMarshal_1000_Director(b *testing.B) {
	benchmarkToPBMarshal("benchmark/directors1000.bin", b)
}

func benchmarkToPBUnmarshal(file string, b *testing.B) {
	b.ReportAllocs()
	var sg SubGraph
	var l Latency

	f, err := ioutil.ReadFile(file)
	if err != nil {
		b.Error(err)
	}

	buf := bytes.NewBuffer(f)
	dec := gob.NewDecoder(buf)
	err = dec.Decode(&sg)
	if err != nil {
		b.Error(err)
	}
	p, err := sg.ToProtocolBuffer(&l)
	if err != nil {
		b.Fatal(err)
	}

	pbb, err := proto.Marshal(p)
	if err != nil {
		b.Fatal(err)
	}

	pdu := &graph.Node{}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err = proto.Unmarshal(pbb, pdu)
		if err != nil {
			b.Fatal(err)
		}

	}
}

func BenchmarkToPBUnmarshal_10_Actor(b *testing.B) {
	benchmarkToPBUnmarshal("benchmark/actors10.bin", b)
}
func BenchmarkToPBUnmarshal_10_Director(b *testing.B) {
	benchmarkToPBUnmarshal("benchmark/directors10.bin", b)
}
func BenchmarkToPBUnmarshal_100_Actor(b *testing.B) {
	benchmarkToPBUnmarshal("benchmark/actors100.bin", b)
}
func BenchmarkToPBUnmarshal_100_Director(b *testing.B) {
	benchmarkToPBUnmarshal("benchmark/directors100.bin", b)
}
func BenchmarkToPBUnmarshal_1000_Actor(b *testing.B) {
	benchmarkToPBUnmarshal("benchmark/actors1000.bin", b)
}
func BenchmarkToPBUnmarshal_1000_Director(b *testing.B) {
	benchmarkToPBUnmarshal("benchmark/directors1000.bin", b)
}
