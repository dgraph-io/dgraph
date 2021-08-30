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

// This file contains tests related to parsing of Schema, Count, GraphQL, Vars.
func TestParseCountValError(t *testing.T) {
	query := `
{
  me(func: uid(1)) {
    Upvote {
       u as Author
    }
    count(val(u))
  }
}
	`
	_, err := Parse(Request{Str: query})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Count of a variable is not allowed")
}

func TestParseVarError(t *testing.T) {
	query := `
	{
		var(func: uid(0x0a)) {
			a as friends
		}

		me(func: uid(a)) {
			uid(a)
		}
	}
`
	_, err := Parse(Request{Str: query})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Cannot do uid() of a variable")
}

func TestLenFunctionWithMultipleVariableError(t *testing.T) {
	query := `
	{
		var(func: uid(0x0a)) {
			fr as friends {
				a as age
			}
		}

		me(func: uid(fr)) @filter(gt(len(a, b), 10)) {
		 name
		}
	}
`
	// Multiple vars not allowed.
	_, err := Parse(Request{Str: query})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Multiple variables not allowed in len function")
}

func TestLenFunctionWithNoVariable(t *testing.T) {
	query := `
	{
		var(func: uid(0x0a)) {
			fr as friends {
				a as age
			}
		}

		me(func: uid(fr)) @filter(len(), 10) {
			name
		}
	}
`
	_, err := Parse(Request{Str: query})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Got empty attr for function")
}

func TestCountWithLenFunctionError(t *testing.T) {
	query := `
	{
		var(func: uid(0x0a)) {
			fr as friends {
				a as age
			}
		}

		me(func: uid(fr)) @filter(count(name), len(fr)) {
			name
		}
	}
`
	_, err := Parse(Request{Str: query})
	// TODO(pawan) - Error message can be improved.
	require.Error(t, err)
}

func TestParseShortestPathWithUidVars(t *testing.T) {
	query := `{
		a as var(func: uid(0x01))
		b as var(func: uid(0x02))

		shortest(from: uid(a), to: uid(b)) {
			password
			friend
		}

	}`
	res, err := Parse(Request{Str: query})
	require.NoError(t, err)
	q := res.Query[2]
	require.NotNil(t, q.ShortestPathArgs.From)
	require.Equal(t, 1, len(q.ShortestPathArgs.From.NeedsVar))
	require.Equal(t, "a", q.ShortestPathArgs.From.NeedsVar[0].Name)
	require.Equal(t, "uid", q.ShortestPathArgs.From.Name)
	require.NotNil(t, q.ShortestPathArgs.To)
	require.Equal(t, 1, len(q.ShortestPathArgs.To.NeedsVar))
}

func TestParseSchema(t *testing.T) {
	query := `
		schema (pred : name) {
			pred
			type
		}
	`
	res, err := Parse(Request{Str: query})
	require.NoError(t, err)
	require.Equal(t, res.Schema.Predicates[0], "name")
	require.Equal(t, len(res.Schema.Fields), 2)
	require.Equal(t, res.Schema.Fields[0], "pred")
	require.Equal(t, res.Schema.Fields[1], "type")
}

func TestParseSchemaMulti(t *testing.T) {
	query := `
		schema (pred : [name,hi]) {
			pred
			type
		}
	`
	res, err := Parse(Request{Str: query})
	require.NoError(t, err)
	require.Equal(t, len(res.Schema.Predicates), 2)
	require.Equal(t, res.Schema.Predicates[0], "name")
	require.Equal(t, res.Schema.Predicates[1], "hi")
	require.Equal(t, len(res.Schema.Fields), 2)
	require.Equal(t, res.Schema.Fields[0], "pred")
	require.Equal(t, res.Schema.Fields[1], "type")
}

func TestParseSchemaAll(t *testing.T) {
	query := `
		schema {
			pred
			type
		}
	`
	res, err := Parse(Request{Str: query})
	require.NoError(t, err)
	require.Equal(t, len(res.Schema.Predicates), 0)
	require.Equal(t, len(res.Schema.Fields), 2)
	require.Equal(t, res.Schema.Fields[0], "pred")
	require.Equal(t, res.Schema.Fields[1], "type")
}

func TestParseSchemaWithComments(t *testing.T) {
	query := `
		schema (pred : name) {
			#hi
			pred #bye
			type
		}
	`
	res, err := Parse(Request{Str: query})
	require.NoError(t, err)
	require.Equal(t, res.Schema.Predicates[0], "name")
	require.Equal(t, len(res.Schema.Fields), 2)
	require.Equal(t, res.Schema.Fields[0], "pred")
	require.Equal(t, res.Schema.Fields[1], "type")
}

func TestParseSchemaAndQuery(t *testing.T) {
	query1 := `
		schema {
			pred
			type
		}
		query {
			me(func: uid( tomhanks)) {
				name
				hometown
			}
		}
	`
	query2 := `
		query {
			me(func: uid( tomhanks)) {
				name
				hometown
			}
		}
		schema {
			pred
			type
		}
	`

	_, err := Parse(Request{Str: query1})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Schema block is not allowed with query block")

	_, err = Parse(Request{Str: query2})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Schema block is not allowed with query block")
}

func TestParseSchemaType(t *testing.T) {
	query := `
		schema (type: Person) {
		}
	`
	res, err := Parse(Request{Str: query})
	require.NoError(t, err)
	require.Equal(t, len(res.Schema.Predicates), 0)
	require.Equal(t, len(res.Schema.Types), 1)
	require.Equal(t, res.Schema.Types[0], "Person")
	require.Equal(t, len(res.Schema.Fields), 0)
}

func TestParseSchemaTypeMulti(t *testing.T) {
	query := `
		schema (type: [Person, Animal]) {
		}
	`
	res, err := Parse(Request{Str: query})
	require.NoError(t, err)
	require.Equal(t, len(res.Schema.Predicates), 0)
	require.Equal(t, len(res.Schema.Types), 2)
	require.Equal(t, res.Schema.Types[0], "Person")
	require.Equal(t, res.Schema.Types[1], "Animal")
	require.Equal(t, len(res.Schema.Fields), 0)
}

func TestParseSchemaSpecialChars(t *testing.T) {
	query := `
		schema (pred: [Person, <人物>]) {
		}
	`
	res, err := Parse(Request{Str: query})
	require.NoError(t, err)
	require.Equal(t, len(res.Schema.Predicates), 2)
	require.Equal(t, len(res.Schema.Types), 0)
	require.Equal(t, res.Schema.Predicates[0], "Person")
	require.Equal(t, res.Schema.Predicates[1], "人物")
	require.Equal(t, len(res.Schema.Fields), 0)
}

func TestParseSchemaTypeSpecialChars(t *testing.T) {
	query := `
		schema (type: [Person, <人物>]) {
		}
	`
	res, err := Parse(Request{Str: query})
	require.NoError(t, err)
	require.Equal(t, len(res.Schema.Predicates), 0)
	require.Equal(t, len(res.Schema.Types), 2)
	require.Equal(t, res.Schema.Types[0], "Person")
	require.Equal(t, res.Schema.Types[1], "人物")
	require.Equal(t, len(res.Schema.Fields), 0)
}

func TestParseSchemaError(t *testing.T) {
	query := `
		schema () {
			pred
			type
		}
	`
	_, err := Parse(Request{Str: query})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Invalid schema block")
}

func TestParseSchemaErrorMulti(t *testing.T) {
	query := `
		schema {
			pred
			type
		}
		schema {
			pred
			type
		}
	`
	_, err := Parse(Request{Str: query})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Only one schema block allowed")
}

func TestParseVarInFacet(t *testing.T) {
	query := `
query works($since: string = "2018") {
  q(func: has(works_in)) @cascade {
    name
    works_in @facets @facets(gt(since, $since)) {
      name
    }
  }
}`

	res, err := Parse(Request{Str: query})
	require.NoError(t, err)
	require.NotNil(t, res.Query[0])
	require.Equal(t, "2018", res.Query[0].Children[1].FacetsFilter.Func.Args[0].Value)
}

func TestParseVariablesError1(t *testing.T) {
	query := `
	query testQuery($a: string, $b: int!){
			root(func: uid( 0x0a)) {
				type.object.name.es-419
			}
		}
	`
	_, err := Parse(Request{Str: query})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Unrecognized character in lexText: U+0034 '4'")
}

func TestParseCountAsFuncMultiple(t *testing.T) {
	query := `{
		me(func: uid(1)) {
			count(friends), count(relatives)
			count(classmates)
			gender,age
			hometown
		}
	}
`
	gq, err := Parse(Request{Str: query})
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
	query := `{
		me(func: uid(1)) {
			count(friends, relatives
			classmates)
			gender,age
			hometown
		}
	}
`
	_, err := Parse(Request{Str: query})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Multiple predicates not allowed in single count")
}

func TestParseCountAsFunc(t *testing.T) {
	query := `{
		me(func: uid(1)) {
			count(friends)
			gender,age
			hometown
		}
	}
`
	gq, err := Parse(Request{Str: query})
	require.NoError(t, err)
	require.Equal(t, true, gq.Query[0].Children[0].IsCount)
	require.Equal(t, 4, len(gq.Query[0].Children))

}

func TestParseCountError1(t *testing.T) {
	query := `{
		me(func: uid(1)) {
			count(friends
			gender,age
			hometown
		}
	}
`
	_, err := Parse(Request{Str: query})
	require.Error(t, err)
	require.Contains(t, err.Error(),
		"Unrecognized character inside a func: U+007D '}'")
}

func TestParseCountError2(t *testing.T) {
	query := `{
		me(func: uid(1)) {
			count((friends)
			gender,age
			hometown
		}
	}
`
	_, err := Parse(Request{Str: query})
	require.Error(t, err)
	require.Contains(t, err.Error(),
		"Unrecognized character inside a func: U+007D '}'")
}

func TestParseGroupbyWithCountVar(t *testing.T) {
	query := `
	query {
		me(func: uid(0x1)) {
			friends @groupby(friends) {
				a as count(uid)
			}
			hometown
			age
		}

		groups(func: uid(a)) {
			uid
			val(a)
		}
	}
`
	res, err := Parse(Request{Str: query})
	require.NoError(t, err)
	require.Equal(t, 1, len(res.Query[0].Children[0].GroupbyAttrs))
	require.Equal(t, "friends", res.Query[0].Children[0].GroupbyAttrs[0].Attr)
	require.Equal(t, "a", res.Query[0].Children[0].Children[0].Var)
}

func TestParseGroupbyWithMaxVar(t *testing.T) {
	query := `
	query {
		me(func: uid(0x1)) {
			friends @groupby(friends) {
				a as max(first-name@en:ta)
			}
			hometown
			age
		}

		groups(func: uid(a)) {
			uid
			val(a)
		}
	}
`
	res, err := Parse(Request{Str: query})
	require.NoError(t, err)
	require.Equal(t, 1, len(res.Query[0].Children[0].GroupbyAttrs))
	require.Equal(t, "friends", res.Query[0].Children[0].GroupbyAttrs[0].Attr)
	require.Equal(t, "first-name", res.Query[0].Children[0].Children[0].Attr)
	require.Equal(t, []string{"en", "ta"}, res.Query[0].Children[0].Children[0].Langs)
	require.Equal(t, "a", res.Query[0].Children[0].Children[0].Var)
}
func TestParseFacetsError1(t *testing.T) {
	query := `
	query {
		me(func: uid(0x1)) {
			friends @facets {
				name @facets(facet1,, facet2)
			}
			hometown
			age
		}
	}
`
	_, err := Parse(Request{Str: query})
	require.Error(t, err)
	require.Contains(t, err.Error(),
		"Consecutive commas not allowed.")
}

func TestParseFacetsVarError(t *testing.T) {
	query := `
	query {
		me(func: uid(0x1)) {
			friends @facets {
				name @facets(facet1, b as)
			}
			hometown
			age
		}
	}
`
	_, err := Parse(Request{Str: query})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Expected name in facet list")
}
func TestParseFacetsError2(t *testing.T) {
	query := `
	query {
		me(func: uid(0x1)) {
			friends @facets {
				name @facets(facet1 facet2)
			}
			hometown
			age
		}
	}
`
	_, err := Parse(Request{Str: query})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Expected ( after func name [facet1]")
}

func TestParseFacetsOrderError1(t *testing.T) {
	query := `
	query {
		me(func: uid(0x1)) {
			friends @facets(orderdesc: orderdesc: closeness) {
				name
			}
		}
	}
`
	_, err := Parse(Request{Str: query})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Expected ( after func name [orderdesc]")
}

func TestParseFacetsOrderError2(t *testing.T) {
	query := `
	query {
		me(func: uid(0x1)) {
			friends @facets(a as b as closeness) {
				name
			}
		}
	}
`
	_, err := Parse(Request{Str: query})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Expected ( after func name [a]")
}

func TestParseFacetsOrderWithAlias(t *testing.T) {
	query := `
	query {
		me(func: uid(0x1)) {
			friends @facets(orderdesc: closeness, b as some, order: abc, key, key1: val, abcd) {
				val(b)
			}
		}
	}
`
	res, err := Parse(Request{Str: query})
	require.NoError(t, err)
	node := res.Query[0].Children[0].Facets
	require.Equal(t, 6, len(node.Param))
	require.Equal(t, "order", node.Param[0].Alias)
	require.Equal(t, "abc", node.Param[0].Key)
	require.Equal(t, "abcd", node.Param[1].Key)
	require.Equal(t, "val", node.Param[5].Key)
	require.Equal(t, "key1", node.Param[5].Alias)
}

func TestParseFacetsDuplicateVarError(t *testing.T) {
	query := `
	query {
		me(func: uid(0x1)) {
			friends @facets(a as closeness, b as closeness) {
				name
			}
		}
	}
`
	_, err := Parse(Request{Str: query})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Duplicate variable mappings")
}

func TestParseFacetsOrderVar(t *testing.T) {
	query := `
	query {
		me1(func: uid(0x1)) {
			friends @facets(orderdesc: a as b) {
				name
			}
		}
		me2(func: uid(a)) { }
	}
`
	_, err := Parse(Request{Str: query})
	require.NoError(t, err)
}

func TestParseFacetsOrderVar2(t *testing.T) {
	query := `
	query {
		me(func: uid(0x1)) {
			friends @facets(a as orderdesc: b) {
				name
			}
		}
		me(func: uid(a)) {

		}
	}
`
	_, err := Parse(Request{Str: query})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Expected ( after func name [a]")
}

func TestParseFacets(t *testing.T) {
	query := `
	query {
		me(func: uid(0x1)) {
			friends @facets(orderdesc: closeness) {
				name
			}
		}
	}
`
	res, err := Parse(Request{Str: query})
	require.NoError(t, err)
	require.NotNil(t, res.Query[0])
	require.Equal(t, []string{"friends"}, childAttrs(res.Query[0]))
	require.NotNil(t, res.Query[0].Children[0].Facets)
	require.Equal(t, []string{"name"}, childAttrs(res.Query[0].Children[0]))
	require.Equal(t, "closeness", res.Query[0].Children[0].FacetsOrder[0].Key)
	require.True(t, res.Query[0].Children[0].FacetsOrder[0].Desc)
}

func TestParseOrderbyMultipleFacets(t *testing.T) {
	query := `
	query {
		me(func: uid(0x1)) {
			friends @facets(orderdesc: closeness, orderasc: since) {
				name
			}
		}
	}
`
	res, err := Parse(Request{Str: query})
	require.NoError(t, err)
	require.NotNil(t, res.Query[0])
	require.Equal(t, []string{"friends"}, childAttrs(res.Query[0]))
	require.NotNil(t, res.Query[0].Children[0].Facets)
	require.Equal(t, []string{"name"}, childAttrs(res.Query[0].Children[0]))
	require.Equal(t, 2, len(res.Query[0].Children[0].FacetsOrder))
	require.Equal(t, "closeness", res.Query[0].Children[0].FacetsOrder[0].Key)
	require.True(t, res.Query[0].Children[0].FacetsOrder[0].Desc)
	require.Equal(t, "since", res.Query[0].Children[0].FacetsOrder[1].Key)
	require.False(t, res.Query[0].Children[0].FacetsOrder[1].Desc)
}

func TestParseOrderbyMultipleFacetsWithAlias(t *testing.T) {
	query := `
	query {
		me(func: uid(0x1)) {
			friends @facets(orderdesc: closeness, orderasc: since, score, location:from) {
				name
			}
		}
	}
`
	res, err := Parse(Request{Str: query})
	require.NoError(t, err)
	require.NotNil(t, res.Query[0])
	require.Equal(t, []string{"friends"}, childAttrs(res.Query[0]))
	require.NotNil(t, res.Query[0].Children[0].Facets)
	require.Equal(t, []string{"name"}, childAttrs(res.Query[0].Children[0]))
	require.Equal(t, 2, len(res.Query[0].Children[0].FacetsOrder))
	require.Equal(t, "closeness", res.Query[0].Children[0].FacetsOrder[0].Key)
	require.True(t, res.Query[0].Children[0].FacetsOrder[0].Desc)
	require.Equal(t, "since", res.Query[0].Children[0].FacetsOrder[1].Key)
	require.False(t, res.Query[0].Children[0].FacetsOrder[1].Desc)
	require.Equal(t, 4, len(res.Query[0].Children[0].Facets.Param))
	require.Nil(t, res.Query[0].Children[0].FacetsFilter)
	require.Empty(t, res.Query[0].Children[0].FacetVar)
	for _, param := range res.Query[0].Children[0].Facets.Param {
		if param.Key == "from" {
			require.Equal(t, "location", param.Alias)
			break
		}
	}
}

func TestParseOrderbySameFacetsMultipleTimes(t *testing.T) {
	query := `
	query {
		me(func: uid(0x1)) {
			friends @facets(orderdesc: closeness, orderasc: closeness) {
				name
			}
		}
	}
`
	_, err := Parse(Request{Str: query})
	require.Contains(t, err.Error(),
		"Sorting by facet: [closeness] can only be done once")
}
func TestParseFacetsMultiple(t *testing.T) {
	query := `
	query {
		me(func: uid(0x1)) {
			friends @facets {
				name @facets(key1, key2, key3)
			}
			hometown
			age
		}
	}
`
	res, err := Parse(Request{Str: query})
	require.NoError(t, err)
	require.NotNil(t, res.Query[0])
	require.Equal(t, []string{"friends", "hometown", "age"}, childAttrs(res.Query[0]))
	require.NotNil(t, res.Query[0].Children[0].Facets)
	require.Equal(t, true, res.Query[0].Children[0].Facets.AllKeys)
	require.Equal(t, []string{"name"}, childAttrs(res.Query[0].Children[0]))
	require.NotNil(t, res.Query[0].Children[0].Children[0].Facets)
	require.Equal(t, false, res.Query[0].Children[0].Children[0].Facets.AllKeys)
	require.Equal(t, 3, len(res.Query[0].Children[0].Children[0].Facets.Param))
}

func TestParseFacetsAlias(t *testing.T) {
	query := `
	query {
		me(func: uid(0x1)) {
			friends @facets {
				name @facets(a1: key1, a2: key2, a3: key3)
			}
		}
	}
`
	res, err := Parse(Request{Str: query})
	require.NoError(t, err)
	require.NotNil(t, res.Query[0])

	require.NotNil(t, res.Query[0].Children[0].Facets)
	require.Equal(t, true, res.Query[0].Children[0].Facets.AllKeys)
	require.Equal(t, []string{"name"}, childAttrs(res.Query[0].Children[0]))
	require.NotNil(t, res.Query[0].Children[0].Children[0].Facets)

	node := res.Query[0].Children[0].Children[0].Facets
	require.Equal(t, false, node.AllKeys)
	require.Equal(t, 3, len(node.Param))
	require.Equal(t, "a1", node.Param[0].Alias)
	require.Equal(t, "key1", node.Param[0].Key)
	require.Equal(t, "a3", node.Param[2].Alias)
	require.Equal(t, "key3", node.Param[2].Key)
}

func TestParseFacetsMultipleVar(t *testing.T) {
	query := `
	query {
		me(func: uid(0x1)) {
			friends @facets {
				name @facets(a as key1, key2, b as key3)
			}
			hometown
			age
		}
		h(func: uid(a, b)) {
			uid
		}
	}
`
	res, err := Parse(Request{Str: query})
	require.NoError(t, err)
	require.NotNil(t, res.Query[0])
	require.Equal(t, []string{"friends", "hometown", "age"}, childAttrs(res.Query[0]))
	require.NotNil(t, res.Query[0].Children[0].Facets)
	require.Equal(t, true, res.Query[0].Children[0].Facets.AllKeys)
	require.Equal(t, []string{"name"}, childAttrs(res.Query[0].Children[0]))
	require.NotNil(t, res.Query[0].Children[0].Children[0].Facets)
	require.Equal(t, false, res.Query[0].Children[0].Children[0].Facets.AllKeys)
	require.Equal(t, 3, len(res.Query[0].Children[0].Children[0].Facets.Param))
	require.Equal(t, "a", res.Query[0].Children[0].Children[0].FacetVar["key1"])
	require.Equal(t, "", res.Query[0].Children[0].Children[0].FacetVar["key2"])
	require.Equal(t, "b", res.Query[0].Children[0].Children[0].FacetVar["key3"])
}

func TestParseFacetsMultipleRepeat(t *testing.T) {
	query := `
	query {
		me(func: uid(0x1)) {
			friends @facets {
				name @facets(key1, key2, key3, key1)
			}
			hometown
			age
		}
	}
`
	res, err := Parse(Request{Str: query})
	require.NoError(t, err)
	require.NotNil(t, res.Query[0])
	require.Equal(t, []string{"friends", "hometown", "age"}, childAttrs(res.Query[0]))
	require.NotNil(t, res.Query[0].Children[0].Facets)
	require.Equal(t, true, res.Query[0].Children[0].Facets.AllKeys)
	require.Equal(t, []string{"name"}, childAttrs(res.Query[0].Children[0]))
	require.NotNil(t, res.Query[0].Children[0].Children[0].Facets)
	require.Equal(t, false, res.Query[0].Children[0].Children[0].Facets.AllKeys)
	require.Equal(t, 3, len(res.Query[0].Children[0].Children[0].Facets.Param))
}

func TestParseFacetsEmpty(t *testing.T) {
	query := `
	query {
		me(func: uid(0x1)) {
			friends @facets() {
			}
			hometown
			age
		}
	}
`
	res, err := Parse(Request{Str: query})
	require.NoError(t, err)
	require.NotNil(t, res.Query[0])
	require.Equal(t, []string{"friends", "hometown", "age"}, childAttrs(res.Query[0]))
	require.NotNil(t, res.Query[0].Children[0].Facets)
	require.Equal(t, false, res.Query[0].Children[0].Facets.AllKeys)
	require.Equal(t, 0, len(res.Query[0].Children[0].Facets.Param))
}

func TestParseFacetsFail1(t *testing.T) {
	// key can not be empty..
	query := `
	query {
		me(func: uid(0x1)) {
			friends @facets(key1,, key2) {
			}
			hometown
			age
		}
	}
`
	_, err := Parse(Request{Str: query})
	require.Error(t, err)
	require.Contains(t, err.Error(),
		"Consecutive commas not allowed.")
}

func TestCountAtRoot(t *testing.T) {
	query := `{
		me(func: uid( 1)) {
			count(uid)
			count(enemy)
		}
	}`
	_, err := Parse(Request{Str: query})
	require.NoError(t, err)
}

func TestCountAtRootErr(t *testing.T) {
	query := `{
		me(func: uid( 1)) {
			count(enemy) {
				name
			}
		}
	}`
	_, err := Parse(Request{Str: query})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Cannot have children attributes when asking for count")
}

func TestCountAtRootErr2(t *testing.T) {
	query := `{
		me(func: uid( 1)) {
			count()
		}
	}`
	_, err := Parse(Request{Str: query})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Cannot use count(), please use count(uid)")
}

func TestMathWithoutVarAlias(t *testing.T) {
	query := `{
			f(func: anyofterms(name, "Rick Michonne Andrea")) {
				ageVar as age
				math(ageVar *2)
			}
		}`
	_, err := Parse(Request{Str: query})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Function math should be used with a variable or have an alias")
}

func TestOrderByVarAndPred(t *testing.T) {
	query := `{
		q(func: uid(1), orderasc: name, orderdesc: val(n)) {
		}

		var(func: uid(0x0a)) {
			friends {
				n AS name
			}
		}

	}`
	_, err := Parse(Request{Str: query})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Multiple sorting only allowed by predicates.")

	query = `{
		q(func: uid(1)) {
		}

		var(func: uid(0x0a)) {
			friends (orderasc: name, orderdesc: val(n)) {
				n AS name
			}
		}

	}`
	_, err = Parse(Request{Str: query})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Multiple sorting only allowed by predicates.")

	query = `{
		q(func: uid(1)) {
		}

		var(func: uid(0x0a)) {
			friends (orderasc: name, orderdesc: genre) {
				name
			}
		}

	}`
	_, err = Parse(Request{Str: query})
	require.NoError(t, err)
}
func TestParseMissingGraphQLVar(t *testing.T) {
	for _, q := range []string{
		"{ q(func: eq(name, $a)) { name }}",
		"query { q(func: eq(name, $a)) { name }}",
		"query foo { q(func: eq(name, $a)) { name }}",
		"query foo () { q(func: eq(name, $a)) { name }}",
		"query foo ($b: string) { q(func: eq(name, $a)) { name }}",
		"query foo ($a: string) { q(func: eq(name, $b)) { name }}",
	} {
		r := Request{
			Str:       q,
			Variables: map[string]string{"$a": "alice"},
		}
		_, err := Parse(r)
		t.Log(q)
		t.Log(err)
		require.Error(t, err)
	}
}

func TestParseGraphQLVarPaginationRoot(t *testing.T) {
	for _, q := range []string{
		"query test($a: int = 2){ q(func: uid(0x1), first: $a) { name }}",
		"query test($a: int = 2){ q(func: uid(0x1), offset: $a) { name }}",
		"query test($a: int = 2){ q(func: uid(0x1), orderdesc: name, first: $a) { name }}",
		"query test($a: int = 2){ q(func: uid(0x1), orderdesc: name, offset: $a) { name }}",
		"query test($a: int = 2){ q(func: eq(name, \"abc\"), orderdesc: name, first: $a) { name }}",
		"query test($a: int = 2){ q(func: eq(name, \"abc\"), orderdesc: name, offset: $a) { name }}",
	} {
		r := Request{
			Str:       q,
			Variables: map[string]string{"$a": "3"},
		}
		gq, err := Parse(r)
		t.Log(q)
		t.Log(err)
		require.NoError(t, err)
		args := gq.Query[0].Args
		require.True(t, args["first"] == "3" || args["offset"] == "3")
	}
}

func TestParseGraphQLVarPaginationChild(t *testing.T) {
	for _, q := range []string{
		"query test($a: int = 2){ q(func: uid(0x1)) { friend(first: $a) }}",
		"query test($a: int = 2){ q(func: uid(0x1)) { friend(offset: $a) }}",
		"query test($a: int = 2){ q(func: uid(0x1), orderdesc: name) { friend(first: $a) }}",
		"query test($a: int = 2){ q(func: uid(0x1), orderdesc: name) { friend(offset: $a) }}",
		"query test($a: int = 2){ q(func: eq(name, \"abc\"), orderdesc: name) { friend(first: $a) }}",
		"query test($a: int = 2){ q(func: eq(name, \"abc\"), orderdesc: name) { friend(offset: $a) }}",
	} {
		r := Request{
			Str:       q,
			Variables: map[string]string{"$a": "3"},
		}
		gq, err := Parse(r)
		t.Log(q)
		t.Log(err)
		require.NoError(t, err)
		args := gq.Query[0].Children[0].Args
		require.True(t, args["first"] == "3" || args["offset"] == "3")
	}
}

func TestParseGraphQLVarPaginationRootMultiple(t *testing.T) {
	q := `query test($a: int, $b: int, $after: string){
		q(func: uid(0x1), first: $a, offset: $b, after: $after, orderasc: name) {
			friend
		}
	}`

	r := Request{
		Str:       q,
		Variables: map[string]string{"$a": "3", "$b": "5", "$after": "0x123"},
	}
	gq, err := Parse(r)
	require.NoError(t, err)
	args := gq.Query[0].Args
	require.Equal(t, args["first"], "3")
	require.Equal(t, args["offset"], "5")
	require.Equal(t, args["after"], "0x123")
	require.Equal(t, gq.Query[0].Order[0].Attr, "name")
}

func TestParseGraphQLVarArray(t *testing.T) {
	tests := []struct {
		q    string
		vars map[string]string
		args int
	}{
		{q: `query test($a: string){q(func: eq(name, [$a])) {name}}`,
			vars: map[string]string{"$a": "srfrog"}, args: 1},
		{q: `query test($a: string, $b: string){q(func: eq(name, [$a, $b])) {name}}`,
			vars: map[string]string{"$a": "srfrog", "$b": "horseman"}, args: 2},
		{q: `query test($a: string, $b: string, $c: string){q(func: eq(name, [$a, $b, $c])) {name}}`,
			vars: map[string]string{"$a": "srfrog", "$b": "horseman", "$c": "missbug"}, args: 3},
		// mixed var and value
		{q: `query test($a: string){q(func: eq(name, [$a, "mrtrout"])) {name}}`,
			vars: map[string]string{"$a": "srfrog"}, args: 2},
		{q: `query test($a: string){q(func: eq(name, ["mrtrout", $a])) {name}}`,
			vars: map[string]string{"$a": "srfrog"}, args: 2},
	}
	for _, tc := range tests {
		gq, err := Parse(Request{Str: tc.q, Variables: tc.vars})
		require.NoError(t, err)
		require.Equal(t, 1, len(gq.Query))
		require.Equal(t, "eq", gq.Query[0].Func.Name)
		require.Equal(t, tc.args, len(gq.Query[0].Func.Args))
		var found bool
		for _, val := range tc.vars {
			found = false
			for _, arg := range gq.Query[0].Func.Args {
				if val == arg.Value {
					found = true
					break
				}
			}
			require.True(t, found, "vars not matched: %v", tc.vars)
		}
	}
}

func TestParseGraphQLVarArrayUID_IN(t *testing.T) {
	tests := []struct {
		q    string
		vars map[string]string
		args int
	}{
		// uid_in test cases (uids and predicate inside uid_in are dummy)
		{q: `query test($a: string){q(func: uid_in(director.film, [$a])) {name}}`,
			vars: map[string]string{"$a": "0x4e472a"}, args: 1},
		{q: `query test($a: string, $b: string){q(func: uid_in(director.film, [$a, $b])) {name}}`,
			vars: map[string]string{"$a": "0x4e472a", "$b": "0x4e9545"}, args: 2},
		{q: `query test($a: string){q(func: uid_in(name, [$a, "0x4e9545"])) {name}}`,
			vars: map[string]string{"$a": "0x4e472a"}, args: 2},
		{q: `query test($a: string){q(func: uid_in(name, ["0x4e9545", $a])) {name}}`,
			vars: map[string]string{"$a": "0x4e472a"}, args: 2},
	}
	for _, tc := range tests {
		gq, err := Parse(Request{Str: tc.q, Variables: tc.vars})
		require.NoError(t, err)
		require.Equal(t, 1, len(gq.Query))
		require.Equal(t, "uid_in", gq.Query[0].Func.Name)
		require.Equal(t, tc.args, len(gq.Query[0].Func.Args))
		var found bool
		for _, val := range tc.vars {
			found = false
			for _, arg := range gq.Query[0].Func.Args {
				if val == arg.Value {
					found = true
					break
				}
			}
			require.True(t, found, "vars not matched: %v", tc.vars)
		}
	}
}

func TestParseGraphQLValueArray(t *testing.T) {
	q := `
	{
		q(func: eq(name, ["srfrog", "horseman"])) {
			name
		}
	}`
	gq, err := Parse(Request{Str: q})
	require.NoError(t, err)
	require.Equal(t, 1, len(gq.Query))
	require.Equal(t, "eq", gq.Query[0].Func.Name)
	require.Equal(t, 2, len(gq.Query[0].Func.Args))
	require.Equal(t, "srfrog", gq.Query[0].Func.Args[0].Value)
	require.Equal(t, "horseman", gq.Query[0].Func.Args[1].Value)
}

func TestParseGraphQLMixedVarArray(t *testing.T) {
	q := `
	query test($a: string, $b: string, $c: string){
		q(func: eq(name, ["uno", $a, $b, "cuatro", $c])) {
			name
		}
	}`
	r := Request{
		Str:       q,
		Variables: map[string]string{"$a": "dos", "$b": "tres", "$c": "cinco"},
	}
	gq, err := Parse(r)
	require.NoError(t, err)
	require.Equal(t, 1, len(gq.Query))
	require.Equal(t, "eq", gq.Query[0].Func.Name)
	require.Equal(t, 5, len(gq.Query[0].Func.Args))
	require.Equal(t, "uno", gq.Query[0].Func.Args[0].Value)
	require.Equal(t, "dos", gq.Query[0].Func.Args[1].Value)
	require.Equal(t, "tres", gq.Query[0].Func.Args[2].Value)
	require.Equal(t, "cuatro", gq.Query[0].Func.Args[3].Value)
	require.Equal(t, "cinco", gq.Query[0].Func.Args[4].Value)
}

func TestParseVarAfterCountQry(t *testing.T) {
	query := `
		{
			q(func: allofterms(name@en, "steven spielberg")) {
				director.film {
					u1 as count(uid)
					genre {
						u2 as math(1)
					}
			  	}
			}

			sum() {
				totalMovies: sum(val(u1))
				totalGenres: sum(val(u2))
			}
		}
	`

	_, err := Parse(Request{Str: query})
	require.NoError(t, err)
}
