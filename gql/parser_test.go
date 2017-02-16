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

func TestParseQueryWithVarMultiRoot(t *testing.T) {
	query := `
	{	
		me(var:[L, J, K]) {name}
		var(id:0x0a) {L AS friends}
		var(id:0x0a) {J AS friends}
		var(id:0x0a) {K AS friends}
	}
`
	res, err := Parse(query)
	require.NoError(t, err)
	require.NotNil(t, res.Query)
	require.Equal(t, 4, len(res.Query))
	require.Equal(t, "L", res.Query[0].NeedsVar[0])
	require.Equal(t, "J", res.Query[0].NeedsVar[1])
	require.Equal(t, "K", res.Query[0].NeedsVar[2])
	require.Equal(t, "L", res.Query[1].Children[0].Var)
	require.Equal(t, "J", res.Query[2].Children[0].Var)
	require.Equal(t, "K", res.Query[3].Children[0].Var)
}

func TestParseQueryWithVar(t *testing.T) {
	query := `
	{	
		me(var:L) {name}
		him(var:J) {name}
		you(var:K) {name}
		var(id:0x0a) {L AS friends}
		var(id:0x0a) {J AS friends}
		var(id:0x0a) {K AS friends}
	}
`
	res, err := Parse(query)
	require.NoError(t, err)
	require.NotNil(t, res.Query)
	require.Equal(t, 6, len(res.Query))
	require.Equal(t, "L", res.Query[0].NeedsVar[0])
	require.Equal(t, "J", res.Query[1].NeedsVar[0])
	require.Equal(t, "K", res.Query[2].NeedsVar[0])
	require.Equal(t, "L", res.Query[3].Children[0].Var)
	require.Equal(t, "J", res.Query[4].Children[0].Var)
	require.Equal(t, "K", res.Query[5].Children[0].Var)
}

func TestParseQueryWithVarError1(t *testing.T) {
	query := `
	{	
		him(var:J) {name}
		you(var:K) {name}
		var(id:0x0a) {L AS friends}
		var(id:0x0a) {J AS friends}
		var(id:0x0a) {K AS friends}
	}
`
	_, err := Parse(query)
	require.Error(t, err)
}

func TestParseQueryWithVarError2(t *testing.T) {
	query := `
	{	
		me(var:L) {name}
		him(var:J) {name}	
		you(var:K) {name}
		var(id:0x0a) {L AS friends}
		var(id:0x0a) {K AS friends}
	}
`
	_, err := Parse(query)
	require.Error(t, err)
}

func TestParseQueryWithVarAtRootFilterID(t *testing.T) {
	query := `
	{
		K as var(id:0x0a) {
			L AS friends
		}
		me(var:K) @filter(id(L)) {
		 name	
		}
	}
`
	res, err := Parse(query)
	require.NoError(t, err)
	require.NotNil(t, res.Query)
	require.Equal(t, 2, len(res.Query))
	require.Equal(t, "K", res.Query[0].Var)
	require.Equal(t, "L", res.Query[0].Children[0].Var)
	require.Equal(t, "L", res.Query[1].Filter.Func.NeedsVar[0])
	require.Equal(t, []string{"K", "L"}, res.QueryVars[0].Defines)
}

func TestParseQueryWithVarAtRoot(t *testing.T) {
	query := `
	{
		K AS var(id:0x0a) {
			fr as friends
		}
		me(var:fr) @filter(id(K)) {
		 name	@filter(id(fr))
		}
	}
`
	res, err := Parse(query)
	require.NoError(t, err)
	require.NotNil(t, res.Query)
	require.Equal(t, 2, len(res.Query))
	require.Equal(t, "K", res.Query[0].Var)
	require.Equal(t, "fr", res.Query[0].Children[0].Var)
	require.Equal(t, "fr", res.Query[1].NeedsVar[0])
	require.Equal(t, []string{"K", "fr"}, res.QueryVars[0].Defines)
}

func TestParseQueryWithVar1(t *testing.T) {
	query := `
	{
		var(id:0x0a) {
			L AS friends
		}
	
		me(var:L) {
		 name	
		}
	}
`
	res, err := Parse(query)
	require.NoError(t, err)
	require.NotNil(t, res.Query)
	require.Equal(t, 2, len(res.Query))
	require.Equal(t, "L", res.Query[0].Children[0].Var)
	require.Equal(t, "L", res.Query[1].NeedsVar[0])
}

func TestParseQueryWithMultipleVar(t *testing.T) {
	query := `
	{
		var(id:0x0a) {
			L AS friends {
				B AS relatives
			}
		}
	
		me(var:L) {
		 name	
		}

		relatives(var:B) {
			name
		}
	}
`
	res, err := Parse(query)
	require.NoError(t, err)
	require.NotNil(t, res.Query)
	require.Equal(t, 3, len(res.Query))
	require.Equal(t, "L", res.Query[0].Children[0].Var)
	require.Equal(t, "B", res.Query[0].Children[0].Children[0].Var)
	require.Equal(t, "L", res.Query[1].NeedsVar[0])
	require.Equal(t, "B", res.Query[2].NeedsVar[0])
	require.Equal(t, []string{"L", "B"}, res.QueryVars[0].Defines)
	require.Equal(t, []string{"L"}, res.QueryVars[1].Needs)
	require.Equal(t, []string{"B"}, res.QueryVars[2].Needs)
}

func TestParseShortestPath(t *testing.T) {
	query := `
	{
		shortest(from:0x0a, to:0x0b) {
			friends
			name
		}
	}
`
	res, err := Parse(query)
	require.NoError(t, err)
	require.NotNil(t, res.Query)
	require.Equal(t, 1, len(res.Query))
	require.Equal(t, "0x0a", res.Query[0].Args["from"])
	require.Equal(t, "0x0b", res.Query[0].Args["to"])
}

func TestParseMultipleQueries(t *testing.T) {
	query := `
	{
		you(id:0x0a) {
			name
		}

		me(id:0x0b) {
		 friends
		}
	}
`
	res, err := Parse(query)
	require.NoError(t, err)
	require.NotNil(t, res.Query)
	require.Equal(t, 2, len(res.Query))
}

func TestParseRootArgs1(t *testing.T) {
	query := `
	query {
		me(id:0x0a, first:4, offset:1) {
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
	require.Equal(t, 1, len(res.Query))
	require.Equal(t, 2, len(res.Query[0].Args))
	require.Equal(t, "4", res.Query[0].Args["first"])
	require.Equal(t, "1", res.Query[0].Args["offset"])
	require.Equal(t, childAttrs(res.Query[0]), []string{"friends", "gender", "age", "hometown"})
	require.Equal(t, childAttrs(res.Query[0].Children[0]), []string{"name"})
}

func TestParseRootArgs2(t *testing.T) {
	query := `
	query {
		me(id:0x0a, first: 1, offset:0) {
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
	require.Equal(t, 1, len(res.Query))
	require.Equal(t, childAttrs(res.Query[0]), []string{"friends", "gender", "age", "hometown"})
	require.Equal(t, childAttrs(res.Query[0].Children[0]), []string{"name"})
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
	require.NotNil(t, res.Query[0])
	require.Equal(t, childAttrs(res.Query[0]), []string{"type.object.name"})
}

func TestParseIdList(t *testing.T) {
	query := `
	query {
		user(id: [0x1]) {
			type.object.name
		}
	}`
	r, err := Parse(query)
	gq := r.Query[0]
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
	gq := r.Query[0]
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
	require.NotNil(t, res.Query[0])
	require.Equal(t, []string{"type.object.name", "friends"}, childAttrs(res.Query[0]))
	require.Equal(t, "10", res.Query[0].Children[1].Args["first"])
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
	require.NotNil(t, res.Query[0])
	require.Equal(t, childAttrs(res.Query[0]), []string{"type.object.name", "friends"})
	require.Equal(t, res.Query[0].Children[1].Args["first"], "10")
	require.Equal(t, res.Query[0].Children[1].Args["after"], "3")
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
	require.NotNil(t, res.Query[0])
	require.Equal(t, childAttrs(res.Query[0]), []string{"type.object.name", "friends"})
	require.Equal(t, res.Query[0].Children[1].Args["first"], "10")
	require.Equal(t, res.Query[0].Children[1].Args["offset"], "3")
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
	require.NotNil(t, res.Query[0])
	require.Equal(t, childAttrs(res.Query[0]), []string{"name", "friends"})
	require.Empty(t, childAttrs(res.Query[0].Children[1]))
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
	require.NotNil(t, res.Query[0])
	require.Equal(t, childAttrs(res.Query[0]), []string{"name", "friends"})
	require.Equal(t, res.Query[0].Children[1].Alias, "bestFriend")
	require.Equal(t, childAttrs(res.Query[0].Children[1]), []string{"name"})
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
	require.NotNil(t, res.Query[0])
	require.Equal(t, childAttrs(res.Query[0]), []string{"type.object.name.en", "friends"})
	require.Equal(t, res.Query[0].Children[1].Alias, "bestFriend")
	require.Equal(t, res.Query[0].Children[1].Children[0].Alias, "name")
	require.Equal(t, childAttrs(res.Query[0].Children[1]), []string{"type.object.name.hi"})
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
	require.NotNil(t, res.Query[0])
	require.Equal(t, childAttrs(res.Query[0]), []string{"type.object.name.es-419"})
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

func TestParseMutationAndQueryWithComments(t *testing.T) {
	query := `
	# Mutation
		mutation {
			# Set block
			set {
				<name> <is> <something> .
				<hometown> <is> <san francisco> .
			}
			# Delete block
			delete {
				<name> <is> <something-else> .
			}
		}
		# Query starts here.
		query {
			me(id: tomhanks) { # now mention children
				name		# Name
				hometown # hometown of the person
			}
		}
	`
	res, err := Parse(query)
	require.NoError(t, err)
	require.NotNil(t, res.Mutation)
	require.NotEqual(t, strings.Index(res.Mutation.Set, "<name> <is> <something> ."), -1)
	require.NotEqual(t, strings.Index(res.Mutation.Set, "<hometown> <is> <san francisco> ."), -1)
	require.NotEqual(t, strings.Index(res.Mutation.Del, "<name> <is> <something-else> ."), -1)

	require.NotNil(t, res.Query[0])
	require.Equal(t, 1, len(res.Query[0].UID))
	require.Equal(t, childAttrs(res.Query[0]), []string{"name", "hometown"})
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

	require.NotNil(t, res.Query[0])
	require.Equal(t, 1, len(res.Query[0].UID))
	require.Equal(t, childAttrs(res.Query[0]), []string{"name", "hometown"})
}

func TestParseFragmentMultiQuery(t *testing.T) {
	query := `
	{
		user(id:0x0a) {
			...fragmenta,...fragmentb
			friends {
				name
			}
			...fragmentc
			hobbies
			...fragmentd
		}

		me(id:0x01) {
			...fragmenta
			...fragmentb
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
	require.NotNil(t, res.Query[0])
	require.Equal(t, []string{"name", "id", "friends", "name", "hobbies", "id"}, childAttrs(res.Query[0]))
	require.Equal(t, []string{"name", "id"}, childAttrs(res.Query[1]))
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
	require.NotNil(t, res.Query[0])
	require.Equal(t, childAttrs(res.Query[0]), []string{"name", "id", "friends", "name", "hobbies", "id"})
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
	require.NotNil(t, res.Query[0])
	require.Equal(t, childAttrs(res.Query[0]), []string{"id", "hobbies", "friends"})
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
	require.NotNil(t, res.Query[0])
	require.Equal(t, childAttrs(res.Query[0]), []string{"friends"})
	require.Equal(t, childAttrs(res.Query[0].Children[0]), []string{"name", "nickname"})
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
	require.NotNil(t, res.Query[0])
	require.Equal(t, childAttrs(res.Query[0]), []string{"id", "friends"})
	require.Empty(t, childAttrs(res.Query[0].Children[0]))
	require.Equal(t, childAttrs(res.Query[0].Children[1]), []string{"name"})
	require.Equal(t, res.Query[0].Children[0].Args["first"], "5")
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
		me(func:abc(abc)) @filter(allof(name, "alice")) {
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
	require.NotNil(t, res.Query[0])
	require.NotNil(t, res.Query[0].Filter)
	require.Equal(t, `(allof "name" "alice")`, res.Query[0].Filter.debugString())
	require.Equal(t, []string{"friends", "gender", "age", "hometown"}, childAttrs(res.Query[0]))
	require.Equal(t, []string{"name"}, childAttrs(res.Query[0].Children[0]))
	require.Nil(t, res.Query[0].Children[0].Filter)
	require.Equal(t, `(eq "a")`, res.Query[0].Children[1].Filter.debugString())
	require.Equal(t, `(neq "a" "b")`, res.Query[0].Children[2].Filter.debugString())
	require.Equal(t, `(namefilter "a")`, res.Query[0].Children[0].Children[0].Filter.debugString())
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
	require.NotNil(t, res.Query[0])
	require.Equal(t, []string{"friends", "gender", "age", "hometown"}, childAttrs(res.Query[0]))
	require.Equal(t, []string{"name"}, childAttrs(res.Query[0].Children[0]))
	require.Nil(t, res.Query[0].Children[0].Filter)
	require.Equal(t, `(eq "a")`, res.Query[0].Children[1].Filter.debugString())
	require.Equal(t, `(neq "a" "b")`, res.Query[0].Children[2].Filter.debugString())
	require.Equal(t, `(namefilter "a")`, res.Query[0].Children[0].Children[0].Filter.debugString())
}

// Test operator precedence. and should be evaluated before or.
func TestParseFilter_op(t *testing.T) {
	query := `
	query {
		me(id:0x0a) {
			friends @filter(a("a") or b("a")
			and c("a")) {
				name
			}
			gender,age
			hometown
		}
	}
`
	res, err := Parse(query)
	require.NoError(t, err)
	require.NotNil(t, res.Query[0])
	require.Equal(t, []string{"friends", "gender", "age", "hometown"}, childAttrs(res.Query[0]))
	require.Equal(t, []string{"name"}, childAttrs(res.Query[0].Children[0]))
	require.Equal(t, `(OR (a "a") (AND (b "a") (c "a")))`, res.Query[0].Children[0].Filter.debugString())
}

func TestParseFilter_opError(t *testing.T) {
	query := `
	query {
		me(id:0x0a) {
			friends @filter(a("a") or b("a")
			and ) {
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

func TestParseFilter_opNot1(t *testing.T) {
	query := `
	query {
		me(id:0x0a) {
			friends @filter(not a("a")) {
				name
			}
			gender,age
			hometown
		}
	}
`
	res, err := Parse(query)
	require.NoError(t, err)
	require.NotNil(t, res.Query[0])
	require.Equal(t, []string{"friends", "gender", "age", "hometown"}, childAttrs(res.Query[0]))
	require.Equal(t, []string{"name"}, childAttrs(res.Query[0].Children[0]))
	require.Equal(t, `(NOT (a "a"))`, res.Query[0].Children[0].Filter.debugString())
}

func TestParseFilter_opNot2(t *testing.T) {
	query := `
	query {
		me(id:0x0a) {
			friends @filter(not(a("a") or (b("a"))) and c("a")) {
				name
			}
			gender,age
			hometown
		}
	}
`
	res, err := Parse(query)
	require.NoError(t, err)
	require.NotNil(t, res.Query[0])
	require.Equal(t, []string{"friends", "gender", "age", "hometown"}, childAttrs(res.Query[0]))
	require.Equal(t, []string{"name"}, childAttrs(res.Query[0].Children[0]))
	require.Equal(t, `(AND (NOT (OR (a "a") (b "a"))) (c "a"))`, res.Query[0].Children[0].Filter.debugString())
}

// Test operator precedence. Let brackets make or evaluates before and.
func TestParseFilter_op2(t *testing.T) {
	query := `
	query {
		me(id:0x0a) {
			friends @filter((a("a") Or b("a"))
			 and c("a")) {
				name
			}
			gender,age
			hometown
		}
	}
`
	res, err := Parse(query)
	require.NoError(t, err)
	require.NotNil(t, res.Query[0])
	require.Equal(t, []string{"friends", "gender", "age", "hometown"}, childAttrs(res.Query[0]))
	require.Equal(t, []string{"name"}, childAttrs(res.Query[0].Children[0]))
	require.Equal(t, `(AND (OR (a "a") (b "a")) (c "a"))`, res.Query[0].Children[0].Filter.debugString())
}

// Test operator precedence. More elaborate brackets.
func TestParseFilter_brac(t *testing.T) {
	query := `
	query {
		me(id:0x0a) {
			friends @filter(  a("hello") or b("world", "is") and (c("a") or (d("haha") or e("a"))) and f("a")){
				name
			}
			gender,age
			hometown
		}
	}
`
	res, err := Parse(query)
	require.NoError(t, err)
	require.NotNil(t, res.Query[0])
	require.Equal(t, []string{"friends", "gender", "age", "hometown"}, childAttrs(res.Query[0]))
	require.Equal(t, []string{"name"}, childAttrs(res.Query[0].Children[0]))
	require.Equal(t,
		`(OR (a "hello") (AND (AND (b "world" "is") (OR (c "a") (OR (d "haha") (e "a")))) (f "a")))`,
		res.Query[0].Children[0].Filter.debugString())
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
			friends @filter(allof(name,,)) {
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
	require.Equal(t, 6, len(gq.Query[0].Children))
	require.Equal(t, true, gq.Query[0].Children[0].IsCount)
	require.Equal(t, "friends", gq.Query[0].Children[0].Attr)
	require.Equal(t, true, gq.Query[0].Children[1].IsCount)
	require.Equal(t, "relatives", gq.Query[0].Children[1].Attr)
	require.Equal(t, true, gq.Query[0].Children[2].IsCount)
	require.Equal(t, "classmates", gq.Query[0].Children[2].Attr)
}

func TestParseCountAsFuncMultipleError(t *testing.T) {
	require.NoError(t, schema.ParseBytes([]byte("scalar name:string @index")))
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
	require.Equal(t, true, gq.Query[0].Children[0].IsCount)
	require.Equal(t, 4, len(gq.Query[0].Children))

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

func TestParseCheckPwd(t *testing.T) {
	schema.ParseBytes([]byte("scalar name:string @index"))
	query := `{
		me(id:1) {
			checkpwd("123456")
			hometown
		}
	}
`
	gq, err := Parse(query)
	require.NoError(t, err)
	require.Equal(t, "checkpwd", gq.Query[0].Children[0].Func.Name)
	require.Equal(t, "123456", gq.Query[0].Children[0].Func.Args[0])
	require.Equal(t, "password", gq.Query[0].Children[0].Attr)
}

func TestParseComments(t *testing.T) {
	schema.ParseBytes([]byte("scalar name:string @index"))
	query := `
	# Something
	{
		me(func:allof("name", "barack")) {
			friends {
				name
			} # Something
			gender,age
			hometown
		}
	}
`
	_, err := Parse(query)
	require.NoError(t, err)
}

func TestParseComments1(t *testing.T) {
	schema.ParseBytes([]byte("scalar name:string @index"))
	query := `{
		#Something 
		me(func:allof("name", "barack")) {
			friends {
				name  # Name of my friend
			}
			gender,age
			hometown
		}
	}
`
	_, err := Parse(query)
	require.NoError(t, err)
}

func TestParseGenerator(t *testing.T) {
	schema.ParseBytes([]byte("scalar name:string @index"))
	query := `{
		me(func:allof("name", "barack")) {
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

func TestParseIRIRef(t *testing.T) {
	query := `{
		me(id: <http://helloworld.com/how/are/you>) {
			<http://verygood.com/what/about/you>
			friends @filter(allof(<http://verygood.com/what/about/you>,
				"good better bad")){
				name
			}
			gender,age
			hometown
		}
	}`

	gq, err := Parse(query)
	require.NoError(t, err)
	require.Equal(t, 5, len(gq.Query[0].Children))
	require.Equal(t, "http://verygood.com/what/about/you", gq.Query[0].Children[0].Attr)
	require.Equal(t, `(allof "http://verygood.com/what/about/you" "good better bad")`,
		gq.Query[0].Children[1].Filter.debugString())
}

func TestParseIRIRef2(t *testing.T) {
	require.NoError(t, schema.ParseBytes(
		[]byte("scalar <http://helloworld.com/how/are/you>:string @index")))
	query := `{
		me(func:anyof(<http://helloworld.com/how/are/you>, "good better bad")) {
			<http://verygood.com/what/about/you>
			friends @filter(allof(<http://verygood.com/what/about/you>,
				"good better bad")){
				name
			}
		}
	}`

	gq, err := Parse(query)
	require.NoError(t, err)
	require.Equal(t, 2, len(gq.Query[0].Children))
	require.Equal(t, "http://verygood.com/what/about/you", gq.Query[0].Children[0].Attr)
	require.Equal(t, `(allof "http://verygood.com/what/about/you" "good better bad")`,
		gq.Query[0].Children[1].Filter.debugString())
	require.Equal(t, "http://helloworld.com/how/are/you", gq.Query[0].Func.Attr)
}

func TestParseIRIRefSpace(t *testing.T) {
	query := `{
		me(id: <http://helloworld.com/how/are/ you>) {
		}
	      }`

	_, err := Parse(query)
	require.Error(t, err) // because of space.
}

func TestParseIRIRefInvalidChar(t *testing.T) {
	query := `{
		me(id: <http://helloworld.com/how/are/^you>) {
		}
	      }`

	_, err := Parse(query)
	require.Error(t, err) // because of ^
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

func TestMutationPassword(t *testing.T) {
	query := `
	mutation {
		set {
			<alice> <password> "PenAndPencil"^^<pwd:password>
		}
	}
	`
	_, err := Parse(query)
	require.NoError(t, err)
}

func TestLangs(t *testing.T) {
	query := `
	query {
		me(id:1) {
			name@en,name@en:ru:hu
		}
	}
	`

	gq, err := Parse(query)
	require.NoError(t, err)
	require.Equal(t, 2, len(gq.Query[0].Children))
	require.Equal(t, "name", gq.Query[0].Children[0].Attr)
	require.Equal(t, []string{"en"}, gq.Query[0].Children[0].Langs)
	require.Equal(t, "name", gq.Query[0].Children[1].Attr)
	require.Equal(t, []string{"en", "ru", "hu"}, gq.Query[0].Children[1].Langs)
}

func TestLangsInvalid1(t *testing.T) {
	query := `
	query {
		me(id:1) {
			name@en@ru
		}
	}
	`

	_, err := Parse(query)
	require.Error(t, err)
}

func TestLangsInvalid2(t *testing.T) {
	query := `
	query {
		me(id:1) {
			@en:ru
		}
	}
	`

	_, err := Parse(query)
	require.Error(t, err)
}

func TestLangsInvalid3(t *testing.T) {
	query := `
	query {
		me(id:1) {
			name@en:ru, @en:ru
		}
	}
	`

	_, err := Parse(query)
	require.Error(t, err)
}

func TestLangsInvalid4(t *testing.T) {
	query := `
	query {
		me(id:1) {
			name@
		}
	}
	`

	_, err := Parse(query)
	require.Error(t, err)
}

func TestLangsInvalid5(t *testing.T) {
	query := `
	query {
		me(id:1) {
			name@<something.wrong>
		}
	}
	`

	_, err := Parse(query)
	require.Error(t, err)
}

func TestParseNormalize(t *testing.T) {
	query := `
	query {
		me(id: abc) @normalize {
			friends {
				name
			}
			gender
			hometown
		}
}
`
	res, err := Parse(query)
	require.NoError(t, err)
	require.NotNil(t, res.Query[0])
	require.True(t, res.Query[0].Normalize)
}

func TestParseFacets(t *testing.T) {
	query := `
	query {
		me(id:0x1) {
			friends @facets {
				name @facets(facet1)
			}
			hometown
			age
		}
	}
`
	res, err := Parse(query)
	require.NoError(t, err)
	require.NotNil(t, res.Query[0])
	require.Equal(t, []string{"friends", "hometown", "age"}, childAttrs(res.Query[0]))
	require.NotNil(t, res.Query[0].Children[0].Facets)
	require.Equal(t, true, res.Query[0].Children[0].Facets.AllKeys)
	require.Equal(t, []string{"name"}, childAttrs(res.Query[0].Children[0]))
	require.NotNil(t, res.Query[0].Children[0].Children[0].Facets)
	require.Equal(t, false, res.Query[0].Children[0].Children[0].Facets.AllKeys)
	require.Equal(t, []string{"facet1"}, res.Query[0].Children[0].Children[0].Facets.Keys)
}

func TestParseFacetsMultiple(t *testing.T) {
	query := `
	query {
		me(id:0x1) {
			friends @facets {
				name @facets(key1, key2, key3)
			}
			hometown
			age
		}
	}
`
	res, err := Parse(query)
	require.NoError(t, err)
	require.NotNil(t, res.Query[0])
	require.Equal(t, []string{"friends", "hometown", "age"}, childAttrs(res.Query[0]))
	require.NotNil(t, res.Query[0].Children[0].Facets)
	require.Equal(t, true, res.Query[0].Children[0].Facets.AllKeys)
	require.Equal(t, []string{"name"}, childAttrs(res.Query[0].Children[0]))
	require.NotNil(t, res.Query[0].Children[0].Children[0].Facets)
	require.Equal(t, false, res.Query[0].Children[0].Children[0].Facets.AllKeys)
	require.Equal(t, []string{"key1", "key2", "key3"}, res.Query[0].Children[0].Children[0].Facets.Keys)
}

func TestParseFacetsEmpty(t *testing.T) {
	query := `
	query {
		me(id:0x1) {
			friends @facets() {
			}
			hometown
			age
		}
	}
`
	res, err := Parse(query)
	require.NoError(t, err)
	require.NotNil(t, res.Query[0])
	require.Equal(t, []string{"friends", "hometown", "age"}, childAttrs(res.Query[0]))
	require.NotNil(t, res.Query[0].Children[0].Facets)
	require.Equal(t, true, res.Query[0].Children[0].Facets.AllKeys)
}

func TestParseFacetsFail1(t *testing.T) {
	// key can not be empty..
	query := `
	query {
		me(id:0x1) {
			friends @facets(key1,, key2) {
			}
			hometown
			age
		}
	}
`
	_, err := Parse(query)
	require.Error(t, err)
}

func TestFacetsFilter(t *testing.T) {
	// all friends close to 0x1
	query := `
		{
			me(id:0x1) {
				name
				friend @filter(eq(@facets(close), true)) {
				       name
				       gender
				}
			}
		}
	`

	res, err := Parse(query)
	require.NoError(t, err)
	require.NotNil(t, res.Query[0])
	require.Equal(t, []string{"name", "friend"}, childAttrs(res.Query[0]))
	require.NotNil(t, res.Query[0].Children[1].Filter)
	require.NotNil(t, res.Query[0].Children[1].Filter.Func)
	require.Equal(t, "eq", res.Query[0].Children[1].Filter.Func.Name)
	require.Equal(t, "close", res.Query[0].Children[1].Filter.Func.Attr)
	require.Equal(t, true, res.Query[0].Children[1].Filter.Func.IsFacet)
	require.Equal(t, 1, len(res.Query[0].Children[1].Filter.Func.Args))
	require.Equal(t, "true", res.Query[0].Children[1].Filter.Func.Args[0])
}

func TestFacetsFilter2(t *testing.T) {
	// all male friends of 0x1 whose name has french origin.
	query := `
		{
			me(id:0x1) {
				name
				friend @filter(eq(@facets(close), true)) {
				       name @filter(eq(@facets(origin), "french"))
				       gender
				}
			}
		}
	`

	res, err := Parse(query)
	require.NoError(t, err)
	require.NotNil(t, res.Query[0])
	require.Equal(t, []string{"name", "friend"}, childAttrs(res.Query[0]))
	child1 := res.Query[0].Children[1]
	require.Equal(t, []string{"name", "gender"}, childAttrs(child1))
	require.NotNil(t, child1.Children[0].Filter)
	require.NotNil(t, child1.Children[0].Filter.Func)
	require.Equal(t, "eq", child1.Children[0].Filter.Func.Name)
	require.Equal(t, "origin", child1.Children[0].Filter.Func.Attr)
	require.Equal(t, true, child1.Children[0].Filter.Func.IsFacet)
	require.Equal(t, 1, len(child1.Children[0].Filter.Func.Args))
	require.Equal(t, "french", child1.Children[0].Filter.Func.Args[0])
}

func TestFacetsFilterFail(t *testing.T) {
	// @facets() not allowed - should have some facet
	query := `
		{
			me(id:0x1) {
				name
				friend @filter(eq(@facets(), true)) {
					name
					gender
				}
			}
		}
	`

	_, err := Parse(query)
	require.Error(t, err)
}

func TestFacetsFilterFail2(t *testing.T) {
	// @facets not allowed - should have some facet
	query := `
		{
			me(id:0x1) {
				name
				friend @filter(eq(@facets, true)) {
					name
					gender
				}
			}
		}
	`

	_, err := Parse(query)
	require.Error(t, err)
}

func TestFacetsFilterFail3(t *testing.T) {
	// @facets only allowed in attr part ; not at arguments.
	query := `
		{
			me(id:0x1) {
				name
				friend @filter(somefn(abc, @facets, def)) {
					name
					gender
				}
			}
		}
	`

	_, err := Parse(query)
	require.Error(t, err)
}

func TestFacetsFilterFail4(t *testing.T) {
	// multiple facets not allowed in facets inside filters
	query := `
		{
			me(id:0x1) {
				name
				friend @filter(eq(@facets(close, family, since), true)) {
					name
					gender
				}
			}
		}
	`

	_, err := Parse(query)
	require.Error(t, err)
}
