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

	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/tok"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
)

func verifyStringIndex(attr string, funcType FuncType) (string, bool) {
	var requiredTokenizer string
	switch funcType {
	case FullTextSearchFn:
		requiredTokenizer = tok.FullTextTokenizer{}.Name()
	default:
		requiredTokenizer = tok.TermTokenizer{}.Name()
	}

	if !schema.State().IsIndexed(attr) {
		return requiredTokenizer, false
	}

	tokenizers := schema.State().Tokenizer(attr)
	for _, tokenizer := range tokenizers {
		// check for prefix, in case of explicit usage of language specific full text tokenizer
		if strings.HasPrefix(tokenizer.Name(), requiredTokenizer) {
			return requiredTokenizer, true
		}
	}

	return requiredTokenizer, false
}

// Return string tokens from function arguments. It maps function type to correct tokenizer.
// Note: regexp functions require regexp compilation of argument, not tokenization.
func getStringTokens(funcArgs []string, lang string, funcType FuncType) ([]string, error) {
	switch funcType {
	case FullTextSearchFn:
		return tok.GetTextTokens(funcArgs, lang)
	default:
		return tok.GetTokens(funcArgs)
	}
}

// getInequalityTokens gets tokens ge / le compared to given token using the first sortable
// index that is found for the predicate.
func getInequalityTokens(attr, f string, ineqValue types.Val) ([]string, string, error) {
	// Get the tokenizers and choose the corresponding one.
	if !schema.State().IsIndexed(attr) {
		return nil, "", x.Errorf("Attribute %s is not indexed.", attr)
	}

	tokenizers := schema.State().Tokenizer(attr)
	var comparableTok tok.Tokenizer
	var sortableTok tok.Tokenizer
	for _, t := range tokenizers {
		if comparableTok == nil && !t.IsLossy() {
			comparableTok = t
		}
		if sortableTok == nil && t.IsSortable() {
			sortableTok = t
		}

		if comparableTok != nil && sortableTok != nil {
			break
		}
	}

	if f == "eq" {
		// eq operations requires either a comparable or a sortable tokenizer.
		if comparableTok == nil && sortableTok == nil {
			return nil, "", x.Errorf("Attribute: %s does not have proper index for comparison",
				attr)
		}
		// rest of the cases, ge, gt , le , lt require a sortable tokenizer.
	} else if sortableTok == nil {
		return nil, "", x.Errorf("Attribute:%s does not have proper index for sorting",
			attr)
	}

	var tokenizer tok.Tokenizer
	// sortableTok can be used for all operations, eq, ge, gt, lt, le so if an attr has one lets
	// use that.
	if sortableTok != nil {
		tokenizer = sortableTok
	} else {
		tokenizer = comparableTok
	}

	// Get the token for the value passed in function.
	ineqTokens, err := tokenizer.Tokens(ineqValue)
	if err != nil {
		return nil, "", err
	}
	if len(ineqTokens) != 1 {
		return nil, "", x.Errorf("Attribute %s doest not have a valid tokenizer.", attr)
	}
	ineqToken := ineqTokens[0]

	it := pstore.NewIterator()
	defer it.Close()
	isgeOrGt := f == "ge" || f == "gt"
	if isgeOrGt {
		it.Seek(x.IndexKey(attr, ineqToken))
	} else {
		it.SeekForPrev(x.IndexKey(attr, ineqToken))
	}

	isPresent := it.Valid() && it.Value() != nil && it.Value().Size() > 0
	idxKey := x.Parse(it.Key().Data())
	if f == "eq" {
		if isPresent && idxKey.Term == ineqToken {
			return []string{ineqToken}, ineqToken, nil
		}
		return []string{}, "", nil
	}

	var out []string
	indexPrefix := x.IndexKey(attr, string(tokenizer.Identifier()))
	for it.Valid() && it.ValidForPrefix(indexPrefix) {
		k := x.Parse(it.Key().Data())
		x.AssertTrue(k != nil)
		out = append(out, k.Term)
		if isgeOrGt {
			it.Next()
		} else {
			it.Prev()
		}
	}
	return out, ineqToken, nil
}
