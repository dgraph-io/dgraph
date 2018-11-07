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

package stopx

import (
	"github.com/blevesearch/bleve/analysis"
	_ "github.com/blevesearch/bleve/analysis/lang/ar"
	_ "github.com/blevesearch/bleve/analysis/lang/bg"
	_ "github.com/blevesearch/bleve/analysis/lang/ca"
	_ "github.com/blevesearch/bleve/analysis/lang/ckb"
	_ "github.com/blevesearch/bleve/analysis/lang/cs"
	_ "github.com/blevesearch/bleve/analysis/lang/da"
	_ "github.com/blevesearch/bleve/analysis/lang/de"
	_ "github.com/blevesearch/bleve/analysis/lang/el"
	_ "github.com/blevesearch/bleve/analysis/lang/en"
	_ "github.com/blevesearch/bleve/analysis/lang/es"
	_ "github.com/blevesearch/bleve/analysis/lang/eu"
	_ "github.com/blevesearch/bleve/analysis/lang/fa"
	_ "github.com/blevesearch/bleve/analysis/lang/fi"
	_ "github.com/blevesearch/bleve/analysis/lang/fr"
	_ "github.com/blevesearch/bleve/analysis/lang/ga"
	_ "github.com/blevesearch/bleve/analysis/lang/gl"
	_ "github.com/blevesearch/bleve/analysis/lang/hi"
	_ "github.com/blevesearch/bleve/analysis/lang/hu"
	_ "github.com/blevesearch/bleve/analysis/lang/hy"
	_ "github.com/blevesearch/bleve/analysis/lang/id"
	_ "github.com/blevesearch/bleve/analysis/lang/it"
	_ "github.com/blevesearch/bleve/analysis/lang/nl"
	_ "github.com/blevesearch/bleve/analysis/lang/no"
	_ "github.com/blevesearch/bleve/analysis/lang/pt"
	_ "github.com/blevesearch/bleve/analysis/lang/ro"
	_ "github.com/blevesearch/bleve/analysis/lang/ru"
	_ "github.com/blevesearch/bleve/analysis/lang/sv"
	_ "github.com/blevesearch/bleve/analysis/lang/tr"
	"github.com/blevesearch/bleve/registry"
	"github.com/blevesearch/cld2"
	"github.com/golang/glog"
	// _ "github.com/blevesearch/blevex/lang/th"
)

const Name = "stop_tokens_proxy"

// TODO: fix Thai stop tokens.
var knownFilters = []string{
	"stop_ar",
	"stop_bg",
	"stop_ca",
	"stop_ckb",
	"stop_cs",
	"stop_da",
	"stop_de",
	"stop_el",
	"stop_en",
	"stop_es",
	"stop_eu",
	"stop_fa",
	"stop_fi",
	"stop_fr",
	"stop_ga",
	"stop_gl",
	"stop_hi",
	"stop_hu",
	"stop_hy",
	"stop_id",
	"stop_it",
	"stop_nl",
	"stop_no",
	"stop_pt",
	"stop_ro",
	"stop_ru",
	"stop_sv",
	// "stop_th",
	"stop_tr",
}

// StopTokensProxyFilter is a proxy to other stop token filters.
type StopTokensProxyFilter struct {
	filters map[string]analysis.TokenFilter
}

// Filter will try to find a stop tokens filters for a detected language.
// The request is forwarded to the next filter if we can't detect language or we don't have a
// filter for it. Otherwise, we run the filter and return the new stream.
// Returns either the same input stream, or a new filtered stream using stop tokens.
func (f *StopTokensProxyFilter) Filter(input analysis.TokenStream) analysis.TokenStream {
	if len(input) == 0 {
		return input
	}
	lang := cld2.Detect(string(input[0].Term))
	glog.V(3).Infof("--- detected lang: %q", lang)
	if lang == "" {
		return input
	}
	if tf, ok := f.filters[lang]; ok {
		glog.V(3).Infof("--- filtered stop tokens for lang: %s", lang)
		return tf.Filter(input)
	}
	return input
}

// Constructor creates a new instance of this filter.
// We run through the list of known stop token filters 'knownFilters' and we try to
// instantiate each and save in our cache.
// Returns the stop token proxy on success, error otherwise.
func Constructor(config map[string]interface{}, cache *registry.Cache) (analysis.TokenFilter, error) {
	proxy := &StopTokensProxyFilter{filters: make(map[string]analysis.TokenFilter)}
	for _, name := range knownFilters {
		tf, err := cache.TokenFilterNamed(name)
		if err != nil {
			glog.V(3).Infof("token filter error: %s", err)
			return nil, err
		}
		proxy.filters[name[5:]] = tf
	}
	return proxy, nil
}

func init() {
	registry.RegisterTokenFilter(Name, Constructor)
}
