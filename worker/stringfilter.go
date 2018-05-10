/*
 * Copyright 2017-2018 Dgraph Labs, Inc.
 *
 * This file is available under the Apache License, Version 2.0,
 * with the Commons Clause restriction.
 */

package worker

import (
	"strings"

	"github.com/dgraph-io/dgraph/protos/intern"
	"github.com/dgraph-io/dgraph/tok"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
)

type matchFn func(types.Val, stringFilter) bool

type stringFilter struct {
	funcName  string
	funcType  FuncType
	lang      string
	tokens    []string
	match     matchFn
	ineqValue types.Val
	eqVals    []types.Val
}

func matchStrings(uids *intern.List, values [][]types.Val, filter stringFilter) *intern.List {
	rv := &intern.List{}
	for i := 0; i < len(values); i++ {
		for j := 0; j < len(values[i]); j++ {
			if filter.match(values[i][j], filter) {
				rv.Uids = append(rv.Uids, uids.Uids[i])
				break
			}
		}
	}

	return rv
}

func defaultMatch(value types.Val, filter stringFilter) bool {
	tokenMap := map[string]bool{}
	for _, t := range filter.tokens {
		tokenMap[t] = false
	}

	tokens := tokenizeValue(value, filter)
	cnt := 0
	for _, token := range tokens {
		previous, ok := tokenMap[token]
		if ok {
			tokenMap[token] = true
			if previous == false { // count only once
				cnt++
			}
		}
	}

	all := strings.HasPrefix(filter.funcName, "allof") // anyofterms or anyoftext

	if all {
		return cnt == len(filter.tokens)
	} else {
		return cnt > 0
	}
}

func ineqMatch(value types.Val, filter stringFilter) bool {
	if len(filter.eqVals) == 0 {
		return types.CompareVals(filter.funcName, value, filter.ineqValue)
	}

	for _, v := range filter.eqVals {
		if types.CompareVals(filter.funcName, value, v) {
			return true
		}
	}
	return false
}

func tokenizeValue(value types.Val, filter stringFilter) []string {
	var tokName string
	switch filter.funcType {
	case StandardFn:
		tokName = "term"
	case FullTextSearchFn:
		tokName = tok.FtsTokenizerName(filter.lang)
	}

	tokenizer, found := tok.GetTokenizer(tokName)

	// tokenizer was used in previous stages of query proccessing, it has to be available
	x.AssertTrue(found)
	tokens, err := tok.BuildTokens(value.Value, tokenizer)
	if err == nil {
		return tokens
	}
	return []string{}
}
