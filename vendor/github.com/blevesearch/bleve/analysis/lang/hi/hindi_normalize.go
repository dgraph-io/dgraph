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

	"github.com/blevesearch/bleve/analysis"
	"github.com/blevesearch/bleve/registry"
)

const NormalizeName = "normalize_hi"

type HindiNormalizeFilter struct {
}

func NewHindiNormalizeFilter() *HindiNormalizeFilter {
	return &HindiNormalizeFilter{}
}

func (s *HindiNormalizeFilter) Filter(input analysis.TokenStream) analysis.TokenStream {
	for _, token := range input {
		term := normalize(token.Term)
		token.Term = term
	}
	return input
}

func normalize(input []byte) []byte {
	runes := bytes.Runes(input)
	for i := 0; i < len(runes); i++ {
		switch runes[i] {
		// dead n -> bindu
		case '\u0928':
			if i+1 < len(runes) && runes[i+1] == '\u094D' {
				runes[i] = '\u0902'
				runes = analysis.DeleteRune(runes, i+1)
			}
		// candrabindu -> bindu
		case '\u0901':
			runes[i] = '\u0902'
		// nukta deletions
		case '\u093C':
			runes = analysis.DeleteRune(runes, i)
			i--
		case '\u0929':
			runes[i] = '\u0928'
		case '\u0931':
			runes[i] = '\u0930'
		case '\u0934':
			runes[i] = '\u0933'
		case '\u0958':
			runes[i] = '\u0915'
		case '\u0959':
			runes[i] = '\u0916'
		case '\u095A':
			runes[i] = '\u0917'
		case '\u095B':
			runes[i] = '\u091C'
		case '\u095C':
			runes[i] = '\u0921'
		case '\u095D':
			runes[i] = '\u0922'
		case '\u095E':
			runes[i] = '\u092B'
		case '\u095F':
			runes[i] = '\u092F'
			// zwj/zwnj -> delete
		case '\u200D', '\u200C':
			runes = analysis.DeleteRune(runes, i)
			i--
			// virama -> delete
		case '\u094D':
			runes = analysis.DeleteRune(runes, i)
			i--
			// chandra/short -> replace
		case '\u0945', '\u0946':
			runes[i] = '\u0947'
		case '\u0949', '\u094A':
			runes[i] = '\u094B'
		case '\u090D', '\u090E':
			runes[i] = '\u090F'
		case '\u0911', '\u0912':
			runes[i] = '\u0913'
		case '\u0972':
			runes[i] = '\u0905'
			// long -> short ind. vowels
		case '\u0906':
			runes[i] = '\u0905'
		case '\u0908':
			runes[i] = '\u0907'
		case '\u090A':
			runes[i] = '\u0909'
		case '\u0960':
			runes[i] = '\u090B'
		case '\u0961':
			runes[i] = '\u090C'
		case '\u0910':
			runes[i] = '\u090F'
		case '\u0914':
			runes[i] = '\u0913'
			// long -> short dep. vowels
		case '\u0940':
			runes[i] = '\u093F'
		case '\u0942':
			runes[i] = '\u0941'
		case '\u0944':
			runes[i] = '\u0943'
		case '\u0963':
			runes[i] = '\u0962'
		case '\u0948':
			runes[i] = '\u0947'
		case '\u094C':
			runes[i] = '\u094B'
		}
	}
	return analysis.BuildTermFromRunes(runes)
}

func NormalizerFilterConstructor(config map[string]interface{}, cache *registry.Cache) (analysis.TokenFilter, error) {
	return NewHindiNormalizeFilter(), nil
}

func init() {
	registry.RegisterTokenFilter(NormalizeName, NormalizerFilterConstructor)
}
