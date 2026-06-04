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
	// Group B: 8 "distractor" vectors hugging the origin (closest to a [0,0] query).
	for i := 1; i <= 8; i++ {
		uid := 0x4000 + i
		fmt.Fprintf(&sb, "<%d> <%s> \"[%g, 0.0]\" .\n", uid, pfVec, float64(i)*0.01)
		fmt.Fprintf(&sb, "<%d> <%s> \"B\" .\n", uid, pfGroup)
	}
	// Group A: 8 in-scope vectors farther from the origin.
	for i := 0; i < 8; i++ {
		uid := 0x4100 + i
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
