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

package posting

import (
	"context"
	"io/ioutil"
	"log"
	"math"
	"math/rand"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/algo"
	"github.com/dgraph-io/dgraph/posting/types"
	"github.com/dgraph-io/dgraph/store"
	"github.com/dgraph-io/dgraph/x"
)

func listToArray(t *testing.T, l *List) []uint64 {
	n := l.Length()
	out := make([]uint64, 0, n)
	for i := 0; i < n; i++ {
		var p types.Posting
		require.True(t, l.Get(&p, i))
		out = append(out, p.Uid())
	}
	return out
}

func ulToArray(l *algo.UIDList) []uint64 {
	n := l.Size()
	out := make([]uint64, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, l.Get(i))
	}
	return out
}

func checkUids(t *testing.T, l *List, uids []uint64) {
	require.Equal(t, listToArray(t, l), uids)
	if len(uids) >= 3 {
		opts := ListOptions{10, nil} // Tests for "after"
		require.Equal(t, ulToArray(l.Uids(opts)), uids[1:])

		opts = ListOptions{80, nil}
		require.Equal(t, ulToArray(l.Uids(opts)), []uint64{81})

		opts = ListOptions{82, nil}
		require.Empty(t, ulToArray(l.Uids(opts)))
	}
}

func addMutation(t *testing.T, l *List, edge x.DirectedEdge, op byte) {
	_, err := l.AddMutation(context.Background(), edge, op)
	require.NoError(t, err)
}

func TestAddMutation(t *testing.T) {
	l := getNew()
	key := Key(1, "name")
	dir, err := ioutil.TempDir("", "storetest_")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	ps, err := store.NewStore(dir)
	require.NoError(t, err)

	l.init(key, ps)

	edge := x.DirectedEdge{
		ValueId:   9,
		Source:    "testing",
		Timestamp: time.Now(),
	}
	addMutation(t, l, edge, Set)

	require.Equal(t, listToArray(t, l), []uint64{9})

	var p types.Posting
	require.True(t, l.Get(&p, 0))
	require.EqualValues(t, p.Source(), "testing")

	// Add another edge now.
	edge.ValueId = 81
	addMutation(t, l, edge, Set)
	require.Equal(t, listToArray(t, l), []uint64{9, 81})

	// Add another edge, in between the two above.
	edge.ValueId = 49
	addMutation(t, l, edge, Set)
	require.Equal(t, listToArray(t, l), []uint64{9, 49, 81})

	checkUids(t, l, []uint64{9, 49, 81})

	// Delete an edge, add an edge, replace an edge
	edge.ValueId = 49
	addMutation(t, l, edge, Del)

	edge.ValueId = 69
	addMutation(t, l, edge, Set)

	edge.ValueId = 9
	edge.Source = "anti-testing"
	addMutation(t, l, edge, Set)

	uids := []uint64{9, 69, 81}
	checkUids(t, l, uids)

	require.True(t, l.Get(&p, 0))
	require.EqualValues(t, p.Source(), "anti-testing")
	l.MergeIfDirty(context.Background())

	// Try reading the same data in another PostingList.
	dl := getNew()
	dl.init(key, ps)
	checkUids(t, dl, uids)

	_, err = dl.MergeIfDirty(context.Background())
	require.NoError(t, err)
	checkUids(t, dl, uids)
}

func checkValue(t *testing.T, ol *List, val string) {
	require.NotEqual(t, ol.Length(), 0)
	var p types.Posting
	require.True(t, ol.Get(&p, 0))
	require.Equal(t, p.Uid(), uint64(math.MaxUint64)) // Cast to prevent overflow.
	require.EqualValues(t, p.ValueBytes(), val)
}

func TestAddMutation_Value(t *testing.T) {
	ol := getNew()
	key := Key(10, "value")
	dir, err := ioutil.TempDir("", "storetest_")
	if err != nil {
		t.Error(err)
		return
	}
	defer os.RemoveAll(dir)

	ps, err := store.NewStore(dir)
	require.NoError(t, err)

	ol.init(key, ps)
	log.Println("Init successful.")

	edge := x.DirectedEdge{
		Value:     []byte("oh hey there"),
		Source:    "new-testing",
		Timestamp: time.Now(),
	}
	addMutation(t, ol, edge, Set)
	checkValue(t, ol, "oh hey there")

	// Run the same check after committing.
	_, err = ol.MergeIfDirty(context.Background())
	require.NoError(t, err)
	checkValue(t, ol, "oh hey there")

	// The value made it to the posting list. Changing it now.
	edge.Value = []byte(strconv.Itoa(119))
	addMutation(t, ol, edge, Set)
	checkValue(t, ol, "119")
}

func TestAddMutation_jchiu1(t *testing.T) {
	ol := getNew()
	key := Key(10, "value")
	dir, err := ioutil.TempDir("", "storetest_")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	ps, err := store.NewStore(dir)
	require.NoError(t, err)
	ol.init(key, ps)

	// Set value to cars and merge to RocksDB.
	edge := x.DirectedEdge{
		Value:     []byte("cars"),
		Source:    "jchiu",
		Timestamp: time.Now(),
	}
	ctx := context.Background()
	addMutation(t, ol, edge, Set)
	merged, err := ol.MergeIfDirty(ctx)
	require.NoError(t, err)
	require.True(t, merged)

	checkValue(t, ol, "cars")

	// Set value to newcars, but don't merge yet.
	edge = x.DirectedEdge{
		Value:     []byte("newcars"),
		Source:    "jchiu",
		Timestamp: time.Now(),
	}
	addMutation(t, ol, edge, Set)
	checkValue(t, ol, "newcars")

	// Set value to someothercars, but don't merge yet.
	edge = x.DirectedEdge{
		Value:     []byte("someothercars"),
		Source:    "jchiu",
		Timestamp: time.Now(),
	}
	addMutation(t, ol, edge, Set)
	checkValue(t, ol, "someothercars")

	// Set value back to the committed value cars, but don't merge yet.
	edge = x.DirectedEdge{
		Value:     []byte("cars"),
		Source:    "jchiu",
		Timestamp: time.Now(),
	}
	addMutation(t, ol, edge, Set)
	checkValue(t, ol, "cars")
}

func TestAddMutation_jchiu2(t *testing.T) {
	ol := getNew()
	key := Key(10, "value")
	dir, err := ioutil.TempDir("", "storetest_")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	ps, err := store.NewStore(dir)
	require.NoError(t, err)
	ol.init(key, ps)

	// Del a value cars and but don't merge.
	edge := x.DirectedEdge{
		Value:     []byte("cars"),
		Source:    "jchiu",
		Timestamp: time.Now(),
	}
	addMutation(t, ol, edge, Del)

	// Set value to newcars, but don't merge yet.
	edge = x.DirectedEdge{
		Value:     []byte("newcars"),
		Source:    "jchiu",
		Timestamp: time.Now(),
	}
	addMutation(t, ol, edge, Set)
	require.NoError(t, err)
	checkValue(t, ol, "newcars")

	// Set value back to cars, but don't merge yet.
	edge = x.DirectedEdge{
		Value:     []byte("cars"),
		Source:    "jchiu",
		Timestamp: time.Now(),
	}
	addMutation(t, ol, edge, Del)
	checkValue(t, ol, "newcars")
}

func TestAddMutation_jchiu3(t *testing.T) {
	ol := getNew()
	key := Key(10, "value")
	dir, err := ioutil.TempDir("", "storetest_")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	ps, err := store.NewStore(dir)
	require.NoError(t, err)
	ol.init(key, ps)

	// Set value to cars and merge to RocksDB.
	edge := x.DirectedEdge{
		Value:     []byte("cars"),
		Source:    "jchiu",
		Timestamp: time.Now(),
	}
	addMutation(t, ol, edge, Set)
	merged, err := ol.MergeIfDirty(context.Background())
	require.NoError(t, err)
	require.True(t, merged)
	checkValue(t, ol, "cars")

	// Del a value cars and but don't merge.
	edge = x.DirectedEdge{
		Value:     []byte("cars"),
		Source:    "jchiu",
		Timestamp: time.Now(),
	}
	addMutation(t, ol, edge, Del)
	require.Equal(t, ol.Length(), 0)

	// Set value to newcars, but don't merge yet.
	edge = x.DirectedEdge{
		Value:     []byte("newcars"),
		Source:    "jchiu",
		Timestamp: time.Now(),
	}
	addMutation(t, ol, edge, Set)
	checkValue(t, ol, "newcars")

	// Del a value othercars and but don't merge.
	edge = x.DirectedEdge{
		Value:     []byte("othercars"),
		Source:    "jchiu",
		Timestamp: time.Now(),
	}
	addMutation(t, ol, edge, Del)
	require.NotEqual(t, ol.Length(), 0)
	checkValue(t, ol, "newcars")

	// Del a value newcars and but don't merge.
	edge = x.DirectedEdge{
		Value:     []byte("newcars"),
		Source:    "jchiu",
		Timestamp: time.Now(),
	}
	addMutation(t, ol, edge, Del)
	require.Equal(t, ol.Length(), 0)
}

func TestAddMutation_mrjn1(t *testing.T) {
	ol := getNew()
	key := Key(10, "value")
	dir, err := ioutil.TempDir("", "storetest_")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	ps, err := store.NewStore(dir)
	require.NoError(t, err)
	ol.init(key, ps)

	// Set a value cars and merge.
	edge := x.DirectedEdge{
		Value:     []byte("cars"),
		Source:    "jchiu",
		Timestamp: time.Now(),
	}
	addMutation(t, ol, edge, Set)
	merged, err := ol.MergeIfDirty(context.Background())
	require.NoError(t, err)
	require.True(t, merged)

	// Delete a non-existent value newcars. This should have no effect.
	edge = x.DirectedEdge{
		Value:     []byte("newcars"),
		Source:    "jchiu",
		Timestamp: time.Now(),
	}
	addMutation(t, ol, edge, Del)
	checkValue(t, ol, "cars")

	// Delete the previously committed value cars. But don't merge.
	edge = x.DirectedEdge{
		Value:     []byte("cars"),
		Source:    "jchiu",
		Timestamp: time.Now(),
	}
	addMutation(t, ol, edge, Del)
	require.Equal(t, ol.Length(), 0)

	// Do this again to cover Del, muid == curUid, inPlist test case.
	// Delete the previously committed value cars. But don't merge.
	edge = x.DirectedEdge{
		Value:     []byte("cars"),
		Source:    "jchiu",
		Timestamp: time.Now(),
	}
	addMutation(t, ol, edge, Del)
	require.Equal(t, ol.Length(), 0)

	// Set the value again to cover Set, muid == curUid, inPlist test case.
	// Set the previously committed value cars. But don't merge.
	edge = x.DirectedEdge{
		Value:     []byte("cars"),
		Source:    "jchiu",
		Timestamp: time.Now(),
	}
	addMutation(t, ol, edge, Set)
	checkValue(t, ol, "cars")

	// Delete it again, just for fun.
	edge = x.DirectedEdge{
		Value:     []byte("cars"),
		Source:    "jchiu",
		Timestamp: time.Now(),
	}
	addMutation(t, ol, edge, Del)
	require.Equal(t, ol.Length(), 0)
}

func TestAddMutation_checksum(t *testing.T) {
	var c1, c2, c3 string

	dir, err := ioutil.TempDir("", "storetest_")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	ps, err := store.NewStore(dir)
	require.NoError(t, err)

	{
		ol := getNew()
		key := Key(10, "value")
		ol.init(key, ps)

		edge := x.DirectedEdge{
			ValueId:   1,
			Source:    "jchiu",
			Timestamp: time.Now(),
		}
		addMutation(t, ol, edge, Set)

		edge = x.DirectedEdge{
			ValueId:   3,
			Source:    "jchiu",
			Timestamp: time.Now(),
		}
		addMutation(t, ol, edge, Set)

		merged, err := ol.MergeIfDirty(context.Background())
		require.NoError(t, err)
		require.True(t, merged)

		pl := ol.getPostingList()
		c1 = string(pl.Checksum())
	}

	{
		ol := getNew()
		key := Key(10, "value2")
		ol.init(key, ps)

		// Add in reverse.
		edge := x.DirectedEdge{
			ValueId:   3,
			Source:    "jchiu",
			Timestamp: time.Now(),
		}
		addMutation(t, ol, edge, Set)

		edge = x.DirectedEdge{
			ValueId:   1,
			Source:    "jchiu",
			Timestamp: time.Now(),
		}
		addMutation(t, ol, edge, Set)

		merged, err := ol.MergeIfDirty(context.Background())
		require.NoError(t, err)
		require.True(t, merged)

		pl := ol.getPostingList()
		c2 = string(pl.Checksum())
	}
	require.Equal(t, c1, c2)

	{
		ol := getNew()
		key := Key(10, "value3")
		ol.init(key, ps)

		// Add in reverse.
		edge := x.DirectedEdge{
			ValueId:   3,
			Source:    "jchiu",
			Timestamp: time.Now(),
		}
		addMutation(t, ol, edge, Set)

		edge = x.DirectedEdge{
			ValueId:   1,
			Source:    "jchiu",
			Timestamp: time.Now(),
		}
		addMutation(t, ol, edge, Set)

		edge = x.DirectedEdge{
			ValueId:   4,
			Source:    "jchiu",
			Timestamp: time.Now(),
		}
		addMutation(t, ol, edge, Set)

		merged, err := ol.MergeIfDirty(context.Background())
		require.NoError(t, err)
		require.True(t, merged)

		pl := ol.getPostingList()
		c3 = string(pl.Checksum())
	}
	require.NotEqual(t, c3, c1)
}

func benchmarkAddMutations(n int, b *testing.B) {
	// logrus.SetLevel(logrus.DebugLevel)
	l := getNew()
	key := Key(1, "name")
	dir, err := ioutil.TempDir("", "storetest_")
	if err != nil {
		b.Error(err)
		return
	}

	defer os.RemoveAll(dir)
	ps, err := store.NewStore(dir)
	if err != nil {
		b.Error(err)
		return
	}

	l.init(key, ps)
	b.ResetTimer()

	ts := time.Now()
	ctx := context.Background()
	for i := 0; i < b.N; i++ {
		edge := x.DirectedEdge{
			ValueId:   uint64(rand.Intn(b.N) + 1),
			Source:    "testing",
			Timestamp: ts.Add(time.Microsecond),
		}
		if _, err := l.AddMutation(ctx, edge, Set); err != nil {
			b.Error(err)
		}
	}
}

func BenchmarkAddMutations_SyncEveryLogEntry(b *testing.B) {
	benchmarkAddMutations(0, b)
}

func BenchmarkAddMutations_SyncEvery10LogEntry(b *testing.B) {
	benchmarkAddMutations(10, b)
}

func BenchmarkAddMutations_SyncEvery100LogEntry(b *testing.B) {
	benchmarkAddMutations(100, b)
}

func BenchmarkAddMutations_SyncEvery1000LogEntry(b *testing.B) {
	benchmarkAddMutations(1000, b)
}
