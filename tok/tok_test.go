/*
 * Copyright 2016 Dgraph Labs, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * 		http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package tok

import (
	"math"
	"sort"
	"testing"

	"github.com/dgraph-io/dgraph/types"
	"github.com/stretchr/testify/require"
)

type encL struct {
	ints   []int32
	tokens []string
}

type byEnc struct{ encL }

func (o byEnc) Less(i, j int) bool { return o.ints[i] < o.ints[j] }

func (o byEnc) Len() int { return len(o.ints) }

func (o byEnc) Swap(i, j int) {
	o.ints[i], o.ints[j] = o.ints[j], o.ints[i]
	o.tokens[i], o.tokens[j] = o.tokens[j], o.tokens[i]
}

func TestIntEncoding(t *testing.T) {
	a := int32(1<<24 + 10)
	b := int32(-1<<24 - 1)
	c := int32(math.MaxInt32)
	d := int32(math.MinInt32)
	enc := encL{}
	arr := []int32{a, b, c, d, 1, 2, 3, 4, -1, -2, -3, 0, 234, 10000, 123, -1543}
	enc.ints = arr
	for _, it := range arr {
		encoded := encodeInt(int32(it))
		enc.tokens = append(enc.tokens, encoded)
	}
	sort.Sort(byEnc{enc})
	for i := 1; i < len(enc.tokens); i++ {
		// The corresponding string tokens should be greater.
		require.True(t, enc.tokens[i-1] < enc.tokens[i], "%d %v vs %d %v",
			enc.ints[i-1], []byte(enc.tokens[i-1]), enc.ints[i], []byte(enc.tokens[i]))
	}
}

func TestFullTextTokenizer(t *testing.T) {
	tokenizer := GetTokenizer("fulltext")
	require.NotNil(t, tokenizer)
	val := types.ValueForType(types.StringID)
	val.Value = "Stemming works!"

	tokens, err := tokenizer.Tokens(val)
	require.Nil(t, err)
	require.Equal(t, 2, len(tokens))
	id := tokenizer.Identifier()
	require.Equal(t, []string{encodeToken("stem", id), encodeToken("work", id)}, tokens)
}

func TestFullTextTokenizerLang(t *testing.T) {
	tokenizer := GetTokenizer(ftsTokenizerName("de"))
	require.NotNil(t, tokenizer)
	val := types.ValueForType(types.StringID)
	val.Value = "Katzen und Auffassung"

	tokens, err := tokenizer.Tokens(val)
	require.NoError(t, err)
	require.Equal(t, 2, len(tokens))
	id := tokenizer.Identifier()
	require.Equal(t, []string{encodeToken("katz", id), encodeToken("auffass", id)}, tokens)
}

func TestTermTokenizer(t *testing.T) {
	tokenizer := GetTokenizer("term")
	require.NotNil(t, tokenizer)
	val := types.ValueForType(types.StringID)
	val.Value = "Tokenizer works!"

	tokens, err := tokenizer.Tokens(val)
	require.Nil(t, err)
	require.Equal(t, 2, len(tokens))
	id := tokenizer.Identifier()
	require.Equal(t, []string{encodeToken("tokenizer", id), encodeToken("works", id)}, tokens)
}
