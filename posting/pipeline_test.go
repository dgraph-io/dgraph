/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package posting

// Tests for MutationPipeline correctness — focused on correctness gaps vs the
// legacy runMutation path discovered during code review.

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/v25/protos/pb"
	"github.com/dgraph-io/dgraph/v25/schema"
	"github.com/dgraph-io/dgraph/v25/types"
	"github.com/dgraph-io/dgraph/v25/x"
)

// commitPipelineTxn flushes a pipeline txn to Badger and marks it committed.
// Mirrors what draft.go does after applyMutations returns.
func commitPipelineTxn(t *testing.T, txn *Txn, commitTs uint64) {
	t.Helper()
	txn.Update()
	txn.UpdateCachedKeys(commitTs)
	writer := NewTxnWriter(pstore)
	require.NoError(t, txn.CommitToDisk(writer, commitTs))
	require.NoError(t, writer.Flush())
}

// applyEdges runs edges through a fresh pipeline txn and commits them.
func applyEdges(t *testing.T, startTs, commitTs uint64, edges ...*pb.DirectedEdge) {
	t.Helper()
	txn := Oracle().RegisterStartTs(startTs)
	mp := NewMutationPipeline(txn)
	require.NoError(t, mp.Process(context.Background(), edges))
	commitPipelineTxn(t, txn, commitTs)
}

// applyEdgesLegacy runs edges through the legacy runMutation path and commits them.
// Used to verify that legacy and pipeline produce the same results.
func applyEdgesLegacy(t *testing.T, startTs, commitTs uint64, edges ...*pb.DirectedEdge) {
	t.Helper()
	txn := Oracle().RegisterStartTs(startTs)
	for _, edge := range edges {
		require.NoError(t, runMutation(context.Background(), edge, txn))
	}
	commitPipelineTxn(t, txn, commitTs)
}

// forwardUids returns the target uids stored in a forward data list at readTs.
func forwardUids(t *testing.T, attr string, entity, readTs uint64) []uint64 {
	t.Helper()
	l, err := GetNoStore(x.DataKey(attr, entity), readTs)
	require.NoError(t, err)
	return uids(l, readTs)
}

// reverseUids returns the source uids stored in a reverse index list at readTs.
func reverseUids(t *testing.T, attr string, target, readTs uint64) []uint64 {
	t.Helper()
	l, err := GetNoStore(x.ReverseKey(attr, target), readTs)
	require.NoError(t, err)
	return uids(l, readTs)
}

// indexUidsForVal reads the exact-index entries for a plain (non-lang) string
// @index predicate.  It reuses indexUidsForLang with an empty language tag.
func indexUidsForVal(t *testing.T, attr, val string, readTs uint64) []uint64 {
	t.Helper()
	return indexUidsForLang(t, attr, "", val, readTs)
}

// Each test uses a unique timestamp band so the Oracle's maxAssignedTs from one
// test never interferes with GetScalarList reads in a subsequent test.
// Band layout (startTs / commitTs / readTs):
//   Pipeline tests:            100-199, 200-299, 300-399, 400-499, 900-999
//   Legacy tests:              500-599, 600-699, 700-799, 800-899, 1000-1099
//   Lang index:                Pipeline 900-999, Legacy 1000-1099
//   Star-delete+SET:           Pipeline 1100-1199, Legacy 1200-1299
//   Single SET+DEL:            Pipeline 1300-1399, Legacy 1400-1499
//   DEL wrong target:          Pipeline 1500-1599, Legacy 1600-1699
//   Lang SET-then-DEL:         Pipeline 1700-1799, Legacy 1800-1899
//   Multi-SET scalar:          Pipeline 1900-1999, Legacy 2000-2099
//   Case-insensitive dup DEL:  Pipeline 2100-2199, Legacy 2200-2299
//   Channel deadlock:          2300-2399
//   Non-matching DEL no-op:    Pipeline 2400-2499, Legacy 2500-2599

// TestMutationPipelineSingleUidReverseUpdate is the regression test for the
// handleOldDeleteForSingle value-comparison bug on non-list uid @reverse predicates.
//
// Root cause: handleOldDeleteForSingle compares posting.Value (always nil for uid
// postings — the target uid lives in posting.Uid) using string byte comparison.
// `string(nil) == string(nil)` is always true, so the function concludes "new value
// equals old value → no-op" for every uid SET, clears the accumulated postings to an
// empty PostingList, and neither the forward write nor the reverse-edge update ever
// happens.
//
// Expected (correct) behaviour after the fix:
//   - alice's forward list is updated to bob.
//   - The new reverse edge bob ← alice is created.
//   - The stale reverse edge charlie ← alice is removed.
//
// Observed (buggy) behaviour without the fix:
//   - alice's forward list still points to charlie (SET is a no-op).
//   - bob's reverse list remains empty.
//   - charlie's reverse list still contains alice.
func TestMutationPipelineSingleUidReverseUpdate(t *testing.T) {
	require.NoError(t, pstore.DropAll())
	MemLayerInstance.clear()

	require.NoError(t, schema.ParseBytes([]byte(`best_friend: uid @reverse .`), 1))

	attr := x.AttrInRootNamespace("best_friend")
	const (
		alice   = uint64(10)
		bob     = uint64(20)
		charlie = uint64(30)
	)

	// Step 1 — initial state: alice → best_friend → charlie.
	applyEdges(t, 110, 111, &pb.DirectedEdge{
		Entity: alice, Attr: attr, ValueId: charlie, Op: pb.DirectedEdge_SET,
	})

	// Sanity-check initial state.
	require.Equal(t, []uint64{charlie}, forwardUids(t, attr, alice, 112),
		"initial: alice should point to charlie")
	require.Equal(t, []uint64{alice}, reverseUids(t, attr, charlie, 112),
		"initial: charlie's reverse should contain alice")
	require.Empty(t, reverseUids(t, attr, bob, 112),
		"initial: bob's reverse should be empty")

	// Step 2 — update: SET alice → best_friend → bob  (replaces charlie).
	applyEdges(t, 120, 121, &pb.DirectedEdge{
		Entity: alice, Attr: attr, ValueId: bob, Op: pb.DirectedEdge_SET,
	})

	const readTs = uint64(122)

	require.Equal(t, []uint64{bob}, forwardUids(t, attr, alice, readTs),
		"alice's best_friend must be updated to bob")
	require.Contains(t, reverseUids(t, attr, bob, readTs), alice,
		"bob's reverse index must now contain alice")
	require.NotContains(t, reverseUids(t, attr, charlie, readTs), alice,
		"charlie's reverse index must no longer contain alice")
}

// TestMutationPipelineSingleUidReverseUpdateChain tests three consecutive updates to
// the same non-list uid @reverse predicate to verify that each transition is applied
// correctly and no stale reverse edges accumulate.
//
//	alice → charlie  →  alice → bob  →  alice → dave
func TestMutationPipelineSingleUidReverseUpdateChain(t *testing.T) {
	require.NoError(t, pstore.DropAll())
	MemLayerInstance.clear()

	require.NoError(t, schema.ParseBytes([]byte(`best_friend: uid @reverse .`), 1))

	attr := x.AttrInRootNamespace("best_friend")
	const (
		alice   = uint64(10)
		bob     = uint64(20)
		charlie = uint64(30)
		dave    = uint64(40)
	)

	// alice → charlie
	applyEdges(t, 210, 211, &pb.DirectedEdge{
		Entity: alice, Attr: attr, ValueId: charlie, Op: pb.DirectedEdge_SET,
	})

	// alice → bob  (first replacement)
	applyEdges(t, 220, 221, &pb.DirectedEdge{
		Entity: alice, Attr: attr, ValueId: bob, Op: pb.DirectedEdge_SET,
	})

	const afterBob = uint64(222)
	require.Equal(t, []uint64{bob}, forwardUids(t, attr, alice, afterBob))
	require.Contains(t, reverseUids(t, attr, bob, afterBob), alice)
	require.NotContains(t, reverseUids(t, attr, charlie, afterBob), alice)

	// alice → dave  (second replacement)
	applyEdges(t, 230, 231, &pb.DirectedEdge{
		Entity: alice, Attr: attr, ValueId: dave, Op: pb.DirectedEdge_SET,
	})

	const afterDave = uint64(232)
	require.Equal(t, []uint64{dave}, forwardUids(t, attr, alice, afterDave),
		"alice's best_friend must be dave after second replacement")
	require.Contains(t, reverseUids(t, attr, dave, afterDave), alice,
		"dave's reverse must contain alice")
	require.NotContains(t, reverseUids(t, attr, bob, afterDave), alice,
		"bob's reverse must no longer contain alice after second replacement")
	require.NotContains(t, reverseUids(t, attr, charlie, afterDave), alice,
		"charlie's reverse must still not contain alice")
}

// TestMutationPipelineSingleUidReverseSetSameValue tests that re-setting the same
// uid value is correctly treated as a no-op: the forward list is unchanged and no
// spurious reverse-edge churn occurs.
func TestMutationPipelineSingleUidReverseSetSameValue(t *testing.T) {
	require.NoError(t, pstore.DropAll())
	MemLayerInstance.clear()

	require.NoError(t, schema.ParseBytes([]byte(`best_friend: uid @reverse .`), 1))

	attr := x.AttrInRootNamespace("best_friend")
	const (
		alice   = uint64(10)
		charlie = uint64(30)
	)

	// Initial: alice → charlie.
	applyEdges(t, 310, 311, &pb.DirectedEdge{
		Entity: alice, Attr: attr, ValueId: charlie, Op: pb.DirectedEdge_SET,
	})

	// Re-set the same value.
	applyEdges(t, 320, 321, &pb.DirectedEdge{
		Entity: alice, Attr: attr, ValueId: charlie, Op: pb.DirectedEdge_SET,
	})

	const readTs = uint64(322)

	require.Equal(t, []uint64{charlie}, forwardUids(t, attr, alice, readTs),
		"alice's best_friend must still be charlie after no-op re-set")
	require.Contains(t, reverseUids(t, attr, charlie, readTs), alice,
		"charlie's reverse must still contain alice after no-op re-set")
}

// TestMutationPipelineSingleUidReverseDelete tests that DEL on a non-list uid
// @reverse predicate removes both the forward entry and the reverse index entry.
func TestMutationPipelineSingleUidReverseDelete(t *testing.T) {
	require.NoError(t, pstore.DropAll())
	MemLayerInstance.clear()

	require.NoError(t, schema.ParseBytes([]byte(`best_friend: uid @reverse .`), 1))

	attr := x.AttrInRootNamespace("best_friend")
	const (
		alice   = uint64(10)
		charlie = uint64(30)
	)

	// Initial: alice → charlie.
	applyEdges(t, 410, 411, &pb.DirectedEdge{
		Entity: alice, Attr: attr, ValueId: charlie, Op: pb.DirectedEdge_SET,
	})

	// Delete alice → charlie. For uid predicates, ValueId carries the target uid.
	applyEdges(t, 420, 421, &pb.DirectedEdge{
		Entity: alice, Attr: attr, ValueId: charlie, Op: pb.DirectedEdge_DEL,
	})

	const readTs = uint64(422)

	require.Empty(t, forwardUids(t, attr, alice, readTs),
		"alice's best_friend must be empty after DEL")
	require.NotContains(t, reverseUids(t, attr, charlie, readTs), alice,
		"charlie's reverse must not contain alice after DEL")
}

// ---------------------------------------------------------------------------
// Legacy-path mirrors — same scenarios as above but via runMutation.
// These establish a baseline: they must all pass both before and after the
// pipeline fix.  Any divergence between Legacy* and non-Legacy* results
// points to a pipeline-specific regression.
// ---------------------------------------------------------------------------

func TestLegacySingleUidReverseUpdate(t *testing.T) {
	require.NoError(t, pstore.DropAll())
	MemLayerInstance.clear()

	require.NoError(t, schema.ParseBytes([]byte(`best_friend: uid @reverse .`), 1))

	attr := x.AttrInRootNamespace("best_friend")
	const (
		alice   = uint64(10)
		bob     = uint64(20)
		charlie = uint64(30)
	)

	applyEdgesLegacy(t, 510, 511, &pb.DirectedEdge{
		Entity: alice, Attr: attr, ValueId: charlie, Op: pb.DirectedEdge_SET,
	})
	applyEdgesLegacy(t, 520, 521, &pb.DirectedEdge{
		Entity: alice, Attr: attr, ValueId: bob, Op: pb.DirectedEdge_SET,
	})

	const readTs = uint64(522)

	require.Equal(t, []uint64{bob}, forwardUids(t, attr, alice, readTs),
		"legacy: alice's best_friend must be updated to bob")
	require.Contains(t, reverseUids(t, attr, bob, readTs), alice,
		"legacy: bob's reverse index must now contain alice")
	require.NotContains(t, reverseUids(t, attr, charlie, readTs), alice,
		"legacy: charlie's reverse index must no longer contain alice")
}

func TestLegacySingleUidReverseUpdateChain(t *testing.T) {
	require.NoError(t, pstore.DropAll())
	MemLayerInstance.clear()

	require.NoError(t, schema.ParseBytes([]byte(`best_friend: uid @reverse .`), 1))

	attr := x.AttrInRootNamespace("best_friend")
	const (
		alice   = uint64(10)
		bob     = uint64(20)
		charlie = uint64(30)
		dave    = uint64(40)
	)

	applyEdgesLegacy(t, 610, 611, &pb.DirectedEdge{
		Entity: alice, Attr: attr, ValueId: charlie, Op: pb.DirectedEdge_SET,
	})
	applyEdgesLegacy(t, 620, 621, &pb.DirectedEdge{
		Entity: alice, Attr: attr, ValueId: bob, Op: pb.DirectedEdge_SET,
	})

	const afterBob = uint64(622)
	require.Equal(t, []uint64{bob}, forwardUids(t, attr, alice, afterBob))
	require.Contains(t, reverseUids(t, attr, bob, afterBob), alice)
	require.NotContains(t, reverseUids(t, attr, charlie, afterBob), alice)

	applyEdgesLegacy(t, 630, 631, &pb.DirectedEdge{
		Entity: alice, Attr: attr, ValueId: dave, Op: pb.DirectedEdge_SET,
	})

	const afterDave = uint64(632)
	require.Equal(t, []uint64{dave}, forwardUids(t, attr, alice, afterDave),
		"legacy: alice's best_friend must be dave after second replacement")
	require.Contains(t, reverseUids(t, attr, dave, afterDave), alice,
		"legacy: dave's reverse must contain alice")
	require.NotContains(t, reverseUids(t, attr, bob, afterDave), alice,
		"legacy: bob's reverse must no longer contain alice")
	require.NotContains(t, reverseUids(t, attr, charlie, afterDave), alice,
		"legacy: charlie's reverse must still not contain alice")
}

func TestLegacySingleUidReverseSetSameValue(t *testing.T) {
	require.NoError(t, pstore.DropAll())
	MemLayerInstance.clear()

	require.NoError(t, schema.ParseBytes([]byte(`best_friend: uid @reverse .`), 1))

	attr := x.AttrInRootNamespace("best_friend")
	const (
		alice   = uint64(10)
		charlie = uint64(30)
	)

	applyEdgesLegacy(t, 710, 711, &pb.DirectedEdge{
		Entity: alice, Attr: attr, ValueId: charlie, Op: pb.DirectedEdge_SET,
	})
	applyEdgesLegacy(t, 720, 721, &pb.DirectedEdge{
		Entity: alice, Attr: attr, ValueId: charlie, Op: pb.DirectedEdge_SET,
	})

	const readTs = uint64(722)

	require.Equal(t, []uint64{charlie}, forwardUids(t, attr, alice, readTs),
		"legacy: alice's best_friend must still be charlie after no-op re-set")
	require.Contains(t, reverseUids(t, attr, charlie, readTs), alice,
		"legacy: charlie's reverse must still contain alice after no-op re-set")
}

func TestLegacySingleUidReverseDelete(t *testing.T) {
	require.NoError(t, pstore.DropAll())
	MemLayerInstance.clear()

	require.NoError(t, schema.ParseBytes([]byte(`best_friend: uid @reverse .`), 1))

	attr := x.AttrInRootNamespace("best_friend")
	const (
		alice   = uint64(10)
		charlie = uint64(30)
	)

	applyEdgesLegacy(t, 810, 811, &pb.DirectedEdge{
		Entity: alice, Attr: attr, ValueId: charlie, Op: pb.DirectedEdge_SET,
	})
	applyEdgesLegacy(t, 820, 821, &pb.DirectedEdge{
		Entity: alice, Attr: attr, ValueId: charlie, Op: pb.DirectedEdge_DEL,
	})

	const readTs = uint64(822)

	require.Empty(t, forwardUids(t, attr, alice, readTs),
		"legacy: alice's best_friend must be empty after DEL")
	require.NotContains(t, reverseUids(t, attr, charlie, readTs), alice,
		"legacy: charlie's reverse must not contain alice after DEL")
}

// ---------------------------------------------------------------------------
// Language predicate @index stale entry tests
// ---------------------------------------------------------------------------

// indexUidsForLang returns entity UIDs found in the exact index for the given
// language-tagged value. attr must already be namespace-qualified.
func indexUidsForLang(t *testing.T, attr, lang, val string, readTs uint64) []uint64 {
	t.Helper()
	tokens, err := indexTokens(context.Background(), &indexMutationInfo{
		tokenizers: schema.State().Tokenizer(context.Background(), attr),
		edge:       &pb.DirectedEdge{Attr: attr, Lang: lang},
		val:        types.Val{Tid: types.StringID, Value: []byte(val)},
	})
	require.NoError(t, err)
	var result []uint64
	for _, token := range tokens {
		l, err := GetNoStore(x.IndexKey(attr, token), readTs)
		require.NoError(t, err)
		result = append(result, uids(l, readTs)...)
	}
	return result
}

// TestMutationPipelineLangIndexStaleEntry is the regression test for the
// ProcessList language-predicate stale index-entry bug.
//
// Root cause: ProcessList accumulates {SET "Alicia"} and passes it directly to
// InsertTokenizerIndexes, which emits only the SET index entry for the new
// value. It never reads the committed data key, so it never generates a DEL
// index entry for the old "Alice" — that entry persists in the index.
//
// The legacy path (AddMutationWithIndex → addMutationHelper) correctly calls
// findPosting to retrieve the old value then emits both:
//
//	addIndexMutations(DEL, "Alice") and addIndexMutations(SET, "Alicia")
//
// Expected (correct) behaviour:
//   - index("Alicia") contains alice.
//   - index("Alice") does NOT contain alice.
//
// Observed (buggy) behaviour without the fix:
//   - index("Alice") still contains alice (stale entry).
func TestMutationPipelineLangIndexStaleEntry(t *testing.T) {
	require.NoError(t, pstore.DropAll())
	MemLayerInstance.clear()

	require.NoError(t, schema.ParseBytes([]byte(`name: string @lang @index(exact) .`), 1))

	attr := x.AttrInRootNamespace("name")
	const alice = uint64(10)

	// Step 1 — initial state: alice's name@en = "Alice".
	applyEdges(t, 910, 911, &pb.DirectedEdge{
		Entity: alice, Attr: attr,
		Value: []byte("Alice"), ValueType: pb.Posting_STRING,
		Lang: "en", Op: pb.DirectedEdge_SET,
	})

	require.Contains(t, indexUidsForLang(t, attr, "en", "Alice", 912), alice,
		"initial: index for 'Alice' must contain alice")
	require.NotContains(t, indexUidsForLang(t, attr, "en", "Alicia", 912), alice,
		"initial: index for 'Alicia' must not contain alice")

	// Step 2 — update: SET alice's name@en = "Alicia" (replaces "Alice").
	applyEdges(t, 920, 921, &pb.DirectedEdge{
		Entity: alice, Attr: attr,
		Value: []byte("Alicia"), ValueType: pb.Posting_STRING,
		Lang: "en", Op: pb.DirectedEdge_SET,
	})

	const readTs = uint64(922)

	require.Contains(t, indexUidsForLang(t, attr, "en", "Alicia", readTs), alice,
		"index for 'Alicia' must contain alice after update")
	require.NotContains(t, indexUidsForLang(t, attr, "en", "Alice", readTs), alice,
		"index for 'Alice' must not contain alice after update — stale entry bug")
}

// TestLegacyLangIndexStaleEntry mirrors TestMutationPipelineLangIndexStaleEntry
// via the legacy runMutation path to establish that the legacy path is correct
// and the stale entry issue is pipeline-specific.
func TestLegacyLangIndexStaleEntry(t *testing.T) {
	require.NoError(t, pstore.DropAll())
	MemLayerInstance.clear()

	require.NoError(t, schema.ParseBytes([]byte(`name: string @lang @index(exact) .`), 1))

	attr := x.AttrInRootNamespace("name")
	const alice = uint64(10)

	applyEdgesLegacy(t, 1010, 1011, &pb.DirectedEdge{
		Entity: alice, Attr: attr,
		Value: []byte("Alice"), ValueType: pb.Posting_STRING,
		Lang: "en", Op: pb.DirectedEdge_SET,
	})
	applyEdgesLegacy(t, 1020, 1021, &pb.DirectedEdge{
		Entity: alice, Attr: attr,
		Value: []byte("Alicia"), ValueType: pb.Posting_STRING,
		Lang: "en", Op: pb.DirectedEdge_SET,
	})

	const readTs = uint64(1022)

	require.Contains(t, indexUidsForLang(t, attr, "en", "Alicia", readTs), alice,
		"legacy: index for 'Alicia' must contain alice after update")
	require.NotContains(t, indexUidsForLang(t, attr, "en", "Alice", readTs), alice,
		"legacy: index for 'Alice' must not contain alice after update")
}

// ---------------------------------------------------------------------------
// Star-delete + same-batch SET tests (Bug 4)
// ---------------------------------------------------------------------------

// TestMutationPipelineStarDeleteThenSet is the regression test for the
// deleteAll-marker overwrite bug in MutationPipeline.
//
// Root cause: Process() handles a star-delete by calling l.handleDeleteAll on
// the List object in cache.plists, which sets deleteAllMarker and
// currentEntries=[{DeleteAll}]. Subsequent ProcessList processing for the same
// key calls AddDelta, which calls list.setCurrentEntries — unconditionally
// resetting deleteAllMarker to math.MaxUint64 and discarding the deleteAll.
// The committed friends (bob, charlie) are therefore NOT cleared.
//
// Expected (correct) behaviour:
//   - alice's friends = [dave] only.
//   - bob's and charlie's reverse edges are gone.
//   - dave's reverse edge is present.
//
// Observed (buggy) behaviour without the fix:
//   - alice's friends = [bob, charlie, dave] (old friends not deleted).
func TestMutationPipelineStarDeleteThenSet(t *testing.T) {
	require.NoError(t, pstore.DropAll())
	MemLayerInstance.clear()

	require.NoError(t, schema.ParseBytes([]byte(`friend: [uid] @reverse .`), 1))

	attr := x.AttrInRootNamespace("friend")
	const (
		alice   = uint64(10)
		bob     = uint64(20)
		charlie = uint64(30)
		dave    = uint64(40)
	)

	// Step 1 — initial state: alice has friends bob and charlie.
	applyEdges(t, 1110, 1111,
		&pb.DirectedEdge{Entity: alice, Attr: attr, ValueId: bob, Op: pb.DirectedEdge_SET},
		&pb.DirectedEdge{Entity: alice, Attr: attr, ValueId: charlie, Op: pb.DirectedEdge_SET},
	)

	require.Len(t, forwardUids(t, attr, alice, 1112), 2,
		"initial: alice should have two friends")

	// Step 2 — batch: star-delete alice's friends, then add dave.
	applyEdges(t, 1120, 1121,
		&pb.DirectedEdge{Entity: alice, Attr: attr, Value: []byte(x.Star), Op: pb.DirectedEdge_DEL},
		&pb.DirectedEdge{Entity: alice, Attr: attr, ValueId: dave, Op: pb.DirectedEdge_SET},
	)

	const readTs = uint64(1122)
	require.Equal(t, []uint64{dave}, forwardUids(t, attr, alice, readTs),
		"alice's friends must be only dave after star-delete + set")
	require.Contains(t, reverseUids(t, attr, dave, readTs), alice,
		"dave's reverse must contain alice")
	require.NotContains(t, reverseUids(t, attr, bob, readTs), alice,
		"bob's reverse must not contain alice after star-delete")
	require.NotContains(t, reverseUids(t, attr, charlie, readTs), alice,
		"charlie's reverse must not contain alice after star-delete")
}

// TestLegacyStarDeleteThenSet mirrors TestMutationPipelineStarDeleteThenSet via
// the legacy runMutation path to establish that the legacy path is correct.
func TestLegacyStarDeleteThenSet(t *testing.T) {
	require.NoError(t, pstore.DropAll())
	MemLayerInstance.clear()

	require.NoError(t, schema.ParseBytes([]byte(`friend: [uid] @reverse .`), 1))

	attr := x.AttrInRootNamespace("friend")
	const (
		alice   = uint64(10)
		bob     = uint64(20)
		charlie = uint64(30)
		dave    = uint64(40)
	)

	applyEdgesLegacy(t, 1210, 1211,
		&pb.DirectedEdge{Entity: alice, Attr: attr, ValueId: bob, Op: pb.DirectedEdge_SET},
		&pb.DirectedEdge{Entity: alice, Attr: attr, ValueId: charlie, Op: pb.DirectedEdge_SET},
	)

	applyEdgesLegacy(t, 1220, 1221,
		&pb.DirectedEdge{Entity: alice, Attr: attr, Value: []byte(x.Star), Op: pb.DirectedEdge_DEL},
		&pb.DirectedEdge{Entity: alice, Attr: attr, ValueId: dave, Op: pb.DirectedEdge_SET},
	)

	const readTs = uint64(1222)

	require.Equal(t, []uint64{dave}, forwardUids(t, attr, alice, readTs),
		"legacy: alice's friends must be only dave after star-delete + set")
	require.Contains(t, reverseUids(t, attr, dave, readTs), alice,
		"legacy: dave's reverse must contain alice")
	require.NotContains(t, reverseUids(t, attr, bob, readTs), alice,
		"legacy: bob's reverse must not contain alice after star-delete")
	require.NotContains(t, reverseUids(t, attr, charlie, readTs), alice,
		"legacy: charlie's reverse must not contain alice after star-delete")
}

// ---------------------------------------------------------------------------
// ProcessSingle: SET + DEL different targets in same batch (Bug 5)
// ---------------------------------------------------------------------------

// TestMutationPipelineSingleUidSetAndDelDifferentTargets is the regression test
// for the nil==nil value-comparison bug in ProcessSingle's exists=true DEL branch.
//
// Root cause: when a SET alice→bob is already accumulated for alice, a subsequent
// DEL alice→charlie in the same batch enters the `exists=true, edge.Op==DEL` branch.
// The check `string(edge.Value) == string(oldVal.Value)` compares nil==nil (both
// Value fields are nil for uid postings — the uid lives in posting.Uid), which is
// always true. setPosting() therefore REPLACES the accumulated {SET bob} with
// {DEL charlie}. The SET is lost.
//
// Expected (correct) behaviour after the fix:
//   - alice's best_friend is bob.
//   - bob's reverse index contains alice.
//   - charlie's reverse index does NOT contain alice.
//
// Observed (buggy) behaviour without the fix:
//   - alice has no best_friend (the SET bob was replaced by DEL charlie).
//   - bob's reverse index is empty.
func TestMutationPipelineSingleUidSetAndDelDifferentTargets(t *testing.T) {
	require.NoError(t, pstore.DropAll())
	MemLayerInstance.clear()

	require.NoError(t, schema.ParseBytes([]byte(`best_friend: uid @reverse .`), 1))

	attr := x.AttrInRootNamespace("best_friend")
	const (
		alice   = uint64(10)
		bob     = uint64(20)
		charlie = uint64(30)
	)

	// Step 1 — initial state: alice → best_friend → charlie.
	applyEdges(t, 1310, 1311, &pb.DirectedEdge{
		Entity: alice, Attr: attr, ValueId: charlie, Op: pb.DirectedEdge_SET,
	})

	// Step 2 — batch: SET alice→bob first, then DEL alice→charlie.
	// The SET and DEL target different uids (bob ≠ charlie), triggering Bug 5.
	applyEdges(t, 1320, 1321,
		&pb.DirectedEdge{Entity: alice, Attr: attr, ValueId: bob, Op: pb.DirectedEdge_SET},
		&pb.DirectedEdge{Entity: alice, Attr: attr, ValueId: charlie, Op: pb.DirectedEdge_DEL},
	)

	const readTs = uint64(1322)

	require.Equal(t, []uint64{bob}, forwardUids(t, attr, alice, readTs),
		"alice's best_friend must be bob after set-bob + del-charlie batch")
	require.Contains(t, reverseUids(t, attr, bob, readTs), alice,
		"bob's reverse must contain alice")
	require.NotContains(t, reverseUids(t, attr, charlie, readTs), alice,
		"charlie's reverse must not contain alice after del")
}

// TestLegacySingleUidSetAndDelDifferentTargets mirrors
// TestMutationPipelineSingleUidSetAndDelDifferentTargets via the legacy path.
func TestLegacySingleUidSetAndDelDifferentTargets(t *testing.T) {
	require.NoError(t, pstore.DropAll())
	MemLayerInstance.clear()

	require.NoError(t, schema.ParseBytes([]byte(`best_friend: uid @reverse .`), 1))

	attr := x.AttrInRootNamespace("best_friend")
	const (
		alice   = uint64(10)
		bob     = uint64(20)
		charlie = uint64(30)
	)

	applyEdgesLegacy(t, 1410, 1411, &pb.DirectedEdge{
		Entity: alice, Attr: attr, ValueId: charlie, Op: pb.DirectedEdge_SET,
	})

	applyEdgesLegacy(t, 1420, 1421,
		&pb.DirectedEdge{Entity: alice, Attr: attr, ValueId: bob, Op: pb.DirectedEdge_SET},
		&pb.DirectedEdge{Entity: alice, Attr: attr, ValueId: charlie, Op: pb.DirectedEdge_DEL},
	)

	const readTs = uint64(1422)

	require.Equal(t, []uint64{bob}, forwardUids(t, attr, alice, readTs),
		"legacy: alice's best_friend must be bob after set-bob + del-charlie batch")
	require.Contains(t, reverseUids(t, attr, bob, readTs), alice,
		"legacy: bob's reverse must contain alice")
	require.NotContains(t, reverseUids(t, attr, charlie, readTs), alice,
		"legacy: charlie's reverse must not contain alice after del")
}

// ---------------------------------------------------------------------------
// ProcessSingle: DEL wrong target uid (Bug 6)
// ---------------------------------------------------------------------------

// TestMutationPipelineSingleUidDelWrongTarget documents the expected behaviour
// when a DEL targets a uid that does NOT match the committed value.
//
// Root cause (Bug 6): in ProcessSingle's exists=false DEL branch, the comparison
// `string(oldVal.Value) == string(edge.Value)` is nil==nil for uid postings, so
// setPosting() is always called regardless of whether the DEL target uid
// (edge.ValueId) matches the committed uid (oldVal.Uid). This creates a spurious
// {DEL wrong_uid} delta. The delta has no visible effect on forward data or the
// reverse index (because the wrong uid was never stored for that entity), so the
// observable state is identical to the correct no-op.
//
// Both pipeline and legacy should pass: alice retains bob, bob's reverse keeps
// alice, and charlie's reverse (which never contained alice) is unchanged.
func TestMutationPipelineSingleUidDelWrongTarget(t *testing.T) {
	require.NoError(t, pstore.DropAll())
	MemLayerInstance.clear()

	require.NoError(t, schema.ParseBytes([]byte(`best_friend: uid @reverse .`), 1))

	attr := x.AttrInRootNamespace("best_friend")
	const (
		alice   = uint64(10)
		bob     = uint64(20)
		charlie = uint64(30)
	)

	// Step 1 — initial state: alice → best_friend → bob.
	applyEdges(t, 1510, 1511, &pb.DirectedEdge{
		Entity: alice, Attr: attr, ValueId: bob, Op: pb.DirectedEdge_SET,
	})

	// Step 2 — DEL alice→charlie (charlie is the wrong target; alice has bob).
	// This DEL is a no-op: alice does not have charlie as best_friend.
	applyEdges(t, 1520, 1521, &pb.DirectedEdge{
		Entity: alice, Attr: attr, ValueId: charlie, Op: pb.DirectedEdge_DEL,
	})

	const readTs = uint64(1522)

	require.Equal(t, []uint64{bob}, forwardUids(t, attr, alice, readTs),
		"alice's best_friend must still be bob — DEL wrong target is a no-op")
	require.Contains(t, reverseUids(t, attr, bob, readTs), alice,
		"bob's reverse must still contain alice")
	require.NotContains(t, reverseUids(t, attr, charlie, readTs), alice,
		"charlie's reverse must not contain alice — no spurious reverse entry")
}

// TestLegacySingleUidDelWrongTarget mirrors TestMutationPipelineSingleUidDelWrongTarget
// via the legacy path to confirm both paths treat DEL-wrong-target as a no-op.
func TestLegacySingleUidDelWrongTarget(t *testing.T) {
	require.NoError(t, pstore.DropAll())
	MemLayerInstance.clear()

	require.NoError(t, schema.ParseBytes([]byte(`best_friend: uid @reverse .`), 1))

	attr := x.AttrInRootNamespace("best_friend")
	const (
		alice   = uint64(10)
		bob     = uint64(20)
		charlie = uint64(30)
	)

	applyEdgesLegacy(t, 1610, 1611, &pb.DirectedEdge{
		Entity: alice, Attr: attr, ValueId: bob, Op: pb.DirectedEdge_SET,
	})

	applyEdgesLegacy(t, 1620, 1621, &pb.DirectedEdge{
		Entity: alice, Attr: attr, ValueId: charlie, Op: pb.DirectedEdge_DEL,
	})

	const readTs = uint64(1622)

	require.Equal(t, []uint64{bob}, forwardUids(t, attr, alice, readTs),
		"legacy: alice's best_friend must still be bob — DEL wrong target is a no-op")
	require.Contains(t, reverseUids(t, attr, bob, readTs), alice,
		"legacy: bob's reverse must still contain alice")
	require.NotContains(t, reverseUids(t, attr, charlie, readTs), alice,
		"legacy: charlie's reverse must not contain alice")
}

// ---------------------------------------------------------------------------
// ProcessList lang: SET new then DEL old in same batch (Scenario 4)
// ---------------------------------------------------------------------------

// TestMutationPipelineLangSetNewThenDelOldSameBatch is the regression test for
// the index-loss bug when a lang predicate batch contains SET new-value followed
// by DEL old-value for the same language tag.
//
// Root cause: ProcessList accumulates postings in a MutableLayer keyed by
// FingerprintEdge(lang).  Both SET "Alicia"@en and DEL "Alice"@en share the
// same fingerprint (fingerprint of "en").  insertPosting stores them under the
// same uid slot and the second write (DEL "Alice") overwrites the first (SET
// "Alicia").  InsertTokenizerIndexes then only sees {DEL "Alice"} — it emits the
// removal of the old index entry but never emits the SET entry for the new value.
//
// The legacy path (addMutationHelper per edge) generates index mutations at
// edge-processing time, BEFORE the same-uid overwrite affects the mutation layer.
// So legacy produces the correct index even though the forward scalar value is
// also corrupted by the overwrite (no value stored for alice's name@en — a
// separate bug shared by both paths).
//
// Expected (correct) behaviour:
//   - index("Alicia") contains alice.
//   - index("Alice") does NOT contain alice.
//
// Observed (buggy) behaviour without the fix:
//   - index("Alicia") does NOT contain alice (SET entry was lost).
//   - index("Alice") also no longer contains alice (DEL was applied correctly).
func TestMutationPipelineLangSetNewThenDelOldSameBatch(t *testing.T) {
	require.NoError(t, pstore.DropAll())
	MemLayerInstance.clear()

	require.NoError(t, schema.ParseBytes([]byte(`name: string @lang @index(exact) .`), 1))

	attr := x.AttrInRootNamespace("name")
	const alice = uint64(10)

	// Step 1 — initial state: alice's name@en = "Alice".
	applyEdges(t, 1710, 1711, &pb.DirectedEdge{
		Entity: alice, Attr: attr,
		Value: []byte("Alice"), ValueType: pb.Posting_STRING,
		Lang: "en", Op: pb.DirectedEdge_SET,
	})

	require.Contains(t, indexUidsForLang(t, attr, "en", "Alice", 1712), alice,
		"initial: index for 'Alice' must contain alice")

	// Step 2 — batch: SET "Alicia"@en first, then DEL "Alice"@en.
	// Both share fingerprint("en") → DEL overwrites SET in the MutableLayer.
	applyEdges(t, 1720, 1721,
		&pb.DirectedEdge{
			Entity: alice, Attr: attr,
			Value: []byte("Alicia"), ValueType: pb.Posting_STRING,
			Lang: "en", Op: pb.DirectedEdge_SET,
		},
		&pb.DirectedEdge{
			Entity: alice, Attr: attr,
			Value: []byte("Alice"), ValueType: pb.Posting_STRING,
			Lang: "en", Op: pb.DirectedEdge_DEL,
		},
	)

	const readTs = uint64(1722)

	require.Contains(t, indexUidsForLang(t, attr, "en", "Alicia", readTs), alice,
		"index for 'Alicia' must contain alice after set-new + del-old batch")
	require.NotContains(t, indexUidsForLang(t, attr, "en", "Alice", readTs), alice,
		"index for 'Alice' must not contain alice after del-old")
}

// TestLegacyLangSetNewThenDelOldSameBatch mirrors Scenario 4 via the legacy path.
// Legacy generates index mutations at edge-processing time, so the SET "Alicia"
// index entry is created before the same-uid overwrite discards the SET posting.
func TestLegacyLangSetNewThenDelOldSameBatch(t *testing.T) {
	require.NoError(t, pstore.DropAll())
	MemLayerInstance.clear()

	require.NoError(t, schema.ParseBytes([]byte(`name: string @lang @index(exact) .`), 1))

	attr := x.AttrInRootNamespace("name")
	const alice = uint64(10)

	applyEdgesLegacy(t, 1810, 1811, &pb.DirectedEdge{
		Entity: alice, Attr: attr,
		Value: []byte("Alice"), ValueType: pb.Posting_STRING,
		Lang: "en", Op: pb.DirectedEdge_SET,
	})

	applyEdgesLegacy(t, 1820, 1821,
		&pb.DirectedEdge{
			Entity: alice, Attr: attr,
			Value: []byte("Alicia"), ValueType: pb.Posting_STRING,
			Lang: "en", Op: pb.DirectedEdge_SET,
		},
		&pb.DirectedEdge{
			Entity: alice, Attr: attr,
			Value: []byte("Alice"), ValueType: pb.Posting_STRING,
			Lang: "en", Op: pb.DirectedEdge_DEL,
		},
	)

	const readTs = uint64(1822)

	require.Contains(t, indexUidsForLang(t, attr, "en", "Alicia", readTs), alice,
		"legacy: index for 'Alicia' must contain alice after set-new + del-old batch")
	require.NotContains(t, indexUidsForLang(t, attr, "en", "Alice", readTs), alice,
		"legacy: index for 'Alice' must not contain alice after del-old")
}

// ---------------------------------------------------------------------------
// Multiple SETs same uid same batch — scalar @index (Scenario 5)
// ---------------------------------------------------------------------------

// TestMutationPipelineMultipleSetsSameUidScalarIndex verifies that when a batch
// contains two SET edges for the same entity on a scalar @index predicate, only
// the LAST value ends up in the index (last-SET-wins), and the intermediate
// value never appears in the index.
//
// This scenario is NOT pipeline-specific; both pipeline and legacy apply
// last-SET-wins semantics.  The test exists to confirm that no intermediate index
// entry is created for the first SET value.
//
// Setup: alice has nickname = "v1" committed.
// Batch:  SET alice nickname = "v1"   (same as committed — redundant)
//
//	SET alice nickname = "v2"   (the desired new value)
//
// Expected:
//   - index("v2") contains alice.
//   - index("v1") does NOT contain alice (removed by handleOldDeleteForSingle's
//     synthetic DEL for the committed value; the intermediate SET v1 was absorbed
//     into SET v2 by last-SET-wins accumulation, so no redundant v1 index entry).
func TestMutationPipelineMultipleSetsSameUidScalarIndex(t *testing.T) {
	require.NoError(t, pstore.DropAll())
	MemLayerInstance.clear()

	require.NoError(t, schema.ParseBytes([]byte(`nickname: string @index(exact) .`), 1))

	attr := x.AttrInRootNamespace("nickname")
	const alice = uint64(10)

	// Step 1 — commit initial value "v1".
	applyEdges(t, 1910, 1911, &pb.DirectedEdge{
		Entity: alice, Attr: attr,
		Value: []byte("v1"), ValueType: pb.Posting_STRING,
		Op: pb.DirectedEdge_SET,
	})

	require.Contains(t, indexUidsForVal(t, attr, "v1", 1912), alice,
		"initial: index('v1') must contain alice")

	// Step 2 — batch: redundant SET v1 + final SET v2.
	// ProcessSingle accumulates SET v1 then SET v2 → last-SET-wins → [{SET v2}].
	// handleOldDeleteForSingle reads committed v1, synthesises {DEL v1}.
	// InsertTokenizerIndexes: SET v2 (add) + DEL v1 (remove).
	applyEdges(t, 1920, 1921,
		&pb.DirectedEdge{
			Entity: alice, Attr: attr,
			Value: []byte("v1"), ValueType: pb.Posting_STRING,
			Op: pb.DirectedEdge_SET,
		},
		&pb.DirectedEdge{
			Entity: alice, Attr: attr,
			Value: []byte("v2"), ValueType: pb.Posting_STRING,
			Op: pb.DirectedEdge_SET,
		},
	)

	const readTs = uint64(1922)

	require.Contains(t, indexUidsForVal(t, attr, "v2", readTs), alice,
		"index('v2') must contain alice after last-SET-wins")
	require.NotContains(t, indexUidsForVal(t, attr, "v1", readTs), alice,
		"index('v1') must not contain alice — old value removed by synthetic DEL")
}

// TestLegacyMultipleSetsSameUidScalarIndex mirrors Scenario 5 via the legacy path.
func TestLegacyMultipleSetsSameUidScalarIndex(t *testing.T) {
	require.NoError(t, pstore.DropAll())
	MemLayerInstance.clear()

	require.NoError(t, schema.ParseBytes([]byte(`nickname: string @index(exact) .`), 1))

	attr := x.AttrInRootNamespace("nickname")
	const alice = uint64(10)

	applyEdgesLegacy(t, 2010, 2011, &pb.DirectedEdge{
		Entity: alice, Attr: attr,
		Value: []byte("v1"), ValueType: pb.Posting_STRING,
		Op: pb.DirectedEdge_SET,
	})

	applyEdgesLegacy(t, 2020, 2021,
		&pb.DirectedEdge{
			Entity: alice, Attr: attr,
			Value: []byte("v1"), ValueType: pb.Posting_STRING,
			Op: pb.DirectedEdge_SET,
		},
		&pb.DirectedEdge{
			Entity: alice, Attr: attr,
			Value: []byte("v2"), ValueType: pb.Posting_STRING,
			Op: pb.DirectedEdge_SET,
		},
	)

	const readTs = uint64(2022)

	require.Contains(t, indexUidsForVal(t, attr, "v2", readTs), alice,
		"legacy: index('v2') must contain alice after last-SET-wins")
	require.NotContains(t, indexUidsForVal(t, attr, "v1", readTs), alice,
		"legacy: index('v1') must not contain alice — old value removed")
}

// ---------------------------------------------------------------------------
// Bug 5: ProcessSingle case-insensitive tokenizer — duplicate synthetic DEL
// ---------------------------------------------------------------------------

// TestMutationPipelineCaseInsensitiveTokenizerDupDel is the regression test for the
// index-loss bug that occurs when all three of the following hold:
//
//  1. The predicate uses a case-insensitive tokenizer (e.g. "term", which applies a
//     lowercase filter so "Apple" and "APPLE" hash to the same bucket "apple").
//  2. The user explicitly sends a DEL for the old value alongside a SET for a new
//     value in the same batch.
//  3. The old and new values produce the same token after normalization.
//
// Root cause: ProcessSingle accumulates {DEL "Apple", SET "APPLE"} from the user
// edges.  handleOldDeleteForSingle then reads the committed value "Apple" and sees
// that it differs from the new value "APPLE" (byte-level comparison), so it appends
// a synthetic DEL "Apple".  The resulting PostingList has three entries:
//
//	[{DEL "Apple"}, {SET "APPLE"}, {DEL "Apple" (synthetic)}]
//
// InsertTokenizerIndexes only applies the isSingleEdge reversal when len == 2.
// With len == 3 no reversal fires, so the postings are fed to insertPosting in
// original order.  Both DEL "Apple" entries produce the same index key ("apple")
// and the same entity uid.  The three insertPosting calls leave currentEntries[alice]
// as DEL — so alice is removed from the "apple" bucket even though her tag is now
// "APPLE" (which tokenises to "apple").
//
// Expected (correct) behaviour:
//   - index("APPLE") — i.e. the "apple" bucket — still contains alice after the
//     batch, because her new value "APPLE" maps to that bucket.
//
// Observed (buggy) behaviour without the fix:
//   - index("APPLE") does NOT contain alice — she was incorrectly removed.
func TestMutationPipelineCaseInsensitiveTokenizerDupDel(t *testing.T) {
	require.NoError(t, pstore.DropAll())
	MemLayerInstance.clear()

	require.NoError(t, schema.ParseBytes([]byte(`casetag: string @index(term) .`), 1))

	attr := x.AttrInRootNamespace("casetag")
	const alice = uint64(10)

	// Step 1 — commit initial value "Apple".  Term-tokenises to "apple".
	applyEdges(t, 2110, 2111, &pb.DirectedEdge{
		Entity: alice, Attr: attr,
		Value: []byte("Apple"), ValueType: pb.Posting_STRING,
		Op: pb.DirectedEdge_SET,
	})

	require.Contains(t, indexUidsForVal(t, attr, "Apple", 2112), alice,
		"initial: term index for 'Apple' must contain alice")

	// Step 2 — batch: explicit DEL "Apple" + SET "APPLE".
	// "Apple" and "APPLE" share the term token "apple" (lowercase filter).
	// handleOldDeleteForSingle appends a synthetic DEL "Apple", creating 3 postings;
	// InsertTokenizerIndexes does not apply the len==2 reversal — final index = DEL.
	applyEdges(t, 2120, 2121,
		&pb.DirectedEdge{
			Entity: alice, Attr: attr,
			Value: []byte("Apple"), ValueType: pb.Posting_STRING,
			Op: pb.DirectedEdge_DEL,
		},
		&pb.DirectedEdge{
			Entity: alice, Attr: attr,
			Value: []byte("APPLE"), ValueType: pb.Posting_STRING,
			Op: pb.DirectedEdge_SET,
		},
	)

	const readTs = uint64(2122)

	// "APPLE" term-tokenises to "apple"; alice must be in the "apple" bucket because
	// her tag is now "APPLE".
	require.Contains(t, indexUidsForVal(t, attr, "APPLE", readTs), alice,
		"index for 'APPLE' (term bucket 'apple') must contain alice after explicit-del + set batch")
}

// TestLegacyCaseInsensitiveTokenizerDupDel mirrors Bug 5 via the legacy path to
// confirm that legacy handles explicit-DEL + SET with a shared case-insensitive
// token correctly.  Legacy processes each edge independently so the SET index
// entry is generated after the DEL, leaving alice in the "apple" bucket.
func TestLegacyCaseInsensitiveTokenizerDupDel(t *testing.T) {
	require.NoError(t, pstore.DropAll())
	MemLayerInstance.clear()

	require.NoError(t, schema.ParseBytes([]byte(`casetag: string @index(term) .`), 1))

	attr := x.AttrInRootNamespace("casetag")
	const alice = uint64(10)

	applyEdgesLegacy(t, 2210, 2211, &pb.DirectedEdge{
		Entity: alice, Attr: attr,
		Value: []byte("Apple"), ValueType: pb.Posting_STRING,
		Op: pb.DirectedEdge_SET,
	})

	applyEdgesLegacy(t, 2220, 2221,
		&pb.DirectedEdge{
			Entity: alice, Attr: attr,
			Value: []byte("Apple"), ValueType: pb.Posting_STRING,
			Op: pb.DirectedEdge_DEL,
		},
		&pb.DirectedEdge{
			Entity: alice, Attr: attr,
			Value: []byte("APPLE"), ValueType: pb.Posting_STRING,
			Op: pb.DirectedEdge_SET,
		},
	)

	const readTs = uint64(2222)

	require.Contains(t, indexUidsForVal(t, attr, "APPLE", readTs), alice,
		"legacy: index for 'APPLE' (term bucket 'apple') must contain alice after explicit-del + set batch")
}

// ---------------------------------------------------------------------------
// Issue 7: Process() channel send deadlock on goroutine error
// ---------------------------------------------------------------------------

// TestMutationPipelineChannelDeadlock detects the deadlock that arises when a
// predicate's ProcessPredicate goroutine exits with an error before it has
// drained its edges channel, AND the main Process() loop has already filled that
// channel to capacity with remaining edges.
//
// Root cause: the edges channel has a capacity of 1000.  Process() sends edges
// with a bare `pred.edges <- edge` — there is no `select` on egCtx.Done().
// If the goroutine fails on the first edge (schema-not-found) it exits without
// consuming the 999 buffered items.  The main loop then tries to send the 1001st
// edge into a full channel whose reader has gone — it blocks forever.
//
// Fix: replace `pred.edges <- edge` with:
//
//	select {
//	case pred.edges <- edge:
//	case <-egCtx.Done():
//	    return egCtx.Err()
//	}
//
// This test sends exactly channelCap+2 = 1002 SET edges for a predicate with no
// registered schema.  Sequence:
//  1. Main loop fills the 1000-slot buffer (edges 0-999), then blocks on edge 1000.
//  2. The goroutine reads edge 0 (freeing one slot), main loop sends edge 1000.
//  3. The goroutine calls runMutation(edge 0), fails (no schema), exits — channel
//     still holds 1000 items.
//  4. Main loop tries to send edge 1001 into the full, unread channel → blocks
//     forever (deadlock).
//
// With the fix, egCtx.Done() fires and Process() returns an error within
// milliseconds.  Without the fix, the test blocks until the 3-second timeout.
//
// Note: channelCap+1 = 1001 edges is NOT sufficient — the goroutine reading one
// edge gives the main loop exactly one free slot to push its last edge, so the
// loop completes without ever stalling.  The +2 guarantees one edge remains
// unsendable after the goroutine exits.
func TestMutationPipelineChannelDeadlock(t *testing.T) {
	require.NoError(t, pstore.DropAll())
	MemLayerInstance.clear()

	// No schema registered — ProcessPredicate falls through to the legacy drain
	// loop, which calls runMutation and fails on the first SET edge.
	attr := x.AttrInRootNamespace("no_schema_deadlock_pred")

	const channelCap = 1000
	edges := make([]*pb.DirectedEdge, channelCap+2)
	for i := range edges {
		edges[i] = &pb.DirectedEdge{
			Entity:    uint64(i + 1),
			Attr:      attr,
			Value:     []byte("v"),
			ValueType: pb.Posting_STRING,
			Op:        pb.DirectedEdge_SET,
		}
	}

	done := make(chan error, 1)
	go func() {
		txn := Oracle().RegisterStartTs(2300)
		mp := NewMutationPipeline(txn)
		done <- mp.Process(context.Background(), edges)
	}()

	select {
	case err := <-done:
		// Process() must return an error (schema not found), never nil.
		require.Error(t, err,
			"Process() must return a schema-not-found error, not nil")
	case <-time.After(3 * time.Second):
		t.Fatal("Process() deadlocked: channel send blocked after goroutine error — " +
			"fix by using select{case pred.edges<-edge: case <-egCtx.Done():...}")
	}
}

// ---------------------------------------------------------------------------
// Issue 9: non-matching DEL is a no-op
// ---------------------------------------------------------------------------

// TestMutationPipelineNonMatchingDelNoOp verifies that a DEL edge whose value
// does not match the committed scalar value is silently ignored — the forward
// data key and the index are left unchanged.
//
// In ProcessSingle, postings[uid] is unconditionally initialised to an empty
// PostingList before the committed-value check, so an empty delta IS written to
// Badger even for the no-op case (Issue 9 / minor inefficiency).  The important
// invariant is that the observable state — the value and index — is not altered.
//
// Expected behaviour (both pipeline and legacy):
//   - alice's tag is still "Alice".
//   - index("Alice") still contains alice.
//   - index("Blink") does not contain alice.
func TestMutationPipelineNonMatchingDelNoOp(t *testing.T) {
	require.NoError(t, pstore.DropAll())
	MemLayerInstance.clear()

	require.NoError(t, schema.ParseBytes([]byte(`tag: string @index(exact) .`), 1))

	attr := x.AttrInRootNamespace("tag")
	const alice = uint64(10)

	// Step 1 — commit initial value "Alice".
	applyEdges(t, 2410, 2411, &pb.DirectedEdge{
		Entity: alice, Attr: attr,
		Value: []byte("Alice"), ValueType: pb.Posting_STRING,
		Op: pb.DirectedEdge_SET,
	})

	require.Contains(t, indexUidsForVal(t, attr, "Alice", 2412), alice,
		"initial: index('Alice') must contain alice")

	// Step 2 — DEL with wrong value: alice has "Alice", we DEL "Blink".
	// ProcessSingle: DEL "Blink", old committed = "Alice", "Blink" != "Alice" →
	// setPosting() is NOT called → empty delta written → no change.
	applyEdges(t, 2420, 2421, &pb.DirectedEdge{
		Entity: alice, Attr: attr,
		Value: []byte("Blink"), ValueType: pb.Posting_STRING,
		Op: pb.DirectedEdge_DEL,
	})

	const readTs = uint64(2422)

	require.Contains(t, indexUidsForVal(t, attr, "Alice", readTs), alice,
		"index('Alice') must still contain alice — non-matching DEL is a no-op")
	require.NotContains(t, indexUidsForVal(t, attr, "Blink", readTs), alice,
		"index('Blink') must not contain alice — no such value was ever set")
}

// TestLegacyNonMatchingDelNoOp mirrors Issue 9 via the legacy path to confirm
// that both paths treat a non-matching DEL as a no-op.
func TestLegacyNonMatchingDelNoOp(t *testing.T) {
	require.NoError(t, pstore.DropAll())
	MemLayerInstance.clear()

	require.NoError(t, schema.ParseBytes([]byte(`tag: string @index(exact) .`), 1))

	attr := x.AttrInRootNamespace("tag")
	const alice = uint64(10)

	applyEdgesLegacy(t, 2510, 2511, &pb.DirectedEdge{
		Entity: alice, Attr: attr,
		Value: []byte("Alice"), ValueType: pb.Posting_STRING,
		Op: pb.DirectedEdge_SET,
	})

	applyEdgesLegacy(t, 2520, 2521, &pb.DirectedEdge{
		Entity: alice, Attr: attr,
		Value: []byte("Blink"), ValueType: pb.Posting_STRING,
		Op: pb.DirectedEdge_DEL,
	})

	const readTs = uint64(2522)

	require.Contains(t, indexUidsForVal(t, attr, "Alice", readTs), alice,
		"legacy: index('Alice') must still contain alice — non-matching DEL is a no-op")
	require.NotContains(t, indexUidsForVal(t, attr, "Blink", readTs), alice,
		"legacy: index('Blink') must not contain alice")
}

// ---------------------------------------------------------------------------
// Bug 10: same-value SET on already-committed scalar index entry
// ---------------------------------------------------------------------------

// TestMutationPipelineSameValueSetKeepsIndex is a baseline regression test:
// when the schema already carries @index(term) at the time of the initial SET,
// the index entry must survive a second identical SET on the same uid.
//
// This test passes both before and after the handleOldDeleteForSingle fix,
// because the initial SET already populated the index. The companion test
// TestMutationPipelineSetThenAddIndexThenResetMirrorsAlphaFailure is the one
// that reproduces the actual alpha-level TestSchemaMutationIndexRemove
// failure — that scenario requires the SET to happen *before* the @index
// alter, so the second SET is responsible for populating the index.
//
// Legacy mirror: TestLegacySameValueSetKeepsIndex.
func TestMutationPipelineSameValueSetKeepsIndex(t *testing.T) {
	require.NoError(t, pstore.DropAll())
	MemLayerInstance.clear()

	require.NoError(t, schema.ParseBytes([]byte(`name: string @index(term) .`), 1))

	attr := x.AttrInRootNamespace("name")
	const alice = uint64(10)

	applyEdges(t, 2610, 2611, &pb.DirectedEdge{
		Entity: alice, Attr: attr,
		Value: []byte("Alice"), ValueType: pb.Posting_STRING,
		Op: pb.DirectedEdge_SET,
	})

	require.Contains(t, indexUidsForVal(t, attr, "Alice", 2612), alice,
		"initial: index('Alice') must contain alice")

	applyEdges(t, 2620, 2621, &pb.DirectedEdge{
		Entity: alice, Attr: attr,
		Value: []byte("Alice"), ValueType: pb.Posting_STRING,
		Op: pb.DirectedEdge_SET,
	})

	const readTs = uint64(2622)

	require.Contains(t, indexUidsForVal(t, attr, "Alice", readTs), alice,
		"index('Alice') must still contain alice after same-value SET re-commit")
}

// TestLegacySameValueSetKeepsIndex mirrors Bug 10 via the legacy path.
func TestLegacySameValueSetKeepsIndex(t *testing.T) {
	require.NoError(t, pstore.DropAll())
	MemLayerInstance.clear()

	require.NoError(t, schema.ParseBytes([]byte(`name: string @index(term) .`), 1))

	attr := x.AttrInRootNamespace("name")
	const alice = uint64(10)

	applyEdgesLegacy(t, 2710, 2711, &pb.DirectedEdge{
		Entity: alice, Attr: attr,
		Value: []byte("Alice"), ValueType: pb.Posting_STRING,
		Op: pb.DirectedEdge_SET,
	})

	require.Contains(t, indexUidsForVal(t, attr, "Alice", 2712), alice,
		"legacy initial: index('Alice') must contain alice")

	applyEdgesLegacy(t, 2720, 2721, &pb.DirectedEdge{
		Entity: alice, Attr: attr,
		Value: []byte("Alice"), ValueType: pb.Posting_STRING,
		Op: pb.DirectedEdge_SET,
	})

	const readTs = uint64(2722)

	require.Contains(t, indexUidsForVal(t, attr, "Alice", readTs), alice,
		"legacy: index('Alice') must still contain alice after same-value SET re-commit")
}

// TestMutationPipelineSetThenAddIndexThenResetMirrorsAlphaFailure is the
// regression test for the alpha-level failure TestSchemaMutationIndexRemove.
//
// Sequence:
//  1. Schema is `name: string` (no @index).
//  2. SET <alice> name "Alice" is committed.
//  3. Schema altered to add @index(term).
//  4. SET <alice> name "Alice" again (same value, but now under @index(term)).
//  5. Query the term index for "Alice" — must contain alice.
//
// Root cause (pre-fix): handleOldDeleteForSingle in posting/index.go saw that
// the new value matched the committed value and replaced postings[uid] with
// an empty PostingList. InsertTokenizerIndexes then iterated an empty list
// for that uid and emitted no index entries, so the newly-added index never
// got the "alice" → alice entry. Legacy passes because runMutation calls
// addIndexMutations per SET edge regardless of whether the value changed.
//
// Fix: handleOldDeleteForSingle now `continue`s in the same-value branch
// without clearing postings[uid]. The SET stays in place so
// InsertTokenizerIndexes / ProcessReverse can populate the new index/reverse
// edge. Index/reverse emissions on already-present entries are idempotent.
func TestMutationPipelineSetThenAddIndexThenResetMirrorsAlphaFailure(t *testing.T) {
	require.NoError(t, pstore.DropAll())
	MemLayerInstance.clear()

	// Step 1: no-index schema initially.
	require.NoError(t, schema.ParseBytes([]byte(`name: string .`), 1))
	attr := x.AttrInRootNamespace("name")
	const alice = uint64(10)

	// Step 2: SET Alice with no index.
	applyEdges(t, 2810, 2811, &pb.DirectedEdge{
		Entity: alice, Attr: attr,
		Value: []byte("Alice"), ValueType: pb.Posting_STRING,
		Op: pb.DirectedEdge_SET,
	})

	// Step 3: alter — add term index. Simulate alpha's background reindex by
	// flipping schema then re-running a SET (the reindex on alpha rewrites the
	// index from existing data; here we replay the existing SET under the new
	// schema to drive InsertTokenizerIndexes).
	require.NoError(t, schema.ParseBytes([]byte(`name: string @index(term) .`), 1))
	applyEdges(t, 2820, 2821, &pb.DirectedEdge{
		Entity: alice, Attr: attr,
		Value: []byte("Alice"), ValueType: pb.Posting_STRING,
		Op: pb.DirectedEdge_SET,
	})

	require.Contains(t, indexUidsForVal(t, attr, "Alice", 2822), alice,
		"after reindex: index('Alice') must contain alice")

	// Step 4: SET Alice again, same value, index now exists.
	applyEdges(t, 2830, 2831, &pb.DirectedEdge{
		Entity: alice, Attr: attr,
		Value: []byte("Alice"), ValueType: pb.Posting_STRING,
		Op: pb.DirectedEdge_SET,
	})

	const readTs = uint64(2832)

	require.Contains(t, indexUidsForVal(t, attr, "Alice", readTs), alice,
		"index('Alice') must still contain alice after same-value SET on top of @index(term)")
}

// TestLegacySetThenAddIndexThenResetMirrorsAlphaFailure is the legacy mirror
// of the test above. Legacy is expected to pass because runMutation calls
// addIndexMutations for every SET regardless of whether the value changed.
func TestLegacySetThenAddIndexThenResetMirrorsAlphaFailure(t *testing.T) {
	require.NoError(t, pstore.DropAll())
	MemLayerInstance.clear()

	require.NoError(t, schema.ParseBytes([]byte(`name: string .`), 1))
	attr := x.AttrInRootNamespace("name")
	const alice = uint64(10)

	applyEdgesLegacy(t, 2910, 2911, &pb.DirectedEdge{
		Entity: alice, Attr: attr,
		Value: []byte("Alice"), ValueType: pb.Posting_STRING,
		Op: pb.DirectedEdge_SET,
	})

	require.NoError(t, schema.ParseBytes([]byte(`name: string @index(term) .`), 1))
	applyEdgesLegacy(t, 2920, 2921, &pb.DirectedEdge{
		Entity: alice, Attr: attr,
		Value: []byte("Alice"), ValueType: pb.Posting_STRING,
		Op: pb.DirectedEdge_SET,
	})

	require.Contains(t, indexUidsForVal(t, attr, "Alice", 2922), alice,
		"legacy after reindex: index('Alice') must contain alice")
}

// scalarValue returns the string value of the single scalar posting under
// (attr, entity) at readTs, or empty if there is no posting.
func scalarValue(t *testing.T, attr string, entity, readTs uint64) string {
	t.Helper()
	l, err := GetNoStore(x.DataKey(attr, entity), readTs)
	require.NoError(t, err)
	var out string
	require.NoError(t, l.Iterate(readTs, 0, func(p *pb.Posting) error {
		out = string(p.Value)
		return nil
	}))
	return out
}

// TestMutationPipelineDelWithWrongValueIsNoOp is the regression test for
// TestDeleteScalarValue at the alpha integration level.
//
// Sequence:
//  1. Schema: name: string @index(exact).
//  2. SET <alice> name "xxx" (committed).
//  3. DEL <alice> name "yyy" (wrong value — does not match committed "xxx").
//  4. The data must still read as "xxx" — a DEL whose value does not match
//     the committed value is a no-op per Dgraph semantics.
//
// Bug: ProcessSingle accumulates `postings[alice] = &pb.PostingList{}` for the
// non-matching DEL (sameTarget is false, so setPosting is not called). The
// data-write loop then calls AddDelta(dataKey, emptyPL) which clobbers the
// in-memory list's mutable layer via setCurrentEntries → the next read at
// readTs > commitTs surfaces an empty list.
func TestMutationPipelineDelWithWrongValueIsNoOp(t *testing.T) {
	require.NoError(t, pstore.DropAll())
	MemLayerInstance.clear()

	require.NoError(t, schema.ParseBytes([]byte(`name: string @index(exact) .`), 1))
	attr := x.AttrInRootNamespace("name")
	const alice = uint64(10)

	applyEdges(t, 3010, 3011, &pb.DirectedEdge{
		Entity: alice, Attr: attr,
		Value: []byte("xxx"), ValueType: pb.Posting_STRING,
		Op: pb.DirectedEdge_SET,
	})

	require.Equal(t, "xxx", scalarValue(t, attr, alice, 3012),
		"sanity: committed value should be xxx")

	applyEdges(t, 3020, 3021, &pb.DirectedEdge{
		Entity: alice, Attr: attr,
		Value: []byte("yyy"), ValueType: pb.Posting_STRING,
		Op: pb.DirectedEdge_DEL,
	})

	require.Equal(t, "xxx", scalarValue(t, attr, alice, 3022),
		"DEL with wrong value must be a no-op — name still xxx")
}

// TestLegacyDelWithWrongValueIsNoOp mirrors the above via the legacy path.
func TestLegacyDelWithWrongValueIsNoOp(t *testing.T) {
	require.NoError(t, pstore.DropAll())
	MemLayerInstance.clear()

	require.NoError(t, schema.ParseBytes([]byte(`name: string @index(exact) .`), 1))
	attr := x.AttrInRootNamespace("name")
	const alice = uint64(10)

	applyEdgesLegacy(t, 3110, 3111, &pb.DirectedEdge{
		Entity: alice, Attr: attr,
		Value: []byte("xxx"), ValueType: pb.Posting_STRING,
		Op: pb.DirectedEdge_SET,
	})

	applyEdgesLegacy(t, 3120, 3121, &pb.DirectedEdge{
		Entity: alice, Attr: attr,
		Value: []byte("yyy"), ValueType: pb.Posting_STRING,
		Op: pb.DirectedEdge_DEL,
	})

	require.Equal(t, "xxx", scalarValue(t, attr, alice, 3122),
		"legacy: DEL with wrong value must be a no-op")
}

// TestMutationPipelineDelWrongValueTwoUidsCloseMatch mirrors TestDeleteScalarValue's
// exact shape: two uids both set to "xxx", then DEL on one of them with value "yyy".
// The DEL must not delete; the SET on the other uid must remain.
func TestMutationPipelineDelWrongValueTwoUidsCloseMatch(t *testing.T) {
	require.NoError(t, pstore.DropAll())
	MemLayerInstance.clear()

	require.NoError(t, schema.ParseBytes([]byte(`name: string @index(exact) .`), 1))
	attr := x.AttrInRootNamespace("name")
	const (
		a = uint64(0x12345)
		b = uint64(0x12346)
	)

	applyEdges(t, 3210, 3211,
		&pb.DirectedEdge{Entity: a, Attr: attr, Value: []byte("xxx"), ValueType: pb.Posting_STRING, Op: pb.DirectedEdge_SET},
		&pb.DirectedEdge{Entity: b, Attr: attr, Value: []byte("xxx"), ValueType: pb.Posting_STRING, Op: pb.DirectedEdge_SET},
	)

	require.Equal(t, "xxx", scalarValue(t, attr, a, 3212), "sanity: a=xxx")
	require.Equal(t, "xxx", scalarValue(t, attr, b, 3212), "sanity: b=xxx")

	applyEdges(t, 3220, 3221,
		&pb.DirectedEdge{Entity: a, Attr: attr, Value: []byte("yyy"), ValueType: pb.Posting_STRING, Op: pb.DirectedEdge_DEL},
	)

	const readTs = uint64(3222)
	require.Equal(t, "xxx", scalarValue(t, attr, a, readTs),
		"DEL with wrong value must be a no-op — a still xxx")
	require.Equal(t, "xxx", scalarValue(t, attr, b, readTs),
		"DEL on a must not touch b")
	require.Contains(t, indexUidsForVal(t, attr, "xxx", readTs), a,
		"index('xxx') must still contain a")
	require.Contains(t, indexUidsForVal(t, attr, "xxx", readTs), b,
		"index('xxx') must still contain b")
}
