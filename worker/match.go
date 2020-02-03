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
	"github.com/dgraph-io/dgraph/codec"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/tok"
	"github.com/dgraph-io/dgraph/x"
)

// LevenshteinDistance measures the difference between two strings.
// The Levenshtein distance between two words is the minimum number of
// single-character edits (i.e. insertions, deletions or substitutions)
// required to change one word into the other.
//
// This implemention is optimized to use O(min(m,n)) space and is based on the
// optimized C version found here:
// http://en.wikibooks.org/wiki/Algorithm_implementation/Strings/Levenshtein_distance#C
func levenshteinDistance(s, t string) int {
	if len(s) > len(t) {
		s, t = t, s
	}
	r1, r2 := []rune(s), []rune(t) // len(s) <= len(t) => len(r1) <= len(r2)
	column := make([]int, len(r1)+1)

	for y := 1; y <= len(r1); y++ {
		column[y] = y
	}

	for x := 1; x <= len(r2); x++ {
		column[0] = x

		for y, lastDiag := 1, x-1; y <= len(r1); y++ {
			oldDiag := column[y]
			cost := 0
			if r1[y-1] != r2[x-1] {
				cost = 1
			}
			column[y] = min(column[y]+1, column[y-1]+1, lastDiag+cost)
			lastDiag = oldDiag
		}
	}
	return column[len(r1)]
}

func min(a, b, c int) int {
	if a < b && a < c {
		return a
	} else if b < c {
		return b
	}
	return c
}

// matchFuzzy takes in a value (from posting) and compares it to our list of ngram tokens.
// Returns true if value matches fuzzy tokens, false otherwise.
func matchFuzzy(query, val string, max int) bool {
	if val == "" {
		return false
	}
	return levenshteinDistance(val, query) <= max
}

// uidsForMatch collects a list of uids that "might" match a fuzzy term based on the ngram
// index. matchFuzzy does the actual fuzzy match.
// Returns the list of uids even if empty, or an error otherwise.
func uidsForMatch(attr string, arg funcArgs) (*codec.UIDSet, error) {
	opts := posting.ListOptions{ReadTs: arg.q.ReadTs}
	uidsForNgram := func(ngram string) (*codec.UIDSet, error) {
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

	var out *codec.UIDSet
	for _, t := range tokens {
		if out == nil {
			out, err = uidsForNgram(t)
			if err != nil {
				return nil, err
			}
		} else {
			next, err := uidsForNgram(t)
			if err != nil {
				return nil, err
			}
			out.Merge(next)
		}
	}

	return out, nil
}
