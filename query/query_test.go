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
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/store"
	"github.com/dgraph-io/dgraph/task"
	"github.com/dgraph-io/dgraph/x"
	"github.com/google/flatbuffers/go"
)

func setErr(err *error, nerr error) {
	if err != nil {
		return
	}
	*err = nerr
}

/*
func populateList(key []byte) error {
	pl := posting.Get(key)

	t := x.DirectedEdge{
		ValueId:   9,
		Source:    "query_test",
		Timestamp: time.Now(),
	}
	var err error
	setErr(&err, pl.AddMutation(t, posting.Set))

	t.ValueId = 19
	setErr(&err, pl.AddMutation(t, posting.Set))

	t.ValueId = 29
	setErr(&err, pl.AddMutation(t, posting.Set))

	t.Value = "abracadabra"
	setErr(&err, pl.AddMutation(t, posting.Set))

	return err
}

func TestRun(t *testing.T) {
	logrus.SetLevel(logrus.DebugLevel)

	pdir := NewStore(t)
	defer os.RemoveAll(pdir)
	ps := new(store.Store)
	ps.Init(pdir)

	mdir := NewStore(t)
	defer os.RemoveAll(mdir)
	ms := new(store.Store)
	ms.Init(mdir)
	posting.Init(ps, ms)

	key := posting.Key(11, "testing")
	if err := populateList(key); err != nil {
		t.Error(err)
	}
	key = posting.Key(9, "name")

	m := Message{Id: 11}
	ma := Mattr{Attr: "testing"}
	m.Attrs = append(m.Attrs, ma)

	if err := Run(&m); err != nil {
		t.Error(err)
	}
	ma = m.Attrs[0]
	uids := result.GetRootAsUids(ma.ResultUids, 0)
	if uids.UidLength() != 3 {
		t.Errorf("Expected 3. Got: %v", uids.UidLength())
	}
	var v uint64
	v = 9
	for i := 0; i < uids.UidLength(); i++ {
		if uids.Uid(i) == math.MaxUint64 {
			t.Error("Value posting encountered at index:", i)
		}
		if v != uids.Uid(i) {
			t.Errorf("Expected: %v. Got: %v", v, uids.Uid(i))
		}
		v += 10
	}
	log.Debugf("ResultUid buffer size: %v", len(ma.ResultUids))

	var val string
	if err := posting.ParseValue(&val, ma.ResultValue); err != nil {
		t.Error(err)
	}
	if val != "abracadabra" {
		t.Errorf("Expected abracadabra. Got: [%q]", val)
	}
}
*/

func NewStore(t *testing.T) string {
	path, err := ioutil.TempDir("", "storetest_")
	if err != nil {
		t.Error(err)
		t.Fail()
		return ""
	}
	return path
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
	var name string
	if err := posting.ParseValue(&name, tv.ValBytes()); err != nil {
		t.Error(err)
	}
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
	sg, err := NewGraph(ex, "")
	if err != nil {
		t.Error(err)
	}

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

func TestProcessGraph(t *testing.T) {
	logrus.SetLevel(logrus.DebugLevel)

	pdir := NewStore(t)
	defer os.RemoveAll(pdir)
	ps := new(store.Store)
	ps.Init(pdir)

	mdir := NewStore(t)
	defer os.RemoveAll(mdir)
	ms := new(store.Store)
	ms.Init(mdir)
	posting.Init(ps, ms)

	// So, user we're interested in has uid: 1.
	// She has 4 friends: 23, 24, 25, 31, and 101
	edge := x.DirectedEdge{
		ValueId:   23,
		Source:    "testing",
		Timestamp: time.Now(),
	}
	addEdge(t, edge, posting.Get(posting.Key(1, "friend")))

	edge.ValueId = 24
	addEdge(t, edge, posting.Get(posting.Key(1, "friend")))

	edge.ValueId = 25
	addEdge(t, edge, posting.Get(posting.Key(1, "friend")))

	edge.ValueId = 31
	addEdge(t, edge, posting.Get(posting.Key(1, "friend")))

	edge.ValueId = 101
	addEdge(t, edge, posting.Get(posting.Key(1, "friend")))

	// Now let's add a few properties for the main user.
	edge.Value = "Michonne"
	addEdge(t, edge, posting.Get(posting.Key(1, "name")))

	edge.Value = "female"
	addEdge(t, edge, posting.Get(posting.Key(1, "gender")))

	edge.Value = "alive"
	addEdge(t, edge, posting.Get(posting.Key(1, "status")))

	// Now let's add a name for each of the friends, except 101.
	edge.Value = "Rick Grimes"
	addEdge(t, edge, posting.Get(posting.Key(23, "name")))

	edge.Value = "Glenn Rhee"
	addEdge(t, edge, posting.Get(posting.Key(24, "name")))

	edge.Value = "Daryl Dixon"
	addEdge(t, edge, posting.Get(posting.Key(25, "name")))

	edge.Value = "Andrea"
	addEdge(t, edge, posting.Get(posting.Key(31, "name")))

	// Alright. Now we have everything set up. Let's create the query.
	sg, err := NewGraph(1, "")
	if err != nil {
		t.Error(err)
	}

	// Retrieve friends, and their names.
	csg := new(SubGraph)
	csg.Attr = "friend"
	gsg := new(SubGraph)
	gsg.Attr = "name"
	csg.Children = append(csg.Children, gsg)
	sg.Children = append(sg.Children, csg)

	// Retireve profile information for uid:1.
	csg = new(SubGraph)
	csg.Attr = "name"
	sg.Children = append(sg.Children, csg)
	csg = new(SubGraph)
	csg.Attr = "gender"
	sg.Children = append(sg.Children, csg)
	csg = new(SubGraph)
	csg.Attr = "status"
	sg.Children = append(sg.Children, csg)

	ch := make(chan error)
	go ProcessGraph(sg, ch)
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
