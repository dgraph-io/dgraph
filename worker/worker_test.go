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
	"context"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/algo"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/store"
	"github.com/dgraph-io/dgraph/task"
	"github.com/dgraph-io/dgraph/x"
	flatbuffers "github.com/google/flatbuffers/go"
)

func init() {
	ParseGroupConfig("")
	StartRaftNodes(1, "localhost:12345", "1:localhost:12345", "")
	// Wait for the node to become leader for group 0.
	time.Sleep(5 * time.Second)
}

func addEdge(t *testing.T, edge x.DirectedEdge, l *posting.List) {
	require.NoError(t,
		l.AddMutationWithIndex(context.Background(), edge, posting.Set))
}

func delEdge(t *testing.T, edge x.DirectedEdge, l *posting.List) {
	require.NoError(t,
		l.AddMutationWithIndex(context.Background(), edge, posting.Del))
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

func taskValues(t *testing.T, v *task.ValueList) []string {
	out := make([]string, v.ValuesLength())
	for i := 0; i < v.ValuesLength(); i++ {
		var tv task.Value
		require.True(t, v.Values(&tv, i))
		out[i] = string(tv.ValBytes())
	}
	return out
}

func initTest(t *testing.T) (string, *store.Store) {
	schema.ParseBytes([]byte(`scalar friend:string @index`))

	dir, err := ioutil.TempDir("", "storetest_")
	require.NoError(t, err)

	ps, err := store.NewStore(dir)
	require.NoError(t, err)

	posting.Init()
	SetState(ps)

	posting.InitIndex(ps)
	InitIndex()

	populateGraph(t, ps)
	time.Sleep(200 * time.Millisecond) // Let the index process jobs from channel.

	return dir, ps
}

func TestProcessTask(t *testing.T) {
	dir, ps := initTest(t)
	defer os.RemoveAll(dir)
	defer ps.Close()

	query := newQuery("friend", []uint64{10, 11, 12}, nil)
	result, err := processTask(query)
	require.NoError(t, err)

	r := task.GetRootAsResult(result, 0)
	require.EqualValues(t,
		[][]uint64{
			[]uint64{23, 31},
			[]uint64{23},
			[]uint64{23, 25, 26, 31},
		}, algo.ToUintsListForTest(algo.FromTaskResult(r)))
}

// newQuery creates a Query flatbuffer table, serializes and returns it.
func newQuery(attr string, uids []uint64, tokens []string) []byte {
	b := flatbuffers.NewBuilder(0)

	x.Assert(uids == nil || tokens == nil)

	var vend flatbuffers.UOffsetT
	if uids != nil {
		task.QueryStartUidsVector(b, len(uids))
		for i := len(uids) - 1; i >= 0; i-- {
			b.PrependUint64(uids[i])
		}
		vend = b.EndVector(len(uids))
	} else {
		offsets := make([]flatbuffers.UOffsetT, 0, len(tokens))
		for _, term := range tokens {
			uo := b.CreateString(term)
			offsets = append(offsets, uo)
		}
		task.QueryStartTokensVector(b, len(tokens))
		for i := len(tokens) - 1; i >= 0; i-- {
			b.PrependUOffsetT(offsets[i])
		}
		vend = b.EndVector(len(tokens))
	}

	ao := b.CreateString(attr)
	task.QueryStart(b)
	task.QueryAddAttr(b, ao)
	if uids != nil {
		task.QueryAddUids(b, vend)
	} else {
		task.QueryAddTokens(b, vend)
	}
	qend := task.QueryEnd(b)
	b.Finish(qend)
	return b.Bytes[b.Head():]
}

// Index-related test. Similar to TestProcessTaskIndex but we call MergeLists only
// at the end. In other words, everything is happening only in mutation layers,
// and not committed to RocksDB until near the end.
func TestProcessTaskIndexMLayer(t *testing.T) {
	dir, ps := initTest(t)
	defer os.RemoveAll(dir)
	defer ps.Close()

	query := newQuery("friend", nil, []string{"hey", "photon"})
	result, err := processTask(query)
	require.NoError(t, err)

	r := task.GetRootAsResult(result, 0)
	require.EqualValues(t, [][]uint64{
		[]uint64{},
		[]uint64{10, 12},
	}, algo.ToUintsListForTest(algo.FromTaskResult(r)))

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
	require.NoError(t, err)

	r = task.GetRootAsResult(result, 0)
	require.EqualValues(t, [][]uint64{
		[]uint64{},
		[]uint64{10},
		[]uint64{12},
		[]uint64{},
	}, algo.ToUintsListForTest(algo.FromTaskResult(r)))

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
	require.NoError(t, err)

	r = task.GetRootAsResult(result, 0)
	require.EqualValues(t, [][]uint64{
		[]uint64{},
		[]uint64{},
		[]uint64{12},
	}, algo.ToUintsListForTest(algo.FromTaskResult(r)))

	// Final touch: Merge everything to RocksDB.
	posting.MergeLists(10)
	time.Sleep(200 * time.Millisecond) // Let the index process jobs from channel.

	query = newQuery("friend", nil, []string{"photon", "notphoton", "ignored"})
	result, err = processTask(query)
	require.NoError(t, err)

	r = task.GetRootAsResult(result, 0)
	require.EqualValues(t, [][]uint64{
		[]uint64{},
		[]uint64{},
		[]uint64{12},
	}, algo.ToUintsListForTest(algo.FromTaskResult(r)))
}

// Index-related test. Similar to TestProcessTaskIndeMLayer except we call
// MergeLists in between a lot of updates.
func TestProcessTaskIndex(t *testing.T) {
	dir, ps := initTest(t)
	defer os.RemoveAll(dir)
	defer ps.Close()

	query := newQuery("friend", nil, []string{"hey", "photon"})
	result, err := processTask(query)
	require.NoError(t, err)

	r := task.GetRootAsResult(result, 0)
	require.EqualValues(t, [][]uint64{
		[]uint64{},
		[]uint64{10, 12},
	}, algo.ToUintsListForTest(algo.FromTaskResult(r)))

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
	require.NoError(t, err)

	r = task.GetRootAsResult(result, 0)
	require.EqualValues(t, [][]uint64{
		[]uint64{},
		[]uint64{10},
		[]uint64{12},
		[]uint64{},
	}, algo.ToUintsListForTest(algo.FromTaskResult(r)))

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
	require.NoError(t, err)

	r = task.GetRootAsResult(result, 0)
	require.EqualValues(t, [][]uint64{
		[]uint64{},
		[]uint64{},
		[]uint64{12},
	}, algo.ToUintsListForTest(algo.FromTaskResult(r)))
}

func TestMain(m *testing.M) {
	x.Init()
	os.Exit(m.Run())
}
