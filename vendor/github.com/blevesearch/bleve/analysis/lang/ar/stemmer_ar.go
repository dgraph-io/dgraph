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

package ar

import (
	"bytes"

	"github.com/blevesearch/bleve/analysis"
	"github.com/blevesearch/bleve/registry"
)

const StemmerName = "stemmer_ar"

// These were obtained from org.apache.lucene.analysis.ar.ArabicStemmer
var prefixes = [][]rune{
	[]rune("ال"),
	[]rune("وال"),
	[]rune("بال"),
	[]rune("كال"),
	[]rune("فال"),
	[]rune("لل"),
	[]rune("و"),
}
var suffixes = [][]rune{
	[]rune("ها"),
	[]rune("ان"),
	[]rune("ات"),
	[]rune("ون"),
	[]rune("ين"),
	[]rune("يه"),
	[]rune("ية"),
	[]rune("ه"),
	[]rune("ة"),
	[]rune("ي"),
}

type ArabicStemmerFilter struct{}

func NewArabicStemmerFilter() *ArabicStemmerFilter {
	return &ArabicStemmerFilter{}
}

func (s *ArabicStemmerFilter) Filter(input analysis.TokenStream) analysis.TokenStream {
	for _, token := range input {
		term := stem(token.Term)
		token.Term = term
	}
	return input
}

func canStemPrefix(input, prefix []rune) bool {
	// Wa- prefix requires at least 3 characters.
	if len(prefix) == 1 && len(input) < 4 {
		return false
	}
	// Other prefixes require only 2.
	if len(input)-len(prefix) < 2 {
		return false
	}
	for i := range prefix {
		if prefix[i] != input[i] {
			return false
		}
	}
	return true
}

func canStemSuffix(input, suffix []rune) bool {
	// All suffixes require at least 2 characters after stemming.
	if len(input)-len(suffix) < 2 {
		return false
	}
	stemEnd := len(input) - len(suffix)
	for i := range suffix {
		if suffix[i] != input[stemEnd+i] {
			return false
		}
	}
	return true
}

func stem(input []byte) []byte {
	runes := bytes.Runes(input)
	// Strip a single prefix.
	for _, p := range prefixes {
		if canStemPrefix(runes, p) {
			runes = runes[len(p):]
			break
		}
	}
	// Strip off multiple suffixes, in their order in the suffixes array.
	for _, s := range suffixes {
		if canStemSuffix(runes, s) {
			runes = runes[:len(runes)-len(s)]
		}
	}
	return analysis.BuildTermFromRunes(runes)
}

func StemmerFilterConstructor(config map[string]interface{}, cache *registry.Cache) (analysis.TokenFilter, error) {
	return NewArabicStemmerFilter(), nil
}

func init() {
	registry.RegisterTokenFilter(StemmerName, StemmerFilterConstructor)
}
