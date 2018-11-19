package fr

import (
	"github.com/blevesearch/bleve/analysis"
	"github.com/blevesearch/bleve/registry"
)

const ArticlesName = "articles_fr"

// this content was obtained from:
// lucene-4.7.2/analysis/common/src/resources/org/apache/lucene/analysis

var FrenchArticles = []byte(`
l
m
t
qu
n
s
j
d
c
jusqu
quoiqu
lorsqu
puisqu
`)

func ArticlesTokenMapConstructor(config map[string]interface{}, cache *registry.Cache) (analysis.TokenMap, error) {
	rv := analysis.NewTokenMap()
	err := rv.LoadBytes(FrenchArticles)
	return rv, err
}

func init() {
	registry.RegisterTokenMap(ArticlesName, ArticlesTokenMapConstructor)
}
