/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package posting

// Diagnostic isolation matrix for the batched-mutation orphaning seen in
// systest (a vector inserted in a 50-vector batch is occasionally
// unreachable at any beam width). Modes: {one txn for all inserts} ×
// {one txn per insert} × {monolithic hnsw, partitioned hnsw}. Each rep also
// verifies the BASE DATA count, pinning that the loss is index-only.
//
// Not part of the regression suite: run explicitly with
//   go test ./posting/ -run TestVectorBatchOrphanMatrix -v

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/v25/protos/pb"
	"github.com/dgraph-io/dgraph/v25/schema"
	"github.com/dgraph-io/dgraph/v25/tok"
	"github.com/dgraph-io/dgraph/v25/tok/hnsw"
	tokIndex "github.com/dgraph-io/dgraph/v25/tok/index"
	"github.com/dgraph-io/dgraph/v25/types"
	"github.com/dgraph-io/dgraph/v25/x"
)

func TestVectorBatchOrphanMatrix(t *testing.T) {
	if testing.Short() {
		t.Skip("diagnostic matrix, not a regression test")
	}
	ctx := context.Background()

	const (
		dim     = 16
		baseN   = 300
		insertN = 50
		reps    = 10
	)

	monoSpecs := []*pb.VectorIndexSpec{{
		Name:    "hnsw",
		Options: []*pb.OptionPair{{Key: "metric", Value: "euclidean"}},
	}}
	partSpecs := []*pb.VectorIndexSpec{{
		Name: "hnsw",
		Options: []*pb.OptionPair{
			{Key: "numClusters", Value: "8"},
			{Key: "numProbes", Value: "8"},
			{Key: "metric", Value: "euclidean"},
		},
	}}

	randVec := func(rng *rand.Rand) []float32 {
		v := make([]float32, dim)
		for j := range v {
			v[j] = rng.Float32() * 10
		}
		return v
	}

	// runRep builds a fresh predicate with baseN indexed vectors, inserts
	// insertN more through the live index path (batched into one txn or one
	// txn each), and returns how many of the inserts are unreachable at a
	// corpus-wide beam. Also asserts the base-data count is exact.
	runRep := func(t *testing.T, specs []*pb.VectorIndexSpec, batched bool, seed int64) int {
		attr := vecTestAttr(t, "vbatchdbg")
		indexedSchema := &pb.SchemaUpdate{
			Predicate: attr, ValueType: pb.Posting_VFLOAT,
			Directive: pb.SchemaUpdate_INDEX, IndexSpecs: specs,
		}
		bare := strings.TrimPrefix(attr, "0-")
		require.NoError(t, schema.ParseBytes([]byte(bare+`: float32vector .`), 1))
		// Unique attr per rep; the in-memory schema entry is left behind,
		// which is harmless for a diagnostic.
		schema.State().Set(attr, indexedSchema)

		testingVectorRebuildNumGo = 1
		defer func() { testingVectorRebuildNumGo = 0 }()

		rng := rand.New(rand.NewSource(seed))
		ts := uint64(1)
		vecs := map[uint64][]float32{}
		for uid := uint64(1); uid <= baseN; uid++ {
			v := randVec(rng)
			vecs[uid] = v
			vecTestWriteBase(t, ctx, attr, uid, v, &ts)
		}

		// Build the index over the base corpus.
		rebuildTs := vecTestNextTs(&ts)
		Oracle().ProcessDelta(&pb.OracleDelta{MaxAssigned: rebuildTs})
		rb := &IndexRebuild{
			Attr: attr, StartTs: rebuildTs,
			OldSchema:     &pb.SchemaUpdate{Predicate: attr, ValueType: pb.Posting_VFLOAT},
			CurrentSchema: indexedSchema,
		}
		require.NoError(t, rebuildTokIndex(ctx, rb))

		cspec, err := tok.GetFactoryCreateSpecFromSpec(specs[0])
		require.NoError(t, err)

		// Live inserts through the real index-maintenance path. The batched
		// mode mirrors one Mutate with insertN nquads: one transaction, all
		// index inserts sharing it (applied serially, as the apply path does
		// for <512-edge batches).
		insertUids := make([]uint64, 0, insertN)
		newVecs := map[uint64][]float32{}
		for i := 0; i < insertN; i++ {
			uid := uint64(baseN + 1 + i)
			insertUids = append(insertUids, uid)
			newVecs[uid] = randVec(rng)
			vecs[uid] = newVecs[uid]
		}

		liveIndexInsert := func(txn *Txn, uid uint64, vec []float32) {
			valBytes := types.FloatArrayAsBytes(vec)
			_, err := txn.addIndexMutations(ctx, &indexMutationInfo{
				factorySpecs: []*tok.FactoryCreateSpec{cspec},
				edge: &pb.DirectedEdge{
					Attr: attr, Entity: uid, Value: valBytes,
					ValueType: pb.Posting_VFLOAT, Op: pb.DirectedEdge_SET,
				},
				val: types.Val{Tid: types.VFloatID, Value: valBytes},
				op:  pb.DirectedEdge_SET,
			})
			require.NoError(t, err)
		}

		if batched {
			// Base values first (visible to the index txn), then one shared
			// index transaction for all inserts.
			for _, uid := range insertUids {
				vecTestWriteBase(t, ctx, attr, uid, newVecs[uid], &ts)
			}
			startTs := vecTestNextTs(&ts)
			txn := Oracle().RegisterStartTs(startTs)
			for _, uid := range insertUids {
				liveIndexInsert(txn, uid, newVecs[uid])
			}
			vecTestCommit(t, txn, startTs, vecTestNextTs(&ts))
		} else {
			for _, uid := range insertUids {
				vecTestWriteBase(t, ctx, attr, uid, newVecs[uid], &ts)
				startTs := vecTestNextTs(&ts)
				txn := Oracle().RegisterStartTs(startTs)
				liveIndexInsert(txn, uid, newVecs[uid])
				vecTestCommit(t, txn, startTs, vecTestNextTs(&ts))
			}
		}

		readTs := vecTestNextTs(&ts)
		Oracle().ProcessDelta(&pb.OracleDelta{MaxAssigned: readTs})

		// Base-data count must be exact regardless of index state.
		dataCount := 0
		pk := x.ParsedKey{Attr: attr}
		require.NoError(t, MemLayerInstance.IterateDisk(ctx, IterateDiskArgs{
			Prefix: pk.DataPrefix(), ReadTs: readTs, AllVersions: false,
			CheckInclusion: func(uint64) error { return nil },
			Function: func(l *List, _ x.ParsedKey) error {
				if val, err := l.Value(readTs); err == nil && val.Tid == types.VFloatID {
					dataCount++
				}
				return nil
			},
		}))
		require.Equalf(t, baseN+insertN, dataCount,
			"base data count wrong: count queries WOULD be affected")

		indexer, err := cspec.CreateIndex(attr)
		require.NoError(t, err)
		k := len(vecs) + 8
		orphans := 0
		for _, uid := range insertUids {
			qc := hnsw.NewQueryCache(NewViLocalCache(NewLocalCache(readTs)), readTs)
			found, err := indexer.Search(ctx, qc, newVecs[uid], k, tokIndex.AcceptAll[float32])
			require.NoError(t, err)
			present := false
			for _, f := range found {
				if f == uid {
					present = true
					break
				}
			}
			if !present {
				orphans++
			}
		}
		return orphans
	}

	type mode struct {
		name    string
		specs   []*pb.VectorIndexSpec
		batched bool
	}
	modes := []mode{
		{"monolithic/batched-one-txn", monoSpecs, true},
		{"monolithic/one-txn-per-vector", monoSpecs, false},
		{"partitioned/batched-one-txn", partSpecs, true},
		{"partitioned/one-txn-per-vector", partSpecs, false},
	}

	summary := make(map[string]string, len(modes))
	for _, m := range modes {
		t.Run(strings.ReplaceAll(m.name, "/", "_"), func(t *testing.T) {
			totalOrphans, repsWithOrphans := 0, 0
			for r := 0; r < reps; r++ {
				o := runRep(t, m.specs, m.batched, int64(1000+r))
				totalOrphans += o
				if o > 0 {
					repsWithOrphans++
				}
			}
			summary[m.name] = fmt.Sprintf("%d orphans across %d reps (%d reps affected, %d inserts/rep)",
				totalOrphans, reps, repsWithOrphans, insertN)
			t.Logf("MATRIX %s: %s", m.name, summary[m.name])
		})
	}
	for _, m := range modes {
		t.Logf("RESULT %-32s %s", m.name, summary[m.name])
	}
}

// TestVectorDrainThenInsertMatrix reproduces the e2e restart-leg sequence at
// the posting layer: build with mid-build captured mutations, drain, THEN a
// batch of live inserts — the one prologue the clean batch matrix lacked.
// Decides whether the residual e2e orphaning implicates the drain overlay.
func TestVectorDrainThenInsertMatrix(t *testing.T) {
	if testing.Short() {
		t.Skip("diagnostic matrix, not a regression test")
	}
	ctx := context.Background()
	const (
		dim   = 16
		baseN = 300
		reps  = 10
	)

	specsFor := func(partitioned bool) []*pb.VectorIndexSpec {
		if !partitioned {
			return []*pb.VectorIndexSpec{{
				Name:    "hnsw",
				Options: []*pb.OptionPair{{Key: "metric", Value: "euclidean"}},
			}}
		}
		return []*pb.VectorIndexSpec{{
			Name: "hnsw",
			Options: []*pb.OptionPair{
				{Key: "numClusters", Value: "8"},
				{Key: "numProbes", Value: "8"},
				{Key: "metric", Value: "euclidean"},
			},
		}}
	}

	randVec := func(rng *rand.Rand) []float32 {
		v := make([]float32, dim)
		for j := range v {
			v[j] = rng.Float32() * 10
		}
		return v
	}

	runRep := func(t *testing.T, partitioned bool, midN, delN, postN int, pinNumGo bool, seed int64) (baseOrphans, drainOrphans, postOrphans int) {
		attr := vecTestAttr(t, "vdraindbg")
		specs := specsFor(partitioned)
		indexedSchema := &pb.SchemaUpdate{
			Predicate: attr, ValueType: pb.Posting_VFLOAT,
			Directive: pb.SchemaUpdate_INDEX, IndexSpecs: specs,
		}
		bare := strings.TrimPrefix(attr, "0-")
		require.NoError(t, schema.ParseBytes([]byte(bare+`: float32vector .`), 1))
		schema.State().Set(attr, indexedSchema)

		if pinNumGo {
			testingVectorRebuildNumGo = 1
			defer func() { testingVectorRebuildNumGo = 0 }()
		}

		rng := rand.New(rand.NewSource(seed))
		ts := uint64(1)
		baseVecs := map[uint64][]float32{}
		for uid := uint64(1); uid <= baseN; uid++ {
			v := randVec(rng)
			baseVecs[uid] = v
			vecTestWriteBase(t, ctx, attr, uid, v, &ts)
		}

		cspec, err := tok.GetFactoryCreateSpecFromSpec(specs[0])
		require.NoError(t, err)

		liveIndexInsert := func(txn *Txn, uid uint64, vec []float32, op pb.DirectedEdge_Op) {
			valBytes := types.FloatArrayAsBytes(vec)
			edge := &pb.DirectedEdge{Attr: attr, Entity: uid, ValueType: pb.Posting_VFLOAT, Op: op}
			if op == pb.DirectedEdge_SET {
				edge.Value = valBytes
			}
			_, err := txn.addIndexMutations(ctx, &indexMutationInfo{
				factorySpecs: []*tok.FactoryCreateSpec{cspec},
				edge:         edge,
				val:          types.Val{Tid: types.VFloatID, Value: valBytes},
				op:           op,
			})
			require.NoError(t, err)
		}
		oneTxnMutation := func(uid uint64, vec []float32, op pb.DirectedEdge_Op) {
			startTs := vecTestNextTs(&ts)
			txn := Oracle().RegisterStartTs(startTs)
			liveIndexInsert(txn, uid, vec, op)
			vecTestCommit(t, txn, startTs, vecTestNextTs(&ts))
		}

		// The restart-replay shape: gate up before the rebuild, mid-build
		// mutations captured at training_done, drained by the build.
		rebuildTs := vecTestNextTs(&ts)
		Oracle().ProcessDelta(&pb.OracleDelta{MaxAssigned: rebuildTs})
		StartVectorRebuildCapture(attr)
		defer FinishVectorRebuildCapture(attr)

		drainVecs := map[uint64][]float32{}
		injected := false
		TestingVectorRebuildStageHook = func(stage string) {
			if stage != "training_done" || injected {
				return
			}
			injected = true
			for i := 0; i < midN; i++ {
				uid := uint64(baseN + 100 + i)
				v := randVec(rng)
				drainVecs[uid] = v
				vecTestWriteBase(t, ctx, attr, uid, v, &ts)
				oneTxnMutation(uid, v, pb.DirectedEdge_SET)
			}
			for i := 0; i < delN; i++ {
				uid := uint64(1 + i) // delete base uids 1..delN
				oneTxnMutation(uid, baseVecs[uid], pb.DirectedEdge_DEL)
				delete(baseVecs, uid)
			}
		}
		defer func() { TestingVectorRebuildStageHook = nil }()

		rb := &IndexRebuild{
			Attr: attr, StartTs: rebuildTs,
			OldSchema:     &pb.SchemaUpdate{Predicate: attr, ValueType: pb.Posting_VFLOAT},
			CurrentSchema: indexedSchema,
		}
		require.NoError(t, rebuildTokIndex(ctx, rb))
		require.True(t, injected)
		require.False(t, VectorRebuildCaptureActive(attr), "drain must close the gate")

		// The e2e step under suspicion: a batched live insert into the
		// drain-overlaid graph.
		postVecs := map[uint64][]float32{}
		for i := 0; i < postN; i++ {
			uid := uint64(baseN + 200 + i)
			postVecs[uid] = randVec(rng)
			vecTestWriteBase(t, ctx, attr, uid, postVecs[uid], &ts)
		}
		startTs := vecTestNextTs(&ts)
		txn := Oracle().RegisterStartTs(startTs)
		for uid, v := range postVecs {
			liveIndexInsert(txn, uid, v, pb.DirectedEdge_SET)
		}
		vecTestCommit(t, txn, startTs, vecTestNextTs(&ts))

		readTs := vecTestNextTs(&ts)
		Oracle().ProcessDelta(&pb.OracleDelta{MaxAssigned: readTs})
		indexer, err := cspec.CreateIndex(attr)
		require.NoError(t, err)

		total := len(baseVecs) + len(drainVecs) + len(postVecs)
		countOrphans := func(vecs map[uint64][]float32) int {
			n := 0
			for uid, vec := range vecs {
				qc := hnsw.NewQueryCache(NewViLocalCache(NewLocalCache(readTs)), readTs)
				found, err := indexer.Search(ctx, qc, vec, total+8, tokIndex.AcceptAll[float32])
				require.NoError(t, err)
				present := false
				for _, f := range found {
					if f == uid {
						present = true
						break
					}
				}
				if !present {
					n++
				}
			}
			return n
		}
		b, d, pn := countOrphans(baseVecs), countOrphans(drainVecs), countOrphans(postVecs)
		// Structural autopsy for a lost drained vector: does ANY graph row
		// point at it (write-side vs search-side loss), and does it have
		// rows of its own?
		if d > 0 && len(drainVecs) <= 2 {
			for uid := range drainVecs {
				ownRows, backlinks := 0, 0
				vecAttr := hnsw.ConcatStrings(attr, hnsw.VecKeyword)
				pk2 := x.ParsedKey{Attr: vecAttr}
				require.NoError(t, MemLayerInstance.IterateDisk(ctx, IterateDiskArgs{
					Prefix: pk2.DataPrefix(), ReadTs: readTs, AllVersions: false,
					CheckInclusion: func(uint64) error { return nil },
					Function: func(l *List, k x.ParsedKey) error {
						val, err := l.Value(readTs)
						if err != nil || val.Value == nil {
							return nil
						}
						if k.Uid == uid {
							ownRows++
							var m [][]uint64
							if decodeUint64MatrixUnsafe(val.Value.([]byte), &m) == nil {
								t.Logf("AUTOPSY uid %d own adjacency: %v", uid, m)
							}
							return nil
						}
						var m [][]uint64
						if decodeUint64MatrixUnsafe(val.Value.([]byte), &m) != nil {
							return nil
						}
						for _, row := range m {
							for _, n := range row {
								if n == uid {
									backlinks++
									return nil
								}
							}
						}
						return nil
					},
				}))
				t.Logf("AUTOPSY uid %d: ownRows=%d rowsWithBacklink=%d (0 backlinks = write-side loss; >0 = search-side)",
					uid, ownRows, backlinks)
			}
		}
		return b, d, pn
	}

	cases := []struct {
		name        string
		partitioned bool
		midN, delN  int
		postN       int
		pinNumGo    bool
	}{
		{"monolithic_mid1_del0_post50", false, 1, 0, 50, true},
		{"monolithic_mid1_del0_nopost", false, 1, 0, 0, true},
		{"monolithic_mid10_del2_post50", false, 10, 2, 50, true},
		{"partitioned_mid10_del2_post50", true, 10, 2, 50, true},
		{"partitioned_prodconc_post50", true, 10, 2, 50, false},
		{"monolithic_prodconc_post50", false, 10, 2, 50, false},
	}
	for _, c := range cases {
		name := c.name
		t.Run(name, func(t *testing.T) {
			tb, td, tp := 0, 0, 0
			for r := 0; r < reps; r++ {
				b, d, p := runRep(t, c.partitioned, c.midN, c.delN, c.postN, c.pinNumGo, int64(2000+r))
				tb, td, tp = tb+b, td+d, tp+p
			}
			t.Logf("RESULT %s drain-then-insert: baseOrphans=%d drainOrphans=%d postInsertOrphans=%d (over %d reps)",
				name, tb, td, tp, reps)
			// Regression contract: nothing may be orphaned. The monolithic
			// drain once orphaned 66% of drained vectors by reusing one
			// persistentHNSW instance — whose per-instance adjacency cache
			// is scoped to a single transaction's view — across the per-uid
			// replay transactions.
			require.Zerof(t, tb, "%s: base vectors orphaned", name)
			require.Zerof(t, td, "%s: drained vectors orphaned", name)
			require.Zerof(t, tp, "%s: post-drain inserts orphaned", name)
		})
	}
}
