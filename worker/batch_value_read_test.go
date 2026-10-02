/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package worker

// Integration tests for the batched value-posting read path.
//
// handleValuePostings fetches scalar values through
// LocalCache.NewBatchedSinglePostingIterator -> GetBatchSinglePosting -> badger
// Txn.GetBatch. These tests drive that whole stack through the real query
// entrypoint (queryState.helpProcessTask, which is what processTask calls after
// its cluster-membership checks) against a real managed badger instance, and
// pin one invariant: for every uid position, the batched path must return
// exactly what the serial GetSinglePosting path returns, regardless of chunk
// boundaries (batches of 10), DivideAndRule goroutine fan-out, holes, deletes,
// MVCC versions, rollups, uncommitted transaction state, or concurrent writers.

import (
	"context"
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/badger/v4"
	bpb "github.com/dgraph-io/badger/v4/pb"
	"github.com/dgraph-io/dgraph/v25/posting"
	"github.com/dgraph-io/dgraph/v25/protos/pb"
	"github.com/dgraph-io/dgraph/v25/schema"
	"github.com/dgraph-io/dgraph/v25/x"
)

// bvSeq hands out globally unique timestamps across all tests in this file.
var bvSeq uint64

func bvNextTs() uint64 {
	return atomic.AddUint64(&bvSeq, 1)
}

func bvSetup(t *testing.T, schemaStr string) {
	prevStore := pstore
	dir := t.TempDir()
	opt := badger.DefaultOptions(dir)
	ps, err := badger.OpenManaged(opt)
	require.NoError(t, err)
	pstore = ps
	posting.Init(ps, 0, false)
	require.NoError(t, schema.ParseBytes([]byte(schemaStr), 1))
	// pstore is shared by every test in this package; hand the original store
	// back before the temp one is closed, or later tests write to a closed DB.
	t.Cleanup(func() {
		require.NoError(t, ps.Close())
		pstore = prevStore
		posting.Init(prevStore, 0, false)
	})
}

// bvApply commits a single edge mutation through the same path a real mutation
// takes: oracle-registered txn, list mutation, oracle delta, delta to badger.
func bvApply(t testing.TB, edge *pb.DirectedEdge) uint64 {
	startTs := bvNextTs()
	txn := posting.Oracle().RegisterStartTs(startTs)
	l, err := posting.GetNoStore(x.DataKey(edge.Attr, edge.Entity), math.MaxUint64)
	require.NoError(t, err)
	l = txn.Store(l)
	l.SetTs(startTs)
	require.NoError(t, l.AddMutationWithIndex(context.Background(), edge, txn))

	commit := bvNextTs()
	od := &pb.OracleDelta{MaxAssigned: atomic.LoadUint64(&bvSeq)}
	od.Txns = append(od.Txns, &pb.TxnStatus{StartTs: startTs, CommitTs: commit})
	posting.Oracle().ProcessDelta(od)

	txn.Update()
	writer := posting.NewTxnWriter(pstore)
	require.NoError(t, txn.CommitToDisk(writer, commit))
	require.NoError(t, writer.Flush())
	return commit
}

func bvSetVal(t testing.TB, attr string, uid uint64, val string) uint64 {
	return bvApply(t, &pb.DirectedEdge{Attr: attr, Entity: uid, Value: []byte(val), Op: pb.DirectedEdge_SET})
}

func bvDelVal(t testing.TB, attr string, uid uint64, val string) uint64 {
	return bvApply(t, &pb.DirectedEdge{Attr: attr, Entity: uid, Value: []byte(val), Op: pb.DirectedEdge_DEL})
}

func bvStarDel(t testing.TB, attr string, uid uint64) uint64 {
	return bvApply(t, &pb.DirectedEdge{Attr: attr, Entity: uid, Value: []byte(x.Star), Op: pb.DirectedEdge_DEL})
}

// bvRollup rolls the key up exactly like incrRollupi.rollUpKey does: read all
// versions, produce the complete posting list, write it back at a fresh ts.
func bvRollup(t testing.TB, attr string, uid uint64) {
	key := x.DataKey(attr, uid)
	l, err := posting.GetNoStore(key, math.MaxUint64)
	require.NoError(t, err)
	kvs, err := l.Rollup(nil, bvNextTs())
	require.NoError(t, err)
	writer := posting.NewTxnWriter(pstore)
	require.NoError(t, writer.Write(&bpb.KVList{Kv: kvs}))
	require.NoError(t, writer.Flush())
}

// bvQuery runs a value-postings task query through the real query path with an
// explicit LocalCache (nil means a fresh NoCache, i.e. a plain read-only query).
func bvQuery(t testing.TB, q *pb.Query, lc *posting.LocalCache) (*pb.Result, error) {
	if lc == nil {
		lc = posting.NoCache(q.ReadTs)
	}
	qs := queryState{cache: lc}
	return qs.helpProcessTask(context.Background(), q, 1)
}

func bvValueQuery(attr string, readTs uint64, uids []uint64) *pb.Query {
	return &pb.Query{
		Attr:    attr,
		UidList: &pb.List{Uids: uids},
		ReadTs:  readTs,
	}
}

// bvVals flattens the ValueMatrix into one []string per queried uid position.
func bvVals(t testing.TB, res *pb.Result, n int) [][]string {
	require.Len(t, res.ValueMatrix, n)
	out := make([][]string, n)
	for i, vl := range res.ValueMatrix {
		for _, tv := range vl.Values {
			out[i] = append(out[i], string(tv.Val))
		}
	}
	return out
}

func bvRange(lo, hi uint64) []uint64 {
	uids := make([]uint64, 0, hi-lo+1)
	for uid := lo; uid <= hi; uid++ {
		uids = append(uids, uid)
	}
	return uids
}

// TestBatchValueQueryFanoutAlignment is the core positional-correctness test.
// Every uid gets a distinct value, and queries of many sizes must return each
// uid's own value at its own position. Sizes are chosen to straddle every
// boundary in the pipeline: the 10-key GetBatch chunk (1, 9, 10, 11, 25), the
// 256-wide DivideAndRule goroutine split (255, 256, 257), multi-goroutine
// fan-out (1000 -> 2, 2560 -> 8 goroutines). Any off-by-one in the start+j
// key wiring, chunk cursor, or result stitching shows up as a value attributed
// to the wrong uid.
func TestBatchValueQueryFanoutAlignment(t *testing.T) {
	bvSetup(t, "bvname: string .")
	attr := x.AttrInRootNamespace("bvname")

	const n = 2560
	for uid := uint64(1); uid <= n; uid++ {
		bvSetVal(t, attr, uid, fmt.Sprintf("v%d", uid))
	}
	readTs := bvNextTs()

	for _, size := range []int{1, 9, 10, 11, 25, 255, 256, 257, 1000, 2560} {
		t.Run(fmt.Sprintf("size=%d", size), func(t *testing.T) {
			uids := bvRange(1, uint64(size))
			res, err := bvQuery(t, bvValueQuery(attr, readTs, uids), nil)
			require.NoError(t, err)
			vals := bvVals(t, res, size)
			for i, uid := range uids {
				require.Equalf(t, []string{fmt.Sprintf("v%d", uid)}, vals[i],
					"wrong value at position %d (uid %d)", i, uid)
			}
		})
	}
}

// TestBatchValueQueryHoles interleaves uids that have no value at all (badger
// misses -> nil GetBatch items) with uids that do. Holes land on the first key
// of the query, on chunk boundaries (positions 9/10) and inside goroutine
// splits. A miss must produce an empty slot at exactly its own position and
// must never shift later results — the classic batched-read misalignment bug.
func TestBatchValueQueryHoles(t *testing.T) {
	bvSetup(t, "bvholes: string .")
	attr := x.AttrInRootNamespace("bvholes")

	const n = 600
	hasVal := func(uid uint64) bool { return uid%3 != 1 } // uid 1, 4, 7, 10... missing
	for uid := uint64(1); uid <= n; uid++ {
		if hasVal(uid) {
			bvSetVal(t, attr, uid, fmt.Sprintf("h%d", uid))
		}
	}
	readTs := bvNextTs()

	uids := bvRange(1, n)
	res, err := bvQuery(t, bvValueQuery(attr, readTs, uids), nil)
	require.NoError(t, err)
	vals := bvVals(t, res, n)
	for i, uid := range uids {
		if hasVal(uid) {
			require.Equalf(t, []string{fmt.Sprintf("h%d", uid)}, vals[i], "uid %d", uid)
		} else {
			require.Emptyf(t, vals[i], "uid %d should have no value", uid)
		}
	}
}

// TestBatchValueQueryDeletes covers every delete shape: an explicit DEL of the
// value, a star (delete-all) posting, and a set-after-star-delete. A star
// posting on one uid must wipe only that uid — never abort or blank the whole
// batch — and deleted values must read back as holes, not stale values.
func TestBatchValueQueryDeletes(t *testing.T) {
	bvSetup(t, "bvdel: string .")
	attr := x.AttrInRootNamespace("bvdel")

	const n = 40
	for uid := uint64(1); uid <= n; uid++ {
		bvSetVal(t, attr, uid, fmt.Sprintf("d%d", uid))
	}
	for uid := uint64(1); uid <= n; uid++ {
		switch uid % 4 {
		case 0:
			bvDelVal(t, attr, uid, fmt.Sprintf("d%d", uid))
		case 1:
			bvStarDel(t, attr, uid)
		}
	}
	// One star-deleted uid gets a fresh value on top of the delete.
	bvSetVal(t, attr, 9, "back9")
	readTs := bvNextTs()

	uids := bvRange(1, n)
	res, err := bvQuery(t, bvValueQuery(attr, readTs, uids), nil)
	require.NoError(t, err)
	vals := bvVals(t, res, n)
	for i, uid := range uids {
		switch {
		case uid == 9:
			require.Equal(t, []string{"back9"}, vals[i])
		case uid%4 == 0 || uid%4 == 1:
			require.Emptyf(t, vals[i], "uid %d was deleted", uid)
		default:
			require.Equalf(t, []string{fmt.Sprintf("d%d", uid)}, vals[i], "uid %d", uid)
		}
	}
}

// TestBatchValueQuerySnapshotIsolation pins MVCC correctness of the batched
// read: a query at readTs must see exactly the versions committed at or before
// readTs, even when half the uids in a chunk have newer committed versions.
// This is the dgraph-level guard for the badger-side stale-read class of bugs
// (a batched iterator picking an older or newer version than the snapshot).
func TestBatchValueQuerySnapshotIsolation(t *testing.T) {
	bvSetup(t, "bvsnap: string .")
	attr := x.AttrInRootNamespace("bvsnap")

	const n = 30
	for uid := uint64(1); uid <= n; uid++ {
		bvSetVal(t, attr, uid, fmt.Sprintf("v1-%d", uid))
	}
	ts1 := bvNextTs()

	// Second version for only the first half, then snapshot, then the rest.
	for uid := uint64(1); uid <= n/2; uid++ {
		bvSetVal(t, attr, uid, fmt.Sprintf("v2-%d", uid))
	}
	tsMid := bvNextTs()
	var lastCommit uint64
	for uid := uint64(n/2 + 1); uid <= n; uid++ {
		lastCommit = bvSetVal(t, attr, uid, fmt.Sprintf("v2-%d", uid))
	}
	// Read at EXACTLY the last commitTs, not one past it: versions committed at readTs
	// must be visible (Zero routinely hands out a readTs equal to the latest commit).
	// The badger-side aliasing bug (done/keysRead) made exactly-at-readTs versions read
	// as absent, and every test that always reads at a fresh later ts would miss that.
	ts2 := lastCommit

	uids := bvRange(1, n)
	check := func(readTs uint64, want func(uid uint64) string) {
		res, err := bvQuery(t, bvValueQuery(attr, readTs, uids), nil)
		require.NoError(t, err)
		vals := bvVals(t, res, n)
		for i, uid := range uids {
			require.Equalf(t, []string{want(uid)}, vals[i], "uid %d at readTs %d", uid, readTs)
		}
	}

	check(ts1, func(uid uint64) string { return fmt.Sprintf("v1-%d", uid) })
	check(tsMid, func(uid uint64) string {
		if uid <= n/2 {
			return fmt.Sprintf("v2-%d", uid)
		}
		return fmt.Sprintf("v1-%d", uid)
	})
	check(ts2, func(uid uint64) string { return fmt.Sprintf("v2-%d", uid) })
}

// TestBatchValueQueryReadYourOwnWrites runs the query with a transaction's own
// LocalCache, the way processTask does for UseTxnCache queries. Uids the
// transaction has mutated (uncommitted) must come from the local cache, and
// untouched uids must come from badger — mixed within the same 10-key chunks.
// The original implementation inverted the cache check and never used local
// state; this pins the integration-visible symptom of that bug.
func TestBatchValueQueryReadYourOwnWrites(t *testing.T) {
	bvSetup(t, "bvryow: string .")
	attr := x.AttrInRootNamespace("bvryow")

	const n = 30
	for uid := uint64(1); uid <= n; uid++ {
		bvSetVal(t, attr, uid, fmt.Sprintf("disk%d", uid))
	}

	startTs := bvNextTs()
	txn := posting.Oracle().RegisterStartTs(startTs)
	for uid := uint64(2); uid <= n; uid += 2 {
		edge := &pb.DirectedEdge{
			Attr: attr, Entity: uid, Value: []byte(fmt.Sprintf("txn%d", uid)),
			Op: pb.DirectedEdge_SET,
		}
		l, err := posting.GetNoStore(x.DataKey(attr, uid), math.MaxUint64)
		require.NoError(t, err)
		l = txn.Store(l)
		l.SetTs(startTs)
		require.NoError(t, l.AddMutationWithIndex(context.Background(), edge, txn))
	}

	lc := posting.Oracle().CacheAt(startTs)
	require.NotNil(t, lc)
	uids := bvRange(1, n)
	res, err := bvQuery(t, bvValueQuery(attr, startTs, uids), lc)
	require.NoError(t, err)
	vals := bvVals(t, res, n)
	for i, uid := range uids {
		want := fmt.Sprintf("disk%d", uid)
		if uid%2 == 0 {
			want = fmt.Sprintf("txn%d", uid)
		}
		require.Equalf(t, []string{want}, vals[i], "uid %d", uid)
	}
}

// TestBatchValueQueryAfterRollup exercises the storage shapes rollups create:
// complete posting lists (BitCompletePosting), a fresh delta stacked on top of
// a rolled-up base, and — via delete-then-rollup — BitEmptyPosting entries
// whose badger value is zero bytes. The empty-value case is the sharpest one:
// a found-but-empty item must read as "no value", never be confused with an
// absent key or crash the batch decoder.
func TestBatchValueQueryAfterRollup(t *testing.T) {
	bvSetup(t, "bvroll: string .")
	attr := x.AttrInRootNamespace("bvroll")

	const n = 25
	for round := 1; round <= 3; round++ {
		for uid := uint64(1); uid <= n; uid++ {
			bvSetVal(t, attr, uid, fmt.Sprintf("r%d-%d", uid, round))
		}
	}
	for uid := uint64(1); uid <= n; uid++ {
		bvRollup(t, attr, uid)
	}

	// Delete two uids entirely and roll them up: their latest badger version
	// becomes an empty posting list with a zero-byte value.
	bvStarDel(t, attr, 5)
	bvStarDel(t, attr, 6)
	bvRollup(t, attr, 5)
	bvRollup(t, attr, 6)

	// Stack a new delta on top of a rolled-up key: latest version must win.
	bvSetVal(t, attr, 7, "r7-4")

	readTs := bvNextTs()
	uids := bvRange(1, n)
	res, err := bvQuery(t, bvValueQuery(attr, readTs, uids), nil)
	require.NoError(t, err)
	vals := bvVals(t, res, n)
	for i, uid := range uids {
		switch uid {
		case 5, 6:
			require.Emptyf(t, vals[i], "uid %d was delete-all'd and rolled up", uid)
		case 7:
			require.Equal(t, []string{"r7-4"}, vals[i])
		default:
			require.Equalf(t, []string{fmt.Sprintf("r%d-3", uid)}, vals[i], "uid %d", uid)
		}
	}
}

// TestBatchValueQueryEqFilter drives the compareAttrFn branch: eq() over an
// unindexed predicate with a uid list fetches values through the batched
// iterator and filters uids by comparison. A cursor bug here doesn't just
// return wrong values — it silently includes/excludes the wrong uids.
func TestBatchValueQueryEqFilter(t *testing.T) {
	bvSetup(t, "bveq: string .")
	attr := x.AttrInRootNamespace("bveq")

	const n = 600
	for uid := uint64(1); uid <= n; uid++ {
		val := "no"
		if uid%2 == 1 {
			val = "yes"
		}
		bvSetVal(t, attr, uid, val)
	}
	readTs := bvNextTs()

	q := bvValueQuery(attr, readTs, bvRange(1, n))
	q.SrcFunc = &pb.SrcFunction{Name: "eq", Args: []string{"yes"}}
	res, err := bvQuery(t, q, nil)
	require.NoError(t, err)

	var got []uint64
	for _, l := range res.UidMatrix {
		got = append(got, l.Uids...)
	}
	var want []uint64
	for uid := uint64(1); uid <= n; uid += 2 {
		want = append(want, uid)
	}
	require.Equal(t, want, got)
}

// TestBatchValueQueryDuplicateUids sends uid lists with repeats, including a
// run of one uid longer than the 10-key chunk. Every occurrence must be
// answered independently and identically.
func TestBatchValueQueryDuplicateUids(t *testing.T) {
	bvSetup(t, "bvdup: string .")
	attr := x.AttrInRootNamespace("bvdup")

	bvSetVal(t, attr, 7, "seven")
	bvSetVal(t, attr, 8, "eight")
	readTs := bvNextTs()

	uids := []uint64{7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 8, 7, 8, 8, 7}
	res, err := bvQuery(t, bvValueQuery(attr, readTs, uids), nil)
	require.NoError(t, err)
	vals := bvVals(t, res, len(uids))
	for i, uid := range uids {
		want := "seven"
		if uid == 8 {
			want = "eight"
		}
		require.Equalf(t, []string{want}, vals[i], "position %d", i)
	}
}

// TestBatchValueQueryCorruptValue plants an unparseable delta under one key in
// the middle of a chunk. The query must fail with an error — not panic, and
// not silently return partial or misaligned results.
func TestBatchValueQueryCorruptValue(t *testing.T) {
	bvSetup(t, "bvcorrupt: string .")
	attr := x.AttrInRootNamespace("bvcorrupt")

	const n = 12
	for uid := uint64(1); uid <= n; uid++ {
		bvSetVal(t, attr, uid, fmt.Sprintf("c%d", uid))
	}
	writer := posting.NewTxnWriter(pstore)
	garbage := []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}
	require.NoError(t, writer.SetAt(x.DataKey(attr, 6), garbage, posting.BitDeltaPosting, bvNextTs()))
	require.NoError(t, writer.Flush())
	readTs := bvNextTs()

	_, err := bvQuery(t, bvValueQuery(attr, readTs, bvRange(1, n)), nil)
	require.Error(t, err)
}

// TestBatchValueQueryConcurrentWrites pins snapshot stability under load:
// while writers keep committing new versions at higher timestamps, repeated
// queries at a fixed readTs must keep returning the pinned values. Run with
// -race this also shakes out data races between the batched read path and the
// mutation path.
func TestBatchValueQueryConcurrentWrites(t *testing.T) {
	bvSetup(t, "bvconc: string .")
	attr := x.AttrInRootNamespace("bvconc")

	const n = 64
	for uid := uint64(1); uid <= n; uid++ {
		bvSetVal(t, attr, uid, fmt.Sprintf("s%d", uid))
	}
	readTs := bvNextTs()
	uids := bvRange(1, n)

	var wg sync.WaitGroup
	for g := 0; g < 4; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for k := 0; k < 50; k++ {
				uid := uint64(g*50+k)%n + 1
				bvSetVal(t, attr, uid, fmt.Sprintf("overwrite-%d-%d", g, k))
			}
		}()
	}
	for r := 0; r < 4; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for k := 0; k < 25; k++ {
				res, err := bvQuery(t, bvValueQuery(attr, readTs, uids), nil)
				require.NoError(t, err)
				vals := bvVals(t, res, n)
				for i, uid := range uids {
					require.Equalf(t, []string{fmt.Sprintf("s%d", uid)}, vals[i],
						"uid %d must stay pinned at readTs %d", uid, readTs)
				}
			}
		}()
	}
	wg.Wait()
}

// TestValueQueryLangStaysUnbatched guards the boundary of the optimization:
// @lang predicates take the getMultiplePosting path (batching only reads the
// latest version, which is wrong for language variants), and that path must
// keep returning correct per-language values.
func TestValueQueryLangStaysUnbatched(t *testing.T) {
	bvSetup(t, "bvlang: string @lang .")
	attr := x.AttrInRootNamespace("bvlang")

	const n = 12
	for uid := uint64(1); uid <= n; uid++ {
		bvApply(t, &pb.DirectedEdge{
			Attr: attr, Entity: uid, Value: []byte(fmt.Sprintf("hallo%d", uid)),
			Lang: "de", Op: pb.DirectedEdge_SET,
		})
		bvApply(t, &pb.DirectedEdge{
			Attr: attr, Entity: uid, Value: []byte(fmt.Sprintf("hello%d", uid)),
			Lang: "en", Op: pb.DirectedEdge_SET,
		})
	}
	readTs := bvNextTs()

	q := bvValueQuery(attr, readTs, bvRange(1, n))
	q.Langs = []string{"en"}
	res, err := bvQuery(t, q, nil)
	require.NoError(t, err)
	vals := bvVals(t, res, n)
	for i, uid := range bvRange(1, n) {
		require.Equalf(t, []string{fmt.Sprintf("hello%d", uid)}, vals[i], "uid %d", uid)
	}
}

// TestBatchValueQueryMatchesSerialReads is the differential oracle over a
// mixed corpus: every storage shape at once (missing, single version, many
// deltas, value-delete, star-delete, rollup + fresh delta). For each uid the
// query's answer must equal what a fresh serial GetSinglePosting returns for
// that key. This is the invariant the whole feature rests on, checked
// end-to-end rather than at the LocalCache unit level.
func TestBatchValueQueryMatchesSerialReads(t *testing.T) {
	bvSetup(t, "bvdiff: string .")
	attr := x.AttrInRootNamespace("bvdiff")

	const n = 120
	for uid := uint64(1); uid <= n; uid++ {
		switch uid % 6 {
		case 0: // no value at all
		case 1:
			bvSetVal(t, attr, uid, fmt.Sprintf("one%d", uid))
		case 2: // several deltas
			for r := 0; r < 3; r++ {
				bvSetVal(t, attr, uid, fmt.Sprintf("multi%d-%d", uid, r))
			}
		case 3: // set then delete the value
			bvSetVal(t, attr, uid, fmt.Sprintf("gone%d", uid))
			bvDelVal(t, attr, uid, fmt.Sprintf("gone%d", uid))
		case 4: // set then delete-all
			bvSetVal(t, attr, uid, fmt.Sprintf("wiped%d", uid))
			bvStarDel(t, attr, uid)
		case 5: // rollup with a delta on top
			bvSetVal(t, attr, uid, fmt.Sprintf("base%d", uid))
			bvRollup(t, attr, uid)
			bvSetVal(t, attr, uid, fmt.Sprintf("after%d", uid))
		}
	}
	readTs := bvNextTs()

	uids := bvRange(1, n)
	res, err := bvQuery(t, bvValueQuery(attr, readTs, uids), nil)
	require.NoError(t, err)
	vals := bvVals(t, res, n)

	for i, uid := range uids {
		pl, err := posting.NoCache(readTs).GetSinglePosting(x.DataKey(attr, uid))
		require.NoError(t, err)
		var want []string
		if pl != nil {
			for _, p := range pl.Postings {
				want = append(want, string(p.Value))
			}
		}
		require.Equalf(t, want, vals[i],
			"batched query and serial GetSinglePosting disagree for uid %d", uid)
	}
}
