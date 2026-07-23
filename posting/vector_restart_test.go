/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package posting

import (
	"context"
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	bpb "github.com/dgraph-io/badger/v4/pb"
	"github.com/dgraph-io/dgraph/v25/protos/pb"
	"github.com/dgraph-io/dgraph/v25/schema"
	"github.com/dgraph-io/dgraph/v25/tok"
	"github.com/dgraph-io/dgraph/v25/tok/hnsw"
	tokIndex "github.com/dgraph-io/dgraph/v25/tok/index"
	"github.com/dgraph-io/dgraph/v25/tok/partitioned_hnsw"
	"github.com/dgraph-io/dgraph/v25/types"
	"github.com/dgraph-io/dgraph/v25/x"
)

// TestPartitionedRestartHydration simulates an alpha restart: build a
// partitioned vector index, then force a brand-new index instance (empty
// in-memory centroids, exactly what a restarted process has) and verify that
// searches through the real query-path caches hydrate the persisted
// centroids and route correctly — every vector must find itself as its own
// nearest neighbor.
func TestPartitionedRestartHydration(t *testing.T) {
	ctx := context.Background()
	attr := x.AttrInRootNamespace("phrestart")

	indexedSchema := &pb.SchemaUpdate{
		Predicate: attr,
		ValueType: pb.Posting_VFLOAT,
		Directive: pb.SchemaUpdate_INDEX,
		IndexSpecs: []*pb.VectorIndexSpec{{
			Name: partitioned_hnsw.PartitionedHNSW,
			Options: []*pb.OptionPair{
				{Key: partitioned_hnsw.NumClustersOpt, Value: "4"},
				{Key: partitioned_hnsw.NumProbesOpt, Value: "2"},
				{Key: "metric", Value: "euclidean"},
			},
		}},
	}
	require.NoError(t, schema.ParseBytes(
		[]byte(`phrestart: float32vector @index(partionedhnsw(numClusters: "4", numProbes: "2", metric: "euclidean")) .`), 1))

	// Four well-separated blobs in 4d, 15 vectors each.
	corners := [][]float32{
		{0, 0, 0, 0},
		{100, 100, 0, 0},
		{0, 0, 100, 100},
		{100, 0, 0, 100},
	}
	vecs := make(map[uint64][]float32)
	uid := uint64(1)
	for _, corner := range corners {
		for i := range 15 {
			v := make([]float32, 4)
			for j := range v {
				v[j] = corner[j] + float32(i)*0.1 + float32(j)*0.05
			}
			vecs[uid] = v
			uid++
		}
	}

	ts := uint64(1)
	nextTs := func() uint64 { ts++; return ts }

	writeVec := func(uid uint64, vec []float32) {
		startTs := nextTs()
		txn := Oracle().RegisterStartTs(startTs)
		key := x.DataKey(attr, uid)
		l, err := GetNoStore(key, math.MaxUint64)
		require.NoError(t, err)
		l = txn.Store(l)
		l.SetTs(startTs)
		edge := &pb.DirectedEdge{
			Attr:      attr,
			Entity:    uid,
			Value:     types.FloatArrayAsBytes(vec),
			ValueType: pb.Posting_VFLOAT,
			Op:        pb.DirectedEdge_SET,
		}
		require.NoError(t, l.addMutation(ctx, txn, edge))

		commitTs := nextTs()
		Oracle().ProcessDelta(&pb.OracleDelta{
			Txns:        []*pb.TxnStatus{{StartTs: startTs, CommitTs: commitTs}},
			MaxAssigned: commitTs,
		})
		txn.Update()
		writer := NewTxnWriter(pstore)
		require.NoError(t, txn.CommitToDisk(writer, commitTs))
		require.NoError(t, writer.Flush())
	}

	for uid, vec := range vecs {
		writeVec(uid, vec)
	}

	// Build the index the same way a schema alter does.
	rebuildTs := nextTs()
	Oracle().ProcessDelta(&pb.OracleDelta{MaxAssigned: rebuildTs})
	rb := &IndexRebuild{
		Attr:          attr,
		StartTs:       rebuildTs,
		OldSchema:     &pb.SchemaUpdate{Predicate: attr, ValueType: pb.Posting_VFLOAT},
		CurrentSchema: indexedSchema,
	}
	require.NoError(t, rebuildTokIndex(ctx, rb))

	cspec, err := tok.GetFactoryCreateSpecFromSpec(indexedSchema.IndexSpecs[0])
	require.NoError(t, err)

	readTs := nextTs()
	Oracle().ProcessDelta(&pb.OracleDelta{MaxAssigned: readTs})
	newQueryCache := func() tokIndex.CacheType {
		return hnsw.NewQueryCache(NewViLocalCache(NewLocalCache(readTs)), readTs)
	}

	selfRecall := func(t *testing.T, indexer tokIndex.VectorIndex[float32]) {
		for uid, vec := range vecs {
			found, err := indexer.Search(ctx, newQueryCache(), vec, 1, tokIndex.AcceptAll[float32])
			require.NoErrorf(t, err, "uid %d: search failed", uid)
			require.Lenf(t, found, 1, "uid %d: no results", uid)
			if uid != found[0] {
				all, err := indexer.Search(ctx, newQueryCache(), vec, len(vecs), tokIndex.AcceptAll[float32])
				require.NoError(t, err)
				resolver := indexer.(tokIndex.VectorDeadListResolver[float32])
				deadAttr, err := resolver.DeadAttrForVector(newQueryCache(), vec)
				require.NoError(t, err)
				t.Fatalf("uid %d did not find itself: top1=%d, insert shard=%q, full probe returned %d uids: %v",
					uid, found[0], deadAttr, len(all), all)
			}
		}
	}

	// Sanity: the long-lived instance the rebuild registered routes fine.
	indexer, err := cspec.FindOrCreateIndex(attr)
	require.NoError(t, err)
	t.Logf("centroids after rebuild: %v", indexer.GetCentroids())
	for i := range 4 {
		attrName := hnsw.SplitVecAttr(attr, i)
		n := 0
		require.NoError(t, MemLayerInstance.IterateDisk(ctx, IterateDiskArgs{
			Prefix:         x.PredicatePrefix(attrName),
			ReadTs:         readTs,
			AllVersions:    false,
			CheckInclusion: func(uint64) error { return nil },
			Function: func(l *List, pk x.ParsedKey) error {
				n++
				return nil
			},
		}))
		t.Logf("cluster %d: %d graph keys", i, n)
	}
	selfRecall(t, indexer)

	// "Restart": a brand-new instance with empty in-memory centroids must
	// hydrate from the persisted centroid key and route identically.
	fresh, err := cspec.CreateIndex(attr)
	require.NoError(t, err)
	selfRecall(t, fresh)

	// Roll up every aux key the way the background rollup process would,
	// then search again through a fresh instance: rollups must not corrupt
	// or lose graph state (an alpha restart makes rolled-up data the only
	// copy, so this is the restart-durability contract).
	rollupTs := nextTs()
	Oracle().ProcessDelta(&pb.OracleDelta{MaxAssigned: rollupTs})
	for i := range 4 {
		for _, attrName := range []string{
			hnsw.SplitEntryAttr(attr, i),
			hnsw.SplitVecAttr(attr, i),
			hnsw.SplitDeadAttr(attr, i),
		} {
			require.NoError(t, MemLayerInstance.IterateDisk(ctx, IterateDiskArgs{
				Prefix:         x.PredicatePrefix(attrName),
				ReadTs:         rollupTs,
				AllVersions:    false,
				CheckInclusion: func(uint64) error { return nil },
				Function: func(l *List, pk x.ParsedKey) error {
					key := x.DataKey(attrName, pk.Uid)
					pl, err := GetNoStore(key, rollupTs)
					require.NoError(t, err)
					kvs, err := pl.Rollup(nil, rollupTs+1)
					require.NoError(t, err)
					writer := NewTxnWriter(pstore)
					require.NoError(t, writer.Write(&bpb.KVList{Kv: kvs}))
					require.NoError(t, writer.Flush())
					return nil
				},
			}))
		}
	}
	// Drop cached lists so reads go back to the rolled-up disk state.
	memClear := nextTs()
	Oracle().ProcessDelta(&pb.OracleDelta{MaxAssigned: memClear})
	readTs = nextTs()
	Oracle().ProcessDelta(&pb.OracleDelta{MaxAssigned: readTs})

	afterRollup, err := cspec.CreateIndex(attr)
	require.NoError(t, err)
	selfRecall(t, afterRollup)

	// A restarted alpha replays the schema mutation from the raft WAL and
	// re-runs the whole rebuild: drop the index prefixes and rebuild with
	// the SAME IndexRebuild (same StartTs). The replayed index must serve
	// identical results.
	for _, prefix := range prefixesToDropVectorIndexEdges(ctx, rb) {
		require.NoError(t, pstore.DropPrefix(prefix))
	}
	require.NoError(t, rebuildTokIndex(ctx, rb))

	readTs = nextTs()
	Oracle().ProcessDelta(&pb.OracleDelta{MaxAssigned: readTs})
	replayed, err := cspec.CreateIndex(attr)
	require.NoError(t, err)
	selfRecall(t, replayed)
}
