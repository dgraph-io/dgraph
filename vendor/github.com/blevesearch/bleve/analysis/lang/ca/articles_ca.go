package ca

import (
	"github.com/blevesearch/bleve/analysis"
	"github.com/blevesearch/bleve/registry"
)

const ArticlesName = "articles_ca"

// this content was obtained from:
// lucene-4.7.2/analysis/common/src/resources/org/apache/lucene/analysis

var CatalanArticles = []byte(`
d
l
m
n
s
t
`)

func ArticlesTokenMapConstructor(config map[string]interface{}, cache *registry.Cache) (analysis.TokenMap, error) {
	rv := analysis.NewTokenMap()
	err := rv.LoadBytes(CatalanArticles)
	return rv, err
}

func init() {
	registry.RegisterTokenMap(ArticlesName, ArticlesTokenMapConstructor)
}
