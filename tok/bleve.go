/*
 * Copyright 2018 Dgraph Labs, Inc. and Contributors
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
	"github.com/dgraph-io/dgraph/x"

	"github.com/blevesearch/bleve/analysis/analyzer/custom"
	"github.com/blevesearch/bleve/analysis/token/lowercase"
	"github.com/blevesearch/bleve/analysis/token/unicodenorm"
	"github.com/blevesearch/bleve/analysis/tokenizer/unicode"
	"github.com/blevesearch/bleve/registry"
)

const unicodenormName = "unicodenorm_nfkc"

var bleveCache = registry.NewCache()

func registerBleveTokenizers() {
	// unicode normalizer filter - simplifies unicode words using Normalization Form KC (NFKC)
	// See: http://unicode.org/reports/tr15/#Norm_Forms
	_, err := bleveCache.DefineTokenFilter(unicodenormName,
		map[string]interface{}{
			"type": unicodenorm.Name,
			"form": unicodenorm.NFKC,
		})
	x.Check(err)

	// term analyzer - splits on word boundaries, lowercase and normalize tokens.
	_, err = bleveCache.DefineAnalyzer("term",
		map[string]interface{}{
			"type":      custom.Name,
			"tokenizer": unicode.Name,
			"token_filters": []string{
				lowercase.Name,
				unicodenormName,
			},
		})
	x.Check(err)

	// fulltext analyzer - does language stop-words removal and stemming.
	_, err = bleveCache.DefineAnalyzer("fulltext",
		map[string]interface{}{
			"type":      custom.Name,
			"tokenizer": unicode.Name,
			"token_filters": []string{
				lowercase.Name,
				unicodenormName,
			},
		})
	x.Check(err)

	registerTokenizer(TermTokenizer{})
	registerTokenizer(FullTextTokenizer{})
}

func getTermTokens(str string) ([]string, error) {
	if str == "" {
		// XXX: why do we need this???
		return []string{""}, nil
	}
	analyzer, err := bleveCache.AnalyzerNamed("term")
	if err != nil {
		return nil, err
	}
	tokens := analyzer.Analyze([]byte(str))
	var terms []string
	for i := range tokens {
		terms = append(terms, string(tokens[i].Term))
	}
	terms = x.RemoveDuplicates(terms)
	return terms, nil
}

func (t *FullTextTokenizer) getFullTextTokens(s string) ([]string, error) {
	if s == "" {
		return []string{}, nil
	}
	analyzer, err := bleveCache.AnalyzerNamed("fulltext")
	if err != nil {
		return nil, err
	}
	lang := langBase(t.lang)
	// pass 1 - lowercase and normalize input
	tokens := analyzer.Analyze([]byte(s))
	// pass 2 - filter stop words
	tokens = filterStopwords(lang, tokens)
	// pass 3 - filter stems
	tokens = filterStemmers(lang, tokens)
	// finally, return the terms.
	var terms []string
	for i := range tokens {
		terms = append(terms, string(tokens[i].Term))
	}
	terms = x.RemoveDuplicates(terms)
	return terms, nil
}
