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
	"errors"

	cindex "github.com/google/codesearch/index"

	"github.com/dgraph-io/dgraph/algo"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/intern"
	"github.com/dgraph-io/dgraph/tok"
	"github.com/dgraph-io/dgraph/x"
)

const maxUidsForTrigram = 1000000

var regexTooWideErr = errors.New("Regular expression is too wide-ranging and can't be executed efficiently.")

func uidsForRegex(attr string, arg funcArgs,
	query *cindex.Query, intersect *intern.List) (*intern.List, error) {
	var results *intern.List
	opts := posting.ListOptions{
		ReadTs: arg.q.ReadTs,
	}
	if intersect.Size() > 0 {
		opts.Intersect = intersect
	}

	uidsForTrigram := func(trigram string) (*intern.List, error) {
		key := x.IndexKey(attr, trigram)
		pl := posting.Get(key)
		return pl.Uids(opts)
	}

	switch query.Op {
	case cindex.QAnd:
		tok.EncodeRegexTokens(query.Trigram)
		for _, t := range query.Trigram {
			trigramUids, err := uidsForTrigram(t)
			if err != nil {
				return nil, err
			}
			if results == nil {
				results = trigramUids
			} else {
				algo.IntersectWith(results, trigramUids, results)
			}

			if results.Size() == 0 {
				return results, nil
			} else if results.Size() > maxUidsForTrigram {
				return nil, regexTooWideErr
			}
		}
		for _, sub := range query.Sub {
			if results == nil {
				results = intersect
			}
			// current list of result is passed for intersection
			var err error
			results, err = uidsForRegex(attr, arg, sub, results)
			if err != nil {
				return nil, err
			}
			if results.Size() == 0 {
				return results, nil
			} else if results.Size() > maxUidsForTrigram {
				return nil, regexTooWideErr
			}
		}
	case cindex.QOr:
		tok.EncodeRegexTokens(query.Trigram)
		uidMatrix := make([]*intern.List, len(query.Trigram))
		var err error
		for i, t := range query.Trigram {
			uidMatrix[i], err = uidsForTrigram(t)
			if err != nil {
				return nil, err
			}
		}
		results = algo.MergeSorted(uidMatrix)
		for _, sub := range query.Sub {
			if results == nil {
				results = intersect
			}
			subUids, err := uidsForRegex(attr, arg, sub, intersect)
			if err != nil {
				return nil, err
			}
			results = algo.MergeSorted([]*intern.List{results, subUids})
			if results.Size() > maxUidsForTrigram {
				return nil, regexTooWideErr
			}
		}
	default:
		return nil, regexTooWideErr
	}
	return results, nil
}
