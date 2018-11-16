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
	"unicode"

	"github.com/blevesearch/bleve/analysis"
	"github.com/blevesearch/bleve/registry"
)

const LightStemmerName = "stemmer_fr_light"

type FrenchLightStemmerFilter struct {
}

func NewFrenchLightStemmerFilter() *FrenchLightStemmerFilter {
	return &FrenchLightStemmerFilter{}
}

func (s *FrenchLightStemmerFilter) Filter(input analysis.TokenStream) analysis.TokenStream {
	for _, token := range input {
		runes := bytes.Runes(token.Term)
		runes = stem(runes)
		token.Term = analysis.BuildTermFromRunes(runes)
	}
	return input
}

func stem(input []rune) []rune {

	inputLen := len(input)

	if inputLen > 5 && input[inputLen-1] == 'x' {
		if input[inputLen-3] == 'a' && input[inputLen-2] == 'u' && input[inputLen-4] != 'e' {
			input[inputLen-2] = 'l'
		}
		input = input[0 : inputLen-1]
		inputLen = len(input)
	}

	if inputLen > 3 && input[inputLen-1] == 'x' {
		input = input[0 : inputLen-1]
		inputLen = len(input)
	}

	if inputLen > 3 && input[inputLen-1] == 's' {
		input = input[0 : inputLen-1]
		inputLen = len(input)
	}

	if inputLen > 9 && analysis.RunesEndsWith(input, "issement") {
		input = input[0 : inputLen-6]
		inputLen = len(input)
		input[inputLen-1] = 'r'
		return norm(input)
	}

	if inputLen > 8 && analysis.RunesEndsWith(input, "issant") {
		input = input[0 : inputLen-4]
		inputLen = len(input)
		input[inputLen-1] = 'r'
		return norm(input)
	}

	if inputLen > 6 && analysis.RunesEndsWith(input, "ement") {
		input = input[0 : inputLen-4]
		inputLen = len(input)
		if inputLen > 3 && analysis.RunesEndsWith(input, "ive") {
			input = input[0 : inputLen-1]
			inputLen = len(input)
			input[inputLen-1] = 'f'
		}
		return norm(input)
	}

	if inputLen > 11 && analysis.RunesEndsWith(input, "ficatrice") {
		input = input[0 : inputLen-5]
		inputLen = len(input)
		input[inputLen-2] = 'e'
		input[inputLen-1] = 'r'
		return norm(input)
	}

	if inputLen > 10 && analysis.RunesEndsWith(input, "ficateur") {
		input = input[0 : inputLen-4]
		inputLen = len(input)
		input[inputLen-2] = 'e'
		input[inputLen-1] = 'r'
		return norm(input)
	}

	if inputLen > 9 && analysis.RunesEndsWith(input, "catrice") {
		input = input[0 : inputLen-3]
		inputLen = len(input)
		input[inputLen-4] = 'q'
		input[inputLen-3] = 'u'
		input[inputLen-2] = 'e'
		//s[len-1] = 'r' <-- unnecessary, already 'r'.
		return norm(input)
	}

	if inputLen > 8 && analysis.RunesEndsWith(input, "cateur") {
		input = input[0 : inputLen-2]
		inputLen = len(input)
		input[inputLen-4] = 'q'
		input[inputLen-3] = 'u'
		input[inputLen-2] = 'e'
		input[inputLen-1] = 'r'
		return norm(input)
	}

	if inputLen > 8 && analysis.RunesEndsWith(input, "atrice") {
		input = input[0 : inputLen-4]
		inputLen = len(input)
		input[inputLen-2] = 'e'
		input[inputLen-1] = 'r'
		return norm(input)
	}

	if inputLen > 7 && analysis.RunesEndsWith(input, "ateur") {
		input = input[0 : inputLen-3]
		inputLen = len(input)
		input[inputLen-2] = 'e'
		input[inputLen-1] = 'r'
		return norm(input)
	}

	if inputLen > 6 && analysis.RunesEndsWith(input, "trice") {
		input = input[0 : inputLen-1]
		inputLen = len(input)
		input[inputLen-3] = 'e'
		input[inputLen-2] = 'u'
		input[inputLen-1] = 'r'
	}

	if inputLen > 5 && analysis.RunesEndsWith(input, "ième") {
		return norm(input[0 : inputLen-4])
	}

	if inputLen > 7 && analysis.RunesEndsWith(input, "teuse") {
		input = input[0 : inputLen-2]
		inputLen = len(input)
		input[inputLen-1] = 'r'
		return norm(input)
	}

	if inputLen > 6 && analysis.RunesEndsWith(input, "teur") {
		input = input[0 : inputLen-1]
		inputLen = len(input)
		input[inputLen-1] = 'r'
		return norm(input)
	}

	if inputLen > 5 && analysis.RunesEndsWith(input, "euse") {
		return norm(input[0 : inputLen-2])
	}

	if inputLen > 8 && analysis.RunesEndsWith(input, "ère") {
		input = input[0 : inputLen-1]
		inputLen = len(input)
		input[inputLen-2] = 'e'
		return norm(input)
	}

	if inputLen > 7 && analysis.RunesEndsWith(input, "ive") {
		input = input[0 : inputLen-1]
		inputLen = len(input)
		input[inputLen-1] = 'f'
		return norm(input)
	}

	if inputLen > 4 &&
		(analysis.RunesEndsWith(input, "folle") ||
			analysis.RunesEndsWith(input, "molle")) {
		input = input[0 : inputLen-2]
		inputLen = len(input)
		input[inputLen-1] = 'u'
		return norm(input)
	}

	if inputLen > 9 && analysis.RunesEndsWith(input, "nnelle") {
		return norm(input[0 : inputLen-5])
	}

	if inputLen > 9 && analysis.RunesEndsWith(input, "nnel") {
		return norm(input[0 : inputLen-3])
	}

	if inputLen > 4 && analysis.RunesEndsWith(input, "ète") {
		input = input[0 : inputLen-1]
		inputLen = len(input)
		input[inputLen-2] = 'e'
	}

	if inputLen > 8 && analysis.RunesEndsWith(input, "ique") {
		input = input[0 : inputLen-4]
		inputLen = len(input)
	}

	if inputLen > 8 && analysis.RunesEndsWith(input, "esse") {
		return norm(input[0 : inputLen-3])
	}

	if inputLen > 7 && analysis.RunesEndsWith(input, "inage") {
		return norm(input[0 : inputLen-3])
	}

	if inputLen > 9 && analysis.RunesEndsWith(input, "isation") {
		input = input[0 : inputLen-7]
		inputLen = len(input)
		if inputLen > 5 && analysis.RunesEndsWith(input, "ual") {
			input[inputLen-2] = 'e'
		}
		return norm(input)
	}

	if inputLen > 9 && analysis.RunesEndsWith(input, "isateur") {
		return norm(input[0 : inputLen-7])
	}

	if inputLen > 8 && analysis.RunesEndsWith(input, "ation") {
		return norm(input[0 : inputLen-5])
	}

	if inputLen > 8 && analysis.RunesEndsWith(input, "ition") {
		return norm(input[0 : inputLen-5])
	}

	return norm(input)

}

func norm(input []rune) []rune {

	if len(input) > 4 {
		for i := 0; i < len(input); i++ {
			switch input[i] {
			case 'à', 'á', 'â':
				input[i] = 'a'
			case 'ô':
				input[i] = 'o'
			case 'è', 'é', 'ê':
				input[i] = 'e'
			case 'ù', 'û':
				input[i] = 'u'
			case 'î':
				input[i] = 'i'
			case 'ç':
				input[i] = 'c'
			}

			ch := input[0]
			for i := 1; i < len(input); i++ {
				if input[i] == ch && unicode.IsLetter(ch) {
					input = analysis.DeleteRune(input, i)
					i -= 1
				} else {
					ch = input[i]
				}
			}
		}
	}

	if len(input) > 4 && analysis.RunesEndsWith(input, "ie") {
		input = input[0 : len(input)-2]
	}

	if len(input) > 4 {
		if input[len(input)-1] == 'r' {
			input = input[0 : len(input)-1]
		}
		if input[len(input)-1] == 'e' {
			input = input[0 : len(input)-1]
		}
		if input[len(input)-1] == 'e' {
			input = input[0 : len(input)-1]
		}
		if input[len(input)-1] == input[len(input)-2] && unicode.IsLetter(input[len(input)-1]) {
			input = input[0 : len(input)-1]
		}
	}

	return input
}

func FrenchLightStemmerFilterConstructor(config map[string]interface{}, cache *registry.Cache) (analysis.TokenFilter, error) {
	return NewFrenchLightStemmerFilter(), nil
}

func init() {
	registry.RegisterTokenFilter(LightStemmerName, FrenchLightStemmerFilterConstructor)
}
