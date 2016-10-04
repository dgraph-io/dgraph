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
	"io/ioutil"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gogo/protobuf/proto"

	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/query/graph"
	"github.com/dgraph-io/dgraph/schema"
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
	if err := l.AddMutationWithIndex(context.Background(), edge, posting.Set); err != nil {
		t.Error(err)
	}
}

func checkName(t *testing.T, sg *SubGraph, idx int, expected string) {
	var tv task.Value
	if ok := sg.Values.Values(&tv, idx); !ok {
		t.Error("Unable to retrieve value")
	}
	name := tv.ValBytes()
	if string(name) != expected {
		t.Errorf("Expected: %v. Got: %v", expected, string(name))
	}
}

func checkSingleValue(t *testing.T, child *SubGraph, attr string, value string) {
	if child.Attr != attr || child.Result.Size() == 0 {
		t.Error("Expected attr name with some result", attr)
	}

	if child.Values.ValuesLength() != 1 {
		t.Errorf("Expected value length 1. Got: %v", child.Values.ValuesLength())
	}
	if child.Result.Size() != 1 {
		t.Errorf("Expected uidmatrix length 1. Got: %v", child.Result.Size())
	}

	if child.Result.Get(0).Size() != 0 {
		t.Errorf("Expected uids length 0. Got: %v", child.Result.Get(0).Size())
	}
	checkName(t, child, 0, value)
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

	worker.SetWorkerState(worker.NewState(ps, 0, 1))

	if sg.Result.Size() != 1 {
		t.Errorf("Expected length 1. Got: %v", sg.Result.Size())
	}
	if sg.Result.Get(0).Size() != 1 {
		t.Errorf("Expected length 1. Got: %v", sg.Result.Get(0).Size())
	}
	if sg.Result.Get(0).Get(0) != 101 {
		t.Errorf("Expected uid: %v. Got: %v", 101, sg.Result.Get(0).Get(0))
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

	worker.SetWorkerState(worker.NewState(ps, 0, 1))
	posting.Init()
	posting.ReadIndexConfigs([]byte(`{"config": [{"attribute": "name"}]}`))
	posting.InitIndex(ps)

	// So, user we're interested in has uid: 1.
	// She has 5 friends: 23, 24, 25, 31, and 101
	edge := x.DirectedEdge{
		ValueId:   23,
		Source:    "testing",
		Timestamp: time.Now(),
		Attribute: "friend",
	}
	addEdge(t, edge, getOrCreate(posting.Key(1, "friend"), ps))

	edge.ValueId = 24
	edge.Entity = 1
	addEdge(t, edge, getOrCreate(posting.Key(1, "friend"), ps))

	edge.ValueId = 25
	edge.Entity = 1
	addEdge(t, edge, getOrCreate(posting.Key(1, "friend"), ps))

	edge.ValueId = 31
	edge.Entity = 1
	addEdge(t, edge, getOrCreate(posting.Key(1, "friend"), ps))

	edge.ValueId = 101
	edge.Entity = 1
	addEdge(t, edge, getOrCreate(posting.Key(1, "friend"), ps))

	// Now let's add a few properties for the main user.
	edge.Value = []byte("Michonne")
	edge.Attribute = "name"
	edge.Entity = 1
	addEdge(t, edge, getOrCreate(posting.Key(1, "name"), ps))

	edge.Value = []byte("female")
	edge.Attribute = "gender"
	edge.Entity = 1
	addEdge(t, edge, getOrCreate(posting.Key(1, "gender"), ps))

	edge.Value, _ = types.Int32(15).MarshalBinary()
	edge.ValueType = byte(types.Int32Type.ID())
	edge.Attribute = "age"
	edge.Entity = 1
	addEdge(t, edge, getOrCreate(posting.Key(1, "age"), ps))
	edge.ValueType = 0

	edge.Value = []byte("31, 32 street, Jupiter")
	edge.Attribute = "address"
	edge.Entity = 1
	addEdge(t, edge, getOrCreate(posting.Key(1, "address"), ps))

	edge.Value, _ = types.Bool(true).MarshalBinary()
	edge.Attribute = "alive"
	edge.ValueType = byte(types.BooleanType.ID())
	edge.Entity = 1
	addEdge(t, edge, getOrCreate(posting.Key(1, "alive"), ps))
	edge.ValueType = 0

	edge.Value = []byte("38")
	edge.Attribute = "age"
	edge.Entity = 1
	addEdge(t, edge, getOrCreate(posting.Key(1, "age"), ps))

	edge.Value = []byte("98.99%")
	edge.Attribute = "survival_rate"
	edge.Entity = 1
	addEdge(t, edge, getOrCreate(posting.Key(1, "survival_rate"), ps))

	edge.Value = []byte("true")
	edge.Attribute = "sword_present"
	edge.Entity = 1
	addEdge(t, edge, getOrCreate(posting.Key(1, "sword_present"), ps))

	// Now let's add a name for each of the friends, except 101.
	edge.Value = []byte("Rick Grimes")
	edge.Attribute = "name"
	edge.Entity = 23
	edge.ValueType = byte(types.StringType.ID())
	addEdge(t, edge, getOrCreate(posting.Key(23, "name"), ps))
	edge.ValueType = 0

	edge.Value = []byte("15")
	addEdge(t, edge, getOrCreate(posting.Key(23, "age"), ps))

	edge.Value = []byte("21, mark street, Mars")
	addEdge(t, edge, getOrCreate(posting.Key(23, "address"), ps))

	edge.Value = []byte("Glenn Rhee")
	edge.Entity = 24
	addEdge(t, edge, getOrCreate(posting.Key(24, "name"), ps))

	edge.Value = []byte("Daryl Dixon")
	edge.Entity = 25
	addEdge(t, edge, getOrCreate(posting.Key(25, "name"), ps))

	edge.Value = []byte("Andrea")
	edge.Entity = 31
	addEdge(t, edge, getOrCreate(posting.Key(31, "name"), ps))

	edge.Value = []byte("mich")
	edge.Attribute = "_xid_"
	edge.Entity = 1
	addEdge(t, edge, getOrCreate(posting.Key(1, "_xid_"), ps))

	time.Sleep(200 * time.Millisecond) // Let the index process jobs from channel.
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
	go ProcessGraph(ctx, sg, nil, ch)
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

func TestSchema1(t *testing.T) {
	err := schema.Parse("test_schema")

	if err != nil {
		t.Error(err)
	}

	dir, _ := populateGraph(t)
	defer os.RemoveAll(dir)

	// Alright. Now we have everything set up. Let's create the query.
	query := `
		{
			person(_uid_:0x01) {
				alive
				survival_rate
				friend
			}
		}
	`
	mp := processToJson(t, query)
	resp := mp["person"]
	name := resp.([]interface{})[0].(map[string]interface{})["name"].(string)
	if name != "Michonne" {
		t.Errorf("Expected name Michonne. Got %s", name)
	}
	if alive, ok := resp.([]interface{})[0].(map[string]interface{})["alive"]; !ok || !alive.(bool) {
		t.Errorf("Expected alive true. Got %v ", alive)
	}

	if age, ok := resp.([]interface{})[0].(map[string]interface{})["age"]; !ok || (age.(float64) != 38) {
		t.Errorf("Expected age 38. Got %v", age)
	}

	if _, ok := resp.([]interface{})[0].(map[string]interface{})["survival_rate"]; !ok {
		t.Error("Expected survival rate as its not part of person")
	}

	friends := resp.([]interface{})[0].(map[string]interface{})["friend"].([]interface{})
	co := 0
	res := 0
	for _, it := range friends {
		if len(it.(map[string]interface{})) == 0 {
			co++
		} else {
			res = len(it.(map[string]interface{}))
		}
	}
	if co != 4 {
		t.Error("Invalid result")
	}
	if res != 3 {
		t.Error("Invalid result")
	}

	actorMap := mp["person"].([]interface{})[0].(map[string]interface{})
	if _, success := actorMap["name"].(string); !success {
		t.Errorf("Expected json type string for: %v\n", actorMap["name"])
	}
	// json parses ints as floats
	if _, success := actorMap["age"].(float64); !success {
		t.Errorf("Expected json type int for: %v\n", actorMap["age"])
	}
	if _, success := actorMap["survival_rate"].(float64); success {
		t.Errorf("Survival rate not part of person, so it doesnt have to be coerced: %v\n", actorMap["survival_rate"])
	}
}

func TestGetUID(t *testing.T) {
	dir, _ := populateGraph(t)
	defer os.RemoveAll(dir)

	// Alright. Now we have everything set up. Let's create the query.
	query := `
		{
			me(_uid_:0x01) {
				name
				_uid_
				gender
				alive
				friend {
					_count_
				}
			}
		}
	`
	mp := processToJson(t, query)
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
				alive
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
				alive
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
				alive
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
				alive
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
				alive
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
				alive	
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
	go ProcessGraph(ctx, sg, nil, ch)
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
	if child.Result.Size() == 0 {
		t.Errorf("Expected some.Result.")
		return
	}

	if sg.Result.Size() != 1 {
		t.Errorf("Expected 1 matrix. Got: %v", sg.Result.Size())
	}

	ul := child.Result.Get(0)
	if ul.Size() != 5 {
		t.Errorf("Expected 5 friends. Got: %v", ul.Size())
	}
	if ul.Get(0) != 23 || ul.Get(1) != 24 || ul.Get(2) != 25 ||
		ul.Get(3) != 31 || ul.Get(4) != 101 {
		t.Errorf("Friend ids don't match")
	}
	if len(child.Children) != 1 || child.Children[0].Attr != "name" {
		t.Errorf("Expected attr name")
	}
	child = child.Children[0]

	values := child.Values
	if values.ValuesLength() != 5 {
		t.Errorf("Expected 5 names of 5 friends")
	}
	checkName(t, child, 0, "Rick Grimes")
	checkName(t, child, 1, "Glenn Rhee")
	checkName(t, child, 2, "Daryl Dixon")
	checkName(t, child, 3, "Andrea")
	{
		var tv task.Value
		if ok := values.Values(&tv, 4); !ok {
			t.Error("Unable to retrieve value")
		}
		if !bytes.Equal(tv.ValBytes(), []byte{}) {
			t.Error("Expected a null byte slice")
		}
	}

	checkSingleValue(t, sg.Children[1], "name", "Michonne")
	checkSingleValue(t, sg.Children[2], "gender", "female")
}

func TestToJSON(t *testing.T) {
	dir, _ := populateGraph(t)
	defer os.RemoveAll(dir)

	// Alright. Now we have everything set up. Let's create the query.
	query := `
		{
			me(_uid_:0x01) {
				name
				gender
			  alive	
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
	go ProcessGraph(ctx, sg, nil, ch)
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

func TestToJSONFilter(t *testing.T) {
	dir, _ := populateGraph(t)
	defer os.RemoveAll(dir)
	query := `
		{
			me(_uid_:0x01) {
				name
				gender
				friend @filter(eq("name", "Andrea")) {
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
	go ProcessGraph(ctx, sg, nil, ch)
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
	if s != `{"me":[{"friend":[{"name":"Andrea"}],"gender":"female","name":"Michonne"}]}` {
		t.Errorf("Wrong output: %s", s)
	}
}

func TestToJSONFilterUID(t *testing.T) {
	dir, _ := populateGraph(t)
	defer os.RemoveAll(dir)
	query := `
		{
			me(_uid_:0x01) {
				name
				gender
				friend @filter(eq("name", "Andrea")) {
					_uid_
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
	go ProcessGraph(ctx, sg, nil, ch)
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
	if s != `{"me":[{"friend":[{"_uid_":"0x1f"}],"gender":"female","name":"Michonne"}]}` {
		t.Errorf("Wrong output: %s", s)
	}
}

func TestToJSONFilterOr(t *testing.T) {
	dir, _ := populateGraph(t)
	defer os.RemoveAll(dir)
	query := `
		{
			me(_uid_:0x01) {
				name
				gender
				friend @filter(eq("name", "Andrea") || eq("name", "Glenn Rhee")) {
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
	go ProcessGraph(ctx, sg, nil, ch)
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
	if s != `{"me":[{"friend":[{"name":"Glenn Rhee"},{"name":"Andrea"}],"gender":"female","name":"Michonne"}]}` {
		t.Errorf("Wrong output: %s", s)
	}
}

func TestToJSONFilterOrFirst(t *testing.T) {
	dir, _ := populateGraph(t)
	defer os.RemoveAll(dir)
	query := `
		{
			me(_uid_:0x01) {
				name
				gender
				friend(first:2) @filter(eq("name", "Andrea") || eq("name", "Glenn Rhee") || eq("name", "Daryl Dixon")) {
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
	go ProcessGraph(ctx, sg, nil, ch)
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
	if s != `{"me":[{"friend":[{"name":"Glenn Rhee"},{"name":"Daryl Dixon"}],"gender":"female","name":"Michonne"}]}` {
		t.Errorf("Wrong output: %s", s)
	}
}

func TestToJSONFilterOrOffset(t *testing.T) {
	dir, _ := populateGraph(t)
	defer os.RemoveAll(dir)
	query := `
		{
			me(_uid_:0x01) {
				name
				gender
				friend(offset:1) @filter(eq("name", "Andrea") || eq("name", "Glenn Rhee") || eq("name", "Daryl Dixon")) {
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
	go ProcessGraph(ctx, sg, nil, ch)
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
	if s != `{"me":[{"friend":[{"name":"Daryl Dixon"},{"name":"Andrea"}],"gender":"female","name":"Michonne"}]}` {
		t.Errorf("Wrong output: %s", s)
	}
}

// No filter. Just to test first and offset.
func TestToJSONFirstOffset(t *testing.T) {
	dir, _ := populateGraph(t)
	defer os.RemoveAll(dir)
	query := `
		{
			me(_uid_:0x01) {
				name
				gender
				friend(offset:1, first:1) {
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
	go ProcessGraph(ctx, sg, nil, ch)
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
	if s != `{"me":[{"friend":[{"name":"Glenn Rhee"}],"gender":"female","name":"Michonne"}]}` {
		t.Errorf("Wrong output: %s", s)
	}
}

func TestToJSONFilterOrFirstOffset(t *testing.T) {
	dir, _ := populateGraph(t)
	defer os.RemoveAll(dir)
	query := `
		{
			me(_uid_:0x01) {
				name
				gender
				friend(offset:1, first:1) @filter(eq("name", "Andrea") || eq("name", "Glenn Rhee") || eq("name", "Daryl Dixon")) {
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
	go ProcessGraph(ctx, sg, nil, ch)
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
	if s != `{"me":[{"friend":[{"name":"Daryl Dixon"}],"gender":"female","name":"Michonne"}]}` {
		t.Errorf("Wrong output: %s", s)
	}
}

func TestToJSONFilterOrFirstNegative(t *testing.T) {
	dir, _ := populateGraph(t)
	defer os.RemoveAll(dir)
	// When negative first/count is specified, we ignore offset and returns the last
	// few number of items.
	query := `
		{
			me(_uid_:0x01) {
				name
				gender
				friend(first:-1, offset:0) @filter(eq("name", "Andrea") || eq("name", "Glenn Rhee") || eq("name", "Daryl Dixon")) {
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
	go ProcessGraph(ctx, sg, nil, ch)
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
	if s != `{"me":[{"friend":[{"name":"Andrea"}],"gender":"female","name":"Michonne"}]}` {
		t.Errorf("Wrong output: %s", s)
	}
}

func TestToJSONFilterAnd(t *testing.T) {
	dir, _ := populateGraph(t)
	defer os.RemoveAll(dir)
	query := `
		{
			me(_uid_:0x01) {
				name
				gender
				friend @filter(eq("name", "Andrea") && eq("name", "Glenn Rhee")) {
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
	go ProcessGraph(ctx, sg, nil, ch)
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
	if s != `{"me":[{"gender":"female","name":"Michonne"}]}` {
		t.Errorf("Wrong output: %s", s)
	}
}

func getProperty(properties []*graph.Property, prop string) *graph.Value {
	for _, p := range properties {
		if p.Prop == prop {
			return p.Value
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
				alive
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
	go ProcessGraph(ctx, sg, nil, ch)
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
	if string(getProperty(gr.Properties, "name").GetBytesVal()) != "Michonne" {
		t.Errorf("Expected property name to have value Michonne, Got: %v",
			getProperty(gr.Properties, "name").GetBytesVal())
	}
	if getProperty(gr.Properties, "alive").GetBoolVal() != true {
		t.Errorf("Expected property age to have value true, Got: %v",
			getProperty(gr.Properties, "alive").GetBoolVal())
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
	if getProperty(child.Properties, "name").GetStrVal() != "Rick Grimes" {
		t.Errorf("Expected property name to have value Rick Grimes, Got: %v",
			getProperty(child.Properties, "name").GetStrVal())
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

func TestSchema(t *testing.T) {
	dir, _ := populateGraph(t)
	defer os.RemoveAll(dir)

	query := `
		{
			debug(_uid_:0x1) {
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
	go ProcessGraph(ctx, sg, nil, ch)
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
	if string(getProperty(gr.Properties, "name").GetBytesVal()) != "Michonne" {
		t.Errorf("Expected property name to have value Michonne, Got: %v",
			getProperty(gr.Properties, "name").GetBytesVal())
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
	if getProperty(child.Properties, "name").GetStrVal() != "Rick Grimes" {
		t.Errorf("Expected property name to have value Rick Grimes, Got: %v",
			getProperty(child.Properties, "name").GetStrVal())
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

func TestToPBFilter(t *testing.T) {
	dir, _ := populateGraph(t)
	defer os.RemoveAll(dir)

	// Alright. Now we have everything set up. Let's create the query.
	query := `
		{
			me(_uid_:0x01) {
				name
				gender
				friend @filter(eq("name", "Andrea")) {
					name
				}
			}
		}
	`

	gq, _, err := gql.Parse(query)
	if err != nil {
		t.Error(err)
		return
	}
	ctx := context.Background()
	sg, err := ToSubGraph(ctx, gq)
	if err != nil {
		t.Error(err)
		return
	}

	ch := make(chan error)
	go ProcessGraph(ctx, sg, nil, ch)
	err = <-ch
	if err != nil {
		t.Error(err)
		return
	}

	var l Latency
	pb, err := sg.ToProtocolBuffer(&l)
	if err != nil {
		t.Error(err)
		return
	}

	expectedPb := `attribute: "me"
properties: <
  prop: "name"
  value: <
    bytes_val: "Michonne"
  >
>
properties: <
  prop: "gender"
  value: <
    bytes_val: "female"
  >
>
children: <
  attribute: "friend"
  properties: <
    prop: "name"
    value: <
      bytes_val: "Andrea"
    >
  >
>
`
	pbText := proto.MarshalTextString(pb)
	if pbText != expectedPb {
		t.Errorf("Output wrong: %v", pbText)
		return
	}
}

func TestToPBFilterOr(t *testing.T) {
	dir, _ := populateGraph(t)
	defer os.RemoveAll(dir)

	// Alright. Now we have everything set up. Let's create the query.
	query := `
		{
			me(_uid_:0x01) {
				name
				gender
				friend @filter(eq("name", "Andrea") || eq("name", "Glenn Rhee")) {
					name
				}
			}
		}
	`

	gq, _, err := gql.Parse(query)
	if err != nil {
		t.Error(err)
		return
	}
	ctx := context.Background()
	sg, err := ToSubGraph(ctx, gq)
	if err != nil {
		t.Error(err)
		return
	}

	ch := make(chan error)
	go ProcessGraph(ctx, sg, nil, ch)
	err = <-ch
	if err != nil {
		t.Error(err)
		return
	}

	var l Latency
	pb, err := sg.ToProtocolBuffer(&l)
	if err != nil {
		t.Error(err)
		return
	}

	expectedPb := `attribute: "me"
properties: <
  prop: "name"
  value: <
    bytes_val: "Michonne"
  >
>
properties: <
  prop: "gender"
  value: <
    bytes_val: "female"
  >
>
children: <
  attribute: "friend"
  properties: <
    prop: "name"
    value: <
      bytes_val: "Glenn Rhee"
    >
  >
>
children: <
  attribute: "friend"
  properties: <
    prop: "name"
    value: <
      bytes_val: "Andrea"
    >
  >
>
`
	pbText := proto.MarshalTextString(pb)
	if pbText != expectedPb {
		t.Errorf("Output wrong: %v", pbText)
		return
	}
}

func TestToPBFilterAnd(t *testing.T) {
	dir, _ := populateGraph(t)
	defer os.RemoveAll(dir)

	// Alright. Now we have everything set up. Let's create the query.
	query := `
		{
			me(_uid_:0x01) {
				name
				gender
				friend @filter(eq("name", "Andrea") && eq("name", "Glenn Rhee")) {
					name
				}
			}
		}
	`

	gq, _, err := gql.Parse(query)
	if err != nil {
		t.Error(err)
		return
	}
	ctx := context.Background()
	sg, err := ToSubGraph(ctx, gq)
	if err != nil {
		t.Error(err)
		return
	}

	ch := make(chan error)
	go ProcessGraph(ctx, sg, nil, ch)
	err = <-ch
	if err != nil {
		t.Error(err)
		return
	}

	var l Latency
	pb, err := sg.ToProtocolBuffer(&l)
	if err != nil {
		t.Error(err)
		return
	}

	expectedPb := `attribute: "me"
properties: <
  prop: "name"
  value: <
    bytes_val: "Michonne"
  >
>
properties: <
  prop: "gender"
  value: <
    bytes_val: "female"
  >
>
`
	pbText := proto.MarshalTextString(pb)
	if pbText != expectedPb {
		t.Errorf("Output wrong: %v", pbText)
		return
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
