/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package dql

import (
	"fmt"
	"strconv"
	"strings"
)

// hybrid() is convenience sugar for the common two-channel hybrid-search case:
// BM25 text relevance fused with vector similarity. It is rewritten, before
// variable collection and dependency checking, into the equivalent explicit form:
//
//	x as var(func: hybrid(textPred, "query", vecPred, $vec, topk: 100, method: "rrf", k: 60))
//
// becomes
//
//	__hybrid0_bm25 as var(func: bm25(textPred, "query"), first: 100)
//	__hybrid0_vec  as var(func: similar_to(vecPred, 100, $vec))
//	x              as var(func: fuse(__hybrid0_bm25, __hybrid0_vec, method: "rrf", k: 60))
//
// There is therefore no distinct hybrid execution path: it desugars entirely to
// the fuse() primitive. Positional args are textPred, "query", vecPred and the
// query vector ($var or literal); named options are topk (vector neighbor count,
// default 100) plus the fuse options method/k/weights/normalize.
const (
	hybridTopKOption  = "topk"
	hybridDefaultTopK = "100"
	// hybridVarPrefix namespaces the synthetic channel variables a hybrid() block
	// expands into. It is reserved: user variables may not start with it.
	hybridVarPrefix = "__hybrid"
)

// rewriteHybridBlocks expands every hybrid() query block in res into its three
// constituent blocks. It runs before fragment expansion, variable substitution,
// and dependency checking so the generated blocks participate normally.
func rewriteHybridBlocks(res *Result) error {
	hasHybrid := false
	for _, qu := range res.Query {
		if qu != nil && qu.Func != nil && qu.Func.Name == hybridFunc {
			hasHybrid = true
			break
		}
	}
	if !hasHybrid {
		return nil
	}

	// Guard against the (extremely unlikely) case of a user variable colliding with
	// the synthetic channel names we generate, which would otherwise produce a
	// confusing "defined multiple times" error the user can't act on. Variables can
	// be defined in nested blocks too, so check the whole query tree, not just roots.
	for _, qu := range res.Query {
		if v, ok := findReservedHybridVar(qu); ok {
			return fmt.Errorf("variable %q uses the reserved prefix %q (used internally by hybrid)",
				v, hybridVarPrefix)
		}
	}

	expanded := make([]*GraphQuery, 0, len(res.Query)+2)
	hybridIdx := 0
	for _, qu := range res.Query {
		if qu == nil || qu.Func == nil || qu.Func.Name != hybridFunc {
			expanded = append(expanded, qu)
			continue
		}
		blocks, err := expandHybridBlock(qu, hybridIdx)
		if err != nil {
			return err
		}
		hybridIdx++
		expanded = append(expanded, blocks...)
	}
	res.Query = expanded
	return nil
}

// validateFuseBlocks rejects fuse() blocks whose modifiers would be silently
// ignored. fuse() is computed coordinator-side from resolved value variables and is
// skipped by ProcessGraph, so an @filter, ordering, pagination, cascade, or child
// selection on the fuse block itself never executes. It must also be bound to a
// variable (that is the only way its result is consumed), and its channel list must
// not repeat a variable (a duplicate would double that channel's contribution).
func validateFuseBlocks(res *Result) error {
	for _, qu := range res.Query {
		if qu == nil || qu.Func == nil || qu.Func.Name != fuseFunc {
			continue
		}
		if qu.Var == "" {
			return fmt.Errorf("fuse must be assigned to a variable, e.g. " +
				"`f as var(func: fuse(a, b))`; consume it with uid(f) / val(f)")
		}
		if qu.Filter != nil || len(qu.Order) > 0 || len(qu.Cascade) > 0 ||
			len(qu.Children) > 0 || qu.Args["first"] != "" || qu.Args["offset"] != "" {
			return fmt.Errorf("fuse: @filter, ordering, pagination, cascade, and child " +
				"blocks are not supported on a fuse() block; apply them on the " +
				"consuming uid(var) block (use topk: to bound fused results)")
		}
		seen := make(map[string]bool, len(qu.Func.NeedsVar))
		for _, nv := range qu.Func.NeedsVar {
			if seen[nv.Name] {
				return fmt.Errorf("fuse: duplicate channel variable %q", nv.Name)
			}
			seen[nv.Name] = true
		}
	}
	return nil
}

// findReservedHybridVar walks a query block and its children for any variable using
// the reserved hybrid prefix, returning the first one found.
func findReservedHybridVar(qu *GraphQuery) (string, bool) {
	if qu == nil {
		return "", false
	}
	if strings.HasPrefix(qu.Var, hybridVarPrefix) {
		return qu.Var, true
	}
	for _, ch := range qu.Children {
		if v, ok := findReservedHybridVar(ch); ok {
			return v, true
		}
	}
	return "", false
}

// expandHybridBlock turns a single hybrid() block into [bm25, similar_to, fuse].
func expandHybridBlock(qu *GraphQuery, idx int) ([]*GraphQuery, error) {
	if qu.Var == "" {
		return nil, fmt.Errorf("hybrid must be assigned to a variable, e.g. " +
			"`x as var(func: hybrid(textPred, \"query\", vecPred, $vec))`")
	}
	// The rewrite builds fresh channel/fuse blocks and discards everything else on the
	// original hybrid block, so silently accepting modifiers here would drop them.
	// Reject them explicitly: apply @filter/order/cascade on the outer uid(var) block.
	if qu.Filter != nil || len(qu.Children) > 0 || len(qu.Order) > 0 || len(qu.Cascade) > 0 ||
		qu.Args["first"] != "" || qu.Args["offset"] != "" {
		return nil, fmt.Errorf("hybrid: @filter, ordering, pagination, cascade, and child " +
			"blocks are not supported on a hybrid() block; bind it to a variable and apply " +
			"them on the uid(var) block (use topk: to bound the fused candidate set)")
	}
	fn := qu.Func
	textPred := fn.Attr
	if textPred == "" {
		return nil, fmt.Errorf("hybrid: missing text predicate (first argument)")
	}

	// hybrid has exactly three positional args in Args (the text predicate is the
	// function Attr): queryText, vecPred and the query vector. Any further args are
	// key/value option pairs appended by the parser.
	const numPositional = 3
	if len(fn.Args) < numPositional {
		return nil, fmt.Errorf("hybrid requires textPred, \"query text\", vecPred and a "+
			"query vector; got %d positional arguments", len(fn.Args)+1)
	}
	// Keep the whole Arg (not just its Value) so a parameterized text query keeps its
	// IsDQLVar flag and gets substituted — just like the vector arg below. Taking only
	// .Value would send the literal "$var" to bm25.
	queryArg := fn.Args[0]
	vecPred := fn.Args[1].Value
	vecArg := fn.Args[2]

	// Options follow the positionals as key/value pairs; an odd remainder means a
	// malformed option list rather than something to silently drop.
	if (len(fn.Args)-numPositional)%2 != 0 {
		return nil, fmt.Errorf("hybrid: malformed options (expected key:value pairs)")
	}

	// Parse options: topk feeds similar_to's neighbor count; the rest feed fuse.
	topk := hybridDefaultTopK
	var fuseArgs []Arg
	for i := numPositional; i+1 < len(fn.Args); i += 2 {
		key := strings.ToLower(fn.Args[i].Value)
		val := fn.Args[i+1]
		if key == hybridTopKOption {
			topk = val.Value
			continue
		}
		fuseArgs = append(fuseArgs, Arg{Value: key}, val)
	}

	// topk feeds similar_to's neighbor count and bm25's `first`; a non-positive value
	// reaches HNSW as expectedNeighbors<=0 and panics the search. Reject it up front.
	if n, err := strconv.Atoi(topk); err != nil || n <= 0 {
		return nil, fmt.Errorf("hybrid: topk must be a positive integer, got %q", topk)
	}

	chanBM25 := fmt.Sprintf("%s%d_bm25", hybridVarPrefix, idx)
	chanVec := fmt.Sprintf("%s%d_vec", hybridVarPrefix, idx)

	// Bound the bm25 channel to the same topk candidate budget as the vector channel
	// so a broad text query does not score and materialize the entire corpus before
	// fusion. bm25 honors `first` with WAND top-k early termination.
	bm25Block := &GraphQuery{
		Alias: "var",
		Var:   chanBM25,
		Func:  &Function{Name: "bm25", Attr: textPred, Args: []Arg{queryArg}},
		Args:  map[string]string{"first": topk},
	}
	simBlock := &GraphQuery{
		Alias: "var",
		Var:   chanVec,
		Func:  &Function{Name: similarToFn, Attr: vecPred, Args: []Arg{{Value: topk}, vecArg}},
		Args:  map[string]string{},
	}

	channels := []VarContext{
		{Name: chanBM25, Typ: ValueVar},
		{Name: chanVec, Typ: ValueVar},
	}
	fuseBlock := &GraphQuery{
		Alias:    "var",
		Var:      qu.Var,
		Func:     &Function{Name: fuseFunc, NeedsVar: channels, Args: fuseArgs},
		NeedsVar: channels,
		Args:     map[string]string{},
	}

	return []*GraphQuery{bm25Block, simBlock, fuseBlock}, nil
}
