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
		me(_uid_:0x0a) {
			friends {
				name
			}
			gender,age
			hometown
		}
	}
`
	gq, _, err := Parse(query)
	require.NoError(t, err)
	require.NotNil(t, gq)
	require.Equal(t, childAttrs(gq), []string{"friends", "gender", "age", "hometown"})
	require.Equal(t, childAttrs(gq.Children[0]), []string{"name"})
}

func TestParseError(t *testing.T) {
	query := `
		me(_uid_:0x0a) {
			friends {
				name
			}
			gender,age
			hometown
		}
	}
`
	_, _, err := Parse(query)
	require.Error(t, err)
}

func TestParseXid(t *testing.T) {
	// logrus.SetLevel(logrus.DebugLevel)
	// TODO: Why does the query not have _xid_ attribute?
	query := `
	query {
		user(_uid_: 0x11) {
			type.object.name
		}
	}`
	gq, _, err := Parse(query)
	require.NoError(t, err)
	require.NotNil(t, gq)
	require.Equal(t, childAttrs(gq), []string{"type.object.name"})
}

func TestParseFirst(t *testing.T) {
	query := `
	query {
		user(_xid_: m.abcd) {
			type.object.name
			friends (first: 10) {
			}
		}
	}`
	gq, _, err := Parse(query)
	require.NoError(t, err)
	require.NotNil(t, gq)
	require.Equal(t, childAttrs(gq), []string{"type.object.name", "friends"})
	require.Equal(t, gq.Children[1].Args["first"], "10")
}

func TestParseFirst_error(t *testing.T) {
	query := `
	query {
		user(_xid_: m.abcd) {
			type.object.name
			friends (first: ) {
			}
		}
	}`
	_, _, err := Parse(query)
	require.Error(t, err)
}

func TestParseAfter(t *testing.T) {
	query := `
	query {
		user(_xid_: m.abcd) {
			type.object.name
			friends (first: 10, after: 3) {
			}
		}
	}`
	gq, _, err := Parse(query)
	require.NoError(t, err)
	require.NotNil(t, gq)
	require.Equal(t, childAttrs(gq), []string{"type.object.name", "friends"})
	require.Equal(t, gq.Children[1].Args["first"], "10")
	require.Equal(t, gq.Children[1].Args["after"], "3")
}

func TestParseOffset(t *testing.T) {
	query := `
	query {
		user(_xid_: m.abcd) {
			type.object.name
			friends (first: 10, offset: 3) {
			}
		}
	}`
	gq, _, err := Parse(query)
	require.NoError(t, err)
	require.NotNil(t, gq)
	require.Equal(t, childAttrs(gq), []string{"type.object.name", "friends"})
	require.Equal(t, gq.Children[1].Args["first"], "10")
	require.Equal(t, gq.Children[1].Args["offset"], "3")
}

func TestParseOffset_error(t *testing.T) {
	query := `
	query {
		user(_xid_: m.abcd) {
			type.object.name
			friends (first: 10, offset: ) {
			}
		}
	}`
	_, _, err := Parse(query)
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
	_, _, err := Parse(query)
	require.Error(t, err)
}

func TestParse_pass1(t *testing.T) {
	query := `
		{
			me(_uid_:0x0a) {
				name,
				friends(xid:what) {  # xid would be ignored.
				}
			}
		}
	`
	gq, _, err := Parse(query)
	require.NoError(t, err)
	require.NotNil(t, gq)
	require.Equal(t, childAttrs(gq), []string{"name", "friends"})
	require.Empty(t, childAttrs(gq.Children[1]))
}

func TestParse_alias(t *testing.T) {
	query := `
		{
			me(_uid_:0x0a) {
				name,
				bestFriend: friends(first: 10) { 
					name	
				}
			}
		}
	`
	gq, _, err := Parse(query)
	require.NoError(t, err)
	require.NotNil(t, gq)
	require.Equal(t, childAttrs(gq), []string{"name", "friends"})
	require.Equal(t, gq.Children[1].Alias, "bestFriend")
	require.Equal(t, childAttrs(gq.Children[1]), []string{"name"})
}

func TestParse_block(t *testing.T) {
	query := `
		{
			root(_uid_: 0x0a) {
				type.object.name.es-419
			}
		}
	`
	gq, _, err := Parse(query)
	require.NoError(t, err)
	require.NotNil(t, gq)
	require.Equal(t, childAttrs(gq), []string{"type.object.name.es-419"})
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
	_, mu, err := Parse(query)
	require.NoError(t, err)
	require.NotEqual(t, strings.Index(mu.Set, "<name> <is> <something> ."), -1)
	require.NotEqual(t, strings.Index(mu.Set, "<hometown> <is> <san francisco> ."), -1)
	require.NotEqual(t, strings.Index(mu.Del, "<name> <is> <something-else> ."), -1)
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
	_, _, err := Parse(query)
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
	_, _, err := Parse(query)
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
			me(_xid_: tomhanks) {
				name
				hometown
			}
		}
	`
	gq, mu, err := Parse(query)
	require.NoError(t, err)
	require.NotNil(t, mu)
	require.NotEqual(t, strings.Index(mu.Set, "<name> <is> <something> ."), -1)
	require.NotEqual(t, strings.Index(mu.Set, "<hometown> <is> <san francisco> ."), -1)
	require.NotEqual(t, strings.Index(mu.Del, "<name> <is> <something-else> ."), -1)

	require.NotNil(t, gq)
	require.Equal(t, gq.XID, "tomhanks")
	require.Equal(t, childAttrs(gq), []string{"name", "hometown"})
}

func TestParseFragmentNoNesting(t *testing.T) {
	query := `
	query {
		user(_uid_:0x0a) {
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
	gq, _, err := Parse(query)
	require.NoError(t, err)
	require.NotNil(t, gq)
	require.Equal(t, childAttrs(gq), []string{"name", "id", "friends", "name", "hobbies", "id"})
}

func TestParseFragmentNest1(t *testing.T) {
	query := `
	query {
		user(_uid_:0x0a) {
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
	gq, _, err := Parse(query)
	require.NoError(t, err)
	require.NotNil(t, gq)
	require.Equal(t, childAttrs(gq), []string{"id", "hobbies", "friends"})
}

func TestParseFragmentNest2(t *testing.T) {
	query := `
	query {
		user(_uid_:0x0a) {
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
	gq, _, err := Parse(query)
	require.NoError(t, err)
	require.NotNil(t, gq)
	require.Equal(t, childAttrs(gq), []string{"friends"})
	require.Equal(t, childAttrs(gq.Children[0]), []string{"name", "nickname"})
}

func TestParseFragmentCycle(t *testing.T) {
	query := `
	query {
		user(_uid_:0x0a) {
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
	_, _, err := Parse(query)
	require.Error(t, err, "Expected error with cycle")
}

func TestParseFragmentMissing(t *testing.T) {
	query := `
	query {
		user(_uid_:0x0a) {
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
	_, _, err := Parse(query)
	require.Error(t, err, "Expected error with missing fragment")
}

func TestParseVariables(t *testing.T) {
	query := `{
		"query": "query testQuery( $a  : int   , $b: int){root(_uid_: 0x0a) {name(first: $b, after: $a){english}}}", 
		"variables": {"$a": "6", "$b": "5" } 
	}`
	_, _, err := Parse(query)
	require.NoError(t, err)
}

func TestParseVariables1(t *testing.T) {
	query := `{
		"query": "query testQuery($a: int , $b: int!){root(_uid_: 0x0a) {name(first: $b){english}}}", 
		"variables": {"$b": "5" } 
	}`
	_, _, err := Parse(query)
	require.NoError(t, err)
}

func TestParseVariables2(t *testing.T) {
	query := `{
		"query": "query testQuery($a: float , $b: bool!){root(_uid_: 0x0a) {name{english}}}", 
		"variables": {"$b": "false", "$a": "3.33" } 
	}`
	_, _, err := Parse(query)
	require.NoError(t, err)
}

func TestParseVariables3(t *testing.T) {
	query := `{
		"query": "query testQuery($a: int , $b: int! ){root(_uid_: 0x0a) {name(first: $b){english}}}", 
		"variables": {"$a": "5", "$b": "3"} 
	}`
	_, _, err := Parse(query)
	require.NoError(t, err)
}

func TestParseVariablesStringfiedJSON(t *testing.T) {
	query := `{
		"query": "query testQuery($a: int! , $b: int){root(_uid_: 0x0a) {name(first: $b){english}}}", 
		"variables": "{\"$a\": \"5\" }" 
	}`
	_, _, err := Parse(query)
	require.NoError(t, err)
}

func TestParseVariablesDefault1(t *testing.T) {
	query := `{
		"query": "query testQuery($a: int = 3  , $b: int =  4 ,  $c : int = 3){root(_uid_: 0x0a) {name(first: $b, after: $a, offset: $c){english}}}", 
		"variables": {"$b": "5" } 
	}`
	_, _, err := Parse(query)
	require.NoError(t, err)
}
func TestParseVariablesFragments(t *testing.T) {
	query := `{
	"query": "query test($a: int){user(_uid_:0x0a) {...fragmentd,friends(first: $a, offset: $a) {name}}} fragment fragmentd {id(first: $a)}",
	"variables": {"$a": "5"}
}`
	gq, _, err := Parse(query)
	require.NoError(t, err)
	require.NotNil(t, gq)
	require.Equal(t, childAttrs(gq), []string{"id", "friends"})
	require.Empty(t, childAttrs(gq.Children[0]))
	require.Equal(t, childAttrs(gq.Children[1]), []string{"name"})
	require.Equal(t, gq.Children[0].Args["first"], "5")
}

func TestParseVariablesError1(t *testing.T) {
	query := `
	query testQuery($a: string, $b: int!){
			root(_uid_: 0x0a) {
				type.object.name.es-419
			}
		}
	`
	_, _, err := Parse(query)
	require.Error(t, err)
}

func TestParseVariablesError2(t *testing.T) {
	query := `{
		"query": "query testQuery($a: int, $b: int, $c: int!){
			root(_uid_: 0x0a) {name(first: $b, after: $a){english}}
		}", 
		"variables": {"$a": "6", "$b": "5" } 
	}`
	_, _, err := Parse(query)
	require.Error(t, err, "Expected value for variable $c")
}

func TestParseVariablesError3(t *testing.T) {
	query := `{
		"query": "query testQuery($a: int, $b: , $c: int!){
			root(_uid_: 0x0a) {name(first: $b, after: $a){english}}
		}", 
		"variables": {"$a": "6", "$b": "5" } 
	}`
	_, _, err := Parse(query)
	require.Error(t, err, "Expected type for variable $b")
}

func TestParseVariablesError4(t *testing.T) {
	query := `{
		"query": "query testQuery($a: bool , $b: float! = 3){root(_uid_: 0x0a) {name(first: $b){english}}}", 
		"variables": {"$a": "5" } 
	}`
	_, _, err := Parse(query)
	require.Error(t, err, "Expected type error")
}

func TestParseVariablesError5(t *testing.T) {
	query := `{
		"query": "query ($a: int, $b: int){root(_uid_: 0x0a) {name(first: $b, after: $a){english}}}", 
		"variables": {"$a": "6", "$b": "5" } 
	}`
	_, _, err := Parse(query)
	require.Error(t, err, "Expected error: Query with variables should be named")
}

func TestParseVariablesError6(t *testing.T) {
	query := `{
		"query": "query ($a: int, $b: random){root(_uid_: 0x0a) {name(first: $b, after: $a){english}}}", 
		"variables": {"$a": "6", "$b": "5" } 
	}`
	_, _, err := Parse(query)
	require.Error(t, err, "Expected error: Type random not supported")
}

func TestParseVariablesError7(t *testing.T) {
	query := `{
		"query": "query testQuery($a: int, $b: int, $c: int!){
			root(_uid_: 0x0a) {name(first: $b, after: $a){english}}
		}", 
		"variables": {"$a": "6", "$b": "5", "$d": "abc" } 
	}`
	_, _, err := Parse(query)
	require.Error(t, err, "Expected type for variable $d")
}

func TestParseVariablesiError8(t *testing.T) {
	query := `{
		"query": "query testQuery($a: int = 3  , $b: int! =  4 ,  $c : int = 3){root(_uid_: 0x0a) {name(first: $b, after: $a, offset: $c){english}}}", 
		"variables": {"$b": "5" } 
	}`
	_, _, err := Parse(query)
	require.Error(t, err, "Variables type ending with ! cant have default value")
}

func TestParseFilter_simplest(t *testing.T) {
	query := `
	query {
		me(_uid_:0x0a) {
			friends @filter() {
				name @filter(namefilter())
			}
			gender @filter(eq()),age @filter(neq("a", "b"))
			hometown
		}
	}
`
	gq, _, err := Parse(query)
	require.NoError(t, err)
	require.NotNil(t, gq)
	require.Equal(t, childAttrs(gq), []string{"friends", "gender", "age", "hometown"})
	require.Equal(t, childAttrs(gq.Children[0]), []string{"name"})
	require.Nil(t, gq.Children[0].Filter)
	require.Equal(t, gq.Children[1].Filter.debugString(), `(eq)`)
	require.Equal(t, gq.Children[2].Filter.debugString(), `(neq "a" "b")`)
	require.Equal(t, gq.Children[0].Children[0].Filter.debugString(), "(namefilter)")
}

// Test operator precedence. && should be evaluated before ||.
func TestParseFilter_op(t *testing.T) {
	query := `
	query {
		me(_uid_:0x0a) {
			friends @filter(a() || b() 
			&& c()) {
				name
			}
			gender,age
			hometown
		}
	}
`
	gq, _, err := Parse(query)
	require.NoError(t, err)
	require.NotNil(t, gq)
	require.Equal(t, childAttrs(gq), []string{"friends", "gender", "age", "hometown"})
	require.Equal(t, childAttrs(gq.Children[0]), []string{"name"})
	require.Equal(t, gq.Children[0].Filter.debugString(), `(OR (a) (AND (b) (c)))`)
}

// Test operator precedence. Let brackets make || evaluates before &&.
func TestParseFilter_op2(t *testing.T) {
	query := `
	query {
		me(_uid_:0x0a) {
			friends @filter((a() || b())
			 && c()) {
				name
			}
			gender,age
			hometown
		}
	}
`
	gq, _, err := Parse(query)
	require.NoError(t, err)
	require.NotNil(t, gq)
	require.Equal(t, childAttrs(gq), []string{"friends", "gender", "age", "hometown"})
	require.Equal(t, childAttrs(gq.Children[0]), []string{"name"})
	require.Equal(t, gq.Children[0].Filter.debugString(), `(AND (OR (a) (b)) (c))`)
}

// Test operator precedence. More elaborate brackets.
func TestParseFilter_brac(t *testing.T) {
	query := `
	query {
		me(_uid_:0x0a) {
			friends @filter(  a("hello") || b("world", "is") && (c() || (d("haha") || e())) && f()){
				name
			}
			gender,age
			hometown
		}
	}
`
	gq, _, err := Parse(query)
	require.NoError(t, err)
	require.NotNil(t, gq)
	require.Equal(t, childAttrs(gq), []string{"friends", "gender", "age", "hometown"})
	require.Equal(t, childAttrs(gq.Children[0]), []string{"name"})
	require.Equal(t, gq.Children[0].Filter.debugString(),
		`(OR (a "hello") (AND (AND (b "world" "is") (OR (c) (OR (d "haha") (e)))) (f)))`)
}

// Test if unbalanced brac will lead to errors.
func TestParseFilter_unbalancedbrac(t *testing.T) {
	query := `
	query {
		me(_uid_:0x0a) {
			friends @filter(  ()
				name
			}
			gender,age
			hometown
		}
	}
`
	_, _, err := Parse(query)
	require.Error(t, err)
}

func TestParseFilter_unknowndirective(t *testing.T) {
	query := `
	query {
		me(_uid_:0x0a) {
			friends @filtererr
				name
			}
			gender,age
			hometown
		}
	}
`
	_, _, err := Parse(query)
	require.Error(t, err)
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
	_, _, err := Parse(query)
	require.NoError(t, err)
}
