//  Copyright (c) 2018 Couchbase, Inc.
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

package no

import (
	"github.com/blevesearch/bleve/analysis"
	"github.com/blevesearch/bleve/registry"

	"github.com/blevesearch/snowballstem"
	"github.com/blevesearch/snowballstem/norwegian"
)

const SnowballStemmerName = "stemmer_no_snowball"

type NorwegianStemmerFilter struct {
}

func NewNorwegianStemmerFilter() *NorwegianStemmerFilter {
	return &NorwegianStemmerFilter{}
}

func (s *NorwegianStemmerFilter) Filter(input analysis.TokenStream) analysis.TokenStream {
	for _, token := range input {
		env := snowballstem.NewEnv(string(token.Term))
		norwegian.Stem(env)
		token.Term = []byte(env.Current())
	}
	return input
}

func NorwegianStemmerFilterConstructor(config map[string]interface{}, cache *registry.Cache) (analysis.TokenFilter, error) {
	return NewNorwegianStemmerFilter(), nil
}

func init() {
	registry.RegisterTokenFilter(SnowballStemmerName, NorwegianStemmerFilterConstructor)
}
