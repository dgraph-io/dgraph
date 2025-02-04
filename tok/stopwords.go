/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package tok

import (
	"github.com/blevesearch/bleve/v2/analysis"
	_ "github.com/blevesearch/bleve/v2/analysis/lang/ar" // Needed for bleve language support.
	_ "github.com/blevesearch/bleve/v2/analysis/lang/bg"
	_ "github.com/blevesearch/bleve/v2/analysis/lang/ca"
	_ "github.com/blevesearch/bleve/v2/analysis/lang/ckb"
	_ "github.com/blevesearch/bleve/v2/analysis/lang/cs"
	_ "github.com/blevesearch/bleve/v2/analysis/lang/da"
	_ "github.com/blevesearch/bleve/v2/analysis/lang/de"
	_ "github.com/blevesearch/bleve/v2/analysis/lang/el"
	_ "github.com/blevesearch/bleve/v2/analysis/lang/en"
	_ "github.com/blevesearch/bleve/v2/analysis/lang/es"
	_ "github.com/blevesearch/bleve/v2/analysis/lang/eu"
	_ "github.com/blevesearch/bleve/v2/analysis/lang/fa"
	_ "github.com/blevesearch/bleve/v2/analysis/lang/fi"
	_ "github.com/blevesearch/bleve/v2/analysis/lang/fr"
	_ "github.com/blevesearch/bleve/v2/analysis/lang/ga"
	_ "github.com/blevesearch/bleve/v2/analysis/lang/gl"
	_ "github.com/blevesearch/bleve/v2/analysis/lang/hi"
	_ "github.com/blevesearch/bleve/v2/analysis/lang/hu"
	_ "github.com/blevesearch/bleve/v2/analysis/lang/hy"
	_ "github.com/blevesearch/bleve/v2/analysis/lang/id"
	_ "github.com/blevesearch/bleve/v2/analysis/lang/it"
	_ "github.com/blevesearch/bleve/v2/analysis/lang/nl"
	_ "github.com/blevesearch/bleve/v2/analysis/lang/no"
	_ "github.com/blevesearch/bleve/v2/analysis/lang/pt"
	_ "github.com/blevesearch/bleve/v2/analysis/lang/ro"
	_ "github.com/blevesearch/bleve/v2/analysis/lang/ru"
	_ "github.com/blevesearch/bleve/v2/analysis/lang/sv"
	_ "github.com/blevesearch/bleve/v2/analysis/lang/tr"
	"github.com/golang/glog"
)

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

// filterStopwords filters stop words using an existing filter, imported here.
// If the lang filter is found, the we will forward requests to it.
// Returns filtered tokens if filter is found, otherwise returns tokens unmodified.
func filterStopwords(lang string, input analysis.TokenStream) analysis.TokenStream {
	if len(input) == 0 {
		return input
	}
	// check if we have stop words filter for this lang.
	name, ok := langStops[lang]
	if !ok {
		return input
	}
	// get filter from concurrent cache so we dont recreate.
	filter, err := bleveCache.TokenFilterNamed(name)
	if err != nil {
		glog.Errorf("Error while filtering %q stop words: %s", lang, err)
		return input
	}
	if filter != nil {
		return filter.Filter(input)
	}
	return input
}
