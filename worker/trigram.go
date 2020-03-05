/*
 * Copyright 2017-2018 Dgraph Labs, Inc. and Contributors
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
	"errors"

	cindex "github.com/google/codesearch/index"

	"github.com/dgraph-io/dgraph/codec"
	"github.com/dgraph-io/dgraph/protos/pb"
)

var errRegexTooWide = errors.New(
	"regular expression is too wide-ranging and can't be executed efficiently")

// TODO: Is passing intersect here really the best way?
func uidsForRegex(attr string, arg funcArgs,
	query *cindex.Query, intersect *pb.List) (*codec.ListMap, error) {
	return nil, nil
	// opts := posting.ListOptions{
	// 	ReadTs: arg.q.ReadTs,
	// }
	// if intersect.Size() > 0 {
	// 	opts.Intersect = intersect
	// }

	// uidsForTrigram := func(trigram string) (*codec.ListMap, error) {
	// 	key := x.IndexKey(attr, trigram)
	// 	pl, err := posting.GetNoStore(key)
	// 	if err != nil {
	// 		return nil, err
	// 	}
	// 	return pl.Uids(opts)
	// }

	// switch query.Op {
	// case cindex.QAnd:
	// 	tok.EncodeRegexTokens(query.Trigram)
	// 	var results *codec.ListMap
	// 	for _, t := range query.Trigram {
	// 		trigramUids, err := uidsForTrigram(t)
	// 		if err != nil {
	// 			return nil, err
	// 		}
	// 		if results == nil {
	// 			results = trigramUids
	// 		} else {
	// 			results.Intersect(trigramUids)
	// 		}

	// 		if results.IsEmpty() {
	// 			return results, nil
	// 		}
	// 	}
	// 	for _, sub := range query.Sub {
	// 		if results == nil {
	// 			results = codec.FromListXXX(intersect)
	// 		}
	// 		// current list of result is passed for intersection
	// 		var err error
	// 		results, err = uidsForRegex(attr, arg, sub, results)
	// 		if err != nil {
	// 			return nil, err
	// 		}
	// 		if results.IsEmpty() {
	// 			return results, nil
	// 		}
	// 	}
	// case cindex.QOr:
	// 	tok.EncodeRegexTokens(query.Trigram)
	// 	results := codec.NewListMap(nil)
	// 	for i, t := range query.Trigram {
	// 		lm, err := uidsForTrigram(t)
	// 		if err != nil {
	// 			return nil, err
	// 		}
	// 		results.Merge(lm)
	// 	}
	// 	for _, sub := range query.Sub {
	// 		if results == nil {
	// 			results = intersect
	// 		}
	// 		subUids, err := uidsForRegex(attr, arg, sub, intersect)
	// 		if err != nil {
	// 			return nil, err
	// 		}
	// 		results = algo.MergeSorted([]*pb.List{results, subUids})
	// 	}
	// default:
	// 	return nil, errRegexTooWide
	// }
	// return results, nil
}
