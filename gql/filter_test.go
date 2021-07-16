/*
 * Copyright 2015-2018 Dgraph Labs, Inc. and Contributors
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

package gql

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// This file tests filters parsing.
func TestParseStringVarInFilter(t *testing.T) {
	query := `
		query versions($version: string = "v0.7.3/beta")
		{
			versions(func:eq(type, "version"))
			{
				versions @filter(eq(version_number, $version))
				{
					version_number
				}
			}
		}
	`
	res, err := Parse(Request{Str: query})
	require.NoError(t, err)
	require.NotNil(t, res.Query[0])
	require.Equal(t, "v0.7.3/beta", res.Query[0].Children[0].Filter.Func.Args[0].Value)
}

func TestParseFilter_root(t *testing.T) {
	query := `
	query {
		me(func:anyofterms(abc, "Abc")) @filter(allofterms(name, "alice")) {
			friends @filter() {
				name @filter(namefilter(name, "a"))
			}
			gender @filter(eq(g, "a")),age @filter(neq(a, "b"))
			hometown
		}
	}
`
	res, err := Parse(Request{Str: query})
	require.NoError(t, err)
	require.NotNil(t, res.Query[0])
	require.NotNil(t, res.Query[0].Filter)
	require.Equal(t, `(allofterms name "alice")`, res.Query[0].Filter.debugString())
	require.Equal(t, []string{"friends", "gender", "age", "hometown"}, childAttrs(res.Query[0]))
	require.Equal(t, []string{"name"}, childAttrs(res.Query[0].Children[0]))
	require.Nil(t, res.Query[0].Children[0].Filter)
	require.Equal(t, `(eq g "a")`, res.Query[0].Children[1].Filter.debugString())
	require.Equal(t, `(neq a "b")`, res.Query[0].Children[2].Filter.debugString())
	require.Equal(t, `(namefilter name "a")`, res.Query[0].Children[0].Children[0].Filter.debugString())
}

func TestParseFilter_root2(t *testing.T) {
	query := `
	query {
		me(func:anyofterms(abc, "Abc")) @filter(gt(count(friends), 10)) {
			friends @filter() {
				name
			}
			hometown
		}
	}
`
	res, err := Parse(Request{Str: query})
	require.NoError(t, err)
	require.NotNil(t, res.Query[0])
	require.NotNil(t, res.Query[0].Filter)
	require.Equal(t, `(gt count(friends) "10")`, res.Query[0].Filter.debugString())
	require.Equal(t, []string{"friends", "hometown"}, childAttrs(res.Query[0]))
	require.Equal(t, []string{"name"}, childAttrs(res.Query[0].Children[0]))
	require.Nil(t, res.Query[0].Children[0].Filter)
}

func TestParseFilter_root_Error2(t *testing.T) {
	// filter-by-count only support first argument as function
	query := `
	query {
		me(func:anyofterms(abc, "Abc")) @filter(gt(count(friends), sum(friends))) {
			friends @filter() {
				name
			}
			hometown
		}
	}
`
	_, err := Parse(Request{Str: query})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Multiple functions as arguments not allowed")
}

func TestParseFilter_simplest(t *testing.T) {
	query := `
	query {
		me(func: uid(0x0a)) {
			friends @filter() {
				name @filter(namefilter(name, "a"))
			}
			gender @filter(eq(g, "a")),age @filter(neq(a, "b"))
			hometown
		}
	}
`
	res, err := Parse(Request{Str: query})
	require.NoError(t, err)
	require.NotNil(t, res.Query[0])
	require.Equal(t, []string{"friends", "gender", "age", "hometown"}, childAttrs(res.Query[0]))
	require.Equal(t, []string{"name"}, childAttrs(res.Query[0].Children[0]))
	require.Nil(t, res.Query[0].Children[0].Filter)
	require.Equal(t, `(eq g "a")`, res.Query[0].Children[1].Filter.debugString())
	require.Equal(t, `(neq a "b")`, res.Query[0].Children[2].Filter.debugString())
	require.Equal(t, `(namefilter name "a")`, res.Query[0].Children[0].Children[0].Filter.debugString())
}

// Test operator precedence. and should be evaluated before or.
func TestParseFilter_op(t *testing.T) {
	query := `
	query {
		me(func: uid(0x0a)) {
			friends @filter(a(aa, "aaa") or b(bb, "bbb")
			and c(cc, "ccc")) {
				name
			}
			gender,age
			hometown
		}
	}
`
	res, err := Parse(Request{Str: query})
	require.NoError(t, err)
	require.NotNil(t, res.Query[0])
	require.Equal(t, []string{"friends", "gender", "age", "hometown"}, childAttrs(res.Query[0]))
	require.Equal(t, []string{"name"}, childAttrs(res.Query[0].Children[0]))
	require.Equal(t, `(OR (a aa "aaa") (AND (b bb "bbb") (c cc "ccc")))`, res.Query[0].Children[0].Filter.debugString())
}

func TestParseFilter_opError1(t *testing.T) {
	query := `
	query {
		me(func: uid(0x0a)) {
			friends @filter(a(aa "aaa") or b(b "bbb")) {
				name
			}
			gender,age
			hometown
		}
	}
`
	_, err := Parse(Request{Str: query})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Expected comma or language but got: \"aaa\"")
}

func TestParseFilter_opNoError2(t *testing.T) {
	query := `
	query {
		me(func: uid(0x0a)) {
			friends @filter(a(aa, "aaa") or b(b, "bbb")) {
				name
			}
			gender,age
			hometown
		}
	}
`
	_, err := Parse(Request{Str: query})
	require.NoError(t, err)
	// It's valid.  Makes sure TestParseFilter_opError3 fails for the expected reason.
}

func TestParseFilter_opError3(t *testing.T) {
	query := `
		query {
		me(func: uid(0x0a)) {
			friends @filter(a(aa, "aaa") or b(b, "bbb") and) {
				name
			}
			gender,age
			hometown
		}
	}
`
	_, err := Parse(Request{Str: query})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Invalid filter statement")
}

func TestParseFilter_opNot1(t *testing.T) {
	query := `
	query {
		me(func: uid(0x0a)) {
			friends @filter(not a(aa, "aaa")) {
				name
			}
			gender,age
			hometown
		}
	}
`
	res, err := Parse(Request{Str: query})
	require.NoError(t, err)
	require.NotNil(t, res.Query[0])
	require.Equal(t, []string{"friends", "gender", "age", "hometown"}, childAttrs(res.Query[0]))
	require.Equal(t, []string{"name"}, childAttrs(res.Query[0].Children[0]))
	require.Equal(t, `(NOT (a aa "aaa"))`, res.Query[0].Children[0].Filter.debugString())
}

func TestParseFilter_opNot2(t *testing.T) {
	query := `
	query {
		me(func: uid(0x0a)) {
			friends @filter(not(a(aa, "aaa") or (b(bb, "bbb"))) and c(cc, "ccc")) {
				name
			}
			gender,age
			hometown
		}
	}
`
	res, err := Parse(Request{Str: query})
	require.NoError(t, err)
	require.NotNil(t, res.Query[0])
	require.Equal(t, []string{"friends", "gender", "age", "hometown"}, childAttrs(res.Query[0]))
	require.Equal(t, []string{"name"}, childAttrs(res.Query[0].Children[0]))
	require.Equal(t, `(AND (NOT (OR (a aa "aaa") (b bb "bbb"))) (c cc "ccc"))`, res.Query[0].Children[0].Filter.debugString())
}

// Test operator precedence. Let brackets make or evaluates before and.
func TestParseFilter_op2(t *testing.T) {
	query := `
	query {
		me(func: uid(0x0a)) {
			friends @filter((a(aa, "aaa") Or b(bb, "bbb"))
			 and c(cc, "ccc")) {
				name
			}
			gender,age
			hometown
		}
	}
`
	res, err := Parse(Request{Str: query})
	require.NoError(t, err)
	require.NotNil(t, res.Query[0])
	require.Equal(t, []string{"friends", "gender", "age", "hometown"}, childAttrs(res.Query[0]))
	require.Equal(t, []string{"name"}, childAttrs(res.Query[0].Children[0]))
	require.Equal(t, `(AND (OR (a aa "aaa") (b bb "bbb")) (c cc "ccc"))`, res.Query[0].Children[0].Filter.debugString())
}

// Test operator precedence. More elaborate brackets.
func TestParseFilter_brac(t *testing.T) {
	query := `
	query {
		me(func: uid(0x0a)) {
			friends @filter(  a(name, "hello") or b(name, "world", "is") and (c(aa, "aaa") or (d(dd, "haha") or e(ee, "aaa"))) and f(ff, "aaa")){
				name
			}
			gender,age
			hometown
		}
	}
`
	res, err := Parse(Request{Str: query})
	require.NoError(t, err)
	require.NotNil(t, res.Query[0])
	require.Equal(t, []string{"friends", "gender", "age", "hometown"}, childAttrs(res.Query[0]))
	require.Equal(t, []string{"name"}, childAttrs(res.Query[0].Children[0]))
	require.Equal(t,
		`(OR (a name "hello") (AND (AND (b name "world" "is") (OR (c aa "aaa") (OR (d dd "haha") (e ee "aaa")))) (f ff "aaa")))`,
		res.Query[0].Children[0].Filter.debugString())
}

// Test if unbalanced brac will lead to errors.
func TestParseFilter_unbalancedbrac(t *testing.T) {
	query := `
	query {
		me(func: uid(0x0a)) {
			friends @filter(  () {
				name
			}
			gender,age
			hometown
		}
	}
`
	_, err := Parse(Request{Str: query})
	require.Error(t, err)
	require.Contains(t, err.Error(),
		"Unrecognized character inside a func: U+007B '{'")
}

func TestParseFilter_Geo1(t *testing.T) {
	query := `
	query {
		me(func: uid(0x0a)) {
			friends @filter(near(loc, [-1.12 , 2.0123 ], 100.123 )) {
				name
			}
			gender,age
			hometown
		}
	}
`
	resp, err := Parse(Request{Str: query})
	require.NoError(t, err)
	require.Equal(t, "[-1.12,2.0123]", resp.Query[0].Children[0].Filter.Func.Args[0].Value)
	require.Equal(t, "100.123", resp.Query[0].Children[0].Filter.Func.Args[1].Value)
	require.Equal(t, false, resp.Query[0].Children[0].Filter.Func.Args[0].IsValueVar)
	require.Equal(t, false, resp.Query[0].Children[0].Filter.Func.Args[1].IsValueVar)
}

func TestParseFilter_Geo2(t *testing.T) {
	query := `
	query {
		me(func: uid(0x0a)) {
			friends @filter(within(loc, [[11.2 , -2.234 ], [ -31.23, 4.3214] , [5.312, 6.53]] )) {
				name
			}
			gender,age
			hometown
		}
	}
`
	resp, err := Parse(Request{Str: query})
	require.NoError(t, err)
	require.Equal(t, "[[11.2,-2.234],[-31.23,4.3214],[5.312,6.53]]", resp.Query[0].Children[0].Filter.Func.Args[0].Value)
}

func TestParseFilter_Geo3(t *testing.T) {
	query := `
	query {
		me(func: uid(0x0a)) {
			friends @filter(near(loc, [[1 , 2 ], [[3, 4] , [5, 6]] )) {
				name
			}
			gender,age
			hometown
		}
	}
`
	_, err := Parse(Request{Str: query})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Found invalid item")
}

func TestParseFilter_Geo4(t *testing.T) {
	query := `
	query {
		me(func: uid(0x0a)) {
			friends @filter(near(loc, [[1 , 2 ], [3, 4] , [5, 6]]] )) {
				name
			}
			gender,age
			hometown
		}
	}
`
	_, err := Parse(Request{Str: query})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Expected right round or comma")
	require.Contains(t, err.Error(), "\"]\"")
}

// Test if empty brackets will lead to errors.
func TestParseFilter_emptyargument(t *testing.T) {
	query := `
	query {
		me(func: uid(0x0a)) {
			friends @filter(allofterms(name,,)) {
				name
			}
			gender,age
			hometown
		}
	}
`
	_, err := Parse(Request{Str: query})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Consecutive commas not allowed")

}

func TestParseFilter_unknowndirectiveError1(t *testing.T) {
	query := `
	query {
		me(func: uid(0x0a)) {
			friends @filtererr {
				name
			}
			gender,age
			hometown
		}
	}`
	_, err := Parse(Request{Str: query})
	require.Error(t, err)
	// We can't differentiate between @filtererr being a directive or a language. As we don't
	// see a () after it we assume its a language but attr which specify a language can't have
	// children.
	// The test below tests for unknown directive.
	require.Contains(t, err.Error(), "Cannot have children for attr: friends with lang tags:")
}

func TestParseFilter_unknowndirectiveError2(t *testing.T) {
	query := `
	query {
		me(func: uid(0x0a)) {
			friends @filtererr ()
			gender,age
			hometown
		}
	}`
	_, err := Parse(Request{Str: query})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Unknown directive [filtererr]")
}
func TestLangsFilter(t *testing.T) {
	query := `
	query {
		me(func: uid(0x0a)) {
			friends @filter(alloftext(descr@en, "something")) {
				name
			}
			gender,age
			hometown
		}
	}
`
	res, err := Parse(Request{Str: query})
	require.NoError(t, err)
	require.NotNil(t, res.Query[0])
	require.NotNil(t, res.Query[0].Children[0])
	require.NotNil(t, res.Query[0].Children[0].Filter)
	require.NotNil(t, res.Query[0].Children[0].Filter.Func)
	require.Equal(t, "descr", res.Query[0].Children[0].Filter.Func.Attr)
	require.Equal(t, "en", res.Query[0].Children[0].Filter.Func.Lang)
}

func TestLangsFilter_error1(t *testing.T) {
	// this query should fail, because '@lang' is used twice (and only one appearance is allowed)
	query := `
	query {
		me(func: uid(0x0a)) {
			friends @filter(alloftext(descr@en@de, "something")) {
				name
			}
			gender,age
			hometown
		}
	}
`
	_, err := Parse(Request{Str: query})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Invalid usage of '@' in function argument")
}

func TestLangsFilter_error2(t *testing.T) {
	// this query should fail, because there is no lang after '@'
	query := `
	query {
		me(func: uid(0x0a)) {
			friends @filter(alloftext(descr@, "something")) {
				name
			}
			gender,age
			hometown
		}
	}
`
	_, err := Parse(Request{Str: query})
	require.Error(t, err)
	require.Contains(t, err.Error(),
		"Unrecognized character in lexDirective: U+002C ','")
}

// Test facets parsing for filtering..
func TestFacetsFilterSimple(t *testing.T) {
	// all friends of 0x1 who are close to him
	query := `
	       {
		me(func: uid(0x1)) {
			name
			friend @facets(eq(close, true)) {
				name
				gender
			}
		}
	}
`

	res, err := Parse(Request{Str: query})
	require.NoError(t, err)
	require.NotNil(t, res.Query[0])
	require.Equal(t, []string{"name", "friend"}, childAttrs(res.Query[0]))
	require.Nil(t, res.Query[0].Children[1].Facets)
	require.NotNil(t, res.Query[0].Children[1].FacetsFilter)
	require.Equal(t, `(eq close "true")`,
		res.Query[0].Children[1].FacetsFilter.debugString())
}

func TestFacetsFilterAll(t *testing.T) {
	// all friends of 0x1 who are close to him or are in his family
	query := `
	       {
		me(func: uid(0x1)) {
			name
			friend @facets(eq(close, true) or eq(family, true)) @facets(close, family, since) {
				name @facets
				gender
			}
		}
	}
`

	res, err := Parse(Request{Str: query})
	require.NoError(t, err)
	require.NotNil(t, res.Query[0])
	require.Equal(t, []string{"name", "friend"}, childAttrs(res.Query[0]))
	require.NotNil(t, res.Query[0].Children[1].Facets)
	require.Equal(t, "close", res.Query[0].Children[1].Facets.Param[0].Key)
	require.Equal(t, "family", res.Query[0].Children[1].Facets.Param[1].Key)
	require.Equal(t, "since", res.Query[0].Children[1].Facets.Param[2].Key)
	require.NotNil(t, res.Query[0].Children[1].FacetsFilter)
	require.Equal(t, `(OR (eq close "true") (eq family "true"))`,
		res.Query[0].Children[1].FacetsFilter.debugString())

	require.Equal(t, []string{"name", "gender"}, childAttrs(res.Query[0].Children[1]))
	nameChild := res.Query[0].Children[1].Children[0]
	require.NotNil(t, nameChild)
	require.NotNil(t, nameChild.Facets)
	require.Nil(t, nameChild.FacetsFilter)
	genderChild := res.Query[0].Children[1].Children[1]
	require.NotNil(t, genderChild)
	require.Nil(t, genderChild.Facets)
	require.Nil(t, genderChild.FacetsFilter)
}

func TestFacetsFilterFail(t *testing.T) {
	// multiple @facets and @facets(close, since) are not allowed.
	query := `
	       {
		me(func: uid(0x1)) {
			name
			friend @facets @facets(close, since) {
				name
				gender
			}
		}
	}
`

	_, err := Parse(Request{Str: query})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Only one facets allowed")
}

func TestFacetsFilterFail2(t *testing.T) {
	// multiple facets-filter not allowed
	query := `
	       {
		me(func: uid(0x1)) {
			name
			friend @facets(eq(close, true)) @facets(eq(family, true)) {
				name
				gender
			}
		}
	}
`

	_, err := Parse(Request{Str: query})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Only one facets filter allowed")
}

func TestFacetsFilterFail3(t *testing.T) {
	// vars are not allowed in facets filtering.
	query := `
	{
		K as var(func: uid(0x0a)) {
			L AS friends
		}
		me(func: uid(K)) {
			friend @facets(uid(L)) {
				name
			}
		}
	}
`

	_, err := Parse(Request{Str: query})
	require.Error(t, err)
	require.Contains(t, err.Error(), "variables are not allowed in facets filter")
}

func TestFacetsFilterFailRoot(t *testing.T) {
	query := `
	{
		me(func: uid(0x1)) @facets(eq(some-facet, true)) {
			friend	{
				name
			}
		}
	}
`

	_, err := Parse(Request{Str: query})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Unknown directive [facets]")
}

func TestFacetsFilterAtValue(t *testing.T) {
	// gql parses facets at value level as well.
	query := `
	{
		me(func: uid(0x1)) {
			friend	{
				name @facets(eq(some.facet, true))
			}
		}
	}
`

	res, err := Parse(Request{Str: query})
	require.NoError(t, err)
	require.Equal(t, []string{"name"}, childAttrs(res.Query[0].Children[0]))
	nameChild := res.Query[0].Children[0].Children[0]
	require.NotNil(t, nameChild)
	require.NotNil(t, nameChild.FacetsFilter)
	require.Equal(t, `(eq some.facet "true")`, nameChild.FacetsFilter.debugString())
}

func TestHasFilterAtRoot(t *testing.T) {
	query := `{
		me(func: allofterms(name, "Steven Tom")) @filter(has(director.film)) {
			name
		}
	}`
	_, err := Parse(Request{Str: query})
	require.NoError(t, err)
}

func TestHasFilterAtChild(t *testing.T) {
	query := `{
		me(func: anyofterms(name, "Steven Tom")) {
			name
			director.film @filter(has(genre)) {
			}
		}
	}`
	_, err := Parse(Request{Str: query})
	require.NoError(t, err)
}

func TestFilterError(t *testing.T) {
	query := `
	{
		me(func: uid(1, 3 , 5, 7)) { @filter(uid(3, 7))
			name
		}
	}
	`
	_, err := Parse(Request{Str: query})
	require.Error(t, err)
}

func TestFilterError2(t *testing.T) {
	query := `
	{
		me(func: uid(1, 3 , 5, 7)) {
			name @filter(eq(name, 	"abc")) @filter(eq(name2, 	"abc"))
		}
	}
	`
	_, err := Parse(Request{Str: query})
	require.Error(t, err)
}

func TestDoubleGroupByError(t *testing.T) {
	query := `
	{
		me(func: uid(1, 3 , 5, 7)) {
			name @groupby(abc) @groupby(bcd)
		}
	}
	`
	_, err := Parse(Request{Str: query})
	require.Error(t, err)
}

func TestFilterError3(t *testing.T) {
	query := `
	{
		me(func: uid(1, 3 , 5, 7)) {
			expand(_all_) @filter(eq(name, "abc"))
		}
	}
	`
	_, err := Parse(Request{Str: query})
	require.Error(t, err)
}

func TestFilterUid(t *testing.T) {
	query := `
	{
		me(func: uid(1, 3 , 5, 7)) @filter(uid(3, 7)) {
			name
		}
	}
	`
	gql, err := Parse(Request{Str: query})
	require.NoError(t, err)
	require.Equal(t, []uint64{1, 3, 5, 7}, gql.Query[0].UID)
	require.Equal(t, []uint64{3, 7}, gql.Query[0].Filter.Func.UID)
}

func TestFilterVarErr(t *testing.T) {
	query := `
	{
		x as m(func: allofterms(name, "Pawan Rawal"))
	}
	{
		me(func: uid(1, 3 , 5, 7)) @filter(var(x)) {
			name
		}
	}
	`
	_, err := Parse(Request{Str: query})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Unexpected var()")
}

func TestParseLangTagAfterStringInFilter(t *testing.T) {
	// This is a fix for #1499.
	query := `
		{
			q(func: uid(0x01)) @filter(eq(name, "Hello"@en)) {
				uid
			}
		}
	`
	_, err := Parse(Request{Str: query})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Invalid usage of '@' in function argument")
}

func TestTypeInFilter(t *testing.T) {
	q := `
	query {
		me(func: uid(0x01)) @filter(type(Person)) {
			name
		}
	}`
	gq, err := Parse(Request{Str: q})
	require.NoError(t, err)
	require.Equal(t, 1, len(gq.Query))
	require.Equal(t, "uid", gq.Query[0].Func.Name)
	require.Equal(t, 1, len(gq.Query[0].Children))
	require.Equal(t, "name", gq.Query[0].Children[0].Attr)
	require.Equal(t, "type", gq.Query[0].Filter.Func.Name)
	require.Equal(t, 1, len(gq.Query[0].Filter.Func.Args))
	require.Equal(t, "Person", gq.Query[0].Filter.Func.Args[0].Value)
}

func TestTypeFilterInPredicate(t *testing.T) {
	q := `
	query {
		me(func: uid(0x01)) {
			friend @filter(type(Person)) {
				name
			}
		}
	}`
	gq, err := Parse(Request{Str: q})
	require.NoError(t, err)
	require.Equal(t, 1, len(gq.Query))
	require.Equal(t, "uid", gq.Query[0].Func.Name)
	require.Equal(t, 1, len(gq.Query[0].Children))
	require.Equal(t, "friend", gq.Query[0].Children[0].Attr)

	require.Equal(t, "type", gq.Query[0].Children[0].Filter.Func.Name)
	require.Equal(t, 1, len(gq.Query[0].Children[0].Filter.Func.Args))
	require.Equal(t, "Person", gq.Query[0].Children[0].Filter.Func.Args[0].Value)

	require.Equal(t, 1, len(gq.Query[0].Children[0].Children))
	require.Equal(t, "name", gq.Query[0].Children[0].Children[0].Attr)
}
func TestParseExpandFilter(t *testing.T) {
	query := `
		{
			q(func: eq(name, "Frodo")) {
				expand(_all_) @filter(type(Person)) {
					uid
				}
			}
		}`

	gq, err := Parse(Request{Str: query})
	require.NoError(t, err)
	require.Equal(t, 1, len(gq.Query))
	require.Equal(t, 1, len(gq.Query[0].Children))
	require.Equal(t, "type", gq.Query[0].Children[0].Filter.Func.Name)
	require.Equal(t, 1, len(gq.Query[0].Children[0].Filter.Func.Args))
	require.Equal(t, "Person", gq.Query[0].Children[0].Filter.Func.Args[0].Value)
}

func TestParseExpandFilterErr(t *testing.T) {
	query := `
		{
			q(func: eq(name, "Frodo")) {
				expand(_all_) @filter(has(Person)) {
					uid
				}
			}
		}`

	_, err := Parse(Request{Str: query})
	require.Error(t, err)
	require.Contains(t, err.Error(), "expand is only compatible with type filters")
}

func TestFilterWithDollar(t *testing.T) {
	query := `
	{
		q(func: eq(name, "Bob"), first:5) @filter(eq(description, "$yo")) {
		  name
		  description
		}
	  }
	`
	gq, err := Parse(Request{
		Str: query,
	})
	require.NoError(t, err)
	require.Equal(t, gq.Query[0].Filter.Func.Args[0].Value, "$yo")
}

func TestFilterWithDollarError(t *testing.T) {
	query := `
	{
		q(func: eq(name, "Bob"), first:5) @filter(eq(description, $yo)) {
		  name
		  description
		}
	  }
	`
	_, err := Parse(Request{
		Str: query,
	})

	require.Error(t, err)
}

func TestFilterWithVar(t *testing.T) {
	query := `query data($a: string = "dgraph")
	{
		data(func: eq(name, "Bob"), first:5) @filter(eq(description, $a)) {
			name
			description
		  }
	}`
	gq, err := Parse(Request{
		Str: query,
	})
	require.NoError(t, err)
	require.Equal(t, gq.Query[0].Filter.Func.Args[0].Value, "dgraph")
}

func TestFilterWithEmpty(t *testing.T) {
	query := `{
		names(func: has(name)) @filter(eq(name, "")) {
		  count(uid)
		}
	  }`
	gq, err := Parse(Request{
		Str: query,
	})
	require.NoError(t, err)
	require.Equal(t, gq.Query[0].Filter.Func.Args[0].Value, "")
}
