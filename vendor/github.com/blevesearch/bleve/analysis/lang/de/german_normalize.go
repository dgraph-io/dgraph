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

const NormalizeName = "normalize_de"

const (
	N = 0 /* ordinary state */
	V = 1 /* stops 'u' from entering umlaut state */
	U = 2 /* umlaut state, allows e-deletion */
)

type GermanNormalizeFilter struct {
}

func NewGermanNormalizeFilter() *GermanNormalizeFilter {
	return &GermanNormalizeFilter{}
}

func (s *GermanNormalizeFilter) Filter(input analysis.TokenStream) analysis.TokenStream {
	for _, token := range input {
		term := normalize(token.Term)
		token.Term = term
	}
	return input
}

func normalize(input []byte) []byte {
	state := N
	runes := bytes.Runes(input)
	for i := 0; i < len(runes); i++ {
		switch runes[i] {
		case 'a', 'o':
			state = U
		case 'u':
			if state == N {
				state = U
			} else {
				state = V
			}
		case 'e':
			if state == U {
				runes = analysis.DeleteRune(runes, i)
				i--
			}
			state = V
		case 'i', 'q', 'y':
			state = V
		case 'ä':
			runes[i] = 'a'
			state = V
		case 'ö':
			runes[i] = 'o'
			state = V
		case 'ü':
			runes[i] = 'u'
			state = V
		case 'ß':
			runes[i] = 's'
			i++
			runes = analysis.InsertRune(runes, i, 's')
			state = N
		default:
			state = N
		}
	}
	return analysis.BuildTermFromRunes(runes)
}

func NormalizerFilterConstructor(config map[string]interface{}, cache *registry.Cache) (analysis.TokenFilter, error) {
	return NewGermanNormalizeFilter(), nil
}

func init() {
	registry.RegisterTokenFilter(NormalizeName, NormalizerFilterConstructor)
}
