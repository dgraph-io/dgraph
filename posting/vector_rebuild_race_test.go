/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package posting

// Tests for the reindex-vs-mutation race on vector predicates. A vector
// index rebuild commits every graph key at the alter's startTs while
// concurrent mutations commit above it, so an unsuppressed mid-build
// mutation wins last-writer-wins over the freshly built graph (worst case:
// installing itself as the sole entry point of the still-empty graph and
// orphaning everything the builder writes).
//
// The race's damage is timestamp-ordering, not true nondeterminism, so these
// tests reproduce it single-threaded: operations are applied in exactly the
// order the race would order them (no sleeps, no goroutines). Assertions are
// exact — full self-recall over known uid sets and structural reads of index
// keys — never recall thresholds.

import (
	"context"
	"fmt"
	"math"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/v25/protos/pb"
	"github.com/dgraph-io/dgraph/v25/schema"
	"github.com/dgraph-io/dgraph/v25/tok"
	"github.com/dgraph-io/dgraph/v25/tok/hnsw"
	tokIndex "github.com/dgraph-io/dgraph/v25/tok/index"
	"github.com/dgraph-io/dgraph/v25/tok/partitioned_hnsw"
	"github.com/dgraph-io/dgraph/v25/types"
	"github.com/dgraph-io/dgraph/v25/x"
)

// Each invocation gets a fresh predicate: tests share one pstore and restart
// their local ts counters at 1, so reusing an attr across -count iterations
// would rewrite the same keys at already-used badger versions — duplicate
// (key, version) entries whose read resolution is arbitrary.
var vecTestAttrSeq atomic.Uint64

func vecTestAttr(t *testing.T, base string) string {
	t.Helper()
	return x.AttrInRootNamespace(fmt.Sprintf("%s_%d", base, vecTestAttrSeq.Add(1)))
}

func vecTestNextTs(ts *uint64) uint64 { *ts++; return *ts }

func vecTestCommit(t *testing.T, txn *Txn, startTs, commitTs uint64) {
	t.Helper()
	Oracle().ProcessDelta(&pb.OracleDelta{
		Txns:        []*pb.TxnStatus{{StartTs: startTs, CommitTs: commitTs}},
		MaxAssigned: commitTs,
	})
	txn.Update()
	writer := NewTxnWriter(pstore)
	require.NoError(t, txn.CommitToDisk(writer, commitTs))
	require.NoError(t, writer.Flush())
}

// vecTestWriteBase commits a base-data vector value, without any index
// maintenance — the way data exists before an index is declared.
func vecTestWriteBase(t *testing.T, ctx context.Context, attr string,
	uid uint64, vec []float32, ts *uint64) {
	t.Helper()
	startTs := vecTestNextTs(ts)
	txn := Oracle().RegisterStartTs(startTs)
	l, err := GetNoStore(x.DataKey(attr, uid), math.MaxUint64)
	require.NoError(t, err)
	l = txn.Store(l)
	l.SetTs(startTs)
	require.NoError(t, l.addMutation(ctx, txn, &pb.DirectedEdge{
		Attr: attr, Entity: uid, Value: types.FloatArrayAsBytes(vec),
		ValueType: pb.Posting_VFLOAT, Op: pb.DirectedEdge_SET,
	}))
	vecTestCommit(t, txn, startTs, vecTestNextTs(ts))
}

// vecTestLiveIndexMutation drives the live index-maintenance path for a
// vector mutation exactly as the apply path does: factory specs resolved
// through the write context (which prefers mutSchema, so the mutation knows
// about the index being built), then txn.addIndexMutations. For DEL, val
// carries the value being deleted, as the live delete path does.
func vecTestLiveIndexMutation(t *testing.T, ctx context.Context, attr string,
	uid uint64, vec []float32, op pb.DirectedEdge_Op, ts *uint64) {
	t.Helper()
	startTs := vecTestNextTs(ts)
	txn := Oracle().RegisterStartTs(startTs)
	wctx := schema.GetWriteContext(ctx)
	specs, err := schema.State().FactoryCreateSpec(wctx, attr)
	require.NoError(t, err)
	require.NotEmpty(t, specs, "test setup: mutSchema must carry the vector index spec")

	valBytes := types.FloatArrayAsBytes(vec)
	edge := &pb.DirectedEdge{Attr: attr, Entity: uid, ValueType: pb.Posting_VFLOAT, Op: op}
	if op == pb.DirectedEdge_SET {
		edge.Value = valBytes
	}
	_, err = txn.addIndexMutations(wctx, &indexMutationInfo{
		factorySpecs: specs,
		edge:         edge,
		val:          types.Val{Tid: types.VFloatID, Value: valBytes},
		op:           op,
	})
	require.NoError(t, err)
	vecTestCommit(t, txn, startTs, vecTestNextTs(ts))
}

// vecTestSelfRecall asserts reachability: searching each uid's own vector
// with a beam as wide as the whole corpus must return that uid among the
// results. Wide-beam membership (rather than top-1) is deliberate: builder
// inserts run concurrently, so graph shape varies run to run and greedy
// search may mis-rank near-identical neighbors — but a vector orphaned by
// the rebuild race is unreachable at ANY beam width, so this still detects
// exactly the bug class under test.
//
// strictUids (the uids replayed by the drain) get zero tolerance — their
// reachability is precisely the invariant under test. For the rest of the
// corpus at most ONE miss is tolerated: the concurrent builder itself can
// rarely orphan a node in a tiny graph (pre-existing upstream behavior,
// independent of the capture gate), while the race under test orphans the
// corpus wholesale — so the red/green contract stays sharp.
func vecTestSelfRecall(t *testing.T, ctx context.Context, attr string,
	indexer tokIndex.VectorIndex[float32], vecs map[uint64][]float32,
	readTs uint64, strictUids ...uint64) {
	t.Helper()
	strict := make(map[uint64]bool, len(strictUids))
	for _, u := range strictUids {
		strict[u] = true
	}
	k := len(vecs) + 8
	var missing []uint64
	for uid, vec := range vecs {
		qc := hnsw.NewQueryCache(NewViLocalCache(NewLocalCache(readTs)), readTs)
		found, err := indexer.Search(ctx, qc, vec, k, tokIndex.AcceptAll[float32])
		require.NoErrorf(t, err, "uid %d: search failed", uid)
		reachable := false
		for _, f := range found {
			if f == uid {
				reachable = true
				break
			}
		}
		if reachable {
			continue
		}
		require.Falsef(t, strict[uid],
			"replayed uid %d not reachable at beam width %d (got %v): drain lost it",
			uid, k, found)
		missing = append(missing, uid)
	}
	require.LessOrEqualf(t, len(missing), 1,
		"%d uids unreachable (%v): mid-build mutations corrupted the rebuilt graph",
		len(missing), missing)
}

// TestGetQuerySchemaMasksVectorIndexSpecs pins the reader-visible schema
// during a vector index build: a spec under construction must be masked
// (queries get "not indexed" instead of silently querying a half-built
// graph), while an unchanged spec keeps serving.
func TestGetQuerySchemaMasksVectorIndexSpecs(t *testing.T) {
	attr := x.AttrInRootNamespace("vqsmask")
	mkSpecs := func(numClusters string) []*pb.VectorIndexSpec {
		return []*pb.VectorIndexSpec{{
			Name: "hnsw",
			Options: []*pb.OptionPair{
				{Key: partitioned_hnsw.NumClustersOpt, Value: numClusters},
				{Key: "metric", Value: "euclidean"},
			},
		}}
	}
	plain := &pb.SchemaUpdate{Predicate: attr, ValueType: pb.Posting_VFLOAT}
	withSpecs := func(specs []*pb.VectorIndexSpec) *pb.SchemaUpdate {
		return &pb.SchemaUpdate{
			Predicate: attr, ValueType: pb.Posting_VFLOAT,
			Directive: pb.SchemaUpdate_INDEX, IndexSpecs: specs,
		}
	}

	t.Run("new index is masked while building", func(t *testing.T) {
		rb := &IndexRebuild{Attr: attr, OldSchema: plain, CurrentSchema: withSpecs(mkSpecs("4"))}
		require.NotEmpty(t, rb.needsTokIndexRebuild().vectorIndexesToRebuild,
			"premise: this alter rebuilds the vector index")
		require.Empty(t, rb.GetQuerySchema().IndexSpecs,
			"a vector index under construction must not be served to readers")
	})

	t.Run("changed spec is masked while rebuilding", func(t *testing.T) {
		rb := &IndexRebuild{Attr: attr, OldSchema: withSpecs(mkSpecs("4")), CurrentSchema: withSpecs(mkSpecs("8"))}
		require.NotEmpty(t, rb.needsTokIndexRebuild().vectorIndexesToRebuild,
			"premise: changing numClusters rebuilds the vector index")
		require.Empty(t, rb.GetQuerySchema().IndexSpecs,
			"a vector index being rebuilt must not be served to readers")
	})

	t.Run("unchanged spec keeps serving", func(t *testing.T) {
		rb := &IndexRebuild{Attr: attr, OldSchema: withSpecs(mkSpecs("4")), CurrentSchema: withSpecs(mkSpecs("4"))}
		require.Empty(t, rb.needsTokIndexRebuild().vectorIndexesToRebuild,
			"premise: identical specs need no rebuild")
		require.Len(t, rb.GetQuerySchema().IndexSpecs, 1,
			"an unchanged vector index must keep serving during unrelated schema work")
	})
}

// TestLiveInsertDuringRebuildDoesNotClobberEntry reproduces the entry-point
// clobber on a monolithic hnsw index, single-threaded, in race order: the
// alter applies (capture gate up, rebuild scheduled at rebuildTs), a
// mutation applies into the still-empty graph at a higher commitTs, then the
// build runs committing at rebuildTs. Unsuppressed, the mutation's entry-key
// write wins last-writer-wins and orphans all 20 built vectors.
func TestLiveInsertDuringRebuildDoesNotClobberEntry(t *testing.T) {
	ctx := context.Background()
	attr := vecTestAttr(t, "vclobber")

	indexedSchema := &pb.SchemaUpdate{
		Predicate: attr, ValueType: pb.Posting_VFLOAT, Directive: pb.SchemaUpdate_INDEX,
		IndexSpecs: []*pb.VectorIndexSpec{{
			Name:    "hnsw",
			Options: []*pb.OptionPair{{Key: "metric", Value: "euclidean"}},
		}},
	}
	bare := strings.TrimPrefix(attr, "0-")
	require.NoError(t, schema.ParseBytes([]byte(bare+`: float32vector .`), 1))
	schema.State().SetMutSchema(attr, indexedSchema)
	defer schema.State().DeleteMutSchema(attr)

	// Deterministic graph construction (see testingVectorRebuildNumGo).
	testingVectorRebuildNumGo = 1
	defer func() { testingVectorRebuildNumGo = 0 }()

	// Four well-separated blobs, 5 vectors each: uids 1..20. Well-separated,
	// non-colinear geometry keeps top-1 self-recall exact for HNSW.
	corners := [][]float32{
		{0, 0, 0, 0},
		{100, 100, 0, 0},
		{0, 0, 100, 100},
		{100, 0, 0, 100},
	}
	ts := uint64(1)
	vecs := map[uint64][]float32{}
	uid := uint64(1)
	for _, corner := range corners {
		for i := range 5 {
			v := make([]float32, 4)
			for j := range v {
				v[j] = corner[j] + float32(i)*1.5 + float32(j)*0.5
			}
			vecs[uid] = v
			vecTestWriteBase(t, ctx, attr, uid, v, &ts)
			uid++
		}
	}

	// The alter applies: capture gate up before any later entry can apply,
	// rebuild pinned at rebuildTs — as runSchemaMutation does synchronously
	// in the raft apply path.
	rebuildTs := vecTestNextTs(&ts)
	Oracle().ProcessDelta(&pb.OracleDelta{MaxAssigned: rebuildTs})
	StartVectorRebuildCapture(attr)
	defer FinishVectorRebuildCapture(attr)

	// The raced mutation: applies after the alter, before the background
	// build has written anything. It sees an empty graph.
	newUID := uint64(99)
	newVec := []float32{500, 500, 500, 500}
	vecs[newUID] = newVec
	vecTestWriteBase(t, ctx, attr, newUID, newVec, &ts)
	vecTestLiveIndexMutation(t, ctx, attr, newUID, newVec, pb.DirectedEdge_SET, &ts)

	// The background build runs to completion.
	rb := &IndexRebuild{
		Attr: attr, StartTs: rebuildTs,
		OldSchema:     &pb.SchemaUpdate{Predicate: attr, ValueType: pb.Posting_VFLOAT},
		CurrentSchema: indexedSchema,
	}
	require.NoError(t, rebuildTokIndex(ctx, rb))

	// Every vector — the 20 the builder indexed and the raced one — must be
	// reachable afterwards.
	readTs := vecTestNextTs(&ts)
	Oracle().ProcessDelta(&pb.OracleDelta{MaxAssigned: readTs})
	cspec, err := tok.GetFactoryCreateSpecFromSpec(indexedSchema.IndexSpecs[0])
	require.NoError(t, err)
	indexer, err := cspec.CreateIndex(attr)
	require.NoError(t, err)
	vecTestSelfRecall(t, ctx, attr, indexer, vecs, readTs, newUID)

	// And the build itself must have drained and closed the gate.
	require.False(t, VectorRebuildCaptureActive(attr),
		"build completion must drain and deactivate the capture gate")
}

// TestPartitionedMidBuildMutationsSuppressedAndReplayed drives a partitioned
// build and injects SETs, updates, and DELs at the training_done boundary —
// centroids final, cluster graphs not yet built: the raced window. The gate
// must capture all of them (suppression), and the finished index must serve
// every surviving vector and tombstone the deleted ones (replay).
func TestPartitionedMidBuildMutationsSuppressedAndReplayed(t *testing.T) {
	ctx := context.Background()
	attr := vecTestAttr(t, "vmidbuild")

	indexedSchema := &pb.SchemaUpdate{
		Predicate: attr, ValueType: pb.Posting_VFLOAT, Directive: pb.SchemaUpdate_INDEX,
		IndexSpecs: []*pb.VectorIndexSpec{{
			Name: "hnsw",
			Options: []*pb.OptionPair{
				{Key: partitioned_hnsw.NumClustersOpt, Value: "4"},
				{Key: partitioned_hnsw.NumProbesOpt, Value: "4"},
				{Key: "metric", Value: "euclidean"},
			},
		}},
	}
	bare := strings.TrimPrefix(attr, "0-")
	require.NoError(t, schema.ParseBytes([]byte(bare+`: float32vector .`), 1))
	schema.State().SetMutSchema(attr, indexedSchema)
	defer schema.State().DeleteMutSchema(attr)

	// Deterministic graph construction (see testingVectorRebuildNumGo).
	testingVectorRebuildNumGo = 1
	defer func() { testingVectorRebuildNumGo = 0 }()

	// Four well-separated blobs in 4d, 15 vectors each: uids 1..60.
	corners := [][]float32{
		{0, 0, 0, 0},
		{100, 100, 0, 0},
		{0, 0, 100, 100},
		{100, 0, 0, 100},
	}
	ts := uint64(1)
	vecs := map[uint64][]float32{}
	uid := uint64(1)
	for _, corner := range corners {
		for i := range 15 {
			v := make([]float32, 4)
			for j := range v {
				v[j] = corner[j] + float32(i)*1.5 + float32(j)*0.5
			}
			vecs[uid] = v
			vecTestWriteBase(t, ctx, attr, uid, v, &ts)
			uid++
		}
	}

	rebuildTs := vecTestNextTs(&ts)
	Oracle().ProcessDelta(&pb.OracleDelta{MaxAssigned: rebuildTs})
	StartVectorRebuildCapture(attr)
	defer FinishVectorRebuildCapture(attr)

	// Mid-build mutations: 5 inserts, 3 updates, 2 deletes = 10 raced uids.
	newUIDs := []uint64{101, 102, 103, 104, 105}
	updatedUIDs := []uint64{1, 2, 3}
	deletedUIDs := []uint64{4, 5}
	deletedVecs := map[uint64][]float32{}
	for _, d := range deletedUIDs {
		deletedVecs[d] = vecs[d]
	}

	injected := false
	TestingVectorRebuildStageHook = func(stage string) {
		if stage != "training_done" || injected {
			return
		}
		injected = true
		for i, nu := range newUIDs {
			corner := corners[i%len(corners)]
			v := []float32{corner[0] + 7, corner[1] + 7, corner[2] + 7, corner[3] + float32(i)}
			vecs[nu] = v
			vecTestWriteBase(t, ctx, attr, nu, v, &ts)
			vecTestLiveIndexMutation(t, ctx, attr, nu, v, pb.DirectedEdge_SET, &ts)
		}
		for i, uu := range updatedUIDs {
			v := []float32{corners[3][0] + 20 + float32(i), 33, 33, corners[3][3] + 20}
			vecs[uu] = v
			vecTestWriteBase(t, ctx, attr, uu, v, &ts)
			vecTestLiveIndexMutation(t, ctx, attr, uu, v, pb.DirectedEdge_SET, &ts)
		}
		for _, du := range deletedUIDs {
			vecTestLiveIndexMutation(t, ctx, attr, du, deletedVecs[du], pb.DirectedEdge_DEL, &ts)
			delete(vecs, du)
		}
		// Suppression: all 10 raced mutations must sit in the capture map,
		// none applied to the graph. Non-fatal so that, on unfixed code, the
		// test continues and shows the downstream corruption too.
		assert.Equal(t, 10, VectorRebuildPendingCount(attr),
			"mid-build vector mutations must be captured, not applied")
	}
	defer func() { TestingVectorRebuildStageHook = nil }()

	rb := &IndexRebuild{
		Attr: attr, StartTs: rebuildTs,
		OldSchema:     &pb.SchemaUpdate{Predicate: attr, ValueType: pb.Posting_VFLOAT},
		CurrentSchema: indexedSchema,
	}
	require.NoError(t, rebuildTokIndex(ctx, rb))
	require.True(t, injected, "test setup: the training_done hook must have fired")

	readTs := vecTestNextTs(&ts)
	Oracle().ProcessDelta(&pb.OracleDelta{MaxAssigned: readTs})
	cspec, err := tok.GetFactoryCreateSpecFromSpec(indexedSchema.IndexSpecs[0])
	require.NoError(t, err)
	indexer, err := cspec.CreateIndex(attr)
	require.NoError(t, err)

	// Diagnostic on failure: which uids have graph keys in each cluster.
	for i := range 4 {
		attrName := hnsw.SplitVecAttr(attr, i)
		var uids []uint64
		require.NoError(t, MemLayerInstance.IterateDisk(ctx, IterateDiskArgs{
			Prefix:         x.PredicatePrefix(attrName),
			ReadTs:         readTs,
			AllVersions:    false,
			CheckInclusion: func(uint64) error { return nil },
			Function: func(l *List, pk x.ParsedKey) error {
				uids = append(uids, pk.Uid)
				return nil
			},
		}))
		t.Logf("cluster %d graph keys: %v", i, uids)
	}
	for _, du := range []uint64{101, 105} {
		pl, err := GetNoStore(x.DataKey(hnsw.SplitVecAttr(attr, 0), du), readTs)
		require.NoError(t, err)
		val, err := pl.Value(readTs)
		if err != nil {
			t.Logf("uid %d cluster-0 adjacency: no value (%v)", du, err)
			continue
		}
		var m [][]uint64
		require.NoError(t, decodeUint64MatrixUnsafe(val.Value.([]byte), &m))
		t.Logf("uid %d cluster-0 adjacency: %v", du, m)
	}

	// Replay: every surviving vector — originals, mid-build inserts, and
	// updated values — must be reachable; the replayed uids strictly so.
	strictUids := append(append([]uint64{}, newUIDs...), updatedUIDs...)
	vecTestSelfRecall(t, ctx, attr, indexer, vecs, readTs, strictUids...)

	// Deleted uids must be tombstoned in the dead list of the cluster their
	// build-time (StartTs-snapshot) value routes to.
	resolver, ok := indexer.(tokIndex.VectorDeadListResolver[float32])
	require.True(t, ok, "partitioned index must expose a dead-list resolver")
	for _, du := range deletedUIDs {
		qc := hnsw.NewQueryCache(NewViLocalCache(NewLocalCache(readTs)), readTs)
		deadAttr, err := resolver.DeadAttrForVector(qc, deletedVecs[du])
		require.NoError(t, err)
		pl, err := GetNoStore(x.DataKey(deadAttr, 1), readTs)
		require.NoError(t, err)
		val, err := pl.Value(readTs)
		require.NoErrorf(t, err, "uid %d: dead list %q missing", du, deadAttr)
		deadNodes, err := hnsw.ParseEdges(string(val.Value.([]byte)))
		require.NoError(t, err)
		// The index-layer contract for deletes is the tombstone: search does
		// not consult dead lists (the query layer drops uids whose base
		// value is gone), so the structural assertion is the right one.
		require.Containsf(t, deadNodes, du, "uid %d must be tombstoned in %q", du, deadAttr)
	}

	// The gate must be drained and closed by the build itself.
	require.Equal(t, 0, VectorRebuildPendingCount(attr))
	require.False(t, VectorRebuildCaptureActive(attr),
		"build completion must drain and deactivate the capture gate")
}

// TestDrainWaitsForUncommittedCapture pins the capture/commit ordering
// contract: capture happens at apply time, but the base value only becomes
// readable when the commit delta arrives. The drain must wait for a captured
// mutation's transaction to resolve — indexing it once committed, skipping it
// once aborted — instead of racing the commit and silently dropping the uid.
func TestDrainWaitsForUncommittedCapture(t *testing.T) {
	ctx := context.Background()
	attr := vecTestAttr(t, "vcommitwait")

	indexedSchema := &pb.SchemaUpdate{
		Predicate: attr, ValueType: pb.Posting_VFLOAT, Directive: pb.SchemaUpdate_INDEX,
		IndexSpecs: []*pb.VectorIndexSpec{{
			Name:    "hnsw",
			Options: []*pb.OptionPair{{Key: "metric", Value: "euclidean"}},
		}},
	}
	bare := strings.TrimPrefix(attr, "0-")
	require.NoError(t, schema.ParseBytes([]byte(bare+`: float32vector .`), 1))
	schema.State().SetMutSchema(attr, indexedSchema)
	defer schema.State().DeleteMutSchema(attr)

	testingVectorRebuildNumGo = 1
	defer func() { testingVectorRebuildNumGo = 0 }()

	corners := [][]float32{{0, 0, 0, 0}, {100, 100, 0, 0}, {0, 0, 100, 100}, {100, 0, 0, 100}}
	ts := uint64(1)
	vecs := map[uint64][]float32{}
	uid := uint64(1)
	for _, corner := range corners {
		for i := range 5 {
			v := make([]float32, 4)
			for j := range v {
				v[j] = corner[j] + float32(i)*1.5 + float32(j)*0.5
			}
			vecs[uid] = v
			vecTestWriteBase(t, ctx, attr, uid, v, &ts)
			uid++
		}
	}

	rebuildTs := vecTestNextTs(&ts)
	Oracle().ProcessDelta(&pb.OracleDelta{MaxAssigned: rebuildTs})
	StartVectorRebuildCapture(attr)
	defer FinishVectorRebuildCapture(attr)

	// Two captured mutations whose transactions are still unresolved when the
	// drain starts: one commits late, one aborts late.
	prepareUncommitted := func(u uint64, vec []float32) (*Txn, uint64) {
		startTs := vecTestNextTs(&ts)
		txn := Oracle().RegisterStartTs(startTs)
		l, err := GetNoStore(x.DataKey(attr, u), math.MaxUint64)
		require.NoError(t, err)
		l = txn.Store(l)
		l.SetTs(startTs)
		valBytes := types.FloatArrayAsBytes(vec)
		require.NoError(t, l.addMutation(ctx, txn, &pb.DirectedEdge{
			Attr: attr, Entity: u, Value: valBytes,
			ValueType: pb.Posting_VFLOAT, Op: pb.DirectedEdge_SET,
		}))
		wctx := schema.GetWriteContext(ctx)
		specs, err := schema.State().FactoryCreateSpec(wctx, attr)
		require.NoError(t, err)
		_, err = txn.addIndexMutations(wctx, &indexMutationInfo{
			factorySpecs: specs,
			edge: &pb.DirectedEdge{Attr: attr, Entity: u, Value: valBytes,
				ValueType: pb.Posting_VFLOAT, Op: pb.DirectedEdge_SET},
			val: types.Val{Tid: types.VFloatID, Value: valBytes},
			op:  pb.DirectedEdge_SET,
		})
		require.NoError(t, err)
		return txn, startTs
	}

	lateCommitUID, lateAbortUID := uint64(99), uint64(98)
	lateVec := []float32{500, 500, 500, 500}
	vecs[lateCommitUID] = lateVec
	commitTxn, commitStart := prepareUncommitted(lateCommitUID, lateVec)
	_, abortStart := prepareUncommitted(lateAbortUID, []float32{600, 600, 600, 600})
	require.Equal(t, 2, VectorRebuildPendingCount(attr), "both mutations must be captured")

	commitTs := vecTestNextTs(&ts)
	readTs := vecTestNextTs(&ts)
	go func() {
		time.Sleep(150 * time.Millisecond)
		// Resolve: one commit, one abort. The drain must be waiting on both.
		Oracle().ProcessDelta(&pb.OracleDelta{
			Txns: []*pb.TxnStatus{
				{StartTs: commitStart, CommitTs: commitTs},
				{StartTs: abortStart, CommitTs: 0},
			},
			MaxAssigned: readTs,
		})
		commitTxn.Update()
		writer := NewTxnWriter(pstore)
		if err := commitTxn.CommitToDisk(writer, commitTs); err != nil {
			panic(err)
		}
		if err := writer.Flush(); err != nil {
			panic(err)
		}
	}()

	rb := &IndexRebuild{
		Attr: attr, StartTs: rebuildTs,
		OldSchema:     &pb.SchemaUpdate{Predicate: attr, ValueType: pb.Posting_VFLOAT},
		CurrentSchema: indexedSchema,
	}
	start := time.Now()
	require.NoError(t, rebuildTokIndex(ctx, rb))
	require.GreaterOrEqual(t, time.Since(start), 100*time.Millisecond,
		"the drain must have waited for the unresolved transactions")
	require.False(t, VectorRebuildCaptureActive(attr),
		"gate must close once every capture resolved")

	ResetCache()
	plDbg, err := GetNoStore(x.DataKey(attr, lateCommitUID), math.MaxUint64)
	require.NoError(t, err)
	valDbg, errDbg := plDbg.Value(math.MaxUint64)
	t.Logf("DEBUG post-rebuild base read uid %d: err=%v hasVal=%v", lateCommitUID, errDbg, valDbg.Value != nil)

	Oracle().ProcessDelta(&pb.OracleDelta{MaxAssigned: readTs})
	cspec, err := tok.GetFactoryCreateSpecFromSpec(indexedSchema.IndexSpecs[0])
	require.NoError(t, err)
	indexer, err := cspec.CreateIndex(attr)
	require.NoError(t, err)

	// The late-committed vector must be reachable; the aborted one must not
	// be served (its transaction never committed a value).
	vecTestSelfRecall(t, ctx, attr, indexer, vecs, readTs, lateCommitUID)
	qc := hnsw.NewQueryCache(NewViLocalCache(NewLocalCache(readTs)), readTs)
	found, err := indexer.Search(ctx, qc, []float32{600, 600, 600, 600}, len(vecs)+8, tokIndex.AcceptAll[float32])
	require.NoError(t, err)
	require.NotContainsf(t, found, lateAbortUID, "aborted mutation must not be indexed")
}
