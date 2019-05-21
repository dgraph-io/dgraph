/*
 * Copyright 2017-2018 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package worker

import (
	"strings"

	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/tok"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
	"github.com/golang/glog"
)

type matchFunc func(types.Val, stringFilter) bool

type stringFilter struct {
	funcName  string
	funcType  FuncType
	lang      string
	tokens    []string
	match     matchFunc
	ineqValue types.Val
	eqVals    []types.Val
}

func matchStrings(uids *pb.List, values [][]types.Val, filter stringFilter) *pb.List {
	rv := &pb.List{}
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
	}
	return cnt > 0
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
	case standardFn:
		tokName = "term"
	case fullTextSearchFn:
		tokName = "fulltext"
	}

	tokenizer, found := tok.GetTokenizer(tokName)
	// tokenizer was used in previous stages of query processing, it has to be available
	x.AssertTrue(found)

	tokens, err := tok.BuildTokens(value.Value, tok.GetLangTokenizer(tokenizer, filter.lang))
	if err != nil {
		glog.Errorf("Error while building tokens: %s", err)
		return []string{}
	}
	return tokens
}
