/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package dql

import (
	"fmt"
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
//	__hybrid0_bm25 as var(func: bm25(textPred, "query"))
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
	// confusing "defined multiple times" error the user can't act on.
	for _, qu := range res.Query {
		if qu != nil && strings.HasPrefix(qu.Var, hybridVarPrefix) {
			return fmt.Errorf("variable %q uses the reserved prefix %q (used internally by hybrid)",
				qu.Var, hybridVarPrefix)
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

// expandHybridBlock turns a single hybrid() block into [bm25, similar_to, fuse].
func expandHybridBlock(qu *GraphQuery, idx int) ([]*GraphQuery, error) {
	if qu.Var == "" {
		return nil, fmt.Errorf("hybrid must be assigned to a variable, e.g. " +
			"`x as var(func: hybrid(textPred, \"query\", vecPred, $vec))`")
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
	queryText := fn.Args[0].Value
	vecPred := fn.Args[1].Value
	vecArg := fn.Args[2]

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

	chanBM25 := fmt.Sprintf("%s%d_bm25", hybridVarPrefix, idx)
	chanVec := fmt.Sprintf("%s%d_vec", hybridVarPrefix, idx)

	bm25Block := &GraphQuery{
		Alias: "var",
		Var:   chanBM25,
		Func:  &Function{Name: "bm25", Attr: textPred, Args: []Arg{{Value: queryText}}},
		Args:  map[string]string{},
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
