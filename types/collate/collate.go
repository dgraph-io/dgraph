/*
 * Copyright 2019 Dgraph Labs, Inc. and Contributors
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

package collate

import (
	"sync"

	"github.com/golang/glog"
	"golang.org/x/text/collate"
	"golang.org/x/text/language"
)

const enBase = "en"

// collateCache stores mapping of lang -> Collator.
var collateCache struct {
	sync.Mutex
	m map[string]*collate.Collator
}

// Collator returns the collator object used to compare and sort strings according to a particular collation order.
// If the confidence of the matching is better than none, we return that particular collator object.
// Otherwise, we return the default collator object of "en" (English).
func Collator(lang string) *collate.Collator {
	collateCache.Lock()
	defer collateCache.Unlock()

	if lang == "" {
		return collateCache.m[enBase]
	}

	// Check for collator object in cache.
	if cl, found := collateCache.m[lang]; found {
		return cl
	}

	// Parse will return the best guess for a language tag.
	// It will return undefined, or 'language.Und', if it gives up. That means the language
	// tag is either new (to the standard) or simply invalid.
	if tag, err := language.Parse(lang); err != nil {
		glog.Errorf("While trying to parse lang %q. Error: %v", lang, err)
	} else if tag != language.Und {
		if _, conf := tag.Base(); conf > language.No {
			// Caching the collator object for future use.
			cl := collate.New(tag)
			collateCache.m[lang] = cl
			return cl
		}
	}

	glog.Warningf("Unable to find lang %q. Reverting to English.", lang)
	return collateCache.m[enBase]
}

func init() {
	// Initalize the cache and add the default Collator.
	collateCache.m = make(map[string]*collate.Collator)
	defaultTag, _ := language.Parse(enBase)
	defaultCollater := collate.New(defaultTag)
	collateCache.m[enBase] = defaultCollater
}
