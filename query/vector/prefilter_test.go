//go:build integration

/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package query

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Pre-filtered ANN: similar_to(..., filter: <uidVar>) restricts the search to a uid
// set DURING the HNSW traversal. The data below is arranged so the globally-nearest
// vectors are out of scope (group B, clustered at the origin) while the in-scope
// vectors (group A) are farther away — so post-filtering a fixed top-k would return
// nothing, but pre-filtering returns k in-scope neighbors.

const (
	pfVec   = "pfvec"
	pfGroup = "pfgroup"
)

type pfRow struct {
	UID   string `json:"uid"`
	Group string `json:"pfgroup"`
}

func pfSetup(t *testing.T) {
	setSchema(fmt.Sprintf(`
		%s: float32vector @index(hnsw(metric: "euclidean")) .
		%s: string @index(exact) .
	`, pfVec, pfGroup))

	var sb strings.Builder
	// Use small explicit UIDs (well under the zero's default initial lease, as the
	// sibling vector tests do): higher UIDs would be rejected with
	// "Uid: [...] cannot be greater than lease".
	//
	// Group B: 8 "distractor" vectors hugging the origin (closest to a [0,0] query).
	for i := 1; i <= 8; i++ {
		uid := 0x10 + i
		fmt.Fprintf(&sb, "<%d> <%s> \"[%g, 0.0]\" .\n", uid, pfVec, float64(i)*0.01)
		fmt.Fprintf(&sb, "<%d> <%s> \"B\" .\n", uid, pfGroup)
	}
	// Group A: 8 in-scope vectors farther from the origin.
	for i := 0; i < 8; i++ {
		uid := 0x30 + i
		fmt.Fprintf(&sb, "<%d> <%s> \"[%g, 0.0]\" .\n", uid, pfVec, 0.5+float64(i)*0.1)
		fmt.Fprintf(&sb, "<%d> <%s> \"A\" .\n", uid, pfGroup)
	}
	require.NoError(t, addTriplesToCluster(sb.String()))
}

func pfTeardown() {
	dropPredicate(pfVec)
	dropPredicate(pfGroup)
}

func pfQuery(t *testing.T, query string) []pfRow {
	t.Helper()
	resp, err := client.Query(query)
	require.NoError(t, err)
	var data struct {
		Result []pfRow `json:"result"`
	}
	require.NoError(t, json.Unmarshal(resp.Json, &data))
	return data.Result
}

// TestPreFilterANN_ScopesToFilterVar is the core test: a filtered search returns k
// in-scope (group A) neighbors even though the nearest vectors are out of scope.
func TestPreFilterANN_ScopesToFilterVar(t *testing.T) {
	pfSetup(t)
	defer pfTeardown()

	// Sanity: the unfiltered top-5 are all group B (the distractors near the origin).
	unfiltered := pfQuery(t, fmt.Sprintf(`
	{
		result(func: similar_to(%s, 5, "[0.0, 0.0]")) {
			uid
			%s
		}
	}`, pfVec, pfGroup))
	require.Len(t, unfiltered, 5)
	for _, r := range unfiltered {
		require.Equal(t, "B", r.Group, "unfiltered nearest should be group B")
	}

	// Pre-filtered to group A: must return 5 group-A neighbors despite the closer
	// group-B vectors. Post-filtering the unfiltered top-5 would have yielded zero.
	filtered := pfQuery(t, fmt.Sprintf(`
	{
		allowed as var(func: eq(%s, "A"))
		result(func: similar_to(%s, 5, "[0.0, 0.0]", ef: 100, filter: allowed)) {
			uid
			%s
		}
	}`, pfGroup, pfVec, pfGroup))
	require.Len(t, filtered, 5, "pre-filter must still return k in-scope results")
	for _, r := range filtered {
		require.Equal(t, "A", r.Group, "every result must be in scope (group A)")
	}
}

// TestPreFilterANN_ScopesWithoutEfOverride is the same scope restriction as the core
// test but WITHOUT an explicit `ef:` override. A filtered search must route through
// the wider max(k, efSearch) candidate budget on its own, so it still returns k
// in-scope (group A) neighbors even though the nearest vectors are out of scope. With
// the narrow k-only budget this under-returned.
func TestPreFilterANN_ScopesWithoutEfOverride(t *testing.T) {
	pfSetup(t)
	defer pfTeardown()

	filtered := pfQuery(t, fmt.Sprintf(`
	{
		allowed as var(func: eq(%s, "A"))
		result(func: similar_to(%s, 5, "[0.0, 0.0]", filter: allowed)) {
			uid
			%s
		}
	}`, pfGroup, pfVec, pfGroup))
	require.Len(t, filtered, 5, "pre-filter must return k in-scope results without an ef override")
	for _, r := range filtered {
		require.Equal(t, "A", r.Group, "every result must be in scope (group A)")
	}
}

// TestPreFilterANN_EmptyScope: a filter var resolving to no uids yields no results.
func TestPreFilterANN_EmptyScope(t *testing.T) {
	pfSetup(t)
	defer pfTeardown()

	res := pfQuery(t, fmt.Sprintf(`
	{
		allowed as var(func: eq(%s, "NOPE"))
		result(func: similar_to(%s, 5, "[0.0, 0.0]", filter: allowed)) {
			uid
			%s
		}
	}`, pfGroup, pfVec, pfGroup))
	require.Empty(t, res, "empty scope returns no results")
}

// TestPreFilterANN_ScopeSupersetEqualsUnfiltered: when the filter admits every uid,
// the result matches the unfiltered top-k.
func TestPreFilterANN_ScopeSupersetEqualsUnfiltered(t *testing.T) {
	pfSetup(t)
	defer pfTeardown()

	unfiltered := pfQuery(t, fmt.Sprintf(`
	{
		result(func: similar_to(%s, 5, "[0.0, 0.0]")) { uid %s }
	}`, pfVec, pfGroup))

	filtered := pfQuery(t, fmt.Sprintf(`
	{
		allowed as var(func: has(%s))
		result(func: similar_to(%s, 5, "[0.0, 0.0]", ef: 100, filter: allowed)) { uid %s }
	}`, pfVec, pfVec, pfGroup))

	require.Len(t, filtered, len(unfiltered))
	got := map[string]bool{}
	for _, r := range filtered {
		got[r.UID] = true
	}
	for _, r := range unfiltered {
		require.True(t, got[r.UID], "uid %s from unfiltered top-k missing under all-admitting filter", r.UID)
	}
}

// TestPreFilterANN_WithDistanceThreshold: filter composes with distance_threshold.
// Group A sits at euclidean distance 0.5, 0.6, ..., 1.2 from the origin, so a 0.85
// threshold keeps exactly 0x30..0x33 even though k asks for 8.
func TestPreFilterANN_WithDistanceThreshold(t *testing.T) {
	pfSetup(t)
	defer pfTeardown()

	res := pfQuery(t, fmt.Sprintf(`
	{
		allowed as var(func: eq(%s, "A"))
		result(func: similar_to(%s, 8, "[0.0, 0.0]", ef: 100, distance_threshold: 0.85, filter: allowed)) {
			uid
			%s
		}
	}`, pfGroup, pfVec, pfGroup))
	require.Len(t, res, 4)
	got := map[string]bool{}
	for _, r := range res {
		require.Equal(t, "A", r.Group)
		got[r.UID] = true
	}
	for i := 0; i < 4; i++ {
		require.True(t, got[fmt.Sprintf("%#x", 0x30+i)], "uid %#x within threshold must be returned", 0x30+i)
	}
}

// TestPreFilterANN_ByUidQueryVector: the query vector may be given as a uid (the
// SearchWithUidAndOptions path). Querying by 0x30 (group A) scoped to group B must
// return only group-B neighbors.
func TestPreFilterANN_ByUidQueryVector(t *testing.T) {
	pfSetup(t)
	defer pfTeardown()

	res := pfQuery(t, fmt.Sprintf(`
	{
		allowed as var(func: eq(%s, "B"))
		result(func: similar_to(%s, 5, "0x30", ef: 100, filter: allowed)) {
			uid
			%s
		}
	}`, pfGroup, pfVec, pfGroup))
	require.Len(t, res, 5)
	for _, r := range res {
		require.Equal(t, "B", r.Group, "scope must win over the query node's own group")
	}
}

// TestPreFilterANN_ValueVarScopesByItsKeys: a value variable in filter: behaves like
// uid(var) does for a value variable — the scope is the set of uids it is keyed by.
func TestPreFilterANN_ValueVarScopesByItsKeys(t *testing.T) {
	pfSetup(t)
	defer pfTeardown()

	res := pfQuery(t, fmt.Sprintf(`
	{
		var(func: eq(%s, "A")) { g as %s }
		result(func: similar_to(%s, 5, "[0.0, 0.0]", ef: 100, filter: g)) {
			uid
			%s
		}
	}`, pfGroup, pfGroup, pfVec, pfGroup))
	require.Len(t, res, 5)
	for _, r := range res {
		require.Equal(t, "A", r.Group)
	}
}

// TestPreFilterANN_VarDefinedAfterUse: block order does not matter; the scheduler
// resolves the filter var first because it is a uid dependency of the search.
func TestPreFilterANN_VarDefinedAfterUse(t *testing.T) {
	pfSetup(t)
	defer pfTeardown()

	res := pfQuery(t, fmt.Sprintf(`
	{
		result(func: similar_to(%s, 5, "[0.0, 0.0]", ef: 100, filter: allowed)) {
			uid
			%s
		}
		allowed as var(func: eq(%s, "A"))
	}`, pfVec, pfGroup, pfGroup))
	require.Len(t, res, 5)
	for _, r := range res {
		require.Equal(t, "A", r.Group)
	}
}

// TestPreFilterANN_FilterVarReusedInFilterDirective: using the same var both as the
// search scope and in @filter(uid(...)) must not interfere with either.
func TestPreFilterANN_FilterVarReusedInFilterDirective(t *testing.T) {
	pfSetup(t)
	defer pfTeardown()

	res := pfQuery(t, fmt.Sprintf(`
	{
		allowed as var(func: eq(%s, "A"))
		result(func: similar_to(%s, 5, "[0.0, 0.0]", ef: 100, filter: allowed)) @filter(uid(allowed)) {
			uid
			%s
		}
	}`, pfGroup, pfVec, pfGroup))
	require.Len(t, res, 5)
	for _, r := range res {
		require.Equal(t, "A", r.Group)
	}
}

// TestPreFilterANN_MultipleBlocksIndependentScopes: two searches in one query each
// keep their own scope.
func TestPreFilterANN_MultipleBlocksIndependentScopes(t *testing.T) {
	pfSetup(t)
	defer pfTeardown()

	resp, err := client.Query(fmt.Sprintf(`
	{
		onlyA as var(func: eq(%[1]s, "A"))
		onlyB as var(func: eq(%[1]s, "B"))
		ra(func: similar_to(%[2]s, 3, "[0.0, 0.0]", ef: 100, filter: onlyA)) { uid %[1]s }
		rb(func: similar_to(%[2]s, 3, "[0.0, 0.0]", ef: 100, filter: onlyB)) { uid %[1]s }
	}`, pfGroup, pfVec))
	require.NoError(t, err)
	var data struct {
		Ra []pfRow `json:"ra"`
		Rb []pfRow `json:"rb"`
	}
	require.NoError(t, json.Unmarshal(resp.Json, &data))
	require.Len(t, data.Ra, 3)
	require.Len(t, data.Rb, 3)
	for _, r := range data.Ra {
		require.Equal(t, "A", r.Group)
	}
	for _, r := range data.Rb {
		require.Equal(t, "B", r.Group)
	}
}

// TestPreFilterANN_EmptyScopeWithEfOverride: an empty scope stays empty even when
// the caller widens the candidate budget.
func TestPreFilterANN_EmptyScopeWithEfOverride(t *testing.T) {
	pfSetup(t)
	defer pfTeardown()

	res := pfQuery(t, fmt.Sprintf(`
	{
		allowed as var(func: eq(%s, "NOPE"))
		result(func: similar_to(%s, 5, "[0.0, 0.0]", ef: 500, filter: allowed)) {
			uid
			%s
		}
	}`, pfGroup, pfVec, pfGroup))
	require.Empty(t, res)
}

// pfScaleSetup loads nB distractors spaced 0.001 apart from the origin and nA
// in-scope vectors interleaved among the farther distractors, using blank nodes so
// the test never depends on Zero's uid lease.
//
// Interleaving is deliberate. An in-scope cluster placed far from every distractor
// is not reachable from the HNSW entry point even by an UNFILTERED search once the
// distractors' neighbor lists are full (the back-edges to the far cluster are pruned
// at insert time; a pre-existing index-construction limitation), so such a layout
// would test graph connectivity, not the filter. Interleaved vectors are each the
// closest neighbor of the distractors beside them and therefore stay linked.
func pfScaleSetup(t *testing.T, nB, nA int) {
	setSchema(fmt.Sprintf(`
		%s: float32vector @index(hnsw(metric: "euclidean")) .
		%s: string @index(exact) .
	`, pfVec, pfGroup))

	var sb strings.Builder
	for i := 1; i <= nB; i++ {
		fmt.Fprintf(&sb, "_:b%d <%s> \"[%g, 0.0]\" .\n", i, pfVec, float64(i)*0.001)
		fmt.Fprintf(&sb, "_:b%d <%s> \"B\" .\n", i, pfGroup)
	}
	// In-scope vectors sit between distractors in the outer half of the line, so the
	// nearest nA*k vectors to the origin are all distractors.
	start := float64(nB)*0.001/2 + 0.0005
	for j := 0; j < nA; j++ {
		fmt.Fprintf(&sb, "_:a%d <%s> \"[%g, 0.0]\" .\n", j, pfVec, start+float64(j)*0.01)
		fmt.Fprintf(&sb, "_:a%d <%s> \"A\" .\n", j, pfGroup)
	}
	require.NoError(t, addTriplesToCluster(sb.String()))
}

// TestPreFilterANN_SelectiveScopeAtScale: more distractors than the default
// candidate budget (efSearch, 90 unless the schema sets it), and a scope that is a
// small minority of the index and farther from the query than a hundred
// distractors. With a budget wider than the index the search must return k in-scope
// results. With the default budget the count may legitimately fall short (the
// budget is fixed, not adaptive to selectivity; see worker/task.go), but every
// returned row must still be in scope.
func TestPreFilterANN_SelectiveScopeAtScale(t *testing.T) {
	const nB, nA = 200, 10
	pfScaleSetup(t, nB, nA)
	defer pfTeardown()

	// Precondition on the layout, not on the filter: every vector must be reachable
	// by an unfiltered search, otherwise the assertions below would blame the filter
	// for an index-connectivity problem.
	all := pfQuery(t, fmt.Sprintf(`
	{
		result(func: similar_to(%s, %d, "[0.0, 0.0]", ef: 1000)) { uid %s }
	}`, pfVec, nB+nA, pfGroup))
	require.Len(t, all, nB+nA, "test layout must keep every vector reachable from the entry point")

	unfiltered := pfQuery(t, fmt.Sprintf(`
	{
		result(func: similar_to(%s, 5, "[0.0, 0.0]")) { uid %s }
	}`, pfVec, pfGroup))
	require.Len(t, unfiltered, 5)
	for _, r := range unfiltered {
		require.Equal(t, "B", r.Group, "unfiltered nearest must be the distractors")
	}

	wide := pfQuery(t, fmt.Sprintf(`
	{
		allowed as var(func: eq(%s, "A"))
		result(func: similar_to(%s, 5, "[0.0, 0.0]", ef: 500, filter: allowed)) { uid %s }
	}`, pfGroup, pfVec, pfGroup))
	require.Len(t, wide, 5, "a budget larger than the index must reach k in-scope results")
	for _, r := range wide {
		require.Equal(t, "A", r.Group)
	}

	defaultBudget := pfQuery(t, fmt.Sprintf(`
	{
		allowed as var(func: eq(%s, "A"))
		result(func: similar_to(%s, 5, "[0.0, 0.0]", filter: allowed)) { uid %s }
	}`, pfGroup, pfVec, pfGroup))
	require.LessOrEqual(t, len(defaultBudget), 5)
	for _, r := range defaultBudget {
		require.Equal(t, "A", r.Group, "an out-of-scope row must never be returned, whatever the budget")
	}
}

// TestPreFilterANN_InsideFilterDirective: similar_to may also run inside @filter,
// where the block has source uids. The allow-set must still apply there.
func TestPreFilterANN_InsideFilterDirective(t *testing.T) {
	pfSetup(t)
	defer pfTeardown()

	// Probe whether this build accepts similar_to inside @filter at all; the
	// pre-filter option can only be exercised there if it does.
	probe := fmt.Sprintf(`
	{
		result(func: has(%s)) @filter(similar_to(%s, 3, "[0.0, 0.0]")) { uid %s }
	}`, pfVec, pfVec, pfGroup)
	if _, err := client.Query(probe); err != nil {
		t.Skipf("similar_to inside @filter is not supported on this build: %v", err)
	}

	res := pfQuery(t, fmt.Sprintf(`
	{
		allowed as var(func: eq(%s, "A"))
		result(func: has(%s)) @filter(similar_to(%s, 3, "[0.0, 0.0]", ef: 100, filter: allowed)) {
			uid
			%s
		}
	}`, pfGroup, pfVec, pfVec, pfGroup))
	require.Len(t, res, 3)
	for _, r := range res {
		require.Equal(t, "A", r.Group)
	}
}
