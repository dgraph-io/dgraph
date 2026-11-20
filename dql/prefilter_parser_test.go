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

// TestParseSimilarTo_FilterWithAllOptionsAnyOrder: filter composes with ef and
// distance_threshold regardless of position, the other options stay in Args as
// key/value pairs, and the var name still never leaks into Args.
func TestParseSimilarTo_FilterWithAllOptionsAnyOrder(t *testing.T) {
	orders := []string{
		`distance_threshold: 0.5, filter: allowed, ef: 32`,
		`filter: allowed, ef: 32, distance_threshold: 0.5`,
		`ef: 32, distance_threshold: 0.5, filter: allowed`,
	}
	for _, opts := range orders {
		query := `
		{
			allowed as var(func: type(Chunk))
			q(func: similar_to(emb, 10, "[0.1, 0.2]", ` + opts + `)) { uid }
		}`
		res, err := Parse(Request{Str: query})
		require.NoError(t, err, "options: %s", opts)

		var sim *GraphQuery
		for _, b := range res.Query {
			if b.Func != nil && b.Func.Name == "similar_to" {
				sim = b
			}
		}
		require.NotNil(t, sim, "options: %s", opts)
		require.Equal(t, "allowed", sim.Func.VectorFilterVar)

		// k, vector, then three key/value pairs.
		require.Len(t, sim.Func.Args, 2+3*2, "options: %s", opts)
		got := map[string]string{}
		for i := 2; i+1 < len(sim.Func.Args); i += 2 {
			got[sim.Func.Args[i].Value] = sim.Func.Args[i+1].Value
			require.NotEqual(t, "allowed", sim.Func.Args[i+1].Value)
		}
		require.Equal(t, map[string]string{"ef": "32", "distance_threshold": "0.5", "filter": "1"}, got,
			"options: %s", opts)
	}
}

// TestParseSimilarTo_FilterVarDefinedAfterUse: DQL resolves variables by dependency,
// not by textual order, so defining the filter var in a later block must parse and
// still register the uid dependency.
func TestParseSimilarTo_FilterVarDefinedAfterUse(t *testing.T) {
	query := `
	{
		q(func: similar_to(emb, 10, "[0.1, 0.2]", filter: allowed)) { uid }
		allowed as var(func: type(Chunk))
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
	require.Equal(t, "allowed", sim.Func.VectorFilterVar)
	found := false
	for _, nv := range sim.NeedsVar {
		if nv.Name == "allowed" && nv.Typ == UidVar {
			found = true
		}
	}
	require.True(t, found)
}

// TestParseSimilarTo_FilterVarReusedInFilterDirective: the same var may scope the
// search and also appear in @filter(uid(...)) on the block; both uses parse.
func TestParseSimilarTo_FilterVarReusedInFilterDirective(t *testing.T) {
	query := `
	{
		allowed as var(func: type(Chunk))
		q(func: similar_to(emb, 10, "[0.1, 0.2]", filter: allowed)) @filter(uid(allowed)) {
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
	require.Equal(t, "allowed", sim.Func.VectorFilterVar)
	require.NotNil(t, sim.Filter, "the @filter must still be attached")
	require.NotNil(t, sim.Filter.Func)
	require.Equal(t, "uid", sim.Filter.Func.Name)
}

// TestParseSimilarTo_FilterWithGraphQLVariableVector: the vector may come from a
// GraphQL variable while the scope comes from a DQL variable.
func TestParseSimilarTo_FilterWithGraphQLVariableVector(t *testing.T) {
	query := `
	query q($vec: string) {
		allowed as var(func: type(Chunk))
		q(func: similar_to(emb, 10, $vec, filter: allowed)) { uid }
	}`
	res, err := Parse(Request{Str: query, Variables: map[string]string{"$vec": "[0.1, 0.2]"}})
	require.NoError(t, err)
	var sim *GraphQuery
	for _, b := range res.Query {
		if b.Func != nil && b.Func.Name == "similar_to" {
			sim = b
		}
	}
	require.NotNil(t, sim)
	require.Equal(t, "allowed", sim.Func.VectorFilterVar)
	require.Equal(t, "[0.1, 0.2]", sim.Func.Args[1].Value)
}

// TestParseSimilarTo_FilterByUidQueryVector: similar_to may take a uid instead of a
// vector as its second argument; filter must compose with that form too.
func TestParseSimilarTo_FilterByUidQueryVector(t *testing.T) {
	query := `
	{
		allowed as var(func: type(Chunk))
		q(func: similar_to(emb, 10, "0x30", filter: allowed)) { uid }
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
	require.Equal(t, "allowed", sim.Func.VectorFilterVar)
	require.Equal(t, "0x30", sim.Func.Args[1].Value)
}

// TestParseSimilarTo_FilterOnOtherFunctionsErrors: `filter:` is a similar_to option
// only; on any other function it must not silently parse.
func TestParseSimilarTo_FilterOnOtherFunctionsErrors(t *testing.T) {
	query := `
	{
		allowed as var(func: type(Chunk))
		q(func: eq(name, "x", filter: allowed)) { uid }
	}`
	_, err := Parse(Request{Str: query})
	require.Error(t, err)
}

// TestParseSimilarTo_FilterMissingValueErrors: `filter:` with nothing after the colon.
func TestParseSimilarTo_FilterMissingValueErrors(t *testing.T) {
	query := `
	{
		allowed as var(func: type(Chunk))
		q(func: similar_to(emb, 10, "[0.1, 0.2]", filter: )) { uid }
	}`
	_, err := Parse(Request{Str: query})
	require.Error(t, err)
}
