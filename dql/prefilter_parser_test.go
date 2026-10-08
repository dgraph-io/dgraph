/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package dql

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseSimilarTo_FilterOption(t *testing.T) {
	query := `
	{
		allowed as var(func: type(Chunk))
		q(func: similar_to(emb, 10, "[0.1, 0.2]", filter: allowed)) {
			uid
		}
	}`
	res, err := Parse(Request{Str: query})
	require.NoError(t, err)

	var sim *GraphQuery
	for _, b := range res.Query {
		if b.Func != nil && b.Func.Name == "similar_to" {
			sim = b
		}
	}
	require.NotNil(t, sim)

	// The filter var is recorded and registered as a uid dependency.
	require.Equal(t, "allowed", sim.Func.VectorFilterVar)
	found := false
	for _, nv := range sim.Func.NeedsVar {
		if nv.Name == "allowed" {
			require.Equal(t, UidVar, nv.Typ)
			found = true
		}
	}
	require.True(t, found, "filter var must be a NeedsVar (uid)")

	// A `filter` marker arg is present (so the worker can tell an empty scope from no
	// filter), but the var NAME itself must never be an arg — the allow-set travels
	// via the task UidList, and the var name lives only in NeedsVar/VectorFilterVar.
	hasMarker := false
	for i := 0; i+1 < len(sim.Func.Args); i += 2 {
		if sim.Func.Args[i].Value == "filter" {
			hasMarker = true
		}
		require.NotEqual(t, "allowed", sim.Func.Args[i].Value)
		require.NotEqual(t, "allowed", sim.Func.Args[i+1].Value)
	}
	require.True(t, hasMarker, "a filter marker arg must be present")
}

func TestParseSimilarTo_FilterWithEf(t *testing.T) {
	query := `
	{
		allowed as var(func: type(Chunk))
		q(func: similar_to(emb, 10, "[0.1, 0.2]", ef: 64, filter: allowed)) {
			uid
		}
	}`
	res, err := Parse(Request{Str: query})
	require.NoError(t, err)
	for _, b := range res.Query {
		if b.Func != nil && b.Func.Name == "similar_to" {
			require.Equal(t, "allowed", b.Func.VectorFilterVar)
		}
	}
}

func TestParseSimilarTo_FilterUndefinedVarErrors(t *testing.T) {
	// A filter var that no block defines is a parse-time dependency error.
	query := `
	{
		q(func: similar_to(emb, 10, "[0.1, 0.2]", filter: ghost)) {
			uid
		}
	}`
	_, err := Parse(Request{Str: query})
	require.Error(t, err)
	require.Contains(t, err.Error(), "not defined")
}

// TestParseSimilarTo_FilterPropagatesToGraphQueryNeedsVar guards the dependency
// edge the whole feature relies on. The parser records the filter var on
// Func.NeedsVar, but the var-dependency scheduler (collectVars) and newGraph both
// read the GraphQuery-level NeedsVar — populated for the root func at parse time.
// If that propagation ever regresses, scheduling would not resolve the var first
// and the filter would silently see an empty/garbage allow-set, so assert it here.
func TestParseSimilarTo_FilterPropagatesToGraphQueryNeedsVar(t *testing.T) {
	query := `
	{
		allowed as var(func: type(Chunk))
		q(func: similar_to(emb, 10, "[0.1, 0.2]", filter: allowed)) {
			uid
		}
	}`
	res, err := Parse(Request{Str: query})
	require.NoError(t, err)

	var sim *GraphQuery
	for _, b := range res.Query {
		if b.Func != nil && b.Func.Name == "similar_to" {
			sim = b
		}
	}
	require.NotNil(t, sim)

	found := false
	for _, nv := range sim.NeedsVar {
		if nv.Name == "allowed" {
			require.Equal(t, UidVar, nv.Typ)
			found = true
		}
	}
	require.True(t, found,
		"filter var must surface on GraphQuery.NeedsVar so the scheduler resolves it first")
}

func TestParseSimilarTo_DuplicateFilterErrors(t *testing.T) {
	query := `
	{
		allowed as var(func: type(Chunk))
		q(func: similar_to(emb, 10, "[0.1, 0.2]", filter: allowed, filter: allowed)) {
			uid
		}
	}`
	_, err := Parse(Request{Str: query})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Duplicate key")
}

func TestParseSimilarTo_FilterNonNameValueErrors(t *testing.T) {
	// The filter option's value must be a bare uid-variable name. Non-name tokens
	// ($var, lists, parens) are rejected at the option parser with an explicit
	// message; string/number literals lex as names and are rejected downstream as
	// undefined vars. Either way they must not parse into a valid filter.
	explicit := []string{`$x`, `[1, 2]`, `()`}
	for _, val := range explicit {
		query := `{ q(func: similar_to(emb, 10, "[0.1, 0.2]", filter: ` + val + `)) { uid } }`
		_, err := Parse(Request{Str: query})
		require.Error(t, err, "filter: %s should be rejected", val)
		require.Contains(t, err.Error(), "filter option expects a uid variable name",
			"filter: %s", val)
	}

	downstream := []string{`"foo"`, `5`}
	for _, val := range downstream {
		query := `{ q(func: similar_to(emb, 10, "[0.1, 0.2]", filter: ` + val + `)) { uid } }`
		_, err := Parse(Request{Str: query})
		require.Error(t, err, "filter: %s should be rejected", val)
		require.Contains(t, err.Error(), "not defined", "filter: %s", val)
	}
}
