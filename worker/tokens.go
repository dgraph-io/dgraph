/*
 * Copyright 2016-2018 Dgraph Labs, Inc. and Contributors
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
	"github.com/dgraph-io/badger/v2"

	"bytes"

	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/tok"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
	"github.com/pkg/errors"
)

func verifyStringIndex(attr string, funcType FuncType) (string, bool) {
	var requiredTokenizer tok.Tokenizer
	switch funcType {
	case fullTextSearchFn:
		requiredTokenizer = tok.FullTextTokenizer{}
	case matchFn:
		requiredTokenizer = tok.TrigramTokenizer{}
	default:
		requiredTokenizer = tok.TermTokenizer{}
	}

	if !schema.State().IsIndexed(schema.ReadCtx, attr) {
		return requiredTokenizer.Name(), false
	}

	id := requiredTokenizer.Identifier()
	for _, t := range schema.State().Tokenizer(schema.ReadCtx, attr) {
		if t.Identifier() == id {
			return requiredTokenizer.Name(), true
		}
	}
	return requiredTokenizer.Name(), false
}

func verifyCustomIndex(attr string, tokenizerName string) bool {
	if !schema.State().IsIndexed(schema.ReadCtx, attr) {
		return false
	}
	for _, t := range schema.State().Tokenizer(schema.ReadCtx, attr) {
		if t.Identifier() >= tok.IdentCustom && t.Name() == tokenizerName {
			return true
		}
	}
	return false
}

// Return string tokens from function arguments. It maps function type to correct tokenizer.
// Note: regexp functions require regexp compilation of argument, not tokenization.
func getStringTokens(funcArgs []string, lang string, funcType FuncType) ([]string, error) {
	if lang == "." {
		lang = "en"
	}
	if funcType == fullTextSearchFn {
		return tok.GetFullTextTokens(funcArgs, lang)
	}
	return tok.GetTermTokens(funcArgs)
}

func pickTokenizer(attr string, f string) (tok.Tokenizer, error) {
	// Get the tokenizers and choose the corresponding one.
	if !schema.State().IsIndexed(schema.ReadCtx, attr) {
		return nil, errors.Errorf("Attribute %s is not indexed.", attr)
	}

	tokenizers := schema.State().Tokenizer(schema.ReadCtx, attr)
	for _, t := range tokenizers {
		// If function is eq and we found a tokenizer thats !Lossy(), lets return it
		switch f {
		case "eq":
			// For equality, find a non-lossy tokenizer.
			if !t.IsLossy() {
				return t, nil
			}
		default:
			// rest of the cases: ge, gt, le, lt require a sortable tokenizer.
			if t.IsSortable() {
				return t, nil
			}
		}
	}

	// Should we return an error if we don't find a non-lossy tokenizer for eq function.
	if f != "eq" {
		return nil, errors.Errorf("Attribute:%s does not have proper index for comparison", attr)
	}

	// We didn't find a sortable or !isLossy() tokenizer, lets return the first one.
	return tokenizers[0], nil
}

// getInequalityTokens gets tokens ge / le compared to given token using the first sortable
// index that is found for the predicate.
func getInequalityTokens(readTs uint64, attr, f, lang string,
	ineqValue types.Val) ([]string, string, error) {
	tokenizer, err := pickTokenizer(attr, f)
	if err != nil {
		return nil, "", err
	}

	// Get the token for the value passed in function.
	// XXX: the lang should be query.Langs, but it only matters in edge case test below.
	tokenizer = tok.GetTokenizerForLang(tokenizer, lang)
	ineqTokens, err := tok.BuildTokens(ineqValue.Value, tokenizer)
	if err != nil {
		return nil, "", err
	}

	switch {
	case len(ineqTokens) == 0:
		return nil, "", nil

	// Allow eq with term/fulltext tokenizers, even though they give multiple tokens.
	case f == "eq" &&
		(tokenizer.Identifier() == tok.IdentTerm || tokenizer.Identifier() == tok.IdentFullText):
		break

	case len(ineqTokens) > 1:
		return nil, "", errors.Errorf("Attribute %s does not have a valid tokenizer.", attr)
	}
	ineqToken := ineqTokens[0]

	if f == "eq" {
		return []string{ineqToken}, ineqToken, nil
	}

	// If some new index key was written as part of same transaction it won't be on disk
	// until the txn is committed. This is OK, we don't need to overlay in-memory contents on the
	// DB, to keep the design simple and efficient.
	txn := pstore.NewTransactionAt(readTs, false)
	defer txn.Discard()

	seekKey := x.IndexKey(attr, ineqToken)

	isgeOrGt := f == "ge" || f == "gt"
	itOpt := badger.DefaultIteratorOptions
	itOpt.PrefetchValues = false
	itOpt.Reverse = !isgeOrGt
	itOpt.Prefix = x.IndexKey(attr, string(tokenizer.Identifier()))
	itr := txn.NewIterator(itOpt)
	defer itr.Close()

	// used for inequality comparison below
	ineqTokenInBytes := []byte(ineqToken)

	var out []string
	for itr.Seek(seekKey); itr.Valid(); itr.Next() {
		item := itr.Item()
		key := item.Key()
		k, err := x.Parse(key)
		if err != nil {
			return nil, "", err
		}

		switch {
		// if its lossy then we handle inequality comparison later
		// in handleCompareFunction
		case tokenizer.IsLossy():
			out = append(out, k.Term)

		// for non Lossy lets compare for inequality (gt & lt)
		// to see if key needs to be included
		case f == "gt":
			if bytes.Compare([]byte(k.Term), ineqTokenInBytes) > 0 {
				out = append(out, k.Term)
			}
		case f == "lt":
			if bytes.Compare([]byte(k.Term), ineqTokenInBytes) < 0 {
				out = append(out, k.Term)
			}
		default:
			// for le or ge or any other fn consider the key
			out = append(out, k.Term)
		}
	}
	return out, ineqToken, nil
}
