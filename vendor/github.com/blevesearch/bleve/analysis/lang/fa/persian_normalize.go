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

package fa

import (
	"bytes"

	"github.com/blevesearch/bleve/analysis"
	"github.com/blevesearch/bleve/registry"
)

const NormalizeName = "normalize_fa"

const (
	Yeh        = '\u064A'
	FarsiYeh   = '\u06CC'
	YehBarree  = '\u06D2'
	Keheh      = '\u06A9'
	Kaf        = '\u0643'
	HamzaAbove = '\u0654'
	HehYeh     = '\u06C0'
	HehGoal    = '\u06C1'
	Heh        = '\u0647'
)

type PersianNormalizeFilter struct {
}

func NewPersianNormalizeFilter() *PersianNormalizeFilter {
	return &PersianNormalizeFilter{}
}

func (s *PersianNormalizeFilter) Filter(input analysis.TokenStream) analysis.TokenStream {
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
		case FarsiYeh, YehBarree:
			runes[i] = Yeh
		case Keheh:
			runes[i] = Kaf
		case HehYeh, HehGoal:
			runes[i] = Heh
		case HamzaAbove: // necessary for HEH + HAMZA
			runes = analysis.DeleteRune(runes, i)
			i--
		}
	}
	return analysis.BuildTermFromRunes(runes)
}

func NormalizerFilterConstructor(config map[string]interface{}, cache *registry.Cache) (analysis.TokenFilter, error) {
	return NewPersianNormalizeFilter(), nil
}

func init() {
	registry.RegisterTokenFilter(NormalizeName, NormalizerFilterConstructor)
}
