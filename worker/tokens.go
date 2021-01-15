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
	"bytes"
	"context"

	"github.com/pkg/errors"

	"github.com/dgraph-io/badger/v3"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/tok"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
)

func verifyStringIndex(ctx context.Context, attr string, funcType FuncType) (string, bool) {
	var requiredTokenizer tok.Tokenizer
	switch funcType {
	case fullTextSearchFn:
		requiredTokenizer = tok.FullTextTokenizer{}
	case matchFn:
		requiredTokenizer = tok.TrigramTokenizer{}
	default:
		requiredTokenizer = tok.TermTokenizer{}
	}

	if !schema.State().IsIndexed(ctx, attr) {
		return requiredTokenizer.Name(), false
	}

	id := requiredTokenizer.Identifier()
	for _, t := range schema.State().Tokenizer(ctx, attr) {
		if t.Identifier() == id {
			return requiredTokenizer.Name(), true
		}
	}
	return requiredTokenizer.Name(), false
}

func verifyCustomIndex(ctx context.Context, attr string, tokenizerName string) bool {
	if !schema.State().IsIndexed(ctx, attr) {
		return false
	}
	for _, t := range schema.State().Tokenizer(ctx, attr) {
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

func pickTokenizer(ctx context.Context, attr string, f string) (tok.Tokenizer, error) {
	// Get the tokenizers and choose the corresponding one.
	if !schema.State().IsIndexed(ctx, attr) {
		return nil, errors.Errorf("Attribute %s is not indexed.", attr)
	}

	tokenizers := schema.State().Tokenizer(ctx, attr)
	for _, t := range tokenizers {
		// If function is eq and we found a tokenizer that's !Lossy(), lets return it
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

	// If we didn't find a !isLossy() tokenizer for eq function on string type predicates,
	// then let's see if we can find a non-trigram tokenizer
	if typ, err := schema.State().TypeOf(attr); err == nil && typ == types.StringID {
		for _, t := range tokenizers {
			if t.Identifier() != tok.IdentTrigram {
				return t, nil
			}
		}
	}

	// otherwise, lets return the first one.
	return tokenizers[0], nil
}

// getInequalityTokens gets tokens ge/le/between compared to given tokens using the first sortable
// index that is found for the predicate.
// In case of ge/gt/le/lt/eq len(ineqValues) should be 1, else(between) len(ineqValues) should be 2.
func getInequalityTokens(ctx context.Context, readTs uint64, attr, f, lang string,
	ineqValues []types.Val) ([]string, []string, error) {

	tokenizer, err := pickTokenizer(ctx, attr, f)
	if err != nil {
		return nil, nil, err
	}

	// Get the token for the value passed in function.
	// XXX: the lang should be query.Langs, but it only matters in edge case test below.
	tokenizer = tok.GetTokenizerForLang(tokenizer, lang)

	var ineqTokensFinal []string
	for _, ineqValue := range ineqValues {
		ineqTokens, err := tok.BuildTokens(ineqValue.Value, tokenizer)
		if err != nil {
			return nil, nil, err
		}

		switch {
		case len(ineqTokens) == 0:
			return nil, nil, nil

		// Allow eq with term/fulltext tokenizers, even though they give multiple tokens.
		case f == "eq" &&
			(tokenizer.Identifier() == tok.IdentTerm || tokenizer.Identifier() == tok.IdentFullText):
			break

		case len(ineqTokens) > 1:
			return nil, nil, errors.Errorf("Attribute %s does not have a valid tokenizer.", attr)
		}

		ineqToken := ineqTokens[0]
		ineqTokensFinal = append(ineqTokensFinal, ineqToken)

		if f == "eq" {
			return []string{ineqToken}, ineqTokensFinal, nil
		}
	}

	// If some new index key was written as part of same transaction it won't be on disk
	// until the txn is committed. This is OK, we don't need to overlay in-memory contents on the
	// DB, to keep the design simple and efficient.
	txn := pstore.NewTransactionAt(readTs, false)
	defer txn.Discard()

	seekKey := x.IndexKey(attr, ineqTokensFinal[0])

	isgeOrGt := f == "ge" || f == "gt" || f == "between"
	itOpt := badger.DefaultIteratorOptions
	itOpt.PrefetchValues = false
	itOpt.Reverse = !isgeOrGt
	itOpt.Prefix = x.IndexKey(attr, string(tokenizer.Identifier()))
	itr := txn.NewIterator(itOpt)
	defer itr.Close()

	// used for inequality comparison below
	ineqTokenInBytes1 := []byte(ineqTokensFinal[0])

	var ineqTokenInBytes2 []byte
	if f == "between" {
		ineqTokenInBytes2 = []byte(ineqTokensFinal[1])
	}

	var out []string
LOOP:
	for itr.Seek(seekKey); itr.Valid(); itr.Next() {
		item := itr.Item()
		key := item.Key()
		k, err := x.Parse(key)
		if err != nil {
			return nil, nil, err
		}

		switch {
		// if its lossy then we handle inequality comparison later
		// in handleCompareFunction
		case tokenizer.IsLossy():
			if f == "between" && bytes.Compare([]byte(k.Term), ineqTokenInBytes2) > 0 {
				break LOOP
			}
			out = append(out, k.Term)

		// for non Lossy lets compare for inequality (gt & lt)
		// to see if key needs to be included
		case f == "gt":
			if bytes.Compare([]byte(k.Term), ineqTokenInBytes1) > 0 {
				out = append(out, k.Term)
			}
		case f == "lt":
			if bytes.Compare([]byte(k.Term), ineqTokenInBytes1) < 0 {
				out = append(out, k.Term)
			}
		case f == "between":
			if bytes.Compare([]byte(k.Term), ineqTokenInBytes1) >= 0 &&
				bytes.Compare([]byte(k.Term), ineqTokenInBytes2) <= 0 {
				out = append(out, k.Term)
			} else { // We should break out of loop as soon as we are out of between range.
				break LOOP
			}
		default:
			// for le or ge or any other fn consider the key
			out = append(out, k.Term)
		}
	}

	return out, ineqTokensFinal, nil
}
