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
	"github.com/google/codesearch/index"

	"github.com/dgraph-io/dgraph/algo"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/taskp"
	"github.com/dgraph-io/dgraph/tok"
	"github.com/dgraph-io/dgraph/x"
)

const (
	maxUidsForTrigram = 1000000
)

func uidsForRegex(attr string, gid uint32,
	query *index.Query, intersect *taskp.List) (*taskp.List, error) {
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

			if results.Size() == 0 || results.Size() > maxUidsForTrigram {
				return results, regexToWideError()
			}
		}
		for _, sub := range query.Sub {
			if results == nil {
				results = intersect
			}
			// current list of result is passed for intersection
			results, err := uidsForRegex(attr, gid, sub, results)
			if err != nil {
				return nil, err
			}
			if results.Size() == 0 || results.Size() > maxUidsForTrigram {
				return nil, regexToWideError()
			}
		}
	case index.QOr:
		tok.EncodeRegexTokens(query.Trigram)
		uidMatrix := make([]*taskp.List, len(query.Trigram))
		for i, t := range query.Trigram {
			uidMatrix[i] = uidsForTrigram(t)
		}
		results = algo.MergeSorted(uidMatrix)
		for _, sub := range query.Sub {
			if results == nil {
				results = intersect
			}
			subUids, err := uidsForRegex(attr, gid, sub, intersect)
			if err != nil {
				return nil, err
			}
			results = algo.MergeSorted([]*taskp.List{results, subUids})
			if results.Size() > maxUidsForTrigram {
				return nil, regexToWideError()
			}
		}
	default:
		return nil, regexToWideError()
	}
	return results, nil
}

func regexToWideError() error {
	return x.Errorf("Regular expression is too wide-ranging and can't be executed efficiently.")
}
