// +build tok

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

import "testing"

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
	}

	for _, d := range testData {
		func(in string, expected []string) {
			tokenizer, err := NewTokenizer([]byte(d.in))
			defer tokenizer.Destroy()
			if err != nil {
				t.Error(err)
				return
			}
			var tokens []string
			for {
				sPtr := tokenizer.Next()
				if sPtr == nil {
					break
				}
				tokens = append(tokens, *sPtr)
			}

			if len(tokens) != len(expected) {
				t.Errorf("Wrong number of tokens: %d vs %d", len(tokens), len(expected))
				return
			}
			for i := 0; i < len(tokens); i++ {
				if tokens[i] != expected[i] {
					t.Errorf("Expected token [%s] but got [%s]", expected[i], tokens[i])
					return
				}
			}
		}(d.in, d.expected)
	}
}
