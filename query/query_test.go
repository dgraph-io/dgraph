/*
 * Copyright 2015 Manish R Jain <manishrjain@gmail.com>
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
	"fmt"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/dgraph-io/dgraph/commit"
	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/posting"
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
	if err := l.AddMutation(edge, posting.Set); err != nil {
		t.Error(err)
	}
}

func checkName(t *testing.T, r *task.Result, idx int, expected string) {
	var tv task.Value
	if ok := r.Values(&tv, idx); !ok {
		t.Error("Unable to retrieve value")
	}
	var iname interface{}
	if err := posting.ParseValue(&iname, tv.ValBytes()); err != nil {
		t.Error(err)
	}
	name := iname.(string)
	if name != expected {
		t.Errorf("Expected: %v. Got: %v", expected, name)
	}
}

func checkSingleValue(t *testing.T, child *SubGraph,
	attr string, value string) {
	if child.Attr != attr || len(child.result) == 0 {
		t.Error("Expected attr name with some result")
	}
	uo := flatbuffers.GetUOffsetT(child.result)
	r := new(task.Result)
	r.Init(child.result, uo)
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
		t.Error("Expected uids length 0. Got: %v", ul.UidsLength())
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
	sg, err := newGraph(ex, "", ps)
	if err != nil {
		t.Error(err)
	}

	worker.Init(ps)

	uo := flatbuffers.GetUOffsetT(sg.result)
	r := new(task.Result)
	r.Init(sg.result, uo)
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

	worker.Init(ps)

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
	edge.Value = "Michonne"
	addEdge(t, edge, posting.GetOrCreate(posting.Key(1, "name"), ps))

	edge.Value = "female"
	addEdge(t, edge, posting.GetOrCreate(posting.Key(1, "gender"), ps))

	edge.Value = "alive"
	addEdge(t, edge, posting.GetOrCreate(posting.Key(1, "status"), ps))

	// Now let's add a name for each of the friends, except 101.
	edge.Value = "Rick Grimes"
	addEdge(t, edge, posting.GetOrCreate(posting.Key(23, "name"), ps))

	edge.Value = "Glenn Rhee"
	addEdge(t, edge, posting.GetOrCreate(posting.Key(24, "name"), ps))

	edge.Value = "Daryl Dixon"
	addEdge(t, edge, posting.GetOrCreate(posting.Key(25, "name"), ps))

	edge.Value = "Andrea"
	addEdge(t, edge, posting.GetOrCreate(posting.Key(31, "name"), ps))

	return dir, ps
}

func TestProcessGraph(t *testing.T) {
	dir, ps := populateGraph(t)
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
	gq, err := gql.Parse(query)
	if err != nil {
		t.Error(err)
	}
	sg, err := ToSubGraph(gq, ps)
	if err != nil {
		t.Error(err)
	}

	ch := make(chan error)
	go ProcessGraph(sg, ch, ps)
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
	if len(child.result) == 0 {
		t.Errorf("Expected some result.")
		return
	}
	uo := flatbuffers.GetUOffsetT(child.result)
	r := new(task.Result)
	r.Init(child.result, uo)

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
	uo = flatbuffers.GetUOffsetT(child.result)
	r.Init(child.result, uo)
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
		if tv.ValLength() != 1 || tv.ValBytes()[0] != 0x00 {
			t.Error("Expected a null byte")
		}
	}

	checkSingleValue(t, sg.Children[1], "name", "Michonne")
	checkSingleValue(t, sg.Children[2], "gender", "female")
	checkSingleValue(t, sg.Children[3], "status", "alive")
}

func TestToJson(t *testing.T) {
	dir, ps := populateGraph(t)
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

	gq, err := gql.Parse(query)
	if err != nil {
		t.Error(err)
	}
	sg, err := ToSubGraph(gq, ps)
	if err != nil {
		t.Error(err)
	}

	ch := make(chan error)
	go ProcessGraph(sg, ch, ps)
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
