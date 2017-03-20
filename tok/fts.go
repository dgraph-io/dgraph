/*
 * Copyright 2016 Dgraph Labs, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * 		http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package tok

import (
	"github.com/dgraph-io/dgraph/x"

	"github.com/blevesearch/bleve/analysis/analyzer/custom"
	"github.com/blevesearch/bleve/analysis/token/lowercase"
	"github.com/blevesearch/bleve/analysis/token/porter"
	"github.com/blevesearch/bleve/analysis/token/stop"
	"github.com/blevesearch/bleve/analysis/token/unicodenorm"
	"github.com/blevesearch/bleve/analysis/tokenizer/unicode"
	"github.com/blevesearch/bleve/analysis/tokenmap"
	"github.com/blevesearch/bleve/registry"
	"github.com/blevesearch/blevex/stemmer"
	"github.com/tebeka/snowball"
)

var (
	bleveCache *registry.Cache
	langToCode map[string]string // maps language name to country code
)

const (
	normalizerFormNFKC = "nfkc"
	normalizerName     = "nfkc_normalizer"
)

func initFullTextTokenizers() {
	bleveCache = registry.NewCache()

	defineNormalizer()
	defineTermAnalyzer()

	// List of supported languages (as defined in https://github.com/tebeka/snowball):
	// danish, dutch, english, finnish, french, german, hungarian, italian, norwegian, porter,
	// portuguese, romanian, russian, spanish, swedish, turkish
	for _, lang := range snowball.LangList() {
		if lang == "porter" {
			continue
		}

		defineStemmer(lang)
		defineStopWordsList(lang)
		defineAnalyzer(lang)
		RegisterTokenizer(&FullTextTokenizer{Lang: countryCode(lang)})
	}

	// Default full text tokenizer, with Porter stemmer (it works with English only).
	defineDefaultFullTextAnalyzer()
	RegisterTokenizer(FullTextTokenizer{})
}

// Create normalizer using Normalization Form KC (NFKC) - Compatibility Decomposition, followed
// by Canonical Composition. See: http://unicode.org/reports/tr15/#Norm_Forms
func defineNormalizer() {
	_, err := bleveCache.DefineTokenFilter(normalizerName, map[string]interface{}{
		"type": unicodenorm.Name,
		"form": normalizerFormNFKC,
	})
	x.Check(err)
}

func defineStemmer(lang string) {
	_, err := bleveCache.DefineTokenFilter(stemmerName(countryCode(lang)), map[string]interface{}{
		"type": stemmer.Name,
		"lang": lang,
	})
	x.Check(err)
}

func defineStopWordsList(lang string) {
	name := stopWordsListName(countryCode(lang))
	_, err := bleveCache.DefineTokenMap(name, map[string]interface{}{
		"type":   tokenmap.Name,
		"tokens": stopwords[lang],
	})
	x.Check(err)

	_, err = bleveCache.DefineTokenFilter(name, map[string]interface{}{
		"type":           stop.Name,
		"stop_token_map": name,
	})
	x.Check(err)
}

// basic analyzer - splits on word boundaries, lowercase and normalize tokens
func defineTermAnalyzer() {
	_, err := bleveCache.DefineAnalyzer("term", map[string]interface{}{
		"type":          custom.Name,
		"tokenizer":     unicode.Name,
		"token_filters": []string{lowercase.Name, normalizerName},
	})
	x.Check(err)
}

// default full text search analyzer - does english stop-words removal and Porter stemming
func defineDefaultFullTextAnalyzer() {
	_, err := bleveCache.DefineAnalyzer("fulltext", map[string]interface{}{
		"type":      custom.Name,
		"tokenizer": unicode.Name,
		"token_filters": []string{
			lowercase.Name,
			normalizerName,
			stopWordsListName("en"),
			porter.Name},
	})
	x.Check(err)
}

// full text search analyzer - does language-specific stop-words removal and stemming
func defineAnalyzer(lang string) {
	ln := countryCode(lang)
	_, err := bleveCache.DefineAnalyzer(ftsTokenizerName(ln), map[string]interface{}{
		"type":      custom.Name,
		"tokenizer": unicode.Name,
		"token_filters": []string{
			lowercase.Name,
			normalizerName,
			stopWordsListName(ln),
			stemmerName(ln),
		},
	})
	x.Check(err)
}

func ftsTokenizerName(lang string) string {
	return "fulltext" + lang
}

func stemmerName(lang string) string {
	return stemmer.Name + lang
}

func stopWordsListName(lang string) string {
	return stop.Name + lang
}

func countryCode(lang string) string {
	code, ok := langToCode[lang]
	x.AssertTruef(ok, "Unsupported language: %s", lang)
	return code
}

func init() {
	// List based on https://godoc.org/golang.org/x/text/language#Tag
	// It contains more languages than supported by Bleve, to enable seamless addition of new langs.
	langToCode = map[string]string{
		"afrikaans":            "af",
		"amharic":              "am",
		"arabic":               "ar",
		"modernstandardarabic": "ar-001",
		"azerbaijani":          "az",
		"bulgarian":            "bg",
		"bengali":              "bn",
		"catalan":              "ca",
		"czech":                "cs",
		"danish":               "da",
		"german":               "de",
		"greek":                "el",
		"english":              "en",
		"americanenglish":      "en-us",
		"britishenglish":       "en-gb",
		"spanish":              "es",
		"europeanspanish":      "es-es",
		"latinamericanspanish": "es-419",
		"estonian":             "et",
		"persian":              "fa",
		"finnish":              "fi",
		"filipino":             "fil",
		"french":               "fr",
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
		"portuguese":           "pt",
		"brazilianportuguese":  "pt-br",
		"europeanportuguese":   "pt-pt",
		"romanian":             "ro",
		"russian":              "ru",
		"sinhala":              "si",
		"slovak":               "sk",
		"slovenian":            "sl",
		"albanian":             "sq",
		"serbian":              "sr",
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
