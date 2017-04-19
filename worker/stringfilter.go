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

	"github.com/dgraph-io/dgraph/protos/taskp"
	"github.com/dgraph-io/dgraph/tok"
	"github.com/dgraph-io/dgraph/types"
)

type stringFilter struct {
	funcName string
	funcType FuncType
	lang     string
	tokenMap map[string]bool
}

func matchStrings(uids *taskp.List, values []types.Val, filter stringFilter) *taskp.List {
	rv := &taskp.List{}
	for i := 0; i < len(values); i++ {
		if len(values[i].Value.(string)) == 0 {
			continue
		}

		if match(values[i], filter) {
			rv.Uids = append(rv.Uids, uids.Uids[i])
		}
	}

	return rv
}

func match(value types.Val, filter stringFilter) bool {
	tokens := tokenizeValue(value, filter)
	cnt := 0
	for _, token := range tokens {
		previous, ok := filter.tokenMap[token]
		if ok {
			filter.tokenMap[token] = true
			if previous == false { // count only once
				cnt++
			}
		}
	}

	all := strings.HasPrefix(filter.funcName, "allof") // anyofterms or anyoftext

	if all {
		return cnt == len(filter.tokenMap)
	} else {
		return cnt > 0
	}
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

	if found {
		tokens, err := tokenizer.Tokens(value)
		if err == nil {
			return tokens
		}
	}
	return []string{}
}
