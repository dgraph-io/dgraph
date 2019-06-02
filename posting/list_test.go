/*
 * Copyright 2015-2018 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
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
	"math"
	"math/rand"
	"os"
	"sort"
	"strconv"
	"testing"

	"github.com/dgraph-io/badger"
	bpb "github.com/dgraph-io/badger/pb"
	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/x"
)

func (l *List) PostingList() *pb.PostingList {
	l.RLock()
	defer l.RUnlock()
	return l.plist
}

func listToArray(t *testing.T, afterUid uint64, l *List, readTs uint64) []uint64 {
	out := make([]uint64, 0, 10)
	l.Iterate(readTs, afterUid, func(p *pb.Posting) error {
		out = append(out, p.Uid)
		return nil
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

func addMutationHelper(t *testing.T, l *List, edge *pb.DirectedEdge, op uint32, txn *Txn) {
	if op == Del {
		edge.Op = pb.DirectedEdge_DEL
	} else if op == Set {
		edge.Op = pb.DirectedEdge_SET
	} else {
		x.Fatalf("Unhandled op: %v", op)
	}
	err := l.addMutation(context.Background(), txn, edge)
	require.NoError(t, err)
}

func (l *List) commitMutation(startTs, commitTs uint64) error {
	l.Lock()
	defer l.Unlock()

	plist, ok := l.mutationMap[startTs]
	if !ok {
		// It was already committed, might be happening due to replay.
		return nil
	}
	if commitTs == 0 {
		// Abort mutation.
		delete(l.mutationMap, startTs)
		return nil
	}

	// We have a valid commit.
	plist.CommitTs = commitTs
	for _, mpost := range plist.Postings {
		mpost.CommitTs = commitTs
	}

	// In general, a posting list shouldn't try to mix up it's job of keeping
	// things in memory, with writing things to disk. A separate process can
	// roll up and write them to disk. posting list should only keep things in
	// memory, to make it available for transactions. So, all we need to do here
	// is to roll them up periodically, now being done by draft.go.
	// For the PLs in memory, we roll them up after we do the disk rollup.
	return nil
}

func TestAddMutation(t *testing.T) {
	key := x.DataKey("name", 2)

	txn := NewTxn(1)
	l, err := txn.Get(key)
	require.NoError(t, err)

	edge := &pb.DirectedEdge{
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
	l.commitMutation(1, 2)

	uids := []uint64{9, 69, 81}
	checkUids(t, l, uids, 3)

	p = getFirst(l, 3)
	require.NotNil(t, p, "Unable to retrieve posting")
	require.EqualValues(t, "anti-testing", p.Label)
}

func getFirst(l *List, readTs uint64) (res pb.Posting) {
	l.Iterate(readTs, 0, func(p *pb.Posting) error {
		res = *p
		return errStopIteration
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
	edge := &pb.DirectedEdge{
		Value: []byte("oh hey there"),
		Label: "new-testing",
	}
	txn := &Txn{StartTs: 1}
	addMutationHelper(t, ol, edge, Set, txn)
	checkValue(t, ol, "oh hey there", txn.StartTs)

	// Run the same check after committing.
	ol.commitMutation(txn.StartTs, txn.StartTs+1)
	checkValue(t, ol, "oh hey there", uint64(3))

	// The value made it to the posting list. Changing it now.
	edge.Value = []byte(strconv.Itoa(119))
	txn = &Txn{StartTs: 3}
	addMutationHelper(t, ol, edge, Set, txn)
	checkValue(t, ol, "119", txn.StartTs)
}

func TestAddMutation_jchiu1(t *testing.T) {
	key := x.DataKey("value", 12)
	ol, err := GetNoStore(key)
	require.NoError(t, err)

	// Set value to cars and merge to BadgerDB.
	edge := &pb.DirectedEdge{
		Value: []byte("cars"),
		Label: "jchiu",
	}
	txn := &Txn{StartTs: 1}
	addMutationHelper(t, ol, edge, Set, txn)
	ol.commitMutation(1, uint64(2))

	// TODO: Read at commitTimestamp with all committed
	require.EqualValues(t, 1, ol.Length(uint64(3), 0))
	checkValue(t, ol, "cars", uint64(3))

	txn = &Txn{StartTs: 3}
	// Set value to newcars, but don't merge yet.
	edge = &pb.DirectedEdge{
		Value: []byte("newcars"),
		Label: "jchiu",
	}
	addMutationHelper(t, ol, edge, Set, txn)
	require.EqualValues(t, 1, ol.Length(txn.StartTs, 0))
	checkValue(t, ol, "newcars", txn.StartTs)

	// Set value to someothercars, but don't merge yet.
	edge = &pb.DirectedEdge{
		Value: []byte("someothercars"),
		Label: "jchiu",
	}
	addMutationHelper(t, ol, edge, Set, txn)
	require.EqualValues(t, 1, ol.Length(txn.StartTs, 0))
	checkValue(t, ol, "someothercars", txn.StartTs)

	// Set value back to the committed value cars, but don't merge yet.
	edge = &pb.DirectedEdge{
		Value: []byte("cars"),
		Label: "jchiu",
	}
	addMutationHelper(t, ol, edge, Set, txn)
	require.EqualValues(t, 1, ol.Length(txn.StartTs, 0))
	checkValue(t, ol, "cars", txn.StartTs)
}

func TestAddMutation_DelSet(t *testing.T) {
	key := x.DataKey("value", 1534)
	ol, err := GetNoStore(key)
	require.NoError(t, err)

	// DO sp*, don't commit
	// Del a value cars and but don't merge.
	edge := &pb.DirectedEdge{
		Value: []byte(x.Star),
		Op:    pb.DirectedEdge_DEL,
	}
	txn := &Txn{StartTs: 1}
	err = ol.addMutation(context.Background(), txn, edge)
	require.NoError(t, err)

	// Set value to newcars, commit it
	edge = &pb.DirectedEdge{
		Value: []byte("newcars"),
	}
	txn = &Txn{StartTs: 2}
	addMutationHelper(t, ol, edge, Set, txn)
	ol.commitMutation(2, uint64(3))
	require.EqualValues(t, 1, ol.Length(3, 0))
	checkValue(t, ol, "newcars", 3)
}

func TestAddMutation_DelRead(t *testing.T) {
	key := x.DataKey("value", 1543)
	ol, err := GetNoStore(key)
	require.NoError(t, err)

	// Set value to newcars, and commit it
	edge := &pb.DirectedEdge{
		Value: []byte("newcars"),
	}
	txn := &Txn{StartTs: 1}
	addMutationHelper(t, ol, edge, Set, txn)
	ol.commitMutation(1, uint64(2))
	require.EqualValues(t, 1, ol.Length(2, 0))
	checkValue(t, ol, "newcars", 2)

	// DO sp*, don't commit
	// Del a value cars and but don't merge.
	edge = &pb.DirectedEdge{
		Value: []byte(x.Star),
		Op:    pb.DirectedEdge_DEL,
	}
	txn = &Txn{StartTs: 3}
	err = ol.addMutation(context.Background(), txn, edge)
	require.NoError(t, err)

	// Part of same transaction as sp*, so should see zero length even
	// if not committed yet.
	require.EqualValues(t, 0, ol.Length(3, 0))

	// Commit sp* only in oracle, don't apply to pl yet
	ol.commitMutation(3, 5)

	// This read should ignore sp*, since readts is 4 and it was committed at 5
	require.EqualValues(t, 1, ol.Length(4, 0))
	checkValue(t, ol, "newcars", 4)

	require.EqualValues(t, 0, ol.Length(6, 0))
}

func TestAddMutation_jchiu2(t *testing.T) {
	key := x.DataKey("value", 15)
	ol, err := GetNoStore(key)
	require.NoError(t, err)

	// Del a value cars and but don't merge.
	edge := &pb.DirectedEdge{
		Value: []byte("cars"),
		Label: "jchiu",
	}
	txn := &Txn{StartTs: 1}
	addMutationHelper(t, ol, edge, Del, txn)
	require.EqualValues(t, 0, ol.Length(txn.StartTs, 0))

	// Set value to newcars, but don't merge yet.
	edge = &pb.DirectedEdge{
		Value: []byte("newcars"),
		Label: "jchiu",
	}
	addMutationHelper(t, ol, edge, Set, txn)
	require.EqualValues(t, 1, ol.Length(txn.StartTs, 0))
	checkValue(t, ol, "newcars", txn.StartTs)
}

func TestAddMutation_jchiu2_Commit(t *testing.T) {
	key := x.DataKey("value", 16)
	ol, err := GetNoStore(key)
	require.NoError(t, err)

	// Del a value cars and but don't merge.
	edge := &pb.DirectedEdge{
		Value: []byte("cars"),
		Label: "jchiu",
	}
	txn := &Txn{StartTs: 1}
	addMutationHelper(t, ol, edge, Del, txn)
	ol.commitMutation(1, uint64(2))
	require.EqualValues(t, 0, ol.Length(uint64(3), 0))

	// Set value to newcars, but don't merge yet.
	edge = &pb.DirectedEdge{
		Value: []byte("newcars"),
		Label: "jchiu",
	}
	txn = &Txn{StartTs: 3}
	addMutationHelper(t, ol, edge, Set, txn)
	ol.commitMutation(3, uint64(4))
	require.EqualValues(t, 1, ol.Length(5, 0))
	checkValue(t, ol, "newcars", 5)
}

func TestAddMutation_jchiu3(t *testing.T) {
	key := x.DataKey("value", 29)
	ol, err := GetNoStore(key)
	require.NoError(t, err)

	// Set value to cars and merge to BadgerDB.
	edge := &pb.DirectedEdge{
		Value: []byte("cars"),
		Label: "jchiu",
	}
	txn := &Txn{StartTs: 1}
	addMutationHelper(t, ol, edge, Set, txn)
	ol.commitMutation(1, uint64(2))
	require.Equal(t, 1, ol.Length(uint64(3), 0))
	require.EqualValues(t, 1, ol.Length(uint64(3), 0))
	checkValue(t, ol, "cars", uint64(3))

	// Del a value cars and but don't merge.
	edge = &pb.DirectedEdge{
		Value: []byte("cars"),
		Label: "jchiu",
	}
	txn = &Txn{StartTs: 3}
	addMutationHelper(t, ol, edge, Del, txn)
	require.Equal(t, 0, ol.Length(txn.StartTs, 0))

	// Set value to newcars, but don't merge yet.
	edge = &pb.DirectedEdge{
		Value: []byte("newcars"),
		Label: "jchiu",
	}
	addMutationHelper(t, ol, edge, Set, txn)
	require.EqualValues(t, 1, ol.Length(txn.StartTs, 0))
	checkValue(t, ol, "newcars", txn.StartTs)

	// Del a value newcars and but don't merge.
	edge = &pb.DirectedEdge{
		Value: []byte("newcars"),
		Label: "jchiu",
	}
	addMutationHelper(t, ol, edge, Del, txn)
	require.Equal(t, 0, ol.Length(txn.StartTs, 0))
}

func TestAddMutation_mrjn1(t *testing.T) {
	key := x.DataKey("value", 21)
	ol, err := GetNoStore(key)
	require.NoError(t, err)

	// Set a value cars and merge.
	edge := &pb.DirectedEdge{
		Value: []byte("cars"),
		Label: "jchiu",
	}
	txn := &Txn{StartTs: 1}
	addMutationHelper(t, ol, edge, Set, txn)
	ol.commitMutation(1, uint64(2))

	// Delete the previously committed value cars. But don't merge.
	txn = &Txn{StartTs: 3}
	edge = &pb.DirectedEdge{
		Value: []byte("cars"),
		Label: "jchiu",
	}
	addMutationHelper(t, ol, edge, Del, txn)
	require.Equal(t, 0, ol.Length(txn.StartTs, 0))

	// Do this again to cover Del, muid == curUid, inPlist test case.
	// Delete the previously committed value cars. But don't merge.
	edge = &pb.DirectedEdge{
		Value: []byte("cars"),
		Label: "jchiu",
	}
	addMutationHelper(t, ol, edge, Del, txn)
	require.Equal(t, 0, ol.Length(txn.StartTs, 0))

	// Set the value again to cover Set, muid == curUid, inPlist test case.
	// Set the previously committed value cars. But don't merge.
	edge = &pb.DirectedEdge{
		Value: []byte("cars"),
		Label: "jchiu",
	}
	addMutationHelper(t, ol, edge, Set, txn)
	checkValue(t, ol, "cars", txn.StartTs)

	// Delete it again, just for fun.
	edge = &pb.DirectedEdge{
		Value: []byte("cars"),
		Label: "jchiu",
	}
	addMutationHelper(t, ol, edge, Del, txn)
	require.Equal(t, 0, ol.Length(txn.StartTs, 0))
}

func TestMillion(t *testing.T) {
	// Ensure list is stored in a single part.
	maxListSize = math.MaxInt32

	key := x.DataKey("bal", 1331)
	ol, err := getNew(key, ps)
	require.NoError(t, err)
	var commits int
	N := int(1e6)
	for i := 2; i <= N; i += 2 {
		edge := &pb.DirectedEdge{
			ValueId: uint64(i),
		}
		txn := Txn{StartTs: uint64(i)}
		addMutationHelper(t, ol, edge, Set, &txn)
		require.NoError(t, ol.commitMutation(uint64(i), uint64(i)+1))
		if i%10000 == 0 {
			// Do a rollup, otherwise, it gets too slow to add a million mutations to one posting
			// list.
			t.Logf("Start Ts: %d. Rolling up posting list.\n", txn.StartTs)
			kvs, err := ol.Rollup()
			require.NoError(t, err)
			require.NoError(t, writePostingListToDisk(kvs))
			ol, err = getNew(key, ps)
			require.NoError(t, err)
		}
		commits++
	}

	t.Logf("Completed a million writes.\n")
	opt := ListOptions{ReadTs: uint64(N) + 1}
	l, err := ol.Uids(opt)
	require.NoError(t, err)
	require.Equal(t, commits, len(l.Uids), "List of Uids received: %+v", l.Uids)
	for i, uid := range l.Uids {
		require.Equal(t, uint64(i+1)*2, uid)
	}
}

// Test the various mutate, commit and abort sequences.
func TestAddMutation_mrjn2(t *testing.T) {
	ctx := context.Background()
	key := x.DataKey("bal", 1001)
	ol, err := getNew(key, ps)
	require.NoError(t, err)
	var readTs uint64
	for readTs = 1; readTs < 10; readTs++ {
		edge := &pb.DirectedEdge{
			ValueId:   readTs,
			ValueType: pb.Posting_INT,
		}
		txn := &Txn{StartTs: readTs}
		addMutationHelper(t, ol, edge, Set, txn)
	}
	for i := 1; i < 10; i++ {
		// Each of these txns see their own write.
		opt := ListOptions{ReadTs: uint64(i)}
		list, err := ol.Uids(opt)
		require.NoError(t, err)
		require.EqualValues(t, 1, len(list.Uids))
		require.EqualValues(t, uint64(i), list.Uids[0])
	}
	require.EqualValues(t, 0, ol.Length(readTs, 0))
	require.NoError(t, ol.commitMutation(1, 0))
	require.NoError(t, ol.commitMutation(3, 4))
	require.NoError(t, ol.commitMutation(6, 10))
	require.NoError(t, ol.commitMutation(9, 14))
	require.EqualValues(t, 3, ol.Length(15, 0)) // The three commits.

	{
		edge := &pb.DirectedEdge{
			Value: []byte(x.Star),
			Op:    pb.DirectedEdge_DEL,
		}
		txn := &Txn{StartTs: 7}
		err := ol.addMutation(ctx, txn, edge)
		require.NoError(t, err)

		// Add edge just to test that the deletion still happens.
		edge = &pb.DirectedEdge{
			ValueId:   7,
			ValueType: pb.Posting_INT,
		}
		err = ol.addMutation(ctx, txn, edge)
		require.NoError(t, err)

		require.EqualValues(t, 3, ol.Length(15, 0)) // The three commits should still be found.
		require.NoError(t, ol.commitMutation(7, 11))

		require.EqualValues(t, 2, ol.Length(10, 0)) // Two commits should be found.
		require.EqualValues(t, 1, ol.Length(12, 0)) // Only one commit should be found.
		require.EqualValues(t, 2, ol.Length(15, 0)) // Only one commit should be found.
	}
	{
		edge := &pb.DirectedEdge{
			Value: []byte(x.Star),
			Op:    pb.DirectedEdge_DEL,
		}
		txn := &Txn{StartTs: 5}
		err := ol.addMutation(ctx, txn, edge)
		require.NoError(t, err)
		require.NoError(t, ol.commitMutation(5, 7))

		// Commits are:
		// 4, 7 (Delete *), 10, 11 (Delete *), 14
		require.EqualValues(t, 1, ol.Length(8, 0)) // Nothing below 8, but consider itself.
		require.NoError(t, ol.commitMutation(8, 0))
		require.EqualValues(t, 0, ol.Length(8, 0))  // Nothing <= 8.
		require.EqualValues(t, 1, ol.Length(10, 0)) // Find committed 10.
		require.EqualValues(t, 1, ol.Length(12, 0)) // Find committed 11.
		require.EqualValues(t, 2, ol.Length(15, 0)) // Find committed 14.
		opts := ListOptions{ReadTs: 15}
		list, err := ol.Uids(opts)
		require.NoError(t, err)
		require.EqualValues(t, 7, list.Uids[0])
		require.EqualValues(t, 9, list.Uids[1])
	}
}

func TestAddMutation_gru(t *testing.T) {
	key := x.DataKey("question.tag", 0x01)
	ol, err := getNew(key, ps)
	require.NoError(t, err)

	{
		// Set two tag ids and merge.
		edge := &pb.DirectedEdge{
			ValueId: 0x2b693088816b04b7,
			Label:   "gru",
		}
		txn := &Txn{StartTs: 1}
		addMutationHelper(t, ol, edge, Set, txn)
		edge = &pb.DirectedEdge{
			ValueId: 0x29bf442b48a772e0,
			Label:   "gru",
		}
		addMutationHelper(t, ol, edge, Set, txn)
		ol.commitMutation(1, uint64(2))
	}

	{
		edge := &pb.DirectedEdge{
			ValueId: 0x38dec821d2ac3a79,
			Label:   "gru",
		}
		txn := &Txn{StartTs: 3}
		addMutationHelper(t, ol, edge, Set, txn)
		edge = &pb.DirectedEdge{
			ValueId: 0x2b693088816b04b7,
			Label:   "gru",
		}
		addMutationHelper(t, ol, edge, Del, txn)
		ol.commitMutation(3, uint64(4))
	}
}

func TestAddMutation_gru2(t *testing.T) {
	key := x.DataKey("question.tag", 0x100)
	ol, err := getNew(key, ps)
	require.NoError(t, err)

	{
		// Set two tag ids and merge.
		edge := &pb.DirectedEdge{
			ValueId: 0x02,
			Label:   "gru",
		}
		txn := &Txn{StartTs: 1}
		addMutationHelper(t, ol, edge, Set, txn)
		edge = &pb.DirectedEdge{
			ValueId: 0x03,
			Label:   "gru",
		}
		txn = &Txn{StartTs: 1}
		addMutationHelper(t, ol, edge, Set, txn)
		ol.commitMutation(1, uint64(2))
	}

	{
		// Lets set a new tag and delete the two older ones.
		edge := &pb.DirectedEdge{
			ValueId: 0x02,
			Label:   "gru",
		}
		txn := &Txn{StartTs: 3}
		addMutationHelper(t, ol, edge, Del, txn)
		edge = &pb.DirectedEdge{
			ValueId: 0x03,
			Label:   "gru",
		}
		addMutationHelper(t, ol, edge, Del, txn)

		edge = &pb.DirectedEdge{
			ValueId: 0x04,
			Label:   "gru",
		}
		addMutationHelper(t, ol, edge, Set, txn)

		ol.commitMutation(3, uint64(4))
	}

	// Posting list should just have the new tag.
	uids := []uint64{0x04}
	require.Equal(t, uids, listToArray(t, 0, ol, uint64(5)))
}

func TestAddAndDelMutation(t *testing.T) {
	// Ensure each test uses unique key since we don't clear the postings
	// after each test
	key := x.DataKey("dummy_key", 0x927)
	ol, err := getNew(key, ps)
	require.NoError(t, err)

	{
		edge := &pb.DirectedEdge{
			ValueId: 0x02,
			Label:   "gru",
		}
		txn := &Txn{StartTs: 1}
		addMutationHelper(t, ol, edge, Set, txn)
		ol.commitMutation(1, uint64(2))
	}

	{
		edge := &pb.DirectedEdge{
			ValueId: 0x02,
			Label:   "gru",
		}
		txn := &Txn{StartTs: 3}
		addMutationHelper(t, ol, edge, Del, txn)
		addMutationHelper(t, ol, edge, Del, txn)
		ol.commitMutation(3, uint64(4))

		checkUids(t, ol, []uint64{}, 5)
	}
	checkUids(t, ol, []uint64{}, 5)
}

func TestAfterUIDCount(t *testing.T) {
	key := x.DataKey("value", 22)
	ol, err := getNew(key, ps)
	require.NoError(t, err)
	// Set value to cars and merge to BadgerDB.
	edge := &pb.DirectedEdge{
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

	// Set value to cars and merge to BadgerDB.
	edge := &pb.DirectedEdge{
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

	// Set value to cars and merge to BadgerDB.
	edge := &pb.DirectedEdge{
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
	ol.commitMutation(txn.StartTs, txn.StartTs+1)

	require.EqualValues(t, 0, ol.Length(txn.StartTs+2, 0))
}

func TestAfterUIDCountWithCommit(t *testing.T) {
	key := x.DataKey("value", 26)
	ol, err := getNew(key, ps)
	require.NoError(t, err)

	// Set value to cars and merge to BadgerDB.
	edge := &pb.DirectedEdge{
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
	ol.commitMutation(txn.StartTs, txn.StartTs+1)

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

func createMultiPartList(t *testing.T, size int, addLabel bool) (*List, int) {
	// For testing, set the max list size to a lower threshold.
	maxListSize = 5000
	defer func() {
		maxListSize = math.MaxInt32
	}()

	key := x.DataKey("multi-bal", 1331)
	ol, err := getNew(key, ps)
	require.NoError(t, err)
	commits := 0
	for i := 1; i <= size; i++ {
		edge := &pb.DirectedEdge{
			ValueId: uint64(i),
		}
		if addLabel {
			edge.Label = strconv.Itoa(i)
		}

		txn := Txn{StartTs: uint64(i)}
		addMutationHelper(t, ol, edge, Set, &txn)
		require.NoError(t, ol.commitMutation(uint64(i), uint64(i)+1))
		if i%2000 == 0 {
			kvs, err := ol.Rollup()
			require.NoError(t, err)
			require.NoError(t, writePostingListToDisk(kvs))
			ol, err = getNew(key, ps)
			require.NoError(t, err)
		}
		commits++
	}

	kvs, err := ol.Rollup()
	require.NoError(t, err)
	require.NoError(t, writePostingListToDisk(kvs))
	ol, err = getNew(key, ps)
	require.NoError(t, err)

	return ol, commits
}

func createAndDeleteMultiPartList(t *testing.T, size int) (*List, int) {
	// For testing, set the max list size to a lower threshold.
	maxListSize = 5000
	defer func() {
		maxListSize = math.MaxInt32
	}()

	key := x.DataKey("bal_del", 1331)
	ol, err := getNew(key, ps)
	require.NoError(t, err)
	commits := 0
	for i := 1; i <= size; i++ {
		edge := &pb.DirectedEdge{
			ValueId: uint64(i),
		}

		txn := Txn{StartTs: uint64(i)}
		addMutationHelper(t, ol, edge, Set, &txn)
		require.NoError(t, ol.commitMutation(uint64(i), uint64(i)+1))
		if i%2000 == 0 {
			kvs, err := ol.Rollup()
			require.NoError(t, err)
			require.NoError(t, writePostingListToDisk(kvs))
			ol, err = getNew(key, ps)
			require.NoError(t, err)
		}
		commits++
	}

	// Delete all the previously inserted entries from the list.
	baseStartTs := uint64(size) + 1
	for i := 1; i <= size; i++ {
		edge := &pb.DirectedEdge{
			ValueId: uint64(i),
		}
		txn := Txn{StartTs: baseStartTs + uint64(i)}
		addMutationHelper(t, ol, edge, Del, &txn)
		require.NoError(t, ol.commitMutation(baseStartTs+uint64(i), baseStartTs+uint64(i)+1))
		if i%2000 == 0 {
			kvs, err := ol.Rollup()
			require.NoError(t, err)
			require.NoError(t, writePostingListToDisk(kvs))
			ol, err = getNew(key, ps)
			require.NoError(t, err)
		}
		commits++
	}

	return ol, commits
}

func writePostingListToDisk(kvs []*bpb.KV) error {
	writer := NewTxnWriter(pstore)
	for _, kv := range kvs {
		if err := writer.SetAt(kv.Key, kv.Value, kv.UserMeta[0], kv.Version); err != nil {
			return err
		}
	}
	return writer.Flush()
}

// Create a multi-part list and verify all the uids are there.
func TestMultiPartListBasic(t *testing.T) {
	size := int(1e5)
	ol, commits := createMultiPartList(t, size, false)
	t.Logf("List parts %v", len(ol.plist.Splits))
	opt := ListOptions{ReadTs: uint64(size) + 1}
	l, err := ol.Uids(opt)
	require.NoError(t, err)
	require.Equal(t, commits, len(l.Uids), "List of Uids received: %+v", l.Uids)
	for i, uid := range l.Uids {
		require.Equal(t, uint64(i+1), uid)
	}
}

// Verify that iteration works with an afterUid value greater than zero.
func TestMultiPartListIterAfterUid(t *testing.T) {
	size := int(1e5)
	ol, _ := createMultiPartList(t, size, false)
	t.Logf("List parts %v", len(ol.plist.Splits))

	var visitedUids []uint64
	ol.Iterate(uint64(size+1), 50000, func(p *pb.Posting) error {
		visitedUids = append(visitedUids, p.Uid)
		return nil
	})
	require.Equal(t, 50000, len(visitedUids))
	for i, uid := range visitedUids {
		require.Equal(t, uint64(50000+i+1), uid)
	}
}

// Verify that postings can be retrieved in multi-part lists.
func TestMultiPartListWithPostings(t *testing.T) {
	size := int(1e5)
	ol, commits := createMultiPartList(t, size, true)
	t.Logf("List parts %v", len(ol.plist.Splits))

	var labels []string
	err := ol.Iterate(uint64(size)+1, 0, func(p *pb.Posting) error {
		if len(p.Label) > 0 {
			labels = append(labels, p.Label)
		}
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, commits, len(labels))
	for i, label := range labels {
		require.Equal(t, label, strconv.Itoa(int(i+1)))
	}
}

// Verify marshaling of multi-part lists.
func TestMultiPartListMarshal(t *testing.T) {
	size := int(1e5)
	ol, _ := createMultiPartList(t, size, false)
	t.Logf("List parts %v", len(ol.plist.Splits))

	kvs, err := ol.Rollup()
	require.NoError(t, err)
	require.Equal(t, len(kvs), len(ol.plist.Splits)+1)
	require.NoError(t, writePostingListToDisk(kvs))

	sort.Slice(kvs, func(i, j int) bool {
		return string(kvs[i].Key) < string(kvs[j].Key)
	})

	key := x.DataKey("multi-bal", 1331)
	require.Equal(t, key, kvs[0].Key)

	for i, startUid := range ol.plist.Splits {
		partKey := x.GetSplitKey(key, startUid)
		require.Equal(t, partKey, kvs[i+1].Key)
		part, err := ol.readListPart(startUid)
		require.NoError(t, err)
		data, err := part.Marshal()
		require.NoError(t, err)
		require.Equal(t, data, kvs[i+1].Value)
		require.Equal(t, []byte{BitCompletePosting}, kvs[i+1].UserMeta)
		require.Equal(t, ol.minTs, kvs[i+1].Version)
	}
}

// Verify that writing a multi-part list to disk works correctly.
func TestMultiPartListWriteToDisk(t *testing.T) {
	size := int(1e5)
	originalList, commits := createMultiPartList(t, size, false)

	kvs, err := originalList.Rollup()
	require.NoError(t, err)
	require.Equal(t, len(kvs), len(originalList.plist.Splits)+1)

	require.NoError(t, writePostingListToDisk(kvs))
	newList, err := getNew(kvs[0].Key, ps)
	require.NoError(t, err)

	opt := ListOptions{ReadTs: uint64(size) + 1}
	originalUids, err := originalList.Uids(opt)
	require.NoError(t, err)
	newUids, err := newList.Uids(opt)
	require.NoError(t, err)
	require.Equal(t, commits, len(originalUids.Uids))
	require.Equal(t, len(originalUids.Uids), len(newUids.Uids))
	for i := range originalUids.Uids {
		require.Equal(t, originalUids.Uids[i], newUids.Uids[i])
	}
}

// Verify that adding and deleting all the entries returns an empty list.
func TestMultiPartListDelete(t *testing.T) {
	size := int(1e4)
	ol, commits := createAndDeleteMultiPartList(t, size)
	t.Logf("List parts %v", len(ol.plist.Splits))
	require.Equal(t, size*2, commits)

	counter := 0
	ol.Iterate(math.MaxUint64, 0, func(p *pb.Posting) error {
		counter++
		return nil
	})
	require.Equal(t, 0, counter)

	kvs, err := ol.Rollup()
	require.NoError(t, err)
	require.Equal(t, len(kvs), 1)

	for _, kv := range kvs {
		require.Equal(t, []byte{BitEmptyPosting}, kv.UserMeta)
		require.Equal(t, ol.minTs, kv.Version)
	}
}

// Verify that the first part of a multi-part list is kept even when all its
// entries have been deleted. Do this by creating a list, deleting the first
// half, and ensuring iteration and mutation still work as expected.
func TestMultiPartListDeleteAndAdd(t *testing.T) {
	size := int(1e5)
	// For testing, set the max list size to a lower threshold.
	maxListSize = 5000
	defer func() {
		maxListSize = math.MaxInt32
	}()

	// Add entries to the maps.
	key := x.DataKey("del_add", 1331)
	ol, err := getNew(key, ps)
	require.NoError(t, err)
	for i := 1; i <= size; i++ {
		edge := &pb.DirectedEdge{
			ValueId: uint64(i),
		}

		txn := Txn{StartTs: uint64(i)}
		addMutationHelper(t, ol, edge, Set, &txn)
		require.NoError(t, ol.commitMutation(uint64(i), uint64(i)+1))
		if i%2000 == 0 {
			kvs, err := ol.Rollup()
			require.NoError(t, err)
			require.NoError(t, writePostingListToDisk(kvs))
			ol, err = getNew(key, ps)
			require.NoError(t, err)
		}
	}

	// Verify all entries are in the list.
	opt := ListOptions{ReadTs: math.MaxUint64}
	l, err := ol.Uids(opt)
	require.NoError(t, err)
	require.Equal(t, size, len(l.Uids), "List of Uids received: %+v", l.Uids)
	for i, uid := range l.Uids {
		require.Equal(t, uint64(i+1), uid)
	}

	// Delete the first half of the previously inserted entries from the list.
	baseStartTs := uint64(size) + 1
	for i := 1; i <= 50000; i++ {
		edge := &pb.DirectedEdge{
			ValueId: uint64(i),
		}
		txn := Txn{StartTs: baseStartTs + uint64(i)}
		addMutationHelper(t, ol, edge, Del, &txn)
		require.NoError(t, ol.commitMutation(baseStartTs+uint64(i), baseStartTs+uint64(i)+1))
		if i%2000 == 0 {
			kvs, err := ol.Rollup()
			require.NoError(t, err)
			require.NoError(t, writePostingListToDisk(kvs))
			ol, err = getNew(key, ps)
			require.NoError(t, err)
		}
	}

	// Rollup list at the end of all the deletions.
	kvs, err := ol.Rollup()
	require.NoError(t, err)
	require.NoError(t, writePostingListToDisk(kvs))
	ol, err = getNew(key, ps)
	require.NoError(t, err)

	// Verify that the entries were actually deleted.
	opt = ListOptions{ReadTs: math.MaxUint64}
	l, err = ol.Uids(opt)
	require.NoError(t, err)
	require.Equal(t, 50000, len(l.Uids), "List of Uids received: %+v", l.Uids)
	for i, uid := range l.Uids {
		require.Equal(t, 50000+uint64(i+1), uid)
	}

	// Re-add the entries that were just deleted.
	baseStartTs = uint64(2*size) + 1
	for i := 1; i <= 50000; i++ {
		edge := &pb.DirectedEdge{
			ValueId: uint64(i),
		}
		txn := Txn{StartTs: baseStartTs + uint64(i)}
		addMutationHelper(t, ol, edge, Set, &txn)
		require.NoError(t, ol.commitMutation(baseStartTs+uint64(i), baseStartTs+uint64(i)+1))

		if i%2000 == 0 {
			kvs, err := ol.Rollup()
			require.NoError(t, err)
			require.NoError(t, writePostingListToDisk(kvs))
			ol, err = getNew(key, ps)
			require.NoError(t, err)
		}
	}

	// Rollup list at the end of all the additions
	kvs, err = ol.Rollup()
	require.NoError(t, err)
	require.NoError(t, writePostingListToDisk(kvs))
	ol, err = getNew(key, ps)
	require.NoError(t, err)

	// Verify all entries are once again in the list.
	opt = ListOptions{ReadTs: math.MaxUint64}
	l, err = ol.Uids(opt)
	require.NoError(t, err)
	require.Equal(t, size, len(l.Uids), "List of Uids received: %+v", l.Uids)
	for i, uid := range l.Uids {
		require.Equal(t, uint64(i+1), uid)
	}
}

var ps *badger.DB

func TestMain(m *testing.M) {
	x.Init()
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
		edge := &pb.DirectedEdge{
			ValueId: uint64(rand.Intn(b.N) + 1),
			Label:   "testing",
			Op:      pb.DirectedEdge_SET,
		}
		txn := &Txn{StartTs: 1}
		if err = l.addMutation(ctx, txn, edge); err != nil {
			b.Error(err)
		}
	}
}
