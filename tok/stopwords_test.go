/*
 * Copyright 2018 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package tok

import (
	"testing"

	"github.com/blevesearch/bleve/analysis"
	"github.com/stretchr/testify/require"
)

func TestFilterStopwords(t *testing.T) {
	tests := []struct {
		lang string
		in   analysis.TokenStream
		out  analysis.TokenStream
	}{
		{lang: "en",
			in: analysis.TokenStream{
				&analysis.Token{Term: []byte("the")},
				&analysis.Token{Term: []byte("quick")},
				&analysis.Token{Term: []byte("brown")},
				&analysis.Token{Term: []byte("foxes")},
				&analysis.Token{Term: []byte("jump")},
				&analysis.Token{Term: []byte("over")},
				&analysis.Token{Term: []byte("the")},
				&analysis.Token{Term: []byte("big")},
				&analysis.Token{Term: []byte("dogs")},
			},
			out: analysis.TokenStream{
				&analysis.Token{Term: []byte("quick")},
				&analysis.Token{Term: []byte("brown")},
				&analysis.Token{Term: []byte("foxes")},
				&analysis.Token{Term: []byte("jump")},
				&analysis.Token{Term: []byte("big")},
				&analysis.Token{Term: []byte("dogs")},
			},
		},
		{lang: "es",
			in: analysis.TokenStream{
				&analysis.Token{Term: []byte("deseándoles")},
				&analysis.Token{Term: []byte("muchas")},
				&analysis.Token{Term: []byte("alegrías")},
				&analysis.Token{Term: []byte("a")},
				&analysis.Token{Term: []byte("las")},
				&analysis.Token{Term: []byte("señoritas")},
				&analysis.Token{Term: []byte("y")},
				&analysis.Token{Term: []byte("los")},
				&analysis.Token{Term: []byte("señores")},
				&analysis.Token{Term: []byte("programadores")},
				&analysis.Token{Term: []byte("de")},
				&analysis.Token{Term: []byte("Dgraph")},
			},
			out: analysis.TokenStream{
				&analysis.Token{Term: []byte("deseándoles")},
				&analysis.Token{Term: []byte("muchas")},
				&analysis.Token{Term: []byte("alegrías")},
				&analysis.Token{Term: []byte("señoritas")},
				&analysis.Token{Term: []byte("señores")},
				&analysis.Token{Term: []byte("programadores")},
				&analysis.Token{Term: []byte("Dgraph")},
			},
		},
		{lang: "x-klingon",
			in: analysis.TokenStream{
				&analysis.Token{Term: []byte("tlhIngan")},
				&analysis.Token{Term: []byte("maH!")},
			},
			out: analysis.TokenStream{
				&analysis.Token{Term: []byte("tlhIngan")},
				&analysis.Token{Term: []byte("maH!")},
			},
		},
		{lang: "en",
			in: analysis.TokenStream{
				&analysis.Token{
					Term: []byte("same"),
				},
			},
			out: analysis.TokenStream{},
		},
		{lang: "en",
			in:  analysis.TokenStream{},
			out: analysis.TokenStream{},
		},
		{lang: "",
			in: analysis.TokenStream{
				&analysis.Token{
					Term: []byte(""),
				},
			},
			out: analysis.TokenStream{
				&analysis.Token{
					Term: []byte(""),
				},
			},
		},
	}

	for _, tc := range tests {
		out := filterStopwords(tc.lang, tc.in)
		require.Equal(t, tc.out, out)
	}
}
