//go:build integration || cloud || upgrade

/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

//nolint:lll
package query

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestShinglesBasic(t *testing.T) {
	query := `
	{
		me(func: shingles(description, "quick brown fox")) {
			uid
			description
		}
	}
	`
	js := processQueryNoErr(t, query)

	require.Contains(t, js, "The quick brown fox jumps over the lazy dog")
	require.Contains(t, js, "A quick brown fox leaps over a sleeping dog")
}

func TestShinglesCountAtRoot(t *testing.T) {
	query := `
	{
		me(func: shingles(description, "quick brown")) {
			count(uid)
		}
	}
	`
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"count": 2}]}}`, js)
}

func TestShinglesWithFilter(t *testing.T) {
	query := `
	{
		me(func: has(description)) @filter(shingles(description, "brown fox")) {
			uid
			description
		}
	}
	`
	js := processQueryNoErr(t, query)
	require.Contains(t, js, "The quick brown fox jumps over the lazy dog")
	require.Contains(t, js, "A quick brown fox leaps over a sleeping dog")
	require.Contains(t, js, "Brown foxes are quick and agile animals")
}

func TestShinglesMultipleTerms(t *testing.T) {
	query := `
	{
		me(func: shingles(description, "machine learning algorithms")) {
			uid
			description
		}
	}
	`
	js := processQueryNoErr(t, query)
	require.Contains(t, js, "Machine learning algorithms process natural language text")
	require.NotContains(t, js, "Natural language processing uses advanced algorithms")
	require.NotContains(t, js, "Text processing algorithms analyze linguistic patterns")
	require.NotContains(t, js, "Advanced machine learning techniques improve accuracy")
}

func TestShinglesEmptyQuery(t *testing.T) {
	query := `
	{
		me(func: shingles(description, "")) {
			count(uid)
		}
	}
	`
	js := processQueryNoErr(t, query)
	// Empty query should return empty results
	require.JSONEq(t, `{"data": {"me":[{"count": 0}]}}`, js)
}

func TestShinglesNonExistentTerms(t *testing.T) {
	query := `
	{
		me(func: shingles(description, "nonexistent randomword")) {
			uid
			description
		}
	}
	`
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[]}}`, js)
}

func TestShinglesWithVariables(t *testing.T) {
	query := `
	{
		var(func: shingles(description, "lazy dogs")) {
			d as uid
		}
		
		me(func: uid(d)) {
			uid
			description
		}
	}
	`
	js := processQueryNoErr(t, query)
	require.Contains(t, js, "The quick brown fox jumps over the lazy dog")
	require.Contains(t, js, "The lazy dog sleeps under the warm sun")
}

func TestShinglesAggregation(t *testing.T) {
	query := `
	{
		var(func: shingles(description, "quick brown fox")) {
			total as count(uid)
		}
		
		me(func: uid(total)) {
			count: val(total)
		}
	}
	`
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"count": 2}]}}`, js)
}

func TestShinglesLongPhrase(t *testing.T) {
	query := `
	{
		me(func: shingles(description, "natural language processing advanced algorithms")) {
			uid
			description
		}
	}
	`
	js := processQueryNoErr(t, query)
	require.Contains(t, js, "Natural language processing uses advanced algorithms")
}
