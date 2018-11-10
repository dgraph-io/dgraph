package ar

import (
	"github.com/blevesearch/bleve/analysis"
	"github.com/blevesearch/bleve/registry"
)

const StopName = "stop_ar"

// this content was obtained from:
// lucene-4.7.2/analysis/common/src/resources/org/apache/lucene/analysis
// ` was changed to ' to allow for literal string

var ArabicStopWords = []byte(`# This file was created by Jacques Savoy and is distributed under the BSD license.
# See http://members.unine.ch/jacques.savoy/clef/index.html.
# Also see http://www.opensource.org/licenses/bsd-license.html
# Cleaned on October 11, 2009 (not normalized, so use before normalization)
# This means that when modifying this list, you might need to add some 
# redundant entries, for example containing forms with both أ and ا
من
ومن
منها
منه
في
وفي
فيها
فيه
و
ف
ثم
او
أو
ب
بها
به
ا
أ
اى
اي
أي
أى
لا
ولا
الا
ألا
إلا
لكن
ما
وما
كما
فما
عن
مع
اذا
إذا
ان
أن
إن
انها
أنها
إنها
انه
أنه
إنه
بان
بأن
فان
فأن
وان
وأن
وإن
التى
التي
الذى
الذي
الذين
الى
الي
إلى
إلي
على
عليها
عليه
اما
أما
إما
ايضا
أيضا
كل
وكل
لم
ولم
لن
ولن
هى
هي
هو
وهى
وهي
وهو
فهى
فهي
فهو
انت
أنت
لك
لها
له
هذه
هذا
تلك
ذلك
هناك
كانت
كان
يكون
تكون
وكانت
وكان
غير
بعض
قد
نحو
بين
بينما
منذ
ضمن
حيث
الان
الآن
خلال
بعد
قبل
حتى
عند
عندما
لدى
جميع
`)

func TokenMapConstructor(config map[string]interface{}, cache *registry.Cache) (analysis.TokenMap, error) {
	rv := analysis.NewTokenMap()
	err := rv.LoadBytes(ArabicStopWords)
	return rv, err
}

func init() {
	registry.RegisterTokenMap(StopName, TokenMapConstructor)
}
