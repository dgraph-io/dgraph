package bleve

import (
	"errors"

	"github.com/blevesearch/bleve/analysis"
	"github.com/blevesearch/bleve/registry"
	"github.com/yanyiwu/gojieba"
)

type JiebaTokenizer struct {
	handle *gojieba.Jieba
}

func NewJiebaTokenizer(dictpath, hmmpath, userdictpath, idf, stop_words string) *JiebaTokenizer {
	x := gojieba.NewJieba(dictpath, hmmpath, userdictpath, idf, stop_words)
	return &JiebaTokenizer{x}
}

func (x *JiebaTokenizer) Free() {
	x.handle.Free()
}

func (x *JiebaTokenizer) Tokenize(sentence []byte) analysis.TokenStream {
	result := make(analysis.TokenStream, 0)
	pos := 1
	words := x.handle.Tokenize(string(sentence), gojieba.SearchMode, false)
	for _, word := range words {
		token := analysis.Token{
			Term:     []byte(word.Str),
			Start:    word.Start,
			End:      word.End,
			Position: pos,
			Type:     analysis.Ideographic,
		}
		result = append(result, &token)
		pos++
	}
	return result
}

func tokenizerConstructor(config map[string]interface{}, cache *registry.Cache) (analysis.Tokenizer, error) {
	dictpath, ok := config["dictpath"].(string)
	if !ok {
		return nil, errors.New("config dictpath not found")
	}
	hmmpath, ok := config["hmmpath"].(string)
	if !ok {
		return nil, errors.New("config hmmpath not found")
	}
	userdictpath, ok := config["userdictpath"].(string)
	if !ok {
		return nil, errors.New("config userdictpath not found")
	}
	idf, ok := config["idf"].(string)
	if !ok {
		return nil, errors.New("config idf not found")
	}
	stop_words, ok := config["stop_words"].(string)
	if !ok {
		return nil, errors.New("config stop_words not found")
	}
	return NewJiebaTokenizer(dictpath, hmmpath, userdictpath, idf, stop_words), nil
}

func init() {
	registry.RegisterTokenizer("gojieba", tokenizerConstructor)
}
