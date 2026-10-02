/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package posting

import (
	"bytes"
	"context"
	"math"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/dgraph-io/dgraph/v25/protos/pb"
	"github.com/dgraph-io/dgraph/v25/x"
)

// TestBM25DropClearsStats guards against orphaned corpus statistics when the BM25
// index is dropped or rebuilt. BM25 term postings live under the IdentBM25 token
// prefix, but the stats buckets live under a separate reserved token prefix. The
// drop/rebuild machinery deletes by token prefix, so unless the stats prefix is
// also returned by prefixesToDeleteTokensFor, the stats survive a drop and then
// double-count when the index is rebuilt on top of them.
func TestBM25DropClearsStats(t *testing.T) {
	attr := x.AttrInRootNamespace("bm25dropstats")
	prefixes, err := prefixesToDeleteTokensFor(attr, "bm25", false)
	require.NoError(t, err)

	// Every stats bucket key must be covered by one of the deletion prefixes.
	for bucket := 0; bucket < 3; bucket++ {
		statsKey := x.BM25StatsKey(attr, bucket)
		covered := false
		for _, p := range prefixes {
			if bytes.HasPrefix(statsKey, p) {
				covered = true
				break
			}
		}
		require.Truef(t, covered,
			"stats bucket %d key not covered by any bm25 deletion prefix (orphaned on drop)", bucket)
	}
}

func TestBM25ValueCodecRoundTrip(t *testing.T) {
	cases := [][2]uint32{{0, 0}, {1, 1}, {3, 12}, {7, 200}, {65535, 1 << 20}, {1 << 24, 1 << 24}}
	for _, c := range cases {
		tf, dl, ok := decodeBM25Value(encodeBM25Value(c[0], c[1]))
		require.True(t, ok)
		require.Equal(t, c[0], tf)
		require.Equal(t, c[1], dl)
	}
	// Malformed/truncated input is reported as invalid so callers can skip it.
	_, _, ok := decodeBM25Value(nil)
	require.False(t, ok)
	_, _, ok = decodeBM25Value([]byte{0x80}) // varint continuation byte with no terminator
	require.False(t, ok)
}

// TestBM25ValueSurvivesRollup verifies the linchpin of the BM25 redesign: a REF
// index posting that carries a packed (tf, docLen) value is retained — value and
// all — through rollup, instead of being collapsed to a UID-only Pack entry (the
// default behavior for REF postings before the len(p.Value) > 0 retention clause
// in List.encode).
func TestBM25ValueSurvivesRollup(t *testing.T) {
	attr := x.AttrInRootNamespace("bm25rollup")
	encodedTerm := string([]byte{0x10}) + "fox" // IdentBM25 || term
	key := x.BM25IndexKey(attr, encodedTerm)

	docs := []struct {
		uid    uint64
		tf     uint32
		docLen uint32
	}{
		{uid: 5, tf: 3, docLen: 12},
		{uid: 9, tf: 1, docLen: 40},
		{uid: 100, tf: 7, docLen: 200},
	}

	ts := uint64(1)
	for _, d := range docs {
		l, err := GetNoStore(key, ts)
		require.NoError(t, err)
		edge := &pb.DirectedEdge{
			ValueId:   d.uid,
			Attr:      attr,
			Value:     encodeBM25Value(d.tf, d.docLen),
			ValueType: pb.Posting_BINARY,
		}
		addMutation(t, l, edge, Set, ts, ts+1, false)
		ts += 2
	}

	// Force a rollup and decode the resulting posting list directly.
	l, err := getNew(key, pstore, math.MaxUint64, false)
	require.NoError(t, err)
	kvs, err := l.Rollup(nil, math.MaxUint64)
	require.NoError(t, err)
	require.NotEmpty(t, kvs)

	var plist pb.PostingList
	require.NoError(t, proto.Unmarshal(kvs[0].Value, &plist))

	got := make(map[uint64][2]uint32)
	for _, p := range plist.Postings {
		tf, docLen, ok := decodeBM25Value(p.Value)
		require.True(t, ok)
		got[p.Uid] = [2]uint32{tf, docLen}
	}
	for _, d := range docs {
		v, ok := got[d.uid]
		require.Truef(t, ok, "uid %d posting missing after rollup (value stripped?)", d.uid)
		require.Equal(t, d.tf, v[0], "tf for uid %d", d.uid)
		require.Equal(t, d.docLen, v[1], "docLen for uid %d", d.uid)
	}

	// Reading the list back materializes the same (uid, tf, docLen) triples.
	posts, err := ReadBM25TermPostings(func(k []byte) (*List, error) {
		return getNew(k, pstore, math.MaxUint64, false)
	}, attr, encodedTerm, math.MaxUint64)
	require.NoError(t, err)
	require.Len(t, posts, len(docs))
	for _, p := range posts {
		require.Equal(t, got[p.Uid][0], p.TF)
		require.Equal(t, got[p.Uid][1], p.DocLen)
	}
}

// TestBM25StatsConflictKeyValueIndependent guards the invariant that makes the
// corpus-stats read-modify-write counter safe under concurrent live transactions:
// two overlapping transactions that read the same bucket base and write DIFFERENT
// resulting totals must still be detected as conflicting (so one retries instead of
// silently losing an update). This holds because on a scalar (non-list) predicate
// fingerprintEdge returns MaxUint64 for every untagged value, so all stats writes to
// a bucket share the conflict key getKey(statsKey, MaxUint64) — independent of the
// value bytes. It is precisely why @index(bm25) is rejected on list predicates, where
// fingerprintEdge would become value-dependent and let differing totals slip past
// conflict detection.
func TestBM25StatsConflictKeyValueIndependent(t *testing.T) {
	attr := x.AttrInRootNamespace("bm25statsconflict")
	key := x.BM25StatsKey(attr, 3)
	pk, err := x.Parse(key)
	require.NoError(t, err)

	// Mirror addMutationInternal: a value posting (no ValueId) gets its ValueId set
	// from fingerprintEdge before the conflict key is computed.
	mkEdge := func(val []byte) *pb.DirectedEdge {
		e := &pb.DirectedEdge{Attr: attr, Value: val, ValueType: pb.Posting_BINARY, Op: pb.DirectedEdge_SET}
		if NewPosting(e).PostingType != pb.Posting_REF {
			e.ValueId = fingerprintEdge(e)
		}
		return e
	}
	e1 := mkEdge(encodeBM25Stats(10, 100))
	e2 := mkEdge(encodeBM25Stats(11, 137)) // different totals
	require.Equal(t, uint64(math.MaxUint64), e1.ValueId,
		"scalar untagged stats values must carry the MaxUint64 sentinel ValueId")
	require.Equal(t, e1.ValueId, e2.ValueId, "stats ValueId must not depend on the value bytes")

	ck1 := GetConflictKey(pk, key, e1)
	ck2 := GetConflictKey(pk, key, e2)
	require.NotZero(t, ck1, "stats writes must register a conflict key")
	require.Equal(t, ck1, ck2,
		"two writers to the same stats bucket must share a conflict key regardless of the value")
}

// TestBM25StatsRebuildAccumulator covers the fix for stats undercounting during index
// rebuild. The streaming rebuild processes documents across many goroutines, each with
// its own transaction/cache that is periodically reset — so a per-transaction
// read-modify-write counter loses updates (the last writer's partial total wins on
// merge). Routing rebuild stats through a shared accumulator and flushing the buckets
// once (single writer) must reproduce the exact corpus totals. This test models the
// rebuild by feeding documents (several sharing a bucket) through SEPARATE
// transactions that all share one accumulator, then flushing and reading back.
func TestBM25StatsRebuildAccumulator(t *testing.T) {
	ctx := context.Background()
	attr := x.AttrInRootNamespace("bm25rebuildacc")
	acc := newBM25StatsAccum()

	docs := []struct {
		uid uint64
		dl  int64
	}{{1, 10}, {2, 20}, {33, 5}, {65, 7}, {3, 8}, {35, 4}, {97, 9}, {4, 6}}
	var wantCount, wantTerms int64
	for _, d := range docs {
		txn := NewTxn(900)
		txn.cache = NewLocalCache(900)
		txn.bm25Acc = acc
		require.NoError(t, txn.updateBM25Stats(ctx, attr, d.uid, 1, d.dl))
		wantCount++
		wantTerms += d.dl
	}

	// Single-writer finalize: flush the accumulator through one transaction and commit.
	txn := Oracle().RegisterStartTs(901)
	txn.cache = NewLocalCache(901)
	require.NoError(t, acc.flush(ctx, txn, attr))
	txn.Update()
	txn.UpdateCachedKeys(902)
	writer := NewTxnWriter(pstore)
	require.NoError(t, txn.CommitToDisk(writer, 902))
	require.NoError(t, writer.Flush())

	get := func(k []byte) (*List, error) { return GetNoStore(k, 903) }
	dc, tt, err := ReadBM25Stats(get, attr, 903)
	require.NoError(t, err)
	require.Equal(t, uint64(wantCount), dc, "rebuilt doc count must equal the total across all transactions")
	require.Equal(t, uint64(wantTerms), tt, "rebuilt total terms must equal the total across all transactions")
}

// TestBM25StatsBucketed verifies that bucketed corpus statistics accumulate
// correctly across documents (including two documents that hash to the same
// bucket, exercising in-transaction read-your-own-writes) and that deletes
// subtract correctly.
func TestBM25StatsBucketed(t *testing.T) {
	ctx := context.Background()
	attr := x.AttrInRootNamespace("bm25stats")
	ts := uint64(101)
	txn := Oracle().RegisterStartTs(ts)

	// uid 1 and uid 33 both fall in bucket 1 (mod 32), exercising same-bucket
	// accumulation within a single transaction.
	docs := []struct {
		uid uint64
		dl  int64
	}{{1, 10}, {2, 20}, {33, 5}, {64, 7}, {100, 8}}

	var wantCount, wantTerms int64
	for _, d := range docs {
		require.NoError(t, txn.updateBM25Stats(ctx, attr, d.uid, 1, d.dl))
		wantCount++
		wantTerms += d.dl
	}

	get := func(k []byte) (*List, error) { return txn.cache.GetFromDelta(k) }
	dc, tt, err := ReadBM25Stats(get, attr, ts)
	require.NoError(t, err)
	require.Equal(t, uint64(wantCount), dc)
	require.Equal(t, uint64(wantTerms), tt)

	// Delete uid 2: docCount and totalTerms drop accordingly.
	require.NoError(t, txn.updateBM25Stats(ctx, attr, 2, -1, -20))
	dc, tt, err = ReadBM25Stats(get, attr, ts)
	require.NoError(t, err)
	require.Equal(t, uint64(wantCount-1), dc)
	require.Equal(t, uint64(wantTerms-20), tt)
}

// TestBM25StatsAccumulateAcrossTxns verifies that stats accumulate across
// separately-committed transactions (not just within one). This guards against
// the read-modify-write reading only the in-memory delta instead of committed
// disk state, which would make each transaction overwrite its bucket and collapse
// the corpus document count.
func TestBM25StatsAccumulateAcrossTxns(t *testing.T) {
	ctx := context.Background()
	attr := x.AttrInRootNamespace("bm25statsxtxn")

	// Two documents in the SAME bucket (uid 5 and uid 37 → bucket 5), committed in
	// two separate transactions.
	commitDoc := func(startTs, commitTs, uid uint64, docLen int64) {
		txn := Oracle().RegisterStartTs(startTs)
		txn.cache = NewLocalCache(startTs)
		require.NoError(t, txn.updateBM25Stats(ctx, attr, uid, 1, docLen))
		txn.Update()
		txn.UpdateCachedKeys(commitTs)
		writer := NewTxnWriter(pstore)
		require.NoError(t, txn.CommitToDisk(writer, commitTs))
		require.NoError(t, writer.Flush())
	}

	commitDoc(201, 202, 5, 10)
	commitDoc(203, 204, 37, 6)

	// A fresh reader at a later ts must see BOTH documents (count 2, terms 16),
	// not just the most recently committed one.
	get := func(k []byte) (*List, error) { return GetNoStore(k, 205) }
	dc, tt, err := ReadBM25Stats(get, attr, 205)
	require.NoError(t, err)
	require.Equal(t, uint64(2), dc, "doc count must accumulate across transactions")
	require.Equal(t, uint64(16), tt, "total terms must accumulate across transactions")
}
