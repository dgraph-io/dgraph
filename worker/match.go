/*
 * Copyright 2019 Dgraph Labs, Inc. and Contributors
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
	"github.com/dgraph-io/dgraph/algo"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/tok"
	"github.com/dgraph-io/dgraph/x"
	"github.com/lithammer/fuzzysearch/fuzzy"
)

// matchFuzzy takes in a value (from posting) and compares it to our list of ngram tokens.
// Returns true if value matches fuzzy tokens, false otherwise.
func matchFuzzy(srcFn *functionContext, val string) bool {
	if val == "" {
		return false
	}

	terms, err := tok.GetTokens(tok.IdentTerm, val)
	if err != nil {
		return false
	}

	for i := range srcFn.tokens {
		r := fuzzy.RankFind(srcFn.tokens[i], terms)
		if len(r) != 0 {
			return true
		}
	}
	return false
}

// uidsForMatch collects a list of uids that "might" match a fuzzy term based on the ngram
// index. matchFuzzy does the actual fuzzy match.
// Returns the list of uids even if empty, or an error otherwise.
func uidsForMatch(attr string, arg funcArgs) (*pb.List, error) {
	opts := posting.ListOptions{ReadTs: arg.q.ReadTs}
	uidsForNgram := func(ngram string) (*pb.List, error) {
		key := x.IndexKey(attr, ngram)
		pl, err := posting.GetNoStore(key)
		if err != nil {
			return nil, err
		}
		return pl.Uids(opts)
	}

	tokens, err := tok.GetTokens(tok.IdentTrigram, arg.srcFn.tokens...)
	if err != nil {
		return nil, err
	}

	uidMatrix := make([]*pb.List, len(tokens))
	for i, t := range tokens {
		uidMatrix[i], err = uidsForNgram(t)
		if err != nil {
			return nil, err
		}
	}
	return algo.MergeSorted(uidMatrix), nil
}
