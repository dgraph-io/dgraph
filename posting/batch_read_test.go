/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package posting

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/dgraph-io/dgraph/v25/protos/pb"
	"github.com/dgraph-io/dgraph/v25/x"
)

// The tests in this file pin GetBatchSinglePosting as a batched equivalent of
// GetSinglePosting: for every key, GetBatchSinglePosting(keys)[i] must return exactly what
// GetSinglePosting(keys[i]) returns — committed values from badger, the transaction's own
// local-cache deltas, nil for absent keys, and nil for keys wiped by a delete-all — with
// no cross-talk between positions of the batch.

// commitValuePosting writes a scalar value posting for key at (startTs -> commitTs)
// directly to pstore, following the TestCacheAfterDeltaUpdateRecieved pattern
// (mvcc_test.go): stage a delta on a registered txn, then CommitToDisk + Flush.
func commitValuePosting(t testing.TB, key []byte, value []byte, startTs, commitTs uint64) {
	t.Helper()
	p := &pb.PostingList{
		Postings: []*pb.Posting{{
			Uid:      1,
			Value:    value,
			Op:       Set,
			StartTs:  startTs,
			CommitTs: commitTs,
		}},
	}
	delta, err := proto.Marshal(p)
	require.NoError(t, err)

	txn := Oracle().RegisterStartTs(startTs)
	txn.cache.deltas[string(key)] = delta

	writer := NewTxnWriter(pstore)
	require.NoError(t, txn.CommitToDisk(writer, commitTs))
	require.NoError(t, writer.Flush())
}

// commitDeleteAll writes a delete-all (star) posting for key at (startTs -> commitTs).
func commitDeleteAll(t testing.TB, key []byte, startTs, commitTs uint64) {
	t.Helper()
	p := &pb.PostingList{
		Postings: []*pb.Posting{{
			Uid:      1,
			Value:    []byte(x.Star),
			Op:       Del,
			StartTs:  startTs,
			CommitTs: commitTs,
		}},
	}
	delta, err := proto.Marshal(p)
	require.NoError(t, err)

	txn := Oracle().RegisterStartTs(startTs)
	txn.cache.deltas[string(key)] = delta

	writer := NewTxnWriter(pstore)
	require.NoError(t, txn.CommitToDisk(writer, commitTs))
	require.NoError(t, writer.Flush())
}

// singleValue extracts the one posting value from a batch result.
func singleValue(t testing.TB, pl *pb.PostingList) []byte {
	t.Helper()
	require.NotNil(t, pl)
	require.Len(t, pl.Postings, 1)
	return pl.Postings[0].Value
}

// TestGetBatchSinglePostingOneRecord: insert exactly one value posting and commit it; a
// batch read of exactly that key must return one result holding exactly that value
// ("insert 1 record -> count is 1, and it is the inserted record only").
func TestGetBatchSinglePostingOneRecord(t *testing.T) {
	attr := x.AttrInRootNamespace("batchOneRecord")
	key := x.DataKey(attr, 1)
	commitValuePosting(t, key, []byte("Alice"), 3, 4)

	lc := NewLocalCache(10)
	pls, err := lc.GetBatchSinglePosting([][]byte{key})
	require.NoError(t, err)
	require.Len(t, pls, 1)
	require.Equal(t, []byte("Alice"), singleValue(t, pls[0]))
}

// TestGetBatchSinglePostingEachKeyOwnValue: 15 uids with 15 distinct committed values; a
// batch read must return each uid's OWN value at its own position — no reuse, no shift.
func TestGetBatchSinglePostingEachKeyOwnValue(t *testing.T) {
	attr := x.AttrInRootNamespace("batchOwnValue")
	keys := make([][]byte, 15)
	for i := 0; i < 15; i++ {
		keys[i] = x.DataKey(attr, uint64(i+1))
		commitValuePosting(t, keys[i], fmt.Appendf(nil, "name%d", i+1), uint64(2*i+3), uint64(2*i+4))
	}

	lc := NewLocalCache(100)
	pls, err := lc.GetBatchSinglePosting(keys)
	require.NoError(t, err)
	require.Len(t, pls, 15)
	for i := 0; i < 15; i++ {
		require.Equal(t, fmt.Sprintf("name%d", i+1), string(singleValue(t, pls[i])),
			"uid %d must get its own value", i+1)
	}
}

// TestGetBatchSinglePostingDeleteAll: three committed keys, the middle one wiped by a
// delete-all. The batch must return [v1, nil, v3] — one deleted key must not abort or
// blank the whole batch.
func TestGetBatchSinglePostingDeleteAll(t *testing.T) {
	attr := x.AttrInRootNamespace("batchDeleteAll")
	k1, k2, k3 := x.DataKey(attr, 1), x.DataKey(attr, 2), x.DataKey(attr, 3)
	commitValuePosting(t, k1, []byte("v1"), 3, 4)
	commitDeleteAll(t, k2, 5, 6)
	commitValuePosting(t, k3, []byte("v3"), 7, 8)

	lc := NewLocalCache(10)
	pls, err := lc.GetBatchSinglePosting([][]byte{k1, k2, k3})
	require.NoError(t, err)
	require.Len(t, pls, 3, "one delete-all key must not blank the whole batch")
	require.Equal(t, []byte("v1"), singleValue(t, pls[0]))
	require.True(t, pls[1] == nil || len(pls[1].Postings) == 0, "deleted key reads as absent")
	require.Equal(t, []byte("v3"), singleValue(t, pls[2]))
}

// TestGetBatchSinglePostingAbsentKey: an absent key among present ones must read as
// absent (nil or zero postings), and must not disturb its neighbors' positions.
func TestGetBatchSinglePostingAbsentKey(t *testing.T) {
	attr := x.AttrInRootNamespace("batchAbsent")
	k1, k3 := x.DataKey(attr, 1), x.DataKey(attr, 3)
	absent := x.DataKey(attr, 2)
	commitValuePosting(t, k1, []byte("v1"), 3, 4)
	commitValuePosting(t, k3, []byte("v3"), 5, 6)

	lc := NewLocalCache(10)
	pls, err := lc.GetBatchSinglePosting([][]byte{k1, absent, k3})
	require.NoError(t, err)
	require.Len(t, pls, 3)
	require.Equal(t, []byte("v1"), singleValue(t, pls[0]))
	require.True(t, pls[1] == nil || len(pls[1].Postings) == 0, "absent key reads as absent")
	require.Equal(t, []byte("v3"), singleValue(t, pls[2]))
}

// TestGetBatchSinglePostingSeesLocalDelta: a key whose value exists only as an
// uncommitted delta in the LocalCache (this transaction's own write) must be answered
// from the cache — read-your-own-writes — not from badger (where it does not exist).
func TestGetBatchSinglePostingSeesLocalDelta(t *testing.T) {
	attr := x.AttrInRootNamespace("batchLocalDelta")
	key := x.DataKey(attr, 1)

	p := &pb.PostingList{
		Postings: []*pb.Posting{{
			Uid:   1,
			Value: []byte("uncommitted"),
			Op:    Set,
		}},
	}
	delta, err := proto.Marshal(p)
	require.NoError(t, err)

	lc := NewLocalCache(10)
	lc.deltas[string(key)] = delta

	pls, err := lc.GetBatchSinglePosting([][]byte{key})
	require.NoError(t, err)
	require.Len(t, pls, 1)
	require.Equal(t, []byte("uncommitted"), singleValue(t, pls[0]),
		"batch read must see the transaction's own uncommitted delta")
}

// TestGetBatchSinglePostingEquivalence: over a mixed state — committed keys, an absent
// key, a delete-all'ed key, and a local-cache delta — the batch result at every position
// must match GetSinglePosting for that key exactly.
func TestGetBatchSinglePostingEquivalence(t *testing.T) {
	attr := x.AttrInRootNamespace("batchEquivalence")
	kCommitted1 := x.DataKey(attr, 1)
	kAbsent := x.DataKey(attr, 2)
	kDeleted := x.DataKey(attr, 3)
	kDelta := x.DataKey(attr, 4)
	kCommitted2 := x.DataKey(attr, 5)

	commitValuePosting(t, kCommitted1, []byte("c1"), 3, 4)
	commitDeleteAll(t, kDeleted, 5, 6)
	commitValuePosting(t, kCommitted2, []byte("c2"), 7, 8)

	deltaPl := &pb.PostingList{
		Postings: []*pb.Posting{{Uid: 1, Value: []byte("mine"), Op: Set}},
	}
	delta, err := proto.Marshal(deltaPl)
	require.NoError(t, err)

	keys := [][]byte{kCommitted1, kAbsent, kDeleted, kDelta, kCommitted2}

	newCache := func() *LocalCache {
		lc := NewLocalCache(20)
		lc.deltas[string(kDelta)] = delta
		return lc
	}

	singles := make([]*pb.PostingList, len(keys))
	lcS := newCache()
	for i, key := range keys {
		singles[i], err = lcS.GetSinglePosting(key)
		require.NoError(t, err)
	}

	lcB := newCache()
	batch, err := lcB.GetBatchSinglePosting(keys)
	require.NoError(t, err)
	require.Len(t, batch, len(keys))

	for i := range keys {
		if singles[i] == nil || len(singles[i].Postings) == 0 {
			require.True(t, batch[i] == nil || len(batch[i].Postings) == 0,
				"key %d: single says absent, batch must agree", i)
			continue
		}
		require.NotNil(t, batch[i], "key %d: single has a value, batch must too", i)
		require.Equal(t, len(singles[i].Postings), len(batch[i].Postings), "key %d", i)
		require.Equal(t, singles[i].Postings[0].Value, batch[i].Postings[0].Value, "key %d", i)
	}
}

// TestBatchedSinglePostingIteratorEachKeyOwnValue: 25 committed keys with distinct
// values, consumed through the chunked iterator (batches of 10, as handleValuePostings
// does). Every sequential call must yield that position's own value — in particular
// across batch boundaries (positions 10 and 20) and past the final short batch.
func TestBatchedSinglePostingIteratorEachKeyOwnValue(t *testing.T) {
	attr := x.AttrInRootNamespace("batchIterOwnValue")
	keys := make([][]byte, 25)
	for i := 0; i < 25; i++ {
		keys[i] = x.DataKey(attr, uint64(i+1))
		commitValuePosting(t, keys[i], fmt.Appendf(nil, "val%d", i+1), uint64(2*i+3), uint64(2*i+4))
	}

	lc := NewLocalCache(100)
	next := lc.NewBatchedSinglePostingIterator(25, func(j int) []byte { return keys[j] })
	for i := 0; i < 25; i++ {
		pl, err := next()
		require.NoError(t, err)
		require.Equal(t, fmt.Sprintf("val%d", i+1), string(singleValue(t, pl)),
			"call %d must yield key %d's own value", i, i+1)
	}
}

// TestGetBatchSinglePostingDuplicateKeys: the same key twice in one batch must yield the
// same (correct) result at both positions — position bookkeeping must not skew.
func TestGetBatchSinglePostingDuplicateKeys(t *testing.T) {
	attr := x.AttrInRootNamespace("batchDupKeys")
	k1, k2 := x.DataKey(attr, 1), x.DataKey(attr, 2)
	commitValuePosting(t, k1, []byte("v1"), 3, 4)
	commitValuePosting(t, k2, []byte("v2"), 5, 6)

	lc := NewLocalCache(10)
	pls, err := lc.GetBatchSinglePosting([][]byte{k1, k2, k1})
	require.NoError(t, err)
	require.Len(t, pls, 3)
	require.Equal(t, []byte("v1"), singleValue(t, pls[0]))
	require.Equal(t, []byte("v2"), singleValue(t, pls[1]))
	require.Equal(t, []byte("v1"), singleValue(t, pls[2]), "duplicate key gets the same answer")
}

// TestGetBatchSinglePostingCorruptDelta: a local-cache delta that fails to unmarshal must
// surface as an error — not be silently treated as a cache miss or an empty value.
func TestGetBatchSinglePostingCorruptDelta(t *testing.T) {
	attr := x.AttrInRootNamespace("batchCorruptDelta")
	key := x.DataKey(attr, 1)

	lc := NewLocalCache(10)
	lc.deltas[string(key)] = []byte{0xff, 0x00, 0xba, 0xad} // not a valid PostingList

	_, err := lc.GetBatchSinglePosting([][]byte{key})
	require.Error(t, err, "corrupt cache delta must propagate an error")

	// And the single path must agree.
	_, errSingle := lc.GetSinglePosting(key)
	require.Error(t, errSingle)
}

// TestGetBatchSinglePostingSeesPlist: a key answered by the LocalCache's plists map (a
// List cached by this transaction) must be served from it, equivalently to the single
// path — the second of the two local-cache branches, deltas being the first.
func TestGetBatchSinglePostingSeesPlist(t *testing.T) {
	attr := x.AttrInRootNamespace("batchPlist")
	key := x.DataKey(attr, 1)
	commitValuePosting(t, key, []byte("from-badger"), 3, 4)

	// Prime a LocalCache so the List lands in lc.plists (readFromDisk path).
	lc := NewLocalCache(10)
	_, err := lc.getInternal(key, true, false)
	require.NoError(t, err)
	require.NotNil(t, lc.plists[string(key)], "list must be cached in plists")

	single, err := lc.GetSinglePosting(key)
	require.NoError(t, err)

	batch, err := lc.GetBatchSinglePosting([][]byte{key})
	require.NoError(t, err)
	require.Len(t, batch, 1)
	require.Equal(t, []byte("from-badger"), singleValue(t, batch[0]))
	require.Equal(t, singleValue(t, single), singleValue(t, batch[0]),
		"plists-served batch result must match the single path")
}

// commitDelValuePosting writes a targeted Del posting (delete of a specific value, not a
// star delete-all) for key at (startTs -> commitTs).
func commitDelValuePosting(t testing.TB, key []byte, value []byte, startTs, commitTs uint64) {
	t.Helper()
	p := &pb.PostingList{
		Postings: []*pb.Posting{{
			Uid:      1,
			Value:    value,
			Op:       Del,
			StartTs:  startTs,
			CommitTs: commitTs,
		}},
	}
	delta, err := proto.Marshal(p)
	require.NoError(t, err)

	txn := Oracle().RegisterStartTs(startTs)
	txn.cache.deltas[string(key)] = delta

	writer := NewTxnWriter(pstore)
	require.NoError(t, txn.CommitToDisk(writer, commitTs))
	require.NoError(t, writer.Flush())
}

// TestGetBatchSinglePostingRepeatedReadsStable guards the A5 hazard: both the single and
// the batch path filter Del postings out of the returned *pb.PostingList IN PLACE, and
// the plists path (List.StaticValue) hands back memory shared with the transaction's
// cached state. This test pins the observable contract that must survive any future
// copy-before-filter fix: with a Del posting in the merged read, repeated reads of the
// same key through the same LocalCache must keep returning identical results, and the
// batch path must keep agreeing with the single path on every read — the first in-place
// filter must not change what later reads observe.
func TestGetBatchSinglePostingRepeatedReadsStable(t *testing.T) {
	attr := x.AttrInRootNamespace("batchStableReads")
	key := x.DataKey(attr, 1)
	commitValuePosting(t, key, []byte("keep"), 61, 62)
	commitDelValuePosting(t, key, []byte("keep"), 63, 64)

	vals := func(pl *pb.PostingList) []string {
		if pl == nil {
			return nil
		}
		var out []string
		for _, p := range pl.Postings {
			out = append(out, fmt.Sprintf("%s/%d", p.Value, p.Op))
		}
		return out
	}

	// Prime a LocalCache so the List lands in lc.plists and reads flow through
	// StaticValue's shared PostingList — the sharing case A5 describes.
	lc := NewLocalCache(70)
	_, err := lc.getInternal(key, true, false)
	require.NoError(t, err)
	require.NotNil(t, lc.plists[string(key)], "list must be cached in plists")

	single1, err := lc.GetSinglePosting(key)
	require.NoError(t, err)
	single2, err := lc.GetSinglePosting(key)
	require.NoError(t, err)
	require.Equal(t, vals(single1), vals(single2),
		"a second single read must observe exactly what the first did")

	batch1, err := lc.GetBatchSinglePosting([][]byte{key})
	require.NoError(t, err)
	require.Len(t, batch1, 1)
	batch2, err := lc.GetBatchSinglePosting([][]byte{key})
	require.NoError(t, err)
	require.Len(t, batch2, 1)

	require.Equal(t, vals(batch1[0]), vals(batch2[0]),
		"a second batch read must observe exactly what the first did")
	require.Equal(t, vals(single1), vals(batch1[0]),
		"batch and single paths must agree in the presence of Del postings")

	// The same key read through a fresh cache must agree too: in-place filtering by the
	// primed cache must not have leaked into what other readers observe.
	fresh, err := NoCache(70).GetSinglePosting(key)
	require.NoError(t, err)
	require.Equal(t, vals(single1), vals(fresh),
		"reads through a fresh cache must match the primed cache's reads")
}
