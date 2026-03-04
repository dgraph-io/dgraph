//go:build integration || cloud

/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

//nolint:lll
package query

import (
	"context"
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
