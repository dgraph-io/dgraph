/*
 * Copyright 2017-2018 Dgraph Labs, Inc.
 *
 * This file is available under the Apache License, Version 2.0,
 * with the Commons Clause restriction.
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
		pl, err := posting.Get(key)
		if err != nil {
			return nil, err
		}
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
