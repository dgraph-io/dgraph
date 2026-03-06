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
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBM25Basic(t *testing.T) {
	query := `
	{
		me(func: bm25(description_bm25, "quick brown fox")) {
			uid
			description_bm25
		}
	}
	`
	js := processQueryNoErr(t, query)
	// Should return documents containing "quick", "brown", or "fox"
	require.Contains(t, js, "quick brown fox jumps")
	require.Contains(t, js, "quick brown fox leaps")
}

func TestBM25Ordering(t *testing.T) {
	// BM25 returns all matching documents. Use first:1 to verify the highest-scored
	// document is "fox fox fox" (tf=3, short doc).
	query := `
	{
		me(func: bm25(description_bm25, "fox")) {
			uid
			description_bm25
		}
	}
	`
	js := processQueryNoErr(t, query)
	// Should contain all fox-mentioning documents.
	require.Contains(t, js, "fox fox fox")
	require.Contains(t, js, "quick brown fox jumps")

	// first:1 should return the top-ranked document.
	topQuery := `
	{
		me(func: bm25(description_bm25, "fox"), first: 1) {
			uid
			description_bm25
		}
	}
	`
	topJs := processQueryNoErr(t, topQuery)
	require.Contains(t, topJs, "fox fox fox",
		"top-1 BM25 result for 'fox' should be 'fox fox fox' (highest tf, shortest doc)")
}

func TestBM25WithParams(t *testing.T) {
	// Custom k and b parameters
	query := `
	{
		me(func: bm25(description_bm25, "fox", "1.5", "0.5")) {
			uid
			description_bm25
		}
	}
	`
	js := processQueryNoErr(t, query)
	require.Contains(t, js, "fox")
}

func TestBM25InvalidParams(t *testing.T) {
	// Negative k should be rejected.
	query := `
	{
		me(func: bm25(description_bm25, "fox", "-1.0", "0.75")) {
			uid
		}
	}
	`
	_, err := processQuery(context.Background(), t, query)
	require.Error(t, err)
	require.Contains(t, err.Error(), "bm25: k must be a positive finite number")

	// b > 1 should be rejected.
	query2 := `
	{
		me(func: bm25(description_bm25, "fox", "1.2", "1.5")) {
			uid
		}
	}
	`
	_, err = processQuery(context.Background(), t, query2)
	require.Error(t, err)
	require.Contains(t, err.Error(), "bm25: b must be between 0 and 1")

	// b < 0 should be rejected.
	query3 := `
	{
		me(func: bm25(description_bm25, "fox", "1.2", "-0.5")) {
			uid
		}
	}
	`
	_, err = processQuery(context.Background(), t, query3)
	require.Error(t, err)
	require.Contains(t, err.Error(), "bm25: b must be between 0 and 1")
}

func TestBM25AsFilter(t *testing.T) {
	query := `
	{
		me(func: has(description_bm25)) @filter(bm25(description_bm25, "fox")) {
			uid
			description_bm25
		}
	}
	`
	js := processQueryNoErr(t, query)
	require.Contains(t, js, "fox")
	// Should not contain documents without "fox"
	require.NotContains(t, js, "Dogs are loyal")
}

func TestBM25NoResults(t *testing.T) {
	query := `
	{
		me(func: bm25(description_bm25, "xyznonexistent")) {
			uid
			description_bm25
		}
	}
	`
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[]}}`, js)
}

func TestBM25SingleTerm(t *testing.T) {
	query := `
	{
		me(func: bm25(description_bm25, "dog")) {
			uid
			description_bm25
		}
	}
	`
	js := processQueryNoErr(t, query)
	require.Contains(t, js, "dog")
}

func TestBM25MultiTerm(t *testing.T) {
	query := `
	{
		me(func: bm25(description_bm25, "quick lazy")) {
			uid
			description_bm25
		}
	}
	`
	js := processQueryNoErr(t, query)
	// Should find docs with "quick" or "lazy" (scores accumulate).
	// Doc 501 has both "quick" and "lazy", so it should rank high.
	require.Contains(t, js, "quick brown fox jumps over the lazy dog")
}

func TestBM25AllStopwords(t *testing.T) {
	// A query consisting entirely of stopwords should return no results.
	query := `
	{
		me(func: bm25(description_bm25, "the a an")) {
			uid
			description_bm25
		}
	}
	`
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[]}}`, js)
}

func TestBM25EmptyPredicate(t *testing.T) {
	query := `
	{
		me(func: bm25(description_bm25, "")) {
			uid
		}
	}
	`
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[]}}`, js)
}

func TestBM25WithCount(t *testing.T) {
	query := `
	{
		me(func: bm25(description_bm25, "fox")) {
			count(uid)
		}
	}
	`
	js := processQueryNoErr(t, query)
	// Should have at least 2 results (docs with "fox")
	require.Contains(t, js, "count")
}

func TestBM25Pagination(t *testing.T) {
	query := `
	{
		me(func: bm25(description_bm25, "fox"), first: 1) {
			uid
			description_bm25
		}
	}
	`
	js := processQueryNoErr(t, query)
	// With first:1, should return exactly one result (the highest-scoring).
	// Doc 503 "fox fox fox" should be the top result.
	require.Contains(t, js, "fox fox fox")
}

func TestBM25ScoreOrdering(t *testing.T) {
	// Use the bm25_score pseudo-predicate with var block to order results by score.
	query := `
	{
		var(func: bm25(description_bm25, "fox")) {
			score as bm25_score
		}
		me(func: uid(score), orderdesc: val(score), first: 1) {
			uid
			description_bm25
			val(score)
		}
	}
	`
	js := processQueryNoErr(t, query)
	// "fox fox fox" (doc 503) has the highest BM25 score (tf=3, shortest doc).
	require.Contains(t, js, "fox fox fox")
}

func TestBM25ScoreOrderingMultiTerm(t *testing.T) {
	// Multi-term query with score ordering: "quick lazy" should rank doc 501 highest
	// since it contains both terms.
	query := `
	{
		var(func: bm25(description_bm25, "quick lazy")) {
			score as bm25_score
		}
		me(func: uid(score), orderdesc: val(score), first: 1) {
			uid
			description_bm25
			val(score)
		}
	}
	`
	js := processQueryNoErr(t, query)
	require.Contains(t, js, "quick brown fox jumps over the lazy dog")
}

func TestBM25ScoreOrderingAllResults(t *testing.T) {
	// Verify all results are returned in score-descending order via val(score).
	query := `
	{
		var(func: bm25(description_bm25, "fox")) {
			score as bm25_score
		}
		me(func: uid(score), orderdesc: val(score)) {
			uid
			description_bm25
			val(score)
		}
	}
	`
	js := processQueryNoErr(t, query)
	// All fox-containing docs should appear.
	require.Contains(t, js, "fox fox fox")
	require.Contains(t, js, "quick brown fox jumps")
	// Score values should be present.
	require.Contains(t, js, "val(score)")
}

func TestBM25ScoreWithPagination(t *testing.T) {
	// Use offset with score ordering.
	query := `
	{
		var(func: bm25(description_bm25, "fox")) {
			score as bm25_score
		}
		me(func: uid(score), orderdesc: val(score), first: 1, offset: 1) {
			uid
			description_bm25
		}
	}
	`
	js := processQueryNoErr(t, query)
	// Should return the second-highest scored document (not "fox fox fox").
	require.NotContains(t, js, "fox fox fox")
	require.Contains(t, js, "fox")
}

// parseScoresFromJSON extracts uid → score from JSON responses containing val(score).
func parseScoresFromJSON(t *testing.T, js string) map[string]float64 {
	t.Helper()
	var resp struct {
		Data struct {
			Me []struct {
				UID   string  `json:"uid"`
				Score float64 `json:"val(score)"`
			} `json:"me"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(js), &resp))
	scores := make(map[string]float64)
	for _, item := range resp.Data.Me {
		scores[item.UID] = item.Score
	}
	return scores
}

func TestBM25IncrementalAddBatch(t *testing.T) {
	batch1 := `
		<600> <description_bm25> "alpha bravo charlie" .
		<601> <description_bm25> "delta echo foxtrot" .
	`
	batch2 := `
		<602> <description_bm25> "golf hotel india" .
		<603> <description_bm25> "juliet kilo lima" .
		<604> <description_bm25> "mike november oscar" .
	`
	batch3 := `
		<605> <description_bm25> "papa quebec romeo" .
		<606> <description_bm25> "sierra tango uniform" .
		<607> <description_bm25> "victor whiskey xray" .
	`
	cleanup := func() {
		deleteTriplesInCluster(`
			<600> <description_bm25> * .
			<601> <description_bm25> * .
			<602> <description_bm25> * .
			<603> <description_bm25> * .
			<604> <description_bm25> * .
			<605> <description_bm25> * .
			<606> <description_bm25> * .
			<607> <description_bm25> * .
		`)
	}
	t.Cleanup(cleanup)

	countQuery := `
	{
		me(func: bm25(description_bm25, "alpha bravo delta echo golf juliet mike papa sierra victor")) {
			count(uid)
		}
	}
	`

	// Batch 1: add 2 docs.
	require.NoError(t, addTriplesToCluster(batch1))
	js := processQueryNoErr(t, countQuery)
	require.Contains(t, js, `"count":2`)

	// Batch 2: add 3 more docs → total 5.
	require.NoError(t, addTriplesToCluster(batch2))
	js = processQueryNoErr(t, countQuery)
	require.Contains(t, js, `"count":5`)

	// Batch 3: add 3 more docs → total 8.
	require.NoError(t, addTriplesToCluster(batch3))
	js = processQueryNoErr(t, countQuery)
	require.Contains(t, js, `"count":8`)

	// Verify specific new UIDs are searchable.
	js = processQueryNoErr(t, `{ me(func: bm25(description_bm25, "whiskey")) { uid } }`)
	require.Contains(t, js, `"0x25e"`) // 606
}

func TestBM25CorpusStatsAffectIDF(t *testing.T) {
	// Capture baseline score for "fox" query.
	scoreQuery := `
	{
		var(func: bm25(description_bm25, "fox")) {
			score as bm25_score
		}
		me(func: uid(score), orderdesc: val(score)) {
			uid
			val(score)
		}
	}
	`
	jsBefore := processQueryNoErr(t, scoreQuery)
	scoresBefore := parseScoresFromJSON(t, jsBefore)
	require.NotEmpty(t, scoresBefore, "baseline should have fox results")

	// Add 10 non-fox docs → N grows, df("fox") stays same → IDF should increase.
	var triples string
	for i := 610; i < 620; i++ {
		triples += fmt.Sprintf(`<%d> <description_bm25> "completely unrelated document about cats and dogs number %d" .
`, i, i)
	}
	require.NoError(t, addTriplesToCluster(triples))
	t.Cleanup(func() {
		var del string
		for i := 610; i < 620; i++ {
			del += fmt.Sprintf("<%d> <description_bm25> * .\n", i)
		}
		deleteTriplesInCluster(del)
	})

	jsAfter := processQueryNoErr(t, scoreQuery)
	scoresAfter := parseScoresFromJSON(t, jsAfter)

	// Compare score for UID 503 ("fox fox fox") — should increase.
	uid503 := "0x1f7"
	before, ok1 := scoresBefore[uid503]
	after, ok2 := scoresAfter[uid503]
	require.True(t, ok1 && ok2, "UID 503 should appear in both before and after results")
	require.Greater(t, after, before,
		"IDF should increase when corpus grows with non-matching docs (before=%f, after=%f)", before, after)
}

func TestBM25DocumentUpdate(t *testing.T) {
	// Add a doc with lots of "fox".
	require.NoError(t, addTriplesToCluster(`<620> <description_bm25> "fox fox fox fox" .`))
	t.Cleanup(func() {
		deleteTriplesInCluster(`<620> <description_bm25> * .`)
	})

	// Should rank top for "fox".
	js := processQueryNoErr(t, `
	{
		me(func: bm25(description_bm25, "fox"), first: 1) {
			uid
		}
	}`)
	require.Contains(t, js, `"0x26c"`) // 620

	// Update to remove "fox", add "cat".
	deleteTriplesInCluster(`<620> <description_bm25> "fox fox fox fox" .`)
	require.NoError(t, addTriplesToCluster(`<620> <description_bm25> "the cat sat on the mat" .`))

	// Should no longer appear in "fox" results.
	js = processQueryNoErr(t, `
	{
		me(func: bm25(description_bm25, "fox")) {
			uid
		}
	}`)
	require.NotContains(t, js, `"0x26c"`)

	// Should appear in "cat" results.
	js = processQueryNoErr(t, `
	{
		me(func: bm25(description_bm25, "cat")) {
			uid
		}
	}`)
	require.Contains(t, js, `"0x26c"`)
}

func TestBM25DocumentDeletion(t *testing.T) {
	require.NoError(t, addTriplesToCluster(`<625> <description_bm25> "unique elephant term" .`))
	t.Cleanup(func() {
		// Cleanup in case test fails before explicit delete.
		deleteTriplesInCluster(`<625> <description_bm25> * .`)
	})

	// Should find the elephant doc.
	js := processQueryNoErr(t, `{ me(func: bm25(description_bm25, "elephant")) { uid } }`)
	require.Contains(t, js, `"0x271"`) // 625

	// Delete it.
	deleteTriplesInCluster(`<625> <description_bm25> "unique elephant term" .`)

	// Should return empty for "elephant".
	js = processQueryNoErr(t, `{ me(func: bm25(description_bm25, "elephant")) { uid } }`)
	require.JSONEq(t, `{"data": {"me":[]}}`, js)

	// Baseline "fox" results should be unaffected.
	js = processQueryNoErr(t, `{ me(func: bm25(description_bm25, "fox")) { uid } }`)
	require.Contains(t, js, "fox")
}

func TestBM25ScoreStabilityAsCorpusGrows(t *testing.T) {
	scoreQuery := `
	{
		var(func: bm25(description_bm25, "fox")) {
			score as bm25_score
		}
		me(func: uid(score), orderdesc: val(score)) {
			uid
			val(score)
		}
	}
	`
	uid503 := "0x1f7"

	// Phase 1: baseline score.
	js1 := processQueryNoErr(t, scoreQuery)
	scores1 := parseScoresFromJSON(t, js1)
	score1, ok := scores1[uid503]
	require.True(t, ok, "UID 503 must appear in baseline")

	// Phase 2: add 5 fox docs → IDF decreases.
	var foxTriples string
	for i := 630; i < 635; i++ {
		foxTriples += fmt.Sprintf(`<%d> <description_bm25> "the fox runs quickly across the field number %d" .
`, i, i)
	}
	require.NoError(t, addTriplesToCluster(foxTriples))
	t.Cleanup(func() {
		var del string
		for i := 630; i < 640; i++ {
			del += fmt.Sprintf("<%d> <description_bm25> * .\n", i)
		}
		deleteTriplesInCluster(del)
	})

	js2 := processQueryNoErr(t, scoreQuery)
	scores2 := parseScoresFromJSON(t, js2)
	score2, ok := scores2[uid503]
	require.True(t, ok, "UID 503 must appear after adding fox docs")
	require.Greater(t, score1, score2,
		"Adding fox docs should decrease IDF and thus score (phase1=%f, phase2=%f)", score1, score2)

	// Phase 3: add 5 non-fox docs → IDF increases relative to phase 2.
	var nonFoxTriples string
	for i := 635; i < 640; i++ {
		nonFoxTriples += fmt.Sprintf(`<%d> <description_bm25> "unrelated content about birds and fish number %d" .
`, i, i)
	}
	require.NoError(t, addTriplesToCluster(nonFoxTriples))

	js3 := processQueryNoErr(t, scoreQuery)
	scores3 := parseScoresFromJSON(t, js3)
	score3, ok := scores3[uid503]
	require.True(t, ok, "UID 503 must appear after adding non-fox docs")
	require.Greater(t, score3, score2,
		"Adding non-fox docs should increase IDF relative to phase2 (phase2=%f, phase3=%f)", score2, score3)
}

func TestBM25LargeCorpus(t *testing.T) {
	// Add 100 docs: 50 with "alpha", 50 with "beta".
	var triples string
	for i := 700; i < 750; i++ {
		triples += fmt.Sprintf(`<%d> <description_bm25> "alpha document content number %d with some padding words" .
`, i, i)
	}
	for i := 750; i < 800; i++ {
		triples += fmt.Sprintf(`<%d> <description_bm25> "beta document content number %d with some padding words" .
`, i, i)
	}
	require.NoError(t, addTriplesToCluster(triples))
	t.Cleanup(func() {
		var del string
		for i := 700; i < 800; i++ {
			del += fmt.Sprintf("<%d> <description_bm25> * .\n", i)
		}
		deleteTriplesInCluster(del)
	})

	// Count alpha docs.
	js := processQueryNoErr(t, `{ me(func: bm25(description_bm25, "alpha")) { count(uid) } }`)
	require.Contains(t, js, `"count":50`)

	// Count beta docs.
	js = processQueryNoErr(t, `{ me(func: bm25(description_bm25, "beta")) { count(uid) } }`)
	require.Contains(t, js, `"count":50`)

	// Union count: "alpha beta" should match all 100.
	js = processQueryNoErr(t, `{ me(func: bm25(description_bm25, "alpha beta")) { count(uid) } }`)
	require.Contains(t, js, `"count":100`)

	// Pagination: first:10, offset:40 for alpha should return 10 results.
	js = processQueryNoErr(t, `
	{
		var(func: bm25(description_bm25, "alpha")) {
			score as bm25_score
		}
		me(func: uid(score), orderdesc: val(score), first: 10, offset: 40) {
			uid
		}
	}`)
	var resp struct {
		Data struct {
			Me []struct{ UID string } `json:"me"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(js), &resp))
	require.Len(t, resp.Data.Me, 10, "pagination first:10 offset:40 should return exactly 10 results")
}

func TestBM25EdgeCaseSingleCharTerm(t *testing.T) {
	require.NoError(t, addTriplesToCluster(`<640> <description_bm25> "x y z" .`))
	t.Cleanup(func() {
		deleteTriplesInCluster(`<640> <description_bm25> * .`)
	})

	// Single-char terms may or may not be indexed depending on tokenizer.
	// Just verify no panic/error.
	_, err := processQuery(context.Background(), t, `
	{
		me(func: bm25(description_bm25, "x")) {
			uid
		}
	}`)
	require.NoError(t, err)
}

func TestBM25EdgeCaseLongDocument(t *testing.T) {
	// Build a ~500-word document with "fox" appearing once.
	words := make([]string, 500)
	for i := range words {
		words[i] = "padding"
	}
	words[250] = "fox"
	longDoc := strings.Join(words, " ")

	require.NoError(t, addTriplesToCluster(fmt.Sprintf(`<645> <description_bm25> %q .`, longDoc)))
	t.Cleanup(func() {
		deleteTriplesInCluster(`<645> <description_bm25> * .`)
	})

	// Get scores for "fox" query.
	scoreQuery := `
	{
		var(func: bm25(description_bm25, "fox")) {
			score as bm25_score
		}
		me(func: uid(score), orderdesc: val(score)) {
			uid
			val(score)
		}
	}
	`
	js := processQueryNoErr(t, scoreQuery)
	scores := parseScoresFromJSON(t, js)

	uid503 := "0x1f7" // "fox fox fox" (doclen=3)
	uid645 := "0x285" // long doc (doclen~500)
	s503, ok1 := scores[uid503]
	s645, ok2 := scores[uid645]
	require.True(t, ok1, "UID 503 must appear in fox results")
	require.True(t, ok2, "UID 645 must appear in fox results")
	require.Greater(t, s503, s645,
		"Short doc with high tf should score higher than long doc with low tf (503=%f, 645=%f)", s503, s645)
}

func TestBM25EdgeCaseUnicode(t *testing.T) {
	triples := `
		<650> <description_bm25> "der schnelle braune Fuchs springt" .
		<651> <description_bm25> "le renard brun rapide saute" .
		<652> <description_bm25> "el zorro marrón rápido salta" .
	`
	require.NoError(t, addTriplesToCluster(triples))
	t.Cleanup(func() {
		deleteTriplesInCluster(`
			<650> <description_bm25> * .
			<651> <description_bm25> * .
			<652> <description_bm25> * .
		`)
	})

	// Query German term.
	js := processQueryNoErr(t, `{ me(func: bm25(description_bm25, "Fuchs")) { uid } }`)
	require.Contains(t, js, `"0x28a"`) // 650

	// Query French term.
	js = processQueryNoErr(t, `{ me(func: bm25(description_bm25, "renard")) { uid } }`)
	require.Contains(t, js, `"0x28b"`) // 651

	// Query Spanish term.
	js = processQueryNoErr(t, `{ me(func: bm25(description_bm25, "zorro")) { uid } }`)
	require.Contains(t, js, `"0x28c"`) // 652
}

func TestBM25EdgeCaseAllStopwordsDoc(t *testing.T) {
	require.NoError(t, addTriplesToCluster(`<655> <description_bm25> "the a an is are was were" .`))
	t.Cleanup(func() {
		deleteTriplesInCluster(`<655> <description_bm25> * .`)
	})

	// Query "the" — should return empty since "the" is a stopword.
	js := processQueryNoErr(t, `{ me(func: bm25(description_bm25, "the")) { uid } }`)
	require.NotContains(t, js, `"0x28f"`) // 655 should not appear

	// But the doc should exist via has().
	js = processQueryNoErr(t, `
	{
		me(func: has(description_bm25)) @filter(uid(655)) {
			uid
		}
	}`)
	require.Contains(t, js, `"0x28f"`)
}

func TestBM25WithUidFilter(t *testing.T) {
	// BM25 root with uid filter to restrict results.
	query := `
	{
		me(func: bm25(description_bm25, "fox")) @filter(uid(501, 503)) {
			uid
			description_bm25
		}
	}
	`
	js := processQueryNoErr(t, query)
	// Should contain only UIDs 501 and 503.
	require.Contains(t, js, `"0x1f5"`) // 501
	require.Contains(t, js, `"0x1f7"`) // 503
	// Should NOT contain other fox docs like 502, 506, 507.
	require.NotContains(t, js, `"0x1f6"`) // 502
	require.NotContains(t, js, `"0x1fa"`) // 506
}

func TestBM25ScoreValuesAreValidFloats(t *testing.T) {
	scoreQuery := `
	{
		var(func: bm25(description_bm25, "fox")) {
			score as bm25_score
		}
		me(func: uid(score), orderdesc: val(score)) {
			uid
			val(score)
		}
	}
	`
	js := processQueryNoErr(t, scoreQuery)
	scores := parseScoresFromJSON(t, js)
	require.NotEmpty(t, scores, "should have at least one result")

	var prevScore float64
	first := true
	// Iterate over results in order (they're orderdesc by score).
	var resp struct {
		Data struct {
			Me []struct {
				UID   string  `json:"uid"`
				Score float64 `json:"val(score)"`
			} `json:"me"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(js), &resp))

	for _, item := range resp.Data.Me {
		score := item.Score
		require.False(t, math.IsNaN(score), "score should not be NaN for uid %s", item.UID)
		require.False(t, math.IsInf(score, 0), "score should not be Inf for uid %s", item.UID)
		require.Greater(t, score, 0.0, "score should be positive for uid %s", item.UID)

		if !first {
			require.GreaterOrEqual(t, prevScore, score,
				"scores should be in descending order: %f >= %f", prevScore, score)
		}
		prevScore = score
		first = false
	}
}

func TestBM25IncrementalAddThenDeleteThenReadd(t *testing.T) {
	t.Cleanup(func() {
		deleteTriplesInCluster(`<670> <description_bm25> * .`)
	})

	// Phase 1: add with "elephant".
	require.NoError(t, addTriplesToCluster(`<670> <description_bm25> "elephant roams the savanna" .`))
	js := processQueryNoErr(t, `{ me(func: bm25(description_bm25, "elephant")) { uid } }`)
	require.Contains(t, js, `"0x29e"`) // 670

	// Phase 2: delete.
	deleteTriplesInCluster(`<670> <description_bm25> "elephant roams the savanna" .`)
	js = processQueryNoErr(t, `{ me(func: bm25(description_bm25, "elephant")) { uid } }`)
	require.NotContains(t, js, `"0x29e"`)

	// Phase 3: re-add with different content.
	require.NoError(t, addTriplesToCluster(`<670> <description_bm25> "penguin waddles on the ice" .`))
	js = processQueryNoErr(t, `{ me(func: bm25(description_bm25, "penguin")) { uid } }`)
	require.Contains(t, js, `"0x29e"`)

	// "elephant" should still not match 670.
	js = processQueryNoErr(t, `{ me(func: bm25(description_bm25, "elephant")) { uid } }`)
	require.NotContains(t, js, `"0x29e"`)
}

func TestBM25NonIndexedPredicateError(t *testing.T) {
	// "name" predicate does not have @index(bm25).
	query := `
	{
		me(func: bm25(name, "alice")) {
			uid
		}
	}
	`
	_, err := processQuery(context.Background(), t, query)
	require.Error(t, err)
	require.Contains(t, err.Error(), "bm25")
}

func TestBM25ConcurrentBatchAdd(t *testing.T) {
	// Add 5 batches of 4 docs each (UIDs 680-699) back-to-back.
	t.Cleanup(func() {
		var del string
		for i := 680; i < 700; i++ {
			del += fmt.Sprintf("<%d> <description_bm25> * .\n", i)
		}
		deleteTriplesInCluster(del)
	})

	for batch := 0; batch < 5; batch++ {
		var triples string
		for j := 0; j < 4; j++ {
			uid := 680 + batch*4 + j
			triples += fmt.Sprintf(`<%d> <description_bm25> "searchterm batch%d doc%d content here" .
`, uid, batch, j)
		}
		require.NoError(t, addTriplesToCluster(triples))
	}

	// All 20 docs should be findable.
	js := processQueryNoErr(t, `{ me(func: bm25(description_bm25, "searchterm")) { count(uid) } }`)
	require.Contains(t, js, `"count":20`)

	// Spot-check a doc from each batch.
	for batch := 0; batch < 5; batch++ {
		uid := 680 + batch*4
		hexUID := fmt.Sprintf(`"0x%x"`, uid)
		term := fmt.Sprintf("batch%d", batch)
		js = processQueryNoErr(t, fmt.Sprintf(`{ me(func: bm25(description_bm25, "%s")) { uid } }`, term))
		require.Contains(t, js, hexUID, "doc %d from batch %d should be searchable", uid, batch)
	}
}

// parseCorpusCount returns the total number of documents with the description_bm25 predicate.
func parseCorpusCount(t *testing.T) float64 {
	t.Helper()
	js := processQueryNoErr(t, `{ me(func: has(description_bm25)) { count(uid) } }`)
	var resp struct {
		Data struct {
			Me []struct {
				Count int `json:"count"`
			} `json:"me"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(js), &resp))
	require.NotEmpty(t, resp.Data.Me)
	n := float64(resp.Data.Me[0].Count)
	require.Greater(t, n, 0.0, "corpus must have documents")
	return n
}

func TestBM25ExactScoreValues(t *testing.T) {
	// Exact score verification using b=0 (BM15 variant) to eliminate avgDL dependency.
	// With b=0: score = idf * (k+1) * tf / (k + tf)
	// This validates the core BM25 formula computes correct numerical values.
	triples := `
		<850> <description_bm25> "quasar quasar quasar" .
		<851> <description_bm25> "quasar nebula pulsar" .
	`
	require.NoError(t, addTriplesToCluster(triples))
	t.Cleanup(func() {
		deleteTriplesInCluster(`
			<850> <description_bm25> * .
			<851> <description_bm25> * .
		`)
	})

	N := parseCorpusCount(t)

	// Query "quasar" with b=0 so score depends only on tf, k, and IDF (not avgDL).
	scoreQuery := `
	{
		var(func: bm25(description_bm25, "quasar", "1.2", "0")) {
			score as bm25_score
		}
		me(func: uid(score), orderdesc: val(score)) {
			uid
			val(score)
		}
	}`
	js := processQueryNoErr(t, scoreQuery)
	scores := parseScoresFromJSON(t, js)

	k := 1.2
	df := 2.0 // both 850 and 851 contain "quasar"
	idf := math.Log1p((N - df + 0.5) / (df + 0.5))

	// Doc 850 "quasar quasar quasar": tf=3, b=0 → score = idf * 2.2 * 3 / 4.2
	expected850 := idf * (k + 1) * 3.0 / (k + 3.0)
	// Doc 851 "quasar nebula pulsar": tf=1, b=0 → score = idf * 2.2 * 1 / 2.2 = idf
	expected851 := idf * (k + 1) * 1.0 / (k + 1.0)

	actual850, ok := scores["0x352"] // 850
	require.True(t, ok, "UID 850 (0x352) must be in results")
	actual851, ok := scores["0x353"] // 851
	require.True(t, ok, "UID 851 (0x353) must be in results")

	require.InEpsilon(t, expected850, actual850, 1e-6,
		"Doc 850 score mismatch: expected %f, got %f (N=%f, df=%f, idf=%f)",
		expected850, actual850, N, df, idf)
	require.InEpsilon(t, expected851, actual851, 1e-6,
		"Doc 851 score mismatch: expected %f, got %f (N=%f, df=%f, idf=%f)",
		expected851, actual851, N, df, idf)

	// Verify ordering: higher tf should yield higher score.
	require.Greater(t, actual850, actual851)
}

func TestBM25BM15NoLengthNormalization(t *testing.T) {
	// With b=0 (BM15 variant), document length should NOT affect the score.
	// Two docs with the same term frequency but different lengths must score identically.
	triples := `
		<860> <description_bm25> "vortex" .
		<861> <description_bm25> "vortex alpha bravo charlie delta echo foxtrot golf hotel india" .
	`
	require.NoError(t, addTriplesToCluster(triples))
	t.Cleanup(func() {
		deleteTriplesInCluster(`
			<860> <description_bm25> * .
			<861> <description_bm25> * .
		`)
	})

	// Query with b=0: length normalization disabled.
	scoreQuery := `
	{
		var(func: bm25(description_bm25, "vortex", "1.2", "0")) {
			score as bm25_score
		}
		me(func: uid(score), orderdesc: val(score)) {
			uid
			val(score)
		}
	}`
	js := processQueryNoErr(t, scoreQuery)
	scores := parseScoresFromJSON(t, js)

	score860, ok1 := scores["0x35c"] // 860
	score861, ok2 := scores["0x35d"] // 861
	require.True(t, ok1, "UID 860 must be in results")
	require.True(t, ok2, "UID 861 must be in results")

	// With b=0 and same tf=1, scores must be equal regardless of document length.
	require.InDelta(t, score860, score861, 1e-9,
		"b=0 should disable length normalization: short doc score=%f, long doc score=%f",
		score860, score861)

	// Now verify that with default b=0.75, the shorter doc scores higher.
	scoreQueryDefault := `
	{
		var(func: bm25(description_bm25, "vortex")) {
			score as bm25_score
		}
		me(func: uid(score), orderdesc: val(score)) {
			uid
			val(score)
		}
	}`
	js = processQueryNoErr(t, scoreQueryDefault)
	scoresDefault := parseScoresFromJSON(t, js)

	defScore860, ok1 := scoresDefault["0x35c"]
	defScore861, ok2 := scoresDefault["0x35d"]
	require.True(t, ok1, "UID 860 must be in default results")
	require.True(t, ok2, "UID 861 must be in default results")
	require.Greater(t, defScore860, defScore861,
		"With b=0.75, shorter doc (doclen=1) should score higher than longer doc (doclen=10)")
}

func TestBM25SingleMatchingDocument(t *testing.T) {
	// Edge case: a single document matching the query term (df=1).
	// IDF should be high since the term is very rare.
	triples := `<865> <description_bm25> "aardvark" .`
	require.NoError(t, addTriplesToCluster(triples))
	t.Cleanup(func() {
		deleteTriplesInCluster(`<865> <description_bm25> * .`)
	})

	N := parseCorpusCount(t)

	// Query with b=0 for exact verification.
	scoreQuery := `
	{
		var(func: bm25(description_bm25, "aardvark", "1.2", "0")) {
			score as bm25_score
		}
		me(func: uid(score), orderdesc: val(score)) {
			uid
			val(score)
		}
	}`
	js := processQueryNoErr(t, scoreQuery)
	scores := parseScoresFromJSON(t, js)

	require.Len(t, scores, 1, "exactly one document should match 'aardvark'")

	actual, ok := scores["0x361"] // 865
	require.True(t, ok, "UID 865 (0x361) must be in results")

	// With df=1, tf=1, b=0, k=1.2:
	// idf = log1p((N - 1 + 0.5) / (1 + 0.5)) = log1p((N - 0.5) / 1.5)
	// score = idf * 2.2 * 1 / (1.2 + 1) = idf * 2.2 / 2.2 = idf
	k := 1.2
	df := 1.0
	idf := math.Log1p((N - df + 0.5) / (df + 0.5))
	expected := idf * (k + 1) * 1.0 / (k + 1.0) // simplifies to idf

	require.InEpsilon(t, expected, actual, 1e-6,
		"Single-doc score mismatch: expected %f, got %f (N=%f, idf=%f)",
		expected, actual, N, idf)
	require.Greater(t, actual, 0.0, "score must be positive")
	require.False(t, math.IsInf(actual, 0), "score must be finite")
}
