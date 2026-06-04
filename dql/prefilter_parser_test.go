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
