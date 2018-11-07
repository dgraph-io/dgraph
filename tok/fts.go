/*
 * Copyright 2017-2018 Dgraph Labs, Inc. and Contributors
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
	"strings"

	"github.com/dgraph-io/dgraph/tok/stemmerx"
	"github.com/dgraph-io/dgraph/tok/stopx"
	"github.com/dgraph-io/dgraph/x"

	"github.com/blevesearch/bleve/analysis/analyzer/custom"
	"github.com/blevesearch/bleve/analysis/lang/cjk"
	"github.com/blevesearch/bleve/analysis/token/lowercase"
	"github.com/blevesearch/bleve/analysis/token/unicodenorm"
	"github.com/blevesearch/bleve/analysis/tokenizer/unicode"
	"github.com/blevesearch/bleve/registry"
)

var (
	bleveCache *registry.Cache
	langToCode map[string]string // maps language name to country code
)

const (
	// FTSTokenizerName  = "fulltext"
	filterUnicodeNorm = "unicodenorm_nfkc"
)

func initFullTextTokenizers() {
	bleveCache = registry.NewCache()

	// unicode normalizer filter - simplifies unicode words using Normalization Form KC (NFKC)
	// See: http://unicode.org/reports/tr15/#Norm_Forms
	_, err := bleveCache.DefineTokenFilter(filterUnicodeNorm,
		map[string]interface{}{
			"type": unicodenorm.Name,
			"form": unicodenorm.NFKC,
		})
	x.Check(err)

	// term analyzer - splits on word boundaries, lowercase and normalize tokens
	_, err = bleveCache.DefineAnalyzer("term",
		map[string]interface{}{
			"type":      custom.Name,
			"tokenizer": unicode.Name,
			"token_filters": []string{
				lowercase.Name,
				filterUnicodeNorm,
			},
		})
	x.Check(err)

	// Default full text tokenizer, with Porter stemmer (it works with English only).
	// fulltext analyzer - does english stop-words removal and Porter stemming
	_, err = bleveCache.DefineAnalyzer("fulltext",
		map[string]interface{}{
			"type":      custom.Name,
			"tokenizer": unicode.Name,
			"token_filters": []string{
				lowercase.Name,
				filterUnicodeNorm,
				stopx.Name,
				stemmerx.Name,
			},
		})
	x.Check(err)

	registerTokenizer(FullTextTokenizer{})
}

// Full text search analyzer - does Chinese/Japanese/Korean style bigram
// tokenization. It's language unaware (so doesn't do stemming or stop
// words), but works OK in some contexts.
func defineCJKAnalyzer(cc string) {
	_, err := bleveCache.DefineAnalyzer(FtsTokenizerName(cc), map[string]interface{}{
		"type":      custom.Name,
		"tokenizer": unicode.Name,
		"token_filters": []string{
			filterUnicodeNorm,
			cjk.BigramName,
		},
	})
	x.Check(err)
}

func countryCode(lang string) string {
	return countryCodes(lang)[0]
}

func countryCodes(lang string) []string {
	codes, ok := langToCode[lang]
	x.AssertTruef(ok, "Unsupported language: %s", lang)
	return strings.Split(codes, ",")
}

func init() {
	// List based on https://godoc.org/golang.org/x/text/language#Tag
	// It contains more languages than supported by Bleve, to enable seamless addition of new langs.
	// Issue#2601: added aliasing of related languages to broaden support. When those langs are added
	// the aliases won't matter.
	langToCode = map[string]string{
		"afrikaans":            "af",
		"amharic":              "am",
		"arabic":               "ar,ar-001",
		"modernstandardarabic": "ar-001",
		"azerbaijani":          "az",
		"bulgarian":            "bg",
		"bengali":              "bn",
		"catalan":              "ca",
		"czech":                "cs",
		"danish":               "da",
		"german":               "de",
		"greek":                "el",
		"english":              "en,en-us,en-gb",
		"americanenglish":      "en-us",
		"britishenglish":       "en-gb",
		"spanish":              "es,es-es,es-419",
		"europeanspanish":      "es-es",
		"latinamericanspanish": "es-419",
		"estonian":             "et",
		"persian":              "fa",
		"finnish":              "fi",
		"filipino":             "fil",
		"french":               "fr,fr-ca",
		"canadianfrench":       "fr-ca",
		"gujarati":             "gu",
		"hebrew":               "he",
		"hindi":                "hi",
		"croatian":             "hr",
		"hungarian":            "hu",
		"armenian":             "hy",
		"indonesian":           "id",
		"icelandic":            "is",
		"italian":              "it",
		"japanese":             "ja",
		"georgian":             "ka",
		"kazakh":               "kk",
		"khmer":                "km",
		"kannada":              "kn",
		"korean":               "ko",
		"kirghiz":              "ky",
		"lao":                  "lo",
		"lithuanian":           "lt",
		"latvian":              "lv",
		"macedonian":           "mk",
		"malayalam":            "ml",
		"mongolian":            "mn",
		"marathi":              "mr",
		"malay":                "ms",
		"burmese":              "my",
		"nepali":               "ne",
		"dutch":                "nl",
		"norwegian":            "no",
		"punjabi":              "pa",
		"polish":               "pl",
		"portuguese":           "pt,pt-br,pt-pt",
		"brazilianportuguese":  "pt-br",
		"europeanportuguese":   "pt-pt",
		"romanian":             "ro",
		"russian":              "ru",
		"sinhala":              "si",
		"slovak":               "sk",
		"slovenian":            "sl",
		"albanian":             "sq",
		"serbian":              "sr,sr-latn",
		"serbianlatin":         "sr-latn",
		"swedish":              "sv",
		"swahili":              "sw",
		"tamil":                "ta",
		"telugu":               "te",
		"thai":                 "th",
		"turkish":              "tr",
		"ukrainian":            "uk",
		"urdu":                 "ur",
		"uzbek":                "uz",
		"vietnamese":           "vi",
		"chinese":              "zh",
		"simplifiedchinese":    "zh-hans",
		"traditionalchinese":   "zh-hant",
		"zulu":                 "zu",
	}
}
