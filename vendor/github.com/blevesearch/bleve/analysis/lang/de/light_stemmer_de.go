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

package de

import (
	"bytes"

	"github.com/blevesearch/bleve/analysis"
	"github.com/blevesearch/bleve/registry"
)

const LightStemmerName = "stemmer_de_light"

type GermanLightStemmerFilter struct {
}

func NewGermanLightStemmerFilter() *GermanLightStemmerFilter {
	return &GermanLightStemmerFilter{}
}

func (s *GermanLightStemmerFilter) Filter(input analysis.TokenStream) analysis.TokenStream {
	for _, token := range input {
		runes := bytes.Runes(token.Term)
		runes = stem(runes)
		token.Term = analysis.BuildTermFromRunes(runes)
	}
	return input
}

func stem(input []rune) []rune {

	for i, r := range input {
		switch r {
		case 'ä', 'à', 'á', 'â':
			input[i] = 'a'
		case 'ö', 'ò', 'ó', 'ô':
			input[i] = 'o'
		case 'ï', 'ì', 'í', 'î':
			input[i] = 'i'
		case 'ü', 'ù', 'ú', 'û':
			input[i] = 'u'
		}
	}

	input = step1(input)
	return step2(input)
}

func stEnding(ch rune) bool {
	switch ch {
	case 'b', 'd', 'f', 'g', 'h', 'k', 'l', 'm', 'n', 't':
		return true
	}
	return false
}

func step1(s []rune) []rune {
	l := len(s)
	if l > 5 && s[l-3] == 'e' && s[l-2] == 'r' && s[l-1] == 'n' {
		return s[:l-3]
	}

	if l > 4 && s[l-2] == 'e' {
		switch s[l-1] {
		case 'm', 'n', 'r', 's':
			return s[:l-2]
		}
	}

	if l > 3 && s[l-1] == 'e' {
		return s[:l-1]
	}

	if l > 3 && s[l-1] == 's' && stEnding(s[l-2]) {
		return s[:l-1]
	}

	return s
}

func step2(s []rune) []rune {
	l := len(s)
	if l > 5 && s[l-3] == 'e' && s[l-2] == 's' && s[l-1] == 't' {
		return s[:l-3]
	}

	if l > 4 && s[l-2] == 'e' && (s[l-1] == 'r' || s[l-1] == 'n') {
		return s[:l-2]
	}

	if l > 4 && s[l-2] == 's' && s[l-1] == 't' && stEnding(s[l-3]) {
		return s[:l-2]
	}

	return s
}

func GermanLightStemmerFilterConstructor(config map[string]interface{}, cache *registry.Cache) (analysis.TokenFilter, error) {
	return NewGermanLightStemmerFilter(), nil
}

func init() {
	registry.RegisterTokenFilter(LightStemmerName, GermanLightStemmerFilterConstructor)
}
