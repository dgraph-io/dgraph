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

package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	"github.com/dgraph-io/dgraph/commit"
	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/loader"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/query"
	"github.com/dgraph-io/dgraph/store"
	"github.com/dgraph-io/dgraph/worker"
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

func prepare() (dir1, dir2 string, ps *store.Store, clog *commit.Logger, rerr error) {
	var err error
	dir1, err = ioutil.TempDir("", "storetest_")
	if err != nil {
		return "", "", nil, nil, err
	}
	ps = new(store.Store)
	ps.Init(dir1)

	dir2, err = ioutil.TempDir("", "storemuts_")
	if err != nil {
		return dir1, "", nil, nil, err
	}
	clog = commit.NewLogger(dir2, "mutations", 50<<20)
	clog.Init()

	posting.Init(clog)
	worker.Init(ps)

	f, err := os.Open("testdata.nq")
	if err != nil {
		return dir1, dir2, nil, clog, err
	}
	defer f.Close()
	_, err = loader.HandleRdfReader(f, 0, 1, ps, ps)
	if err != nil {
		return dir1, dir2, nil, clog, err
	}
	return dir1, dir2, ps, clog, nil
}

func closeAll(dir1, dir2 string, clog *commit.Logger) {
	clog.Close()
	os.RemoveAll(dir2)
	os.RemoveAll(dir1)
}

func TestQuery(t *testing.T) {
	dir1, dir2, ps, clog, err := prepare()
	if err != nil {
		t.Error(err)
		return
	}
	defer closeAll(dir1, dir2, clog)

	// Parse GQL into internal query representation.
	gq, err := gql.Parse(q0)
	if err != nil {
		t.Error(err)
		return
	}
	g, err := query.ToSubGraph(gq, ps)
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
	go query.ProcessGraph(g, ch, ps)
	if err := <-ch; err != nil {
		t.Error(err)
		return
	}
	var l query.Latency
	js, err := g.ToJson(&l)
	if err != nil {
		t.Error(err)
		return
	}
	fmt.Println(string(js))
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
	dir1, dir2, ps, clog, err := prepare()
	if err != nil {
		b.Error(err)
		return
	}
	defer closeAll(dir1, dir2, clog)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		gq, err := gql.Parse(q1)
		if err != nil {
			b.Error(err)
			return
		}
		g, err := query.ToSubGraph(gq, ps)
		if err != nil {
			b.Error(err)
			return
		}

		ch := make(chan error)
		go query.ProcessGraph(g, ch, ps)
		if err := <-ch; err != nil {
			b.Error(err)
			return
		}
		var l query.Latency
		_, err = g.ToJson(&l)
		if err != nil {
			b.Error(err)
			return
		}
	}
}
