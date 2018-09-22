//  Copyright (c) 2014 Couchbase, Inc.
//  Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file
//  except in compliance with the License. You may obtain a copy of the License at
//    http://www.apache.org/licenses/LICENSE-2.0
//  Unless required by applicable law or agreed to in writing, software distributed under the
//  License is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND,
//  either express or implied. See the License for the specific language governing permissions
//  and limitations under the License.

package stemmer

import (
	"fmt"

	"github.com/tebeka/snowball"
	"github.com/blevesearch/bleve/analysis"
	"github.com/blevesearch/bleve/registry"
)

const Name = "stem"

type StemmerFilter struct {
	lang        string
	stemmerPool chan *snowball.Stemmer
}

func NewStemmerFilter(lang string) (*StemmerFilter, error) {
	stemmerPool := make(chan *snowball.Stemmer, 4)
	for i := 0; i < 4; i++ {
		stemmer, err := snowball.New(lang)
		if err != nil {
			return nil, err
		}
		stemmerPool <- stemmer
	}
	return &StemmerFilter{
		lang:        lang,
		stemmerPool: stemmerPool,
	}, nil
}

func MustNewStemmerFilter(lang string) *StemmerFilter {
	sf, err := NewStemmerFilter(lang)
	if err != nil {
		panic(err)
	}
	return sf
}

func (s *StemmerFilter) List() []string {
	return snowball.LangList()
}

func (s *StemmerFilter) Filter(input analysis.TokenStream) analysis.TokenStream {
	for _, token := range input {
		// if it is not a protected keyword, stem it
		if !token.KeyWord {
			stemmer := <-s.stemmerPool
			stemmed := stemmer.Stem(string(token.Term))
			s.stemmerPool <- stemmer
			token.Term = []byte(stemmed)
		}
	}
	return input
}

func StemmerFilterConstructor(config map[string]interface{}, cache *registry.Cache) (analysis.TokenFilter, error) {
	langVal, ok := config["lang"].(string)
	if !ok {
		return nil, fmt.Errorf("must specify stemmer language")
	}
	lang := langVal
	return NewStemmerFilter(lang)
}

func init() {
	registry.RegisterTokenFilter(Name, StemmerFilterConstructor)
}
