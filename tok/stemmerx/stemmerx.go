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
	"strings"

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
	"github.com/blevesearch/cld2"
	"github.com/golang/glog"
)

const Name = "stemmer_proxy"

var knownFilters = []string{
	"cjk_bigram",
	"stemmer_ar",
	"stemmer_ckb",
	"stemmer_da_snowball",
	"stemmer_de_light",
	"stemmer_es_light",
	"stemmer_fi_snowball",
	"stemmer_fr_light",
	"stemmer_hi",
	"stemmer_hu_snowball",
	"stemmer_it_light",
	"stemmer_nl_snowball",
	"stemmer_no_snowball",
	"stemmer_porter",
	"stemmer_pt_light",
	"stemmer_ro_snowball",
	"stemmer_ru_snowball",
	"stemmer_sv_snowball",
	"stemmer_tr_snowball",
}

// StemmerProxyFilter is a proxy to other stemmer filters.
type StemmerProxyFilter struct {
	filters map[string]analysis.TokenFilter
}

// Filter will try to find a stemmer filters for a detected language.
// The request is forwarded to the next filter if we can't detect language or we don't have a
// filter for it. Otherwise, we run the filter and return the new stream.
// Returns either the same input stream, or a new filtered stream using stemmer.
func (f *StemmerProxyFilter) Filter(input analysis.TokenStream) analysis.TokenStream {
	if len(input) > 0 {
		lang := cld2.Detect(string(input[0].Term))
		glog.V(3).Infof("--- detected lang: %q", lang)
		if tf, ok := f.filters[lang]; ok {
			glog.V(3).Infof("--- filtered stemmers for lang: %s", lang)
			return tf.Filter(input)
		}
	}
	return input
}

// Constructor creates a new instance of this filter.
// We run through the list of known stemmer filters 'knownFilters' and we try to
// instantiate each and save in our cache.
// Returns the stemmer proxy on success, error otherwise.
func Constructor(config map[string]interface{}, cache *registry.Cache) (analysis.TokenFilter, error) {
	proxy := &StemmerProxyFilter{filters: make(map[string]analysis.TokenFilter)}
	for _, name := range knownFilters {
		tf, err := cache.TokenFilterNamed(name)
		if err != nil {
			glog.V(3).Infof("token filter error: %s", err)
			return nil, err
		}
		switch name {
		case "stemmer_porter":
			// English uses porter.
			proxy.filters["en"] = tf
		case "cjk_bigram":
			// Japanese, Korean and Chinese use CJK bigram.
			proxy.filters["ja"] = tf
			proxy.filters["ko"] = tf
			proxy.filters["zh"] = tf
		default:
			// split: "stemmer_lang_extra" => {"stemmer", "lang", "extra"}
			proxy.filters[strings.Split(name, "_")[1]] = tf
		}
	}
	return proxy, nil
}

func init() {
	registry.RegisterTokenFilter(Name, Constructor)
}
