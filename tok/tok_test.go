package tok

import (
	"testing"
)

func TestBasic(t *testing.T) {
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
