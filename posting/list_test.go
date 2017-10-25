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

	"github.com/dgraph-io/badger"
	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/x"
)

func (l *List) PostingList() *protos.PostingList {
	l.RLock()
	defer l.RUnlock()
	return l.plist
}

func listToArray(t *testing.T, afterUid uint64, l *List, readTs uint64) []uint64 {
	out := make([]uint64, 0, 10)
	l.Iterate(readTs, afterUid, func(p *protos.Posting) bool {
		out = append(out, p.Uid)
		return true
	})
	return out
}

func checkUids(t *testing.T, l *List, uids []uint64, readTs uint64) {
	require.Equal(t, uids, listToArray(t, 0, l, readTs))
	if len(uids) >= 3 {
		require.Equal(t, uids[1:], listToArray(t, 10, l, readTs), uids[1:])
		require.Equal(t, []uint64{81}, listToArray(t, 80, l, readTs))
		require.Empty(t, listToArray(t, 82, l, readTs))
	}
}

func addMutationHelper(t *testing.T, l *List, edge *protos.DirectedEdge, op uint32, txn *Txn) {
	if op == Del {
		edge.Op = protos.DirectedEdge_DEL
	} else if op == Set {
		edge.Op = protos.DirectedEdge_SET
	} else {
		x.Fatalf("Unhandled op: %v", op)
	}
	_, err := l.AddMutation(context.Background(), txn, edge)
	require.NoError(t, err)
}

func TestAddMutation(t *testing.T) {
	key := x.DataKey("name", 2)

	l := Get(key)

	txn := &Txn{StartTs: uint64(1)}
	edge := &protos.DirectedEdge{
		ValueId: 9,
		Label:   "testing",
	}
	addMutationHelper(t, l, edge, Set, txn)

	require.Equal(t, listToArray(t, 0, l, 1), []uint64{9})

	p := getFirst(l, 1)
	require.NotNil(t, p, "Unable to retrieve posting")
	require.EqualValues(t, p.Label, "testing")

	// Add another edge now.
	edge.ValueId = 81
	addMutationHelper(t, l, edge, Set, txn)
	require.Equal(t, listToArray(t, 0, l, 1), []uint64{9, 81})

	// Add another edge, in between the two above.
	edge.ValueId = 49
	addMutationHelper(t, l, edge, Set, txn)
	require.Equal(t, listToArray(t, 0, l, 1), []uint64{9, 49, 81})

	checkUids(t, l, []uint64{9, 49, 81}, 1)

	// Delete an edge, add an edge, replace an edge
	edge.ValueId = 49
	addMutationHelper(t, l, edge, Del, txn)

	edge.ValueId = 69
	addMutationHelper(t, l, edge, Set, txn)

	edge.ValueId = 9
	edge.Label = "anti-testing"
	addMutationHelper(t, l, edge, Set, txn)
	l.CommitMutation(context.Background(), 1, 2)

	uids := []uint64{9, 69, 81}
	checkUids(t, l, uids, 3)

	p = getFirst(l, 3)
	require.NotNil(t, p, "Unable to retrieve posting")
	require.EqualValues(t, "anti-testing", p.Label)

	// Try reading the same data in another PostingList.
	dl := Get(key)
	checkUids(t, dl, uids, 3)
}

func getFirst(l *List, readTs uint64) (res protos.Posting) {
	l.Iterate(readTs, 0, func(p *protos.Posting) bool {
		res = *p
		return false
	})
	return res
}

func checkValue(t *testing.T, ol *List, val string, readTs uint64) {
	p := getFirst(ol, readTs)
	require.Equal(t, uint64(math.MaxUint64), p.Uid) // Cast to prevent overflow.
	require.EqualValues(t, val, p.Value)
}

// TODO(txn): Add tests after lru eviction
func TestAddMutation_Value(t *testing.T) {
	key := x.DataKey("value", 10)
	ol, err := getNew(key, ps)
	require.NoError(t, err)
	edge := &protos.DirectedEdge{
		Value: []byte("oh hey there"),
		Label: "new-testing",
	}
	txn := &Txn{StartTs: 1}
	addMutationHelper(t, ol, edge, Set, txn)
	checkValue(t, ol, "oh hey there", txn.StartTs)

	// Run the same check after committing.
	ol.CommitMutation(context.Background(), txn.StartTs, txn.StartTs+1)
	_, err = ol.SyncIfDirty(false)
	require.NoError(t, err)
	checkValue(t, ol, "oh hey there", uint64(3))

	// The value made it to the posting list. Changing it now.
	edge.Value = []byte(strconv.Itoa(119))
	txn = &Txn{StartTs: 3}
	addMutationHelper(t, ol, edge, Set, txn)
	checkValue(t, ol, "119", txn.StartTs)
}

func TestAddMutation_jchiu1(t *testing.T) {
	key := x.DataKey("value", 12)
	ol := Get(key)

	// Set value to cars and merge to RocksDB.
	edge := &protos.DirectedEdge{
		Value: []byte("cars"),
		Label: "jchiu",
	}
	txn := &Txn{StartTs: 1}
	addMutationHelper(t, ol, edge, Set, txn)
	ol.CommitMutation(context.Background(), 1, uint64(2))
	merged, err := ol.SyncIfDirty(false)
	require.NoError(t, err)
	require.True(t, merged)

	// TODO: Read at commitTimestamp with all committed
	require.EqualValues(t, 1, ol.Length(uint64(3), 0))
	checkValue(t, ol, "cars", uint64(3))

	txn = &Txn{StartTs: 3}
	// Set value to newcars, but don't merge yet.
	edge = &protos.DirectedEdge{
		Value: []byte("newcars"),
		Label: "jchiu",
	}
	addMutationHelper(t, ol, edge, Set, txn)
	require.EqualValues(t, 1, ol.Length(txn.StartTs, 0))
	checkValue(t, ol, "newcars", txn.StartTs)

	// Set value to someothercars, but don't merge yet.
	edge = &protos.DirectedEdge{
		Value: []byte("someothercars"),
		Label: "jchiu",
	}
	addMutationHelper(t, ol, edge, Set, txn)
	require.EqualValues(t, 1, ol.Length(txn.StartTs, 0))
	checkValue(t, ol, "someothercars", txn.StartTs)

	// Set value back to the committed value cars, but don't merge yet.
	edge = &protos.DirectedEdge{
		Value: []byte("cars"),
		Label: "jchiu",
	}
	addMutationHelper(t, ol, edge, Set, txn)
	require.EqualValues(t, 1, ol.Length(txn.StartTs, 0))
	checkValue(t, ol, "cars", txn.StartTs)
}

func TestAddMutation_jchiu2(t *testing.T) {
	key := x.DataKey("value", 15)
	ol := Get(key)

	// Del a value cars and but don't merge.
	edge := &protos.DirectedEdge{
		Value: []byte("cars"),
		Label: "jchiu",
	}
	txn := &Txn{StartTs: 1}
	addMutationHelper(t, ol, edge, Del, txn)
	require.EqualValues(t, 0, ol.Length(txn.StartTs, 0))

	// Set value to newcars, but don't merge yet.
	edge = &protos.DirectedEdge{
		Value: []byte("newcars"),
		Label: "jchiu",
	}
	addMutationHelper(t, ol, edge, Set, txn)
	require.EqualValues(t, 1, ol.Length(txn.StartTs, 0))
	checkValue(t, ol, "newcars", txn.StartTs)

	// Del a value cars. This operation should be ignored.
	edge = &protos.DirectedEdge{
		Value: []byte("cars"),
		Label: "jchiu",
	}
	addMutationHelper(t, ol, edge, Del, txn)
	require.EqualValues(t, 1, ol.Length(txn.StartTs, 0))
	checkValue(t, ol, "newcars", txn.StartTs)
}

func TestAddMutation_jchiu2_Commit(t *testing.T) {
	key := x.DataKey("value", 16)
	ol := Get(key)

	// Del a value cars and but don't merge.
	edge := &protos.DirectedEdge{
		Value: []byte("cars"),
		Label: "jchiu",
	}
	txn := &Txn{StartTs: 1}
	addMutationHelper(t, ol, edge, Del, txn)
	ol.CommitMutation(context.Background(), 1, uint64(2))
	require.EqualValues(t, 0, ol.Length(uint64(3), 0))

	// Set value to newcars, but don't merge yet.
	edge = &protos.DirectedEdge{
		Value: []byte("newcars"),
		Label: "jchiu",
	}
	txn = &Txn{StartTs: 3}
	addMutationHelper(t, ol, edge, Set, txn)
	ol.CommitMutation(context.Background(), 3, uint64(4))
	require.EqualValues(t, 1, ol.Length(5, 0))
	checkValue(t, ol, "newcars", 5)

	// Del a value cars. This operation should be ignored.
	edge = &protos.DirectedEdge{
		Value: []byte("cars"),
		Label: "jchiu",
	}
	txn = &Txn{StartTs: 5}
	addMutationHelper(t, ol, edge, Del, txn)
	ol.CommitMutation(context.Background(), 5, uint64(6))
	require.EqualValues(t, 1, ol.Length(txn.StartTs+1, 0))
	checkValue(t, ol, "newcars", txn.StartTs+1)
}

func TestAddMutation_jchiu3(t *testing.T) {
	key := x.DataKey("value", 29)
	ol := Get(key)

	// Set value to cars and merge to RocksDB.
	edge := &protos.DirectedEdge{
		Value: []byte("cars"),
		Label: "jchiu",
	}
	txn := &Txn{StartTs: 1}
	addMutationHelper(t, ol, edge, Set, txn)
	ol.CommitMutation(context.Background(), 1, uint64(2))
	require.Equal(t, 1, ol.Length(uint64(3), 0))
	merged, err := ol.SyncIfDirty(false)
	require.NoError(t, err)
	require.True(t, merged)
	require.EqualValues(t, 1, ol.Length(uint64(3), 0))
	checkValue(t, ol, "cars", uint64(3))

	// Del a value cars and but don't merge.
	edge = &protos.DirectedEdge{
		Value: []byte("cars"),
		Label: "jchiu",
	}
	txn = &Txn{StartTs: 3}
	addMutationHelper(t, ol, edge, Del, txn)
	require.Equal(t, 0, ol.Length(txn.StartTs, 0))

	// Set value to newcars, but don't merge yet.
	edge = &protos.DirectedEdge{
		Value: []byte("newcars"),
		Label: "jchiu",
	}
	addMutationHelper(t, ol, edge, Set, txn)
	require.EqualValues(t, 1, ol.Length(txn.StartTs, 0))
	checkValue(t, ol, "newcars", txn.StartTs)

	// Del a value othercars and but don't merge.
	edge = &protos.DirectedEdge{
		Value: []byte("othercars"),
		Label: "jchiu",
	}
	addMutationHelper(t, ol, edge, Del, txn)
	require.NotEqual(t, 0, ol.Length(txn.StartTs, 0))
	checkValue(t, ol, "newcars", txn.StartTs)

	// Del a value newcars and but don't merge.
	edge = &protos.DirectedEdge{
		Value: []byte("newcars"),
		Label: "jchiu",
	}
	addMutationHelper(t, ol, edge, Del, txn)
	require.Equal(t, 0, ol.Length(txn.StartTs, 0))
}

func TestAddMutation_mrjn1(t *testing.T) {
	key := x.DataKey("value", 21)
	ol := Get(key)

	// Set a value cars and merge.
	edge := &protos.DirectedEdge{
		Value: []byte("cars"),
		Label: "jchiu",
	}
	txn := &Txn{StartTs: 1}
	addMutationHelper(t, ol, edge, Set, txn)
	ol.CommitMutation(context.Background(), 1, uint64(2))
	merged, err := ol.SyncIfDirty(false)
	require.NoError(t, err)
	require.True(t, merged)

	// Delete a non-existent value newcars. This should have no effect.
	edge = &protos.DirectedEdge{
		Value: []byte("newcars"),
		Label: "jchiu",
	}
	txn = &Txn{StartTs: 3}
	addMutationHelper(t, ol, edge, Del, txn)
	checkValue(t, ol, "cars", txn.StartTs)

	// Delete the previously committed value cars. But don't merge.
	edge = &protos.DirectedEdge{
		Value: []byte("cars"),
		Label: "jchiu",
	}
	addMutationHelper(t, ol, edge, Del, txn)
	require.Equal(t, 0, ol.Length(txn.StartTs, 0))

	// Do this again to cover Del, muid == curUid, inPlist test case.
	// Delete the previously committed value cars. But don't merge.
	edge = &protos.DirectedEdge{
		Value: []byte("cars"),
		Label: "jchiu",
	}
	addMutationHelper(t, ol, edge, Del, txn)
	require.Equal(t, 0, ol.Length(txn.StartTs, 0))

	// Set the value again to cover Set, muid == curUid, inPlist test case.
	// Set the previously committed value cars. But don't merge.
	edge = &protos.DirectedEdge{
		Value: []byte("cars"),
		Label: "jchiu",
	}
	addMutationHelper(t, ol, edge, Set, txn)
	checkValue(t, ol, "cars", txn.StartTs)

	// Delete it again, just for fun.
	edge = &protos.DirectedEdge{
		Value: []byte("cars"),
		Label: "jchiu",
	}
	addMutationHelper(t, ol, edge, Del, txn)
	require.Equal(t, 0, ol.Length(txn.StartTs, 0))
}

func TestAddMutation_gru(t *testing.T) {
	key := x.DataKey("question.tag", 0x01)
	ol, err := getNew(key, ps)
	require.NoError(t, err)

	{
		// Set two tag ids and merge.
		edge := &protos.DirectedEdge{
			ValueId: 0x2b693088816b04b7,
			Label:   "gru",
		}
		txn := &Txn{StartTs: 1}
		addMutationHelper(t, ol, edge, Set, txn)
		edge = &protos.DirectedEdge{
			ValueId: 0x29bf442b48a772e0,
			Label:   "gru",
		}
		addMutationHelper(t, ol, edge, Set, txn)
		ol.CommitMutation(context.Background(), 1, uint64(2))
		merged, err := ol.SyncIfDirty(false)
		require.NoError(t, err)
		require.True(t, merged)
	}

	{
		edge := &protos.DirectedEdge{
			ValueId: 0x38dec821d2ac3a79,
			Label:   "gru",
		}
		txn := &Txn{StartTs: 3}
		addMutationHelper(t, ol, edge, Set, txn)
		edge = &protos.DirectedEdge{
			ValueId: 0x2b693088816b04b7,
			Label:   "gru",
		}
		addMutationHelper(t, ol, edge, Del, txn)
		ol.CommitMutation(context.Background(), 3, uint64(4))
		merged, err := ol.SyncIfDirty(false)
		require.NoError(t, err)
		require.True(t, merged)
	}
}

func TestAddMutation_gru2(t *testing.T) {
	key := x.DataKey("question.tag", 0x100)
	ol, err := getNew(key, ps)
	require.NoError(t, err)

	{
		// Set two tag ids and merge.
		edge := &protos.DirectedEdge{
			ValueId: 0x02,
			Label:   "gru",
		}
		txn := &Txn{StartTs: 1}
		addMutationHelper(t, ol, edge, Set, txn)
		edge = &protos.DirectedEdge{
			ValueId: 0x03,
			Label:   "gru",
		}
		txn = &Txn{StartTs: 1}
		addMutationHelper(t, ol, edge, Set, txn)
		ol.CommitMutation(context.Background(), 1, uint64(2))
		merged, err := ol.SyncIfDirty(false)
		require.NoError(t, err)
		require.True(t, merged)
	}

	{
		// Lets set a new tag and delete the two older ones.
		edge := &protos.DirectedEdge{
			ValueId: 0x02,
			Label:   "gru",
		}
		txn := &Txn{StartTs: 3}
		addMutationHelper(t, ol, edge, Del, txn)
		edge = &protos.DirectedEdge{
			ValueId: 0x03,
			Label:   "gru",
		}
		addMutationHelper(t, ol, edge, Del, txn)

		edge = &protos.DirectedEdge{
			ValueId: 0x04,
			Label:   "gru",
		}
		addMutationHelper(t, ol, edge, Set, txn)

		merged, err := ol.SyncIfDirty(false)
		ol.CommitMutation(context.Background(), 3, uint64(4))
		require.Equal(t, err, errUncomitted)
		require.False(t, merged)
	}

	// Posting list should just have the new tag.
	uids := []uint64{0x04}
	require.Equal(t, uids, listToArray(t, 0, ol, uint64(5)))
}

func TestAfterUIDCount(t *testing.T) {
	key := x.DataKey("value", 22)
	ol, err := getNew(key, ps)
	require.NoError(t, err)
	// Set value to cars and merge to RocksDB.
	edge := &protos.DirectedEdge{
		Label: "jchiu",
	}

	txn := &Txn{StartTs: 1}
	for i := 100; i < 300; i++ {
		edge.ValueId = uint64(i)
		addMutationHelper(t, ol, edge, Set, txn)
	}
	require.EqualValues(t, 200, ol.Length(txn.StartTs, 0))
	require.EqualValues(t, 100, ol.Length(txn.StartTs, 199))
	require.EqualValues(t, 0, ol.Length(txn.StartTs, 300))

	// Delete half of the edges.
	for i := 100; i < 300; i += 2 {
		edge.ValueId = uint64(i)
		addMutationHelper(t, ol, edge, Del, txn)
	}
	require.EqualValues(t, 100, ol.Length(txn.StartTs, 0))
	require.EqualValues(t, 50, ol.Length(txn.StartTs, 199))
	require.EqualValues(t, 0, ol.Length(txn.StartTs, 300))

	// Try to delete half of the edges. Redundant deletes.
	for i := 100; i < 300; i += 2 {
		edge.ValueId = uint64(i)
		addMutationHelper(t, ol, edge, Del, txn)
	}
	require.EqualValues(t, 100, ol.Length(txn.StartTs, 0))
	require.EqualValues(t, 50, ol.Length(txn.StartTs, 199))
	require.EqualValues(t, 0, ol.Length(txn.StartTs, 300))

	// Delete everything.
	for i := 100; i < 300; i++ {
		edge.ValueId = uint64(i)
		addMutationHelper(t, ol, edge, Del, txn)
	}
	require.EqualValues(t, 0, ol.Length(txn.StartTs, 0))
	require.EqualValues(t, 0, ol.Length(txn.StartTs, 199))
	require.EqualValues(t, 0, ol.Length(txn.StartTs, 300))

	// Insert 1/4 of the edges.
	for i := 100; i < 300; i += 4 {
		edge.ValueId = uint64(i)
		addMutationHelper(t, ol, edge, Set, txn)
	}
	require.EqualValues(t, 50, ol.Length(txn.StartTs, 0))
	require.EqualValues(t, 25, ol.Length(txn.StartTs, 199))
	require.EqualValues(t, 0, ol.Length(txn.StartTs, 300))

	// Insert 1/4 of the edges.
	edge.Label = "somethingelse"
	for i := 100; i < 300; i += 4 {
		edge.ValueId = uint64(i)
		addMutationHelper(t, ol, edge, Set, txn)
	}
	require.EqualValues(t, 50, ol.Length(txn.StartTs, 0)) // Expect no change.
	require.EqualValues(t, 25, ol.Length(txn.StartTs, 199))
	require.EqualValues(t, 0, ol.Length(txn.StartTs, 300))

	// Insert 1/4 of the edges.
	for i := 103; i < 300; i += 4 {
		edge.ValueId = uint64(i)
		addMutationHelper(t, ol, edge, Set, txn)
	}
	require.EqualValues(t, 100, ol.Length(txn.StartTs, 0))
	require.EqualValues(t, 50, ol.Length(txn.StartTs, 199))
	require.EqualValues(t, 0, ol.Length(txn.StartTs, 300))
}

func TestAfterUIDCount2(t *testing.T) {
	key := x.DataKey("value", 23)
	ol, err := getNew(key, ps)
	require.NoError(t, err)

	// Set value to cars and merge to RocksDB.
	edge := &protos.DirectedEdge{
		Label: "jchiu",
	}

	txn := &Txn{StartTs: 1}
	for i := 100; i < 300; i++ {
		edge.ValueId = uint64(i)
		addMutationHelper(t, ol, edge, Set, txn)
	}
	require.EqualValues(t, 200, ol.Length(txn.StartTs, 0))
	require.EqualValues(t, 100, ol.Length(txn.StartTs, 199))
	require.EqualValues(t, 0, ol.Length(txn.StartTs, 300))

	// Re-insert 1/4 of the edges. Counts should not change.
	edge.Label = "somethingelse"
	for i := 100; i < 300; i += 4 {
		edge.ValueId = uint64(i)
		addMutationHelper(t, ol, edge, Set, txn)
	}
	require.EqualValues(t, 200, ol.Length(txn.StartTs, 0))
	require.EqualValues(t, 100, ol.Length(txn.StartTs, 199))
	require.EqualValues(t, 0, ol.Length(txn.StartTs, 300))
}

func TestDelete(t *testing.T) {
	key := x.DataKey("value", 25)
	ol, err := getNew(key, ps)
	require.NoError(t, err)

	// Set value to cars and merge to RocksDB.
	edge := &protos.DirectedEdge{
		Label: "jchiu",
	}

	txn := &Txn{StartTs: 1}
	for i := 1; i <= 30; i++ {
		edge.ValueId = uint64(i)
		addMutationHelper(t, ol, edge, Set, txn)
	}
	require.EqualValues(t, 30, ol.Length(txn.StartTs, 0))
	edge.Value = []byte(x.Star)
	addMutationHelper(t, ol, edge, Del, txn)
	require.EqualValues(t, 0, ol.Length(txn.StartTs, 0))
	ol.CommitMutation(context.Background(), txn.StartTs, txn.StartTs+1)
	commited, err := ol.SyncIfDirty(false)
	require.NoError(t, err)
	require.True(t, commited)

	require.EqualValues(t, 0, ol.Length(txn.StartTs+2, 0))
}

func TestAfterUIDCountWithCommit(t *testing.T) {
	key := x.DataKey("value", 26)
	ol, err := getNew(key, ps)
	require.NoError(t, err)

	// Set value to cars and merge to RocksDB.
	edge := &protos.DirectedEdge{
		Label: "jchiu",
	}

	txn := &Txn{StartTs: 1}
	for i := 100; i < 400; i++ {
		edge.ValueId = uint64(i)
		addMutationHelper(t, ol, edge, Set, txn)
	}
	require.EqualValues(t, 300, ol.Length(txn.StartTs, 0))
	require.EqualValues(t, 200, ol.Length(txn.StartTs, 199))
	require.EqualValues(t, 0, ol.Length(txn.StartTs, 400))

	// Commit to database.
	ol.CommitMutation(context.Background(), txn.StartTs, txn.StartTs+1)
	merged, err := ol.SyncIfDirty(false)
	require.NoError(t, err)
	require.True(t, merged)

	txn = &Txn{StartTs: 3}
	// Mutation layer starts afresh from here.
	// Delete half of the edges.
	for i := 100; i < 400; i += 2 {
		edge.ValueId = uint64(i)
		addMutationHelper(t, ol, edge, Del, txn)
	}
	require.EqualValues(t, 150, ol.Length(txn.StartTs, 0))
	require.EqualValues(t, 100, ol.Length(txn.StartTs, 199))
	require.EqualValues(t, 0, ol.Length(txn.StartTs, 400))

	// Try to delete half of the edges. Redundant deletes.
	for i := 100; i < 400; i += 2 {
		edge.ValueId = uint64(i)
		addMutationHelper(t, ol, edge, Del, txn)
	}
	require.EqualValues(t, 150, ol.Length(txn.StartTs, 0))
	require.EqualValues(t, 100, ol.Length(txn.StartTs, 199))
	require.EqualValues(t, 0, ol.Length(txn.StartTs, 400))

	// Delete everything.
	for i := 100; i < 400; i++ {
		edge.ValueId = uint64(i)
		addMutationHelper(t, ol, edge, Del, txn)
	}
	require.EqualValues(t, 0, ol.Length(txn.StartTs, 0))
	require.EqualValues(t, 0, ol.Length(txn.StartTs, 199))
	require.EqualValues(t, 0, ol.Length(txn.StartTs, 400))

	// Insert 1/4 of the edges.
	for i := 100; i < 300; i += 4 {
		edge.ValueId = uint64(i)
		addMutationHelper(t, ol, edge, Set, txn)
	}
	require.EqualValues(t, 50, ol.Length(txn.StartTs, 0))
	require.EqualValues(t, 25, ol.Length(txn.StartTs, 199))
	require.EqualValues(t, 0, ol.Length(txn.StartTs, 300))

	// Insert 1/4 of the edges.
	edge.Label = "somethingelse"
	for i := 100; i < 300; i += 4 {
		edge.ValueId = uint64(i)
		addMutationHelper(t, ol, edge, Set, txn)
	}
	require.EqualValues(t, 50, ol.Length(txn.StartTs, 0)) // Expect no change.
	require.EqualValues(t, 25, ol.Length(txn.StartTs, 199))
	require.EqualValues(t, 0, ol.Length(txn.StartTs, 300))

	// Insert 1/4 of the edges.
	for i := 103; i < 300; i += 4 {
		edge.ValueId = uint64(i)
		addMutationHelper(t, ol, edge, Set, txn)
	}
	require.EqualValues(t, 100, ol.Length(txn.StartTs, 0))
	require.EqualValues(t, 50, ol.Length(txn.StartTs, 199))
	require.EqualValues(t, 0, ol.Length(txn.StartTs, 300))
}

var ps *badger.ManagedDB

func TestMain(m *testing.M) {
	x.Init(true)
	Config.AllottedMemory = 1024.0
	Config.CommitFraction = 0.10

	dir, err := ioutil.TempDir("", "storetest_")
	x.Check(err)

	opt := badger.DefaultOptions
	opt.Dir = dir
	opt.ValueDir = dir
	ps, err = badger.OpenManaged(opt)
	x.Check(err)
	Init(ps)
	schema.Init(ps)

	r := m.Run()

	os.RemoveAll(dir)
	os.Exit(r)
}

func BenchmarkAddMutations(b *testing.B) {
	key := x.DataKey("name", 1)
	l, err := getNew(key, ps)
	if err != nil {
		b.Error(err)
	}
	b.ResetTimer()

	ctx := context.Background()
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
		txn := &Txn{StartTs: 1}
		if _, err = l.AddMutation(ctx, txn, edge); err != nil {
			b.Error(err)
		}
	}
}
