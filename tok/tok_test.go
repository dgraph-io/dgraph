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

/*
For testing embedded version, try:
wget https://github.com/dgraph-io/goicu/raw/master/icudt57l.dat -P /tmp
go test -icu /tmp/icudt57l.dat -tags embed
*/

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/x"
)

func TestTokenizeBasic(t *testing.T) {
	testData := []struct {
		in       string
		expected []string
	}{
		{"hello world", []string{"hello", "world"}},
		{"  HE,LLO,  \n  world  ", []string{"he", "llo", "world"}},
		{"在新加坡鞭刑是處置犯人  的方法之一!",
			[]string{"在", "新加坡", "鞭刑", "是", "處置", "犯人", "的", "方法", "之一"}},
		{"cafés   cool", []string{"cafes", "cool"}},
		{"nörmalization", []string{"normalization"}},
		{" 住宅地域における本機の使用は有害な電波妨害を引き起こすことがあり、その場合ユーザーは自己負担で電波妨害の問題を解決しなければなりません。",
			[]string{"住宅", "地域", "における", "本機", "の", "使用", "は", "有害", "な",
				"電波", "妨害", "を", "引き起こす", "こと", "か", "あり", "その", "場合", "ユー",
				"サー", "は", "自己", "負担", "て", "電波", "妨害", "の", "問題", "を", "解決",
				"しな", "け", "れ", "は", "なり", "ま", "せん"}},
		// Exceed the max token width and test that we did not exceed.
		{strings.Repeat("a", (1+maxTokenSize)*5),
			[]string{strings.Repeat("a", maxTokenSize)}},
	}

	for _, d := range testData {
		func(in string, expected []string) {
			tokenizer, err := NewTokenizer([]byte(d.in))
			defer tokenizer.Destroy()
			require.NoError(t, err)
			require.NotNil(t, tokenizer)
			tokensBytes := tokenizer.Tokens()
			var tokens []string
			for _, token := range tokensBytes {
				tokens = append(tokens, string(token))
			}
			require.EqualValues(t, expected, tokens)
		}(d.in, d.expected)
	}
}

func TestNoICU(t *testing.T) {
	disableICU = true
	tokenizer, err := NewTokenizer([]byte("hello world"))
	defer tokenizer.Destroy()
	require.NotNil(t, tokenizer)
	require.NoError(t, err)
	tokens := tokenizer.Tokens()
	require.Empty(t, tokens)
}

func TestMain(m *testing.M) {
	x.Init()
	os.Exit(m.Run())
}
