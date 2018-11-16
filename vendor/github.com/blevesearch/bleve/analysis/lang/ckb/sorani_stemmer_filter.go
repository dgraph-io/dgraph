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

package ckb

import (
	"bytes"
	"unicode/utf8"

	"github.com/blevesearch/bleve/analysis"
	"github.com/blevesearch/bleve/registry"
)

const StemmerName = "stemmer_ckb"

type SoraniStemmerFilter struct {
}

func NewSoraniStemmerFilter() *SoraniStemmerFilter {
	return &SoraniStemmerFilter{}
}

func (s *SoraniStemmerFilter) Filter(input analysis.TokenStream) analysis.TokenStream {
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

	// postposition
	if inputLen > 5 && bytes.HasSuffix(input, []byte("دا")) {
		input = truncateRunes(input, 2)
		inputLen = utf8.RuneCount(input)
	} else if inputLen > 4 && bytes.HasSuffix(input, []byte("نا")) {
		input = truncateRunes(input, 1)
		inputLen = utf8.RuneCount(input)
	} else if inputLen > 6 && bytes.HasSuffix(input, []byte("ەوە")) {
		input = truncateRunes(input, 3)
		inputLen = utf8.RuneCount(input)
	}

	// possessive pronoun
	if inputLen > 6 &&
		(bytes.HasSuffix(input, []byte("مان")) ||
			bytes.HasSuffix(input, []byte("یان")) ||
			bytes.HasSuffix(input, []byte("تان"))) {
		input = truncateRunes(input, 3)
		inputLen = utf8.RuneCount(input)
	}

	// indefinite singular ezafe
	if inputLen > 6 && bytes.HasSuffix(input, []byte("ێکی")) {
		return truncateRunes(input, 3)
	} else if inputLen > 7 && bytes.HasSuffix(input, []byte("یەکی")) {
		return truncateRunes(input, 4)
	}

	if inputLen > 5 && bytes.HasSuffix(input, []byte("ێک")) {
		// indefinite singular
		return truncateRunes(input, 2)
	} else if inputLen > 6 && bytes.HasSuffix(input, []byte("یەک")) {
		// indefinite singular
		return truncateRunes(input, 3)
	} else if inputLen > 6 && bytes.HasSuffix(input, []byte("ەکە")) {
		// definite singular
		return truncateRunes(input, 3)
	} else if inputLen > 5 && bytes.HasSuffix(input, []byte("کە")) {
		// definite singular
		return truncateRunes(input, 2)
	} else if inputLen > 7 && bytes.HasSuffix(input, []byte("ەکان")) {
		// definite plural
		return truncateRunes(input, 4)
	} else if inputLen > 6 && bytes.HasSuffix(input, []byte("کان")) {
		// definite plural
		return truncateRunes(input, 3)
	} else if inputLen > 7 && bytes.HasSuffix(input, []byte("یانی")) {
		// indefinite plural ezafe
		return truncateRunes(input, 4)
	} else if inputLen > 6 && bytes.HasSuffix(input, []byte("انی")) {
		// indefinite plural ezafe
		return truncateRunes(input, 3)
	} else if inputLen > 6 && bytes.HasSuffix(input, []byte("یان")) {
		// indefinite plural
		return truncateRunes(input, 3)
	} else if inputLen > 5 && bytes.HasSuffix(input, []byte("ان")) {
		// indefinite plural
		return truncateRunes(input, 2)
	} else if inputLen > 7 && bytes.HasSuffix(input, []byte("یانە")) {
		// demonstrative plural
		return truncateRunes(input, 4)
	} else if inputLen > 6 && bytes.HasSuffix(input, []byte("انە")) {
		// demonstrative plural
		return truncateRunes(input, 3)
	} else if inputLen > 5 && (bytes.HasSuffix(input, []byte("ایە")) || bytes.HasSuffix(input, []byte("ەیە"))) {
		// demonstrative singular
		return truncateRunes(input, 2)
	} else if inputLen > 4 && bytes.HasSuffix(input, []byte("ە")) {
		// demonstrative singular
		return truncateRunes(input, 1)
	} else if inputLen > 4 && bytes.HasSuffix(input, []byte("ی")) {
		// absolute singular ezafe
		return truncateRunes(input, 1)
	}
	return input
}

func truncateRunes(input []byte, num int) []byte {
	runes := bytes.Runes(input)
	runes = runes[:len(runes)-num]
	out := buildTermFromRunes(runes)
	return out
}

func buildTermFromRunes(runes []rune) []byte {
	rv := make([]byte, 0, len(runes)*4)
	for _, r := range runes {
		runeBytes := make([]byte, utf8.RuneLen(r))
		utf8.EncodeRune(runeBytes, r)
		rv = append(rv, runeBytes...)
	}
	return rv
}

func StemmerFilterConstructor(config map[string]interface{}, cache *registry.Cache) (analysis.TokenFilter, error) {
	return NewSoraniStemmerFilter(), nil
}

func init() {
	registry.RegisterTokenFilter(StemmerName, StemmerFilterConstructor)
}
