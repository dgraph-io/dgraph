/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package worker

import (
	"errors"

	cindex "github.com/google/codesearch/index"
	"google.golang.org/protobuf/proto"

	"github.com/hypermodeinc/dgraph/v25/algo"
	"github.com/hypermodeinc/dgraph/v25/posting"
	"github.com/hypermodeinc/dgraph/v25/protos/pb"
	"github.com/hypermodeinc/dgraph/v25/tok"
	"github.com/hypermodeinc/dgraph/v25/x"
)

var errRegexTooWide = errors.New(
	"regular expression is too wide-ranging and can't be executed efficiently")

func uidsForRegex(attr string, arg funcArgs,
	query *cindex.Query, intersect *pb.List) (*pb.List, error) {
	var results *pb.List
	opts := posting.ListOptions{
		ReadTs:   arg.q.ReadTs,
		First:    int(arg.q.First),
		AfterUid: arg.q.AfterUid,
	}
	if proto.Size(intersect) > 0 {
		opts.Intersect = intersect
	}

	uidsForTrigram := func(trigram string) (*pb.List, error) {
		key := x.IndexKey(attr, trigram)
		pl, err := posting.GetNoStore(key, arg.q.ReadTs)
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

			if proto.Size(results) == 0 {
				return results, nil
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
			if proto.Size(results) == 0 {
				return results, nil
			}
		}
	case cindex.QOr:
		tok.EncodeRegexTokens(query.Trigram)
		uidMatrix := make([]*pb.List, len(query.Trigram))
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
			results = algo.MergeSorted([]*pb.List{results, subUids})
		}
	default:
		return nil, errRegexTooWide
	}
	return results, nil
}
