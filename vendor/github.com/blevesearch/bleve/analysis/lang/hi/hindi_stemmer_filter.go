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

package hi

import (
	"bytes"
	"unicode/utf8"

	"github.com/blevesearch/bleve/analysis"
	"github.com/blevesearch/bleve/registry"
)

const StemmerName = "stemmer_hi"

type HindiStemmerFilter struct {
}

func NewHindiStemmerFilter() *HindiStemmerFilter {
	return &HindiStemmerFilter{}
}

func (s *HindiStemmerFilter) Filter(input analysis.TokenStream) analysis.TokenStream {
	for _, token := range input {
		// if not protected keyword, stem it
		if !token.KeyWord {
			stemmed := stem(token.Term)
			token.Term = stemmed
		}
	}
	return input
}

func stem(input []byte) []byte {
	inputLen := utf8.RuneCount(input)

	// 5
	if inputLen > 6 &&
		(bytes.HasSuffix(input, []byte("ाएंगी")) ||
			bytes.HasSuffix(input, []byte("ाएंगे")) ||
			bytes.HasSuffix(input, []byte("ाऊंगी")) ||
			bytes.HasSuffix(input, []byte("ाऊंगा")) ||
			bytes.HasSuffix(input, []byte("ाइयाँ")) ||
			bytes.HasSuffix(input, []byte("ाइयों")) ||
			bytes.HasSuffix(input, []byte("ाइयां"))) {
		return analysis.TruncateRunes(input, 5)
	}

	// 4
	if inputLen > 5 &&
		(bytes.HasSuffix(input, []byte("ाएगी")) ||
			bytes.HasSuffix(input, []byte("ाएगा")) ||
			bytes.HasSuffix(input, []byte("ाओगी")) ||
			bytes.HasSuffix(input, []byte("ाओगे")) ||
			bytes.HasSuffix(input, []byte("एंगी")) ||
			bytes.HasSuffix(input, []byte("ेंगी")) ||
			bytes.HasSuffix(input, []byte("एंगे")) ||
			bytes.HasSuffix(input, []byte("ेंगे")) ||
			bytes.HasSuffix(input, []byte("ूंगी")) ||
			bytes.HasSuffix(input, []byte("ूंगा")) ||
			bytes.HasSuffix(input, []byte("ातीं")) ||
			bytes.HasSuffix(input, []byte("नाओं")) ||
			bytes.HasSuffix(input, []byte("नाएं")) ||
			bytes.HasSuffix(input, []byte("ताओं")) ||
			bytes.HasSuffix(input, []byte("ताएं")) ||
			bytes.HasSuffix(input, []byte("ियाँ")) ||
			bytes.HasSuffix(input, []byte("ियों")) ||
			bytes.HasSuffix(input, []byte("ियां"))) {
		return analysis.TruncateRunes(input, 4)
	}

	// 3
	if inputLen > 4 &&
		(bytes.HasSuffix(input, []byte("ाकर")) ||
			bytes.HasSuffix(input, []byte("ाइए")) ||
			bytes.HasSuffix(input, []byte("ाईं")) ||
			bytes.HasSuffix(input, []byte("ाया")) ||
			bytes.HasSuffix(input, []byte("ेगी")) ||
			bytes.HasSuffix(input, []byte("ेगा")) ||
			bytes.HasSuffix(input, []byte("ोगी")) ||
			bytes.HasSuffix(input, []byte("ोगे")) ||
			bytes.HasSuffix(input, []byte("ाने")) ||
			bytes.HasSuffix(input, []byte("ाना")) ||
			bytes.HasSuffix(input, []byte("ाते")) ||
			bytes.HasSuffix(input, []byte("ाती")) ||
			bytes.HasSuffix(input, []byte("ाता")) ||
			bytes.HasSuffix(input, []byte("तीं")) ||
			bytes.HasSuffix(input, []byte("ाओं")) ||
			bytes.HasSuffix(input, []byte("ाएं")) ||
			bytes.HasSuffix(input, []byte("ुओं")) ||
			bytes.HasSuffix(input, []byte("ुएं")) ||
			bytes.HasSuffix(input, []byte("ुआं"))) {
		return analysis.TruncateRunes(input, 3)
	}

	// 2
	if inputLen > 3 &&
		(bytes.HasSuffix(input, []byte("कर")) ||
			bytes.HasSuffix(input, []byte("ाओ")) ||
			bytes.HasSuffix(input, []byte("िए")) ||
			bytes.HasSuffix(input, []byte("ाई")) ||
			bytes.HasSuffix(input, []byte("ाए")) ||
			bytes.HasSuffix(input, []byte("ने")) ||
			bytes.HasSuffix(input, []byte("नी")) ||
			bytes.HasSuffix(input, []byte("ना")) ||
			bytes.HasSuffix(input, []byte("ते")) ||
			bytes.HasSuffix(input, []byte("ीं")) ||
			bytes.HasSuffix(input, []byte("ती")) ||
			bytes.HasSuffix(input, []byte("ता")) ||
			bytes.HasSuffix(input, []byte("ाँ")) ||
			bytes.HasSuffix(input, []byte("ां")) ||
			bytes.HasSuffix(input, []byte("ों")) ||
			bytes.HasSuffix(input, []byte("ें"))) {
		return analysis.TruncateRunes(input, 2)
	}

	// 1
	if inputLen > 2 &&
		(bytes.HasSuffix(input, []byte("ो")) ||
			bytes.HasSuffix(input, []byte("े")) ||
			bytes.HasSuffix(input, []byte("ू")) ||
			bytes.HasSuffix(input, []byte("ु")) ||
			bytes.HasSuffix(input, []byte("ी")) ||
			bytes.HasSuffix(input, []byte("ि")) ||
			bytes.HasSuffix(input, []byte("ा"))) {
		return analysis.TruncateRunes(input, 1)
	}

	return input
}

func StemmerFilterConstructor(config map[string]interface{}, cache *registry.Cache) (analysis.TokenFilter, error) {
	return NewHindiStemmerFilter(), nil
}

func init() {
	registry.RegisterTokenFilter(StemmerName, StemmerFilterConstructor)
}
