//  Copyright (c) 2014 Couchbase, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// 		http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package elision

import (
	"fmt"
	"unicode/utf8"

	"github.com/blevesearch/bleve/analysis"
	"github.com/blevesearch/bleve/registry"
)

const Name = "elision"

const RightSingleQuotationMark = '’'
const Apostrophe = '\''

type ElisionFilter struct {
	articles analysis.TokenMap
}

func NewElisionFilter(articles analysis.TokenMap) *ElisionFilter {
	return &ElisionFilter{
		articles: articles,
	}
}

func (s *ElisionFilter) Filter(input analysis.TokenStream) analysis.TokenStream {
	for _, token := range input {
		term := token.Term
		for i := 0; i < len(term); {
			r, size := utf8.DecodeRune(term[i:])
			if r == Apostrophe || r == RightSingleQuotationMark {
				// see if the prefix matches one of the articles
				prefix := term[0:i]
				_, articleMatch := s.articles[string(prefix)]
				if articleMatch {
					token.Term = term[i+size:]
					break
				}
			}
			i += size
		}
	}
	return input
}

func ElisionFilterConstructor(config map[string]interface{}, cache *registry.Cache) (analysis.TokenFilter, error) {
	articlesTokenMapName, ok := config["articles_token_map"].(string)
	if !ok {
		return nil, fmt.Errorf("must specify articles_token_map")
	}
	articlesTokenMap, err := cache.TokenMapNamed(articlesTokenMapName)
	if err != nil {
		return nil, fmt.Errorf("error building elision filter: %v", err)
	}
	return NewElisionFilter(articlesTokenMap), nil
}

func init() {
	registry.RegisterTokenFilter(Name, ElisionFilterConstructor)
}
