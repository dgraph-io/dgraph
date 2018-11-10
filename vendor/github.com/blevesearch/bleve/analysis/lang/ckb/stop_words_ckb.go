package ckb

import (
	"github.com/blevesearch/bleve/analysis"
	"github.com/blevesearch/bleve/registry"
)

const StopName = "stop_ckb"

// this content was obtained from:
// lucene-4.7.2/analysis/common/src/resources/org/apache/lucene/analysis/
// ` was changed to ' to allow for literal string

var SoraniStopWords = []byte(`# set of kurdish stopwords
# note these have been normalized with our scheme (e represented with U+06D5, etc)
# constructed from:
# * Fig 5 of "Building A Test Collection For Sorani Kurdish" (Esmaili et al)
# * "Sorani Kurdish: A Reference Grammar with selected readings" (Thackston)
# * Corpus-based analysis of 77M word Sorani collection: wikipedia, news, blogs, etc

# and
و
# which
کە
# of
ی
# made/did
کرد
# that/which
ئەوەی
# on/head
سەر
# two
دوو
# also
هەروەها
# from/that
لەو
# makes/does
دەکات
# some
چەند
# every
هەر

# demonstratives
# that
ئەو
# this
ئەم

# personal pronouns
# I
من
# we
ئێمە
# you
تۆ
# you
ئێوە
# he/she/it
ئەو
# they
ئەوان

# prepositions
# to/with/by
بە
پێ
# without
بەبێ
# along with/while/during
بەدەم
# in the opinion of
بەلای
# according to
بەپێی
# before
بەرلە
# in the direction of
بەرەوی
# in front of/toward
بەرەوە
# before/in the face of
بەردەم
# without
بێ
# except for
بێجگە
# for
بۆ
# on/in
دە
تێ
# with
دەگەڵ
# after
دوای
# except for/aside from
جگە
# in/from
لە
لێ
# in front of/before/because of
لەبەر
# between/among
لەبەینی
# concerning/about
لەبابەت
# concerning
لەبارەی
# instead of
لەباتی
# beside
لەبن
# instead of
لەبرێتی
# behind
لەدەم
# with/together with
لەگەڵ
# by
لەلایەن
# within
لەناو
# between/among
لەنێو
# for the sake of
لەپێناوی
# with respect to
لەرەوی
# by means of/for
لەرێ
# for the sake of
لەرێگا
# on/on top of/according to
لەسەر
# under
لەژێر
# between/among
ناو
# between/among
نێوان
# after
پاش
# before
پێش
# like
وەک
`)

func TokenMapConstructor(config map[string]interface{}, cache *registry.Cache) (analysis.TokenMap, error) {
	rv := analysis.NewTokenMap()
	err := rv.LoadBytes(SoraniStopWords)
	return rv, err
}

func init() {
	registry.RegisterTokenMap(StopName, TokenMapConstructor)
}
