// Copyright 2011 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package index

import (
	"regexp/syntax"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// A Query is a matching machine, like a regular expression,
// that matches some text and not other text.  When we compute a
// Query from a regexp, the Query is a conservative version of the
// regexp: it matches everything the regexp would match, and probably
// quite a bit more.  We can then filter target files by whether they match
// the Query (using a trigram index) before running the comparatively
// more expensive regexp machinery.
type Query struct {
	Op      QueryOp
	Trigram []string
	Sub     []*Query
}

type QueryOp int

const (
	QAll  QueryOp = iota // Everything matches
	QNone                // Nothing matches
	QAnd                 // All in Sub and Trigram must match
	QOr                  // At least one in Sub or Trigram must match
)

var allQuery = &Query{Op: QAll}
var noneQuery = &Query{Op: QNone}

// and returns the query q AND r, possibly reusing q's and r's storage.
func (q *Query) and(r *Query) *Query {
	return q.andOr(r, QAnd)
}

// or returns the query q OR r, possibly reusing q's and r's storage.
func (q *Query) or(r *Query) *Query {
	return q.andOr(r, QOr)
}

// andOr returns the query q AND r or q OR r, possibly reusing q's and r's storage.
// It works hard to avoid creating unnecessarily complicated structures.
func (q *Query) andOr(r *Query, op QueryOp) (out *Query) {
	opstr := "&"
	if op == QOr {
		opstr = "|"
	}
	//println("andOr", q.String(), opstr, r.String())
	//defer func() { println("  ->", out.String()) }()
	_ = opstr

	if len(q.Trigram) == 0 && len(q.Sub) == 1 {
		q = q.Sub[0]
	}
	if len(r.Trigram) == 0 && len(r.Sub) == 1 {
		r = r.Sub[0]
	}

	// Boolean simplification.
	// If q ⇒ r, q AND r ≡ q.
	// If q ⇒ r, q OR r ≡ r.
	if q.implies(r) {
		//println(q.String(), "implies", r.String())
		if op == QAnd {
			return q
		}
		return r
	}
	if r.implies(q) {
		//println(r.String(), "implies", q.String())
		if op == QAnd {
			return r
		}
		return q
	}

	// Both q and r are QAnd or QOr.
	// If they match or can be made to match, merge.
	qAtom := len(q.Trigram) == 1 && len(q.Sub) == 0
	rAtom := len(r.Trigram) == 1 && len(r.Sub) == 0
	if q.Op == op && (r.Op == op || rAtom) {
		q.Trigram = stringSet.union(q.Trigram, r.Trigram, false)
		q.Sub = append(q.Sub, r.Sub...)
		return q
	}
	if r.Op == op && qAtom {
		r.Trigram = stringSet.union(r.Trigram, q.Trigram, false)
		return r
	}
	if qAtom && rAtom {
		q.Op = op
		q.Trigram = append(q.Trigram, r.Trigram...)
		return q
	}

	// If one matches the op, add the other to it.
	if q.Op == op {
		q.Sub = append(q.Sub, r)
		return q
	}
	if r.Op == op {
		r.Sub = append(r.Sub, q)
		return r
	}

	// We are creating an AND of ORs or an OR of ANDs.
	// Factor out common trigrams, if any.
	common := stringSet{}
	i, j := 0, 0
	wi, wj := 0, 0
	for i < len(q.Trigram) && j < len(r.Trigram) {
		qt, rt := q.Trigram[i], r.Trigram[j]
		if qt < rt {
			q.Trigram[wi] = qt
			wi++
			i++
		} else if qt > rt {
			r.Trigram[wj] = rt
			wj++
			j++
		} else {
			common = append(common, qt)
			i++
			j++
		}
	}
	for ; i < len(q.Trigram); i++ {
		q.Trigram[wi] = q.Trigram[i]
		wi++
	}
	for ; j < len(r.Trigram); j++ {
		r.Trigram[wj] = r.Trigram[j]
		wj++
	}
	q.Trigram = q.Trigram[:wi]
	r.Trigram = r.Trigram[:wj]
	if len(common) > 0 {
		// If there were common trigrams, rewrite
		//
		//	(abc|def|ghi|jkl) AND (abc|def|mno|prs) =>
		//		(abc|def) OR ((ghi|jkl) AND (mno|prs))
		//
		//	(abc&def&ghi&jkl) OR (abc&def&mno&prs) =>
		//		(abc&def) AND ((ghi&jkl) OR (mno&prs))
		//
		// Build up the right one of
		//	(ghi|jkl) AND (mno|prs)
		//	(ghi&jkl) OR (mno&prs)
		// Call andOr recursively in case q and r can now be simplified
		// (we removed some trigrams).
		s := q.andOr(r, op)

		// Add in factored trigrams.
		otherOp := QAnd + QOr - op
		t := &Query{Op: otherOp, Trigram: common}
		return t.andOr(s, t.Op)
	}

	// Otherwise just create the op.
	return &Query{Op: op, Sub: []*Query{q, r}}
}

// implies reports whether q implies r.
// It is okay for it to return false negatives.
func (q *Query) implies(r *Query) bool {
	if q.Op == QNone || r.Op == QAll {
		// False implies everything.
		// Everything implies True.
		return true
	}
	if q.Op == QAll || r.Op == QNone {
		// True implies nothing.
		// Nothing implies False.
		return false
	}

	if q.Op == QAnd || (q.Op == QOr && len(q.Trigram) == 1 && len(q.Sub) == 0) {
		return trigramsImply(q.Trigram, r)
	}

	if q.Op == QOr && r.Op == QOr &&
		len(q.Trigram) > 0 && len(q.Sub) == 0 &&
		stringSet.isSubsetOf(q.Trigram, r.Trigram) {
		return true
	}
	return false
}

func trigramsImply(t []string, q *Query) bool {
	switch q.Op {
	case QOr:
		for _, qq := range q.Sub {
			if trigramsImply(t, qq) {
				return true
			}
		}
		for i := range t {
			if stringSet.isSubsetOf(t[i:i+1], q.Trigram) {
				return true
			}
		}
		return false
	case QAnd:
		for _, qq := range q.Sub {
			if !trigramsImply(t, qq) {
				return false
			}
		}
		if !stringSet.isSubsetOf(q.Trigram, t) {
			return false
		}
		return true
	}
	return false
}

// maybeRewrite rewrites q to use op if it is possible to do so
// without changing the meaning.  It also simplifies if the node
// is an empty OR or AND.
func (q *Query) maybeRewrite(op QueryOp) {
	if q.Op != QAnd && q.Op != QOr {
		return
	}

	// AND/OR doing real work?  Can't rewrite.
	n := len(q.Sub) + len(q.Trigram)
	if n > 1 {
		return
	}

	// Nothing left in the AND/OR?
	if n == 0 {
		if q.Op == QAnd {
			q.Op = QAll
		} else {
			q.Op = QNone
		}
		return
	}

	// Just a sub-node: throw away wrapper.
	if len(q.Sub) == 1 {
		*q = *q.Sub[0]
	}

	// Just a trigram: can use either op.
	q.Op = op
}

// andTrigrams returns q AND the OR of the AND of the trigrams present in each string.
func (q *Query) andTrigrams(t stringSet) *Query {
	if t.minLen() < 3 {
		// If there is a short string, we can't guarantee
		// that any trigrams must be present, so use ALL.
		// q AND ALL = q.
		return q
	}

	//println("andtrigrams", strings.Join(t, ","))
	or := noneQuery
	for _, tt := range t {
		var trig stringSet
		for i := 0; i+3 <= len(tt); i++ {
			trig.add(tt[i : i+3])
		}
		trig.clean(false)
		//println(tt, "trig", strings.Join(trig, ","))
		or = or.or(&Query{Op: QAnd, Trigram: trig})
	}
	q = q.and(or)
	return q
}

func (q *Query) String() string {
	if q == nil {
		return "?"
	}
	if q.Op == QNone {
		return "-"
	}
	if q.Op == QAll {
		return "+"
	}

	if len(q.Sub) == 0 && len(q.Trigram) == 1 {
		return strconv.Quote(q.Trigram[0])
	}

	var (
		s     string
		sjoin string
		end   string
		tjoin string
	)
	if q.Op == QAnd {
		sjoin = " "
		tjoin = " "
	} else {
		s = "("
		sjoin = ")|("
		end = ")"
		tjoin = "|"
	}
	for i, t := range q.Trigram {
		if i > 0 {
			s += tjoin
		}
		s += strconv.Quote(t)
	}
	if len(q.Sub) > 0 {
		if len(q.Trigram) > 0 {
			s += sjoin
		}
		s += q.Sub[0].String()
		for i := 1; i < len(q.Sub); i++ {
			s += sjoin + q.Sub[i].String()
		}
	}
	s += end
	return s
}

// RegexpQuery returns a Query for the given regexp.
func RegexpQuery(re *syntax.Regexp) *Query {
	info := analyze(re)
	info.simplify(true)
	info.addExact()
	return info.match
}

// A regexpInfo summarizes the results of analyzing a regexp.
type regexpInfo struct {
	// canEmpty records whether the regexp matches the empty string
	canEmpty bool

	// exact is the exact set of strings matching the regexp.
	exact stringSet

	// if exact is nil, prefix is the set of possible match prefixes,
	// and suffix is the set of possible match suffixes.
	prefix stringSet // otherwise: the exact set of matching prefixes ...
	suffix stringSet // ... and suffixes

	// match records a query that must be satisfied by any
	// match for the regexp, in addition to the information
	// recorded above.
	match *Query
}

const (
	// Exact sets are limited to maxExact strings.
	// If they get too big, simplify will rewrite the regexpInfo
	// to use prefix and suffix instead.  It's not worthwhile for
	// this to be bigger than maxSet.
	// Because we allow the maximum length of an exact string
	// to grow to 5 below (see simplify), it helps to avoid ridiculous
	// alternations if maxExact is sized so that 3 case-insensitive letters
	// triggers a flush.
	maxExact = 7

	// Prefix and suffix sets are limited to maxSet strings.
	// If they get too big, simplify will replace groups of strings
	// sharing a common leading prefix (or trailing suffix) with
	// that common prefix (or suffix).  It is useful for maxSet
	// to be at least 2³ = 8 so that we can exactly
	// represent a case-insensitive abc by the set
	// {abc, abC, aBc, aBC, Abc, AbC, ABc, ABC}.
	maxSet = 20
)

// anyMatch returns the regexpInfo describing a regexp that
// matches any string.
func anyMatch() regexpInfo {
	return regexpInfo{
		canEmpty: true,
		prefix:   []string{""},
		suffix:   []string{""},
		match:    allQuery,
	}
}

// anyChar returns the regexpInfo describing a regexp that
// matches any single character.
func anyChar() regexpInfo {
	return regexpInfo{
		prefix: []string{""},
		suffix: []string{""},
		match:  allQuery,
	}
}

// noMatch returns the regexpInfo describing a regexp that
// matches no strings at all.
func noMatch() regexpInfo {
	return regexpInfo{
		match: noneQuery,
	}
}

// emptyString returns the regexpInfo describing a regexp that
// matches only the empty string.
func emptyString() regexpInfo {
	return regexpInfo{
		canEmpty: true,
		exact:    []string{""},
		match:    allQuery,
	}
}

// analyze returns the regexpInfo for the regexp re.
func analyze(re *syntax.Regexp) (ret regexpInfo) {
	//println("analyze", re.String())
	//defer func() { println("->", ret.String()) }()
	var info regexpInfo
	switch re.Op {
	case syntax.OpNoMatch:
		return noMatch()

	case syntax.OpEmptyMatch,
		syntax.OpBeginLine, syntax.OpEndLine,
		syntax.OpBeginText, syntax.OpEndText,
		syntax.OpWordBoundary, syntax.OpNoWordBoundary:
		return emptyString()

	case syntax.OpLiteral:
		if re.Flags&syntax.FoldCase != 0 {
			switch len(re.Rune) {
			case 0:
				return emptyString()
			case 1:
				// Single-letter case-folded string:
				// rewrite into char class and analyze.
				re1 := &syntax.Regexp{
					Op: syntax.OpCharClass,
				}
				re1.Rune = re1.Rune0[:0]
				r0 := re.Rune[0]
				re1.Rune = append(re1.Rune, r0, r0)
				for r1 := unicode.SimpleFold(r0); r1 != r0; r1 = unicode.SimpleFold(r1) {
					re1.Rune = append(re1.Rune, r1, r1)
				}
				info = analyze(re1)
				return info
			}
			// Multi-letter case-folded string:
			// treat as concatenation of single-letter case-folded strings.
			re1 := &syntax.Regexp{
				Op:    syntax.OpLiteral,
				Flags: syntax.FoldCase,
			}
			info = emptyString()
			for i := range re.Rune {
				re1.Rune = re.Rune[i : i+1]
				info = concat(info, analyze(re1))
			}
			return info
		}
		info.exact = stringSet{string(re.Rune)}
		info.match = allQuery

	case syntax.OpAnyCharNotNL, syntax.OpAnyChar:
		return anyChar()

	case syntax.OpCapture:
		return analyze(re.Sub[0])

	case syntax.OpConcat:
		return fold(concat, re.Sub, emptyString())

	case syntax.OpAlternate:
		return fold(alternate, re.Sub, noMatch())

	case syntax.OpQuest:
		return alternate(analyze(re.Sub[0]), emptyString())

	case syntax.OpStar:
		// We don't know anything, so assume the worst.
		return anyMatch()

	case syntax.OpRepeat:
		if re.Min == 0 {
			// Like OpStar
			return anyMatch()
		}
		fallthrough
	case syntax.OpPlus:
		// x+
		// Since there has to be at least one x, the prefixes and suffixes
		// stay the same.  If x was exact, it isn't anymore.
		info = analyze(re.Sub[0])
		if info.exact.have() {
			info.prefix = info.exact
			info.suffix = info.exact.copy()
			info.exact = nil
		}

	case syntax.OpCharClass:
		info.match = allQuery

		// Special case.
		if len(re.Rune) == 0 {
			return noMatch()
		}

		// Special case.
		if len(re.Rune) == 1 {
			info.exact = stringSet{string(re.Rune[0])}
			break
		}

		n := 0
		for i := 0; i < len(re.Rune); i += 2 {
			n += int(re.Rune[i+1] - re.Rune[i])
		}
		// If the class is too large, it's okay to overestimate.
		if n > 100 {
			return anyChar()
		}

		info.exact = []string{}
		for i := 0; i < len(re.Rune); i += 2 {
			lo, hi := re.Rune[i], re.Rune[i+1]
			for rr := lo; rr <= hi; rr++ {
				info.exact.add(string(rr))
			}
		}
	}

	info.simplify(false)
	return info
}

// fold is the usual higher-order function.
func fold(f func(x, y regexpInfo) regexpInfo, sub []*syntax.Regexp, zero regexpInfo) regexpInfo {
	if len(sub) == 0 {
		return zero
	}
	if len(sub) == 1 {
		return analyze(sub[0])
	}
	info := f(analyze(sub[0]), analyze(sub[1]))
	for i := 2; i < len(sub); i++ {
		info = f(info, analyze(sub[i]))
	}
	return info
}

// concat returns the regexp info for xy given x and y.
func concat(x, y regexpInfo) (out regexpInfo) {
	//println("concat", x.String(), "...", y.String())
	//defer func() { println("->", out.String()) }()
	var xy regexpInfo
	xy.match = x.match.and(y.match)
	if x.exact.have() && y.exact.have() {
		xy.exact = x.exact.cross(y.exact, false)
	} else {
		if x.exact.have() {
			xy.prefix = x.exact.cross(y.prefix, false)
		} else {
			xy.prefix = x.prefix
			if x.canEmpty {
				xy.prefix = xy.prefix.union(y.prefix, false)
			}
		}
		if y.exact.have() {
			xy.suffix = x.suffix.cross(y.exact, true)
		} else {
			xy.suffix = y.suffix
			if y.canEmpty {
				xy.suffix = xy.suffix.union(x.suffix, true)
			}
		}
	}

	// If all the possible strings in the cross product of x.suffix
	// and y.prefix are long enough, then the trigram for one
	// of them must be present and would not necessarily be
	// accounted for in xy.prefix or xy.suffix yet.  Cut things off
	// at maxSet just to keep the sets manageable.
	if !x.exact.have() && !y.exact.have() &&
		x.suffix.size() <= maxSet && y.prefix.size() <= maxSet &&
		x.suffix.minLen()+y.prefix.minLen() >= 3 {
		xy.match = xy.match.andTrigrams(x.suffix.cross(y.prefix, false))
	}

	xy.simplify(false)
	return xy
}

// alternate returns the regexpInfo for x|y given x and y.
func alternate(x, y regexpInfo) (out regexpInfo) {
	//println("alternate", x.String(), "...", y.String())
	//defer func() { println("->", out.String()) }()
	var xy regexpInfo
	if x.exact.have() && y.exact.have() {
		xy.exact = x.exact.union(y.exact, false)
	} else if x.exact.have() {
		xy.prefix = x.exact.union(y.prefix, false)
		xy.suffix = x.exact.union(y.suffix, true)
		x.addExact()
	} else if y.exact.have() {
		xy.prefix = x.prefix.union(y.exact, false)
		xy.suffix = x.suffix.union(y.exact.copy(), true)
		y.addExact()
	} else {
		xy.prefix = x.prefix.union(y.prefix, false)
		xy.suffix = x.suffix.union(y.suffix, true)
	}
	xy.canEmpty = x.canEmpty || y.canEmpty
	xy.match = x.match.or(y.match)

	xy.simplify(false)
	return xy
}

// addExact adds to the match query the trigrams for matching info.exact.
func (info *regexpInfo) addExact() {
	if info.exact.have() {
		info.match = info.match.andTrigrams(info.exact)
	}
}

// simplify simplifies the regexpInfo when the exact set gets too large.
func (info *regexpInfo) simplify(force bool) {
	//println("  simplify", info.String(), " force=", force)
	//defer func() { println("  ->", info.String()) }()
	// If there are now too many exact strings,
	// loop over them, adding trigrams and moving
	// the relevant pieces into prefix and suffix.
	info.exact.clean(false)
	if len(info.exact) > maxExact || (info.exact.minLen() >= 3 && force) || info.exact.minLen() >= 4 {
		info.addExact()
		for _, s := range info.exact {
			n := len(s)
			if n < 3 {
				info.prefix.add(s)
				info.suffix.add(s)
			} else {
				info.prefix.add(s[:2])
				info.suffix.add(s[n-2:])
			}
		}
		info.exact = nil
	}

	if !info.exact.have() {
		info.simplifySet(&info.prefix)
		info.simplifySet(&info.suffix)
	}
}

// simplifySet reduces the size of the given set (either prefix or suffix).
// There is no need to pass around enormous prefix or suffix sets, since
// they will only be used to create trigrams.  As they get too big, simplifySet
// moves the information they contain into the match query, which is
// more efficient to pass around.
func (info *regexpInfo) simplifySet(s *stringSet) {
	t := *s
	t.clean(s == &info.suffix)

	// Add the OR of the current prefix/suffix set to the query.
	info.match = info.match.andTrigrams(t)

	for n := 3; n == 3 || t.size() > maxSet; n-- {
		// Replace set by strings of length n-1.
		w := 0
		for _, str := range t {
			if len(str) >= n {
				if s == &info.prefix {
					str = str[:n-1]
				} else {
					str = str[len(str)-n+1:]
				}
			}
			if w == 0 || t[w-1] != str {
				t[w] = str
				w++
			}
		}
		t = t[:w]
		t.clean(s == &info.suffix)
	}

	// Now make sure that the prefix/suffix sets aren't redundant.
	// For example, if we know "ab" is a possible prefix, then it
	// doesn't help at all to know that  "abc" is also a possible
	// prefix, so delete "abc".
	w := 0
	f := strings.HasPrefix
	if s == &info.suffix {
		f = strings.HasSuffix
	}
	for _, str := range t {
		if w == 0 || !f(str, t[w-1]) {
			t[w] = str
			w++
		}
	}
	t = t[:w]

	*s = t
}

func (info regexpInfo) String() string {
	s := ""
	if info.canEmpty {
		s += "canempty "
	}
	if info.exact.have() {
		s += "exact:" + strings.Join(info.exact, ",")
	} else {
		s += "prefix:" + strings.Join(info.prefix, ",")
		s += " suffix:" + strings.Join(info.suffix, ",")
	}
	s += " match: " + info.match.String()
	return s
}

// A stringSet is a set of strings.
// The nil stringSet indicates not having a set.
// The non-nil but empty stringSet is the empty set.
type stringSet []string

// have reports whether we have a stringSet.
func (s stringSet) have() bool {
	return s != nil
}

// contains reports whether s contains str.
func (s stringSet) contains(str string) bool {
	for _, ss := range s {
		if ss == str {
			return true
		}
	}
	return false
}

type byPrefix []string

func (x *byPrefix) Len() int           { return len(*x) }
func (x *byPrefix) Swap(i, j int)      { (*x)[i], (*x)[j] = (*x)[j], (*x)[i] }
func (x *byPrefix) Less(i, j int) bool { return (*x)[i] < (*x)[j] }

type bySuffix []string

func (x *bySuffix) Len() int      { return len(*x) }
func (x *bySuffix) Swap(i, j int) { (*x)[i], (*x)[j] = (*x)[j], (*x)[i] }
func (x *bySuffix) Less(i, j int) bool {
	s := (*x)[i]
	t := (*x)[j]
	for i := 1; i <= len(s) && i <= len(t); i++ {
		si := s[len(s)-i]
		ti := t[len(t)-i]
		if si < ti {
			return true
		}
		if si > ti {
			return false
		}
	}
	return len(s) < len(t)
}

// add adds str to the set.
func (s *stringSet) add(str string) {
	*s = append(*s, str)
}

// clean removes duplicates from the stringSet.
func (s *stringSet) clean(isSuffix bool) {
	t := *s
	if isSuffix {
		sort.Sort((*bySuffix)(s))
	} else {
		sort.Sort((*byPrefix)(s))
	}
	w := 0
	for _, str := range t {
		if w == 0 || t[w-1] != str {
			t[w] = str
			w++
		}
	}
	*s = t[:w]
}

// size returns the number of strings in s.
func (s stringSet) size() int {
	return len(s)
}

// minLen returns the length of the shortest string in s.
func (s stringSet) minLen() int {
	if len(s) == 0 {
		return 0
	}
	m := len(s[0])
	for _, str := range s {
		if m > len(str) {
			m = len(str)
		}
	}
	return m
}

// maxLen returns the length of the longest string in s.
func (s stringSet) maxLen() int {
	if len(s) == 0 {
		return 0
	}
	m := len(s[0])
	for _, str := range s {
		if m < len(str) {
			m = len(str)
		}
	}
	return m
}

// union returns the union of s and t, reusing s's storage.
func (s stringSet) union(t stringSet, isSuffix bool) stringSet {
	s = append(s, t...)
	s.clean(isSuffix)
	return s
}

// cross returns the cross product of s and t.
func (s stringSet) cross(t stringSet, isSuffix bool) stringSet {
	p := stringSet{}
	for _, ss := range s {
		for _, tt := range t {
			p.add(ss + tt)
		}
	}
	p.clean(isSuffix)
	return p
}

// clear empties the set but preserves the storage.
func (s *stringSet) clear() {
	*s = (*s)[:0]
}

// copy returns a copy of the set that does not share storage with the original.
func (s stringSet) copy() stringSet {
	return append(stringSet{}, s...)
}

// isSubsetOf returns true if all strings in s are also in t.
// It assumes both sets are sorted.
func (s stringSet) isSubsetOf(t stringSet) bool {
	j := 0
	for _, ss := range s {
		for j < len(t) && t[j] < ss {
			j++
		}
		if j >= len(t) || t[j] != ss {
			return false
		}
	}
	return true
}
