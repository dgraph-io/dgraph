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
	"bytes"
	"os"
	"runtime/debug"
	"testing"

	"github.com/dgraph-io/dgo/v210/protos/api"
	"github.com/dgraph-io/dgraph/chunker"
	"github.com/dgraph-io/dgraph/lex"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
)

func childAttrs(g *GraphQuery) []string {
	var out []string
	for _, c := range g.Children {
		out = append(out, c.Attr)
	}
	return out
}

func TestLenFunctionInsideUidError(t *testing.T) {
	query := `
	{
		var(func: uid(0x0a)) {
			fr as friends {
				a as age
			}
		}

		me(func: uid(fr)) @filter(uid(len(a), 10)) {
			name
		}
	}
`
	_, err := Parse(Request{Str: query})
	require.Error(t, err)
	require.Contains(t, err.Error(), "len function only allowed inside inequality")
}

func TestLenAsSecondArgumentError(t *testing.T) {
	query := `
	{
		var(func: uid(0x0a)) {
			fr as friends {
				a as age
			}
		}

		me(func: uid(fr)) @filter(10, len(fr)) {
			name
		}
	}
`
	_, err := Parse(Request{Str: query})
	// TODO(pawan) - Error message can be improved. We should validate function names from a
	// whitelist.
	require.Error(t, err)
}

func TestParseShortestPath(t *testing.T) {
	query := `
	{
		shortest(from:0x0a, to:0x0b, numpaths: 3, minweight: 3, maxweight: 6) {
			friends
			name
		}
	}
`
	res, err := Parse(Request{Str: query})
	require.NoError(t, err)
	require.NotNil(t, res.Query)
	require.Equal(t, 1, len(res.Query))
	require.Equal(t, uint64(0xa), res.Query[0].ShortestPathArgs.From.UID[0])
	require.Equal(t, uint64(0xb), res.Query[0].ShortestPathArgs.To.UID[0])
	require.Equal(t, "3", res.Query[0].Args["numpaths"])
	require.Equal(t, "3", res.Query[0].Args["minweight"])
	require.Equal(t, "6", res.Query[0].Args["maxweight"])
}

func TestParseShortestPathInvalidFnError(t *testing.T) {
	query := `{
		shortest(from: eq(a), to: uid(b)) {
			password
			friend
		}

	}`
	_, err := Parse(Request{Str: query})
	require.Error(t, err)
}

func TestParseMultipleQueries(t *testing.T) {
	query := `
	{
		you(func: uid(0x0a)) {
			name
		}

		me(func: uid(0x0b)) {
		 friends
		}
	}
`
	res, err := Parse(Request{Str: query})
	require.NoError(t, err)
	require.NotNil(t, res.Query)
	require.Equal(t, 2, len(res.Query))
}

func TestParseRootArgs1(t *testing.T) {
	query := `
	query {
		me(func: uid(0x0a), first: -4, offset: +1) {
			friends {
				name
			}
			gender,age
			hometown
		}
	}
`
	res, err := Parse(Request{Str: query})
	require.NoError(t, err)
	require.NotNil(t, res.Query)
	require.Equal(t, 1, len(res.Query))
	require.Equal(t, 2, len(res.Query[0].Args))
	require.Equal(t, "-4", res.Query[0].Args["first"])
	require.Equal(t, "+1", res.Query[0].Args["offset"])
	require.Equal(t, childAttrs(res.Query[0]), []string{"friends", "gender", "age", "hometown"})
	require.Equal(t, childAttrs(res.Query[0].Children[0]), []string{"name"})
}

func TestParseRootArgs2(t *testing.T) {
	query := `
	query {
		me(func: uid(0x0a), first: 1, offset:0) {
			friends {
				name
			}
			gender,age
			hometown
		}
	}
`
	res, err := Parse(Request{Str: query})
	require.NoError(t, err)
	require.NotNil(t, res.Query)
	require.Equal(t, 1, len(res.Query))
	require.Equal(t, 2, len(res.Query[0].Args))
	require.Equal(t, "1", res.Query[0].Args["first"])
	require.Equal(t, "0", res.Query[0].Args["offset"])
	require.Equal(t, childAttrs(res.Query[0]), []string{"friends", "gender", "age", "hometown"})
	require.Equal(t, childAttrs(res.Query[0].Children[0]), []string{"name"})
}

func TestParse(t *testing.T) {
	query := `
	query {
		me(func: uid(0x0a)) {
			friends {
				name
			}
			gender,age
			hometown
		}
	}
`
	res, err := Parse(Request{Str: query})
	require.NoError(t, err)
	require.NotNil(t, res.Query)
	require.Equal(t, 1, len(res.Query))
	require.Equal(t, childAttrs(res.Query[0]), []string{"friends", "gender", "age", "hometown"})
	require.Equal(t, childAttrs(res.Query[0].Children[0]), []string{"name"})
}

func TestParseError(t *testing.T) {
	query := `
		me(func: uid(0x0a)) {
			friends {
				name
			}
			gender,age
			hometown
		}
	}
`
	_, err := Parse(Request{Str: query})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Invalid operation type: me")
}

func TestParseXid(t *testing.T) {
	query := `
	query {
		user(func: uid( 0x11)) {
			type.object.name
		}
	}`
	res, err := Parse(Request{Str: query})
	require.NoError(t, err)
	require.NotNil(t, res.Query[0])
	require.Equal(t, childAttrs(res.Query[0]), []string{"type.object.name"})
}

func TestParseIdList(t *testing.T) {
	query := `
	query {
		user(func: uid(0x1)) {
			type.object.name
		}
	}`
	r, err := Parse(Request{Str: query})
	gq := r.Query[0]
	require.NoError(t, err)
	require.NotNil(t, gq)
	require.Equal(t, []string{"type.object.name"}, childAttrs(gq))
	//	require.Equal(t, []uint64{0x1}, gq.UID)
}

func TestParseIdList1(t *testing.T) {
	query := `
	query {
		user(func: uid(0x1, 0x34)) {
			type.object.name
		}
	}`
	r, err := Parse(Request{Str: query})
	gq := r.Query[0]
	require.NoError(t, err)
	require.NotNil(t, gq)
	require.Equal(t, []string{"type.object.name"}, childAttrs(gq))
	require.Equal(t, []uint64{0x1, 0x34}, gq.UID)
	require.Equal(t, 2, len(gq.UID))
}

func TestParseIdListError(t *testing.T) {
	query := `
	query {
		user(func: uid( [0x1, 0x1, abc, ade, 0x34))] {
			type.object.name
		}
	}`
	_, err := Parse(Request{Str: query})
	require.Error(t, err)
	require.Contains(t, err.Error(),
		"Unrecognized character in lexText: U+005D ']'")
}

func TestParseIdListError2(t *testing.T) {
	query := `
	query {
		user(func: uid( [0x1, 0x1, 2, 3, 0x34])) {
			type.object.name
		}
	}`
	_, err := Parse(Request{Str: query})
	require.Error(t, err)
	require.Contains(t, err.Error(),
		"Unexpected character [ while parsing request.")
}

func TestParseFirst(t *testing.T) {
	query := `
	query {
		user(func: uid( 0x1)) {
			type.object.name
			friends (first: 10) {
			}
		}
	}`
	res, err := Parse(Request{Str: query})
	require.NoError(t, err)
	require.NotNil(t, res.Query[0])
	require.Equal(t, []string{"type.object.name", "friends"}, childAttrs(res.Query[0]))
	require.Equal(t, "10", res.Query[0].Children[1].Args["first"])
}

func TestParseFirst_error(t *testing.T) {
	query := `
	query {
		user(func: uid( 0x1)) {
			type.object.name
			friends (first: ) {
			}
		}
	}`
	_, err := Parse(Request{Str: query})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Expecting argument value")
	require.Contains(t, err.Error(), "\")\"")
}

func TestParseAfter(t *testing.T) {
	query := `
	query {
		user(func: uid( 0x1)) {
			type.object.name
			friends (first: 10, after: 3) {
			}
		}
	}`
	res, err := Parse(Request{Str: query})
	require.NoError(t, err)
	require.NotNil(t, res.Query[0])
	require.Equal(t, childAttrs(res.Query[0]), []string{"type.object.name", "friends"})
	require.Equal(t, res.Query[0].Children[1].Args["first"], "10")
	require.Equal(t, res.Query[0].Children[1].Args["after"], "3")
}

func TestParseOffset(t *testing.T) {
	query := `
	query {
		user(func: uid( 0x1)) {
			type.object.name
			friends (first: 10, offset: 3) {
			}
		}
	}`
	res, err := Parse(Request{Str: query})
	require.NoError(t, err)
	require.NotNil(t, res.Query[0])
	require.Equal(t, childAttrs(res.Query[0]), []string{"type.object.name", "friends"})
	require.Equal(t, res.Query[0].Children[1].Args["first"], "10")
	require.Equal(t, res.Query[0].Children[1].Args["offset"], "3")
}

func TestParseOffset_error(t *testing.T) {
	query := `
	query {
		user(func: uid( 0x1)) {
			type.object.name
			friends (first: 10, offset: ) {
			}
		}
	}`
	_, err := Parse(Request{Str: query})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Expecting argument value")
	require.Contains(t, err.Error(), "\")\"")
}

func TestParse_error2(t *testing.T) {
	query := `
		query {
			me {
				name
			}
		}
	`
	_, err := Parse(Request{Str: query})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Expected Left round brackets")
	require.Contains(t, err.Error(), "\"{\"")

}

func TestParse_pass1(t *testing.T) {
	query := `
		{
			me(func: uid(0x0a)) {
				name,
				friends(xid:what) {  # xid would be ignored.
				}
			}
		}
	`
	_, err := Parse(Request{Str: query})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Got invalid keyword: xid")
}

func TestParseBadAlias(t *testing.T) {
	query := `
		{
			me(func: uid(0x0a)) {
				name: type.object.name.en: after_colon
				bestFriend: friends(first: 10) {
					name: type.object.name.hi
				}
			}
		}
	`
	_, err := Parse(Request{Str: query})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Invalid colon after alias declaration")
}

func TestParse_block(t *testing.T) {
	query := `
		{
			root(func: uid( 0x0a)) {
				type.object.name.es.419
			}
		}
	`
	res, err := Parse(Request{Str: query})
	require.NoError(t, err)
	require.NotNil(t, res.Query[0])
	require.Equal(t, childAttrs(res.Query[0]), []string{"type.object.name.es.419"})
}

func TestParseFuncNested(t *testing.T) {
	query := `
	query {
		me(func: gt(count(friend), 10)) {
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
	require.NotNil(t, res.Query[0].Func)
	require.Equal(t, res.Query[0].Func.Name, "gt")
	require.Equal(t, res.Query[0].Func.Args[0].Value, "10")
	require.Equal(t, res.Query[0].Func.IsCount, true)
}

func TestParseFuncNested2(t *testing.T) {
	query := `
	query {
		var(func:uid(1)) {
			a as name
		}
		me(func: eq(name, val(a))) {
			friends @filter() {
				name
			}
			hometown
		}
	}
`
	res, err := Parse(Request{Str: query})
	require.NoError(t, err)
	require.NotNil(t, res.Query[1])
	require.NotNil(t, res.Query[1].Func)
	require.Equal(t, res.Query[1].Func.Name, "eq")
	require.Equal(t, res.Query[1].Func.Args[0].Value, "a")
	require.Equal(t, res.Query[1].Func.Args[0].IsValueVar, true)
	require.Equal(t, res.Query[1].Func.IsCount, false)
}

func TestParseGeneratorError1(t *testing.T) {
	query := `{
		me(allofterms(name, "barack")) {
			friends {
				name
			}
			gender,age
			hometown
			count(friends)
		}
	}
`
	_, err := Parse(Request{Str: query})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Got invalid keyword: allofterms")
}

func TestParseGeneratorError2(t *testing.T) {
	query := `{
		me(func: allofterms(name, "barack")) {
			friends(all: 5) {
				name
			}
			gender,age
			hometown
			count(friends)
		}
	}
`
	_, err := Parse(Request{Str: query})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Got invalid keyword: all")
}

func TestParseQuotedFunctionAttributeError(t *testing.T) {
	query := `{
		me(func: allofterms("name", "barack")) {
			friends {
				name
			}
			gender,age
			hometown
			count(friends)
		}
	}
`
	_, err := Parse(Request{Str: query})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Attribute in function must not be quoted")
}

func TestParseCheckPwd(t *testing.T) {

	query := `{
		me(func: uid(1)) {
			checkpwd(password, "123456")
			hometown
		}
	}
`
	gq, err := Parse(Request{Str: query})
	require.NoError(t, err)
	require.Equal(t, "checkpwd", gq.Query[0].Children[0].Func.Name)
	require.Equal(t, "123456", gq.Query[0].Children[0].Func.Args[0].Value)
	require.Equal(t, "password", gq.Query[0].Children[0].Attr)
}

func TestParseComments(t *testing.T) {
	query := `
	# Something
	{
		me(func:allofterms(name, "barack")) {
			friends {
				name
			} # Something
			gender,age
			hometown
		}
	}
`
	_, err := Parse(Request{Str: query})
	require.NoError(t, err)
}

func TestParseComments1(t *testing.T) {
	query := `{
		#Something
		me(func:allofterms(name, "barack")) {
			friends {
				name  # Name of my friend
			}
			gender,age
			hometown
		}
	}
`
	_, err := Parse(Request{Str: query})
	require.NoError(t, err)
}

func TestParseGenerator(t *testing.T) {
	query := `{
		me(func:allofterms(name, "barack")) {
			friends {
				name
			}
			gender,age
			hometown
		}
	}
`
	_, err := Parse(Request{Str: query})
	require.NoError(t, err)
}

func TestParseNormalize(t *testing.T) {
	query := `
	query {
		me(func: uid( 0x3)) @normalize {
			friends {
				name
			}
			gender
			hometown
		}
}
`
	res, err := Parse(Request{Str: query})
	require.NoError(t, err)
	require.NotNil(t, res.Query[0])
	require.True(t, res.Query[0].Normalize)
}

func TestParseGroupbyRoot(t *testing.T) {
	query := `
	query {
		me(func: uid(1, 2, 3)) @groupby(friends) {
				a as count(uid)
		}

		groups(func: uid(a)) {
			uid
			val(a)
		}
	}
`
	res, err := Parse(Request{Str: query})
	require.NoError(t, err)
	require.Equal(t, 1, len(res.Query[0].GroupbyAttrs))
	require.Equal(t, "friends", res.Query[0].GroupbyAttrs[0].Attr)
	require.Equal(t, "a", res.Query[0].Children[0].Var)
}

func TestParseGroupby(t *testing.T) {
	query := `
	query {
		me(func: uid(0x1)) {
			friends @groupby(name@en) {
				count(uid)
			}
			hometown
			age
		}
	}
`
	res, err := Parse(Request{Str: query})
	require.NoError(t, err)
	require.Equal(t, 1, len(res.Query[0].Children[0].GroupbyAttrs))
	require.Equal(t, "name", res.Query[0].Children[0].GroupbyAttrs[0].Attr)
	require.Equal(t, "en", res.Query[0].Children[0].GroupbyAttrs[0].Langs[0])
}

func TestParseGroupbyWithAlias(t *testing.T) {
	query := `
	query {
		me(func: uid(0x1)) {
			friends @groupby(name) {
				GroupCount: count(uid)
			}
			hometown
			age
		}
	}
`
	res, err := Parse(Request{Str: query})
	require.NoError(t, err)
	require.Equal(t, 1, len(res.Query[0].Children[0].GroupbyAttrs))
	require.Equal(t, "name", res.Query[0].Children[0].GroupbyAttrs[0].Attr)
	require.Equal(t, "GroupCount", res.Query[0].Children[0].Children[0].Alias)
}

func TestParseGroupbyWithAliasForKey(t *testing.T) {
	query := `
	query {
		me(func: uid(0x1)) {
			friends @groupby(Name: name, SchooL: school) {
				count(uid)
			}
			hometown
			age
		}
	}
`
	res, err := Parse(Request{Str: query})
	require.NoError(t, err)
	require.Equal(t, 2, len(res.Query[0].Children[0].GroupbyAttrs))
	require.Equal(t, "Name", res.Query[0].Children[0].GroupbyAttrs[0].Alias)
	require.Equal(t, "SchooL", res.Query[0].Children[0].GroupbyAttrs[1].Alias)
}

func TestParseGroupbyWithAliasForError(t *testing.T) {
	query := `
	query {
		me(func: uid(0x1)) {
			friends @groupby(first: 10, SchooL: school) {
				count(uid)
			}
			hometown
			age
		}
	}
`
	_, err := Parse(Request{Str: query})
	require.Contains(t, err.Error(), "Can't use keyword first as alias in groupby")
}

func TestParseGroupbyError(t *testing.T) {
	// predicates not allowed inside groupby.
	query := `
	query {
		me(func: uid(0x1)) {
			friends @groupby(name) {
				name
				count(uid)
			}
			hometown
			age
		}
	}
`
	_, err := Parse(Request{Str: query})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Only aggregator/count functions allowed inside @groupby")
}

func TestParseRepeatArgsError1(t *testing.T) {
	// key can not be empty..
	query := `
	{
  		me(func: anyoftext(Text, "biology"), func: anyoftext(Text, "science")) {
			Text
  		}
	}
	`
	_, err := Parse(Request{Str: query})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Only one function allowed at root")
}

func TestParseRepeatArgsError2(t *testing.T) {
	// key can not be empty..
	query := `
	{
  		me(func: anyoftext(Text, "science")) {
			Text(first: 1, first: 4)
  		}
	}
	`
	_, err := Parse(Request{Str: query})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Got repeated key \"first\"")
}

func TestHasFuncAtRoot(t *testing.T) {
	query := `{
		me(func: has(name@en)) {
			name
		}
	}`
	_, err := Parse(Request{Str: query})
	require.NoError(t, err)
}

// this test tests parsing of EOF inside '...'
func TestDotsEOF(t *testing.T) {
	query := `{
		me(func: uid( 0x1)) {
			name
			..`
	_, err := Parse(Request{Str: query})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Unclosed action")
}

func TestMathDiv0(t *testing.T) {
	tests := []struct {
		in       string
		hasError bool
	}{
		{`{f(func: uid(1)){x:math(1+1)}}`, false},
		{`{f(func: uid(1)){x:math(1/0)}}`, true},
		{`{f(func: uid(1)){x:math(1/-0)}}`, true},
		{`{f(func: uid(1)){x:math(1/ln(1))}}`, true},
		{`{f(func: uid(1)){x:math(1/sqrt(0))}}`, true},
		{`{f(func: uid(1)){x:math(1/floor(0))}}`, true},
		{`{f(func: uid(1)){x:math(1/floor(0.5))}}`, true},
		{`{f(func: uid(1)){x:math(1/floor(1.01))}}`, false},
		{`{f(func: uid(1)){x:math(1/ceil(0))}}`, true},
		{`{f(func: uid(1)){x:math(1%0}}`, true},
		{`{f(func: uid(1)){x:math(1%floor(0)}}`, true},
		{`{f(func: uid(1)){x:math(1 + 0)}}`, false},
	}
	for _, tc := range tests {
		_, err := Parse(Request{Str: tc.in})
		if tc.hasError {
			require.Error(t, err, "Expected an error for %q", tc.in)
		} else {
			require.NoError(t, err, "Unexpected error for %q: %s", tc.in, err)
		}
	}
}

func TestMultipleEqual(t *testing.T) {
	query := `{
		me(func: eq(name,["Steven Spielberg", "Tom Hanks"])) {
			name
		}
	}`

	gql, err := Parse(Request{Str: query})
	require.NoError(t, err)
	require.Equal(t, 2, len(gql.Query[0].Func.Args))
	require.Equal(t, "Tom Hanks", gql.Query[0].Func.Args[1].Value)
}

func TestParseEqArg(t *testing.T) {
	query := `
	{
		me(func: uid(1, 20)) @filter(eq(name, ["And\"rea", "Bob"])) {
		 name
		}
	}
`
	gql, err := Parse(Request{Str: query})
	require.NoError(t, err)
	require.Equal(t, 2, len(gql.Query[0].Filter.Func.Args))
}

// TestParserFuzz replays inputs that were identified by go-fuzz to cause crash
// in the past. Used for regression testing.
// We don't care here about return value, only about correct handling of
// incorrect input.
func TestParserFuzz(t *testing.T) {
	tests := []struct {
		name string
		in   string
	}{
		{"test001", "{e(){@filter(q(/"},
		{"test002", "{e(){@filter(p(%"},
		{"test003", "{e(){@filter(0(/\\0"},
		{"test004", "query #"},
		{"test005", "{e(){{@filter(q(/"},
		{"test006", "{e(func:0(0(0,/"},
		{"test007", "{e(func:uid()){@filter(0(/\\"},
		{"test008", "{e(){@filter(0(/\\0/"},
		{"test009", "{e(){@filter(p(/\\"},
		{"test010", "{m(func:0(0(0(0,/"},
		{"test011", "{e(){@filter(0(%0"},
		{"test012", "{e(func:uid()){@filter(p(%"},
		{"test013", "{e(orderasc:val(0)){{min(val(0)0("},
		{"test014", "query{e(func:uid(val(0)"},
		{"test015", "{e(){@filter(0(%000"},
		{"test016", "{e(){@filter(0(%"},
		{"test017", "{e(){{@filter(0(/"},
		{"test018", "{e(func:uid(0))@filter(p(/\\"},
		{"test019", "{e(func:uid()){@filter(p(/\\"},
		{"test020", "{e(){@filter(p(/\\00"},
		{"test021", "{e(func:uid(0)){@filter(0(%"},
		{"test022", "{e(){@filter(0(/"},
		{"test023", "{e(func:uid()){@filter(p(/"},
		{"test024", "{s(func:uid(val(0)"},
		{"test025", "{e()@filter(0(%"},
		{"test026", "{e(func:uid(0)){@filter(0(/\\"},
		{"test027", "{e(){@filter(0(%"},
		{"test028", "{e(){@filter(0(%00"},
		{"test029", "{e(func:uid(0)){@filter(p(/\\e/i)){e}}}"},
		{"test030", "{e(func:uid(0)){@filter(p(%//i))00"},
		{"test031", "{e()@filter(p(/�Is))}"},
		{"test032", "{e(){@filter(0(0,/"},
		{"test033", "{e(func:uid()){@filter(0(/"},
		{"test034", "{e()@filter(0(/"},
		{"test035", "{e(func:uid(0)){@filter(p(/\\"},
		{"test036", "{e(func:uid())@filter(0(%"},
		{"test037", "{e(func:uid(0)){@filter(0(%"},
		{"test038", "{e(func:uid(0))@filter(p(/\\0/"},
		{"test039", "{e(func:uid(0)){@filter(p(/\\e/i)){e}00"},
		{"test040", "{e(){@filter(0(-"},
		{"test041", "{e(func:uid(0)){@filter(0(%0"},
		{"test042", "{e()@filter(q(/"},
		{"test043", "{e(func:uid(0)){@filter(p(%"},
		{"test044", "{e()@filter(p(/"},
		{"test045", "{e(func:uid())@filter(0(/"},
		{"test046", "{e(func:uid(0)){@filter(p(/\\e/"},
		{"test047", "{e(func:uid()){@filter(0(%"},
		{"test048", "{e()@filter(0(0,/"},
		{"test049", "{e(){{@filter(0(0,/"},
		{"test050", "{e(func:uid(0)){@filter(p(/"},
		{"test051", "{e()@filter(0(-"},
		{"test052", "{e(func:uid(0)){@filter(0(/"},
		{"test053", "{e(func:uid())@filter(p(%"},
		{"test054", "{e(orderasc:val(0)){min(val(0)0("},
		{"test055", "{e(){@filter(p(/"},
		{"test056", "{e(func:uid())@filter(p(/"},
		{"test057", "a<><\\�"},
		{"test058", "L<\\𝌀"},
		{"test059", "{d(after:<>0)}"},
		{"test060", "{e(orderasc:#"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("parser panic caused by test: '%s', input: '%s': %v\n%s", test.name, test.in, r, debug.Stack())
				}
			}()

			Parse(Request{Str: test.in})
		})
	}
}

func TestParseEqArg2(t *testing.T) {
	query := `
	{
		me(func: eq(age, [1, 20])) @filter(eq(name, ["And\"rea", "Bob"])) {
		 name
		}
	}
`
	gql, err := Parse(Request{Str: query})
	require.NoError(t, err)
	require.Equal(t, 2, len(gql.Query[0].Filter.Func.Args))
	require.Equal(t, 2, len(gql.Query[0].Func.Args))
}

func TestIdErr(t *testing.T) {
	query := `
	{
		me(id: [1, 3 , 5, 7]) @filter(uid(3, 7)) {
			name
		}
	}
	`
	_, err := Parse(Request{Str: query})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Got invalid keyword: id")
}

func TestAggRoot1(t *testing.T) {
	query := `
		{
			var(func: anyofterms(name, "Rick Michonne Andrea")) {
				a as age
			}

			me() {
				sum(val(a))
				avg(val(a))
			}
		}
	`
	gql, err := Parse(Request{Str: query})
	require.NoError(t, err)
	require.Equal(t, "me", gql.Query[1].Alias)
	require.Equal(t, true, gql.Query[1].IsEmpty)
}

func TestAggRootError(t *testing.T) {
	query := `
		{
			var(func: anyofterms(name, "Rick Michonne Andrea")) {
				a as age
			}

			me() {
				sum(val(a))
				avg(val(a))
				friend @filter(anyofterms(name, "Hey")) {
					name
				}
			}
		}
	`
	_, err := Parse(Request{Str: query})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Only aggregation/math functions allowed inside empty blocks. Got: friend")
}

func TestAggRootError2(t *testing.T) {
	query := `
		{
			var(func: anyofterms(name, "Rick Michonne Andrea")) {
				a as age
			}

			me() {
				avg(val(a))
				name
			}
		}
	`
	_, err := Parse(Request{Str: query})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Only aggregation/math functions allowed inside empty blocks. Got: name")
}

func TestAggRootError3(t *testing.T) {
	query := `
		{
			var(func: anyofterms(name, "Rick Michonne Andrea")) {
				a as age
			}

			me() {
				avg
			}
		}
	`
	_, err := Parse(Request{Str: query})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Only aggregation/math functions allowed inside empty blocks. Got: avg")
}

func TestEmptyFunction(t *testing.T) {
	query := `
		{
		  director(func:allofterms()) {
		    name@en
		    director.film (orderdesc: initial_release_date) {
		      name@en
		      initial_release_date
		    }
		  }
		}`

	_, err := Parse(Request{Str: query})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Got empty attr for function: [allofterms]")
}

func TestEqArgWithDollar(t *testing.T) {
	// This is a fix for #1444.
	query := `
	{
		ab(func: eq(name@en, "$pringfield (or, How)")) {
			uid
		}
	}
	`
	gql, err := Parse(Request{Str: query})
	require.NoError(t, err)
	require.Equal(t, gql.Query[0].Func.Args[0].Value, `$pringfield (or, How)`)
}

func TestInvalidValUsage(t *testing.T) {
	query := `
		{
			me(func: uid(0x01)) {
				val(uid) {
					nope
				}
			}
		}
	`
	_, err := Parse(Request{Str: query})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Query syntax invalid.")
}

func parseNquads(b []byte) ([]*api.NQuad, error) {
	var lexer lex.Lexer
	var nqs []*api.NQuad
	for _, line := range bytes.Split(b, []byte{'\n'}) {
		nq, err := chunker.ParseRDF(string(line), &lexer)
		if err == chunker.ErrEmpty {
			continue
		}
		if err != nil {
			return nil, err
		}
		nqs = append(nqs, &nq)
	}
	return nqs, nil
}

func TestUidInWithNoParseErrors(t *testing.T) {
	query := `{
		schoolVar as q(func: uid(5000))
		me(func: uid(1, 23, 24 )) {
			friend @filter(uid_in(school, uid(schoolVar))) {
				name
			}
		}
	}`
	_, err := Parse(Request{Str: query})
	require.NoError(t, err)
}

func TestUidInWithParseErrors(t *testing.T) {
	tcases := []struct {
		description string
		query       string
		expectedErr error
	}{
		{
			description: "uid_in query with without argument",
			query: `{
				me(func: uid(1, 23, 24 )) {
					friend @filter(uid_in(school, )) {
						name
					}
				}
			}`,
			expectedErr: errors.New("Empty Argument"),
		},
		{
			description: "uid_in query with without argument (2)",
			query: `{
				me(func: uid(1, 23, 24 )) {
					friend @filter(uid_in(school )) {
						name
					}
				}
			}`,
			expectedErr: errors.New("uid_in function expects an argument, got none"),
		},
		{
			description: "query with nested uid without variable",
			query: `{
				me(func: uid(1, 23, 24 )) {
					friend @filter(uid_in(school, uid(5000))) {
						name
					}
				}
			}`,
			expectedErr: errors.New("Nested uid fn expects 1 uid variable, got 0"),
		},
		{
			description: "query with nested uid with variable and constant",
			query: `{
				uidVar as q(func: uid( 5000))
				me(func: uid(1, 23, 24 )) {
					friend @filter(uid_in(school, uid(uidVar, 5001))) {
						name
					}
				}
			}`,
			expectedErr: errors.New("Nested uid fn expects only uid variable, got UID"),
		},
		{
			description: "query with nested uid with two variables",
			query: `{
				uidVar1 as q(func: uid( 5000))
				uidVar2 as q(func: uid( 5000))
				me(func: uid(1, 23, 24 )) {
					friend @filter(uid_in(school, uid(uidVar1, uidVar2))) {
						name
					}
				}
			}`,
			expectedErr: errors.New("Nested uid fn expects 1 uid variable, got 2"),
		},
		{
			description: "query with nested uid with gql variable",
			query: `query queryWithGQL($schoolUID: string = "5001"){
				me(func: uid(1, 23, 24 )){
					friend @filter(uid_in(school, uid( $schoolUID))) {
						name
					}
				}
			}`,
			expectedErr: errors.New("Nested uid fn expects 1 uid variable, got 0"),
		},
	}
	for _, test := range tcases {
		t.Run(test.description, func(t *testing.T) {
			_, err := Parse(Request{Str: test.query})
			require.Contains(t, err.Error(), test.expectedErr.Error())
		})
	}
}

func TestLineAndColumnNumberInErrorOutput(t *testing.T) {
	q := `
	query {
		me(func: uid(0x0a)) {
			friends @filter(alloftext(descr@, "something")) {
				name
			}
			gender,age
			hometown
		}
	}`
	_, err := Parse(Request{Str: q})
	require.Error(t, err)
	require.Contains(t, err.Error(),
		"line 4 column 35: Unrecognized character in lexDirective: U+002C ','")
}

func TestTypeFunction(t *testing.T) {
	q := `
	query {
		me(func: type(Person)) {
			name
		}
	}`
	gq, err := Parse(Request{Str: q})
	require.NoError(t, err)
	require.Equal(t, 1, len(gq.Query))
	require.Equal(t, "type", gq.Query[0].Func.Name)
	require.Equal(t, 1, len(gq.Query[0].Func.Args))
	require.Equal(t, "Person", gq.Query[0].Func.Args[0].Value)
}

func TestTypeFunctionError1(t *testing.T) {
	q := `
	query {
		me(func: type(Person, School)) {
			name
		}
	}`
	_, err := Parse(Request{Str: q})
	require.Error(t, err)
	require.Contains(t, err.Error(), "type function only supports one argument")
}

func TestParseExpandType(t *testing.T) {
	query := `
	{
		var(func: has(name)) {
			expand(Person,Animal) {
				uid
			}
		}
	}
`
	gq, err := Parse(Request{Str: query})
	require.NoError(t, err)
	require.Equal(t, 1, len(gq.Query))
	require.Equal(t, 1, len(gq.Query[0].Children))
	require.Equal(t, "expand", gq.Query[0].Children[0].Attr)
	require.Equal(t, "Person,Animal", gq.Query[0].Children[0].Expand)
	require.Equal(t, 1, len(gq.Query[0].Children[0].Children))
	require.Equal(t, "uid", gq.Query[0].Children[0].Children[0].Attr)
}

func TestRecurseWithArgs(t *testing.T) {
	query := `
	{
		me(func: eq(name, "sad")) @recurse(depth: $hello , loop: true) {
		}
	}`
	gq, err := Parse(Request{Str: query, Variables: map[string]string{"$hello": "1"}})
	require.NoError(t, err)
	require.Equal(t, gq.Query[0].RecurseArgs.Depth, uint64(1))

	query = `
	{
		me(func: eq(name, "sad"))@recurse(depth: 1 , loop: $hello) {
		}
	}`
	gq, err = Parse(Request{Str: query, Variables: map[string]string{"$hello": "true"}})
	require.NoError(t, err)
	require.Equal(t, gq.Query[0].RecurseArgs.AllowLoop, true)

	query = `
	{
		me(func: eq(name, "sad"))@recurse(depth: $hello, loop: $hello1) {
		}
	}`
	gq, err = Parse(Request{Str: query, Variables: map[string]string{"$hello": "1", "$hello1": "true"}})
	require.NoError(t, err)
	require.Equal(t, gq.Query[0].RecurseArgs.AllowLoop, true)
	require.Equal(t, gq.Query[0].RecurseArgs.Depth, uint64(1))

	query = `
	{
		me(func: eq(name, "sad"))@recurse(depth: $_hello_hello, loop: $hello1_heelo1) {
		}
	}`
	gq, err = Parse(Request{Str: query, Variables: map[string]string{"$_hello_hello": "1",
		"$hello1_heelo1": "true"}})
	require.NoError(t, err)
	require.Equal(t, gq.Query[0].RecurseArgs.AllowLoop, true)
	require.Equal(t, gq.Query[0].RecurseArgs.Depth, uint64(1))
}

func TestRecurseWithArgsWithError(t *testing.T) {
	query := `
	{
		me(func: eq(name, "sad"))@recurse(depth: $hello, loop: true) {
		}
	}`
	_, err := Parse(Request{Str: query})
	require.Error(t, err)
	require.Contains(t, err.Error(), "variable $hello not defined")

	query = `
	{
		me(func: eq(name, "sad"))@recurse(depth: 1, loop: $hello) {
		}
	}`
	_, err = Parse(Request{Str: query})
	require.Error(t, err)
	require.Contains(t, err.Error(), "variable $hello not defined")

	query = `
	{
		me(func: eq(name, "sad"))@recurse(depth: $hello, loop: $hello1) {
		}
	}`
	_, err = Parse(Request{Str: query, Variables: map[string]string{"$hello": "sd", "$hello1": "true"}})
	require.Error(t, err)
	require.Contains(t, err.Error(), "should be type of integer")

	query = `
	{
		me(func: eq(name, "sad"))@recurse(depth: $hello, loop: $hello1) {
		}
	}`
	_, err = Parse(Request{Str: query, Variables: map[string]string{"$hello": "1", "$hello1": "tre"}})
	require.Error(t, err)
	require.Contains(t, err.Error(), "should be type of boolean")
}

func TestRecurse(t *testing.T) {
	query := `
	{
		me(func: eq(name, "sad"))@recurse(depth: 1, loop: true) {
		}
	}`
	gq, err := Parse(Request{Str: query})
	require.NoError(t, err)
	require.Equal(t, gq.Query[0].RecurseArgs.Depth, uint64(1))
	require.Equal(t, gq.Query[0].RecurseArgs.AllowLoop, true)
}

func TestRecurseWithError(t *testing.T) {
	query := `
	{
		me(func: eq(name, "sad"))@recurse(depth: hello, loop: true) {
		}
	}`
	_, err := Parse(Request{Str: query})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Value inside depth should be type of integer")
	query = `
	{
		me(func: eq(name, "sad"))@recurse(depth: 1, loop: tre) {
		}
	}`
	_, err = Parse(Request{Str: query})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Value inside loop should be type of boolean")
}

func TestLexQueryWithValidQuery(t *testing.T) {
	query := `{
		q(func: allofterms(<name:is>, "hey you there"), first:20, offset:0, orderasc:Pokemon.id){
			uid
			expand(_all_)(first:1){
				uid
				Pokemon.name
				expand(_all_)(first:1)
			}
		}
		n(func:type(Pokemon)){
			count:count(uid)
		}
	}`

	items := LexQuery(query)
	for i, item := range items {
		t.Logf("[%d] item: %+v\n", i, item)
	}
	require.Equal(t, 68, len(items))
}

func TestLexQueryWithInvalidQuery(t *testing.T) {
	query := `{
		q(func: allofterms(<name:is>, "hey you there"), first: 20, offset:0, orderasc:Pokemon.id){
			uid
		}
		n(func:type(Pokemon)){
			count:count(uid)
	}`

	items := LexQuery(query)
	for i, item := range items {
		t.Logf("[%d] item: %+v\n", i, item.Typ)
	}
	require.Equal(t, 45, len(items))
	require.Equal(t, lex.ItemError, items[44].Typ)
}

func TestCascade(t *testing.T) {
	query := `{
		names(func: has(name)) @cascade {
		  name
		}
	  }`
	gq, err := Parse(Request{
		Str: query,
	})
	require.NoError(t, err)
	require.Equal(t, gq.Query[0].Cascade[0], "__all__")
}

func TestCascadeParameterized(t *testing.T) {
	query := `{
		names(func: has(name)) @cascade(name, age) {
		  name
		  age
		  dob
		}
	  }`
	gq, err := Parse(Request{
		Str: query,
	})
	require.NoError(t, err)
	require.Equal(t, gq.Query[0].Cascade[0], "name")
	require.Equal(t, gq.Query[0].Cascade[1], "age")
}

func TestBadCascadeParameterized(t *testing.T) {
	badQueries := []string{
		`{
			names(func: has(name)) @cascade( {
			  name
			  age
			  dob
			}
		  }`,
		`{
			names(func: has(name)) @cascade) {
			  name
			  age
			  dob
			}
		 }`,
		`{
			names(func: has(name)) @cascade() {
			  name
			  age
			  dob
			}
		  }`,
		`{
			names(func: has(name)) @cascade(,) {
			  name
			  age
			  dob
			}
		  }`,
		`{
			names(func: has(name)) @cascade(name,) {
			  name
			  age
			  dob
			}
		  }`,
		`{
			names(func: has(name)) @cascade(,name) {
			  name
			  age
			  dob
			}
		  }`,
	}

	for _, query := range badQueries {
		_, err := Parse(Request{
			Str: query,
		})
		require.Error(t, err)
	}
}

func TestEmptyId(t *testing.T) {
	q := "query me($a: string) { q(func: uid($a)) { name }}"
	r := Request{
		Str:       q,
		Variables: map[string]string{"$a": "   "},
	}
	_, err := Parse(r)
	require.Error(t, err, "ID cannot be empty")
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
