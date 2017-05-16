/*
 * Copyright (C) 2017 Dgraph Labs, Inc. and Contributors
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 */

package posting

import (
	"context"
	"io/ioutil"
	"math"
	"math/rand"
	"os"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/group"
	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/rdb"
	"github.com/dgraph-io/dgraph/store"
	"github.com/dgraph-io/dgraph/x"
)

func listToArray(t *testing.T, afterUid uint64, l *List) []uint64 {
	out := make([]uint64, 0, 10)
	l.Iterate(afterUid, func(p *protos.Posting) bool {
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

func addMutation(t *testing.T, l *List, edge *protos.DirectedEdge, op uint32) {
	if op == Del {
		edge.Op = protos.DirectedEdge_DEL
	} else if op == Set {
		edge.Op = protos.DirectedEdge_SET
	} else {
		x.Fatalf("Unhandled op: %v", op)
	}
	_, err := l.AddMutation(context.Background(), edge)
	require.NoError(t, err)
}

func deletePl(t *testing.T, l *List) {
	lhmapFor(1).EachWithDelete(func(k uint64, l *List) {
	})
	require.NoError(t, l.pstore.Delete(l.key))
}

func TestAddMutation(t *testing.T) {
	key := x.DataKey("name", 1)

	l := getNew(key, ps)

	edge := &protos.DirectedEdge{
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
	deletePl(t, l)
}

func getFirst(l *List) (res protos.Posting) {
	l.Iterate(0, func(p *protos.Posting) bool {
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
	edge := &protos.DirectedEdge{
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

	deletePl(t, ol)
}

func TestAddMutation_jchiu1(t *testing.T) {
	key := x.DataKey("value", 10)
	ol, _ := GetOrCreate(key, 1)

	// Set value to cars and merge to RocksDB.
	edge := &protos.DirectedEdge{
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
	edge = &protos.DirectedEdge{
		Value: []byte("newcars"),
		Label: "jchiu",
	}
	addMutation(t, ol, edge, Set)
	require.EqualValues(t, 1, ol.Length(0))
	checkValue(t, ol, "newcars")

	// Set value to someothercars, but don't merge yet.
	edge = &protos.DirectedEdge{
		Value: []byte("someothercars"),
		Label: "jchiu",
	}
	addMutation(t, ol, edge, Set)
	require.EqualValues(t, 1, ol.Length(0))
	checkValue(t, ol, "someothercars")

	// Set value back to the committed value cars, but don't merge yet.
	edge = &protos.DirectedEdge{
		Value: []byte("cars"),
		Label: "jchiu",
	}
	addMutation(t, ol, edge, Set)
	require.EqualValues(t, 1, ol.Length(0))
	checkValue(t, ol, "cars")

	deletePl(t, ol)
}

func TestAddMutation_jchiu2(t *testing.T) {
	key := x.DataKey("value", 10)
	ol, _ := GetOrCreate(key, 1)

	// Del a value cars and but don't merge.
	edge := &protos.DirectedEdge{
		Value: []byte("cars"),
		Label: "jchiu",
	}
	addMutation(t, ol, edge, Del)
	require.EqualValues(t, 0, ol.Length(0))

	// Set value to newcars, but don't merge yet.
	edge = &protos.DirectedEdge{
		Value: []byte("newcars"),
		Label: "jchiu",
	}
	addMutation(t, ol, edge, Set)
	require.EqualValues(t, 1, ol.Length(0))
	checkValue(t, ol, "newcars")

	// Del a value cars. This operation should be ignored.
	edge = &protos.DirectedEdge{
		Value: []byte("cars"),
		Label: "jchiu",
	}
	addMutation(t, ol, edge, Del)
	require.EqualValues(t, 1, ol.Length(0))
	checkValue(t, ol, "newcars")
}

func TestAddMutation_jchiu3(t *testing.T) {
	key := x.DataKey("value", 10)
	ol, _ := GetOrCreate(key, 1)

	// Set value to cars and merge to RocksDB.
	edge := &protos.DirectedEdge{
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
	edge = &protos.DirectedEdge{
		Value: []byte("cars"),
		Label: "jchiu",
	}
	addMutation(t, ol, edge, Del)
	require.Equal(t, 0, ol.Length(0))

	// Set value to newcars, but don't merge yet.
	edge = &protos.DirectedEdge{
		Value: []byte("newcars"),
		Label: "jchiu",
	}
	addMutation(t, ol, edge, Set)
	require.EqualValues(t, 1, ol.Length(0))
	checkValue(t, ol, "newcars")

	// Del a value othercars and but don't merge.
	edge = &protos.DirectedEdge{
		Value: []byte("othercars"),
		Label: "jchiu",
	}
	addMutation(t, ol, edge, Del)
	require.NotEqual(t, 0, ol.Length(0))
	checkValue(t, ol, "newcars")

	// Del a value newcars and but don't merge.
	edge = &protos.DirectedEdge{
		Value: []byte("newcars"),
		Label: "jchiu",
	}
	addMutation(t, ol, edge, Del)
	require.Equal(t, 0, ol.Length(0))

	deletePl(t, ol)
}

func TestAddMutation_mrjn1(t *testing.T) {
	key := x.DataKey("value", 10)
	ol, _ := GetOrCreate(key, 1)

	// Set a value cars and merge.
	edge := &protos.DirectedEdge{
		Value: []byte("cars"),
		Label: "jchiu",
	}
	addMutation(t, ol, edge, Set)
	merged, err := ol.SyncIfDirty(context.Background())
	require.NoError(t, err)
	require.True(t, merged)

	// Delete a non-existent value newcars. This should have no effect.
	edge = &protos.DirectedEdge{
		Value: []byte("newcars"),
		Label: "jchiu",
	}
	addMutation(t, ol, edge, Del)
	checkValue(t, ol, "cars")

	// Delete the previously committed value cars. But don't merge.
	edge = &protos.DirectedEdge{
		Value: []byte("cars"),
		Label: "jchiu",
	}
	addMutation(t, ol, edge, Del)
	require.Equal(t, 0, ol.Length(0))

	// Do this again to cover Del, muid == curUid, inPlist test case.
	// Delete the previously committed value cars. But don't merge.
	edge = &protos.DirectedEdge{
		Value: []byte("cars"),
		Label: "jchiu",
	}
	addMutation(t, ol, edge, Del)
	require.Equal(t, 0, ol.Length(0))

	// Set the value again to cover Set, muid == curUid, inPlist test case.
	// Set the previously committed value cars. But don't merge.
	edge = &protos.DirectedEdge{
		Value: []byte("cars"),
		Label: "jchiu",
	}
	addMutation(t, ol, edge, Set)
	checkValue(t, ol, "cars")

	// Delete it again, just for fun.
	edge = &protos.DirectedEdge{
		Value: []byte("cars"),
		Label: "jchiu",
	}
	addMutation(t, ol, edge, Del)
	require.Equal(t, 0, ol.Length(0))

	deletePl(t, ol)
}

func TestAddMutation_checksum(t *testing.T) {
	var c1, c2, c3 []byte

	{
		key := x.DataKey("value", 10)
		ol, _ := GetOrCreate(key, 1)

		edge := &protos.DirectedEdge{
			ValueId: 1,
			Label:   "jchiu",
		}
		addMutation(t, ol, edge, Set)

		edge = &protos.DirectedEdge{
			ValueId: 3,
			Label:   "jchiu",
		}
		addMutation(t, ol, edge, Set)

		merged, err := ol.SyncIfDirty(context.Background())
		require.NoError(t, err)
		require.True(t, merged)

		pl := ol.PostingList()
		c1 = pl.Checksum
		deletePl(t, ol)
	}

	{
		key := x.DataKey("value2", 10)
		ol, _ := GetOrCreate(key, 1)

		// Add in reverse.
		edge := &protos.DirectedEdge{
			ValueId: 3,
			Label:   "jchiu",
		}
		addMutation(t, ol, edge, Set)

		edge = &protos.DirectedEdge{
			ValueId: 1,
			Label:   "jchiu",
		}
		addMutation(t, ol, edge, Set)

		merged, err := ol.SyncIfDirty(context.Background())
		require.NoError(t, err)
		require.True(t, merged)

		pl := ol.PostingList()
		c2 = pl.Checksum
		deletePl(t, ol)
	}
	require.Equal(t, c1, c2)

	{
		key := x.DataKey("value3", 10)
		ol, _ := GetOrCreate(key, 1)

		// Add in reverse.
		edge := &protos.DirectedEdge{
			ValueId: 3,
			Label:   "jchiu",
		}
		addMutation(t, ol, edge, Set)

		edge = &protos.DirectedEdge{
			ValueId: 1,
			Label:   "jchiu",
		}
		addMutation(t, ol, edge, Set)

		edge = &protos.DirectedEdge{
			ValueId: 4,
			Label:   "jchiu",
		}
		addMutation(t, ol, edge, Set)

		merged, err := ol.SyncIfDirty(context.Background())
		require.NoError(t, err)
		require.True(t, merged)

		pl := ol.PostingList()
		c3 = pl.Checksum
		deletePl(t, ol)
	}
	require.NotEqual(t, c3, c1)
}

func TestAddMutation_gru(t *testing.T) {
	key := x.DataKey("question.tag", 0x01)
	ol := getNew(key, ps)

	{
		// Set two tag ids and merge.
		edge := &protos.DirectedEdge{
			ValueId: 0x2b693088816b04b7,
			Label:   "gru",
		}
		addMutation(t, ol, edge, Set)
		edge = &protos.DirectedEdge{
			ValueId: 0x29bf442b48a772e0,
			Label:   "gru",
		}
		addMutation(t, ol, edge, Set)
		merged, err := ol.SyncIfDirty(context.Background())
		require.NoError(t, err)
		require.True(t, merged)
	}

	{
		edge := &protos.DirectedEdge{
			ValueId: 0x38dec821d2ac3a79,
			Label:   "gru",
		}
		addMutation(t, ol, edge, Set)
		edge = &protos.DirectedEdge{
			ValueId: 0x2b693088816b04b7,
			Label:   "gru",
		}
		addMutation(t, ol, edge, Del)
		merged, err := ol.SyncIfDirty(context.Background())
		require.NoError(t, err)
		require.True(t, merged)
	}

	deletePl(t, ol)
}

func TestAddMutation_gru2(t *testing.T) {
	key := x.DataKey("question.tag", 0x01)
	ol := getNew(key, ps)

	{
		// Set two tag ids and merge.
		edge := &protos.DirectedEdge{
			ValueId: 0x02,
			Label:   "gru",
		}
		addMutation(t, ol, edge, Set)
		edge = &protos.DirectedEdge{
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
		edge := &protos.DirectedEdge{
			ValueId: 0x02,
			Label:   "gru",
		}
		addMutation(t, ol, edge, Del)
		edge = &protos.DirectedEdge{
			ValueId: 0x03,
			Label:   "gru",
		}
		addMutation(t, ol, edge, Del)

		edge = &protos.DirectedEdge{
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
	deletePl(t, ol)
}

func TestAfterUIDCount(t *testing.T) {
	key := x.DataKey("value", 10)
	ol := getNew(key, ps)

	// Set value to cars and merge to RocksDB.
	edge := &protos.DirectedEdge{
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
	deletePl(t, ol)
}

func TestAfterUIDCount2(t *testing.T) {
	key := x.DataKey("value", 10)
	ol := getNew(key, ps)

	// Set value to cars and merge to RocksDB.
	edge := &protos.DirectedEdge{
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
	deletePl(t, ol)
}

func TestDelete(t *testing.T) {
	key := x.DataKey("value", 10)
	ol := getNew(key, ps)

	// Set value to cars and merge to RocksDB.
	edge := &protos.DirectedEdge{
		Label: "jchiu",
	}

	for i := 1; i <= 30; i++ {
		edge.ValueId = uint64(i)
		addMutation(t, ol, edge, Set)
	}
	require.EqualValues(t, 30, ol.Length(0))
	ol.Lock()
	ol.delete(context.Background(), &protos.DirectedEdge{Attr: "value"})
	ol.Unlock()
	require.EqualValues(t, 0, ol.Length(0))
	commited, err := ol.SyncIfDirty(context.Background())
	require.NoError(t, err)
	require.True(t, commited)
	ol.WaitForCommit()

	require.EqualValues(t, 0, ol.Length(0))
}

func TestAfterUIDCountWithCommit(t *testing.T) {
	key := x.DataKey("value", 10)
	ol := getNew(key, ps)

	// Set value to cars and merge to RocksDB.
	edge := &protos.DirectedEdge{
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
	deletePl(t, ol)
}

var ps store.Store

func TestMain(m *testing.M) {
	x.SetTestRun()
	x.Init()

	dir, err := ioutil.TempDir("", "storetest_")
	x.Check(err)

	ps, err = rdb.NewStore(dir)
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
		edge := &protos.DirectedEdge{
			ValueId: uint64(rand.Intn(b.N) + 1),
			Label:   "testing",
			Op:      protos.DirectedEdge_SET,
		}
		if _, err = l.AddMutation(ctx, edge); err != nil {
			b.Error(err)
		}
	}
}
