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

package fa

import (
	"github.com/blevesearch/bleve/analysis"
	"github.com/blevesearch/bleve/registry"

	"github.com/blevesearch/bleve/analysis/char/zerowidthnonjoiner"
	"github.com/blevesearch/bleve/analysis/lang/ar"
	"github.com/blevesearch/bleve/analysis/token/lowercase"
	"github.com/blevesearch/bleve/analysis/tokenizer/unicode"
)

const AnalyzerName = "fa"

func AnalyzerConstructor(config map[string]interface{}, cache *registry.Cache) (*analysis.Analyzer, error) {
	zFilter, err := cache.CharFilterNamed(zerowidthnonjoiner.Name)
	if err != nil {
		return nil, err
	}
	unicodeTokenizer, err := cache.TokenizerNamed(unicode.Name)
	if err != nil {
		return nil, err
	}
	normArFilter, err := cache.TokenFilterNamed(ar.NormalizeName)
	if err != nil {
		return nil, err
	}
	normFaFilter, err := cache.TokenFilterNamed(NormalizeName)
	if err != nil {
		return nil, err
	}
	toLowerFilter, err := cache.TokenFilterNamed(lowercase.Name)
	if err != nil {
		return nil, err
	}
	stopFaFilter, err := cache.TokenFilterNamed(StopName)
	if err != nil {
		return nil, err
	}
	rv := analysis.Analyzer{
		CharFilters: []analysis.CharFilter{
			zFilter,
		},
		Tokenizer: unicodeTokenizer,
		TokenFilters: []analysis.TokenFilter{
			toLowerFilter,
			normArFilter,
			normFaFilter,
			stopFaFilter,
		},
	}
	return &rv, nil
}

func init() {
	registry.RegisterAnalyzer(AnalyzerName, AnalyzerConstructor)
}
