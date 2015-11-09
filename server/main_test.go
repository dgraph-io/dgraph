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

	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/query"
	"github.com/dgraph-io/dgraph/store"
)

func NewStore(t *testing.T) string {
	path, err := ioutil.TempDir("", "storetest_")
	if err != nil {
		t.Error(err)
		t.Fail()
		return ""
	}
	return path
}

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

func TestQuery(t *testing.T) {
	pdir := NewStore(t)
	defer os.RemoveAll(pdir)
	ps := new(store.Store)
	ps.Init(pdir)

	mdir := NewStore(t)
	defer os.RemoveAll(mdir)
	ms := new(store.Store)
	ms.Init(mdir)
	posting.Init(ps, ms)

	f, err := os.Open("testdata.nq")
	if err != nil {
		t.Error(err)
		return
	}
	defer f.Close()
	count, err := handleRdfReader(f)
	if err != nil {
		t.Error(err)
		return
	}
	t.Logf("Parsed %v RDFs", count)

	// Parse GQL into internal query representation.
	g, err := gql.Parse(q0)
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
	go query.ProcessGraph(g, ch)
	if err := <-ch; err != nil {
		t.Error(err)
		return
	}
	js, err := g.ToJson()
	if err != nil {
		t.Error(err)
		return
	}
	fmt.Println(string(js))
}
