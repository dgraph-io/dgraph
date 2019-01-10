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
	"github.com/blevesearch/bleve/analysis"
	_ "github.com/blevesearch/bleve/analysis/lang/ar" // Needed for bleve language support.
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
	"github.com/golang/glog"
)

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

// filterStemmers filters stems using an existing filter, imported here.
// If the lang filter is found, the we will forward requests to it.
// Returns filtered tokens if filter is found, otherwise returns tokens unmodified.
func filterStemmers(lang string, input analysis.TokenStream) analysis.TokenStream {
	if len(input) == 0 {
		return input
	}
	// check if we have stemmer filter for this lang.
	name, ok := langStemmers[lang]
	if !ok {
		return input
	}
	// get filter from concurrent cache so we dont recreate.
	filter, err := bleveCache.TokenFilterNamed(name)
	if err != nil {
		glog.Errorf("Error while filtering %q stems: %s", lang, err)
		return input
	}
	if filter != nil {
		return filter.Filter(input)
	}
	return input
}
