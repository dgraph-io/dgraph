//  Copyright (c) 2016 Couchbase, Inc.
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

package cjk

import (
	"bytes"
	"unicode/utf8"

	"github.com/blevesearch/bleve/analysis"
	"github.com/blevesearch/bleve/registry"
)

const WidthName = "cjk_width"

type CJKWidthFilter struct{}

func NewCJKWidthFilter() *CJKWidthFilter {
	return &CJKWidthFilter{}
}

func (s *CJKWidthFilter) Filter(input analysis.TokenStream) analysis.TokenStream {
	for _, token := range input {
		runeCount := utf8.RuneCount(token.Term)
		runes := bytes.Runes(token.Term)
		for i := 0; i < runeCount; i++ {
			ch := runes[i]
			if ch >= 0xFF01 && ch <= 0xFF5E {
				// fullwidth ASCII variants
				runes[i] -= 0xFEE0
			} else if ch >= 0xFF65 && ch <= 0xFF9F {
				// halfwidth Katakana variants
				if (ch == 0xFF9E || ch == 0xFF9F) && i > 0 && combine(runes, i, ch) {
					runes = analysis.DeleteRune(runes, i)
					i--
					runeCount = len(runes)
				} else {
					runes[i] = kanaNorm[ch-0xFF65]
				}
			}
		}
		token.Term = analysis.BuildTermFromRunes(runes)
	}

	return input
}

var kanaNorm = []rune{
	0x30fb, 0x30f2, 0x30a1, 0x30a3, 0x30a5, 0x30a7, 0x30a9, 0x30e3, 0x30e5,
	0x30e7, 0x30c3, 0x30fc, 0x30a2, 0x30a4, 0x30a6, 0x30a8, 0x30aa, 0x30ab,
	0x30ad, 0x30af, 0x30b1, 0x30b3, 0x30b5, 0x30b7, 0x30b9, 0x30bb, 0x30bd,
	0x30bf, 0x30c1, 0x30c4, 0x30c6, 0x30c8, 0x30ca, 0x30cb, 0x30cc, 0x30cd,
	0x30ce, 0x30cf, 0x30d2, 0x30d5, 0x30d8, 0x30db, 0x30de, 0x30df, 0x30e0,
	0x30e1, 0x30e2, 0x30e4, 0x30e6, 0x30e8, 0x30e9, 0x30ea, 0x30eb, 0x30ec,
	0x30ed, 0x30ef, 0x30f3, 0x3099, 0x309A,
}

var kanaCombineVoiced = []rune{
	78, 0, 0, 0, 0, 1, 0, 1, 0, 1, 0, 1, 0, 1, 0, 1, 0, 1, 0, 1, 0, 1, 0, 1,
	0, 1, 0, 1, 0, 0, 1, 0, 1, 0, 1, 0, 0, 0, 0, 0, 0, 1, 0, 0, 1, 0, 0, 1,
	0, 0, 1, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 8, 8, 8, 8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1,
}
var kanaCombineHalfVoiced = []rune{
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2, 0, 0, 2, 0, 0, 2,
	0, 0, 2, 0, 0, 2, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
}

func combine(text []rune, pos int, r rune) bool {
	prev := text[pos-1]
	if prev >= 0x30A6 && prev <= 0x30FD {
		if r == 0xFF9F {
			text[pos-1] += kanaCombineHalfVoiced[prev-0x30A6]
		} else {
			text[pos-1] += kanaCombineVoiced[prev-0x30A6]
		}
		return text[pos-1] != prev
	}
	return false
}

func CJKWidthFilterConstructor(config map[string]interface{}, cache *registry.Cache) (analysis.TokenFilter, error) {
	return NewCJKWidthFilter(), nil
}

func init() {
	registry.RegisterTokenFilter(WidthName, CJKWidthFilterConstructor)
}
