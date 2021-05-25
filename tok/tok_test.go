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

	tokens, err := BuildTokens("Stemming works!", GetTokenizerForLang(tokenizer, "en"))
	require.Nil(t, err)
	require.Equal(t, 2, len(tokens))
	id := tokenizer.Identifier()
	require.Equal(t, []string{EncodeToken("stem", id), EncodeToken("work", id)}, tokens)
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

	tokens, err := BuildTokens("Katzen und Auffassung und Auffassung", GetTokenizerForLang(tokenizer, "de"))
	require.NoError(t, err)
	require.Equal(t, 2, len(tokens))
	id := tokenizer.Identifier()
	// tokens should be sorted and unique
	require.Equal(t, []string{EncodeToken("auffassung", id), EncodeToken("katz", id)}, tokens)
}

func TestTermTokenizer(t *testing.T) {
	tokenizer, has := GetTokenizer("term")
	require.True(t, has)
	require.NotNil(t, tokenizer)

	tokens, err := BuildTokens("Tokenizer works works!", tokenizer)
	require.NoError(t, err)
	require.Equal(t, 2, len(tokens))
	id := tokenizer.Identifier()
	require.Equal(t, []string{EncodeToken("tokenizer", id), EncodeToken("works", id)}, tokens)

	// TEMPORARILY COMMENTED OUT AS THIS IS THE IDEAL BEHAVIOUR. WE ARE NOT THERE YET.
	/*
		tokens, err = BuildTokens("Barack Obama made Obamacare", tokenizer)
		require.NoError(t, err)
		require.Equal(t, 3, len(tokens))
		require.Equal(t, []string{
			encodeToken("barack obama", id),
			encodeToken("made", id),
			encodeToken("obamacare", id),
		})
	*/
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
		EncodeToken("Dgr", id),
		EncodeToken("gra", id),
		EncodeToken("rap", id),
		EncodeToken("aph", id),
		EncodeToken("ph ", id),
		EncodeToken("h r", id),
		EncodeToken(" ro", id),
		EncodeToken("roc", id),
		EncodeToken("ock", id),
		EncodeToken("cks", id),
		EncodeToken("ks!", id),
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

	got, err := BuildTokens("他是一个薪水很高的商人", GetTokenizerForLang(tokenizer, "zh"))
	require.NoError(t, err)

	id := tokenizer.Identifier()
	wantToks := []string{
		EncodeToken("一个", id),
		EncodeToken("个薪", id),
		EncodeToken("他是", id),
		EncodeToken("商人", id),
		EncodeToken("很高", id),
		EncodeToken("是一", id),
		EncodeToken("水很", id),
		EncodeToken("的商", id),
		EncodeToken("薪水", id),
		EncodeToken("高的", id),
	}
	require.Equal(t, wantToks, got)
	checkSortedAndUnique(t, got)
}

func TestFullTextTokenizerCJKKorean(t *testing.T) {
	tokenizer, has := GetTokenizer("fulltext")
	require.True(t, has)
	require.NotNil(t, tokenizer)

	got, err := BuildTokens("그는 큰 급여를 가진 사업가입니다.", GetTokenizerForLang(tokenizer, "ko"))
	require.NoError(t, err)

	id := tokenizer.Identifier()
	wantToks := []string{
		EncodeToken("가진", id),
		EncodeToken("그는", id),
		EncodeToken("급여를", id),
		EncodeToken("사업가입니다", id),
		EncodeToken("큰", id),
	}
	require.Equal(t, wantToks, got)
	checkSortedAndUnique(t, got)
}

func TestFullTextTokenizerCJKJapanese(t *testing.T) {
	tokenizer, has := GetTokenizer("fulltext")
	require.True(t, has)
	require.NotNil(t, tokenizer)

	got, err := BuildTokens("彼は大きな給与を持つ実業家です", GetTokenizerForLang(tokenizer, "ja"))
	require.NoError(t, err)

	id := tokenizer.Identifier()
	wantToks := []string{
		EncodeToken("きな", id),
		EncodeToken("つ実", id),
		EncodeToken("です", id),
		EncodeToken("な給", id),
		EncodeToken("は大", id),
		EncodeToken("を持", id),
		EncodeToken("与を", id),
		EncodeToken("大き", id),
		EncodeToken("実業", id),
		EncodeToken("家で", id),
		EncodeToken("彼は", id),
		EncodeToken("持つ", id),
		EncodeToken("業家", id),
		EncodeToken("給与", id),
	}
	require.Equal(t, wantToks, got)
	checkSortedAndUnique(t, got)
}

func TestTermTokenizeCJKChinese(t *testing.T) {
	tokenizer, ok := GetTokenizer("term")
	require.True(t, ok)
	require.NotNil(t, tokenizer)

	got, err := BuildTokens("第一轮 第二轮 第一轮", GetTokenizerForLang(tokenizer, "zh"))
	require.NoError(t, err)

	id := tokenizer.Identifier()
	wantToks := []string{
		EncodeToken("第一轮", id),
		EncodeToken("第二轮", id),
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

func BenchmarkTermTokenizer(b *testing.B) {
	b.Skip() // tmp
}
