//  Copyright (c) 2015 Couchbase, Inc.
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

package fr

import (
	"bytes"

	"github.com/blevesearch/bleve/analysis"
	"github.com/blevesearch/bleve/registry"
)

const MinimalStemmerName = "stemmer_fr_min"

type FrenchMinimalStemmerFilter struct {
}

func NewFrenchMinimalStemmerFilter() *FrenchMinimalStemmerFilter {
	return &FrenchMinimalStemmerFilter{}
}

func (s *FrenchMinimalStemmerFilter) Filter(input analysis.TokenStream) analysis.TokenStream {
	for _, token := range input {
		runes := bytes.Runes(token.Term)
		runes = minstem(runes)
		token.Term = analysis.BuildTermFromRunes(runes)
	}
	return input
}

func minstem(input []rune) []rune {

	if len(input) < 6 {
		return input
	}

	if input[len(input)-1] == 'x' {
		if input[len(input)-3] == 'a' && input[len(input)-2] == 'u' {
			input[len(input)-2] = 'l'
		}
		return input[0 : len(input)-1]
	}

	if input[len(input)-1] == 's' {
		input = input[0 : len(input)-1]
	}
	if input[len(input)-1] == 'r' {
		input = input[0 : len(input)-1]
	}
	if input[len(input)-1] == 'e' {
		input = input[0 : len(input)-1]
	}
	if input[len(input)-1] == 'é' {
		input = input[0 : len(input)-1]
	}
	if input[len(input)-1] == input[len(input)-2] {
		input = input[0 : len(input)-1]
	}
	return input
}

func FrenchMinimalStemmerFilterConstructor(config map[string]interface{}, cache *registry.Cache) (analysis.TokenFilter, error) {
	return NewFrenchMinimalStemmerFilter(), nil
}

func init() {
	registry.RegisterTokenFilter(MinimalStemmerName, FrenchMinimalStemmerFilterConstructor)
}
