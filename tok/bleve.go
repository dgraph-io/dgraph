/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package tok

import (
	"github.com/blevesearch/bleve/v2/analysis"
	"github.com/blevesearch/bleve/v2/analysis/analyzer/custom"
	"github.com/blevesearch/bleve/v2/analysis/token/lowercase"
	"github.com/blevesearch/bleve/v2/analysis/token/unicodenorm"
	"github.com/blevesearch/bleve/v2/analysis/tokenizer/unicode"
	"github.com/blevesearch/bleve/v2/registry"

	"github.com/hypermodeinc/dgraph/v25/x"
)

const unicodenormName = "unicodenorm_nfkc"

var (
	bleveCache                     = registry.NewCache()
	termAnalyzer, fulltextAnalyzer analysis.Analyzer
)

// setupBleve creates bleve filters and analyzers that we use for term and fulltext tokenizers.
func setupBleve() {
	// unicode normalizer filter - simplifies unicode words using Normalization Form KC (NFKC)
	// See: http://unicode.org/reports/tr15/#Norm_Forms
	_, err := bleveCache.DefineTokenFilter(unicodenormName,
		map[string]interface{}{
			"type": unicodenorm.Name,
			"form": unicodenorm.NFKC,
		})
	x.Check(err)

	// term analyzer - splits on word boundaries, lowercase and normalize tokens.
	termAnalyzer, err = bleveCache.DefineAnalyzer("term",
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
	fulltextAnalyzer, err = bleveCache.DefineAnalyzer("fulltext",
		map[string]interface{}{
			"type":      custom.Name,
			"tokenizer": unicode.Name,
			"token_filters": []string{
				lowercase.Name,
				unicodenormName,
			},
		})
	x.Check(err)
}

// uniqueTerms takes a token stream and returns a string slice of unique terms.
func uniqueTerms(tokens analysis.TokenStream) []string {
	var terms []string
	for i := range tokens {
		terms = append(terms, string(tokens[i].Term))
	}
	terms = x.RemoveDuplicates(terms)
	return terms
}
