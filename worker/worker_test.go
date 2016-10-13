/*
 * Copyright 2016 DGraph Labs, Inc.
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

package worker

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/store"
	"github.com/dgraph-io/dgraph/task"
	"github.com/dgraph-io/dgraph/x"
	flatbuffers "github.com/google/flatbuffers/go"
)

func addEdge(t *testing.T, edge x.DirectedEdge, l *posting.List) {
	require.NoError(t,
		l.AddMutationWithIndex(context.Background(), edge, posting.Set))
}

func delEdge(t *testing.T, edge x.DirectedEdge, l *posting.List) {
	require.NoError(t,
		l.AddMutationWithIndex(context.Background(), edge, posting.Del))
}

func check(r *task.Result, idx int, expected []uint64) error {
	var m task.UidList
	if ok := r.Uidmatrix(&m, idx); !ok {
		return fmt.Errorf("Unable to retrieve uidlist")
	}

	if m.UidsLength() != len(expected) {
		return fmt.Errorf("Expected length: %v. Got: %v",
			len(expected), m.UidsLength())
	}
	for i, uid := range expected {
		if m.Uids(i) != uid {
			return fmt.Errorf("Uid mismatch at index: %v. Expected: %v. Got: %v",
				i, uid, m.Uids(i))
		}
	}
	return nil
}

func getOrCreate(key []byte, ps *store.Store) *posting.List {
	l, _ := posting.GetOrCreate(key, ps)
	return l
}

func populateGraph(t *testing.T, ps *store.Store) {
	edge := x.DirectedEdge{
		ValueId:   23,
		Source:    "author0",
		Timestamp: time.Now(),
		Attribute: "friend",
	}
	edge.Entity = 10
	addEdge(t, edge, getOrCreate(posting.Key(10, "friend"), ps))

	edge.Entity = 11
	addEdge(t, edge, getOrCreate(posting.Key(11, "friend"), ps))

	edge.Entity = 12
	addEdge(t, edge, getOrCreate(posting.Key(12, "friend"), ps))

	edge.ValueId = 25
	addEdge(t, edge, getOrCreate(posting.Key(12, "friend"), ps))

	edge.ValueId = 26
	addEdge(t, edge, getOrCreate(posting.Key(12, "friend"), ps))

	edge.Entity = 10
	edge.ValueId = 31
	addEdge(t, edge, getOrCreate(posting.Key(10, "friend"), ps))

	edge.Entity = 12
	addEdge(t, edge, getOrCreate(posting.Key(12, "friend"), ps))

	edge.Entity = 12
	edge.Value = []byte("photon")
	addEdge(t, edge, getOrCreate(posting.Key(12, "friend"), ps))

	edge.Entity = 10
	addEdge(t, edge, getOrCreate(posting.Key(10, "friend"), ps))
}

func TestProcessTask(t *testing.T) {
	schema.ParseBytes([]byte(`scalar friend:string @index`))

	dir, err := ioutil.TempDir("", "storetest_")
	if err != nil {
		t.Error(err)
		return
	}

	defer os.RemoveAll(dir)
	ps, err := store.NewStore(dir)
	if err != nil {
		t.Error(err)
		return
	}
	defer ps.Close()

	SetState(ps)

	posting.Init()
	posting.InitIndex(ps)
	populateGraph(t, ps)

	query := newQuery("friend", []uint64{10, 11, 12}, nil)
	result, err := processTask(query)
	if err != nil {
		t.Error(err)
	}

	r := task.GetRootAsResult(result, 0)
	if r.UidmatrixLength() != 3 {
		t.Errorf("Expected 3. Got uidmatrix length: %v", r.UidmatrixLength())
		return
	}
	if err := check(r, 0, []uint64{23, 31}); err != nil {
		t.Error(err)
	}
	if err := check(r, 1, []uint64{23}); err != nil {
		t.Error(err)
	}
	if err := check(r, 2, []uint64{23, 25, 26, 31}); err != nil {
		t.Error(err)
	}

	var valuesList task.ValueList
	if r.Values(&valuesList) == nil {
		t.Errorf("Error loading ValueList")
		return
	}
	if valuesList.ValuesLength() != 3 {
		t.Errorf("Expected 3. Got values length: %v", valuesList.ValuesLength())
		return
	}
	var tval task.Value
	if ok := valuesList.Values(&tval, 0); !ok {
		t.Errorf("Unable to retrieve value")
	}
	if string(tval.ValBytes()) != "photon" {
		t.Errorf("Expected photon. Got: %q", string(tval.ValBytes()))
	}

	if ok := valuesList.Values(&tval, 1); !ok {
		t.Errorf("Unable to retrieve value")
	}
	if !bytes.Equal(tval.ValBytes(), []byte{}) {
		t.Errorf("Invalid value")
	}

	if ok := valuesList.Values(&tval, 2); !ok {
		t.Errorf("Unable to retrieve value")
	}
	if string(tval.ValBytes()) != "photon" {
		t.Errorf("Expected photon. Got: %q", string(tval.ValBytes()))
	}
}

// newQuery creates a Query flatbuffer table, serializes and returns it.
func newQuery(attr string, uids []uint64, terms []string) []byte {
	b := flatbuffers.NewBuilder(0)

	x.Assert(uids == nil || terms == nil)

	var vend flatbuffers.UOffsetT
	if uids != nil {
		task.QueryStartUidsVector(b, len(uids))
		for i := len(uids) - 1; i >= 0; i-- {
			b.PrependUint64(uids[i])
		}
		vend = b.EndVector(len(uids))
	} else {
		offsets := make([]flatbuffers.UOffsetT, 0, len(terms))
		for _, term := range terms {
			uo := b.CreateString(term)
			offsets = append(offsets, uo)
		}
		task.QueryStartTermsVector(b, len(terms))
		for i := len(terms) - 1; i >= 0; i-- {
			b.PrependUOffsetT(offsets[i])
		}
		vend = b.EndVector(len(terms))
	}

	ao := b.CreateString(attr)
	task.QueryStart(b)
	task.QueryAddAttr(b, ao)
	if uids != nil {
		task.QueryAddUids(b, vend)
	} else {
		task.QueryAddTerms(b, vend)
	}
	qend := task.QueryEnd(b)
	b.Finish(qend)
	return b.Bytes[b.Head():]
}

// Index-related test. Similar to TestProcessTaskIndex but we call MergeLists only
// at the end. In other words, everything is happening only in mutation layers,
// and not committed to RocksDB until near the end.
func TestProcessTaskIndexMLayer(t *testing.T) {
	schema.ParseBytes([]byte(`scalar friend:string @index`))

	dir, err := ioutil.TempDir("", "storetest_")
	if err != nil {
		t.Error(err)
		return
	}

	defer os.RemoveAll(dir)
	ps, err := store.NewStore(dir)
	if err != nil {
		t.Error(err)
		return
	}
	defer ps.Close()

	posting.Init()
	SetState(ps)

	posting.InitIndex(ps)

	populateGraph(t, ps)
	time.Sleep(200 * time.Millisecond) // Let the index process jobs from channel.

	query := newQuery("friend", nil, []string{"hey", "photon"})
	result, err := processTask(query)
	if err != nil {
		t.Error(err)
	}

	r := task.GetRootAsResult(result, 0)
	if r.UidmatrixLength() != 2 {
		t.Errorf("Expected 2. Got uidmatrix length: %v", r.UidmatrixLength())
	}
	if err := check(r, 0, []uint64{}); err != nil {
		t.Error(err)
	}
	if err := check(r, 1, []uint64{10, 12}); err != nil {
		t.Error(err)
	}

	// Now try changing 12's friend value from "photon" to "notphoton_extra" to
	// "notphoton".
	edge := x.DirectedEdge{
		Value:     []byte("notphoton_extra"),
		Source:    "author0",
		Timestamp: time.Now(),
		Attribute: "friend",
		Entity:    12,
	}
	addEdge(t, edge, getOrCreate(posting.Key(12, "friend"), ps))
	edge.Value = []byte("notphoton")
	addEdge(t, edge, getOrCreate(posting.Key(12, "friend"), ps))
	time.Sleep(200 * time.Millisecond) // Let the index process jobs from channel.

	// Issue a similar query.
	query = newQuery("friend", nil, []string{"hey", "photon", "notphoton", "notphoton_extra"})
	result, err = processTask(query)
	if err != nil {
		t.Error(err)
	}

	r = task.GetRootAsResult(result, 0)
	if r.UidmatrixLength() != 4 {
		t.Errorf("Expected 4. Got uidmatrix length: %v", r.UidmatrixLength())
	}
	if err := check(r, 0, []uint64{}); err != nil {
		t.Error(err)
	}
	if err := check(r, 1, []uint64{10}); err != nil {
		t.Error(err)
	}
	if err := check(r, 2, []uint64{12}); err != nil {
		t.Error(err)
	}
	if err := check(r, 3, []uint64{}); err != nil {
		t.Error(err)
	}

	// Try deleting.
	edge = x.DirectedEdge{
		Value:     []byte("photon"),
		Source:    "author0",
		Timestamp: time.Now(),
		Attribute: "friend",
		Entity:    10,
	}
	// Redundant deletes.
	delEdge(t, edge, getOrCreate(posting.Key(10, "friend"), ps))
	delEdge(t, edge, getOrCreate(posting.Key(10, "friend"), ps))

	// Delete followed by set.
	edge.Entity = 12
	edge.Value = []byte("notphoton")
	delEdge(t, edge, getOrCreate(posting.Key(12, "friend"), ps))
	edge.Value = []byte("ignored")
	addEdge(t, edge, getOrCreate(posting.Key(12, "friend"), ps))
	time.Sleep(200 * time.Millisecond) // Let the index process jobs from channel.

	// Issue a similar query.
	query = newQuery("friend", nil, []string{"photon", "notphoton", "ignored"})
	result, err = processTask(query)
	if err != nil {
		t.Error(err)
	}

	r = task.GetRootAsResult(result, 0)
	if r.UidmatrixLength() != 3 {
		t.Errorf("Expected 3. Got uidmatrix length: %v", r.UidmatrixLength())
	}
	if err := check(r, 0, []uint64{}); err != nil {
		t.Error(err)
	}
	if err := check(r, 1, []uint64{}); err != nil {
		t.Error(err)
	}
	if err := check(r, 2, []uint64{12}); err != nil {
		t.Error(err)
	}

	// Final touch: Merge everything to RocksDB.
	posting.MergeLists(10)
	time.Sleep(200 * time.Millisecond) // Let the index process jobs from channel.

	query = newQuery("friend", nil, []string{"photon", "notphoton", "ignored"})
	result, err = processTask(query)
	if err != nil {
		t.Error(err)
	}
	r = task.GetRootAsResult(result, 0)
	if r.UidmatrixLength() != 3 {
		t.Errorf("Expected 3. Got uidmatrix length: %v", r.UidmatrixLength())
	}
	if err := check(r, 0, []uint64{}); err != nil {
		t.Error(err)
	}
	if err := check(r, 1, []uint64{}); err != nil {
		t.Error(err)
	}
	if err := check(r, 2, []uint64{12}); err != nil {
		t.Error(err)
	}
}

// Index-related test. Similar to TestProcessTaskIndeMLayer except we call
// MergeLists in between a lot of updates.
func TestProcessTaskIndex(t *testing.T) {
	schema.ParseBytes([]byte(`scalar friend:string @index`))

	dir, err := ioutil.TempDir("", "storetest_")
	if err != nil {
		t.Error(err)
		return
	}

	defer os.RemoveAll(dir)
	ps, err := store.NewStore(dir)
	if err != nil {
		t.Error(err)
		return
	}
	defer ps.Close()
	posting.InitIndex(ps)

	posting.Init()
	SetState(ps)

	populateGraph(t, ps)
	time.Sleep(200 * time.Millisecond) // Let the index process jobs from channel.

	query := newQuery("friend", nil, []string{"hey", "photon"})
	result, err := processTask(query)
	if err != nil {
		t.Error(err)
	}

	r := task.GetRootAsResult(result, 0)
	if r.UidmatrixLength() != 2 {
		t.Errorf("Expected 2. Got uidmatrix length: %v", r.UidmatrixLength())
	}
	if err := check(r, 0, []uint64{}); err != nil {
		t.Error(err)
	}
	if err := check(r, 1, []uint64{10, 12}); err != nil {
		t.Error(err)
	}

	posting.MergeLists(10)
	time.Sleep(200 * time.Millisecond) // Let the index process jobs from channel.

	// Now try changing 12's friend value from "photon" to "notphoton_extra" to
	// "notphoton".
	edge := x.DirectedEdge{
		Value:     []byte("notphoton_extra"),
		Source:    "author0",
		Timestamp: time.Now(),
		Attribute: "friend",
		Entity:    12,
	}
	addEdge(t, edge, getOrCreate(posting.Key(12, "friend"), ps))
	edge.Value = []byte("notphoton")
	addEdge(t, edge, getOrCreate(posting.Key(12, "friend"), ps))
	time.Sleep(200 * time.Millisecond) // Let the index process jobs from channel.

	// Issue a similar query.
	query = newQuery("friend", nil, []string{"hey", "photon", "notphoton", "notphoton_extra"})
	result, err = processTask(query)
	if err != nil {
		t.Error(err)
	}

	r = task.GetRootAsResult(result, 0)
	if r.UidmatrixLength() != 4 {
		t.Errorf("Expected 4. Got uidmatrix length: %v", r.UidmatrixLength())
	}
	if err := check(r, 0, []uint64{}); err != nil {
		t.Error(err)
	}
	if err := check(r, 1, []uint64{10}); err != nil {
		t.Error(err)
	}
	if err := check(r, 2, []uint64{12}); err != nil {
		t.Error(err)
	}
	if err := check(r, 3, []uint64{}); err != nil {
		t.Error(err)
	}

	posting.MergeLists(10)
	time.Sleep(200 * time.Millisecond) // Let the index process jobs from channel.

	// Try deleting.
	edge = x.DirectedEdge{
		Value:     []byte("photon"),
		Source:    "author0",
		Timestamp: time.Now(),
		Attribute: "friend",
		Entity:    10,
	}
	// Redundant deletes.
	delEdge(t, edge, getOrCreate(posting.Key(10, "friend"), ps))
	delEdge(t, edge, getOrCreate(posting.Key(10, "friend"), ps))

	// Delete followed by set.
	edge.Entity = 12
	edge.Value = []byte("notphoton")
	delEdge(t, edge, getOrCreate(posting.Key(12, "friend"), ps))
	edge.Value = []byte("ignored")
	addEdge(t, edge, getOrCreate(posting.Key(12, "friend"), ps))
	time.Sleep(200 * time.Millisecond) // Let the index process jobs from channel.

	// Issue a similar query.
	query = newQuery("friend", nil, []string{"photon", "notphoton", "ignored"})
	result, err = processTask(query)
	if err != nil {
		t.Error(err)
	}

	r = task.GetRootAsResult(result, 0)
	if r.UidmatrixLength() != 3 {
		t.Errorf("Expected 3. Got uidmatrix length: %v", r.UidmatrixLength())
	}
	if err := check(r, 0, []uint64{}); err != nil {
		t.Error(err)
	}
	if err := check(r, 1, []uint64{}); err != nil {
		t.Error(err)
	}
	if err := check(r, 2, []uint64{12}); err != nil {
		t.Error(err)
	}
}

func TestMain(m *testing.M) {
	x.Init()
	os.Exit(m.Run())
}
