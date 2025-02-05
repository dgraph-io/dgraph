/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
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
	sync.RWMutex
	m map[string]string
}

// LangBase returns the BCP47 base of a language.
// If the confidence of the matching is better than none, we return that base.
// Otherwise, we return "en" (English) which is a good default.
func LangBase(lang string) string {
	if lang == "" {
		return enBase // default to this
	}

	// Acquire the read lock and lookup for the lang in cache.
	langBaseCache.RLock()

	// check if we already have this
	if s, found := langBaseCache.m[lang]; found {
		langBaseCache.RUnlock()
		return s
	}

	// Upgrade the lock only since the lang is not found in cache.
	langBaseCache.RUnlock()
	langBaseCache.Lock()
	defer langBaseCache.Unlock()

	// Recheck if the lang is added to the cache.
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

func init() {
	langBaseCache.m = make(map[string]string)
}
