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

package pt

import (
	"github.com/blevesearch/bleve/analysis"
	"github.com/blevesearch/bleve/registry"

	"github.com/blevesearch/bleve/analysis/token/lowercase"
	"github.com/blevesearch/bleve/analysis/tokenizer/unicode"
)

const AnalyzerName = "pt"

func AnalyzerConstructor(config map[string]interface{}, cache *registry.Cache) (*analysis.Analyzer, error) {
	tokenizer, err := cache.TokenizerNamed(unicode.Name)
	if err != nil {
		return nil, err
	}
	toLowerFilter, err := cache.TokenFilterNamed(lowercase.Name)
	if err != nil {
		return nil, err
	}
	stopPtFilter, err := cache.TokenFilterNamed(StopName)
	if err != nil {
		return nil, err
	}
	stemmerPtFilter, err := cache.TokenFilterNamed(LightStemmerName)
	if err != nil {
		return nil, err
	}
	rv := analysis.Analyzer{
		Tokenizer: tokenizer,
		TokenFilters: []analysis.TokenFilter{
			toLowerFilter,
			stopPtFilter,
			stemmerPtFilter,
		},
	}
	return &rv, nil
}

func init() {
	registry.RegisterAnalyzer(AnalyzerName, AnalyzerConstructor)
}
