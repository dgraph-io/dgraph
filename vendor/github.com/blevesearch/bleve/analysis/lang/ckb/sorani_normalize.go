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
	"unicode"

	"github.com/blevesearch/bleve/analysis"
	"github.com/blevesearch/bleve/registry"
)

const NormalizeName = "normalize_ckb"

const (
	Yeh        = '\u064A'
	DotlessYeh = '\u0649'
	FarsiYeh   = '\u06CC'

	Kaf   = '\u0643'
	Keheh = '\u06A9'

	Heh            = '\u0647'
	Ae             = '\u06D5'
	Zwnj           = '\u200C'
	HehDoachashmee = '\u06BE'
	TehMarbuta     = '\u0629'

	Reh       = '\u0631'
	Rreh      = '\u0695'
	RrehAbove = '\u0692'

	Tatweel  = '\u0640'
	Fathatan = '\u064B'
	Dammatan = '\u064C'
	Kasratan = '\u064D'
	Fatha    = '\u064E'
	Damma    = '\u064F'
	Kasra    = '\u0650'
	Shadda   = '\u0651'
	Sukun    = '\u0652'
)

type SoraniNormalizeFilter struct {
}

func NewSoraniNormalizeFilter() *SoraniNormalizeFilter {
	return &SoraniNormalizeFilter{}
}

func (s *SoraniNormalizeFilter) Filter(input analysis.TokenStream) analysis.TokenStream {
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
		case Yeh, DotlessYeh:
			runes[i] = FarsiYeh
		case Kaf:
			runes[i] = Keheh
		case Zwnj:
			if i > 0 && runes[i-1] == Heh {
				runes[i-1] = Ae
			}
			runes = analysis.DeleteRune(runes, i)
			i--
		case Heh:
			if i == len(runes)-1 {
				runes[i] = Ae
			}
		case TehMarbuta:
			runes[i] = Ae
		case HehDoachashmee:
			runes[i] = Heh
		case Reh:
			if i == 0 {
				runes[i] = Rreh
			}
		case RrehAbove:
			runes[i] = Rreh
		case Tatweel, Kasratan, Dammatan, Fathatan, Fatha, Damma, Kasra, Shadda, Sukun:
			runes = analysis.DeleteRune(runes, i)
			i--
		default:
			if unicode.In(runes[i], unicode.Cf) {
				runes = analysis.DeleteRune(runes, i)
				i--
			}
		}
	}
	return analysis.BuildTermFromRunes(runes)
}

func NormalizerFilterConstructor(config map[string]interface{}, cache *registry.Cache) (analysis.TokenFilter, error) {
	return NewSoraniNormalizeFilter(), nil
}

func init() {
	registry.RegisterTokenFilter(NormalizeName, NormalizerFilterConstructor)
}
