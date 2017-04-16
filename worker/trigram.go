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
	"fmt"

	"github.com/google/codesearch/index"

	"github.com/dgraph-io/dgraph/algo"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/taskp"
	"github.com/dgraph-io/dgraph/tok"
	"github.com/dgraph-io/dgraph/x"
)

func uidsForRegex(attr string, gid uint32, query *index.Query, intersect *taskp.List) *taskp.List {
	var results *taskp.List
	opts := posting.ListOptions{}
	if intersect.Size() > 0 {
		opts.Intersect = intersect
	}

	uidsForTrigram := func(trigram string) *taskp.List {
		key := x.IndexKey(attr, trigram)
		pl, decr := posting.GetOrCreate(key, gid)
		defer decr()
		return pl.Uids(opts)
	}

	switch query.Op {
	case index.QAnd:
		tok.EncodeRegexTokens(query.Trigram)
		for _, t := range query.Trigram {
			trigramUids := uidsForTrigram(t)
			if results == nil {
				results = trigramUids
			} else {
				algo.IntersectWith(results, trigramUids, results)
			}
			fmt.Printf("tzdybal AND trigram: '%s' list: %v\n", t, results)

			if results.Size() == 0 {
				return results
			}
		}
		for _, sub := range query.Sub {
			if results == nil {
				results = intersect
			}
			// current list of result is passed for intersection
			results = uidsForRegex(attr, gid, sub, results)
			fmt.Printf("tzdybal AND sub: '%s' list: %v\n", sub, results)
			if results.Size() == 0 {
				return nil
			}
		}
	case index.QOr:
		tok.EncodeRegexTokens(query.Trigram)
		uidMatrix := make([]*taskp.List, len(query.Trigram))
		for i, t := range query.Trigram {
			uidMatrix[i] = uidsForTrigram(t)
			fmt.Printf("tzdybal OR trigram: '%s' list: %v\n", t, results)
		}
		results = algo.MergeSorted(uidMatrix)
		for _, sub := range query.Sub {
			if results == nil {
				results = intersect
			}
			subUids := uidsForRegex(attr, gid, sub, intersect)
			results = algo.MergeSorted([]*taskp.List{results, subUids})
			fmt.Printf("tzdybal OR sub: '%s' list: %v\n", sub, results)
		}
	default:
		// do nothing - we're going to return nil to indicate that full scan of values is required
	}
	return results
}
