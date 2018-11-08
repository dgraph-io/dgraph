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

import "golang.org/x/text/language"

const enBase = "en"

// langTop20 top 20 languages by use.
var langTop20 = map[string]struct{}{
	"zh": struct{}{},
	"es": struct{}{},
	"en": struct{}{},
	"hi": struct{}{},
	"ar": struct{}{},
	"bn": struct{}{},
	"pt": struct{}{},
	"ru": struct{}{},
	"ja": struct{}{},
	"de": struct{}{},
	"ko": struct{}{},
	"fr": struct{}{},
	"it": struct{}{},
	"pl": struct{}{},
	"da": struct{}{},
	"nl": struct{}{},
	"no": struct{}{},
	"ro": struct{}{},
	"hu": struct{}{},
	"fi": struct{}{},
}

// langBase returns the BCP47 base of a language.
// If the confidence of the matching is better than none, we return that base.
// Otherwise, we return "en" (English) which is a good default.
// TODO: we need a small LRU cache to speed this up in large imports.
func langBase(lang string) string {
	if _, known := langTop20[lang]; known {
		return lang
	}
	tag, _ := language.Parse(lang)
	if tag != language.Und { // something like "x-klingon" should retag to "en"
		if base, conf := tag.Base(); conf > language.No {
			return base.String()
		}
	}
	// return language.English.String()
	return enBase
}
