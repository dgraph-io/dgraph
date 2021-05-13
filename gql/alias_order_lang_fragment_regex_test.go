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

// This file contains the tests related to alias, fragments, IRIRef, Lang, Order and Regex.
func TestParse_alias_count(t *testing.T) {
	query := `
		{
			me(func: uid(0x0a)) {
				name,
				bestFriend: friends(first: 10) {
					nameCount: count(name)
				}
			}
		}
	`
	res, err := Parse(Request{Str: query})
	require.NoError(t, err)
	require.NotNil(t, res.Query[0])
	require.Equal(t, childAttrs(res.Query[0]), []string{"name", "friends"})
	require.Equal(t, res.Query[0].Children[1].Alias, "bestFriend")
	require.Equal(t, childAttrs(res.Query[0].Children[1]), []string{"name"})
	require.Equal(t, "nameCount", res.Query[0].Children[1].Children[0].Alias)
}

func TestParse_alias_var(t *testing.T) {
	query := `
		{
			me(func: uid(0x0a)) {
				name,
				f as bestFriend: friends(first: 10) {
					c as count(friend)
				}
			}

			friend(func: uid(f)) {
				name
				fcount: val(c)
			}
		}
	`
	res, err := Parse(Request{Str: query})
	require.NoError(t, err)
	require.NotNil(t, res.Query[0])
	require.Equal(t, childAttrs(res.Query[0]), []string{"name", "friends"})
	require.Equal(t, res.Query[0].Children[1].Alias, "bestFriend")
	require.Equal(t, "fcount", res.Query[1].Children[1].Alias)
}

func TestParse_alias_max(t *testing.T) {
	query := `
		{
			me(func: uid(0x0a)) {
				name,
				bestFriend: friends(first: 10) {
					x as count(friends)
				}
				maxfriendcount: max(val(x))
			}
		}
	`
	res, err := Parse(Request{Str: query})
	require.NoError(t, err)
	require.NotNil(t, res.Query[0])
	require.Equal(t, res.Query[0].Children[1].Alias, "bestFriend")
	require.Equal(t, "maxfriendcount", res.Query[0].Children[2].Alias)
}

func TestParse_alias(t *testing.T) {
	query := `
		{
			me(func: uid(0x0a)) {
				name,
				bestFriend: friends(first: 10) {
					name
				}
			}
		}
	`
	res, err := Parse(Request{Str: query})
	require.NoError(t, err)
	require.NotNil(t, res.Query[0])
	require.Equal(t, childAttrs(res.Query[0]), []string{"name", "friends"})
	require.Equal(t, res.Query[0].Children[1].Alias, "bestFriend")
	require.Equal(t, childAttrs(res.Query[0].Children[1]), []string{"name"})
}

func TestParse_alias1(t *testing.T) {
	query := `
		{
			me(func: uid(0x0a)) {
				name: type.object.name.en
				bestFriend: friends(first: 10) {
					name: type.object.name.hi
				}
			}
		}
	`
	res, err := Parse(Request{Str: query})
	require.NoError(t, err)
	require.NotNil(t, res.Query[0])
	require.Equal(t, childAttrs(res.Query[0]), []string{"type.object.name.en", "friends"})
	require.Equal(t, res.Query[0].Children[1].Alias, "bestFriend")
	require.Equal(t, res.Query[0].Children[1].Children[0].Alias, "name")
	require.Equal(t, childAttrs(res.Query[0].Children[1]), []string{"type.object.name.hi"})
}

func TestParseFragmentMultiQuery(t *testing.T) {
	query := `
	{
		user(func: uid(0x0a)) {
			...fragmenta,...fragmentb
			friends {
				name
			}
			...fragmentc
			hobbies
			...fragmentd
		}

		me(func: uid(0x01)) {
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
	res, err := Parse(Request{Str: query})
	require.NoError(t, err)
	require.NotNil(t, res.Query[0])
	require.Equal(t, []string{"name", "id", "friends", "name", "hobbies", "id"}, childAttrs(res.Query[0]))
	require.Equal(t, []string{"name", "id"}, childAttrs(res.Query[1]))
}

func TestParseFragmentNoNesting(t *testing.T) {
	query := `
	query {
		user(func: uid(0x0a)) {
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
	res, err := Parse(Request{Str: query})
	require.NoError(t, err)
	require.NotNil(t, res.Query[0])
	require.Equal(t, childAttrs(res.Query[0]), []string{"name", "id", "friends", "name", "hobbies", "id"})
}

func TestParseFragmentNest1(t *testing.T) {
	query := `
	query {
		user(func: uid(0x0a)) {
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
	res, err := Parse(Request{Str: query})
	require.NoError(t, err)
	require.NotNil(t, res.Query[0])
	require.Equal(t, childAttrs(res.Query[0]), []string{"id", "hobbies", "friends"})
}

func TestParseFragmentNest2(t *testing.T) {
	query := `
	query {
		user(func: uid(0x0a)) {
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
	res, err := Parse(Request{Str: query})
	require.NoError(t, err)
	require.NotNil(t, res.Query[0])
	require.Equal(t, childAttrs(res.Query[0]), []string{"friends"})
	require.Equal(t, childAttrs(res.Query[0].Children[0]), []string{"name", "nickname"})
}

func TestParseFragmentCycle(t *testing.T) {
	query := `
	query {
		user(func: uid(0x0a)) {
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
	_, err := Parse(Request{Str: query})
	require.Error(t, err, "Expected error with cycle")
	require.Contains(t, err.Error(), "Cycle detected")
}

func TestParseFragmentMissing(t *testing.T) {
	query := `
	query {
		user(func: uid(0x0a)) {
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
	_, err := Parse(Request{Str: query})
	require.Error(t, err, "Expected error with missing fragment")
	require.Contains(t, err.Error(), "Missing fragment: fragmenta")
}

func TestParseIRIRef(t *testing.T) {
	query := `{
		me(func: uid( 0x1)) {
			<http://verygood.com/what/about/you>
			friends @filter(allofterms(<http://verygood.com/what/about/you>,
				"good better bad")){
				name
			}
			gender,age
			hometown
		}
	}`

	gq, err := Parse(Request{Str: query})
	require.NoError(t, err)
	require.Equal(t, 5, len(gq.Query[0].Children))
	require.Equal(t, "http://verygood.com/what/about/you", gq.Query[0].Children[0].Attr)
	require.Equal(t, `(allofterms http://verygood.com/what/about/you "good better bad")`,
		gq.Query[0].Children[1].Filter.debugString())
}

func TestParseIRIRef2(t *testing.T) {
	query := `{
		me(func:anyofterms(<http://helloworld.com/how/are/you>, "good better bad")) {
			<http://verygood.com/what/about/you>
			friends @filter(allofterms(<http://verygood.com/what/about/you>,
				"good better bad")){
				name
			}
		}
	}`

	gq, err := Parse(Request{Str: query})
	require.NoError(t, err)
	require.Equal(t, 2, len(gq.Query[0].Children))
	require.Equal(t, "http://verygood.com/what/about/you", gq.Query[0].Children[0].Attr)
	require.Equal(t, `(allofterms http://verygood.com/what/about/you "good better bad")`,
		gq.Query[0].Children[1].Filter.debugString())
	require.Equal(t, "http://helloworld.com/how/are/you", gq.Query[0].Func.Attr)
}

func TestParseIRIRefSpace(t *testing.T) {
	query := `{
		me(func: uid( <http://helloworld.com/how/are/ you>)) {
		}
	      }`

	_, err := Parse(Request{Str: query})
	require.Error(t, err) // because of space.
	require.Contains(t, err.Error(), "Unexpected character ' ' while parsing IRI")
}

func TestParseIRIRefInvalidChar(t *testing.T) {
	query := `{
		me(func: uid( <http://helloworld.com/how/are/^you>)) {
		}
	      }`

	_, err := Parse(Request{Str: query})
	require.Error(t, err) // because of ^
	require.Contains(t, err.Error(), "Unexpected character '^' while parsing IRI")
}

func TestLangs(t *testing.T) {
	query := `
	query {
		me(func: uid(1)) {
			name@en,name@en:ru:hu
		}
	}
	`

	gq, err := Parse(Request{Str: query})
	require.NoError(t, err)
	require.Equal(t, 2, len(gq.Query[0].Children))
	require.Equal(t, "name", gq.Query[0].Children[0].Attr)
	require.Equal(t, []string{"en"}, gq.Query[0].Children[0].Langs)
	require.Equal(t, "name", gq.Query[0].Children[1].Attr)
	require.Equal(t, []string{"en", "ru", "hu"}, gq.Query[0].Children[1].Langs)
}

func TestAllLangs(t *testing.T) {
	query := `
	query {
		me(func: uid(1)) {
			name@*
		}
	}
	`

	gq, err := Parse(Request{Str: query})
	require.NoError(t, err)
	require.Equal(t, 1, len(gq.Query[0].Children))
	require.Equal(t, "name", gq.Query[0].Children[0].Attr)
	require.Equal(t, []string{"*"}, gq.Query[0].Children[0].Langs)
}

func TestLangsInvalid1(t *testing.T) {
	query := `
	query {
		me(func: uid(1)) {
			name@en@ru
		}
	}
	`

	_, err := Parse(Request{Str: query})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Expected directive or language list, got @ru")
}

func TestLangsInvalid2(t *testing.T) {
	query := `
	query {
		me(func: uid(1)) {
			@en:ru
		}
	}
	`

	_, err := Parse(Request{Str: query})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Invalid use of directive.")
}

func TestLangsInvalid3(t *testing.T) {
	query := `
	query {
		me(func: uid(1)) {
			name@en:ru, @en:ru
		}
	}
	`

	_, err := Parse(Request{Str: query})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Expected directive or language list, got @en")
}

func TestLangsInvalid4(t *testing.T) {
	query := `
	query {
		me(func: uid(1)) {
			name@
		}
	}
	`

	_, err := Parse(Request{Str: query})
	require.Error(t, err)
	require.Contains(t, err.Error(),
		"Unrecognized character in lexDirective: U+000A")
}

func TestLangsInvalid5(t *testing.T) {
	query := `
	query {
		me(func: uid(1)) {
			name@<something.wrong>
		}
	}
	`

	_, err := Parse(Request{Str: query})
	require.Error(t, err)
	require.Contains(t, err.Error(),
		"Unrecognized character in lexDirective: U+003C '<'")
}

func TestLangsInvalid6(t *testing.T) {
	query := `
		{
			me(func: uid(0x1004)) {
				name@hi:cn:...
			}
		}
	`

	_, err := Parse(Request{Str: query})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Expected only one dot(.) while parsing language list.")
}

func TestLangsInvalid7(t *testing.T) {
	query := `
		{
			me(func: uid(0x1004)) {
				name@...
			}
		}
	`

	_, err := Parse(Request{Str: query})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Expected only one dot(.) while parsing language list.")
}

func TestLangsInvalid8(t *testing.T) {
	query := `
	query {
		me(func: uid(1)) {
			name@*:en
		}
	}
	`

	_, err := Parse(Request{Str: query})
	require.Error(t, err)
	require.Contains(t, err.Error(),
		"If * is used, no other languages are allowed in the language list")
}

func TestLangsInvalid9(t *testing.T) {
	query := `
	query {
		me(func: eqs(name@*, "Amir")) {
			name@en
		}
	}
	`

	_, err := Parse(Request{Str: query})
	require.Error(t, err)
	require.Contains(t, err.Error(),
		"The * symbol cannot be used as a valid language inside functions")
}

func TestLangsInvalid10(t *testing.T) {
	query := `
	query {
		me(func: uid(1)) {
			name@.:*
		}
	}
	`

	_, err := Parse(Request{Str: query})
	require.Error(t, err)
	require.Contains(t, err.Error(),
		"If * is used, no other languages are allowed in the language list")
}

func TestLangsFunction(t *testing.T) {
	query := `
	query {
		me(func:alloftext(descr@en, "something")) {
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
	require.NotNil(t, res.Query[0])
	require.NotNil(t, res.Query[0].Func)
	require.Equal(t, "descr", res.Query[0].Func.Attr)
	require.Equal(t, "en", res.Query[0].Func.Lang)
}

func TestLangsFunctionMultipleLangs(t *testing.T) {
	query := `
	query {
		me(func:alloftext(descr@hi:en, "something")) {
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
	require.Contains(t, err.Error(), "Expected arg after func [alloftext]")
	require.Contains(t, err.Error(), "\":\"")
}

func TestParseOrderbyFacet(t *testing.T) {
	query := `
	query {
		me(func: uid(0x1)) {
			friends @facets {
				name @facets(facet1)
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
	require.Equal(t, "facet1", res.Query[0].Children[0].Children[0].Facets.Param[0].Key)
}

func TestParseRegexp1(t *testing.T) {
	query := `
	{
	  me(func: uid(0x1)) {
	    name
		friend @filter(regexp(name@en, /case INSENSITIVE regexp with \/ escaped value/i)) {
	      name@en
	    }
	  }
    }
`
	res, err := Parse(Request{Str: query})
	require.NoError(t, err)
	require.NotNil(t, res.Query)
	require.Equal(t, 1, len(res.Query))
	require.Equal(t, "case INSENSITIVE regexp with / escaped value",
		res.Query[0].Children[1].Filter.Func.Args[0].Value)
	require.Equal(t, "i", res.Query[0].Children[1].Filter.Func.Args[1].Value)
}

func TestParseRegexp2(t *testing.T) {
	query := `
	{
	  me(func:regexp(name@en, /another\/compilicated ("") regexp('')/)) {
	    name
	  }
    }
`
	res, err := Parse(Request{Str: query})
	require.NoError(t, err)
	require.NotNil(t, res.Query)
	require.Equal(t, 1, len(res.Query))
	require.Equal(t, "another/compilicated (\"\") regexp('')",
		res.Query[0].Func.Args[0].Value)
	require.Equal(t, "", res.Query[0].Func.Args[1].Value)
}

func TestParseRegexp3(t *testing.T) {
	query := `
	{
	  me(func:allofterms(name, "barack")) @filter(regexp(secret, /whitehouse[0-9]{1,4}/fLaGs)) {
	    name
	  }
    }
`
	res, err := Parse(Request{Str: query})
	require.NoError(t, err)
	require.NotNil(t, res.Query)
	require.Equal(t, 1, len(res.Query))
	require.Equal(t, "whitehouse[0-9]{1,4}", res.Query[0].Filter.Func.Args[0].Value)
	require.Equal(t, "fLaGs", res.Query[0].Filter.Func.Args[1].Value)
}

func TestParseRegexp4(t *testing.T) {
	query := `
	{
	  me(func:regexp(name@en, /pattern/123)) {
	    name
	  }
    }
`
	_, err := Parse(Request{Str: query})
	// only [a-zA-Z] characters can be used as flags
	require.Error(t, err)
	require.Contains(t, err.Error(), "Expected comma or language but got: 123")
}

func TestParseRegexp5(t *testing.T) {
	query := `
	{
	  me(func:regexp(name@en, /pattern/flag123)) {
	    name
	  }
    }
`
	_, err := Parse(Request{Str: query})
	// only [a-zA-Z] characters can be used as flags
	require.Error(t, err)
	require.Contains(t, err.Error(), "Expected comma or language but got: 123")
}

func TestParseRegexp6(t *testing.T) {
	query := `
	{
	  me(func:regexp(name@en, /pattern\/)) {
	    name
	  }
    }
`
	_, err := Parse(Request{Str: query})
	require.Error(t, err)
	require.Contains(t, err.Error(),
		"Unclosed regexp")
}
func TestOrder1(t *testing.T) {
	query := `
		{
			me(func: uid(1), orderdesc: name, orderasc: age) {
				name
			}
		}
	`
	gq, err := Parse(Request{Str: query})
	require.NoError(t, err)
	require.Equal(t, 2, len(gq.Query[0].Order))
	require.Equal(t, "name", gq.Query[0].Order[0].Attr)
	require.Equal(t, true, gq.Query[0].Order[0].Desc)
	require.Equal(t, "age", gq.Query[0].Order[1].Attr)
	require.Equal(t, false, gq.Query[0].Order[1].Desc)
}

func TestOrder2(t *testing.T) {
	query := `
		{
			me(func: uid(0x01)) {
				friend(orderasc: alias, orderdesc: name) @filter(lt(alias, "Pat")) {
					alias
				}
			}
		}
	`
	gq, err := Parse(Request{Str: query})
	require.NoError(t, err)
	curp := gq.Query[0].Children[0]
	require.Equal(t, 2, len(curp.Order))
	require.Equal(t, "alias", curp.Order[0].Attr)
	require.Equal(t, false, curp.Order[0].Desc)
	require.Equal(t, "name", curp.Order[1].Attr)
	require.Equal(t, true, curp.Order[1].Desc)
}

func TestMultipleOrderError(t *testing.T) {
	query := `
		{
			me(func: uid(0x01)) {
				friend(orderasc: alias, orderdesc: alias) {
					alias
				}
			}
		}
	`
	_, err := Parse(Request{Str: query})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Sorting by an attribute: [alias] can only be done once")
}

func TestMultipleOrderError2(t *testing.T) {
	query := `
		{
			me(func: uid(0x01),orderasc: alias, orderdesc: alias) {
				friend {
					alias
				}
			}
		}
	`
	_, err := Parse(Request{Str: query})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Sorting by an attribute: [alias] can only be done once")
}

func TestLangWithDash(t *testing.T) {
	query := `{
		q(func: uid(1)) {
			text@en-us
		}
	}`

	gql, err := Parse(Request{Str: query})
	require.NoError(t, err)
	require.Equal(t, []string{"en-us"}, gql.Query[0].Children[0].Langs)
}
func TestOrderWithMultipleLangFail(t *testing.T) {
	query := `
	{
		me(func: uid(0x1), orderasc: name@en:fr, orderdesc: lastname@ci, orderasc: salary) {
			name
		}
	}
	`
	_, err := Parse(Request{Str: query})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Sorting by an attribute: [name@en:fr] can only be done on one language")
}

func TestOrderWithLang(t *testing.T) {
	query := `
	{
		me(func: uid(0x1), orderasc: name@en, orderdesc: lastname@ci, orderasc: salary) {
			name
		}
	}
`
	res, err := Parse(Request{Str: query})
	require.NoError(t, err)
	require.NotNil(t, res.Query)
	require.Equal(t, 1, len(res.Query))
	orders := res.Query[0].Order
	require.Equal(t, "name", orders[0].Attr)
	require.Equal(t, []string{"en"}, orders[0].Langs)
	require.Equal(t, "lastname", orders[1].Attr)
	require.Equal(t, []string{"ci"}, orders[1].Langs)
	require.Equal(t, "salary", orders[2].Attr)
	require.Equal(t, 0, len(orders[2].Langs))
}

func TestParseLangTagAfterStringInRoot(t *testing.T) {
	// This is a fix for #1499.
	query := `
		{
			q(func: anyofterms(name, "Hello"@en)) {
				uid
			}
		}
	`
	_, err := Parse(Request{Str: query})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Invalid usage of '@' in function argument")
}
