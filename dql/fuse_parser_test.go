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

// fuseArgPairs flattens a fuse Function's [key, value, key, value, ...] Args
// into a map for assertions.
func fuseArgPairs(fn *Function) map[string]string {
	args := map[string]string{}
	for i := 0; i+1 < len(fn.Args); i += 2 {
		args[fn.Args[i].Value] = fn.Args[i+1].Value
	}
	return args
}

// Regression gate (Q7): fuse blocks used to silently accept @filter/first/offset/
// order/@cascade/children even though execution never applies them (query/query.go
// skips ProcessGraph for fuse blocks). validateFuseBlocks (dql/hybrid.go) must
// reject every such modifier, mirroring hybrid's guard.
func TestParseFuse_RejectsBlockModifiers(t *testing.T) {
	cases := []struct {
		name  string
		block string
	}{
		{"filter", `f as var(func: fuse(a, b)) @filter(has(name))`},
		{"first", `f as var(func: fuse(a, b), first: 5)`},
		{"offset", `f as var(func: fuse(a, b), offset: 2)`},
		{"orderdesc", `f as var(func: fuse(a, b), orderdesc: val(a))`},
		{"cascade", `f as var(func: fuse(a, b)) @cascade`},
		{"children", `f as var(func: fuse(a, b)) { name }`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			query := `
			{
				a as var(func: bm25(text, "fox"))
				b as var(func: bm25(title, "fox"))
				` + tc.block + `
				result(func: uid(f)) { uid }
			}`
			_, err := Parse(Request{Str: query})
			require.Error(t, err,
				"fuse block modifier %q must be rejected, not silently ignored", tc.name)
			require.Contains(t, err.Error(), "not supported on a fuse() block")
		})
	}
}

// Regression gate (Q7): hybrid's modifier guard originally covered
// @filter/order/cascade/children but not pagination Args — `first`/`offset` on a
// hybrid() block parsed fine and were then silently discarded by expandHybridBlock.
// They must error like the other modifiers.
func TestParseHybrid_RejectsPagination(t *testing.T) {
	for _, mod := range []string{"first: 5", "offset: 2"} {
		t.Run(mod, func(t *testing.T) {
			query := `
			{
				f as var(func: hybrid(description, "fox", emb, "[0.1]"), ` + mod + `)
				result(func: uid(f)) { uid }
			}`
			_, err := Parse(Request{Str: query})
			require.Error(t, err,
				"hybrid pagination %q must be rejected, not silently dropped", mod)
			require.Contains(t, err.Error(), "not supported on a hybrid() block")
		})
	}
}

// Regression pin (Q7/Q10, dc12fbbc1): hybrid's existing modifier guard rejects
// @filter, ordering, @cascade, and child blocks with an actionable message.
func TestParseHybrid_RejectsFilterOrderCascadeChildren(t *testing.T) {
	cases := []struct {
		name  string
		block string
	}{
		{"filter", `f as var(func: hybrid(description, "fox", emb, "[0.1]")) @filter(has(name))`},
		{"orderdesc", `f as var(func: hybrid(description, "fox", emb, "[0.1]"), orderdesc: name)`},
		{"cascade", `f as var(func: hybrid(description, "fox", emb, "[0.1]")) @cascade`},
		{"children", `f as var(func: hybrid(description, "fox", emb, "[0.1]")) { name }`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			query := `
			{
				` + tc.block + `
				result(func: uid(f)) { uid }
			}`
			_, err := Parse(Request{Str: query})
			require.Error(t, err)
			require.Contains(t, err.Error(), "not supported on a hybrid() block")
		})
	}
}

// Regression gate (Q29): fuse(a, a) used to parse with the same channel twice in
// NeedsVar, so RRF added the duplicate channel's 1/(k+rank) twice per uid. A
// duplicate channel must be rejected (or, equivalently, deduped) — never silently
// double-counted.
func TestParseFuse_DuplicateChannelVar(t *testing.T) {
	query := `
	{
		a as var(func: bm25(text, "fox"))
		f as var(func: fuse(a, a))
		result(func: uid(f)) { uid }
	}`
	res, err := Parse(Request{Str: query})
	if err != nil {
		// Rejection is the current resolution (validateFuseBlocks).
		require.Contains(t, err.Error(), "duplicate channel")
		return
	}
	// A documented dedup of the channel list would also be acceptable.
	fb := findVarBlock(t, res, "f")
	require.Len(t, fb.Func.NeedsVar, 1,
		"duplicate fuse channel must be rejected or deduped, not double-counted")
}

// Pins CURRENT deterministic parser behavior for adversarial fuse option values.
// Range/semantic validation of option values (negative k, malformed method) is an
// execution-time concern; the parser only guarantees the shapes asserted here.
func TestParseFuse_AdversarialOptionValues(t *testing.T) {
	parse := func(opts string) (Result, error) {
		query := `
		{
			a as var(func: bm25(text, "fox"))
			b as var(func: bm25(title, "fox"))
			f as var(func: fuse(a, b` + opts + `))
			result(func: uid(f)) { uid }
		}`
		return Parse(Request{Str: query})
	}

	t.Run("negative k unquoted is a lex error", func(t *testing.T) {
		_, err := parse(`, k: -1`)
		require.Error(t, err)
		require.Contains(t, err.Error(), "Expected value for k")
	})

	t.Run("negative k quoted parses through", func(t *testing.T) {
		// The parser treats option values as opaque strings; k range checks happen
		// at execution time.
		res, err := parse(`, k: "-1"`)
		require.NoError(t, err)
		require.Equal(t, "-1", fuseArgPairs(findVarBlock(t, res, "f").Func)["k"])
	})

	t.Run("k with no value is a lex error", func(t *testing.T) {
		_, err := parse(`, k:`)
		require.Error(t, err)
		require.Contains(t, err.Error(), "Expected value for k")
	})

	t.Run("unquoted weights list fails var validation", func(t *testing.T) {
		// weights: 0.7,0.3 lexes "0.3" as an extra bare channel name, which then
		// fails loudly as an undefined variable rather than silently mis-weighting.
		_, err := parse(`, method: "linear", weights: 0.7,0.3`)
		require.Error(t, err)
		require.Contains(t, err.Error(), "used but not defined")
	})

	t.Run("quoted weights list parses", func(t *testing.T) {
		res, err := parse(`, method: "linear", weights: "0.7,0.3"`)
		require.NoError(t, err)
		require.Equal(t, "0.7,0.3", fuseArgPairs(findVarBlock(t, res, "f").Func)["weights"])
	})

	t.Run("method value with colon parses through", func(t *testing.T) {
		// An unknown method string is rejected at execution time, not by the parser.
		res, err := parse(`, method: "rr:f"`)
		require.NoError(t, err)
		require.Equal(t, "rr:f", fuseArgPairs(findVarBlock(t, res, "f").Func)["method"])
	})
}

// Q10 hygiene pin: two hybrid() blocks in one query expand with distinct
// monotonically-indexed channel names, and each fuse block references only its
// own channels — no cross-contamination.
func TestParseHybrid_TwoBlocksUniqueNames(t *testing.T) {
	query := `
	{
		f as var(func: hybrid(description, "fox", emb, "[0.1]"))
		g as var(func: hybrid(title, "dog", emb2, "[0.2]", topk: 7))
		r1(func: uid(f)) { uid }
		r2(func: uid(g)) { uid }
	}`
	res, err := Parse(Request{Str: query})
	require.NoError(t, err)

	// Every generated channel var must be unique across both expansions.
	seen := map[string]bool{}
	for _, q := range res.Query {
		if q.Var == "" {
			continue
		}
		require.False(t, seen[q.Var], "variable %q defined twice", q.Var)
		seen[q.Var] = true
	}
	require.True(t, seen["__hybrid0_bm25"])
	require.True(t, seen["__hybrid0_vec"])
	require.True(t, seen["__hybrid1_bm25"])
	require.True(t, seen["__hybrid1_vec"])

	// Each fuse block consumes exactly its own hybrid's channels.
	fb := findVarBlock(t, res, "f")
	require.Equal(t, "fuse", fb.Func.Name)
	require.Len(t, fb.Func.NeedsVar, 2)
	require.Equal(t, "__hybrid0_bm25", fb.Func.NeedsVar[0].Name)
	require.Equal(t, "__hybrid0_vec", fb.Func.NeedsVar[1].Name)

	gb := findVarBlock(t, res, "g")
	require.Equal(t, "fuse", gb.Func.Name)
	require.Len(t, gb.Func.NeedsVar, 2)
	require.Equal(t, "__hybrid1_bm25", gb.Func.NeedsVar[0].Name)
	require.Equal(t, "__hybrid1_vec", gb.Func.NeedsVar[1].Name)

	// Per-block options stay per-block: the second hybrid's topk bounds only its
	// own channels.
	require.Equal(t, "7", findVarBlock(t, res, "__hybrid1_bm25").Args["first"])
	require.Equal(t, "7", findVarBlock(t, res, "__hybrid1_vec").Func.Args[0].Value)
	require.Equal(t, "100", findVarBlock(t, res, "__hybrid0_bm25").Args["first"])
}

// Q10 hygiene pin: user variables using the reserved __hybrid prefix are rejected
// before the rewrite, whether declared at a root block or nested inside a child.
func TestParseHybrid_ReservedPrefixRejected(t *testing.T) {
	t.Run("root", func(t *testing.T) {
		query := `
		{
			__hybrid0_bm25 as var(func: bm25(text, "fox"))
			f as var(func: hybrid(description, "fox", emb, "[0.1]"))
			r(func: uid(f, __hybrid0_bm25)) { uid }
		}`
		_, err := Parse(Request{Str: query})
		require.Error(t, err)
		require.Contains(t, err.Error(), "reserved prefix")
	})
	t.Run("nested child", func(t *testing.T) {
		query := `
		{
			f as var(func: hybrid(description, "fox", emb, "[0.1]"))
			r(func: uid(f)) { __hybrid0_vec as name }
		}`
		_, err := Parse(Request{Str: query})
		require.Error(t, err)
		require.Contains(t, err.Error(), "reserved prefix")
	})
}

// Q10 hygiene pin: a parameterized hybrid query text ($q) must reach the generated
// bm25 channel substituted, not as the literal "$q" (regression for dc12fbbc1).
func TestParseHybrid_DollarVarTextArg(t *testing.T) {
	query := `
	query q($q: string, $vec: string) {
		f as var(func: hybrid(description, $q, emb, $vec))
		r(func: uid(f)) { uid }
	}`
	res, err := Parse(Request{
		Str:       query,
		Variables: map[string]string{"$q": "lazy dog", "$vec": "[0.9, 0.8]"},
	})
	require.NoError(t, err)

	bb := findVarBlock(t, res, "__hybrid0_bm25")
	require.Equal(t, "bm25", bb.Func.Name)
	require.Len(t, bb.Func.Args, 1)
	require.Equal(t, "lazy dog", bb.Func.Args[0].Value,
		"bm25 channel must receive the substituted text, not the literal $q")

	vb := findVarBlock(t, res, "__hybrid0_vec")
	require.Equal(t, "[0.9, 0.8]", vb.Func.Args[1].Value,
		"similar_to channel must receive the substituted vector")
}

// Q10 hygiene pin: non-positive topk is rejected up front (a non-positive value
// would reach HNSW as expectedNeighbors<=0 and panic the search).
func TestParseHybrid_NonPositiveTopKRejected(t *testing.T) {
	cases := []struct {
		name, topk, wantErr string
	}{
		{"zero", `0`, "topk must be a positive integer"},
		{"negative quoted", `"-1"`, "topk must be a positive integer"},
		// Unquoted negatives never lex as an option value at all.
		{"negative unquoted", `-1`, "Expected value for topk"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			query := `
			{
				f as var(func: hybrid(description, "fox", emb, "[0.1]", topk: ` + tc.topk + `))
				result(func: uid(f)) { uid }
			}`
			_, err := Parse(Request{Str: query})
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.wantErr)
		})
	}
}
