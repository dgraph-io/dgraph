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
	"sync"

	"github.com/golang/glog"
	"golang.org/x/text/language"
)

const enBase = "en"

// langBaseCache keeps a copy of lang -> base conversions.
var langBaseCache struct {
	sync.Mutex
	m map[string]string
}

// langBase returns the BCP47 base of a language.
// If the confidence of the matching is better than none, we return that base.
// Otherwise, we return "en" (English) which is a good default.
func langBase(lang string) string {
	if lang == "" {
		return enBase // default to this
	}
	langBaseCache.Lock()
	defer langBaseCache.Unlock()
	if langBaseCache.m == nil {
		langBaseCache.m = make(map[string]string)
	}
	// check if we already have this
	if s, found := langBaseCache.m[lang]; found {
		return s
	}
	// Parse will return the best guess for a language tag.
	// It will return undefined, or 'language.Und', if it gives up. That means the language
	// tag is either new (to the standard) or simply invalid.
	// We ignore errors from Parse because to Dgraph they aren't fatal.
	tag, err := language.Parse(lang)
	if err != nil {
		glog.Errorf("While trying to parse lang %q. Error: %v", lang, err)

	} else if tag != language.Und {
		// Found a not undefined, i.e. valid language.
		// The tag value returned will have a 'confidence' value attached.
		// The confidence will be one of: No, Low, High, Exact.
		// Low confidence is close to being undefined (see above) so we treat it as such.
		// Any other confidence values are good enough for us.
		// e.g., A lang tag like "x-klingon" should retag to "en"
		if base, conf := tag.Base(); conf > language.No {
			langBaseCache.m[lang] = base.String()
			return base.String()
		}
	}
	glog.Warningf("Unable to find lang %q. Reverting to English.", lang)
	langBaseCache.m[lang] = enBase
	return enBase
}
