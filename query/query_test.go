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
	"encoding/gob"
	"fmt"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"golang.org/x/net/context"

	"github.com/dgraph-io/dgraph/commit"
	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/query/graph"
	"github.com/dgraph-io/dgraph/store"
	"github.com/dgraph-io/dgraph/task"
	"github.com/dgraph-io/dgraph/worker"
	"github.com/dgraph-io/dgraph/x"
	"github.com/google/flatbuffers/go"
)

func setErr(err *error, nerr error) {
	if err != nil {
		return
	}
	*err = nerr
}

func addEdge(t *testing.T, edge x.DirectedEdge, l *posting.List) {
	if err := l.AddMutation(context.Background(), edge, posting.Set); err != nil {
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
	uo := flatbuffers.GetUOffsetT(child.Result)
	r := new(task.Result)
	r.Init(child.Result, uo)
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
	var ex uint64
	ex = 101

	dir, err := ioutil.TempDir("", "storetest_")
	if err != nil {
		t.Error(err)
		return
	}

	ps := new(store.Store)
	ps.Init(dir)
	ctx := context.Background()
	sg, err := newGraph(ctx, ex, "")
	if err != nil {
		t.Error(err)
	}

	worker.Init(ps, nil, 0, 1)

	uo := flatbuffers.GetUOffsetT(sg.Result)
	r := new(task.Result)
	r.Init(sg.Result, uo)
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
	if ul.Uids(0) != ex {
		t.Errorf("Expected uid: %v. Got: %v", ex, ul.Uids(0))
	}
}

func populateGraph(t *testing.T) (string, *store.Store) {
	// logrus.SetLevel(logrus.DebugLevel)
	dir, err := ioutil.TempDir("", "storetest_")
	if err != nil {
		t.Error(err)
		return "", nil
	}

	ps := new(store.Store)
	ps.Init(dir)

	worker.Init(ps, nil, 0, 1)

	clog := commit.NewLogger(dir, "mutations", 50<<20)
	clog.Init()
	posting.Init(clog)

	// So, user we're interested in has uid: 1.
	// She has 4 friends: 23, 24, 25, 31, and 101
	edge := x.DirectedEdge{
		ValueId:   23,
		Source:    "testing",
		Timestamp: time.Now(),
	}
	addEdge(t, edge, posting.GetOrCreate(posting.Key(1, "friend"), ps))

	edge.ValueId = 24
	addEdge(t, edge, posting.GetOrCreate(posting.Key(1, "friend"), ps))

	edge.ValueId = 25
	addEdge(t, edge, posting.GetOrCreate(posting.Key(1, "friend"), ps))

	edge.ValueId = 31
	addEdge(t, edge, posting.GetOrCreate(posting.Key(1, "friend"), ps))

	edge.ValueId = 101
	addEdge(t, edge, posting.GetOrCreate(posting.Key(1, "friend"), ps))

	// Now let's add a few properties for the main user.
	edge.Value = []byte("Michonne")
	addEdge(t, edge, posting.GetOrCreate(posting.Key(1, "name"), ps))

	edge.Value = []byte("female")
	addEdge(t, edge, posting.GetOrCreate(posting.Key(1, "gender"), ps))

	edge.Value = []byte("alive")
	addEdge(t, edge, posting.GetOrCreate(posting.Key(1, "status"), ps))

	// Now let's add a name for each of the friends, except 101.
	edge.Value = []byte("Rick Grimes")
	addEdge(t, edge, posting.GetOrCreate(posting.Key(23, "name"), ps))

	edge.Value = []byte("Glenn Rhee")
	addEdge(t, edge, posting.GetOrCreate(posting.Key(24, "name"), ps))

	edge.Value = []byte("Daryl Dixon")
	addEdge(t, edge, posting.GetOrCreate(posting.Key(25, "name"), ps))

	edge.Value = []byte("Andrea")
	addEdge(t, edge, posting.GetOrCreate(posting.Key(31, "name"), ps))

	edge.Value = []byte("mich")
	addEdge(t, edge, posting.GetOrCreate(posting.Key(1, "_xid_"), ps))

	return dir, ps
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
	uo := flatbuffers.GetUOffsetT(child.Result)
	r := new(task.Result)
	r.Init(child.Result, uo)

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
	uo = flatbuffers.GetUOffsetT(child.Result)
	r.Init(child.Result, uo)
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
	js, err := sg.ToJson(&l)
	if err != nil {
		t.Error(err)
	}
	fmt.Printf(string(js))
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
			me(_uid_:0x01) {
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

	if gr.Attribute != "_root_" {
		t.Errorf("Expected attribute _root_, Got: %v", gr.Attribute)
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
		if _, err := sg.ToJson(&l); err != nil {
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
		if _, err := sg.ToProtocolBuffer(&l); err != nil {
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
