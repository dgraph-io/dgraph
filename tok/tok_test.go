/*
 * Copyright 2016-2018 Dgraph Labs, Inc. and Contributors
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
	"math"
	"sort"
	"testing"
	"time"

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

	tokens, err := BuildTokens("Stemming works!", GetLangTokenizer(tokenizer, "en"))
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
	dt, err := time.Parse(time.RFC3339, "2017-01-01T12:12:12Z")
	require.NoError(t, err)

	tokens, err := BuildTokens(dt, tokenizer)
	require.NoError(t, err)
	require.Equal(t, 1, len(tokens))
	require.Equal(t, 1+2*4, len(tokens[0]))
}

func TestDayTokenizer(t *testing.T) {
	var err error
	tokenizer, has := GetTokenizer("day")
	require.True(t, has)
	require.NotNil(t, tokenizer)
	dt, err := time.Parse(time.RFC3339, "2017-01-01T12:12:12Z")
	require.NoError(t, err)

	tokens, err := BuildTokens(dt, tokenizer)
	require.NoError(t, err)
	require.Equal(t, 1, len(tokens))
	require.Equal(t, 1+2*3, len(tokens[0]))
}

func TestMonthTokenizer(t *testing.T) {
	var err error
	tokenizer, has := GetTokenizer("month")
	require.True(t, has)
	require.NotNil(t, tokenizer)
	dt, err := time.Parse(time.RFC3339, "2017-01-01T12:12:12Z")
	require.NoError(t, err)

	tokens, err := BuildTokens(dt, tokenizer)
	require.NoError(t, err)
	require.Equal(t, 1, len(tokens))
	require.Equal(t, 1+2*2, len(tokens[0]))
}

func TestDateTimeTokenizer(t *testing.T) {
	var err error
	tokenizer, has := GetTokenizer("year")
	require.True(t, has)
	require.NotNil(t, tokenizer)
	dt, err := time.Parse(time.RFC3339, "2017-01-01T12:12:12Z")
	require.NoError(t, err)

	tokens, err := BuildTokens(dt, tokenizer)
	require.NoError(t, err)
	require.Equal(t, 1, len(tokens))
	require.Equal(t, 1+2, len(tokens[0]))
}

func TestFullTextTokenizerLang(t *testing.T) {
	tokenizer, has := GetTokenizer("fulltext")
	require.True(t, has)
	require.NotNil(t, tokenizer)

	tokens, err := BuildTokens("Katzen und Auffassung und Auffassung", GetLangTokenizer(tokenizer, "de"))
	require.NoError(t, err)
	require.Equal(t, 2, len(tokens))
	id := tokenizer.Identifier()
	// tokens should be sorted and unique
	require.Equal(t, []string{encodeToken("auffassung", id), encodeToken("katz", id)}, tokens)
}

func TestTermTokenizer(t *testing.T) {
	tokenizer, has := GetTokenizer("term")
	require.True(t, has)
	require.NotNil(t, tokenizer)

	tokens, err := BuildTokens("Tokenizer works works!", tokenizer)
	require.NoError(t, err)
	require.Equal(t, 2, len(tokens))
	id := tokenizer.Identifier()
	require.Equal(t, []string{encodeToken("tokenizer", id), encodeToken("works", id)}, tokens)
}

func TestTrigramTokenizer(t *testing.T) {
	tokenizer, has := GetTokenizer("trigram")
	require.True(t, has)
	require.NotNil(t, tokenizer)
	tokens, err := BuildTokens("Dgraph rocks!", tokenizer)
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

func TestGetFullTextTokens(t *testing.T) {
	val := "Our chief weapon is surprise...surprise and fear...fear and surprise...." +
		"Our two weapons are fear and surprise...and ruthless efficiency.... " +
		"Our three weapons are fear, surprise, and ruthless efficiency..."
	tokens, err := (&FullTextTokenizer{lang: "en"}).Tokens(val)
	require.NoError(t, err)

	expected := []string{"chief", "weapon", "surpris", "fear", "ruthless", "effici", "two", "three"}
	sort.Strings(expected)

	// ensure that tokens are sorted and unique
	require.Equal(t, expected, tokens)
}

func TestGetFullTextTokens1(t *testing.T) {
	tokens, err := GetFullTextTokens([]string{"Quick brown fox"}, "en")
	require.NoError(t, err)
	require.NotNil(t, tokens)
	require.Equal(t, 3, len(tokens))
}

func TestGetFullTextTokensInvalidLang(t *testing.T) {
	tokens, err := GetFullTextTokens([]string{"Quick brown fox"}, "xxx_such_language")
	require.NoError(t, err)
	require.NotNil(t, tokens)
	require.Equal(t, 3, len(tokens))
}

// NOTE: The Chinese/Japanese/Korean tests were are based on assuming that the
// output is correct (and adding it to the test), with some verification using
// Google translate.

func TestFullTextTokenizerCJKChinese(t *testing.T) {
	tokenizer, has := GetTokenizer("fulltext")
	require.True(t, has)
	require.NotNil(t, tokenizer)

	got, err := BuildTokens("他是一个薪水很高的商人", GetLangTokenizer(tokenizer, "zh"))
	require.NoError(t, err)

	id := tokenizer.Identifier()
	wantToks := []string{
		encodeToken("一个", id),
		encodeToken("个薪", id),
		encodeToken("他是", id),
		encodeToken("商人", id),
		encodeToken("很高", id),
		encodeToken("是一", id),
		encodeToken("水很", id),
		encodeToken("的商", id),
		encodeToken("薪水", id),
		encodeToken("高的", id),
	}
	require.Equal(t, wantToks, got)
	checkSortedAndUnique(t, got)
}

func TestFullTextTokenizerCJKKorean(t *testing.T) {
	tokenizer, has := GetTokenizer("fulltext")
	require.True(t, has)
	require.NotNil(t, tokenizer)

	got, err := BuildTokens("그는 큰 급여를 가진 사업가입니다.", GetLangTokenizer(tokenizer, "ko"))
	require.NoError(t, err)

	id := tokenizer.Identifier()
	wantToks := []string{
		encodeToken("가진", id),
		encodeToken("그는", id),
		encodeToken("급여를", id),
		encodeToken("사업가입니다", id),
		encodeToken("큰", id),
	}
	require.Equal(t, wantToks, got)
	checkSortedAndUnique(t, got)
}

func TestFullTextTokenizerCJKJapanese(t *testing.T) {
	tokenizer, has := GetTokenizer("fulltext")
	require.True(t, has)
	require.NotNil(t, tokenizer)

	got, err := BuildTokens("彼は大きな給与を持つ実業家です", GetLangTokenizer(tokenizer, "ja"))
	require.NoError(t, err)

	id := tokenizer.Identifier()
	wantToks := []string{
		encodeToken("きな", id),
		encodeToken("つ実", id),
		encodeToken("です", id),
		encodeToken("な給", id),
		encodeToken("は大", id),
		encodeToken("を持", id),
		encodeToken("与を", id),
		encodeToken("大き", id),
		encodeToken("実業", id),
		encodeToken("家で", id),
		encodeToken("彼は", id),
		encodeToken("持つ", id),
		encodeToken("業家", id),
		encodeToken("給与", id),
	}
	require.Equal(t, wantToks, got)
	checkSortedAndUnique(t, got)
}

func checkSortedAndUnique(t *testing.T, tokens []string) {
	if !sort.StringsAreSorted(tokens) {
		t.Error("tokens were not sorted")
	}
	set := make(map[string]struct{})
	for _, tok := range tokens {
		if _, ok := set[tok]; ok {
			if ok {
				t.Error("tokens are not unique")
			}
		}
		set[tok] = struct{}{}
	}
}
