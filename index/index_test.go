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

	ps, err := store.NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	clog := commit.NewLogger(dir, "mutations", 50<<20)
	clog.Init()
	posting.Init(clog)

	// So, user we're interested in has uid: 1.
	// She has 2 friends: 23<<16, 24<<16, 25<<16, 31, and 101<<16.
	edge := x.DirectedEdge{
		ValueId:   23 << 16,
		Source:    "testing",
		Timestamp: time.Now(),
	}
	addEdge(t, edge, posting.GetOrCreate(posting.Key(1, "friend"), ps))

	edge.ValueId = 24 << 16
	addEdge(t, edge, posting.GetOrCreate(posting.Key(1, "friend"), ps))

	edge.ValueId = 25 << 16
	addEdge(t, edge, posting.GetOrCreate(posting.Key(1, "friend"), ps))

	edge.ValueId = 31
	addEdge(t, edge, posting.GetOrCreate(posting.Key(1, "friend"), ps))

	edge.ValueId = 101 << 16
	addEdge(t, edge, posting.GetOrCreate(posting.Key(1, "friend"), ps))

	// Now let's add a name for each of the friends, except 101.
	edge.Value = []byte("Rick Grimes")
	addEdge(t, edge, posting.GetOrCreate(posting.Key(23<<16, "name"), ps))

	edge.Value = []byte("Glenn Rhee")
	addEdge(t, edge, posting.GetOrCreate(posting.Key(24<<16, "name"), ps))

	edge.Value = []byte("Daryl Dixon")
	addEdge(t, edge, posting.GetOrCreate(posting.Key(25<<16, "name"), ps))

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

	uids, err := indices.Lookup(context.Background(), "name", "Glenn Rhee")
	if err != nil {
		t.Error(err)
		return
	}
	if len(uids) != 1 {
		t.Errorf("Expected 1 hit, got %d", len(uids))
		return
	}
	if uids[0] != 24<<16 {
		t.Errorf("Expected UID 24<<16, got %d", uids[0])
		return
	}
}

// Backfill followed by frontfill del.
func TestFrontfillDel(t *testing.T) {
	dir, indices := getIndices(t)
	defer os.RemoveAll(dir)

	uids, err := indices.Lookup(context.Background(), "name", "Glenn Rhee")
	if err != nil {
		t.Error(err)
		return
	}
	if len(uids) != 1 {
		t.Errorf("Expected 1 hit, got %d", len(uids))
		return
	}
	if uids[0] != 24<<16 {
		t.Errorf("Expected UID 24<<16, got %d", uids[0])
		return
	}

	// Do frontfill now.
	indices.Remove(context.Background(), "name", 24<<16)

	// Do a pause to make sure frontfill changes go through before we do a lookup.
	time.Sleep(200 * time.Millisecond)
	uids, err = indices.Lookup(context.Background(), "name", "Glenn Rhee")
	if err != nil {
		t.Error(err)
		return
	}
	if len(uids) != 0 {
		t.Errorf("Expected 0 hit, got %d", len(uids))
		return
	}
}

// Backfill followed by frontfill add.
func TestFrontfillAdd(t *testing.T) {
	dir, indices := getIndices(t)
	defer os.RemoveAll(dir)

	uids, err := indices.Lookup(context.Background(), "name", "Glenn Rhee")
	if err != nil {
		t.Error(err)
		return
	}
	if len(uids) != 1 {
		t.Errorf("Expected 1 hit, got %d", len(uids))
		return
	}
	if uids[0] != 24<<16 {
		t.Errorf("Expected UID 24<<16, got %d", uids)
		return
	}

	// Do frontfill now.
	indices.Insert(context.Background(), "name", 24<<16, "NotGlenn")
	// Let a different UID take the name Glenn.
	indices.Insert(context.Background(), "name", 23<<16, "Glenn Rhee")
	indices.Insert(context.Background(), "name", 31, "Glenn Rhee")
	// Do a pause to make sure frontfill changes go through before we do a lookup.
	time.Sleep(200 * time.Millisecond)
	uids, err = indices.Lookup(context.Background(), "name", "Glenn Rhee")
	if err != nil {
		t.Error(err)
		return
	}
	if len(uids) != 2 {
		t.Errorf("Expected 2 hits, got %d", len(uids))
		return
	}
	// Returned UIDs should be sorted. We make sure that indices use big endian so
	// that the string ordering is also the numeric ordering.
	if uids[0] != 31 {
		t.Errorf("Expected UID 31, got %d", uids[0])
		return
	}
	if uids[1] != 23<<16 {
		t.Errorf("Expected UID 23<<16, got %d", uids[1])
		return
	}
}
