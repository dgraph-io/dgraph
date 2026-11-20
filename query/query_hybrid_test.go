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
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
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

// --- generic block helpers ----------------------------------------------------

// valRow is a {uid, val(<var>)} row from an arbitrary result block.
type valRow struct {
	UID   string
	Score float64
}

// blockValRows extracts the ordered {uid, val(varName)} rows of the named block.
// A missing val() key yields Score 0 (callers assert presence separately).
func blockValRows(t *testing.T, js, block, varName string) []valRow {
	t.Helper()
	var resp struct {
		Data map[string]json.RawMessage `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(js), &resp), "response must be valid JSON: %s", js)
	raw, ok := resp.Data[block]
	require.Truef(t, ok, "block %q missing from response: %s", block, js)
	var items []map[string]interface{}
	require.NoError(t, json.Unmarshal(raw, &items))
	key := "val(" + varName + ")"
	rows := make([]valRow, 0, len(items))
	for _, it := range items {
		uid, _ := it["uid"].(string)
		score, _ := it[key].(float64)
		rows = append(rows, valRow{UID: uid, Score: score})
	}
	return rows
}

// blockScoreMap is blockValRows collapsed into a uid->score map.
func blockScoreMap(t *testing.T, js, block, varName string) map[string]float64 {
	t.Helper()
	rows := blockValRows(t, js, block, varName)
	m := make(map[string]float64, len(rows))
	for _, r := range rows {
		m[r.UID] = r.Score
	}
	return m
}

// blockUids extracts the ordered uid list of the named block.
func blockUids(t *testing.T, js, block string) []string {
	t.Helper()
	rows := blockValRows(t, js, block, "")
	uids := make([]string, 0, len(rows))
	for _, r := range rows {
		uids = append(uids, r.UID)
	}
	return uids
}

// uidNum parses a hex uid string ("0x1f5") into its numeric value.
func uidNum(uid string) uint64 {
	v, err := strconv.ParseUint(strings.TrimPrefix(uid, "0x"), 16, 64)
	if err != nil {
		panic(fmt.Sprintf("bad uid %q: %v", uid, err))
	}
	return v
}

// rrfExpected re-implements the fusion contract of query/fuse.go (fuseRRF +
// channelRanks: rank by score desc, tie-break uid asc; contribution
// weight * 1/(k+rank)) so e2e results can be checked against an independent oracle.
func rrfExpected(channels []map[string]float64, weights []float64, k float64) map[string]float64 {
	fused := make(map[string]float64)
	for ci, ch := range channels {
		uids := make([]string, 0, len(ch))
		for uid := range ch {
			uids = append(uids, uid)
		}
		sort.Slice(uids, func(i, j int) bool {
			si, sj := ch[uids[i]], ch[uids[j]]
			if si != sj {
				return si > sj
			}
			return uidNum(uids[i]) < uidNum(uids[j])
		})
		w := 1.0
		if weights != nil {
			w = weights[ci]
		}
		for r, uid := range uids {
			fused[uid] += w * (1.0 / (k + float64(r+1)))
		}
	}
	return fused
}

// --- Q2: ranker score binding under filter + pagination + traversal -----------

// TestRankerScoreBindingUnderFilterPaginationTraversal is a regression pin for the
// uid-keyed rankerScores snapshot (commits 250b79bec/98ae4335e): per-uid scores
// bound by bm25/similar_to must stay attached to the right uid after a var-level
// @filter shrinks the matched set, after orderdesc+first+offset pagination, and
// when the same doc is reached through multiple traversal paths.
func TestRankerScoreBindingUnderFilterPaginationTraversal(t *testing.T) {
	const (
		textPred = "hyb_bind_txt"
		linkPred = "hyb_bind_link"
		vecPred  = "hyb_bind_vec"
	)
	setSchema(fmt.Sprintf(`
		%s: string @index(bm25) .
		%s: [uid] .
		%s: float32vector @index(hnsw(metric:"euclidean")) .
	`, textPred, linkPred, vecPred))
	t.Cleanup(func() {
		dropPredicate(textPred)
		dropPredicate(linkPred)
		dropPredicate(vecPred)
	})

	// 8 text docs, tf("target")=1 each with strictly increasing doclen, so the 8
	// BM25 scores are strictly distinct and rank 7101 > 7102 > ... > 7108.
	// 8 vector docs at distance i from the origin, so similarity 1/(1+i) is
	// strictly distinct and rank 7121 > 7122 > ... > 7128.
	require.NoError(t, addTriplesToCluster(fmt.Sprintf(`
		<7101> <%[1]s> "target" .
		<7102> <%[1]s> "target alpha" .
		<7103> <%[1]s> "target alpha bravo" .
		<7104> <%[1]s> "target alpha bravo charlie" .
		<7105> <%[1]s> "target alpha bravo charlie delta" .
		<7106> <%[1]s> "target alpha bravo charlie delta echo" .
		<7107> <%[1]s> "target alpha bravo charlie delta echo foxtrot" .
		<7108> <%[1]s> "target alpha bravo charlie delta echo foxtrot golf" .
		<7111> <%[2]s> <7103> .
		<7111> <%[2]s> <7105> .
		<7112> <%[2]s> <7103> .
		<7112> <%[2]s> <7105> .
		<7121> <%[3]s> "[1.0, 0.0]" .
		<7122> <%[3]s> "[2.0, 0.0]" .
		<7123> <%[3]s> "[3.0, 0.0]" .
		<7124> <%[3]s> "[4.0, 0.0]" .
		<7125> <%[3]s> "[5.0, 0.0]" .
		<7126> <%[3]s> "[6.0, 0.0]" .
		<7127> <%[3]s> "[7.0, 0.0]" .
		<7128> <%[3]s> "[8.0, 0.0]" .
	`, textPred, linkPred, vecPred)))

	t.Run("BM25", func(t *testing.T) {
		// Unfiltered control: the per-uid score oracle.
		oracleJs := processQueryNoErr(t, fmt.Sprintf(`
		{
			s as var(func: bm25(%s, "target"))
			all(func: uid(s), orderdesc: val(s)) { uid val(s) }
		}`, textPred))
		oracleRows := blockValRows(t, oracleJs, "all", "s")
		require.Len(t, oracleRows, 8)
		for i := 1; i < len(oracleRows); i++ {
			require.Greater(t, oracleRows[i-1].Score, oracleRows[i].Score,
				"scores must be strictly distinct for an unambiguous oracle")
		}
		require.Equal(t, uidHex(t, 7101), oracleRows[0].UID, "shortest doc must rank first")
		oracle := make(map[string]float64, 8)
		for _, r := range oracleRows {
			oracle[r.UID] = r.Score
		}

		// Filter to alternating ranks, then paginate and traverse. Every emitted
		// val(s) must equal the oracle score of the SAME uid (uid-keyed comparison).
		js := processQueryNoErr(t, fmt.Sprintf(`
		{
			s as var(func: bm25(%[1]s, "target")) @filter(uid(7101, 7103, 7105, 7107))
			page(func: uid(s), orderdesc: val(s), first: 2, offset: 1) { uid val(s) }
			parents(func: uid(7111, 7112)) { uid %[2]s { uid val(s) } }
		}`, textPred, linkPred))

		page := blockValRows(t, js, "page", "s")
		require.Len(t, page, 2)
		// Filtered ranking is 7101 > 7103 > 7105 > 7107; offset:1 first:2 -> 7103, 7105.
		require.Equal(t, uidHex(t, 7103), page[0].UID)
		require.Equal(t, uidHex(t, 7105), page[1].UID)
		for _, r := range page {
			require.Equal(t, oracle[r.UID], r.Score,
				"paginated val(s) for %s must equal the unfiltered control score", r.UID)
		}

		// Each doc is reached via TWO parent paths; both must resolve the same score.
		var presp struct {
			Data struct {
				Parents []struct {
					UID  string `json:"uid"`
					Link []struct {
						UID   string  `json:"uid"`
						Score float64 `json:"val(s)"`
					} `json:"hyb_bind_link"`
				} `json:"parents"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal([]byte(js), &presp))
		require.Len(t, presp.Data.Parents, 2)
		for _, p := range presp.Data.Parents {
			require.Len(t, p.Link, 2, "parent %s must reach both docs", p.UID)
			for _, ch := range p.Link {
				require.Equal(t, oracle[ch.UID], ch.Score,
					"traversal val(s) for %s via parent %s must equal the control score", ch.UID, p.UID)
			}
		}
	})

	t.Run("SimilarTo", func(t *testing.T) {
		oracleJs := processQueryNoErr(t, fmt.Sprintf(`
		{
			s as var(func: similar_to(%s, 10, "[0.0, 0.0]"))
			all(func: uid(s), orderdesc: val(s)) { uid val(s) }
		}`, vecPred))
		oracleRows := blockValRows(t, oracleJs, "all", "s")
		require.Len(t, oracleRows, 8)
		oracle := make(map[string]float64, 8)
		for _, r := range oracleRows {
			oracle[r.UID] = r.Score
		}
		// Metric-math cross-check: doc 712i sits at distance i, score 1/(1+i).
		for i := 1; i <= 8; i++ {
			require.InDelta(t, 1.0/(1.0+float64(i)), oracle[uidHex(t, 7120+i)], 1e-6)
		}

		js := processQueryNoErr(t, fmt.Sprintf(`
		{
			s as var(func: similar_to(%s, 10, "[0.0, 0.0]")) @filter(uid(7121, 7123, 7125, 7127))
			page(func: uid(s), orderdesc: val(s), first: 2, offset: 1) { uid val(s) }
		}`, vecPred))
		page := blockValRows(t, js, "page", "s")
		require.Len(t, page, 2)
		// Filtered ranking is 7121 > 7123 > 7125 > 7127; offset:1 first:2 -> 7123, 7125.
		require.Equal(t, uidHex(t, 7123), page[0].UID)
		require.Equal(t, uidHex(t, 7125), page[1].UID)
		for _, r := range page {
			require.Equal(t, oracle[r.UID], r.Score,
				"paginated val(s) for %s must equal the unfiltered control score", r.UID)
		}
	})
}

// --- Q4: similar_to score orientation matches the metric math -----------------

// TestSimilarToScoreValueMatchesMetricMath pins the score orientation contract of
// tok/hnsw similarityScore: cosine/dot surface the raw similarity (higher-better,
// as-is) and euclidean surfaces 1/(1+d) with d the TRUE (non-squared) L2 distance.
// The vectors are chosen so the three metrics rank the corpus differently, so a
// future sign flip, double transform, or squared-vs-true-distance regression
// cannot slip through. A single-channel fuse() consuming the same variable must
// leave the channel's surfaced scores untouched.
func TestSimilarToScoreValueMatchesMetricMath(t *testing.T) {
	preds := map[string]string{
		"euclidean":  "hyb_metric_euc",
		"cosine":     "hyb_metric_cos",
		"dotproduct": "hyb_metric_dot",
	}
	setSchema(fmt.Sprintf(`
		%s: float32vector @index(hnsw(metric:"euclidean")) .
		%s: float32vector @index(hnsw(metric:"cosine")) .
		%s: float32vector @index(hnsw(metric:"dotproduct")) .
	`, preds["euclidean"], preds["cosine"], preds["dotproduct"]))
	t.Cleanup(func() {
		for _, p := range preds {
			dropPredicate(p)
		}
	})

	// Same four vectors on all three predicates; query vector is [1, 0].
	//   7141: [1, 0]    7142: [0.6, 0.8]    7143: [4, 0]    7144: [0, 1]
	var triples strings.Builder
	for _, p := range preds {
		fmt.Fprintf(&triples, `
		<7141> <%[1]s> "[1.0, 0.0]" .
		<7142> <%[1]s> "[0.6, 0.8]" .
		<7143> <%[1]s> "[4.0, 0.0]" .
		<7144> <%[1]s> "[0.0, 1.0]" .`, p)
	}
	require.NoError(t, addTriplesToCluster(triples.String()))

	expected := map[string]map[int]float64{
		// 1/(1+d) with d the true L2 distance to [1,0].
		"euclidean": {
			7141: 1.0,
			7142: 1.0 / (1.0 + math.Sqrt(0.8)),
			7143: 1.0 / (1.0 + 3.0),
			7144: 1.0 / (1.0 + math.Sqrt2),
		},
		// Raw cosine similarity, as-is.
		"cosine": {7141: 1.0, 7142: 0.6, 7143: 1.0, 7144: 0.0},
		// Raw dot product, as-is (unbounded, higher-better).
		"dotproduct": {7141: 1.0, 7142: 0.6, 7143: 4.0, 7144: 0.0},
	}

	scores := make(map[string]map[string]float64)
	for metric, pred := range preds {
		js := processQueryNoErr(t, fmt.Sprintf(`
		{
			s as var(func: similar_to(%s, 10, "[1.0, 0.0]"))
			ranked(func: uid(s), orderdesc: val(s)) { uid val(s) }
			f as var(func: fuse(s, method: "rrf"))
			fused(func: uid(f), orderdesc: val(f)) { uid }
		}`, pred))
		got := blockScoreMap(t, js, "ranked", "s")
		require.Len(t, got, 4, "%s: all four vectors must be returned", metric)
		for decUID, want := range expected[metric] {
			require.InDelta(t, want, got[uidHex(t, decUID)], 1e-6,
				"%s: score for uid %d must match the hand-computed metric value", metric, decUID)
		}
		// fuse() consuming the channel must not perturb it: the fused uid set is the
		// channel's uid set and the surfaced channel scores stay hand-computed-exact.
		require.ElementsMatch(t,
			[]string{uidHex(t, 7141), uidHex(t, 7142), uidHex(t, 7143), uidHex(t, 7144)},
			blockUids(t, js, "fused"), "%s: fused uid set must equal the channel's", metric)
		scores[metric] = got
	}

	// The three metrics must genuinely disagree on the ranking (otherwise this test
	// couldn't catch an orientation mix-up): euclidean ranks 7143 last-but-one,
	// dotproduct ranks it first.
	require.Greater(t, scores["euclidean"][uidHex(t, 7141)], scores["euclidean"][uidHex(t, 7143)])
	require.Greater(t, scores["dotproduct"][uidHex(t, 7143)], scores["dotproduct"][uidHex(t, 7141)])
}

// --- Q5: NaN/Inf query vectors ------------------------------------------------

// KNOWN-FAILING (Q5): a NaN query vector parses silently, poisons every channel score with NaN, and fuse drops the whole vector channel — the hybrid result is byte-identical to a bm25-only control instead of erroring.
// Correct behavior asserted here: a non-finite query vector either fails with a
// clean query error, or produces valid JSON with finite scores, deterministic
// ordering, and a vector channel that actually participates in fusion.
func TestSimilarToNaNInfQueryVector(t *testing.T) {
	// bm25-only control for the channel-participation check.
	controlJs := processQueryNoErr(t, `
	{
		txt as var(func: bm25(description_bm25, "fox"))
		f as var(func: fuse(txt, method: "rrf", k: 60))
		me(func: uid(f), orderdesc: val(f)) { uid val(f) }
	}`)
	controlRows := blockValRows(t, controlJs, "me", "f")
	require.NotEmpty(t, controlRows)

	for _, tc := range []struct{ name, vec string }{
		{"NaN", "[NaN, 0.0, 0.0, 0.0]"},
		{"PlusInf", "[Inf, 0.0, 0.0, 0.0]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Standalone similar_to: either a clean error, or valid JSON with finite scores.
			standalone := fmt.Sprintf(`
			{
				s as var(func: similar_to(description_vec, 7, "%s"))
				me(func: uid(s), orderdesc: val(s)) { uid val(s) }
			}`, tc.vec)
			js, err := processQuery(context.Background(), t, standalone)
			if err == nil {
				rows := blockValRows(t, js, "me", "s") // fails on a bare NaN token in the JSON
				for _, r := range rows {
					require.False(t, math.IsNaN(r.Score) || math.IsInf(r.Score, 0),
						"surfaced score for %s must be finite", r.UID)
				}
			}

			// Fused: the vector channel must not be silently dropped. If the query is
			// accepted, the fused ranking must differ from the bm25-only control
			// (the vector channel contributes ranks even when degenerate) and must be
			// deterministic across runs.
			mixed := fmt.Sprintf(`
			{
				txt as var(func: bm25(description_bm25, "fox"))
				vec as var(func: similar_to(description_vec, 7, "%s"))
				f as var(func: fuse(txt, vec, method: "rrf", k: 60))
				me(func: uid(f), orderdesc: val(f)) { uid val(f) }
			}`, tc.vec)
			jm, err := processQuery(context.Background(), t, mixed)
			if err != nil {
				return // clean rejection is correct behavior
			}
			rows1 := blockValRows(t, jm, "me", "f")
			rows2 := blockValRows(t, processQueryNoErr(t, mixed), "me", "f")
			require.Equal(t, rows1, rows2, "degenerate-vector fusion must be deterministic")
			require.NotEqual(t, controlRows, rows1,
				"fuse(txt, vec) with a %s query vector must not silently equal the bm25-only control "+
					"(the vector channel was dropped without an error)", tc.name)
		})
	}
}

// --- Q6: hybrid topk semantics ------------------------------------------------

// TestHybridTopKIsChannelDepthNotFusedCap pins the CURRENT topk semantics of
// hybrid(): topk bounds each channel's retrieval depth (bm25 first + similar_to
// neighbor count), NOT the fused output size — the fused union of two depth-N
// channels may contain up to 2N uids. AMBIGUOUS-SEMANTICS NOTE: the design doc
// (2026-06-03-hybrid-search-fusion-design.md) describes topk as a cap on emitted
// fused results, and two-stage retrieval inherently omits a doc ranked N+1 in both
// channels even when its exhaustive fused score would win. Whether topk should be
// forwarded as a post-fusion cap (fuse's own topk option) is an open decision;
// this test pins the current deterministic desugaring so a semantics change is a
// deliberate, visible act.
func TestHybridTopKIsChannelDepthNotFusedCap(t *testing.T) {
	hybridQ := `
	{
		f as var(func: hybrid(description_bm25, "fox", description_vec, "[3.0, 0.0, 0.0, 0.0]", topk: 2, method: "rrf", k: 60))
		me(func: uid(f), orderdesc: val(f)) { uid val(f) }
	}`
	explicitQ := `
	{
		txt as var(func: bm25(description_bm25, "fox"), first: 2)
		vec as var(func: similar_to(description_vec, 2, "[3.0, 0.0, 0.0, 0.0]"))
		f as var(func: fuse(txt, vec, method: "rrf", k: 60))
		me(func: uid(f), orderdesc: val(f)) { uid val(f) }
	}`

	hybridRows := blockValRows(t, processQueryNoErr(t, hybridQ), "me", "f")
	explicitRows := blockValRows(t, processQueryNoErr(t, explicitQ), "me", "f")

	// Deterministic across runs.
	hybridRows2 := blockValRows(t, processQueryNoErr(t, hybridQ), "me", "f")
	require.Equal(t, hybridRows, hybridRows2, "hybrid(topk:2) must be deterministic")

	// hybrid desugars exactly to the two-stage explicit form.
	require.Equal(t, len(explicitRows), len(hybridRows))
	for i := range explicitRows {
		require.Equal(t, explicitRows[i].UID, hybridRows[i].UID, "row %d uid mismatch", i)
		require.InDelta(t, explicitRows[i].Score, hybridRows[i].Score, 1e-9, "row %d score mismatch", i)
	}

	// topk is NOT a post-fusion cap: the union of the depth-2 bm25 channel
	// ({503, 501}) and the depth-2 vector channel ({503, 506}) has 3 uids.
	require.Greater(t, len(hybridRows), 2,
		"fused output must exceed topk (topk bounds per-channel retrieval, not fused emission)")
	set := make(map[string]bool)
	for _, r := range hybridRows {
		set[r.UID] = true
	}
	require.True(t, set[uidHex(t, 503)], "503 is rank-1 in both depth-2 channels")
}

// --- Q8: channel validity contract --------------------------------------------

// KNOWN-FAILING (Q8): fuse() silently treats a uid variable (which carries no scores) as a valid empty channel and returns the other channel's ranking with no error.
// Correct behavior asserted here: a non-ranker channel is rejected with an error
// naming the offending channel, mirroring scoresFromVar's "non-numeric scores"
// contract instead of silently no-op'ing the channel.
func TestFuseUidVarChannelRejected(t *testing.T) {
	query := `
	{
		u as var(func: has(description_bm25))
		fox as var(func: bm25(description_bm25, "fox"))
		f as var(func: fuse(u, fox, method: "rrf", k: 60))
		me(func: uid(f), orderdesc: val(f)) { uid }
	}`
	_, err := processQuery(context.Background(), t, query)
	require.Error(t, err, "a uid-var channel (no scores) must be rejected, not silently ignored")
	require.Contains(t, err.Error(), "channel")
}

// KNOWN-FAILING (Q8): a count(uid) variable used as a fuse channel injects the sentinel uid math.MaxUint64 (0xffffffffffffffff) into the fused output as a phantom result.
func TestFuseCountVarChannelNoSentinelUid(t *testing.T) {
	query := `
	{
		var(func: has(description_bm25)) { cnt as count(uid) }
		fox as var(func: bm25(description_bm25, "fox"))
		f as var(func: fuse(cnt, fox, method: "rrf", k: 60))
		me(func: uid(f)) { uid }
	}`
	js, err := processQuery(context.Background(), t, query)
	if err != nil {
		return // rejecting a count-var channel outright is also correct behavior
	}
	require.NotContains(t, js, "0xffffffffffffffff",
		"the count(uid) sentinel uid must never surface as a fused result")
}

// TestFuseEmptyChannelKeepsOtherChannels is a passing pin for the documented
// empty-channel semantics (query/fuse.go computeFuse): a ranker channel that ran
// but matched nothing contributes nothing and must not drop or reorder the other
// channels' results — the fused output equals the surviving channel's single-
// channel fusion exactly.
func TestFuseEmptyChannelKeepsOtherChannels(t *testing.T) {
	js := processQueryNoErr(t, `
	{
		dead as var(func: bm25(description_bm25, "zzzqqqxyzzy"))
		fox as var(func: bm25(description_bm25, "fox"))
		f as var(func: fuse(dead, fox, method: "rrf", k: 60))
		solo as var(func: fuse(fox, method: "rrf", k: 60))
		both(func: uid(f), orderdesc: val(f)) { uid val(f) }
		alone(func: uid(solo), orderdesc: val(solo)) { uid val(solo) }
	}`)
	both := blockValRows(t, js, "both", "f")
	alone := blockValRows(t, js, "alone", "solo")
	require.NotEmpty(t, both)
	require.Equal(t, alone, both,
		"an empty channel must contribute nothing: fuse(dead, fox) == fuse(fox) exactly")
}

// --- Q12: manual union + orderdesc over one channel's scores ------------------

// TestHybridUnionOrderByOneChannelDropsUnscored pins the CURRENT (inherited
// upstream) value-var ordering semantics for the manual-hybrid union pattern:
// `uid(b, v)` unions both channels, but `orderdesc: val(b)` sorts via
// sortAndPaginateUsingVar, which SILENTLY DROPS every uid that has no entry in
// b's score map — vector-only docs vanish from the ordered block even though the
// unordered union contains them. AMBIGUOUS-SEMANTICS NOTE: whether such uids
// should instead sort last (or error) is an open decision; this test pins the
// deterministic drop so any future semantics change is deliberate and visible.
func TestHybridUnionOrderByOneChannelDropsUnscored(t *testing.T) {
	query := `
	{
		b as var(func: bm25(description_bm25, "fox"))
		v as var(func: similar_to(description_vec, 7, "[3.0, 0.0, 0.0, 0.0]"))
		control(func: uid(b, v)) { uid }
		ordered(func: uid(b, v), orderdesc: val(b)) { uid val(b) }
	}`
	js := processQueryNoErr(t, query)

	// The unordered union contains every doc from both channels, including the
	// vector-only docs 504 and 505.
	controlSet := make(map[string]bool)
	for _, uid := range blockUids(t, js, "control") {
		controlSet[uid] = true
	}
	for _, dec := range []int{501, 502, 503, 504, 505, 506, 507} {
		require.True(t, controlSet[uidHex(t, dec)], "unordered union must contain %d", dec)
	}

	// CURRENT behavior: orderdesc: val(b) drops the vector-only docs (absent from
	// b's score map) instead of sorting them last.
	ordered := blockValRows(t, js, "ordered", "b")
	orderedSet := make(map[string]bool)
	for _, r := range ordered {
		orderedSet[r.UID] = true
	}
	require.Len(t, ordered, 5, "current semantics: only b-scored uids survive ordering")
	for _, dec := range []int{501, 502, 503, 506, 507} {
		require.True(t, orderedSet[uidHex(t, dec)], "bm25-scored doc %d must survive ordering", dec)
	}
	require.False(t, orderedSet[uidHex(t, 504)], "vector-only 504 is dropped (current semantics)")
	require.False(t, orderedSet[uidHex(t, 505)], "vector-only 505 is dropped (current semantics)")
	for i := 1; i < len(ordered); i++ {
		require.GreaterOrEqual(t, ordered[i-1].Score, ordered[i].Score)
	}

	// The drop must at least be deterministic.
	js2 := processQueryNoErr(t, query)
	require.Equal(t, ordered, blockValRows(t, js2, "ordered", "b"),
		"the ordered union must be deterministic across runs")
}

// --- Q11: score variables under directives ------------------------------------

// TestScoreVarDirectiveMatrix exercises a ranker-bound score variable under
// @groupby, @cascade, @normalize, and empty-block aggregation, each checked
// against an oracle derived from a plain val(f) projection of the same variable.
func TestScoreVarDirectiveMatrix(t *testing.T) {
	const (
		catPred = "hyb_cat"
		tagPred = "hyb_tag"
	)
	setSchema(fmt.Sprintf("%s: string .\n%s: string .", catPred, tagPred))
	t.Cleanup(func() {
		dropPredicate(catPred)
		dropPredicate(tagPred)
	})
	// Categories cover all five "fox" matches; tags cover all EXCEPT the top-scored
	// doc 503 so @cascade prunes exactly the rank-1 result. Tag values encode the
	// decimal uid so @normalize rows can be re-keyed to their uid.
	require.NoError(t, addTriplesToCluster(fmt.Sprintf(`
		<501> <%[1]s> "groupA" .
		<502> <%[1]s> "groupA" .
		<503> <%[1]s> "groupB" .
		<506> <%[1]s> "groupB" .
		<507> <%[1]s> "groupA" .
		<501> <%[2]s> "501" .
		<502> <%[2]s> "502" .
		<506> <%[2]s> "506" .
		<507> <%[2]s> "507" .
	`, catPred, tagPred)))

	// Oracle: per-uid bm25 scores for the "fox" channel.
	oracleJs := processQueryNoErr(t, `
	{
		f as var(func: bm25(description_bm25, "fox"))
		all(func: uid(f), orderdesc: val(f)) { uid val(f) }
	}`)
	oracleRows := blockValRows(t, oracleJs, "all", "f")
	require.Len(t, oracleRows, 5)
	oracle := make(map[string]float64, 5)
	for _, r := range oracleRows {
		oracle[r.UID] = r.Score
	}
	require.Equal(t, uidHex(t, 503), oracleRows[0].UID, "503 must be the top bm25 'fox' doc")

	// KNOWN-FAILING (Q11): sum(val(f)) under @groupby evaluates the never-executed val child's empty valueMatrix (query/groupby.go:142 index out of range) — the whole query errors instead of aggregating from the var's uid->score map.
	t.Run("GroupbySumOverScoreVar", func(t *testing.T) {
		// The parser rejects aggregating an externally-defined value variable inside
		// @groupby ("Only aggregator/count functions allowed inside @groupby"), so the
		// combination cannot execute — the contract this pins is that it fails CLEANLY
		// as a parse error rather than panicking in groupby aggregation
		// (query/groupby.go valueMatrix indexing, guarded against regardless). If
		// groupby ever learns to aggregate score vars, replace this with the
		// per-group sum oracle assertions.
		_, err := processQuery(context.Background(), t, fmt.Sprintf(`
		{
			f as var(func: bm25(description_bm25, "fox"))
			grouped(func: uid(f)) @groupby(%s) {
				total: sum(val(f))
			}
		}`, catPred))
		require.Error(t, err, "score-var aggregation under @groupby is unsupported and must error cleanly")
		require.Contains(t, err.Error(), "groupby", "must be the parse-time rejection, not a crash")
	})

	t.Run("CascadeAfterOrdering", func(t *testing.T) {
		// @cascade prunes the top-scored doc (503 has no tag); first:2 must then
		// return the two best TAGGED docs, still in score order with correct scores
		// (no uid-order degradation, no double pagination).
		js := processQueryNoErr(t, fmt.Sprintf(`
		{
			f as var(func: bm25(description_bm25, "fox"))
			me(func: uid(f), orderdesc: val(f), first: 2) @cascade {
				uid
				val(f)
				%s
			}
		}`, tagPred))
		rows := blockValRows(t, js, "me", "f")
		require.Len(t, rows, 2, "cascade + first:2 must still fill the page from deeper ranks")
		require.NotContains(t,
			[]string{rows[0].UID, rows[1].UID}, uidHex(t, 503), "untagged 503 must be pruned")
		require.GreaterOrEqual(t, rows[0].Score, rows[1].Score, "score order must survive cascade")
		for _, r := range rows {
			require.Equal(t, oracle[r.UID], r.Score, "cascade must not rebind scores")
		}
		// The two survivors must be the top-2 by score among the tagged docs
		// (multiset comparison so equal-score ties can't flake the assertion).
		tagged := []float64{
			oracle[uidHex(t, 501)], oracle[uidHex(t, 502)],
			oracle[uidHex(t, 506)], oracle[uidHex(t, 507)],
		}
		sort.Sort(sort.Reverse(sort.Float64Slice(tagged)))
		gotScores := []float64{rows[0].Score, rows[1].Score}
		sort.Sort(sort.Reverse(sort.Float64Slice(gotScores)))
		require.Equal(t, tagged[:2], gotScores, "cascade page must be the best two tagged docs")
	})

	t.Run("NormalizePairsScoreWithUid", func(t *testing.T) {
		// @normalize flattens each entity into one row; the (tag, score) pair in a
		// row must belong to the SAME uid (tag values encode the uid).
		js := processQueryNoErr(t, fmt.Sprintf(`
		{
			f as var(func: bm25(description_bm25, "fox"))
			me(func: uid(f), orderdesc: val(f)) @normalize {
				tag: %s
				score: val(f)
			}
		}`, tagPred))
		var resp struct {
			Data struct {
				Me []map[string]interface{} `json:"me"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal([]byte(js), &resp))
		taggedRows := 0
		for _, row := range resp.Data.Me {
			tag, ok := row["tag"].(string)
			if !ok {
				continue // untagged doc (503) flattens without the tag key
			}
			taggedRows++
			dec, err := strconv.Atoi(tag)
			require.NoError(t, err)
			score, ok := row["score"].(float64)
			require.True(t, ok, "normalized row for %s must carry its score", tag)
			require.Equal(t, oracle[uidHex(t, dec)], score,
				"normalized row must pair tag and score of the same uid (%s)", tag)
		}
		require.Equal(t, 4, taggedRows, "all four tagged docs must produce a flattened row")
	})

	t.Run("EmptyBlockAggregatesFullVarMap", func(t *testing.T) {
		// Documented population semantics: an empty block's min/max/sum(val(f))
		// aggregates over the ENTIRE variable map (every uid the ranker bound),
		// regardless of what a sibling consuming block filters down to.
		js := processQueryNoErr(t, `
		{
			f as var(func: bm25(description_bm25, "fox"))
			filtered(func: uid(f)) @filter(uid(503)) { uid }
			agg() {
				mn: min(val(f))
				mx: max(val(f))
				sm: sum(val(f))
			}
		}`)
		var resp struct {
			Data struct {
				Agg []map[string]interface{} `json:"agg"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal([]byte(js), &resp))
		got := make(map[string]float64)
		for _, m := range resp.Data.Agg {
			for k, v := range m {
				if f, ok := v.(float64); ok {
					got[k] = f
				}
			}
		}
		var mn, mx, sm float64
		mn = math.Inf(1)
		for _, s := range oracle {
			mn = math.Min(mn, s)
			mx = math.Max(mx, s)
			sm += s
		}
		require.InDelta(t, mn, got["mn"], 1e-6, "min must cover the full 5-doc var map")
		require.InDelta(t, mx, got["mx"], 1e-6, "max must cover the full 5-doc var map")
		require.InDelta(t, sm, got["sm"], 1e-6,
			"sum must cover the full var map even though a sibling block filters to one uid")
	})
}

// --- Q28: degenerate similar_to inputs ----------------------------------------

// KNOWN-FAILING (Q28): similar_to with numNeighbors <= 0 reaches the HNSW search with maxResults <= 0 and panics the alpha (tok/hnsw/search_layer.go addPathNode indexes an empty neighbors slice) — no positivity guard, no panic-recovery interceptor.
// Correct behavior asserted here: a clean query-level error (or a defined empty
// result) and a cluster that still serves the next query. NOTE: on today's code
// this test can crash the shared test alpha — run it isolated/last when executing
// against a live cluster.
func TestSimilarToDegenerateK(t *testing.T) {
	for _, k := range []string{"0", "-1"} {
		t.Run("k="+k, func(t *testing.T) {
			query := fmt.Sprintf(`
			{
				me(func: similar_to(description_vec, %s, "[3.0, 0.0, 0.0, 0.0]")) { uid }
			}`, k)
			// Either a clean error or a defined (possibly empty) result is acceptable;
			// what is NOT acceptable is taking the alpha down.
			_, _ = processQuery(context.Background(), t, query)

			// The cluster must still answer a trivial follow-up query.
			js := processQueryNoErr(t, `{ me(func: uid(503)) { uid } }`)
			require.Contains(t, js, uidHex(t, 503),
				"cluster must survive a degenerate similar_to k=%s", k)
		})
	}
}

// TestHybridTopKNonPositiveRejected pins the dql/hybrid.go guard (commit
// dc12fbbc1): hybrid() must reject a non-positive topk up front instead of
// forwarding it to similar_to where it would panic the HNSW search.
func TestHybridTopKNonPositiveRejected(t *testing.T) {
	q := `
	{
		f as var(func: hybrid(description_bm25, "fox", description_vec, "[3.0, 0.0, 0.0, 0.0]", topk: 0))
		me(func: uid(f), orderdesc: val(f)) { uid }
	}`
	_, err := processQuery(context.Background(), t, q)
	require.Error(t, err)
	require.Contains(t, err.Error(), "topk must be a positive integer")

	// A negative topk must also fail (whether at the lexer or the hybrid guard).
	qneg := `
	{
		f as var(func: hybrid(description_bm25, "fox", description_vec, "[3.0, 0.0, 0.0, 0.0]", topk: -1))
		me(func: uid(f), orderdesc: val(f)) { uid }
	}`
	_, err = processQuery(context.Background(), t, qneg)
	require.Error(t, err)
}

// --- Q30: fuse-of-fuse and shared channels ------------------------------------

// TestFuseOfFuseAndSharedChannels pins scheduler and purity behavior for
// composed fusions in ONE query: a channel consumed by several fuse blocks, a
// fuse block consuming another fuse's output (a -> f1 -> f3 dependency chain),
// and a three-channel fuse. Every fused score is checked against an independent
// RRF oracle computed from the projected raw channel scores — equality proves
// both that the chain resolved (no "Query couldn't be executed") and that no
// fuse mutated a shared channel's score map.
func TestFuseOfFuseAndSharedChannels(t *testing.T) {
	js := processQueryNoErr(t, `
	{
		a as var(func: bm25(description_bm25, "fox"))
		b as var(func: bm25(description_bm25, "dog"))
		c as var(func: similar_to(description_vec, 7, "[3.0, 0.0, 0.0, 0.0]"))
		f1 as var(func: fuse(a, b, method: "rrf", k: 60))
		f2 as var(func: fuse(a, c, method: "rrf", k: 60, weights: "0.7,0.3"))
		f3 as var(func: fuse(f1, c, method: "rrf", k: 60))
		f4 as var(func: fuse(a, b, c, method: "rrf", k: 60))
		cha(func: uid(a)) { uid val(a) }
		chb(func: uid(b)) { uid val(b) }
		chc(func: uid(c)) { uid val(c) }
		r1(func: uid(f1), orderdesc: val(f1)) { uid val(f1) }
		r2(func: uid(f2), orderdesc: val(f2)) { uid val(f2) }
		r3(func: uid(f3), orderdesc: val(f3)) { uid val(f3) }
		r4(func: uid(f4), orderdesc: val(f4)) { uid val(f4) }
	}`)

	chA := blockScoreMap(t, js, "cha", "a")
	chB := blockScoreMap(t, js, "chb", "b")
	chC := blockScoreMap(t, js, "chc", "c")
	require.NotEmpty(t, chA)
	require.NotEmpty(t, chB)
	require.NotEmpty(t, chC)

	requireFusedEquals := func(block, varName string, want map[string]float64) {
		got := blockScoreMap(t, js, block, varName)
		require.Len(t, got, len(want), "%s: fused uid set must equal the oracle union", block)
		for uid, w := range want {
			g, ok := got[uid]
			require.True(t, ok, "%s: uid %s missing from fused output", block, uid)
			require.InDelta(t, w, g, 1e-9, "%s: fused score mismatch for %s", block, uid)
		}
	}

	// f1 = rrf(a, b); f2 = weighted rrf(a, c) — 'a' is shared by f1/f2/f4, so a
	// match here proves f1's computation did not mutate channel a.
	f1Want := rrfExpected([]map[string]float64{chA, chB}, nil, 60)
	requireFusedEquals("r1", "f1", f1Want)
	requireFusedEquals("r2", "f2",
		rrfExpected([]map[string]float64{chA, chC}, []float64{0.7, 0.3}, 60))

	// f3 = rrf(f1, c): fuse output used as a channel (fuse-of-fuse). Compose the
	// oracle from the SERVER-reported f1 scores so this check isolates f3.
	f1Server := blockScoreMap(t, js, "r1", "f1")
	requireFusedEquals("r3", "f3", rrfExpected([]map[string]float64{f1Server, chC}, nil, 60))

	// Three-channel fuse.
	requireFusedEquals("r4", "f4", rrfExpected([]map[string]float64{chA, chB, chC}, nil, 60))
}
