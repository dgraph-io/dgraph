/*
 * Copyright 2016 Dgraph Labs, Inc.
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

package index

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/dgraph-io/dgraph/commit"
	_ "github.com/dgraph-io/dgraph/index/indexer/memtable"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/store"
	"github.com/dgraph-io/dgraph/x"
)

func addEdge(t *testing.T, edge x.DirectedEdge, l *posting.List) {
	if err := l.AddMutation(context.Background(), edge, posting.Set); err != nil {
		t.Error(err)
	}
}

func getIndices(t *testing.T) (string, *Indices) {
	dir, err := ioutil.TempDir("", "storetest_")
	x.Check(err)

	ps := new(store.Store)
	ps.Init(dir)
	clog := commit.NewLogger(dir, "mutations", 50<<20)
	clog.Init()
	posting.Init(clog)

	// So, user we're interested in has uid: 1.
	// She has 2 friends: 23, 24, 25, 31, and 101
	edge := x.DirectedEdge{
		ValueID:   23,
		Source:    "testing",
		Timestamp: time.Now(),
	}
	addEdge(t, edge, posting.GetOrCreate(posting.Key(1, "friend"), ps))

	edge.ValueID = 24
	addEdge(t, edge, posting.GetOrCreate(posting.Key(1, "friend"), ps))

	edge.ValueID = 25
	addEdge(t, edge, posting.GetOrCreate(posting.Key(1, "friend"), ps))

	edge.ValueID = 31
	addEdge(t, edge, posting.GetOrCreate(posting.Key(1, "friend"), ps))

	edge.ValueID = 101
	addEdge(t, edge, posting.GetOrCreate(posting.Key(1, "friend"), ps))

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
	// Belongs to UID store actually!
	addEdge(t, edge, posting.GetOrCreate(posting.Key(1, "_xid_"), ps))

	// Remember to move data from hash to RocksDB.
	posting.MergeLists(10)

	// Create fake indices.
	reader := bytes.NewReader([]byte(
		`{"Indexer": "memtable", "Config": [{"Attribute": "name"}]}`))
	indicesConfig, err := ReadConfigs(reader)
	x.Check(err)
	indices, err := CreateIndices(indicesConfig, dir)
	x.Check(err)
	x.Check(indices.Backfill(context.Background(), ps))

	return dir, indices
}

// Backfill only. No frontfill.
func TestBackfill(t *testing.T) {
	dir, indices := getIndices(t)
	defer os.RemoveAll(dir)

	li := &LookupSpec{
		Attr:  "name",
		Value: "Glenn Rhee",
	}
	lr := indices.Lookup(li)
	if lr.Err != nil {
		t.Error(lr.Err)
	}
	if len(lr.UIDs) != 1 {
		t.Error(fmt.Errorf("Expected 1 hit, got %d", len(lr.UIDs)))
		return
	}
	if lr.UIDs[0] != 24 {
		t.Error(fmt.Errorf("Expected UID 24, got %d", lr.UIDs[0]))
	}
}

// Backfill followed by frontfill del.
func TestFrontfillDel(t *testing.T) {
	dir, indices := getIndices(t)
	defer os.RemoveAll(dir)

	li := &LookupSpec{
		Attr:  "name",
		Value: "Glenn Rhee",
	}
	lr := indices.Lookup(li)
	if lr.Err != nil {
		t.Error(lr.Err)
	}
	if len(lr.UIDs) != 1 {
		t.Error(fmt.Errorf("Expected 1 hit, got %d", len(lr.UIDs)))
		return
	}
	if lr.UIDs[0] != 24 {
		t.Error(fmt.Errorf("Expected UID 24, got %d", lr.UIDs[0]))
	}

	// Do frontfill now.
	indices.FrontfillDel(context.Background(), "name", 24)

	// Do a pause to make sure frontfill changes go through before we do a lookup.
	time.Sleep(200 * time.Millisecond)
	lr = indices.Lookup(li)
	if lr.Err != nil {
		t.Error(lr.Err)
	}
	if len(lr.UIDs) != 0 {
		t.Error(fmt.Errorf("Expected 0 hit, got %d", len(lr.UIDs)))
		return
	}
}

// Backfill followed by frontfill add.
func TestFrontfillAdd(t *testing.T) {
	dir, indices := getIndices(t)
	defer os.RemoveAll(dir)

	li := &LookupSpec{
		Attr:  "name",
		Value: "Glenn Rhee",
	}
	lr := indices.Lookup(li)
	if lr.Err != nil {
		t.Error(lr.Err)
	}
	if len(lr.UIDs) != 1 {
		t.Error(fmt.Errorf("Expected 1 hit, got %d", len(lr.UIDs)))
		return
	}
	if lr.UIDs[0] != 24 {
		t.Error(fmt.Errorf("Expected UID 24, got %d", lr.UIDs[0]))
	}

	// Do frontfill now.
	indices.FrontfillAdd(context.Background(), "name", 24, "NotGlenn")
	// Let a different UID take the name Glenn.
	indices.FrontfillAdd(context.Background(), "name", 23, "Glenn Rhee")
	// Do a pause to make sure frontfill changes go through before we do a lookup.
	time.Sleep(200 * time.Millisecond)
	lr = indices.Lookup(li)
	if lr.Err != nil {
		t.Error(lr.Err)
	}
	if len(lr.UIDs) != 1 {
		t.Error(fmt.Errorf("Expected 1 hit, got %d", len(lr.UIDs)))
		return
	}
	if lr.UIDs[0] != 23 {
		t.Error(fmt.Errorf("Expected UID 23, got %d", lr.UIDs[0]))
	}
}
