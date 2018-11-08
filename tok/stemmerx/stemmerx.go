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

package stemmerx

import (
	"github.com/blevesearch/bleve/analysis"
	_ "github.com/blevesearch/bleve/analysis/lang/ar"
	_ "github.com/blevesearch/bleve/analysis/lang/cjk"
	_ "github.com/blevesearch/bleve/analysis/lang/ckb"
	_ "github.com/blevesearch/bleve/analysis/lang/da"
	_ "github.com/blevesearch/bleve/analysis/lang/de"
	_ "github.com/blevesearch/bleve/analysis/lang/es"
	_ "github.com/blevesearch/bleve/analysis/lang/fi"
	_ "github.com/blevesearch/bleve/analysis/lang/fr"
	_ "github.com/blevesearch/bleve/analysis/lang/hi"
	_ "github.com/blevesearch/bleve/analysis/lang/hu"
	_ "github.com/blevesearch/bleve/analysis/lang/it"
	_ "github.com/blevesearch/bleve/analysis/lang/nl"
	_ "github.com/blevesearch/bleve/analysis/lang/no"
	_ "github.com/blevesearch/bleve/analysis/lang/pt"
	_ "github.com/blevesearch/bleve/analysis/lang/ro"
	_ "github.com/blevesearch/bleve/analysis/lang/ru"
	_ "github.com/blevesearch/bleve/analysis/lang/sv"
	_ "github.com/blevesearch/bleve/analysis/lang/tr"
	_ "github.com/blevesearch/bleve/analysis/token/porter"
	"github.com/blevesearch/bleve/registry"
)

const Name = "stemmer_proxy"

var langStemmers = map[string]string{
	"ar":  "stemmer_ar",
	"ckb": "stemmer_ckb",
	"da":  "stemmer_da_snowball",
	"de":  "stemmer_de_light",
	"en":  "stemmer_porter",
	"es":  "stemmer_es_light",
	"fi":  "stemmer_fi_snowball",
	"fr":  "stemmer_fr_light",
	"hi":  "stemmer_hi",
	"hu":  "stemmer_hu_snowball",
	"it":  "stemmer_it_light",
	"ja":  "cjk_bigram",
	"ko":  "cjk_bigram",
	"nl":  "stemmer_nl_snowball",
	"no":  "stemmer_no_snowball",
	"pt":  "stemmer_pt_light",
	"ro":  "stemmer_ro_snowball",
	"ru":  "stemmer_ru_snowball",
	"sv":  "stemmer_sv_snowball",
	"tr":  "stemmer_tr_snowball",
	"zh":  "cjk_bigram",
}

func getLangStemmerName(lang string) string {
	if name, ok := langStemmers[lang]; ok {
		return name
	}
	return ""
}

// StemmerProxyFilter is a proxy to other stemmer filters.
type StemmerProxyFilter struct {
	lang   string
	filter analysis.TokenFilter
}

// New returns a new instance of this filter proxy.
// If the lang filter is found, the instance will forward requests to it.
// Otherwise, the instance is no-op.
func New(cache *registry.Cache, lang string) *StemmerProxyFilter {
	name := getLangStemmerName(lang)
	if name == "" {
		return &StemmerProxyFilter{}
	}
	filter, err := cache.TokenFilterNamed(name)
	if err != nil {
		return &StemmerProxyFilter{}
	}
	return &StemmerProxyFilter{lang: lang, filter: filter}
}

// Filter will forward the request to the lang filter and return the new stream.
// Returns either the same input stream, or a new filtered stream using stemmer.
func (f *StemmerProxyFilter) Filter(input analysis.TokenStream) analysis.TokenStream {
	if len(input) > 0 && f.filter != nil {
		return f.filter.Filter(input)
	}
	return input
}

// Constructor creates a new instance of this filter. Used when defining analyzers.
// Returns the stemmer proxy on success, error otherwise.
func Constructor(config map[string]interface{}, cache *registry.Cache) (analysis.TokenFilter, error) {
	lang, ok := config["lang"].(string)
	if !ok {
		lang = "en"
	}
	filter, err := cache.TokenFilterNamed(getLangStemmerName(lang))
	if err != nil {
		return nil, err
	}
	return &StemmerProxyFilter{lang: lang, filter: filter}, nil
}

func init() {
	registry.RegisterTokenFilter(Name, Constructor)
}
