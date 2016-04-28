/*
 * Copyright 2016 DGraph Labs, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *    http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package client

import (
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/dgraph-io/dgraph/commit"
	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/query"
	"github.com/dgraph-io/dgraph/store"
	"github.com/dgraph-io/dgraph/worker"
	"github.com/dgraph-io/dgraph/x"
)

func addEdge(t *testing.T, edge x.DirectedEdge, l *posting.List) {
	if err := l.AddMutation(edge, posting.Set); err != nil {
		t.Error(err)
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

func TestQuery(t *testing.T) {
	dir, _ := populateGraph(t)
	defer os.RemoveAll(dir)

	q0 := `
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

	// Parse GQL into internal query representation.
	gq, _, err := gql.Parse(q0)
	if err != nil {
		t.Error(err)
		return
	}
	g, err := query.ToSubGraph(gq)
	if err != nil {
		t.Error(err)
		return
	}

	ch := make(chan error)
	go query.ProcessGraph(g, ch)
	if err := <-ch; err != nil {
		t.Error(err)
		return
	}
	resp, err := g.PreTraverse()
	if err != nil {
		t.Error(err)
		return
	}

	root := NewEntity(resp)

	if root.Attribute != "_root_" {
		t.Errorf("Expected attribute _root_, Got: %v", root.Attribute)
	}
	if len(root.Properties()) != 3 {
		t.Errorf("Expected 3 properties for entity, Got: %v",
			len(root.Properties()))
	}
	if root.Properties()[0] != "name" {
		t.Errorf("Expected first property to be name, Got: %v",
			root.Properties()[0])
	}
	if !root.HasValue("name") {
		t.Errorf("Expected entity to have value for name, Got: false")
	}
	if root.HasValue("names") {
		t.Errorf("Expected entity to not have value for names, Got: true")
	}
	if string(root.Value("name")) != "Michonne" {
		t.Errorf("Expected value for name to be Michonne, Got: %v",
			string(root.Value("name")))
	}
	if len(root.Value("names")) != 0 {
		t.Errorf("Expected values for names to return empty byte slice, Got: len",
			len(root.Value("names")))
	}
	if root.NumChildren() != 5 {
		t.Errorf("Expected entity to have 5 children, Got: %v", root.NumChildren())
	}

	child := root.Children()[0]
	if !child.HasValue("name") {
		t.Errorf("Expected entity to have value for name, Got: false")
	}
	if child.HasValue("gender") {
		t.Errorf("Expected entity to not have value for gender, Got: true")
	}
	if child.NumChildren() != 0 {
		t.Errorf("Expected entity to have zero children, Got: %v",
			child.NumChildren())
	}
	if string(child.Value("name")) != "Rick Grimes" {
		t.Errorf("Expected child to have name value Rick Grimes, Got: ",
			string(child.Value("name")))
	}
	child = root.Children()[1]
	if string(child.Value("name")) != "Glenn Rhee" {
		t.Errorf("Expected child to have name value Glenn Rhee, Got: ",
			string(child.Value("name")))
	}
	child = root.Children()[2]
	if string(child.Value("name")) != "Daryl Dixon" {
		t.Errorf("Expected child to have name value Daryl Dixon, Got: ",
			string(child.Value("name")))
	}
	child = root.Children()[3]
	if string(child.Value("name")) != "Andrea" {
		t.Errorf("Expected child to have name value Andrea, Got: ",
			string(child.Value("name")))
	}
	child = root.Children()[4]
	if string(child.Value("name")) != "" {
		t.Errorf("Expected child to have name empty name value, Got: ",
			string(child.Value("name")))
	}
}
