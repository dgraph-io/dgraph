/*
 * Copyright (C) 2017 Dgraph Labs, Inc. and Contributors
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
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
	ints   []int64
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
	a := int64(1<<24 + 10)
	b := int64(-1<<24 - 1)
	c := int64(math.MaxInt64)
	d := int64(math.MinInt64)
	enc := encL{}
	arr := []int64{a, b, c, d, 1, 2, 3, 4, -1, -2, -3, 0, 234, 10000, 123, -1543}
	enc.ints = arr
	for _, it := range arr {
		encoded := encodeInt(int64(it))
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
	tokenizer, has := GetTokenizer("fulltext")
	require.True(t, has)
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
	tokenizer, has := GetTokenizer(FtsTokenizerName("de"))
	require.True(t, has)
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
	tokenizer, has := GetTokenizer("term")
	require.True(t, has)
	require.NotNil(t, tokenizer)
	val := types.ValueForType(types.StringID)
	val.Value = "Tokenizer works!"

	tokens, err := tokenizer.Tokens(val)
	require.Nil(t, err)
	require.Equal(t, 2, len(tokens))
	id := tokenizer.Identifier()
	require.Equal(t, []string{encodeToken("tokenizer", id), encodeToken("works", id)}, tokens)
}
