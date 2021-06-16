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
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/tok"
	"github.com/dgraph-io/dgraph/x"
	"github.com/dgraph-io/sroar"
)

var errRegexTooWide = errors.New(
	"regular expression is too wide-ranging and can't be executed efficiently")

func uidsForRegex(attr string, arg funcArgs,
	query *cindex.Query, intersect *sroar.Bitmap) (*sroar.Bitmap, error) {

	opts := posting.ListOptions{
		ReadTs:   arg.q.ReadTs,
		First:    int(arg.q.First),
		AfterUid: arg.q.AfterUid,
	}
	// TODO: Unnecessary conversion here. Avoid if possible.
	if !intersect.IsEmpty() {
		opts.Intersect = &pb.List{
			Bitmap: codec.ToBytes(intersect),
		}
	} else {
		intersect = sroar.NewBitmap()
	}

	uidsForTrigram := func(trigram string) (*sroar.Bitmap, error) {
		key := x.IndexKey(attr, trigram)
		pl, err := posting.GetNoStore(key, arg.q.ReadTs)
		if err != nil {
			return nil, err
		}
		return pl.Bitmap(opts)
	}

	results := sroar.NewBitmap()
	switch query.Op {
	case cindex.QAnd:
		tok.EncodeRegexTokens(query.Trigram)
		for _, t := range query.Trigram {
			trigramUids, err := uidsForTrigram(t)
			if err != nil {
				return nil, err
			}
			if results.IsEmpty() {
				results = trigramUids
			} else {
				results.And(trigramUids)
			}

			if results.IsEmpty() {
				return results, nil
			}
		}
		for _, sub := range query.Sub {
			if results.IsEmpty() {
				results = intersect
			}
			// current list of result is passed for intersection
			var err error
			results, err = uidsForRegex(attr, arg, sub, results)
			if err != nil {
				return nil, err
			}
			if results.IsEmpty() {
				return results, nil
			}
		}
	case cindex.QOr:
		tok.EncodeRegexTokens(query.Trigram)
		for _, t := range query.Trigram {
			out, err := uidsForTrigram(t)
			if err != nil {
				return nil, err
			}
			if results.IsEmpty() {
				results = out
			} else {
				results.Or(out)
			}
		}
		for _, sub := range query.Sub {
			if results.IsEmpty() {
				results = intersect
			}
			// Looks like this won't take the results for intersect, but use the originally passed
			// intersect itself.
			subUids, err := uidsForRegex(attr, arg, sub, intersect)
			if err != nil {
				return nil, err
			}
			if subUids != nil {
				results.Or(subUids)
			}
		}
	default:
		return nil, errRegexTooWide
	}
	return results, nil
}
