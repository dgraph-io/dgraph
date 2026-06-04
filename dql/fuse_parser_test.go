/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package dql

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// findBlock returns the query block whose result variable is varName.
func findVarBlock(t *testing.T, res Result, varName string) *GraphQuery {
	for _, q := range res.Query {
		if q.Var == varName {
			return q
		}
	}
	t.Fatalf("no block defines var %q", varName)
	return nil
}

func TestParseFuse_ChannelsAndOptions(t *testing.T) {
	query := `
	{
		v as var(func: bm25(text, "quick brown fox"))
		e as var(func: similar_to(emb, 100, "[0.1, 0.2]"))
		f as var(func: fuse(v, e, method: "rrf", k: 60))
		result(func: uid(f), orderdesc: val(f)) {
			uid
		}
	}`
	res, err := Parse(Request{Str: query})
	require.NoError(t, err)

	fb := findVarBlock(t, res, "f")
	require.NotNil(t, fb.Func)
	require.Equal(t, "fuse", fb.Func.Name)

	// Channels are captured as value-variable NeedsVar in order.
	require.Len(t, fb.Func.NeedsVar, 2)
	require.Equal(t, "v", fb.Func.NeedsVar[0].Name)
	require.Equal(t, ValueVar, fb.Func.NeedsVar[0].Typ)
	require.Equal(t, "e", fb.Func.NeedsVar[1].Name)

	// Named options are captured as [key, value] arg pairs.
	args := map[string]string{}
	for i := 0; i+1 < len(fb.Func.Args); i += 2 {
		args[fb.Func.Args[i].Value] = fb.Func.Args[i+1].Value
	}
	require.Equal(t, "rrf", args["method"])
	require.Equal(t, "60", args["k"])
}

func TestParseFuse_LinearWeights(t *testing.T) {
	query := `
	{
		v as var(func: bm25(text, "fox"))
		e as var(func: bm25(title, "fox"))
		f as var(func: fuse(v, e, method: "linear", weights: "0.3,0.7", normalize: "max"))
		result(func: uid(f), orderdesc: val(f)) { uid }
	}`
	res, err := Parse(Request{Str: query})
	require.NoError(t, err)
	fb := findVarBlock(t, res, "f")
	require.Len(t, fb.Func.NeedsVar, 2)
	args := map[string]string{}
	for i := 0; i+1 < len(fb.Func.Args); i += 2 {
		args[fb.Func.Args[i].Value] = fb.Func.Args[i+1].Value
	}
	require.Equal(t, "linear", args["method"])
	require.Equal(t, "0.3,0.7", args["weights"])
	require.Equal(t, "max", args["normalize"])
}

func TestParseFuse_ThreeChannels(t *testing.T) {
	query := `
	{
		a as var(func: bm25(text, "fox"))
		b as var(func: bm25(title, "fox"))
		c as var(func: bm25(body, "fox"))
		f as var(func: fuse(a, b, c))
		result(func: uid(f), orderdesc: val(f)) { uid }
	}`
	res, err := Parse(Request{Str: query})
	require.NoError(t, err)
	fb := findVarBlock(t, res, "f")
	require.Len(t, fb.Func.NeedsVar, 3)
}

func TestParseFuse_UnknownOption(t *testing.T) {
	query := `
	{
		v as var(func: bm25(text, "fox"))
		f as var(func: fuse(v, bogus: "x"))
		result(func: uid(f)) { uid }
	}`
	_, err := Parse(Request{Str: query})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Unknown option")
}

func TestParseFuse_DuplicateOption(t *testing.T) {
	query := `
	{
		v as var(func: bm25(text, "fox"))
		f as var(func: fuse(v, k: 10, k: 20))
		result(func: uid(f)) { uid }
	}`
	_, err := Parse(Request{Str: query})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Duplicate key")
}

func TestParseFuse_NoChannels(t *testing.T) {
	query := `
	{
		f as var(func: fuse(method: "rrf"))
		result(func: uid(f)) { uid }
	}`
	_, err := Parse(Request{Str: query})
	require.Error(t, err)
	require.Contains(t, err.Error(), "at least one value variable")
}

func TestParseHybrid_ExpandsToThreeBlocks(t *testing.T) {
	query := `
	{
		f as var(func: hybrid(description, "quick brown fox", emb, "[0.1, 0.2]", topk: 50, method: "rrf", k: 60))
		result(func: uid(f), orderdesc: val(f)) { uid }
	}`
	res, err := Parse(Request{Str: query})
	require.NoError(t, err)

	// The hybrid block is replaced by bm25 + similar_to + fuse (plus the result block).
	var bm25Block, simBlock, fuseBlock *GraphQuery
	for _, q := range res.Query {
		if q.Func == nil {
			continue
		}
		switch q.Func.Name {
		case "bm25":
			bm25Block = q
		case "similar_to":
			simBlock = q
		case "fuse":
			fuseBlock = q
		case "hybrid":
			t.Fatal("hybrid block should have been rewritten away")
		}
	}
	require.NotNil(t, bm25Block, "bm25 channel block must exist")
	require.NotNil(t, simBlock, "similar_to channel block must exist")
	require.NotNil(t, fuseBlock, "fuse block must exist")

	// bm25 channel: predicate + query text.
	require.Equal(t, "description", bm25Block.Func.Attr)
	require.Equal(t, "quick brown fox", bm25Block.Func.Args[0].Value)

	// similar_to channel: predicate + topk + vector.
	require.Equal(t, "emb", simBlock.Func.Attr)
	require.Equal(t, "50", simBlock.Func.Args[0].Value)

	// fuse block keeps the original variable name and the fuse options.
	require.Equal(t, "f", fuseBlock.Var)
	require.Len(t, fuseBlock.Func.NeedsVar, 2)
	args := map[string]string{}
	for i := 0; i+1 < len(fuseBlock.Func.Args); i += 2 {
		args[fuseBlock.Func.Args[i].Value] = fuseBlock.Func.Args[i+1].Value
	}
	require.Equal(t, "rrf", args["method"])
	require.Equal(t, "60", args["k"])
	// topk is consumed by similar_to, not forwarded to fuse.
	require.NotContains(t, args, "topk")
}

func TestParseHybrid_BoundsBM25Channel(t *testing.T) {
	// The generated bm25 channel must be bounded to topk so a broad text query does
	// not score the whole corpus before fusion.
	query := `
	{
		f as var(func: hybrid(description, "fox", emb, "[0.1]", topk: 25))
		result(func: uid(f), orderdesc: val(f)) { uid }
	}`
	res, err := Parse(Request{Str: query})
	require.NoError(t, err)
	for _, q := range res.Query {
		if q.Func != nil && q.Func.Name == "bm25" {
			require.Equal(t, "25", q.Args["first"], "bm25 channel should be capped at topk")
		}
	}
}

func TestParseHybrid_MalformedOptions(t *testing.T) {
	// A trailing option key without a value must be rejected, not silently dropped.
	query := `
	{
		f as var(func: hybrid(description, "fox", emb, "[0.1]", method))
		result(func: uid(f), orderdesc: val(f)) { uid }
	}`
	_, err := Parse(Request{Str: query})
	require.Error(t, err)
}

func TestParseHybrid_DefaultTopK(t *testing.T) {
	query := `
	{
		f as var(func: hybrid(description, "fox", emb, "[0.1]"))
		result(func: uid(f), orderdesc: val(f)) { uid }
	}`
	res, err := Parse(Request{Str: query})
	require.NoError(t, err)
	for _, q := range res.Query {
		if q.Func != nil && q.Func.Name == "similar_to" {
			require.Equal(t, "100", q.Func.Args[0].Value, "default topk should be 100")
		}
	}
}

func TestParseFuse_UndefinedChannelVarErrors(t *testing.T) {
	// A fuse channel referencing a variable that no block defines must be rejected
	// at parse time (not silently stall the scheduler).
	query := `
	{
		v as var(func: bm25(text, "fox"))
		f as var(func: fuse(v, ghost, method: "rrf"))
		result(func: uid(f), orderdesc: val(f)) { uid }
	}`
	_, err := Parse(Request{Str: query})
	require.Error(t, err)
	require.Contains(t, err.Error(), "not defined")
}

func TestParseHybrid_RequiresVar(t *testing.T) {
	query := `
	{
		result(func: hybrid(description, "fox", emb, "[0.1]")) { uid }
	}`
	_, err := Parse(Request{Str: query})
	require.Error(t, err)
	require.Contains(t, err.Error(), "must be assigned to a variable")
}
