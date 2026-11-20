/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package posting

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/v25/protos/pb"
	"github.com/dgraph-io/dgraph/v25/schema"
	"github.com/dgraph-io/dgraph/v25/tok/hnsw"
	"github.com/dgraph-io/dgraph/v25/tok/index"
	"github.com/dgraph-io/dgraph/v25/types"
	"github.com/dgraph-io/dgraph/v25/x"
)

// These tests cover the dead-node lifecycle of the HNSW vector index, which has
// two correctness bugs:
//
//	(A) Deleting a vector is supposed to record its uid in the predicate's
//	    "__vector_dead" list so that removeDeadNodes can strip edges pointing at
//	    it. But addIndexMutations reads the value from AllValues(StartTs) *after*
//	    addMutationHelper has already applied the delete, so the read is empty
//	    and the dead node is never recorded. The old value is available as
//	    info.val but is ignored.
//
//	(B) The dead list is append-only: re-inserting (re-embedding) a previously
//	    deleted uid never removes it from "__vector_dead". Once (A) is fixed and
//	    the list is populated, a re-inserted vector would be permanently treated
//	    as dead — stripped from every neighbour's edge list, leaving it with no
//	    inbound edges and unreachable from the HNSW entry point (it has an
//	    embedding but is missing from search results).
//
// Together these tests pin the full lifecycle: delete records, re-insert clears,
// and a deleted-then-reinserted vector stays searchable.

// parseVecSchema registers an HNSW-indexed float32vector predicate for tests.
func parseVecSchema(t *testing.T, pred string) {
	require.NoError(t, schema.ParseBytes(
		[]byte(pred+`: float32vector @index(hnsw(metric: "euclidean")) .`), 1))
}

// setVector SETs a float32 vector for uid through the full indexed-mutation path
// (AddMutationWithIndex -> addIndexMutations -> HNSW insert).
func setVector(t *testing.T, attr string, uid uint64, vec []float32, startTs, commitTs uint64) {
	key := x.DataKey(attr, uid)
	l, err := GetNoStore(key, startTs)
	require.NoError(t, err)
	edge := &pb.DirectedEdge{
		Attr:      attr,
		Entity:    uid,
		Value:     types.FloatArrayAsBytes(vec),
		ValueType: pb.Posting_VFLOAT,
	}
	addMutation(t, l, edge, Set, startTs, commitTs, true)
}

// delVector DELs the vector for uid through the full indexed-mutation path.
func delVector(t *testing.T, attr string, uid uint64, vec []float32, startTs, commitTs uint64) {
	key := x.DataKey(attr, uid)
	l, err := GetNoStore(key, startTs)
	require.NoError(t, err)
	edge := &pb.DirectedEdge{
		Attr:      attr,
		Entity:    uid,
		Value:     types.FloatArrayAsBytes(vec),
		ValueType: pb.Posting_VFLOAT,
	}
	addMutation(t, l, edge, Del, startTs, commitTs, true)
}

// readDeadList returns the uids recorded in the predicate's __vector_dead list.
func readDeadList(t *testing.T, attr string, readTs uint64) []uint64 {
	deadKey := x.DataKey(hnsw.ConcatStrings(attr, hnsw.VecDead), 1)
	l, err := GetNoStore(deadKey, readTs)
	require.NoError(t, err)
	val, err := l.Value(readTs)
	if err == ErrNoValue {
		return nil
	}
	require.NoError(t, err)
	b, ok := val.Value.([]byte)
	require.True(t, ok, "dead list value should be []byte")
	dead, err := hnsw.ParseEdges(string(b))
	require.NoError(t, err)
	return dead
}

// TestDeletedVectorRecordedAsDead verifies bug (A): deleting a vector must record
// its uid in __vector_dead. The probe in this package confirms the delete itself
// applies (the value is gone), so a missing dead-list entry isolates the
// record-on-delete defect.
func TestDeletedVectorRecordedAsDead(t *testing.T) {
	parseVecSchema(t, "vecdead")
	attr := x.AttrInRootNamespace("vecdead")

	setVector(t, attr, 1, []float32{0, 0}, 1, 2)
	setVector(t, attr, 2, []float32{1, 0}, 3, 4)

	delVector(t, attr, 2, []float32{1, 0}, 5, 6)

	require.Contains(t, readDeadList(t, attr, 7), uint64(2),
		"deleting a vector must record its uid in __vector_dead")
}

// TestReinsertedVectorClearedFromDeadList verifies bug (B): once a deleted uid is
// re-inserted, it must be removed from __vector_dead so it is no longer treated
// as dead.
func TestReinsertedVectorClearedFromDeadList(t *testing.T) {
	parseVecSchema(t, "vecdead2")
	attr := x.AttrInRootNamespace("vecdead2")

	setVector(t, attr, 1, []float32{0, 0}, 11, 12)
	setVector(t, attr, 2, []float32{1, 0}, 13, 14)

	delVector(t, attr, 2, []float32{1, 0}, 15, 16)
	require.Contains(t, readDeadList(t, attr, 17), uint64(2),
		"precondition: delete should record the uid as dead")

	setVector(t, attr, 2, []float32{1, 0}, 18, 19)
	require.NotContains(t, readDeadList(t, attr, 20), uint64(2),
		"re-inserting a vector must remove its uid from __vector_dead")
}

// TestReinsertedVectorIsSearchable re-verifies the lifecycle end-to-end from the
// user-facing angle: after delete + re-insert, a self-search by the node's own
// vector must return it. If the uid stays in the dead list it is stripped from
// neighbour edge lists, orphaned, and missing from search.
func TestReinsertedVectorIsSearchable(t *testing.T) {
	parseVecSchema(t, "vecdead3")
	attr := x.AttrInRootNamespace("vecdead3")
	ctx := context.Background()

	// Entry node first (uid 1), then a small cluster.
	setVector(t, attr, 1, []float32{0, 0}, 31, 32)
	setVector(t, attr, 2, []float32{10, 0}, 33, 34)
	setVector(t, attr, 3, []float32{10, 1}, 35, 36)
	setVector(t, attr, 4, []float32{11, 0}, 37, 38)

	// Delete then re-insert uid 2.
	delVector(t, attr, 2, []float32{10, 0}, 39, 40)
	setVector(t, attr, 2, []float32{10, 0}, 41, 42)

	readTs := uint64(50)
	specs, err := schema.State().FactoryCreateSpec(ctx, attr)
	require.NoError(t, err)
	require.NotEmpty(t, specs)
	indexer, err := specs[0].CreateIndex(attr)
	require.NoError(t, err)

	lc := NewLocalCache(readTs)
	qc := hnsw.NewQueryCache(NewViLocalCache(lc), readTs)
	res, err := indexer.SearchWithUid(ctx, qc, 2, 5, index.AcceptAll[float32])
	require.NoError(t, err)
	require.Contains(t, res, uint64(2),
		"a deleted-then-reinserted vector must be reachable in HNSW search")
}

// TestDeadNodeConcurrentRecord guards the read-modify-write of the single
// dead-node posting. A multi-edge mutation applies its edges in parallel
// goroutines sharing one txn (worker/draft.go), all targeting the same
// __vector_dead posting, so an unsynchronized RMW would lose updates. Every
// concurrently-recorded uid must survive.
func TestDeadNodeConcurrentRecord(t *testing.T) {
	parseVecSchema(t, "vecdeadc")
	attr := x.AttrInRootNamespace("vecdeadc")
	deadAttr := hnsw.ConcatStrings(attr, hnsw.VecDead)
	ctx := context.Background()

	txn := Oracle().RegisterStartTs(100)
	const n = 64
	var wg sync.WaitGroup
	for i := 1; i <= n; i++ {
		wg.Add(1)
		go func(uid uint64) {
			defer wg.Done()
			require.NoError(t, txn.recordDeadNode(ctx, deadAttr, uid))
		}(uint64(i))
	}
	wg.Wait()

	pl, err := txn.Get(x.DataKey(deadAttr, 1))
	require.NoError(t, err)
	val, err := pl.Value(100)
	require.NoError(t, err)
	dead, err := hnsw.ParseEdges(string(val.Value.([]byte)))
	require.NoError(t, err)
	require.Len(t, dead, n, "concurrent recordDeadNode must not lose updates")
}

// TestDeadNodeSameTxnRecordThenClear verifies the read-modify-write sees the
// txn's own pending updates: recording then clearing the same uid within one
// transaction must leave it absent from the dead list.
func TestDeadNodeSameTxnRecordThenClear(t *testing.T) {
	parseVecSchema(t, "vecdeads")
	attr := x.AttrInRootNamespace("vecdeads")
	deadAttr := hnsw.ConcatStrings(attr, hnsw.VecDead)
	ctx := context.Background()

	txn := Oracle().RegisterStartTs(200)
	require.NoError(t, txn.recordDeadNode(ctx, deadAttr, 7))
	require.NoError(t, txn.recordDeadNode(ctx, deadAttr, 8))
	require.NoError(t, txn.clearDeadNode(ctx, deadAttr, 7))

	pl, err := txn.Get(x.DataKey(deadAttr, 1))
	require.NoError(t, err)
	val, err := pl.Value(200)
	require.NoError(t, err)
	dead, err := hnsw.ParseEdges(string(val.Value.([]byte)))
	require.NoError(t, err)
	require.NotContains(t, dead, uint64(7), "clear must observe the in-txn record")
	require.Contains(t, dead, uint64(8))
}
