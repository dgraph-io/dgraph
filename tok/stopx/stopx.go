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
)

const Name = "stop_tokens_proxy"

var langStops = map[string]string{
	"ar":  "stop_ar",
	"bg":  "stop_bg",
	"ca":  "stop_ca",
	"ckb": "stop_ckb",
	"cs":  "stop_cs",
	"da":  "stop_da",
	"de":  "stop_de",
	"el":  "stop_el",
	"en":  "stop_en",
	"es":  "stop_es",
	"eu":  "stop_eu",
	"fa":  "stop_fa",
	"fi":  "stop_fi",
	"fr":  "stop_fr",
	"ga":  "stop_ga",
	"gl":  "stop_gl",
	"hi":  "stop_hi",
	"hu":  "stop_hu",
	"hy":  "stop_hy",
	"id":  "stop_id",
	"it":  "stop_it",
	"nl":  "stop_nl",
	"no":  "stop_no",
	"pt":  "stop_pt",
	"ro":  "stop_ro",
	"ru":  "stop_ru",
	"sv":  "stop_sv",
	"tr":  "stop_tr",
}

func getLangStopName(lang string) string {
	if name, ok := langStops[lang]; ok {
		return name
	}
	return ""
}

// StopTokensProxyFilter is a proxy to other stop token filters.
type StopTokensProxyFilter struct {
	lang   string
	filter analysis.TokenFilter
}

// New returns a new instance of this filter proxy.
// If the lang filter is found, the instance will forward requests to it.
// Otherwise, the instance is no-op.
func New(cache *registry.Cache, lang string) *StopTokensProxyFilter {
	name := getLangStopName(lang)
	if name == "" {
		return &StopTokensProxyFilter{}
	}
	filter, err := cache.TokenFilterNamed(name)
	if err != nil {
		return &StopTokensProxyFilter{}
	}
	return &StopTokensProxyFilter{lang: lang, filter: filter}
}

// Filter will forward the request to the lang filter and return the new stream.
// Returns either the same input stream, or a new filtered stream using stop tokens.
func (f *StopTokensProxyFilter) Filter(input analysis.TokenStream) analysis.TokenStream {
	if len(input) > 0 && f.filter != nil {
		return f.filter.Filter(input)
	}
	return input
}

// Constructor creates a new instance of this filter. Used when defining analyzers.
// Returns the stop token proxy on success, error otherwise.
func Constructor(config map[string]interface{}, cache *registry.Cache) (analysis.TokenFilter, error) {
	lang, ok := config["lang"].(string)
	if !ok {
		lang = "en"
	}
	filter, err := cache.TokenFilterNamed(getLangStopName(lang))
	if err != nil {
		return nil, err
	}
	return &StopTokensProxyFilter{lang: lang, filter: filter}, nil
}

func init() {
	registry.RegisterTokenFilter(Name, Constructor)
}
