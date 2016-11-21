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
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"math/rand"
	"os"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/posting/types"
	"github.com/dgraph-io/dgraph/store"
	"github.com/dgraph-io/dgraph/task"
	"github.com/dgraph-io/dgraph/x"
)

func listToArray(t *testing.T, afterUid uint64, l *List) []uint64 {
	out := make([]uint64, 0, 10)
	l.Iterate(afterUid, func(p *types.Posting) bool {
		out = append(out, p.Uid())
		return true
	})
	return out
}

func checkUids(t *testing.T, l *List, uids []uint64) {
	require.Equal(t, uids, listToArray(t, 0, l))
	if len(uids) >= 3 {
		require.Equal(t, uids[1:], listToArray(t, 10, l), uids[1:])
		require.Equal(t, []uint64{81}, listToArray(t, 80, l))
		require.Empty(t, listToArray(t, 82, l))
	}
}

func addMutation(t *testing.T, l *List, edge *task.DirectedEdge, op byte) {
	_, err := l.AddMutation(context.Background(), edge, op)
	require.NoError(t, err)
}

func TestKey(t *testing.T) {
	var i uint64
	keys := make([]string, 0, 1024)
	for i = 1024; i >= 1; i-- {
		key := Key(i, "testing.key")
		keys = append(keys, string(key))
		require.Equal(t, fmt.Sprintf("testing.key:%x", i), debugKey(key))
	}
	// Test that sorting is as expected.
	sort.Strings(keys)
	require.True(t, sort.StringsAreSorted(keys))
	for i, key := range keys {
		exp := Key(uint64(i+1), "testing.key")
		require.Equal(t, key, string(exp))
	}
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

	edge := &task.DirectedEdge{
		ValueId:   9,
		Source:    "testing",
		Timestamp: x.CurrentTime(),
	}
	addMutation(t, l, edge, Set)

	require.Equal(t, listToArray(t, 0, l), []uint64{9})

	p := getFirst(l)
	require.NotNil(t, p, "Unable to retrieve posting")
	require.EqualValues(t, p.Source(), "testing")

	// Add another edge now.
	edge.ValueId = 81
	addMutation(t, l, edge, Set)
	require.Equal(t, listToArray(t, 0, l), []uint64{9, 81})

	// Add another edge, in between the two above.
	edge.ValueId = 49
	addMutation(t, l, edge, Set)
	require.Equal(t, listToArray(t, 0, l), []uint64{9, 49, 81})

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

	p = getFirst(l)
	require.NotNil(t, p, "Unable to retrieve posting")
	require.EqualValues(t, p.Source(), "anti-testing")
	l.CommitIfDirty(context.Background())

	// Try reading the same data in another PostingList.
	dl := getNew()
	dl.init(key, ps)
	checkUids(t, dl, uids)
	return

	_, err = dl.CommitIfDirty(context.Background())
	require.NoError(t, err)
	checkUids(t, dl, uids)
}

func getFirst(l *List) (res types.Posting) {
	res = *types.GetRootAsPosting(emptyPosting, 0)
	l.Iterate(0, func(p *types.Posting) bool {
		res = *p
		return false
	})
	return res
}

func checkValue(t *testing.T, ol *List, val string) {
	p := getFirst(ol)
	require.Equal(t, uint64(math.MaxUint64), p.Uid()) // Cast to prevent overflow.
	require.EqualValues(t, val, p.ValueBytes())
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

	edge := &task.DirectedEdge{
		Value:     []byte("oh hey there"),
		Source:    "new-testing",
		Timestamp: x.CurrentTime(),
	}
	addMutation(t, ol, edge, Set)
	checkValue(t, ol, "oh hey there")

	// Run the same check after committing.
	_, err = ol.CommitIfDirty(context.Background())
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
	edge := &task.DirectedEdge{
		Value:     []byte("cars"),
		Source:    "jchiu",
		Timestamp: x.CurrentTime(),
	}
	ctx := context.Background()
	addMutation(t, ol, edge, Set)
	merged, err := ol.CommitIfDirty(ctx)
	require.NoError(t, err)
	require.True(t, merged)

	require.EqualValues(t, 1, ol.Length(0))
	checkValue(t, ol, "cars")

	// Set value to newcars, but don't merge yet.
	edge = &task.DirectedEdge{
		Value:     []byte("newcars"),
		Source:    "jchiu",
		Timestamp: x.CurrentTime(),
	}
	addMutation(t, ol, edge, Set)
	require.EqualValues(t, 1, ol.Length(0))
	checkValue(t, ol, "newcars")

	// Set value to someothercars, but don't merge yet.
	edge = &task.DirectedEdge{
		Value:     []byte("someothercars"),
		Source:    "jchiu",
		Timestamp: x.CurrentTime(),
	}
	addMutation(t, ol, edge, Set)
	require.EqualValues(t, 1, ol.Length(0))
	checkValue(t, ol, "someothercars")

	// Set value back to the committed value cars, but don't merge yet.
	edge = &task.DirectedEdge{
		Value:     []byte("cars"),
		Source:    "jchiu",
		Timestamp: x.CurrentTime(),
	}
	addMutation(t, ol, edge, Set)
	require.EqualValues(t, 1, ol.Length(0))
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
	edge := &task.DirectedEdge{
		Value:     []byte("cars"),
		Source:    "jchiu",
		Timestamp: x.CurrentTime(),
	}
	addMutation(t, ol, edge, Del)
	require.EqualValues(t, 0, ol.Length(0))

	// Set value to newcars, but don't merge yet.
	edge = &task.DirectedEdge{
		Value:     []byte("newcars"),
		Source:    "jchiu",
		Timestamp: x.CurrentTime(),
	}
	addMutation(t, ol, edge, Set)
	require.NoError(t, err)
	require.EqualValues(t, 1, ol.Length(0))
	checkValue(t, ol, "newcars")

	// Del a value cars. This operation should be ignored.
	edge = &task.DirectedEdge{
		Value:     []byte("cars"),
		Source:    "jchiu",
		Timestamp: x.CurrentTime(),
	}
	addMutation(t, ol, edge, Del)
	require.EqualValues(t, 1, ol.Length(0))
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
	edge := &task.DirectedEdge{
		Value:     []byte("cars"),
		Source:    "jchiu",
		Timestamp: x.CurrentTime(),
	}
	addMutation(t, ol, edge, Set)
	require.Equal(t, 1, ol.Length(0))
	merged, err := ol.CommitIfDirty(context.Background())
	require.NoError(t, err)
	require.True(t, merged)
	require.EqualValues(t, 1, ol.Length(0))
	checkValue(t, ol, "cars")

	// Del a value cars and but don't merge.
	edge = &task.DirectedEdge{
		Value:     []byte("cars"),
		Source:    "jchiu",
		Timestamp: x.CurrentTime(),
	}
	addMutation(t, ol, edge, Del)
	require.Equal(t, 0, ol.Length(0))

	// Set value to newcars, but don't merge yet.
	edge = &task.DirectedEdge{
		Value:     []byte("newcars"),
		Source:    "jchiu",
		Timestamp: x.CurrentTime(),
	}
	addMutation(t, ol, edge, Set)
	require.EqualValues(t, 1, ol.Length(0))
	checkValue(t, ol, "newcars")

	// Del a value othercars and but don't merge.
	edge = &task.DirectedEdge{
		Value:     []byte("othercars"),
		Source:    "jchiu",
		Timestamp: x.CurrentTime(),
	}
	addMutation(t, ol, edge, Del)
	require.NotEqual(t, 0, ol.Length(0))
	checkValue(t, ol, "newcars")

	// Del a value newcars and but don't merge.
	edge = &task.DirectedEdge{
		Value:     []byte("newcars"),
		Source:    "jchiu",
		Timestamp: x.CurrentTime(),
	}
	addMutation(t, ol, edge, Del)
	require.Equal(t, 0, ol.Length(0))
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
	edge := &task.DirectedEdge{
		Value:     []byte("cars"),
		Source:    "jchiu",
		Timestamp: x.CurrentTime(),
	}
	addMutation(t, ol, edge, Set)
	merged, err := ol.CommitIfDirty(context.Background())
	require.NoError(t, err)
	require.True(t, merged)

	// Delete a non-existent value newcars. This should have no effect.
	edge = &task.DirectedEdge{
		Value:     []byte("newcars"),
		Source:    "jchiu",
		Timestamp: x.CurrentTime(),
	}
	addMutation(t, ol, edge, Del)
	checkValue(t, ol, "cars")

	// Delete the previously committed value cars. But don't merge.
	edge = &task.DirectedEdge{
		Value:     []byte("cars"),
		Source:    "jchiu",
		Timestamp: x.CurrentTime(),
	}
	addMutation(t, ol, edge, Del)
	require.Equal(t, 0, ol.Length(0))

	// Do this again to cover Del, muid == curUid, inPlist test case.
	// Delete the previously committed value cars. But don't merge.
	edge = &task.DirectedEdge{
		Value:     []byte("cars"),
		Source:    "jchiu",
		Timestamp: x.CurrentTime(),
	}
	addMutation(t, ol, edge, Del)
	require.Equal(t, 0, ol.Length(0))

	// Set the value again to cover Set, muid == curUid, inPlist test case.
	// Set the previously committed value cars. But don't merge.
	edge = &task.DirectedEdge{
		Value:     []byte("cars"),
		Source:    "jchiu",
		Timestamp: x.CurrentTime(),
	}
	addMutation(t, ol, edge, Set)
	checkValue(t, ol, "cars")

	// Delete it again, just for fun.
	edge = &task.DirectedEdge{
		Value:     []byte("cars"),
		Source:    "jchiu",
		Timestamp: x.CurrentTime(),
	}
	addMutation(t, ol, edge, Del)
	require.Equal(t, 0, ol.Length(0))
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

		edge := &task.DirectedEdge{
			ValueId:   1,
			Source:    "jchiu",
			Timestamp: x.CurrentTime(),
		}
		addMutation(t, ol, edge, Set)

		edge = &task.DirectedEdge{
			ValueId:   3,
			Source:    "jchiu",
			Timestamp: x.CurrentTime(),
		}
		addMutation(t, ol, edge, Set)

		merged, err := ol.CommitIfDirty(context.Background())
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
		edge := &task.DirectedEdge{
			ValueId:   3,
			Source:    "jchiu",
			Timestamp: x.CurrentTime(),
		}
		addMutation(t, ol, edge, Set)

		edge = &task.DirectedEdge{
			ValueId:   1,
			Source:    "jchiu",
			Timestamp: x.CurrentTime(),
		}
		addMutation(t, ol, edge, Set)

		merged, err := ol.CommitIfDirty(context.Background())
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
		edge := &task.DirectedEdge{
			ValueId:   3,
			Source:    "jchiu",
			Timestamp: x.CurrentTime(),
		}
		addMutation(t, ol, edge, Set)

		edge = &task.DirectedEdge{
			ValueId:   1,
			Source:    "jchiu",
			Timestamp: x.CurrentTime(),
		}
		addMutation(t, ol, edge, Set)

		edge = &task.DirectedEdge{
			ValueId:   4,
			Source:    "jchiu",
			Timestamp: x.CurrentTime(),
		}
		addMutation(t, ol, edge, Set)

		merged, err := ol.CommitIfDirty(context.Background())
		require.NoError(t, err)
		require.True(t, merged)

		pl := ol.getPostingList()
		c3 = string(pl.Checksum())
	}
	require.NotEqual(t, c3, c1)
}

func TestAddMutation_gru(t *testing.T) {
	ol := getNew()
	key := Key(0x01, "question.tag")
	dir, err := ioutil.TempDir("", "storetest_")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	ps, err := store.NewStore(dir)
	require.NoError(t, err)
	ol.init(key, ps)

	{
		// Set two tag ids and merge.
		edge := &task.DirectedEdge{
			ValueId:   0x2b693088816b04b7,
			Source:    "gru",
			Timestamp: x.CurrentTime(),
		}
		addMutation(t, ol, edge, Set)
		edge = &task.DirectedEdge{
			ValueId:   0x29bf442b48a772e0,
			Source:    "gru",
			Timestamp: x.CurrentTime(),
		}
		addMutation(t, ol, edge, Set)
		merged, err := ol.CommitIfDirty(context.Background())
		require.NoError(t, err)
		require.True(t, merged)
	}

	{
		edge := &task.DirectedEdge{
			ValueId:   0x38dec821d2ac3a79,
			Source:    "gru",
			Timestamp: x.CurrentTime(),
		}
		addMutation(t, ol, edge, Set)
		edge = &task.DirectedEdge{
			ValueId:   0x2b693088816b04b7,
			Source:    "gru",
			Timestamp: x.CurrentTime(),
		}
		addMutation(t, ol, edge, Del)
		merged, err := ol.CommitIfDirty(context.Background())
		require.NoError(t, err)
		require.True(t, merged)
	}
}

func TestAddMutation_gru2(t *testing.T) {
	ol := getNew()
	key := Key(0x01, "question.tag")
	dir, err := ioutil.TempDir("", "storetest_")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	ps, err := store.NewStore(dir)
	require.NoError(t, err)
	ol.init(key, ps)

	{
		// Set two tag ids and merge.
		edge := &task.DirectedEdge{
			ValueId:   0x02,
			Source:    "gru",
			Timestamp: x.CurrentTime(),
		}
		addMutation(t, ol, edge, Set)
		edge = &task.DirectedEdge{
			ValueId:   0x03,
			Source:    "gru",
			Timestamp: x.CurrentTime(),
		}
		addMutation(t, ol, edge, Set)
		merged, err := ol.CommitIfDirty(context.Background())
		require.NoError(t, err)
		require.True(t, merged)
	}

	{
		// Lets set a new tag and delete the two older ones.
		edge := &task.DirectedEdge{
			ValueId:   0x02,
			Source:    "gru",
			Timestamp: x.CurrentTime(),
		}
		addMutation(t, ol, edge, Del)
		edge = &task.DirectedEdge{
			ValueId:   0x03,
			Source:    "gru",
			Timestamp: x.CurrentTime(),
		}
		addMutation(t, ol, edge, Del)

		edge = &task.DirectedEdge{
			ValueId:   0x04,
			Source:    "gru",
			Timestamp: x.CurrentTime(),
		}
		addMutation(t, ol, edge, Set)

		merged, err := ol.CommitIfDirty(context.Background())
		require.NoError(t, err)
		require.True(t, merged)
	}

	// Posting list should just have the new tag.
	uids := []uint64{0x04}
	require.Equal(t, uids, listToArray(t, 0, ol))
}

func TestAfterUIDCount(t *testing.T) {
	ol := getNew()
	key := Key(10, "value")
	dir, err := ioutil.TempDir("", "storetest_")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	ps, err := store.NewStore(dir)
	require.NoError(t, err)
	ol.init(key, ps)

	// Set value to cars and merge to RocksDB.
	edge := &task.DirectedEdge{
		Source:    "jchiu",
		Timestamp: x.CurrentTime(),
	}

	for i := 100; i < 300; i++ {
		edge.ValueId = uint64(i)
		addMutation(t, ol, edge, Set)
	}
	require.EqualValues(t, 200, ol.Length(0))
	require.EqualValues(t, 100, ol.Length(199))
	require.EqualValues(t, 0, ol.Length(300))

	// Delete half of the edges.
	for i := 100; i < 300; i += 2 {
		edge.ValueId = uint64(i)
		addMutation(t, ol, edge, Del)
	}
	require.EqualValues(t, 100, ol.Length(0))
	require.EqualValues(t, 50, ol.Length(199))
	require.EqualValues(t, 0, ol.Length(300))

	// Try to delete half of the edges. Redundant deletes.
	for i := 100; i < 300; i += 2 {
		edge.ValueId = uint64(i)
		addMutation(t, ol, edge, Del)
	}
	require.EqualValues(t, 100, ol.Length(0))
	require.EqualValues(t, 50, ol.Length(199))
	require.EqualValues(t, 0, ol.Length(300))

	// Delete everything.
	for i := 100; i < 300; i++ {
		edge.ValueId = uint64(i)
		addMutation(t, ol, edge, Del)
	}
	require.EqualValues(t, 0, ol.Length(0))
	require.EqualValues(t, 0, ol.Length(199))
	require.EqualValues(t, 0, ol.Length(300))

	// Insert 1/4 of the edges.
	for i := 100; i < 300; i += 4 {
		edge.ValueId = uint64(i)
		addMutation(t, ol, edge, Set)
	}
	require.EqualValues(t, 50, ol.Length(0))
	require.EqualValues(t, 25, ol.Length(199))
	require.EqualValues(t, 0, ol.Length(300))

	// Insert 1/4 of the edges.
	edge.Timestamp = x.CurrentTime() // Force an update of the edge.
	edge.Source = "somethingelse"
	for i := 100; i < 300; i += 4 {
		edge.ValueId = uint64(i)
		addMutation(t, ol, edge, Set)
	}
	require.EqualValues(t, 50, ol.Length(0)) // Expect no change.
	require.EqualValues(t, 25, ol.Length(199))
	require.EqualValues(t, 0, ol.Length(300))

	// Insert 1/4 of the edges.
	for i := 103; i < 300; i += 4 {
		edge.ValueId = uint64(i)
		addMutation(t, ol, edge, Set)
	}
	require.EqualValues(t, 100, ol.Length(0))
	require.EqualValues(t, 50, ol.Length(199))
	require.EqualValues(t, 0, ol.Length(300))
}

func TestAfterUIDCount2(t *testing.T) {
	ol := getNew()
	key := Key(10, "value")
	dir, err := ioutil.TempDir("", "storetest_")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	ps, err := store.NewStore(dir)
	require.NoError(t, err)
	ol.init(key, ps)

	// Set value to cars and merge to RocksDB.
	edge := &task.DirectedEdge{
		Source:    "jchiu",
		Timestamp: x.CurrentTime(),
	}

	for i := 100; i < 300; i++ {
		edge.ValueId = uint64(i)
		addMutation(t, ol, edge, Set)
	}
	require.EqualValues(t, 200, ol.Length(0))
	require.EqualValues(t, 100, ol.Length(199))
	require.EqualValues(t, 0, ol.Length(300))

	// Re-insert 1/4 of the edges. Counts should not change.
	edge.Timestamp = x.CurrentTime() // Force an update of the edge.
	edge.Source = "somethingelse"
	for i := 100; i < 300; i += 4 {
		edge.ValueId = uint64(i)
		addMutation(t, ol, edge, Set)
	}
	require.EqualValues(t, 200, ol.Length(0))
	require.EqualValues(t, 100, ol.Length(199))
	require.EqualValues(t, 0, ol.Length(300))
}

func TestAfterUIDCountWithCommit(t *testing.T) {
	ol := getNew()
	key := Key(10, "value")
	dir, err := ioutil.TempDir("", "storetest_")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	ps, err := store.NewStore(dir)
	require.NoError(t, err)
	ol.init(key, ps)

	// Set value to cars and merge to RocksDB.
	edge := &task.DirectedEdge{
		Source:    "jchiu",
		Timestamp: x.CurrentTime(),
	}

	for i := 100; i < 300; i++ {
		edge.ValueId = uint64(i)
		addMutation(t, ol, edge, Set)
	}
	require.EqualValues(t, 200, ol.Length(0))
	require.EqualValues(t, 100, ol.Length(199))
	require.EqualValues(t, 0, ol.Length(300))

	// Commit to database.
	merged, err := ol.CommitIfDirty(context.Background())
	require.NoError(t, err)
	require.True(t, merged)

	// Mutation layer starts afresh from here.
	// Delete half of the edges.
	for i := 100; i < 300; i += 2 {
		edge.ValueId = uint64(i)
		addMutation(t, ol, edge, Del)
	}
	require.EqualValues(t, 100, ol.Length(0))
	require.EqualValues(t, 50, ol.Length(199))
	require.EqualValues(t, 0, ol.Length(300))

	// Try to delete half of the edges. Redundant deletes.
	for i := 100; i < 300; i += 2 {
		edge.ValueId = uint64(i)
		addMutation(t, ol, edge, Del)
	}
	require.EqualValues(t, 100, ol.Length(0))
	require.EqualValues(t, 50, ol.Length(199))
	require.EqualValues(t, 0, ol.Length(300))

	// Delete everything.
	for i := 100; i < 300; i++ {
		edge.ValueId = uint64(i)
		addMutation(t, ol, edge, Del)
	}
	require.EqualValues(t, 0, ol.Length(0))
	require.EqualValues(t, 0, ol.Length(199))
	require.EqualValues(t, 0, ol.Length(300))

	// Insert 1/4 of the edges.
	for i := 100; i < 300; i += 4 {
		edge.ValueId = uint64(i)
		addMutation(t, ol, edge, Set)
	}
	require.EqualValues(t, 50, ol.Length(0))
	require.EqualValues(t, 25, ol.Length(199))
	require.EqualValues(t, 0, ol.Length(300))

	// Insert 1/4 of the edges.
	edge.Timestamp = x.CurrentTime() // Force an update of the edge.
	edge.Source = "somethingelse"
	for i := 100; i < 300; i += 4 {
		edge.ValueId = uint64(i)
		addMutation(t, ol, edge, Set)
	}
	require.EqualValues(t, 50, ol.Length(0)) // Expect no change.
	require.EqualValues(t, 25, ol.Length(199))
	require.EqualValues(t, 0, ol.Length(300))

	// Insert 1/4 of the edges.
	for i := 103; i < 300; i += 4 {
		edge.ValueId = uint64(i)
		addMutation(t, ol, edge, Set)
	}
	require.EqualValues(t, 100, ol.Length(0))
	require.EqualValues(t, 50, ol.Length(199))
	require.EqualValues(t, 0, ol.Length(300))
}

func TestMain(m *testing.M) {
	x.Init()
	os.Exit(m.Run())
}

func BenchmarkAddMutations(b *testing.B) {
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

	//ts := x.CurrentTime()
	ts := time.Now()
	ctx := context.Background()
	for i := 0; i < b.N; i++ {
		ts.Add(time.Microsecond)
		tsData, err := ts.MarshalBinary()
		if err != nil {
			b.Error(err)
			return
		}
		edge := &task.DirectedEdge{
			ValueId:   uint64(rand.Intn(b.N) + 1),
			Source:    "testing",
			Timestamp: tsData,
		}
		if _, err := l.AddMutation(ctx, edge, Set); err != nil {
			b.Error(err)
		}
	}
}
