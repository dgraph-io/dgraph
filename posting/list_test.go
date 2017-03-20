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
	"math"
	"math/rand"
	"os"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/group"
	"github.com/dgraph-io/dgraph/protos/taskp"
	"github.com/dgraph-io/dgraph/protos/typesp"
	"github.com/dgraph-io/dgraph/store"
	"github.com/dgraph-io/dgraph/x"
)

func listToArray(t *testing.T, afterUid uint64, l *List) []uint64 {
	out := make([]uint64, 0, 10)
	l.Iterate(afterUid, func(p *typesp.Posting) bool {
		out = append(out, p.Uid)
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

func addMutation(t *testing.T, l *List, edge *taskp.DirectedEdge, op uint32) {
	if op == Del {
		edge.Op = taskp.DirectedEdge_DEL
	} else if op == Set {
		edge.Op = taskp.DirectedEdge_SET
	} else {
		x.Fatalf("Unhandled op: %v", op)
	}
	_, err := l.AddMutation(context.Background(), edge)
	require.NoError(t, err)
}

func TestAddMutation(t *testing.T) {
	key := x.DataKey("name", 1)

	l := getNew(key, ps)

	edge := &taskp.DirectedEdge{
		ValueId: 9,
		Label:   "testing",
	}
	addMutation(t, l, edge, Set)

	require.Equal(t, listToArray(t, 0, l), []uint64{9})

	p := getFirst(l)
	require.NotNil(t, p, "Unable to retrieve posting")
	require.EqualValues(t, p.Label, "testing")

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
	edge.Label = "anti-testing"
	addMutation(t, l, edge, Set)

	uids := []uint64{9, 69, 81}
	checkUids(t, l, uids)

	p = getFirst(l)
	require.NotNil(t, p, "Unable to retrieve posting")
	require.EqualValues(t, p.Label, "anti-testing")
	l.SyncIfDirty(context.Background())
	l.WaitForCommit()

	// Try reading the same data in another PostingList.
	dl := getNew(key, ps)
	checkUids(t, dl, uids)
}

func getFirst(l *List) (res typesp.Posting) {
	l.Iterate(0, func(p *typesp.Posting) bool {
		res = *p
		return false
	})
	return res
}

func checkValue(t *testing.T, ol *List, val string) {
	p := getFirst(ol)
	require.Equal(t, uint64(math.MaxUint64), p.Uid) // Cast to prevent overflow.
	require.EqualValues(t, val, p.Value)
}

func TestAddMutation_Value(t *testing.T) {
	key := x.DataKey("value", 10)
	ol := getNew(key, ps)
	edge := &taskp.DirectedEdge{
		Value: []byte("oh hey there"),
		Label: "new-testing",
	}
	addMutation(t, ol, edge, Set)
	checkValue(t, ol, "oh hey there")

	// Run the same check after committing.
	_, err := ol.SyncIfDirty(context.Background())
	require.NoError(t, err)
	checkValue(t, ol, "oh hey there")

	// The value made it to the posting list. Changing it now.
	edge.Value = []byte(strconv.Itoa(119))
	addMutation(t, ol, edge, Set)
	checkValue(t, ol, "119")
}

func TestAddMutation_jchiu1(t *testing.T) {
	key := x.DataKey("value", 10)
	ol := getNew(key, ps)

	// Set value to cars and merge to RocksDB.
	edge := &taskp.DirectedEdge{
		Value: []byte("cars"),
		Label: "jchiu",
	}
	ctx := context.Background()
	addMutation(t, ol, edge, Set)
	merged, err := ol.SyncIfDirty(ctx)
	require.NoError(t, err)
	require.True(t, merged)

	require.EqualValues(t, 1, ol.Length(0))
	checkValue(t, ol, "cars")

	// Set value to newcars, but don't merge yet.
	edge = &taskp.DirectedEdge{
		Value: []byte("newcars"),
		Label: "jchiu",
	}
	addMutation(t, ol, edge, Set)
	require.EqualValues(t, 1, ol.Length(0))
	checkValue(t, ol, "newcars")

	// Set value to someothercars, but don't merge yet.
	edge = &taskp.DirectedEdge{
		Value: []byte("someothercars"),
		Label: "jchiu",
	}
	addMutation(t, ol, edge, Set)
	require.EqualValues(t, 1, ol.Length(0))
	checkValue(t, ol, "someothercars")

	// Set value back to the committed value cars, but don't merge yet.
	edge = &taskp.DirectedEdge{
		Value: []byte("cars"),
		Label: "jchiu",
	}
	addMutation(t, ol, edge, Set)
	require.EqualValues(t, 1, ol.Length(0))
	checkValue(t, ol, "cars")
}

func TestAddMutation_jchiu2(t *testing.T) {
	key := x.DataKey("value", 10)
	ol := getNew(key, ps)

	// Del a value cars and but don't merge.
	edge := &taskp.DirectedEdge{
		Value: []byte("cars"),
		Label: "jchiu",
	}
	addMutation(t, ol, edge, Del)
	require.EqualValues(t, 0, ol.Length(0))

	// Set value to newcars, but don't merge yet.
	edge = &taskp.DirectedEdge{
		Value: []byte("newcars"),
		Label: "jchiu",
	}
	addMutation(t, ol, edge, Set)
	require.EqualValues(t, 1, ol.Length(0))
	checkValue(t, ol, "newcars")

	// Del a value cars. This operation should be ignored.
	edge = &taskp.DirectedEdge{
		Value: []byte("cars"),
		Label: "jchiu",
	}
	addMutation(t, ol, edge, Del)
	require.EqualValues(t, 1, ol.Length(0))
	checkValue(t, ol, "newcars")
}

func TestAddMutation_jchiu3(t *testing.T) {
	fmt.Println("at start of jchiu3")
	key := x.DataKey("value", 10)
	ol := getNew(key, ps)

	// Set value to cars and merge to RocksDB.
	edge := &taskp.DirectedEdge{
		Value: []byte("cars"),
		Label: "jchiu",
	}
	addMutation(t, ol, edge, Set)
	require.Equal(t, 1, ol.Length(0))
	merged, err := ol.SyncIfDirty(context.Background())
	require.NoError(t, err)
	require.True(t, merged)
	require.EqualValues(t, 1, ol.Length(0))
	checkValue(t, ol, "cars")

	// Del a value cars and but don't merge.
	edge = &taskp.DirectedEdge{
		Value: []byte("cars"),
		Label: "jchiu",
	}
	addMutation(t, ol, edge, Del)
	require.Equal(t, 0, ol.Length(0))

	// Set value to newcars, but don't merge yet.
	edge = &taskp.DirectedEdge{
		Value: []byte("newcars"),
		Label: "jchiu",
	}
	addMutation(t, ol, edge, Set)
	require.EqualValues(t, 1, ol.Length(0))
	checkValue(t, ol, "newcars")

	// Del a value othercars and but don't merge.
	edge = &taskp.DirectedEdge{
		Value: []byte("othercars"),
		Label: "jchiu",
	}
	addMutation(t, ol, edge, Del)
	require.NotEqual(t, 0, ol.Length(0))
	checkValue(t, ol, "newcars")

	// Del a value newcars and but don't merge.
	edge = &taskp.DirectedEdge{
		Value: []byte("newcars"),
		Label: "jchiu",
	}
	addMutation(t, ol, edge, Del)
	require.Equal(t, 0, ol.Length(0))
}

func TestAddMutation_mrjn1(t *testing.T) {
	key := x.DataKey("value", 10)
	ol := getNew(key, ps)

	// Set a value cars and merge.
	edge := &taskp.DirectedEdge{
		Value: []byte("cars"),
		Label: "jchiu",
	}
	addMutation(t, ol, edge, Set)
	merged, err := ol.SyncIfDirty(context.Background())
	require.NoError(t, err)
	require.True(t, merged)

	// Delete a non-existent value newcars. This should have no effect.
	edge = &taskp.DirectedEdge{
		Value: []byte("newcars"),
		Label: "jchiu",
	}
	addMutation(t, ol, edge, Del)
	checkValue(t, ol, "cars")

	// Delete the previously committed value cars. But don't merge.
	edge = &taskp.DirectedEdge{
		Value: []byte("cars"),
		Label: "jchiu",
	}
	addMutation(t, ol, edge, Del)
	require.Equal(t, 0, ol.Length(0))

	// Do this again to cover Del, muid == curUid, inPlist test case.
	// Delete the previously committed value cars. But don't merge.
	edge = &taskp.DirectedEdge{
		Value: []byte("cars"),
		Label: "jchiu",
	}
	addMutation(t, ol, edge, Del)
	require.Equal(t, 0, ol.Length(0))

	// Set the value again to cover Set, muid == curUid, inPlist test case.
	// Set the previously committed value cars. But don't merge.
	edge = &taskp.DirectedEdge{
		Value: []byte("cars"),
		Label: "jchiu",
	}
	addMutation(t, ol, edge, Set)
	checkValue(t, ol, "cars")

	// Delete it again, just for fun.
	edge = &taskp.DirectedEdge{
		Value: []byte("cars"),
		Label: "jchiu",
	}
	addMutation(t, ol, edge, Del)
	require.Equal(t, 0, ol.Length(0))
}

func TestAddMutation_checksum(t *testing.T) {
	var c1, c2, c3 []byte

	{
		key := x.DataKey("value", 10)
		ol := getNew(key, ps)

		edge := &taskp.DirectedEdge{
			ValueId: 1,
			Label:   "jchiu",
		}
		addMutation(t, ol, edge, Set)

		edge = &taskp.DirectedEdge{
			ValueId: 3,
			Label:   "jchiu",
		}
		addMutation(t, ol, edge, Set)

		merged, err := ol.SyncIfDirty(context.Background())
		require.NoError(t, err)
		require.True(t, merged)

		pl := ol.PostingList()
		c1 = pl.Checksum
	}

	{
		key := x.DataKey("value2", 10)
		ol := getNew(key, ps)

		// Add in reverse.
		edge := &taskp.DirectedEdge{
			ValueId: 3,
			Label:   "jchiu",
		}
		addMutation(t, ol, edge, Set)

		edge = &taskp.DirectedEdge{
			ValueId: 1,
			Label:   "jchiu",
		}
		addMutation(t, ol, edge, Set)

		merged, err := ol.SyncIfDirty(context.Background())
		require.NoError(t, err)
		require.True(t, merged)

		pl := ol.PostingList()
		c2 = pl.Checksum
	}
	require.Equal(t, c1, c2)

	{
		key := x.DataKey("value3", 10)
		ol := getNew(key, ps)

		// Add in reverse.
		edge := &taskp.DirectedEdge{
			ValueId: 3,
			Label:   "jchiu",
		}
		addMutation(t, ol, edge, Set)

		edge = &taskp.DirectedEdge{
			ValueId: 1,
			Label:   "jchiu",
		}
		addMutation(t, ol, edge, Set)

		edge = &taskp.DirectedEdge{
			ValueId: 4,
			Label:   "jchiu",
		}
		addMutation(t, ol, edge, Set)

		merged, err := ol.SyncIfDirty(context.Background())
		require.NoError(t, err)
		require.True(t, merged)

		pl := ol.PostingList()
		c3 = pl.Checksum
	}
	require.NotEqual(t, c3, c1)
}

func TestAddMutation_gru(t *testing.T) {
	key := x.DataKey("question.tag", 0x01)
	ol := getNew(key, ps)

	{
		// Set two tag ids and merge.
		edge := &taskp.DirectedEdge{
			ValueId: 0x2b693088816b04b7,
			Label:   "gru",
		}
		addMutation(t, ol, edge, Set)
		edge = &taskp.DirectedEdge{
			ValueId: 0x29bf442b48a772e0,
			Label:   "gru",
		}
		addMutation(t, ol, edge, Set)
		merged, err := ol.SyncIfDirty(context.Background())
		require.NoError(t, err)
		require.True(t, merged)
	}

	{
		edge := &taskp.DirectedEdge{
			ValueId: 0x38dec821d2ac3a79,
			Label:   "gru",
		}
		addMutation(t, ol, edge, Set)
		edge = &taskp.DirectedEdge{
			ValueId: 0x2b693088816b04b7,
			Label:   "gru",
		}
		addMutation(t, ol, edge, Del)
		merged, err := ol.SyncIfDirty(context.Background())
		require.NoError(t, err)
		require.True(t, merged)
	}
}

func TestAddMutation_gru2(t *testing.T) {
	key := x.DataKey("question.tag", 0x01)
	ol := getNew(key, ps)

	{
		// Set two tag ids and merge.
		edge := &taskp.DirectedEdge{
			ValueId: 0x02,
			Label:   "gru",
		}
		addMutation(t, ol, edge, Set)
		edge = &taskp.DirectedEdge{
			ValueId: 0x03,
			Label:   "gru",
		}
		addMutation(t, ol, edge, Set)
		merged, err := ol.SyncIfDirty(context.Background())
		require.NoError(t, err)
		require.True(t, merged)
	}

	{
		// Lets set a new tag and delete the two older ones.
		edge := &taskp.DirectedEdge{
			ValueId: 0x02,
			Label:   "gru",
		}
		addMutation(t, ol, edge, Del)
		edge = &taskp.DirectedEdge{
			ValueId: 0x03,
			Label:   "gru",
		}
		addMutation(t, ol, edge, Del)

		edge = &taskp.DirectedEdge{
			ValueId: 0x04,
			Label:   "gru",
		}
		addMutation(t, ol, edge, Set)

		merged, err := ol.SyncIfDirty(context.Background())
		require.NoError(t, err)
		require.True(t, merged)
	}

	// Posting list should just have the new tag.
	uids := []uint64{0x04}
	require.Equal(t, uids, listToArray(t, 0, ol))
}

func TestAfterUIDCount(t *testing.T) {
	key := x.DataKey("value", 10)
	ol := getNew(key, ps)

	// Set value to cars and merge to RocksDB.
	edge := &taskp.DirectedEdge{
		Label: "jchiu",
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
	edge.Label = "somethingelse"
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
	key := x.DataKey("value", 10)
	ol := getNew(key, ps)

	// Set value to cars and merge to RocksDB.
	edge := &taskp.DirectedEdge{
		Label: "jchiu",
	}

	for i := 100; i < 300; i++ {
		edge.ValueId = uint64(i)
		addMutation(t, ol, edge, Set)
	}
	require.EqualValues(t, 200, ol.Length(0))
	require.EqualValues(t, 100, ol.Length(199))
	require.EqualValues(t, 0, ol.Length(300))

	// Re-insert 1/4 of the edges. Counts should not change.
	edge.Label = "somethingelse"
	for i := 100; i < 300; i += 4 {
		edge.ValueId = uint64(i)
		addMutation(t, ol, edge, Set)
	}
	require.EqualValues(t, 200, ol.Length(0))
	require.EqualValues(t, 100, ol.Length(199))
	require.EqualValues(t, 0, ol.Length(300))
}

func TestAfterUIDCountWithCommit(t *testing.T) {
	key := x.DataKey("value", 10)
	ol := getNew(key, ps)

	// Set value to cars and merge to RocksDB.
	edge := &taskp.DirectedEdge{
		Label: "jchiu",
	}

	for i := 100; i < 300; i++ {
		edge.ValueId = uint64(i)
		addMutation(t, ol, edge, Set)
	}
	require.EqualValues(t, 200, ol.Length(0))
	require.EqualValues(t, 100, ol.Length(199))
	require.EqualValues(t, 0, ol.Length(300))

	// Commit to database.
	merged, err := ol.SyncIfDirty(context.Background())
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
	edge.Label = "somethingelse"
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

var ps *store.Store

func TestMain(m *testing.M) {
	x.SetTestRun()
	x.Init()

	dir, err := ioutil.TempDir("", "storetest_")
	x.Check(err)

	ps, err = store.NewStore(dir)
	x.Check(err)

	Init(ps)

	group.ParseGroupConfig("")
	os.Exit(m.Run())
}

func BenchmarkAddMutations(b *testing.B) {
	key := x.DataKey("name", 1)
	l := getNew(key, ps)
	b.ResetTimer()

	ctx := context.Background()
	var err error
	for i := 0; i < b.N; i++ {
		if err != nil {
			b.Error(err)
			return
		}
		edge := &taskp.DirectedEdge{
			ValueId: uint64(rand.Intn(b.N) + 1),
			Label:   "testing",
			Op:      taskp.DirectedEdge_SET,
		}
		if _, err = l.AddMutation(ctx, edge); err != nil {
			b.Error(err)
		}
	}
}
