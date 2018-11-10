//  Copyright (c) 2017 Couchbase, Inc.
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

package es

import (
	"bytes"

	"github.com/blevesearch/bleve/analysis"
	"github.com/blevesearch/bleve/registry"
)

const LightStemmerName = "stemmer_es_light"

type SpanishLightStemmerFilter struct {
}

func NewSpanishLightStemmerFilter() *SpanishLightStemmerFilter {
	return &SpanishLightStemmerFilter{}
}

func (s *SpanishLightStemmerFilter) Filter(
	input analysis.TokenStream) analysis.TokenStream {
	for _, token := range input {
		runes := bytes.Runes(token.Term)
		runes = stem(runes)
		token.Term = analysis.BuildTermFromRunes(runes)
	}
	return input
}

func stem(input []rune) []rune {
	l := len(input)
	if l < 5 {
		return input
	}

	for i, r := range input {
		switch r {
		case 'à', 'á', 'â', 'ä':
			input[i] = 'a'
		case 'ò', 'ó', 'ô', 'ö':
			input[i] = 'o'
		case 'è', 'é', 'ê', 'ë':
			input[i] = 'e'
		case 'ù', 'ú', 'û', 'ü':
			input[i] = 'u'
		case 'ì', 'í', 'î', 'ï':
			input[i] = 'i'
		}
	}

	switch input[l-1] {
	case 'o', 'a', 'e':
		return input[:l-1]
	case 's':
		if input[l-2] == 'e' && input[l-3] == 's' && input[l-4] == 'e' {
			return input[:l-2]
		}
		if input[l-2] == 'e' && input[l-3] == 'c' {
			input[l-3] = 'z'
			return input[:l-2]
		}
		if input[l-2] == 'o' || input[l-2] == 'a' || input[l-2] == 'e' {
			return input[:l-2]
		}
	}

	return input
}

func SpanishLightStemmerFilterConstructor(config map[string]interface{},
	cache *registry.Cache) (analysis.TokenFilter, error) {
	return NewSpanishLightStemmerFilter(), nil
}

func init() {
	registry.RegisterTokenFilter(LightStemmerName,
		SpanishLightStemmerFilterConstructor)
}
