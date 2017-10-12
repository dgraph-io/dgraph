/*
 * Copyright (C) 2017 Dgraph Labs, Inc. and Contributors
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 */

package worker

import (
	"strings"

	"github.com/dgraph-io/dgraph/protos"
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

func matchStrings(uids *protos.List, values [][]types.Val, filter stringFilter) *protos.List {
	rv := &protos.List{}
	for i := 0; i < len(values); i++ {
		for j := 0; j < len(values[i]); j++ {
			if len(values[i][j].Value.(string)) == 0 {
				continue
			}

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
