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
	"testing"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/algo"
	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/index"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/query/graph"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/store"
	"github.com/dgraph-io/dgraph/task"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/worker"
	"github.com/dgraph-io/dgraph/x"
)

func init() {
	worker.ParseGroupConfig("")
	worker.StartRaftNodes(1, "localhost:12345", "1:localhost:12345", "")
	// Wait for the node to become leader for group 0.
	time.Sleep(5 * time.Second)
}

func childAttrs(sg *SubGraph) []string {
	var out []string
	for _, c := range sg.Children {
		out = append(out, c.Attr)
	}
	return out
}

func taskValues(t *testing.T, v *task.ValueList) []string {
	out := make([]string, v.ValuesLength())
	for i := 0; i < v.ValuesLength(); i++ {
		var tv task.Value
		require.True(t, v.Values(&tv, i))
		out[i] = string(tv.ValBytes())
	}
	return out
}

func TestNewGraph(t *testing.T) {
	dir, err := ioutil.TempDir("", "storetest_")
	require.NoError(t, err)

	gq := &gql.GraphQuery{
		UID:  101,
		Attr: "me",
	}
	ps, err := store.NewStore(dir)
	require.NoError(t, err)

	ctx := context.Background()
	sg, err := newGraph(ctx, gq)
	require.NoError(t, err)

	worker.SetState(ps)

	require.EqualValues(t,
		[][]uint64{
			[]uint64{101},
		}, algo.ToUintsListForTest(sg.Result))
}

const schemaStr = `
scalar name:string @index
scalar dob:date @index`

func addEdgeToValue(t *testing.T, ps *store.Store, attr string, src uint64,
	value string) {
	edge := x.DirectedEdge{
		Value:     []byte(value),
		Source:    "testing",
		Timestamp: time.Now(),
		Attribute: attr,
		Entity:    src,
	}
	l, _ := posting.GetOrCreate(posting.Key(src, attr), ps)
	require.NoError(t,
		l.AddMutationWithIndex(context.Background(), edge, posting.Set))
}

func addEdgeToTypedValue(t *testing.T, ps *store.Store, attr string, src uint64,
	typ types.TypeID, value []byte) {
	edge := x.DirectedEdge{
		Value:     value,
		ValueType: byte(typ),
		Source:    "testing",
		Timestamp: time.Now(),
		Attribute: attr,
		Entity:    src,
	}
	l, _ := posting.GetOrCreate(posting.Key(src, attr), ps)
	require.NoError(t,
		l.AddMutationWithIndex(context.Background(), edge, posting.Set))
}

func addEdgeToUID(t *testing.T, ps *store.Store, attr string, src uint64, dst uint64) {
	edge := x.DirectedEdge{
		ValueId:   dst,
		Source:    "testing",
		Timestamp: time.Now(),
		Attribute: attr,
		Entity:    src,
	}
	l, _ := posting.GetOrCreate(posting.Key(src, attr), ps)
	require.NoError(t,
		l.AddMutationWithIndex(context.Background(), edge, posting.Set))
}

func populateGraph(t *testing.T) (string, *store.Store) {
	// logrus.SetLevel(logrus.DebugLevel)
	dir, err := ioutil.TempDir("", "storetest_")
	require.NoError(t, err)

	ps, err := store.NewStore(dir)
	require.NoError(t, err)

	worker.SetState(ps)
	posting.Init()
	schema.ParseBytes([]byte(schemaStr))
	posting.InitIndex(ps)
	worker.InitIndex()
	index.InitIndex(ps)

	// So, user we're interested in has uid: 1.
	// She has 5 friends: 23, 24, 25, 31, and 101
	addEdgeToUID(t, ps, "friend", 1, 23)
	addEdgeToUID(t, ps, "friend", 1, 24)
	addEdgeToUID(t, ps, "friend", 1, 25)
	addEdgeToUID(t, ps, "friend", 1, 31)
	addEdgeToUID(t, ps, "friend", 1, 101)

	// Now let's add a few properties for the main user.
	addEdgeToValue(t, ps, "name", 1, "Michonne")
	addEdgeToValue(t, ps, "gender", 1, "female")
	data, err := types.Int32(15).MarshalBinary()
	require.NoError(t, err)
	addEdgeToTypedValue(t, ps, "age", 1, types.Int32ID, data)
	addEdgeToValue(t, ps, "address", 1, "31, 32 street, Jupiter")
	data, err = types.Bool(true).MarshalBinary()
	require.NoError(t, err)
	addEdgeToTypedValue(t, ps, "alive", 1, types.BoolID, data)
	addEdgeToValue(t, ps, "age", 1, "38")
	addEdgeToValue(t, ps, "survival_rate", 1, "98.99")
	addEdgeToValue(t, ps, "sword_present", 1, "true")
	addEdgeToValue(t, ps, "_xid_", 1, "mich")

	// Now let's add a name for each of the friends, except 101.
	addEdgeToTypedValue(t, ps, "name", 23, types.StringID, []byte("Rick Grimes"))
	addEdgeToValue(t, ps, "age", 23, "15")

	addEdgeToValue(t, ps, "address", 23, "21, mark street, Mars")
	addEdgeToValue(t, ps, "name", 24, "Glenn Rhee")
	addEdgeToValue(t, ps, "name", 25, "Daryl Dixon")
	addEdgeToValue(t, ps, "name", 31, "Andrea")

	addEdgeToValue(t, ps, "dob", 23, "1910-05-02")
	addEdgeToValue(t, ps, "dob", 24, "1909-01-05")
	addEdgeToValue(t, ps, "dob", 25, "1909-05-10")
	addEdgeToValue(t, ps, "dob", 31, "1901-01-15")

	time.Sleep(200 * time.Millisecond) // Let the index process jobs from channel.
	return dir, ps
}

func processToJSON(t *testing.T, query string) map[string]interface{} {
	gq, _, err := gql.Parse(query)
	require.NoError(t, err)

	ctx := context.Background()
	sg, err := ToSubGraph(ctx, gq)
	require.NoError(t, err)

	ch := make(chan error)
	go ProcessGraph(ctx, sg, nil, ch)
	err = <-ch
	require.NoError(t, err)

	var l Latency
	js, err := sg.ToJSON(&l)
	require.NoError(t, err)

	var mp map[string]interface{}
	require.NoError(t, json.Unmarshal(js, &mp))

	return mp
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
	mp := processToJSON(t, query)
	resp := mp["me"]
	uid := resp.([]interface{})[0].(map[string]interface{})["_uid_"].(string)
	require.EqualValues(t, "0x1", uid)
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

	mp := processToJSON(t, query)
	resp := mp["debug"]
	uid := resp.([]interface{})[0].(map[string]interface{})["_uid_"].(string)
	require.EqualValues(t, "0x1", uid)
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

	mp := processToJSON(t, query)
	resp := mp["me"]
	uid, ok := resp.([]interface{})[0].(map[string]interface{})["_uid_"].(string)
	require.False(t, ok, "No uid expected but got one %s", uid)
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

	mp := processToJSON(t, query)
	resp := mp["me"]
	friend := resp.([]interface{})[0].(map[string]interface{})["friend"]
	count := int(friend.(map[string]interface{})["_count_"].(float64))
	require.EqualValues(t, count, 5)
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
	require.NoError(t, err)

	ctx := context.Background()
	_, err = ToSubGraph(ctx, gq)
	require.Error(t, err)
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
	require.NoError(t, err)

	ctx := context.Background()
	_, err = ToSubGraph(ctx, gq)
	require.Error(t, err)
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
	require.NoError(t, err)

	ctx := context.Background()
	sg, err := ToSubGraph(ctx, gq)
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
		}, algo.ToUintsListForTest(child.Result))

	require.EqualValues(t, []string{"name"}, childAttrs(child))

	child = child.Children[0]
	require.EqualValues(t,
		[]string{"Rick Grimes", "Glenn Rhee", "Daryl Dixon", "Andrea", ""},
		taskValues(t, child.Values))

	require.EqualValues(t, []string{"Michonne"},
		taskValues(t, sg.Children[1].Values))
	require.EqualValues(t, []string{"female"},
		taskValues(t, sg.Children[2].Values))
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
	require.NoError(t, err)

	ctx := context.Background()
	sg, err := ToSubGraph(ctx, gq)
	require.NoError(t, err)

	ch := make(chan error)
	go ProcessGraph(ctx, sg, nil, ch)
	err = <-ch
	require.NoError(t, err)

	var l Latency
	js, err := sg.ToJSON(&l)
	require.NoError(t, err)
	require.Contains(t, string(js), "Michonne")
}

func TestToJSONFilter(t *testing.T) {
	dir, _ := populateGraph(t)
	defer os.RemoveAll(dir)
	query := `
		{
			me(_uid_:0x01) {
				name
				gender
				friend @filter(anyof("name", "Andrea SomethingElse")) {
					name
				}
			}
		}
	`

	gq, _, err := gql.Parse(query)
	require.NoError(t, err)

	ctx := context.Background()
	sg, err := ToSubGraph(ctx, gq)
	require.NoError(t, err)

	ch := make(chan error)
	go ProcessGraph(ctx, sg, nil, ch)
	err = <-ch
	require.NoError(t, err)

	var l Latency
	js, err := sg.ToJSON(&l)
	require.NoError(t, err)
	require.EqualValues(t,
		`{"me":[{"friend":[{"name":"Andrea"}],"gender":"female","name":"Michonne"}]}`,
		string(js))
}

func TestToJSONFilterAllOf(t *testing.T) {
	dir, _ := populateGraph(t)
	defer os.RemoveAll(dir)
	query := `
		{
			me(_uid_:0x01) {
				name
				gender
				friend @filter(allof("name", "Andrea SomethingElse")) {
					name
				}
			}
		}
	`

	gq, _, err := gql.Parse(query)
	require.NoError(t, err)

	ctx := context.Background()
	sg, err := ToSubGraph(ctx, gq)
	require.NoError(t, err)

	ch := make(chan error)
	go ProcessGraph(ctx, sg, nil, ch)
	err = <-ch
	require.NoError(t, err)

	var l Latency
	js, err := sg.ToJSON(&l)
	require.NoError(t, err)
	require.EqualValues(t,
		`{"me":[{"gender":"female","name":"Michonne"}]}`,
		string(js))
}

func TestToJSONFilterUID(t *testing.T) {
	dir, _ := populateGraph(t)
	defer os.RemoveAll(dir)
	query := `
		{
			me(_uid_:0x01) {
				name
				gender
				friend @filter(anyof("name", "Andrea")) {
					_uid_
				}
			}
		}
	`

	gq, _, err := gql.Parse(query)
	require.NoError(t, err)

	ctx := context.Background()
	sg, err := ToSubGraph(ctx, gq)
	require.NoError(t, err)

	ch := make(chan error)
	go ProcessGraph(ctx, sg, nil, ch)
	err = <-ch
	require.NoError(t, err)

	var l Latency
	js, err := sg.ToJSON(&l)
	require.NoError(t, err)
	require.EqualValues(t, js,
		`{"me":[{"friend":[{"_uid_":"0x1f"}],"gender":"female","name":"Michonne"}]}`)
}

func TestToJSONFilterOr(t *testing.T) {
	dir, _ := populateGraph(t)
	defer os.RemoveAll(dir)
	query := `
		{
			me(_uid_:0x01) {
				name
				gender
				friend @filter(anyof("name", "Andrea") || anyof("name", "Andrea Rhee")) {
					name
				}
			}
		}
	`

	gq, _, err := gql.Parse(query)
	require.NoError(t, err)

	ctx := context.Background()
	sg, err := ToSubGraph(ctx, gq)
	require.NoError(t, err)

	ch := make(chan error)
	go ProcessGraph(ctx, sg, nil, ch)
	err = <-ch
	require.NoError(t, err)

	var l Latency
	js, err := sg.ToJSON(&l)
	require.NoError(t, err)
	require.EqualValues(t, js,
		`{"me":[{"friend":[{"name":"Glenn Rhee"},{"name":"Andrea"}],"gender":"female","name":"Michonne"}]}`)
}

func TestToJSONFilterOrFirst(t *testing.T) {
	dir, _ := populateGraph(t)
	defer os.RemoveAll(dir)
	query := `
		{
			me(_uid_:0x01) {
				name
				gender
				friend(first:2) @filter(anyof("name", "Andrea") || anyof("name", "Glenn SomethingElse") || anyof("name", "Daryl")) {
					name
				}
			}
		}
	`

	gq, _, err := gql.Parse(query)
	require.NoError(t, err)

	ctx := context.Background()
	sg, err := ToSubGraph(ctx, gq)
	require.NoError(t, err)

	ch := make(chan error)
	go ProcessGraph(ctx, sg, nil, ch)
	err = <-ch
	require.NoError(t, err)

	var l Latency
	js, err := sg.ToJSON(&l)
	require.NoError(t, err)
	require.EqualValues(t, js,
		`{"me":[{"friend":[{"name":"Glenn Rhee"},{"name":"Daryl Dixon"}],"gender":"female","name":"Michonne"}]}`)
}

func TestToJSONFilterOrOffset(t *testing.T) {
	dir, _ := populateGraph(t)
	defer os.RemoveAll(dir)
	query := `
		{
			me(_uid_:0x01) {
				name
				gender
				friend(offset:1) @filter(anyof("name", "Andrea") || anyof("name", "Glenn Rhee") || anyof("name", "Daryl Dixon")) {
					name
				}
			}
		}
	`

	gq, _, err := gql.Parse(query)
	require.NoError(t, err)

	ctx := context.Background()
	sg, err := ToSubGraph(ctx, gq)
	require.NoError(t, err)

	ch := make(chan error)
	go ProcessGraph(ctx, sg, nil, ch)
	err = <-ch
	require.NoError(t, err)

	var l Latency
	js, err := sg.ToJSON(&l)
	require.NoError(t, err)
	require.EqualValues(t, js,
		`{"me":[{"friend":[{"name":"Daryl Dixon"},{"name":"Andrea"}],"gender":"female","name":"Michonne"}]}`)
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
	require.NoError(t, err)

	ctx := context.Background()
	sg, err := ToSubGraph(ctx, gq)
	require.NoError(t, err)

	ch := make(chan error)
	go ProcessGraph(ctx, sg, nil, ch)
	err = <-ch
	require.NoError(t, err)

	var l Latency
	js, err := sg.ToJSON(&l)
	require.NoError(t, err)
	require.EqualValues(t, js,
		`{"me":[{"friend":[{"name":"Glenn Rhee"}],"gender":"female","name":"Michonne"}]}`)
}

func TestToJSONFilterOrFirstOffset(t *testing.T) {
	dir, _ := populateGraph(t)
	defer os.RemoveAll(dir)
	query := `
		{
			me(_uid_:0x01) {
				name
				gender
				friend(offset:1, first:1) @filter(anyof("name", "Andrea") || anyof("name", "SomethingElse Rhee") || anyof("name", "Daryl Dixon")) {
					name
				}
			}
		}
	`

	gq, _, err := gql.Parse(query)
	require.NoError(t, err)

	ctx := context.Background()
	sg, err := ToSubGraph(ctx, gq)
	require.NoError(t, err)

	ch := make(chan error)
	go ProcessGraph(ctx, sg, nil, ch)
	err = <-ch
	require.NoError(t, err)

	var l Latency
	js, err := sg.ToJSON(&l)
	require.NoError(t, err)
	require.EqualValues(t, js,
		`{"me":[{"friend":[{"name":"Daryl Dixon"}],"gender":"female","name":"Michonne"}]}`)
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
				friend(first:-1, offset:0) @filter(anyof("name", "Andrea") || anyof("name", "Glenn Rhee") || anyof("name", "Daryl Dixon")) {
					name
				}
			}
		}
	`

	gq, _, err := gql.Parse(query)
	require.NoError(t, err)

	ctx := context.Background()
	sg, err := ToSubGraph(ctx, gq)
	require.NoError(t, err)

	ch := make(chan error)
	go ProcessGraph(ctx, sg, nil, ch)
	err = <-ch
	require.NoError(t, err)

	var l Latency
	js, err := sg.ToJSON(&l)
	require.NoError(t, err)
	require.EqualValues(t, js,
		`{"me":[{"friend":[{"name":"Andrea"}],"gender":"female","name":"Michonne"}]}`)
}

func TestToJSONFilterAnd(t *testing.T) {
	dir, _ := populateGraph(t)
	defer os.RemoveAll(dir)
	query := `
		{
			me(_uid_:0x01) {
				name
				gender
				friend @filter(anyof("name", "Andrea") && anyof("name", "SomethingElse Rhee")) {
					name
				}
			}
		}
	`

	gq, _, err := gql.Parse(query)
	require.NoError(t, err)

	ctx := context.Background()
	sg, err := ToSubGraph(ctx, gq)
	require.NoError(t, err)

	ch := make(chan error)
	go ProcessGraph(ctx, sg, nil, ch)
	err = <-ch
	require.NoError(t, err)

	var l Latency
	js, err := sg.ToJSON(&l)
	require.NoError(t, err)
	require.EqualValues(t, js,
		`{"me":[{"gender":"female","name":"Michonne"}]}`)
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
	require.NoError(t, err)

	ctx := context.Background()
	sg, err := ToSubGraph(ctx, gq)
	require.NoError(t, err)

	ch := make(chan error)
	go ProcessGraph(ctx, sg, nil, ch)
	err = <-ch
	require.NoError(t, err)

	var l Latency
	gr, err := sg.ToProtocolBuffer(&l)
	require.NoError(t, err)

	require.EqualValues(t, "debug", gr.Attribute)
	require.EqualValues(t, 1, gr.Uid)
	require.EqualValues(t, "mich", gr.Xid)
	require.Len(t, gr.Properties, 3)

	require.EqualValues(t, "Michonne",
		getProperty(gr.Properties, "name").GetStrVal())
	require.Len(t, gr.Children, 10)

	child := gr.Children[0]
	require.EqualValues(t, 23, child.Uid)
	require.EqualValues(t, "friend", child.Attribute)

	require.Len(t, child.Properties, 1)
	require.EqualValues(t, "Rick Grimes",
		getProperty(child.Properties, "name").GetStrVal())
	require.Empty(t, child.Children)

	child = gr.Children[5]
	require.EqualValues(t, 23, child.Uid)
	require.EqualValues(t, "friend", child.Attribute)
	require.Empty(t, child.Properties)
	require.Empty(t, child.Children)
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
	require.NoError(t, err)

	ctx := context.Background()
	sg, err := ToSubGraph(ctx, gq)
	require.NoError(t, err)

	ch := make(chan error)
	go ProcessGraph(ctx, sg, nil, ch)
	err = <-ch
	require.NoError(t, err)

	var l Latency
	gr, err := sg.ToProtocolBuffer(&l)
	require.NoError(t, err)

	require.EqualValues(t, "debug", gr.Attribute)
	require.EqualValues(t, 1, gr.Uid)
	require.EqualValues(t, "mich", gr.Xid)
	require.Len(t, gr.Properties, 3)

	require.EqualValues(t, "Michonne",
		getProperty(gr.Properties, "name").GetStrVal())
	require.Len(t, gr.Children, 10)

	child := gr.Children[0]
	require.EqualValues(t, 23, child.Uid)
	require.EqualValues(t, "friend", child.Attribute)

	require.EqualValues(t, 1, len(child.Properties))
	require.EqualValues(t, "Rick Grimes",
		getProperty(child.Properties, "name").GetStrVal())
	require.Empty(t, child.Children)

	child = gr.Children[5]
	require.EqualValues(t, 23, child.Uid)
	require.EqualValues(t, "friend", child.Attribute)
	require.Empty(t, child.Properties)
	require.Empty(t, child.Children)
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
				friend @filter(anyof("name", "Andrea")) {
					name
				}
			}
		}
	`

	gq, _, err := gql.Parse(query)
	require.NoError(t, err)

	ctx := context.Background()
	sg, err := ToSubGraph(ctx, gq)
	require.NoError(t, err)

	ch := make(chan error)
	go ProcessGraph(ctx, sg, nil, ch)
	err = <-ch
	require.NoError(t, err)

	var l Latency
	pb, err := sg.ToProtocolBuffer(&l)
	require.NoError(t, err)

	expectedPb := `attribute: "me"
properties: <
  prop: "name"
  value: <
    str_val: "Michonne"
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
      str_val: "Andrea"
    >
  >
>
`
	require.EqualValues(t, proto.MarshalTextString(pb), expectedPb)
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
				friend @filter(anyof("name", "Andrea") || anyof("name", "Glenn Rhee")) {
					name
				}
			}
		}
	`

	gq, _, err := gql.Parse(query)
	require.NoError(t, err)

	ctx := context.Background()
	sg, err := ToSubGraph(ctx, gq)
	require.NoError(t, err)

	ch := make(chan error)
	go ProcessGraph(ctx, sg, nil, ch)
	err = <-ch
	require.NoError(t, err)

	var l Latency
	pb, err := sg.ToProtocolBuffer(&l)
	require.NoError(t, err)

	expectedPb := `attribute: "me"
properties: <
  prop: "name"
  value: <
    str_val: "Michonne"
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
`
	require.EqualValues(t, proto.MarshalTextString(pb), expectedPb)
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
				friend @filter(anyof("name", "Andrea") && anyof("name", "Glenn Rhee")) {
					name
				}
			}
		}
	`

	gq, _, err := gql.Parse(query)
	require.NoError(t, err)

	ctx := context.Background()
	sg, err := ToSubGraph(ctx, gq)
	require.NoError(t, err)

	ch := make(chan error)
	go ProcessGraph(ctx, sg, nil, ch)
	err = <-ch
	require.NoError(t, err)

	var l Latency
	pb, err := sg.ToProtocolBuffer(&l)
	require.NoError(t, err)

	expectedPb := `attribute: "me"
properties: <
  prop: "name"
  value: <
    str_val: "Michonne"
  >
>
properties: <
  prop: "gender"
  value: <
    bytes_val: "female"
  >
>
`
	require.EqualValues(t, proto.MarshalTextString(pb), expectedPb)
}

// Test sorting without committing to RocksDB.
//func TestToJSONOrderMLayer(t *testing.T) {
//	dir, ps := populateGraph(t)
//	defer os.RemoveAll(dir)
//	defer ps.Close()

//	query := `
//		{
//			me(_uid_:0x01) {
//				name
//				gender
//				friend(order: dob, offset: 2) {
//					name
//				}
//			}
//		}
//	`

//	gq, _, err := gql.Parse(query)
//	require.NoError(t, err)

//	ctx := context.Background()
//	sg, err := ToSubGraph(ctx, gq)
//	require.NoError(t, err)

//	ch := make(chan error)
//	go ProcessGraph(ctx, sg, nil, ch)
//	err = <-ch
//	require.NoError(t, err)

//	//	var l Latency
//	//	js, err := sg.ToJSON(&l)
//	//	require.NoError(t, err)
//	//	require.EqualValues(t,
//	//		`{"me":[{"friend":[{"name":"Andrea"}],"gender":"female","name":"Michonne"}]}`,
//	//		string(js))
//}

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

func TestMain(m *testing.M) {
	x.Init()
	os.Exit(m.Run())
}

func TestSchema1(t *testing.T) {
	require.NoError(t, schema.Parse("test_schema"))

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
	mp := processToJSON(t, query)
	resp := mp["person"]
	name := resp.([]interface{})[0].(map[string]interface{})["name"].(string)
	require.EqualValues(t, "Michonne", name)

	alive, ok := resp.([]interface{})[0].(map[string]interface{})["alive"]
	require.True(t, ok)
	require.EqualValues(t, true, alive)

	age, ok := resp.([]interface{})[0].(map[string]interface{})["age"]
	require.True(t, ok)
	require.EqualValues(t, 38, age.(float64))

	_, ok = resp.([]interface{})[0].(map[string]interface{})["survival_rate"]
	require.True(t, ok)

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
	require.EqualValues(t, 4, co)
	require.EqualValues(t, 3, res)

	actorMap := mp["person"].([]interface{})[0].(map[string]interface{})
	_, success := actorMap["name"].(string)
	require.True(t, success,
		"Expected json type string for: %v", actorMap["name"])

	// json parses ints as floats
	_, success = actorMap["age"].(float64)
	require.True(t, success,
		"Expected json type int for: %v", actorMap["age"])

	_, success = actorMap["survival_rate"].(float64)
	require.True(t, success,
		"Survival rate has to be coerced")
}
