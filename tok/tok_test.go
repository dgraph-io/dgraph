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
	"time"

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

func TestHourTokenizer(t *testing.T) {
	var err error
	tokenizer, has := GetTokenizer("hour")
	require.True(t, has)
	require.NotNil(t, tokenizer)
	val := types.ValueForType(types.DateTimeID)
	val.Value, err = time.Parse(time.RFC3339, "2017-01-01T12:12:12Z")
	require.NoError(t, err)

	tokens, err := tokenizer.Tokens(val)
	require.NoError(t, err)
	require.Equal(t, 1, len(tokens))
	require.Equal(t, 1+2*4, len(tokens[0]))
}

func TestDayTokenizer(t *testing.T) {
	var err error
	tokenizer, has := GetTokenizer("day")
	require.True(t, has)
	require.NotNil(t, tokenizer)
	val := types.ValueForType(types.DateTimeID)
	val.Value, err = time.Parse(time.RFC3339, "2017-01-01T12:12:12Z")
	require.NoError(t, err)

	tokens, err := tokenizer.Tokens(val)
	require.NoError(t, err)
	require.Equal(t, 1, len(tokens))
	require.Equal(t, 1+2*3, len(tokens[0]))
}

func TestMonthTokenizer(t *testing.T) {
	var err error
	tokenizer, has := GetTokenizer("month")
	require.True(t, has)
	require.NotNil(t, tokenizer)
	val := types.ValueForType(types.DateTimeID)
	val.Value, err = time.Parse(time.RFC3339, "2017-01-01T12:12:12Z")
	require.NoError(t, err)

	tokens, err := tokenizer.Tokens(val)
	require.NoError(t, err)
	require.Equal(t, 1, len(tokens))
	require.Equal(t, 1+2*2, len(tokens[0]))
}

func TestDateTimeTokenizer(t *testing.T) {
	var err error
	tokenizer, has := GetTokenizer("year")
	require.True(t, has)
	require.NotNil(t, tokenizer)
	val := types.ValueForType(types.DateTimeID)
	val.Value, err = time.Parse(time.RFC3339, "2017-01-01T12:12:12Z")
	require.NoError(t, err)

	tokens, err := tokenizer.Tokens(val)
	require.NoError(t, err)
	require.Equal(t, 1, len(tokens))
	require.Equal(t, 1+2, len(tokens[0]))
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
	// tokens should be sorted and unique
	require.Equal(t, []string{encodeToken("auffass", id), encodeToken("katz", id)}, tokens)
}

func TestTermTokenizer(t *testing.T) {
	tokenizer, has := GetTokenizer("term")
	require.True(t, has)
	require.NotNil(t, tokenizer)
	val := types.ValueForType(types.StringID)
	val.Value = "Tokenizer works!"

	tokens, err := tokenizer.Tokens(val)
	require.NoError(t, err)
	require.Equal(t, 2, len(tokens))
	id := tokenizer.Identifier()
	require.Equal(t, []string{encodeToken("tokenizer", id), encodeToken("works", id)}, tokens)
}

func TestTrigramTokenizer(t *testing.T) {
	tokenizer, has := GetTokenizer("trigram")
	require.True(t, has)
	require.NotNil(t, tokenizer)
	val := types.ValueForType(types.StringID)
	val.Value = "Dgraph rocks!"
	tokens, err := tokenizer.Tokens(val)
	require.NoError(t, err)
	require.Equal(t, 11, len(tokens))
	id := tokenizer.Identifier()
	expected := []string{
		encodeToken("Dgr", id),
		encodeToken("gra", id),
		encodeToken("rap", id),
		encodeToken("aph", id),
		encodeToken("ph ", id),
		encodeToken("h r", id),
		encodeToken(" ro", id),
		encodeToken("roc", id),
		encodeToken("ock", id),
		encodeToken("cks", id),
		encodeToken("ks!", id),
	}
	sort.Strings(expected)
	require.Equal(t, expected, tokens)
}

func TestGetBleveTokens(t *testing.T) {
	val := types.ValueForType(types.StringID)
	val.Value = "Our chief weapon is surprise...surprise and fear...fear and surprise...." +
		"Our two weapons are fear and surprise...and ruthless efficiency.... " +
		"Our three weapons are fear, surprise, and ruthless efficiency..."
	tokens, err := getBleveTokens(FTSTokenizerName, 0x20, val) // use space as identifier
	require.NoError(t, err)

	expected := []string{" chief", " weapon", " surpris", " fear", " ruthless", " effici"}
	sort.Strings(expected)

	// ensure that tokens are sorted and unique
	require.Equal(t, expected, tokens)
}
