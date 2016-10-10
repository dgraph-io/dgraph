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

package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"context"

	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/loader"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/query"
	"github.com/dgraph-io/dgraph/store"
	"github.com/dgraph-io/dgraph/uid"
	"github.com/dgraph-io/dgraph/worker"
	"github.com/dgraph-io/dgraph/x"
)

var q0 = `
	{
		user(_xid_:alice) {
			follows {
				_xid_
				status
			}
			_xid_
			status
		}
	}
`

func init() {
	worker.ParseGroupConfig("")
	worker.StartRaftNodes(1, "localhost:12345", "1:localhost:12345", "")
	// Wait for the node to become leader for group 0.
	time.Sleep(5 * time.Second)
}

func prepare() (dir1, dir2 string, ps *store.Store, rerr error) {
	var err error
	dir1, err = ioutil.TempDir("", "storetest_")
	if err != nil {
		return "", "", nil, err
	}
	ps, err = store.NewStore(dir1)
	if err != nil {
		return "", "", nil, err
	}

	dir2, err = ioutil.TempDir("", "storemuts_")
	if err != nil {
		return dir1, "", nil, err
	}

	posting.Init()
	worker.SetState(ps)
	uid.Init(ps)
	loader.Init(ps)
	posting.InitIndex(ps)

	{
		// Then load data.
		f, err := os.Open("testdata.nq")
		if err != nil {
			return dir1, dir2, nil, err
		}
		_, err = loader.LoadEdges(f, 0, 1)
		f.Close()
		if err != nil {
			return dir1, dir2, nil, err
		}
	}

	return dir1, dir2, ps, nil
}

func closeAll(dir1, dir2 string) {
	os.RemoveAll(dir2)
	os.RemoveAll(dir1)
}

func TestQuery(t *testing.T) {
	dir1, dir2, _, err := prepare()
	if err != nil {
		t.Error(err)
		return
	}
	defer closeAll(dir1, dir2)

	// Parse GQL into internal query representation.
	gq, _, err := gql.Parse(q0)
	if err != nil {
		t.Error(err)
		return
	}
	ctx := context.Background()
	g, err := query.ToSubGraph(ctx, gq)
	if err != nil {
		t.Error(err)
		return
	}

	// Test internal query representation.
	if len(g.Children) != 3 {
		t.Errorf("Expected 3 children. Got: %v", len(g.Children))
	}

	{
		child := g.Children[0]
		if child.Attr != "follows" {
			t.Errorf("Expected follows. Got: %q", child.Attr)
		}
		if len(child.Children) != 2 {
			t.Errorf("Expected 2 child. Got: %v", len(child.Children))
		}
		gc := child.Children[0]
		if gc.Attr != "_xid_" {
			t.Errorf("Expected _xid_. Got: %q", gc.Attr)
		}
		gc = child.Children[1]
		if gc.Attr != "status" {
			t.Errorf("Expected status. Got: %q", gc.Attr)
		}
	}

	{
		child := g.Children[1]
		if child.Attr != "_xid_" {
			t.Errorf("Expected _xid_. Got: %q", child.Attr)
		}
	}

	{
		child := g.Children[2]
		if child.Attr != "status" {
			t.Errorf("Expected status. Got: %q", child.Attr)
		}
	}

	ch := make(chan error)
	go query.ProcessGraph(ctx, g, nil, ch)
	if err := <-ch; err != nil {
		t.Error(err)
		return
	}
	var l query.Latency
	js, err := g.ToJSON(&l)
	if err != nil {
		t.Error(err)
		return
	}
	fmt.Println(string(js))
}

var qm = `
	mutation {
		set {
  	  <_uid_:0x0a> <pred.rel> <_new_:x> .
    	<_new_:x> <pred.val> "value" .
    	<_new_:x> <pred.rel> <_new_:y> .
    	<_new_:y> <pred.val> "value2" .
  	}
	}
`

func TestAssignUid(t *testing.T) {
	dir1, dir2, _, err := prepare()
	if err != nil {
		t.Error(err)
		return
	}
	defer closeAll(dir1, dir2)

	// Parse GQL into internal query representation.
	_, mu, err := gql.Parse(qm)
	if err != nil {
		t.Error(err)
		return
	}
	ctx := context.Background()
	allocIds, err := mutationHandler(ctx, mu)
	if err != nil {
		t.Error(err)
		return
	}

	if len(allocIds) != 2 {
		t.Errorf("Expected two UIDs to be allocated")
	}
	if _, ok := allocIds["x"]; !ok {
		t.Error("Expected map to contain value for x")
	}
	if _, ok := allocIds["y"]; !ok {
		t.Error("Expected map to contain value for y")
	}
}

func TestConvertToEdges(t *testing.T) {
	q1 := `_uid_:0x01 <type> _uid_:0x02 .
	       _uid_:0x01 <character> _uid_:0x03 .`

	var edges []x.DirectedEdge
	var err error
	nquads, err := convertToNQuad(context.Background(), q1)
	if err != nil {
		t.Errorf("Expected err to be nil. Got: %v", err)
	}
	mr, err := convertToEdges(context.Background(), nquads)
	if err != nil {
		t.Errorf("Expected err to be nil. Got: %v", err)
	}
	if len(mr.edges) != 2 {
		t.Errorf("Expected len of edges to be: %v. Got: %v", 2, len(edges))
	}
}

var q1 = `
{
	al(_xid_: alice) {
		status
		_xid_
		follows {
			status
			_xid_
			follows {
				status
				_xid_
				follows {
					_xid_
					status
				}
			}
		}
		status
		_xid_
	}
}
`

func BenchmarkQuery(b *testing.B) {
	dir1, dir2, _, err := prepare()
	if err != nil {
		b.Error(err)
		return
	}
	defer closeAll(dir1, dir2)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		gq, _, err := gql.Parse(q1)
		if err != nil {
			b.Error(err)
			return
		}
		ctx := context.Background()
		g, err := query.ToSubGraph(ctx, gq)
		if err != nil {
			b.Error(err)
			return
		}

		ch := make(chan error)
		go query.ProcessGraph(ctx, g, nil, ch)
		if err := <-ch; err != nil {
			b.Error(err)
			return
		}
		var l query.Latency
		_, err = g.ToJSON(&l)
		if err != nil {
			b.Error(err)
			return
		}
	}
}
