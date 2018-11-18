//  Copyright (c) 2014 Couchbase, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// 		http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package apostrophe

import (
	"bytes"

	"github.com/blevesearch/bleve/analysis"
	"github.com/blevesearch/bleve/registry"
)

const Name = "apostrophe"

const RightSingleQuotationMark = "’"
const Apostrophe = "'"
const Apostrophes = Apostrophe + RightSingleQuotationMark

type ApostropheFilter struct{}

func NewApostropheFilter() *ApostropheFilter {
	return &ApostropheFilter{}
}

func (s *ApostropheFilter) Filter(input analysis.TokenStream) analysis.TokenStream {
	for _, token := range input {
		firstApostrophe := bytes.IndexAny(token.Term, Apostrophes)
		if firstApostrophe >= 0 {
			// found an apostrophe
			token.Term = token.Term[0:firstApostrophe]
		}
	}

	return input
}

func ApostropheFilterConstructor(config map[string]interface{}, cache *registry.Cache) (analysis.TokenFilter, error) {
	return NewApostropheFilter(), nil
}

func init() {
	registry.RegisterTokenFilter(Name, ApostropheFilterConstructor)
}
