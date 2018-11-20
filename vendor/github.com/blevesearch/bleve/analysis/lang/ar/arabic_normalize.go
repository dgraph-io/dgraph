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

const NormalizeName = "normalize_ar"

const (
	Alef           = '\u0627'
	AlefMadda      = '\u0622'
	AlefHamzaAbove = '\u0623'
	AlefHamzaBelow = '\u0625'
	Yeh            = '\u064A'
	DotlessYeh     = '\u0649'
	TehMarbuta     = '\u0629'
	Heh            = '\u0647'
	Tatweel        = '\u0640'
	Fathatan       = '\u064B'
	Dammatan       = '\u064C'
	Kasratan       = '\u064D'
	Fatha          = '\u064E'
	Damma          = '\u064F'
	Kasra          = '\u0650'
	Shadda         = '\u0651'
	Sukun          = '\u0652'
)

type ArabicNormalizeFilter struct {
}

func NewArabicNormalizeFilter() *ArabicNormalizeFilter {
	return &ArabicNormalizeFilter{}
}

func (s *ArabicNormalizeFilter) Filter(input analysis.TokenStream) analysis.TokenStream {
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
		case AlefMadda, AlefHamzaAbove, AlefHamzaBelow:
			runes[i] = Alef
		case DotlessYeh:
			runes[i] = Yeh
		case TehMarbuta:
			runes[i] = Heh
		case Tatweel, Kasratan, Dammatan, Fathatan, Fatha, Damma, Kasra, Shadda, Sukun:
			runes = analysis.DeleteRune(runes, i)
			i--
		}
	}
	return analysis.BuildTermFromRunes(runes)
}

func NormalizerFilterConstructor(config map[string]interface{}, cache *registry.Cache) (analysis.TokenFilter, error) {
	return NewArabicNormalizeFilter(), nil
}

func init() {
	registry.RegisterTokenFilter(NormalizeName, NormalizerFilterConstructor)
}
