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

	if NumChildren(resp) != 4 {
		t.Errorf("Expected 4 children, Got: %v", NumChildren(resp))
	}
	if HasValue(resp) {
		t.Errorf("Expected HasValue to return false, Got: true")
	}

	child := resp.Children[0]
	if child.Attribute != "name" {
		t.Errorf("Expected attribute name, Got: %v", child.Attribute)
	}
	if !HasValue(child) {
		t.Errorf("Expected HasValue to return true, Got: false")
	}
	if Values(child)[0] != "Michonne" {
		t.Errorf("Expected Value to return Michonee, Got %v", Values(child)[0])
	}

	child = resp.Children[3]
	if child.Attribute != "friend" {
		t.Errorf("Expected attribute friend, Got: %v", child.Attribute)
	}
	if NumChildren(child) != 1 {
		t.Errorf("Expected 1 child, Got: %v", NumChildren(child))
	}
	if HasValue(child) {
		t.Errorf("Expected HasValue to return false, Got: true")
	}

	child = child.Children[0]
	if child.Attribute != "name" {
		t.Errorf("Expected attribute name, Got: %v", child.Attribute)
	}
	if !HasValue(child) {
		t.Errorf("Expected HasValue to return true, Got: false")
	}
	if len(Values(child)) != 4 {
		t.Errorf("Expected 4 Values. Got: %v", len(Values(child)))
	}
}
