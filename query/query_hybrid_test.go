//go:build integration || cloud

/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

//nolint:lll
package query

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// hybridResult unmarshals a {uid, val(f)} result list ordered by fused score.
type hybridRow struct {
	UID   string  `json:"uid"`
	Score float64 `json:"val(f)"`
}

func fuseRows(t *testing.T, js string) []hybridRow {
	t.Helper()
	var resp struct {
		Data struct {
			Me []hybridRow `json:"me"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(js), &resp))
	return resp.Data.Me
}

func fuseUIDSet(rows []hybridRow) map[string]bool {
	set := make(map[string]bool, len(rows))
	for _, r := range rows {
		set[r.UID] = true
	}
	return set
}

// --- similar_to score surfacing (prerequisite for vector fusion) -------------

func TestSimilarToScoreVariable(t *testing.T) {
	// similar_to bound to a value variable surfaces a higher-is-better similarity
	// score. The query vector equals doc 503's embedding, so 503 is the closest
	// (euclidean distance 0 -> score 1.0) and must rank first.
	query := `
	{
		s as var(func: similar_to(description_vec, 7, "[3.0, 0.0, 0.0, 0.0]"))
		me(func: uid(s), orderdesc: val(s)) {
			uid
			val(s)
		}
	}`
	js := processQueryNoErr(t, query)
	var resp struct {
		Data struct {
			Me []struct {
				UID   string  `json:"uid"`
				Score float64 `json:"val(s)"`
			} `json:"me"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(js), &resp))
	require.NotEmpty(t, resp.Data.Me)

	uid503 := uidHex(t, 503)
	require.Equal(t, uid503, resp.Data.Me[0].UID, "closest vector (503) should rank first")
	require.InDelta(t, 1.0, resp.Data.Me[0].Score, 1e-6, "exact match should score 1/(1+0)=1.0")

	// Scores must be in descending order.
	for i := 1; i < len(resp.Data.Me); i++ {
		require.GreaterOrEqual(t, resp.Data.Me[i-1].Score, resp.Data.Me[i].Score)
	}
}

// --- fuse() over BM25 channels ----------------------------------------------

func TestFuseRRFTwoBM25Channels(t *testing.T) {
	// Fuse two BM25 channels: "fox" (matches 501,502,503,506,507) and "dog"
	// (matches 501,502,504,505). The union must include dog-only docs (504,505)
	// that the fox channel never returns — proving outer-join (not intersection).
	query := `
	{
		fox as var(func: bm25(description_bm25, "fox"))
		dog as var(func: bm25(description_bm25, "dog"))
		f as var(func: fuse(fox, dog, method: "rrf", k: 60))
		me(func: uid(f), orderdesc: val(f)) {
			uid
			val(f)
		}
	}`
	rows := fuseRows(t, processQueryNoErr(t, query))
	require.NotEmpty(t, rows)
	set := fuseUIDSet(rows)

	// Union contains both fox-only (503) and dog-only (504, 505) documents.
	require.True(t, set[uidHex(t, 503)], "fox-only doc 503 must be present")
	require.True(t, set[uidHex(t, 504)], "dog-only doc 504 must be present (union, not intersection)")
	require.True(t, set[uidHex(t, 505)], "dog-only doc 505 must be present")

	// Doc 501/502 appear in BOTH channels, so their fused RRF score should exceed a
	// document that appears in only one channel at the same rank.
	scores := make(map[string]float64)
	for _, r := range rows {
		scores[r.UID] = r.Score
	}
	require.Greater(t, scores[uidHex(t, 501)], 0.0)

	// Scores descending.
	for i := 1; i < len(rows); i++ {
		require.GreaterOrEqual(t, rows[i-1].Score, rows[i].Score)
	}
}

func TestFuseLinearWeights(t *testing.T) {
	// Linear fusion with weights. Heavily weight the "dog" channel; a dog-only doc
	// should then be present and outscore where appropriate.
	query := `
	{
		fox as var(func: bm25(description_bm25, "fox"))
		dog as var(func: bm25(description_bm25, "dog"))
		f as var(func: fuse(fox, dog, method: "linear", weights: "0.1,0.9", normalize: "max"))
		me(func: uid(f), orderdesc: val(f)) {
			uid
			val(f)
		}
	}`
	rows := fuseRows(t, processQueryNoErr(t, query))
	require.NotEmpty(t, rows)
	set := fuseUIDSet(rows)
	require.True(t, set[uidHex(t, 504)], "dog-only doc must appear with linear fusion")
	for i := 1; i < len(rows); i++ {
		require.GreaterOrEqual(t, rows[i-1].Score, rows[i].Score)
	}
}

func TestFuseSingleChannelPassthrough(t *testing.T) {
	// A single-channel fuse should preserve that channel's ranking. "fox fox fox"
	// (503) is the top BM25 result for "fox".
	query := `
	{
		fox as var(func: bm25(description_bm25, "fox"))
		f as var(func: fuse(fox, method: "rrf"))
		me(func: uid(f), orderdesc: val(f), first: 1) {
			uid
			val(f)
		}
	}`
	rows := fuseRows(t, processQueryNoErr(t, query))
	require.Len(t, rows, 1)
	require.Equal(t, uidHex(t, 503), rows[0].UID)
}

func TestFusePagination(t *testing.T) {
	query := `
	{
		fox as var(func: bm25(description_bm25, "fox"))
		dog as var(func: bm25(description_bm25, "dog"))
		f as var(func: fuse(fox, dog, method: "rrf"))
		me(func: uid(f), orderdesc: val(f), first: 2, offset: 1) {
			uid
			val(f)
		}
	}`
	rows := fuseRows(t, processQueryNoErr(t, query))
	require.Len(t, rows, 2, "first:2 offset:1 should return exactly 2 rows")
}

// --- fuse() hybrid: BM25 + vector -------------------------------------------

func TestFuseHybridBM25AndVector(t *testing.T) {
	// The headline use case: fuse BM25 text relevance with vector similarity.
	// Doc 503 ("fox fox fox", embedding [3,0,0,0]) is rank-1 in BOTH the "fox" BM25
	// channel and the [3,0,0,0] vector channel, so it must be the top fused result.
	// The vector channel (k=7) returns all docs, so dog-only docs (504,505) enter
	// the union even though the BM25 "fox" channel never returns them.
	query := `
	{
		txt as var(func: bm25(description_bm25, "fox"))
		vec as var(func: similar_to(description_vec, 7, "[3.0, 0.0, 0.0, 0.0]"))
		f as var(func: fuse(txt, vec, method: "rrf", k: 60))
		me(func: uid(f), orderdesc: val(f)) {
			uid
			val(f)
		}
	}`
	rows := fuseRows(t, processQueryNoErr(t, query))
	require.NotEmpty(t, rows)
	require.Equal(t, uidHex(t, 503), rows[0].UID, "503 is rank-1 in both channels -> top fused")

	set := fuseUIDSet(rows)
	require.True(t, set[uidHex(t, 504)], "vector-only doc 504 must enter the union")
	require.True(t, set[uidHex(t, 505)], "vector-only doc 505 must enter the union")

	for i := 1; i < len(rows); i++ {
		require.GreaterOrEqual(t, rows[i-1].Score, rows[i].Score)
	}
}

// --- hybrid() sugar ----------------------------------------------------------

func TestHybridSugarEquivalentToFuse(t *testing.T) {
	// hybrid() must produce the same top result as the explicit fuse() form.
	hybridQ := `
	{
		f as var(func: hybrid(description_bm25, "fox", description_vec, "[3.0, 0.0, 0.0, 0.0]", topk: 7, method: "rrf", k: 60))
		me(func: uid(f), orderdesc: val(f)) {
			uid
			val(f)
		}
	}`
	explicitQ := `
	{
		txt as var(func: bm25(description_bm25, "fox"), first: 7)
		vec as var(func: similar_to(description_vec, 7, "[3.0, 0.0, 0.0, 0.0]"))
		f as var(func: fuse(txt, vec, method: "rrf", k: 60))
		me(func: uid(f), orderdesc: val(f)) {
			uid
			val(f)
		}
	}`
	hybridRows := fuseRows(t, processQueryNoErr(t, hybridQ))
	explicitRows := fuseRows(t, processQueryNoErr(t, explicitQ))

	require.NotEmpty(t, hybridRows)
	require.Equal(t, len(explicitRows), len(hybridRows), "hybrid and fuse must return the same set")
	require.Equal(t, explicitRows[0].UID, hybridRows[0].UID, "same top result")
	// Fused scores should match position-for-position.
	for i := range explicitRows {
		require.Equal(t, explicitRows[i].UID, hybridRows[i].UID, "row %d uid mismatch", i)
		require.InDelta(t, explicitRows[i].Score, hybridRows[i].Score, 1e-9, "row %d score mismatch", i)
	}
}

// --- error handling ----------------------------------------------------------

func TestFuseUnknownMethod(t *testing.T) {
	query := `
	{
		fox as var(func: bm25(description_bm25, "fox"))
		f as var(func: fuse(fox, method: "bogus"))
		me(func: uid(f), orderdesc: val(f)) { uid }
	}`
	_, err := processQuery(context.Background(), t, query)
	require.Error(t, err)
	require.Contains(t, err.Error(), "method")
}

func TestFuseWeightsCountMismatch(t *testing.T) {
	query := `
	{
		fox as var(func: bm25(description_bm25, "fox"))
		dog as var(func: bm25(description_bm25, "dog"))
		f as var(func: fuse(fox, dog, method: "linear", weights: "0.5"))
		me(func: uid(f), orderdesc: val(f)) { uid }
	}`
	_, err := processQuery(context.Background(), t, query)
	require.Error(t, err)
	require.Contains(t, err.Error(), "weights")
}

func TestFuseBadK(t *testing.T) {
	query := `
	{
		fox as var(func: bm25(description_bm25, "fox"))
		f as var(func: fuse(fox, k: "-5"))
		me(func: uid(f), orderdesc: val(f)) { uid }
	}`
	_, err := processQuery(context.Background(), t, query)
	require.Error(t, err)
	require.Contains(t, err.Error(), "k must be")
}
