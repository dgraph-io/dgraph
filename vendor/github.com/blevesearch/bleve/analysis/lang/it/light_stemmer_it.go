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

package it

import (
	"bytes"

	"github.com/blevesearch/bleve/analysis"
	"github.com/blevesearch/bleve/registry"
)

const LightStemmerName = "stemmer_it_light"

type ItalianLightStemmerFilter struct {
}

func NewItalianLightStemmerFilterFilter() *ItalianLightStemmerFilter {
	return &ItalianLightStemmerFilter{}
}

func (s *ItalianLightStemmerFilter) Filter(input analysis.TokenStream) analysis.TokenStream {
	for _, token := range input {
		runes := bytes.Runes(token.Term)
		runes = stem(runes)
		token.Term = analysis.BuildTermFromRunes(runes)
	}
	return input
}

func stem(input []rune) []rune {

	inputLen := len(input)

	if inputLen < 6 {
		return input
	}

	for i := 0; i < inputLen; i++ {
		switch input[i] {
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

	switch input[inputLen-1] {
	case 'e':
		if input[inputLen-2] == 'i' || input[inputLen-2] == 'h' {
			return input[0 : inputLen-2]
		} else {
			return input[0 : inputLen-1]
		}
	case 'i':
		if input[inputLen-2] == 'h' || input[inputLen-2] == 'i' {
			return input[0 : inputLen-2]
		} else {
			return input[0 : inputLen-1]
		}
	case 'a':
		if input[inputLen-2] == 'i' {
			return input[0 : inputLen-2]
		} else {
			return input[0 : inputLen-1]
		}
	case 'o':
		if input[inputLen-2] == 'i' {
			return input[0 : inputLen-2]
		} else {
			return input[0 : inputLen-1]
		}
	}

	return input
}

func ItalianLightStemmerFilterConstructor(config map[string]interface{}, cache *registry.Cache) (analysis.TokenFilter, error) {
	return NewItalianLightStemmerFilterFilter(), nil
}

func init() {
	registry.RegisterTokenFilter(LightStemmerName, ItalianLightStemmerFilterConstructor)
}
