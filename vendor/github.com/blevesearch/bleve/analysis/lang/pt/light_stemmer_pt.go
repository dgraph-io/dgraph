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

package pt

import (
	"bytes"

	"github.com/blevesearch/bleve/analysis"
	"github.com/blevesearch/bleve/registry"
)

const LightStemmerName = "stemmer_pt_light"

type PortugueseLightStemmerFilter struct {
}

func NewPortugueseLightStemmerFilter() *PortugueseLightStemmerFilter {
	return &PortugueseLightStemmerFilter{}
}

func (s *PortugueseLightStemmerFilter) Filter(input analysis.TokenStream) analysis.TokenStream {
	for _, token := range input {
		runes := bytes.Runes(token.Term)
		runes = stem(runes)
		token.Term = analysis.BuildTermFromRunes(runes)
	}
	return input
}

func stem(input []rune) []rune {

	inputLen := len(input)

	if inputLen < 4 {
		return input
	}

	input = removeSuffix(input)
	inputLen = len(input)

	if inputLen > 3 && input[inputLen-1] == 'a' {
		input = normFeminine(input)
		inputLen = len(input)
	}

	if inputLen > 4 {
		switch input[inputLen-1] {
		case 'e', 'a', 'o':
			input = input[0 : inputLen-1]
			inputLen = len(input)
		}
	}

	for i := 0; i < inputLen; i++ {
		switch input[i] {
		case 'à', 'á', 'â', 'ä', 'ã':
			input[i] = 'a'
		case 'ò', 'ó', 'ô', 'ö', 'õ':
			input[i] = 'o'
		case 'è', 'é', 'ê', 'ë':
			input[i] = 'e'
		case 'ù', 'ú', 'û', 'ü':
			input[i] = 'u'
		case 'ì', 'í', 'î', 'ï':
			input[i] = 'i'
		case 'ç':
			input[i] = 'c'
		}
	}

	return input
}

func removeSuffix(input []rune) []rune {

	inputLen := len(input)

	if inputLen > 4 && analysis.RunesEndsWith(input, "es") {
		switch input[inputLen-3] {
		case 'r', 's', 'l', 'z':
			return input[0 : inputLen-2]
		}
	}

	if inputLen > 3 && analysis.RunesEndsWith(input, "ns") {
		input[inputLen-2] = 'm'
		return input[0 : inputLen-1]
	}

	if inputLen > 4 && (analysis.RunesEndsWith(input, "eis") || analysis.RunesEndsWith(input, "éis")) {
		input[inputLen-3] = 'e'
		input[inputLen-2] = 'l'
		return input[0 : inputLen-1]
	}

	if inputLen > 4 && analysis.RunesEndsWith(input, "ais") {
		input[inputLen-2] = 'l'
		return input[0 : inputLen-1]
	}

	if inputLen > 4 && analysis.RunesEndsWith(input, "óis") {
		input[inputLen-3] = 'o'
		input[inputLen-2] = 'l'
		return input[0 : inputLen-1]
	}

	if inputLen > 4 && analysis.RunesEndsWith(input, "is") {
		input[inputLen-1] = 'l'
		return input
	}

	if inputLen > 3 &&
		(analysis.RunesEndsWith(input, "ões") ||
			analysis.RunesEndsWith(input, "ães")) {
		input = input[0 : inputLen-1]
		inputLen = len(input)
		input[inputLen-2] = 'ã'
		input[inputLen-1] = 'o'
		return input
	}

	if inputLen > 6 && analysis.RunesEndsWith(input, "mente") {
		return input[0 : inputLen-5]
	}

	if inputLen > 3 && input[inputLen-1] == 's' {
		return input[0 : inputLen-1]
	}
	return input
}

func normFeminine(input []rune) []rune {
	inputLen := len(input)

	if inputLen > 7 &&
		(analysis.RunesEndsWith(input, "inha") ||
			analysis.RunesEndsWith(input, "iaca") ||
			analysis.RunesEndsWith(input, "eira")) {
		input[inputLen-1] = 'o'
		return input
	}

	if inputLen > 6 {
		if analysis.RunesEndsWith(input, "osa") ||
			analysis.RunesEndsWith(input, "ica") ||
			analysis.RunesEndsWith(input, "ida") ||
			analysis.RunesEndsWith(input, "ada") ||
			analysis.RunesEndsWith(input, "iva") ||
			analysis.RunesEndsWith(input, "ama") {
			input[inputLen-1] = 'o'
			return input
		}

		if analysis.RunesEndsWith(input, "ona") {
			input[inputLen-3] = 'ã'
			input[inputLen-2] = 'o'
			return input[0 : inputLen-1]
		}

		if analysis.RunesEndsWith(input, "ora") {
			return input[0 : inputLen-1]
		}

		if analysis.RunesEndsWith(input, "esa") {
			input[inputLen-3] = 'ê'
			return input[0 : inputLen-1]
		}

		if analysis.RunesEndsWith(input, "na") {
			input[inputLen-1] = 'o'
			return input
		}
	}
	return input
}

func PortugueseLightStemmerFilterConstructor(config map[string]interface{}, cache *registry.Cache) (analysis.TokenFilter, error) {
	return NewPortugueseLightStemmerFilter(), nil
}

func init() {
	registry.RegisterTokenFilter(LightStemmerName, PortugueseLightStemmerFilterConstructor)
}
