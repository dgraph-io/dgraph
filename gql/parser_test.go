/*
 * Copyright 2015 DGraph Labs, Inc.
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

package gql

import (
	"strings"
	"testing"

	"github.com/dgraph-io/dgraph/schema"
	"github.com/stretchr/testify/require"
)

func childAttrs(g *GraphQuery) []string {
	var out []string
	for _, c := range g.Children {
		out = append(out, c.Attr)
	}
	return out
}

func TestParse(t *testing.T) {
	query := `
	query {
		me(id:0x0a) {
			friends {
				name
			}
			gender,age
			hometown
		}
	}
`
	res, err := Parse(query)
	require.NoError(t, err)
	require.NotNil(t, res.Query)
	require.Equal(t, childAttrs(res.Query), []string{"friends", "gender", "age", "hometown"})
	require.Equal(t, childAttrs(res.Query.Children[0]), []string{"name"})
}

func TestParseError(t *testing.T) {
	query := `
		me(id:0x0a) {
			friends {
				name
			}
			gender,age
			hometown
		}
	}
`
	_, err := Parse(query)
	require.Error(t, err)
}

func TestParseXid(t *testing.T) {
	// logrus.SetLevel(logrus.DebugLevel)
	// TODO: Why does the query not have _xid_ attribute?
	query := `
	query {
		user(id: 0x11) {
			type.object.name
		}
	}`
	res, err := Parse(query)
	require.NoError(t, err)
	require.NotNil(t, res.Query)
	require.Equal(t, childAttrs(res.Query), []string{"type.object.name"})
}

func TestParseIdList(t *testing.T) {
	query := `
	query {
		user(id: [0x1]) {
			type.object.name
		}
	}`
	r, err := Parse(query)
	gq := r.Query
	require.NoError(t, err)
	require.NotNil(t, gq)
	require.Equal(t, []string{"type.object.name"}, childAttrs(gq))
	require.Equal(t, []uint64{1}, gq.UID)
}

func TestParseIdList1(t *testing.T) {
	query := `
	query {
		user(id: [m.abcd, 0x1, abc, ade, 0x34]) {
			type.object.name
		}
	}`
	r, err := Parse(query)
	gq := r.Query
	require.NoError(t, err)
	require.NotNil(t, gq)
	require.Equal(t, []string{"type.object.name"}, childAttrs(gq))
	require.Equal(t, []uint64{0xfe5de827fdf27a88, 0x1, 0x24a5b3a074e7f369, 0xf023e8d0d7c08cf3, 0x34}, gq.UID)
	require.Equal(t, 5, len(gq.UID))
}

func TestParseIdListError(t *testing.T) {
	query := `
	query {
		user(id: [m.abcd, 0x1, abc, ade, 0x34) {
			type.object.name
		}
	}`
	_, err := Parse(query)
	require.Error(t, err)
}

func TestParseFirst(t *testing.T) {
	query := `
	query {
		user(id: m.abcd) {
			type.object.name
			friends (first: 10) {
			}
		}
	}`
	res, err := Parse(query)
	require.NoError(t, err)
	require.NotNil(t, res.Query)
	require.Equal(t, []string{"type.object.name", "friends"}, childAttrs(res.Query))
	require.Equal(t, "10", res.Query.Children[1].Args["first"])
}

func TestParseFirst_error(t *testing.T) {
	query := `
	query {
		user(id: m.abcd) {
			type.object.name
			friends (first: ) {
			}
		}
	}`
	_, err := Parse(query)
	require.Error(t, err)
}

func TestParseAfter(t *testing.T) {
	query := `
	query {
		user(id: m.abcd) {
			type.object.name
			friends (first: 10, after: 3) {
			}
		}
	}`
	res, err := Parse(query)
	require.NoError(t, err)
	require.NotNil(t, res.Query)
	require.Equal(t, childAttrs(res.Query), []string{"type.object.name", "friends"})
	require.Equal(t, res.Query.Children[1].Args["first"], "10")
	require.Equal(t, res.Query.Children[1].Args["after"], "3")
}

func TestParseOffset(t *testing.T) {
	query := `
	query {
		user(id: m.abcd) {
			type.object.name
			friends (first: 10, offset: 3) {
			}
		}
	}`
	res, err := Parse(query)
	require.NoError(t, err)
	require.NotNil(t, res.Query)
	require.Equal(t, childAttrs(res.Query), []string{"type.object.name", "friends"})
	require.Equal(t, res.Query.Children[1].Args["first"], "10")
	require.Equal(t, res.Query.Children[1].Args["offset"], "3")
}

func TestParseOffset_error(t *testing.T) {
	query := `
	query {
		user(id: m.abcd) {
			type.object.name
			friends (first: 10, offset: ) {
			}
		}
	}`
	_, err := Parse(query)
	require.Error(t, err)
}

func TestParse_error2(t *testing.T) {
	query := `
		query {
			me {
				name
			}
		}
	`
	_, err := Parse(query)
	require.Error(t, err)
}

func TestParse_pass1(t *testing.T) {
	query := `
		{
			me(id:0x0a) {
				name,
				friends(xid:what) {  # xid would be ignored.
				}
			}
		}
	`
	res, err := Parse(query)
	require.NoError(t, err)
	require.NotNil(t, res.Query)
	require.Equal(t, childAttrs(res.Query), []string{"name", "friends"})
	require.Empty(t, childAttrs(res.Query.Children[1]))
}

func TestParse_alias(t *testing.T) {
	query := `
		{
			me(id:0x0a) {
				name,
				bestFriend: friends(first: 10) { 
					name	
				}
			}
		}
	`
	res, err := Parse(query)
	require.NoError(t, err)
	require.NotNil(t, res.Query)
	require.Equal(t, childAttrs(res.Query), []string{"name", "friends"})
	require.Equal(t, res.Query.Children[1].Alias, "bestFriend")
	require.Equal(t, childAttrs(res.Query.Children[1]), []string{"name"})
}

func TestParse_alias1(t *testing.T) {
	query := `
		{
			me(id:0x0a) {
				name: type.object.name.en
				bestFriend: friends(first: 10) { 
					name: type.object.name.hi	
				}
			}
		}
	`
	res, err := Parse(query)
	require.NoError(t, err)
	require.NotNil(t, res.Query)
	require.Equal(t, childAttrs(res.Query), []string{"type.object.name.en", "friends"})
	require.Equal(t, res.Query.Children[1].Alias, "bestFriend")
	require.Equal(t, res.Query.Children[1].Children[0].Alias, "name")
	require.Equal(t, childAttrs(res.Query.Children[1]), []string{"type.object.name.hi"})
}

func TestParse_block(t *testing.T) {
	query := `
		{
			root(id: 0x0a) {
				type.object.name.es-419
			}
		}
	`
	res, err := Parse(query)
	require.NoError(t, err)
	require.NotNil(t, res.Query)
	require.Equal(t, childAttrs(res.Query), []string{"type.object.name.es-419"})
}

func TestParseMutation(t *testing.T) {
	query := `
		mutation {
			set {
				<name> <is> <something> .
				<hometown> <is> <san francisco> .
			}
			delete {
				<name> <is> <something-else> .
			}
		}
	`
	res, err := Parse(query)
	require.NoError(t, err)
	require.NotEqual(t, strings.Index(res.Mutation.Set, "<name> <is> <something> ."), -1)
	require.NotEqual(t, strings.Index(res.Mutation.Set, "<hometown> <is> <san francisco> ."), -1)
	require.NotEqual(t, strings.Index(res.Mutation.Del, "<name> <is> <something-else> ."), -1)
}

func TestParseMutation_error(t *testing.T) {
	query := `
		mutation {
			set {
				<name> <is> <something> .
				<hometown> <is> <san francisco> .
			}
			delete {
				<name> <is> <something-else> .
		}
	`
	_, err := Parse(query)
	require.Error(t, err)
}

func TestParseMutation_error2(t *testing.T) {
	query := `
		mutation {
			set {
				<name> <is> <something> .
				<hometown> <is> <san francisco> .
			}
			delete {
				<name> <is> <something-else> .
			}
		}
		mutation {
			set {
				another one?
			}
		}

	`
	_, err := Parse(query)
	require.Error(t, err)
}

func TestParseMutationAndQuery(t *testing.T) {
	query := `
		mutation {
			set {
				<name> <is> <something> .
				<hometown> <is> <san francisco> .
			}
			delete {
				<name> <is> <something-else> .
			}
		}
		query {
			me(id: tomhanks) {
				name
				hometown
			}
		}
	`
	res, err := Parse(query)
	require.NoError(t, err)
	require.NotNil(t, res.Mutation)
	require.NotEqual(t, strings.Index(res.Mutation.Set, "<name> <is> <something> ."), -1)
	require.NotEqual(t, strings.Index(res.Mutation.Set, "<hometown> <is> <san francisco> ."), -1)
	require.NotEqual(t, strings.Index(res.Mutation.Del, "<name> <is> <something-else> ."), -1)

	require.NotNil(t, res.Query)
	require.Equal(t, 1, len(res.Query.UID))
	require.Equal(t, childAttrs(res.Query), []string{"name", "hometown"})
}

func TestParseFragmentNoNesting(t *testing.T) {
	query := `
	query {
		user(id:0x0a) {
			...fragmenta,...fragmentb
			friends {
				name
			}
			...fragmentc
			hobbies
			...fragmentd
		}
	}
	
	fragment fragmenta {
		name
	}
	
	fragment fragmentb {
		id
	}
	
	fragment fragmentc {
		name
	}
	
	fragment fragmentd {
		id
	}
`
	res, err := Parse(query)
	require.NoError(t, err)
	require.NotNil(t, res.Query)
	require.Equal(t, childAttrs(res.Query), []string{"name", "id", "friends", "name", "hobbies", "id"})
}

func TestParseFragmentNest1(t *testing.T) {
	query := `
	query {
		user(id:0x0a) {
			...fragmenta
			friends {
				name
			}
		}
	}
	
	fragment fragmenta {
		id
		...fragmentb
	}
	
	fragment fragmentb {
		hobbies
	}
`
	res, err := Parse(query)
	require.NoError(t, err)
	require.NotNil(t, res.Query)
	require.Equal(t, childAttrs(res.Query), []string{"id", "hobbies", "friends"})
}

func TestParseFragmentNest2(t *testing.T) {
	query := `
	query {
		user(id:0x0a) {
			friends {
				...fragmenta
			}
		}
	}
	fragment fragmenta {
		name
		...fragmentb
	}
	fragment fragmentb {
		nickname
	}
`
	res, err := Parse(query)
	require.NoError(t, err)
	require.NotNil(t, res.Query)
	require.Equal(t, childAttrs(res.Query), []string{"friends"})
	require.Equal(t, childAttrs(res.Query.Children[0]), []string{"name", "nickname"})
}

func TestParseFragmentCycle(t *testing.T) {
	query := `
	query {
		user(id:0x0a) {
			...fragmenta
		}
	}
	fragment fragmenta {
		name
		...fragmentb
	}
	fragment fragmentb {
		...fragmentc
	}
	fragment fragmentc {
		id
		...fragmenta
	}
`
	_, err := Parse(query)
	require.Error(t, err, "Expected error with cycle")
}

func TestParseFragmentMissing(t *testing.T) {
	query := `
	query {
		user(id:0x0a) {
			...fragmenta
		}
	}
	fragment fragmentb {
		...fragmentc
	}
	fragment fragmentc {
		id
		...fragmenta
	}
`
	_, err := Parse(query)
	require.Error(t, err, "Expected error with missing fragment")
}

func TestParseVariables(t *testing.T) {
	query := `{
		"query": "query testQuery( $a  : int   , $b: int){root(id: 0x0a) {name(first: $b, after: $a){english}}}", 
		"variables": {"$a": "6", "$b": "5" } 
	}`
	_, err := Parse(query)
	require.NoError(t, err)
}

func TestParseVariables1(t *testing.T) {
	query := `{
		"query": "query testQuery($a: int , $b: int!){root(id: 0x0a) {name(first: $b){english}}}", 
		"variables": {"$b": "5" } 
	}`
	_, err := Parse(query)
	require.NoError(t, err)
}

func TestParseVariables2(t *testing.T) {
	query := `{
		"query": "query testQuery($a: float , $b: bool!){root(id: 0x0a) {name{english}}}", 
		"variables": {"$b": "false", "$a": "3.33" } 
	}`
	_, err := Parse(query)
	require.NoError(t, err)
}

func TestParseVariables3(t *testing.T) {
	query := `{
		"query": "query testQuery($a: int , $b: int! ){root(id: 0x0a) {name(first: $b){english}}}", 
		"variables": {"$a": "5", "$b": "3"} 
	}`
	_, err := Parse(query)
	require.NoError(t, err)
}

func TestParseVariablesStringfiedJSON(t *testing.T) {
	query := `{
		"query": "query testQuery($a: int! , $b: int){root(id: 0x0a) {name(first: $b){english}}}", 
		"variables": "{\"$a\": \"5\" }" 
	}`
	_, err := Parse(query)
	require.NoError(t, err)
}

func TestParseVariablesDefault1(t *testing.T) {
	query := `{
		"query": "query testQuery($a: int = 3  , $b: int =  4 ,  $c : int = 3){root(id: 0x0a) {name(first: $b, after: $a, offset: $c){english}}}", 
		"variables": {"$b": "5" } 
	}`
	_, err := Parse(query)
	require.NoError(t, err)
}
func TestParseVariablesFragments(t *testing.T) {
	query := `{
	"query": "query test($a: int){user(id:0x0a) {...fragmentd,friends(first: $a, offset: $a) {name}}} fragment fragmentd {id(first: $a)}",
	"variables": {"$a": "5"}
}`
	res, err := Parse(query)
	require.NoError(t, err)
	require.NotNil(t, res.Query)
	require.Equal(t, childAttrs(res.Query), []string{"id", "friends"})
	require.Empty(t, childAttrs(res.Query.Children[0]))
	require.Equal(t, childAttrs(res.Query.Children[1]), []string{"name"})
	require.Equal(t, res.Query.Children[0].Args["first"], "5")
}

func TestParseVariablesError1(t *testing.T) {
	query := `
	query testQuery($a: string, $b: int!){
			root(id: 0x0a) {
				type.object.name.es-419
			}
		}
	`
	_, err := Parse(query)
	require.Error(t, err)
}

func TestParseVariablesError2(t *testing.T) {
	query := `{
		"query": "query testQuery($a: int, $b: int, $c: int!){
			root(id: 0x0a) {name(first: $b, after: $a){english}}
		}", 
		"variables": {"$a": "6", "$b": "5" } 
	}`
	_, err := Parse(query)
	require.Error(t, err, "Expected value for variable $c")
}

func TestParseVariablesError3(t *testing.T) {
	query := `{
		"query": "query testQuery($a: int, $b: , $c: int!){
			root(id: 0x0a) {name(first: $b, after: $a){english}}
		}", 
		"variables": {"$a": "6", "$b": "5" } 
	}`
	_, err := Parse(query)
	require.Error(t, err, "Expected type for variable $b")
}

func TestParseVariablesError4(t *testing.T) {
	query := `{
		"query": "query testQuery($a: bool , $b: float! = 3){root(id: 0x0a) {name(first: $b){english}}}", 
		"variables": {"$a": "5" } 
	}`
	_, err := Parse(query)
	require.Error(t, err, "Expected type error")
}

func TestParseVariablesError5(t *testing.T) {
	query := `{
		"query": "query ($a: int, $b: int){root(id: 0x0a) {name(first: $b, after: $a){english}}}", 
		"variables": {"$a": "6", "$b": "5" } 
	}`
	_, err := Parse(query)
	require.Error(t, err, "Expected error: Query with variables should be named")
}

func TestParseVariablesError6(t *testing.T) {
	query := `{
		"query": "query ($a: int, $b: random){root(id: 0x0a) {name(first: $b, after: $a){english}}}", 
		"variables": {"$a": "6", "$b": "5" } 
	}`
	_, err := Parse(query)
	require.Error(t, err, "Expected error: Type random not supported")
}

func TestParseVariablesError7(t *testing.T) {
	query := `{
		"query": "query testQuery($a: int, $b: int, $c: int!){
			root(id: 0x0a) {name(first: $b, after: $a){english}}
		}", 
		"variables": {"$a": "6", "$b": "5", "$d": "abc" } 
	}`
	_, err := Parse(query)
	require.Error(t, err, "Expected type for variable $d")
}

func TestParseVariablesiError8(t *testing.T) {
	query := `{
		"query": "query testQuery($a: int = 3  , $b: int! =  4 ,  $c : int = 3){root(id: 0x0a) {name(first: $b, after: $a, offset: $c){english}}}", 
		"variables": {"$b": "5" } 
	}`
	_, err := Parse(query)
	require.Error(t, err, "Variables type ending with ! cant have default value")
}

func TestParseFilter_root(t *testing.T) {
	schema.ParseBytes([]byte("scalar abc: string @index"))
	query := `
	query {
		me(abc(abc)) @filter(allof(name, "alice")) {
			friends @filter() {
				name @filter(namefilter("a"))
			}
			gender @filter(eq("a")),age @filter(neq("a", "b"))
			hometown
		}
	}
`
	res, err := Parse(query)
	require.NoError(t, err)
	require.NotNil(t, res.Query)
	require.NotNil(t, res.Query.Filter)
	require.Equal(t, `(allof "name" "alice")`, res.Query.Filter.debugString())
	require.Equal(t, []string{"friends", "gender", "age", "hometown"}, childAttrs(res.Query))
	require.Equal(t, []string{"name"}, childAttrs(res.Query.Children[0]))
	require.Nil(t, res.Query.Children[0].Filter)
	require.Equal(t, `(eq "a")`, res.Query.Children[1].Filter.debugString())
	require.Equal(t, `(neq "a" "b")`, res.Query.Children[2].Filter.debugString())
	require.Equal(t, `(namefilter "a")`, res.Query.Children[0].Children[0].Filter.debugString())
}

func TestParseFilter_root_Error(t *testing.T) {
	query := `
	query {
		me(id:0x0a) @filter(allof(name, "alice") {
			friends @filter() {
				name @filter(namefilter("a"))
			}
			gender @filter(eq("a")),age @filter(neq("a", "b"))
			hometown
		}
	}
`
	_, err := Parse(query)
	require.Error(t, err)
}

func TestParseFilter_simplest(t *testing.T) {
	query := `
	query {
		me(id:0x0a) {
			friends @filter() {
				name @filter(namefilter("a"))
			}
			gender @filter(eq("a")),age @filter(neq("a", "b"))
			hometown
		}
	}
`
	res, err := Parse(query)
	require.NoError(t, err)
	require.NotNil(t, res.Query)
	require.Equal(t, []string{"friends", "gender", "age", "hometown"}, childAttrs(res.Query))
	require.Equal(t, []string{"name"}, childAttrs(res.Query.Children[0]))
	require.Nil(t, res.Query.Children[0].Filter)
	require.Equal(t, `(eq "a")`, res.Query.Children[1].Filter.debugString())
	require.Equal(t, `(neq "a" "b")`, res.Query.Children[2].Filter.debugString())
	require.Equal(t, `(namefilter "a")`, res.Query.Children[0].Children[0].Filter.debugString())
}

// Test operator precedence. && should be evaluated before ||.
func TestParseFilter_op(t *testing.T) {
	query := `
	query {
		me(id:0x0a) {
			friends @filter(a("a") || b("a") 
			&& c("a")) {
				name
			}
			gender,age
			hometown
		}
	}
`
	res, err := Parse(query)
	require.NoError(t, err)
	require.NotNil(t, res.Query)
	require.Equal(t, []string{"friends", "gender", "age", "hometown"}, childAttrs(res.Query))
	require.Equal(t, []string{"name"}, childAttrs(res.Query.Children[0]))
	require.Equal(t, `(OR (a "a") (AND (b "a") (c "a")))`, res.Query.Children[0].Filter.debugString())
}

// Test operator precedence. Let brackets make || evaluates before &&.
func TestParseFilter_op2(t *testing.T) {
	query := `
	query {
		me(id:0x0a) {
			friends @filter((a("a") || b("a"))
			 && c("a")) {
				name
			}
			gender,age
			hometown
		}
	}
`
	res, err := Parse(query)
	require.NoError(t, err)
	require.NotNil(t, res.Query)
	require.Equal(t, []string{"friends", "gender", "age", "hometown"}, childAttrs(res.Query))
	require.Equal(t, []string{"name"}, childAttrs(res.Query.Children[0]))
	require.Equal(t, `(AND (OR (a "a") (b "a")) (c "a"))`, res.Query.Children[0].Filter.debugString())
}

// Test operator precedence. More elaborate brackets.
func TestParseFilter_brac(t *testing.T) {
	query := `
	query {
		me(id:0x0a) {
			friends @filter(  a("hello") || b("world", "is") && (c("a") || (d("haha") || e("a"))) && f("a")){
				name
			}
			gender,age
			hometown
		}
	}
`
	res, err := Parse(query)
	require.NoError(t, err)
	require.NotNil(t, res.Query)
	require.Equal(t, []string{"friends", "gender", "age", "hometown"}, childAttrs(res.Query))
	require.Equal(t, []string{"name"}, childAttrs(res.Query.Children[0]))
	require.Equal(t,
		`(OR (a "hello") (AND (AND (b "world" "is") (OR (c "a") (OR (d "haha") (e "a")))) (f "a")))`,
		res.Query.Children[0].Filter.debugString())
}

// Test if unbalanced brac will lead to errors.
func TestParseFilter_unbalancedbrac(t *testing.T) {
	query := `
	query {
		me(id:0x0a) {
			friends @filter(  () {
				name
			}
			gender,age
			hometown
		}
	}
`
	_, err := Parse(query)
	require.Error(t, err)
}

func TestParseFilter_Geo1(t *testing.T) {
	query := `
	query {
		me(id:0x0a) {
			friends @filter(near(loc, [-1.12 , 2.0123 ], 100.123 )) {
				name
			}
			gender,age
			hometown
		}
	}
`
	_, err := Parse(query)
	require.NoError(t, err)
}

func TestParseFilter_Geo2(t *testing.T) {
	query := `
	query {
		me(id:0x0a) {
			friends @filter(within(loc, [[11.2 , -2.234 ], [ -31.23, 4.3214] , [5.312, 6.53]] )) {
				name
			}
			gender,age
			hometown
		}
	}
`
	_, err := Parse(query)
	require.NoError(t, err)
}

func TestParseFilter_Geo3(t *testing.T) {
	query := `
	query {
		me(id:0x0a) {
			friends @filter(near(loc, [[1 , 2 ], [[3, 4] , [5, 6]] )) {
				name
			}
			gender,age
			hometown
		}
	}
`
	_, err := Parse(query)
	require.Error(t, err)
}

func TestParseFilter_Geo4(t *testing.T) {
	query := `
	query {
		me(id:0x0a) {
			friends @filter(near(loc, [[1 , 2 ], [3, 4] , [5, 6]]] )) {
				name
			}
			gender,age
			hometown
		}
	}
`
	_, err := Parse(query)
	require.Error(t, err)
}

// Test if empty brackets will lead to errors.
func TestParseFilter_emptyargument(t *testing.T) {
	query := `
	query {
		me(id:0x0a) {
			friends @filter(allof(name,)) {
				name
			}
			gender,age
			hometown
		}
	}
`
	_, err := Parse(query)
	require.Error(t, err)
}
func TestParseFilter_unknowndirective(t *testing.T) {
	query := `
	query {
		me(id:0x0a) {
			friends @filtererr {
				name
			}
			gender,age
			hometown
		}`
	_, err := Parse(query)
	require.Error(t, err)
}

func TestParseGeneratorError(t *testing.T) {
	query := `{
		me(allof("name", "barack")) {
			friends {
				name
			}
			gender,age
			hometown
			count(friends)
		}
	}
`
	_, err := Parse(query)
	require.Error(t, err)
}

func TestParseCountAsFuncMultiple(t *testing.T) {
	schema.ParseBytes([]byte("scalar name:string @index"))
	query := `{
		me(id:1) {
			count(friends), count(relatives)
			count(classmates)
			gender,age
			hometown
		}
	}
`
	gq, err := Parse(query)
	require.NoError(t, err)
	require.Equal(t, 6, len(gq.Query.Children))
	require.Equal(t, true, gq.Query.Children[0].IsCount)
	require.Equal(t, "friends", gq.Query.Children[0].Attr)
	require.Equal(t, true, gq.Query.Children[1].IsCount)
	require.Equal(t, "relatives", gq.Query.Children[1].Attr)
	require.Equal(t, true, gq.Query.Children[2].IsCount)
	require.Equal(t, "classmates", gq.Query.Children[2].Attr)

}

func TestParseCountAsFuncMultipleError(t *testing.T) {
	schema.ParseBytes([]byte("scalar name:string @index"))
	query := `{
		me(id:1) {
			count(friends, relatives
			classmates)
			gender,age
			hometown
		}
	}
`
	_, err := Parse(query)
	require.Error(t, err)
}

func TestParseCountAsFunc(t *testing.T) {
	schema.ParseBytes([]byte("scalar name:string @index"))
	query := `{
		me(id:1) {
			count(friends)
			gender,age
			hometown
		}
	}
`
	gq, err := Parse(query)
	require.NoError(t, err)
	require.Equal(t, true, gq.Query.Children[0].IsCount)
	require.Equal(t, 4, len(gq.Query.Children))

}

func TestParseCountError1(t *testing.T) {
	schema.ParseBytes([]byte("scalar name:string @index"))
	query := `{
		me(id:1) {
			count(friends
			gender,age
			hometown
		}
	}
`
	_, err := Parse(query)
	require.Error(t, err)
}

func TestParseCountError2(t *testing.T) {
	schema.ParseBytes([]byte("scalar name:string @index"))
	query := `{
		me(id:1) {
			count((friends)
			gender,age
			hometown
		}
	}
`
	_, err := Parse(query)
	require.Error(t, err)
}

func TestParseGenerator(t *testing.T) {
	schema.ParseBytes([]byte("scalar name:string @index"))
	query := `{
		me(allof("name", "barack")) {
			friends {
				name
			}
			gender,age
			hometown
		}
	}
`
	_, err := Parse(query)
	require.NoError(t, err)
}

func TestParseGeoJson(t *testing.T) {
	query := `
	mutation {
		set {
			<_uid_:1> <loc> "{
				\'Type\':\'Point\' , 
				\'Coordinates\':[1.1,2.0]
			}"^^<geo:geojson> . 
		}
	}
	`
	_, err := Parse(query)
	require.NoError(t, err)
}

func TestMutationOpenBrace(t *testing.T) {
	query := `
	mutation {
		set {
			<m.0jx79w>  <type.object.name>  "Emma Miller {documentary actor)"@en .
		}
	}
	`
	_, err := Parse(query)
	require.NoError(t, err)
}

func TestMutationCloseBrace(t *testing.T) {
	query := `
	mutation {
		set {
			<m.0jx79w>  <type.object.name>  "Emma Miller }documentary actor)"@en .
		}
	}
	`
	_, err := Parse(query)
	require.NoError(t, err)
}

func TestMutationOpenCloseBrace(t *testing.T) {
	query := `
	mutation {
		set {
			<m.0jx79w>  <type.object.name>  "Emma Miller {documentary actor})"@en .
		}
	}
	`
	_, err := Parse(query)
	require.NoError(t, err)
}

func TestMutationQuotes(t *testing.T) {
	query := `
	mutation {
		set {
			<m.05vb159>  <type.object.name>  "\"Maison de Hoodle, Satoko Tachibana"@en  .
		}
	}
	`
	_, err := Parse(query)
	require.NoError(t, err)
}

func TestMutationSingleQuote(t *testing.T) {
	query := `
	mutation {
		set {
			_:gabe <name> "Gabe'
		}
	}
	`
	_, err := Parse(query)
	require.Error(t, err)
}
