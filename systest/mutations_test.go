/*
 * Copyright 2017-2018 Dgraph Labs, Inc.
 *
 * This file is available under the Apache License, Version 2.0,
 * with the Commons Clause restriction.
 */

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"sort"
	"testing"
	"time"

	"github.com/dgraph-io/dgo"
	"github.com/dgraph-io/dgo/protos/api"
	"github.com/stretchr/testify/require"
)

func TestSystem(t *testing.T) {
	dir, err := ioutil.TempDir("", "")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	cluster := NewDgraphCluster(dir)
	require.NoError(t, cluster.Start())
	defer cluster.Close()

	wrap := func(fn func(*testing.T, *dgo.Dgraph)) func(*testing.T) {
		return func(t *testing.T) {
			require.NoError(t, cluster.client.Alter(
				context.Background(), &api.Operation{DropAll: true}))
			fn(t, cluster.client)
		}
	}

	t.Run("n-quad mutation", wrap(NQuadMutationTest))
	t.Run("expand all lang test", wrap(ExpandAllLangTest))
	t.Run("list with languages", wrap(ListWithLanguagesTest))
	t.Run("delete all reverse index", wrap(DeleteAllReverseIndex))
	t.Run("expand all with reverse predicates", wrap(ExpandAllReversePredicatesTest))
	t.Run("normalise edge cases", wrap(NormalizeEdgeCasesTest))
	t.Run("facets with order", wrap(FacetOrderTest))
	t.Run("lang and sort bug", wrap(LangAndSortBugTest))
	t.Run("sort facets return nil", wrap(SortFacetsReturnNil))
	t.Run("check schema after deleting node", wrap(SchemaAfterDeleteNode))
	t.Run("fulltext equal", wrap(FullTextEqual))
	t.Run("json blank node", wrap(JSONBlankNode))
	t.Run("scalar to list", wrap(ScalarToList))
	t.Run("list to scalar", wrap(ListToScalar))
	t.Run("set after delete for list", wrap(SetAfterDeletionListType))
	t.Run("empty strings with exact", wrap(EmptyNamesWithExact))
	t.Run("empty strings with term", wrap(EmptyRoomsWithTermIndex))
	t.Run("delete with expand all", wrap(DeleteWithExpandAll))
	t.Run("facets using nquads", wrap(FacetsUsingNQuadsError))
	t.Run("skip empty pl for has", wrap(SkipEmptyPLForHas))
	t.Run("facet expand all", wrap(FacetExpandAll))
	t.Run("has with dash", wrap(HasWithDash))
	t.Run("list geo filter", wrap(ListGeoFilterTest))
	t.Run("list regex filter", wrap(ListRegexFilterTest))
	t.Run("regex query vars", wrap(RegexQueryWithVars))
	t.Run("graphql var child", wrap(GraphQLVarChild))
	t.Run("math ge", wrap(MathGe))
}

func ExpandAllLangTest(t *testing.T, c *dgo.Dgraph) {
	ctx := context.Background()

	check(t, (c.Alter(ctx, &api.Operation{
		Schema: `list: [string] @lang .`,
	})))

	txn := c.NewTxn()
	defer txn.Discard(ctx)
	_, err := txn.Mutate(ctx, &api.Mutation{
		CommitNow: true,
		SetNquads: []byte(`
			<0x1> <name> "abc" .
			<0x1> <name> "abc_en"@en .
			<0x1> <name> "abc_nl"@nl .
			<0x2> <name> "abc_hi"@hi .
			<0x2> <name> "abc_ci"@ci .
			<0x2> <name> "abc_ja"@ja .
			<0x3> <name> "abcd" .
			<0x1> <number> "99"^^<xs:int> .

			<0x1> <list> "first" .
			<0x1> <list> "first_en"@en .
			<0x1> <list> "first_it"@it .
			<0x1> <list> "second" .
		`),
	})
	check(t, err)

	resp, err := c.NewTxn().Query(context.Background(), `
	{
		q(func: uid(0x1,0x2,0x3)) {
			expand(_all_)
		}
	}
	`)
	check(t, err)

	CompareJSON(t, `
	{
		"q": [
			{
				"name": "abcd"
			},
			{
			    "name@ci": "abc_ci",
			    "name@hi": "abc_hi",
			    "name@ja": "abc_ja"
			},
			{
				"name@en": "abc_en",
				"name@nl": "abc_nl",
				"name": "abc",
				"number": 99,
				"list": [
					"second",
					"first"
				],
				"list@en": "first_en",
				"list@it": "first_it"
			}
		]
	}
	`, string(resp.GetJson()))
}

func ListWithLanguagesTest(t *testing.T, c *dgo.Dgraph) {
	ctx := context.Background()

	check(t, (c.Alter(ctx, &api.Operation{
		Schema: `pred: [string] @lang .`,
	})))

	txn := c.NewTxn()
	defer txn.Discard(ctx)
	_, err := txn.Mutate(ctx, &api.Mutation{
		CommitNow: true,
		SetNquads: []byte(`
			<0x1> <pred> "first" .
			<0x1> <pred> "second" .
			<0x1> <pred> "dutch"@nl .
		`),
	})
	check(t, err)

	resp, err := c.NewTxn().Query(context.Background(), `
	{
		q(func: uid(0x1)) {
			pred
		}
	}
	`)
	check(t, err)
	CompareJSON(t, `
	{
		"q": [
			{
				"pred": [
					"first",
					"second"
				]
			}
		]
	}
	`, string(resp.GetJson()))

	resp, err = c.NewTxn().Query(context.Background(), `
	{
		q(func: uid(0x1)) {
			pred@nl
		}
	}
	`)
	check(t, err)
	CompareJSON(t, `
	{
		"q": [
			{
				"pred@nl": "dutch"
			}
		]
	}
	`, string(resp.GetJson()))
}

func NQuadMutationTest(t *testing.T, c *dgo.Dgraph) {
	ctx := context.Background()

	require.NoError(t, c.Alter(ctx, &api.Operation{
		Schema: `xid: string @index(exact) .`,
	}))

	txn := c.NewTxn()
	assigned, err := txn.Mutate(ctx, &api.Mutation{
		SetNquads: []byte(`
			_:breakfast <name> "" .
			_:breakfast <nil_name> "_nil_" .
			_:breakfast <xid> "breakfast" .
			_:breakfast <fruit> _:banana .
			_:breakfast <fruit> _:apple .
			_:breakfast <cereal> _:weetbix .
			_:banana <xid> "banana" .
			_:apple <xid> "apple" .
			_:weetbix <xid> "weetbix" .
		`),
	})
	require.NoError(t, err)
	require.NoError(t, txn.Commit(ctx))

	const breakfastQuery = `
	{
		q(func: eq(xid, "breakfast")) {
			name
			nil_name
			extra
			fruit {
				xid
			}
			cereal {
				xid
			}
		}
	}`

	txn = c.NewTxn()
	resp, err := txn.Query(ctx, breakfastQuery)
	require.NoError(t, err)
	CompareJSON(t, `{ "q": [ {
		"fruit": [
			{ "xid": "apple" },
			{ "xid": "banana" }
		],
		"cereal": [
			{ "xid": "weetbix" }
		],
		"name": "",
		"nil_name": "_nil_"
	}]}`, string(resp.Json))

	txn = c.NewTxn()
	_, err = txn.Mutate(ctx, &api.Mutation{
		DelNquads: []byte(fmt.Sprintf(`
			<%s> <fruit>  <%s> .
			<%s> <cereal> <%s> .
			<%s> <name> * .
			<%s> <nil_name> * .`,
			assigned.Uids["breakfast"], assigned.Uids["banana"],
			assigned.Uids["breakfast"], assigned.Uids["weetbix"],
			assigned.Uids["breakfast"], assigned.Uids["breakfast"])),
	})
	require.NoError(t, err)
	require.NoError(t, txn.Commit(ctx))

	txn = c.NewTxn()
	resp, err = txn.Query(ctx, breakfastQuery)
	require.NoError(t, err)
	CompareJSON(t, `{ "q": [ {
		"fruit": [
			{ "xid": "apple" }
		]
	}]}`, string(resp.Json))
}

func DeleteAllReverseIndex(t *testing.T, c *dgo.Dgraph) {
	ctx := context.Background()
	require.NoError(t, c.Alter(ctx, &api.Operation{Schema: "link: uid @reverse ."}))
	_, err := c.NewTxn().Mutate(ctx, &api.Mutation{
		CommitNow: true,
		SetNquads: []byte("<0x1> <link> <0x2> ."),
	})
	require.NoError(t, err)

	_, err = c.NewTxn().Mutate(ctx, &api.Mutation{
		CommitNow: true,
		DelNquads: []byte("<0x1> <link> * ."),
	})
	resp, err := c.NewTxn().Query(ctx, "{ q(func: uid(0x2)) { ~link { uid } }}")
	require.NoError(t, err)
	CompareJSON(t, `{"q":[]}`, string(resp.Json))

	_, err = c.NewTxn().Mutate(ctx, &api.Mutation{
		CommitNow: true,
		SetNquads: []byte("<0x1> <link> <0x3> ."),
	})
	resp, err = c.NewTxn().Query(ctx, "{ q(func: uid(0x3)) { ~link { uid } }}")
	require.NoError(t, err)
	CompareJSON(t, `{"q":[{"~link": [{"uid": "0x1"}]}]}`, string(resp.Json))
}

func ExpandAllReversePredicatesTest(t *testing.T, c *dgo.Dgraph) {
	ctx := context.Background()

	require.NoError(t, c.Alter(ctx, &api.Operation{
		Schema: `link: uid @reverse .`,
	}))

	_, err := c.NewTxn().Mutate(ctx, &api.Mutation{
		CommitNow: true,
		SetNquads: []byte(`
			# SPO
			<0x1> <link> <0x2> .

			# SP*
			<0x3> <link> <0x4> .
			<0x3> <link> <0x5> .

			# S**
			<0x6> <link> <0x7> .
		`),
	})
	require.NoError(t, err)

	// Make sure expand(_all_) follows reverse edges.
	resp, err := c.NewTxn().Query(ctx, `
		{
			spo(func: uid(0x2)) {
				uid
				expand(_all_) {
					uid
				}
			}
		}
	`)
	require.NoError(t, err)
	CompareJSON(t, `
	{
		"spo": [
			{
				"uid": "0x2",
				"~link": [
					{
						"uid": "0x1"
					}
				]
			}
		]
	}`, string(resp.GetJson()))

	// Delete nodes with reverse edges, and make sure the entries in
	// _predicate_ are removed.
	_, err = c.NewTxn().Mutate(ctx, &api.Mutation{
		CommitNow: true,
		DelNquads: []byte(`
			<0x1> <link> <0x2> .
			<0x3> <link> * .
			<0x6> * * .
		`),
	})
	require.NoError(t, err)

	resp, err = c.NewTxn().Query(ctx, `
		{
			spo(func: uid(0x2)) {
				expand(_all_) {
					uid
				}
			}
			spstar(func: uid(0x4, 0x5)) {
				uid
				expand(_all_) {
					uid
				}
			}
			sstarstar(func: uid(0x7)) {
				uid
				expand(_all_) {
					uid
				}
			}
		}
	`)
	require.NoError(t, err)
	CompareJSON(t, `
		{
		  "spo": [],
		  "spstar": [
			{
			  "uid": "0x4"
			},
			{
			  "uid": "0x5"
			}
		  ],
		  "sstarstar": [
			{
			  "uid": "0x7"
			}
		  ]
		}
	`, string(resp.GetJson()))
}

func NormalizeEdgeCasesTest(t *testing.T, c *dgo.Dgraph) {
	ctx := context.Background()
	require.NoError(t, c.Alter(ctx, &api.Operation{Schema: "xid: string @index(exact) ."}))

	_, err := c.NewTxn().Mutate(ctx, &api.Mutation{
		CommitNow: true,
		SetNquads: []byte(`
			_:root <xid> "root" .
			_:root <link> _:a .
			_:root <link> _:b .
		`),
	})
	require.NoError(t, err)

	for _, test := range []struct{ query, want string }{
		{
			`{ q(func: uid(0x1)) @normalize { count(uid) } } `,
			`{ "q": [] }`,
		},
		{
			`{ q(func: uid(0x1)) @normalize { c: count(uid) } } `,
			`{ "q": [ {"c": 1} ] }`,
		},
		{
			`{ q(func: uid(0x1)) @normalize { count: count(uid) } } `,
			`{ "q": [ {"count": 1} ] }`,
		},
		{
			`{ q(func: eq(xid, "root")) @normalize { link { count(uid) } } } `,
			`{ "q": [] }`,
		},
		{
			`{ q(func: eq(xid, "root")) @normalize { link { c: count(uid) } } } `,
			`{ "q": [ {"c": 2} ] }`,
		},
		{
			`{ q(func: eq(xid, "root")) @normalize { link { count: count(uid) } } } `,
			`{ "q": [ {"count": 2} ] }`,
		},
		{
			`{ q(func: eq(xid, "root")) @normalize { count(link) } } `,
			`{ "q": [] }`,
		},
		{
			`{ q(func: eq(xid, "root")) @normalize { c: count(link) } } `,
			`{ "q": [ {"c": 2} ] }`,
		},
		{
			`{ q(func: eq(xid, "root")) @normalize { count: count(link) } } `,
			`{ "q": [ {"count": 2} ] }`,
		},
		{
			`{ q(func: uid(0x01)) @normalize { one as math(1) uno: val(one) } } `,
			`{ "q": [ {"uno": 1} ] }`,
		},
		{
			`
			{ var(func: uid(0x1, 0x2)) { ones as math(1) }
			q() @normalize { sum(val(ones)) } }
			`,
			`{ "q": [] }`,
		},
		{
			`
			{ var(func: uid(0x1, 0x2)) { ones as math(1) }
			q() @normalize { s: sum(val(ones)) } }
			`,
			`{ "q": [ {"s": 2} ] }`,
		},
	} {
		resp, err := c.NewTxn().Query(ctx, test.query)
		require.NoError(t, err)
		require.JSONEq(t, test.want, string(resp.GetJson()))
	}
}

func FacetOrderTest(t *testing.T, c *dgo.Dgraph) {
	ctx := context.Background()

	require.NoError(t, c.Alter(ctx, &api.Operation{
		Schema: `name: string @index(exact) .`,
	}))

	txn := c.NewTxn()
	_, err := txn.Mutate(ctx, &api.Mutation{
		SetNquads: []byte(`
			_:a <name> "Alice" .
			_:a <friend> _:b (age=13,car="Honda") .
			_:b <name> "Bob" .
			_:a <friend> _:c (age=15,car="Tesla") .
			_:c <name> "Charlie" .
			_:a <friend> _:d .
			_:d <name> "Bubble" .
			_:a <friend> _:e .
			_:a <friend> _:f (age=20,car="Hyundai") .
			_:f <name> "Abc" .
		`),
	})
	require.NoError(t, err)
	require.NoError(t, txn.Commit(ctx))

	const friendQuery = `
	{
		q(func: eq(name, "Alice")) {
			name
			friend(orderdesc: name) @facets {
				name
			}
		}
	}`

	txn = c.NewTxn()
	resp, err := txn.Query(ctx, friendQuery)
	require.NoError(t, err)
	CompareJSON(t, `{
		  "q": [
		    {
		      "friend": [
		        {
		          "friend|age": 15,
		          "friend|car": "Tesla",
		          "name": "Charlie"
		        },
		        {
		          "name": "Bubble"
		        },
		        {
		          "friend|age": 13,
		          "friend|car": "Honda",
		          "name": "Bob"
		        },
		        {
		          "friend|age": 20,
		          "friend|car": "Hyundai",
		          "name": "Abc"
		        }
		      ],
		      "name": "Alice"
		    }
		  ]
		}`, string(resp.Json))

}

// Shows fix for issue #1918.
func LangAndSortBugTest(t *testing.T, c *dgo.Dgraph) {
	ctx := context.Background()
	require.NoError(t, c.Alter(ctx, &api.Operation{Schema: "name: string @index(exact) @lang ."}))

	txn := c.NewTxn()
	_, err := txn.Mutate(ctx, &api.Mutation{
		SetNquads: []byte(`
			_:michael <name> "Michael" .
			_:michael <friend> _:sang .

			_:sang <name> "상현"@ko .
			_:sang <name> "Sang Hyun"@en .
		`),
	})
	resp, err := txn.Query(ctx, `
	{
	  q(func: eq(name, "Michael")) {
	    name
	    friend (orderdesc: name@.) {
	      name@en
	    }
	  }
	}`)
	require.NoError(t, err)
	require.JSONEq(t, `
	{"q":[{
		"name": "Michael",
		"friend": [{ "name@en": "Sang Hyun" }]
	}]}
	`, string(resp.Json))
}

func SortFacetsReturnNil(t *testing.T, c *dgo.Dgraph) {
	ctx := context.Background()

	txn := c.NewTxn()
	assigned, err := txn.Mutate(ctx, &api.Mutation{
		SetNquads: []byte(`
			_:michael <name> "Michael" .
			_:michael <friend> _:sang (since=2012-01-02) .
			_:sang <name> "Sang Hyun" .

			_:michael <friend> _:alice (since=2014-01-02) .
			_:alice <name> "Alice" .


			_:michael <friend> _:charlie .
			_:charlie <name> "Charlie" .
			`),
	})

	michael := assigned.Uids["michael"]
	resp, err := txn.Query(ctx, `
	{
	  q(func: uid(`+michael+`)) {
	    name
	    friend @facets(orderdesc: since) {
	      name
	    }
	  }
	}`)
	require.NoError(t, err)
	require.JSONEq(t, `
	{"q":[{"name":"Michael","friend":[{"name":"Charlie"},{"name":"Alice","friend|since":"2014-01-02T00:00:00Z"},{"name":"Sang Hyun","friend|since":"2012-01-02T00:00:00Z"}]}]}
		`, string(resp.Json))
}

func SchemaAfterDeleteNode(t *testing.T, c *dgo.Dgraph) {
	ctx := context.Background()

	require.NoError(t, c.Alter(ctx, &api.Operation{Schema: "married: bool ."}))

	txn := c.NewTxn()
	assigned, err := txn.Mutate(ctx, &api.Mutation{
		CommitNow: true,
		SetNquads: []byte(`
			_:michael <name> "Michael" .
			_:michael <friend> _:sang .
			`),
	})
	michael := assigned.Uids["michael"]

	sortSchema := func(schema []*api.SchemaNode) {
		sort.Slice(schema, func(i, j int) bool {
			return schema[i].Predicate < schema[j].Predicate
		})
	}

	resp, err := c.NewTxn().Query(ctx, `schema{}`)
	require.NoError(t, err)
	sortSchema(resp.Schema)
	b, err := json.Marshal(resp.Schema)
	require.NoError(t, err)
	require.JSONEq(t, `[{"predicate":"_predicate_","type":"string","list":true},{"predicate":"friend","type":"uid"},{"predicate":"married","type":"bool"},{"predicate":"name","type":"default"}]`, string(b))

	require.NoError(t, c.Alter(ctx, &api.Operation{DropAttr: "married"}))

	// Lets try to do a S P * deletion. Schema for married shouldn't be rederived.
	_, err = c.NewTxn().Mutate(ctx, &api.Mutation{
		CommitNow: true,
		DelNquads: []byte(`
		    <` + michael + `> * * .
			`),
	})
	require.NoError(t, err)

	resp, err = c.NewTxn().Query(ctx, `schema{}`)
	require.NoError(t, err)
	sortSchema(resp.Schema)
	b, err = json.Marshal(resp.Schema)
	require.NoError(t, err)
	require.JSONEq(t, `[{"predicate":"_predicate_","type":"string","list":true},{"predicate":"friend","type":"uid"},{"predicate":"name","type":"default"}]`, string(b))
}

func FullTextEqual(t *testing.T, c *dgo.Dgraph) {
	ctx := context.Background()

	require.NoError(t, c.Alter(ctx, &api.Operation{Schema: "text: string @index(fulltext) ."}))

	texts := []string{"bat man", "aqua man", "bat cave", "bat", "man", "aqua", "cave"}
	var rdfs bytes.Buffer
	for i, text := range texts {
		fmt.Fprintf(&rdfs, "_:node%d <text> %q .\n", i, text)
	}
	txn := c.NewTxn()
	_, err := txn.Mutate(ctx, &api.Mutation{
		CommitNow: true,
		SetNquads: rdfs.Bytes(),
	})
	require.NoError(t, err)

	for _, text := range texts {
		resp, err := c.NewTxn().Query(ctx, `
		{
			q(func: eq(text, "`+text+`")) {
				text
			}
		}`)
		require.NoError(t, err)
		require.Equal(t, `{"q":[{"text":"`+text+`"}]}`, string(resp.GetJson()))
	}

	for _, bad := range []string{"cave dweller", "bat ears", "elephant"} {
		resp, err := c.NewTxn().Query(ctx, `
		{
			q(func: eq(text, "`+bad+`")) {
				text
			}
		}`)
		require.NoError(t, err)
		require.Equal(t, `{"q":[]}`, string(resp.GetJson()))
	}
}

func JSONBlankNode(t *testing.T, c *dgo.Dgraph) {
	ctx := context.Background()

	txn := c.NewTxn()
	assigned, err := txn.Mutate(ctx, &api.Mutation{
		SetJson: []byte(`
			{"uid": "_:michael", "name": "Michael", "friend": [{ "uid": "_:sang", "name": "Sang Hyun"}, { "uid": "_:alice", "name": "Alice"}]}
		`),
		CommitNow: true,
	})
	require.NoError(t, err)

	require.Equal(t, 3, len(assigned.Uids))
	michael := assigned.Uids["michael"]
	alice := assigned.Uids["alice"]
	resp, err := c.NewTxn().Query(ctx, `
	{
	  q(func: uid(`+michael+`)) {
	    name
	    friend @filter(uid(`+alice+`)){
	      name
	    }
	  }
	}`)
	require.NoError(t, err)
	require.JSONEq(t, `{"q":[{"name":"Michael","friend":[{"name":"Alice"}]}]}`, string(resp.Json))
}

func ScalarToList(t *testing.T, c *dgo.Dgraph) {
	ctx := context.Background()

	require.NoError(t, c.Alter(ctx, &api.Operation{Schema: `pred: string @index(exact) .`}))

	uids, err := c.NewTxn().Mutate(ctx, &api.Mutation{
		SetNquads: []byte(`_:blank <pred> "first" .`),
		CommitNow: true,
	})
	require.NoError(t, err)
	uid := uids.Uids["blank"]

	q := `
	{
		me(func: eq(pred, "first")) {
			pred
		}
	}
	`
	resp, err := c.NewTxn().Query(ctx, q)
	require.NoError(t, err)
	require.Equal(t, `{"me":[{"pred":"first"}]}`, string(resp.Json))

	require.NoError(t, c.Alter(ctx, &api.Operation{Schema: `pred: [string] @index(exact) .`}))
	resp, err = c.NewTxn().Query(ctx, q)
	require.NoError(t, err)
	require.Equal(t, `{"me":[{"pred":["first"]}]}`, string(resp.Json))

	_, err = c.NewTxn().Mutate(ctx, &api.Mutation{
		SetNquads: []byte(`<` + uid + `> <pred> "second" .`),
		CommitNow: true,
	})
	require.NoError(t, err)

	resp, err = c.NewTxn().Query(ctx, q)
	require.NoError(t, err)
	require.Equal(t, `{"me":[{"pred":["second","first"]}]}`, string(resp.Json))

	q2 := `
	{
		me(func: eq(pred, "second")) {
			pred
		}
	}
	`
	resp, err = c.NewTxn().Query(ctx, q2)
	require.NoError(t, err)
	require.Equal(t, `{"me":[{"pred":["second","first"]}]}`, string(resp.Json))

	_, err = c.NewTxn().Mutate(ctx, &api.Mutation{
		DelNquads: []byte(`<` + uid + `> <pred> "second" .`),
		SetNquads: []byte(`<` + uid + `> <pred> "third" .`),
		CommitNow: true,
	})
	require.NoError(t, err)

	resp, err = c.NewTxn().Query(ctx, q)
	require.NoError(t, err)
	require.Equal(t, `{"me":[{"pred":["third","first"]}]}`, string(resp.Json))

	_, err = c.NewTxn().Mutate(ctx, &api.Mutation{
		DelNquads: []byte(`<` + uid + `> <pred> "first" .`),
		CommitNow: true,
	})
	require.NoError(t, err)

	q3 := `
	{
		me(func: eq(pred, "third")) {
			pred
		}
	}
	`
	resp, err = c.NewTxn().Query(ctx, q3)
	require.NoError(t, err)
	require.Equal(t, `{"me":[{"pred":["third"]}]}`, string(resp.Json))

	resp, err = c.NewTxn().Query(ctx, q)
	require.NoError(t, err)
	require.Equal(t, `{"me":[]}`, string(resp.Json))

	resp, err = c.NewTxn().Query(ctx, q2)
	require.NoError(t, err)
	require.Equal(t, `{"me":[]}`, string(resp.Json))
}

func ListToScalar(t *testing.T, c *dgo.Dgraph) {
	ctx := context.Background()

	require.NoError(t, c.Alter(ctx, &api.Operation{Schema: `pred: [string] @index(exact) .`}))

	err := c.Alter(ctx, &api.Operation{Schema: `pred: string @index(exact) .`})
	require.Error(t, err)
	require.Contains(t, err.Error(), `Type can't be changed from list to scalar for attr: [pred] without dropping it first.`)

	require.NoError(t, c.Alter(ctx, &api.Operation{DropAttr: `pred`}))
	err = c.Alter(ctx, &api.Operation{Schema: `pred: string @index(exact) .`})
	require.NoError(t, err)
}

func SetAfterDeletionListType(t *testing.T, c *dgo.Dgraph) {
	ctx := context.Background()

	require.NoError(t, c.Alter(ctx, &api.Operation{Schema: `
		property.test: [string] .
	`}))

	m1 := []byte(`
		_:alice <property.test> "initial value" .
	`)
	txn := c.NewTxn()
	assigned, err := txn.Mutate(context.Background(),
		&api.Mutation{SetNquads: m1},
	)
	require.NoError(t, err)
	alice := assigned.Uids["alice"]
	require.NoError(t, txn.Commit(ctx))

	q := `
	{
		me(func: uid(` + alice + `)) {
			property.test
		}
	}
	`
	resp, err := c.NewTxn().Query(ctx, q)
	require.NoError(t, err)
	require.Equal(t, `{"me":[{"property.test":["initial value"]}]}`, string(resp.Json))

	txn = c.NewTxn()
	_, err = txn.Mutate(ctx, &api.Mutation{
		DelNquads: []byte(`<` + alice + `> <property.test> * .`),
	})
	require.NoError(t, err)
	resp, err = txn.Query(ctx, q)
	require.NoError(t, err)
	require.Equal(t, `{"me":[]}`, string(resp.Json))

	_, err = txn.Mutate(ctx, &api.Mutation{
		SetNquads: []byte(`<` + alice + `> <property.test> "rewritten value". `),
	})
	require.NoError(t, err)
	require.NoError(t, txn.Commit(ctx))

	resp, err = c.NewTxn().Query(ctx, q)
	require.NoError(t, err)
	require.Equal(t, `{"me":[{"property.test":["rewritten value"]}]}`, string(resp.Json))
}

func EmptyNamesWithExact(t *testing.T, c *dgo.Dgraph) {
	ctx := context.Background()
	err := c.Alter(ctx, &api.Operation{Schema: `name: string @index(exact) .`})
	require.NoError(t, err)

	_, err = c.NewTxn().Mutate(ctx, &api.Mutation{
		SetNquads: []byte(`
			_:alex <name> "Alex" .
			_:alex <name@en> "Alex" .
			_:y <name> "" .
			_:y <name@ko> "상현" .
			_:amit <name> "" .
			_:amit <name@en> "" .
			_:amit <name@hi> "अमित" .
		`),
		CommitNow: true,
	})
	require.NoError(t, err)

	resp, err := c.NewTxn().Query(ctx, `{
	  names(func: has(name)) @filter(eq(name, "")) {
		count(uid)
	  }
	}`)
	require.NoError(t, err)

	require.Equal(t, `{"names":[{"count":2}]}`, string(resp.Json))
}

func EmptyRoomsWithTermIndex(t *testing.T, c *dgo.Dgraph) {
	op := &api.Operation{}
	op.Schema = `
		room: string @index(term) .
		office.room: uid .
	`
	ctx := context.Background()
	err := c.Alter(ctx, op)
	require.NoError(t, err)

	_, err = c.NewTxn().Mutate(ctx, &api.Mutation{
		SetNquads: []byte(`
			_:o <office> "office 1" .
			_:o <office.room> _:x .
			_:o <office.room> _:y .
			_:o <office.room> _:z .
			_:x <room> "room 1" .
			_:y <room> "room 2" .
			_:z <room> "" .
		`),
		CommitNow: true,
	})
	require.NoError(t, err)

	resp, err := c.NewTxn().Query(ctx, `{
		  offices(func: has(office)) {
			count(office.room @filter(eq(room, "")))
		  }
		}`)
	require.NoError(t, err)
	require.Equal(t, `{"offices":[{"count(office.room)":1}]}`, string(resp.GetJson()))
}

func DeleteWithExpandAll(t *testing.T, c *dgo.Dgraph) {
	ctx := context.Background()
	assigned, err := c.NewTxn().Mutate(ctx, &api.Mutation{
		SetNquads: []byte(`
			_:a <to> _:b .
			_:b <name> "b" .
			_:a <to> _:c .
			_:a <to> _:d .
		`),
		CommitNow: true,
	})
	require.NoError(t, err)
	auid := assigned.Uids["a"]
	buid := assigned.Uids["b"]

	_, err = c.NewTxn().Mutate(ctx, &api.Mutation{
		DelNquads: []byte(`
			<` + auid + `> <to> <` + buid + `> .
			<` + buid + `> * * .
		`),
		CommitNow: true,
	})
	require.NoError(t, err)

	q := `query test($id: string) {
		  me(func: uid($id)) {
			_predicate_
		  }
		}`

	type Preds struct {
		Predicates []string `json:"_predicate_"`
	}

	type Root struct {
		Me []Preds `json:"me"`
	}

	var r Root
	resp, err := c.NewTxn().QueryWithVars(ctx, q, map[string]string{"$id": auid})
	require.NoError(t, err)
	json.Unmarshal(resp.Json, &r)
	// S P O deletion shouldn't delete "to" .
	require.Equal(t, 1, len(r.Me[0].Predicates))
	require.Equal(t, "to", r.Me[0].Predicates[0])

	// b should not have any _predicate_.
	resp, err = c.NewTxn().QueryWithVars(ctx, q, map[string]string{"$id": buid})
	require.NoError(t, err)
	json.Unmarshal(resp.Json, &r)
	require.Equal(t, 0, len(r.Me))
}

func FacetsUsingNQuadsError(t *testing.T, c *dgo.Dgraph) {
	nquads := []*api.NQuad{
		&api.NQuad{
			Subject:   "0x01",
			Predicate: "friend",
			ObjectId:  "0x02",
			Facets: []*api.Facet{
				{
					Key:     "since",
					Value:   []byte(time.Now().Format(time.RFC3339)),
					ValType: api.Facet_DATETIME,
				},
			},
		},
	}
	mu := &api.Mutation{Set: nquads, CommitNow: true}
	ctx := context.Background()
	_, err := c.NewTxn().Mutate(ctx, mu)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Error while parsing facet")

	nquads[0].Facets[0].Value, err = time.Now().MarshalBinary()
	require.NoError(t, err)

	mu = &api.Mutation{Set: nquads, CommitNow: true}
	_, err = c.NewTxn().Mutate(context.Background(), mu)
	require.NoError(t, err)

	q := `query test($id: string) {
		  me(func: uid($id)) {
			friend @facets
		  }
		}`

	resp, err := c.NewTxn().QueryWithVars(ctx, q, map[string]string{"$id": "0x1"})
	require.NoError(t, err)
	require.Contains(t, string(resp.Json), "since")
}

func SkipEmptyPLForHas(t *testing.T, c *dgo.Dgraph) {
	ctx := context.Background()
	_, err := c.NewTxn().Mutate(ctx, &api.Mutation{
		SetNquads: []byte(`
			_:u <__user> "true" .
			_:u <name> "u" .
			_:u1 <__user> "true" .
			_:u1 <name> "u1" .
		`),
		CommitNow: true,
	})
	require.NoError(t, err)

	q := `{
		  users(func: has(__user)) {
			name
		  }
		}`
	resp, err := c.NewTxn().Query(ctx, q)
	require.NoError(t, err)
	CompareJSON(t, `{"users":[{"name":"u"},{"name":"u1"}]}`, string(resp.Json))

	op := &api.Operation{DropAll: true}
	err = c.Alter(ctx, op)
	require.NoError(t, err)

	q = `{
		  users(func: has(__user)) {
			uid
			name
		  }
		}`
	resp, err = c.NewTxn().Query(ctx, q)
	require.NoError(t, err)
	require.JSONEq(t, `{"users": []}`, string(resp.Json))
}

func FacetExpandAll(t *testing.T, c *dgo.Dgraph) {
	ctx := context.Background()

	check(t, (c.Alter(ctx, &api.Operation{
		Schema: `name: string @index(hash) .
				friend: uid @reverse .`,
	})))

	txn := c.NewTxn()
	_, err := txn.Mutate(ctx, &api.Mutation{
		SetNquads: []byte(`
			_:a <name> "Alice" (from="US",to="Canada") .
			_:a <friend> _:b (age=13,car="Honda") .
			_:b <name> "Bob" (from="Toronto",to="Vancouver").
			_:a <friend> _:c (age=15,car="Tesla") .
			_:c <name> "Charlie" .
		`),
	})
	require.NoError(t, err)
	require.NoError(t, txn.Commit(ctx))

	const friendQuery = `
	{
		q(func: eq(name, "Alice")) {
			expand(_all_) {
				expand(_all_)
			}
		}
	}`

	txn = c.NewTxn()
	resp, err := txn.Query(ctx, friendQuery)
	require.NoError(t, err)
	CompareJSON(t, `{
  "q": [
    {
      "friend": [
        {
          "friend|age": 13,
          "friend|car": "Honda",
          "name": "Bob",
          "name|from": "Toronto",
          "name|to": "Vancouver",
          "~friend": [
            {
              "~friend|age": 13,
              "~friend|car": "Honda"
            }
          ]
        },
        {
          "friend|age": 15,
          "friend|car": "Tesla",
          "name": "Charlie",
          "~friend": [
            {
              "~friend|age": 15,
              "~friend|car": "Tesla"
            }
          ]
        }
      ],
      "name": "Alice",
      "name|from": "US",
      "name|to": "Canada"
    }
  ]
}`, string(resp.Json))
}

func HasWithDash(t *testing.T, c *dgo.Dgraph) {
	ctx := context.Background()

	check(t, (c.Alter(ctx, &api.Operation{
		Schema: `name: string @index(hash) .`,
	})))

	txn := c.NewTxn()
	_, err := txn.Mutate(ctx, &api.Mutation{
		SetNquads: []byte(`
			_:a <name> "Alice" .
			_:a <new-friend> _:b .
			_:b <name> "Bob" .
			_:a <new-friend> _:c .
			_:c <name> "Charlie" .
		`),
	})
	require.NoError(t, err)
	require.NoError(t, txn.Commit(ctx))

	const friendQuery = `
	{
		q(func: has(new-friend)) {
			new-friend {
				name
			}
		}
	}`

	txn = c.NewTxn()
	resp, err := txn.Query(ctx, friendQuery)
	require.NoError(t, err)
	CompareJSON(t, `{"q":[{"new-friend":[{"name":"Bob"},{"name":"Charlie"}]}]}`, string(resp.Json))
}

func ListGeoFilterTest(t *testing.T, c *dgo.Dgraph) {
	ctx := context.Background()

	check(t, c.Alter(ctx, &api.Operation{
		Schema: `
			name: string @index(term) .
			loc: [geo] @index(geo) .
		`,
	}))

	txn := c.NewTxn()
	defer txn.Discard(ctx)
	_, err := txn.Mutate(ctx, &api.Mutation{
		CommitNow: true,
		SetNquads: []byte(`
			_:a <name> "read"  .
			_:b <name> "write" .
			_:c <name> "admin" .

			_:a <loc> "{'type':'Point','coordinates':[-122.4220186,37.772318]}"^^<geo:geojson> .
			_:b <loc> "{'type':'Point','coordinates':[-62.4220186,37.772318]}"^^<geo:geojson>  .
			_:c <loc> "{'type':'Point','coordinates':[-62.4220186,37.772318]}"^^<geo:geojson>  .
			_:c <loc> "{'type':'Point','coordinates':[-122.4220186,37.772318]}"^^<geo:geojson> .
		`),
	})
	check(t, err)

	resp, err := c.NewTxn().Query(context.Background(), `{
		q(func: near(loc, [-122.4220186,37.772318], 1000)) {
			name
		}
	}`)
	check(t, err)
	CompareJSON(t, `
	{
		"q": [
			{
				"name": "read"
			},
			{
				"name": "admin"
			}
		]
	}
	`, string(resp.GetJson()))
}

func ListRegexFilterTest(t *testing.T, c *dgo.Dgraph) {
	ctx := context.Background()

	check(t, c.Alter(ctx, &api.Operation{
		Schema: `
			name: string @index(term) .
			per: [string] @index(trigram) .
		`,
	}))

	txn := c.NewTxn()
	defer txn.Discard(ctx)
	_, err := txn.Mutate(ctx, &api.Mutation{
		CommitNow: true,
		SetNquads: []byte(`
			_:a <name> "read"  .
			_:b <name> "write" .
			_:c <name> "admin" .

			_:a <per> "read"  .
			_:b <per> "write" .
			_:c <per> "read"  .
			_:c <per> "write" .
		`),
	})
	check(t, err)

	resp, err := c.NewTxn().Query(context.Background(), `{
		q(func: regexp(per, /^rea.*$/)) {
			name
		}
	}`)
	check(t, err)
	CompareJSON(t, `
	{
		"q": [
			{
				"name": "read"
			},
			{
				"name": "admin"
			}
		]
	}
	`, string(resp.GetJson()))
}

func RegexQueryWithVars(t *testing.T, c *dgo.Dgraph) {
	ctx := context.Background()

	check(t, c.Alter(ctx, &api.Operation{
		Schema: `
			name: string @index(term) .
			per: [string] @index(trigram) .
		`,
	}))

	txn := c.NewTxn()
	defer txn.Discard(ctx)
	_, err := txn.Mutate(ctx, &api.Mutation{
		CommitNow: true,
		SetNquads: []byte(`
			_:a <name> "read"  .
			_:b <name> "write" .
			_:c <name> "admin" .

			_:a <per> "read"  .
			_:b <per> "write" .
			_:c <per> "read"  .
			_:c <per> "write" .
		`),
	})
	check(t, err)

	resp, err := c.NewTxn().QueryWithVars(context.Background(), `
		query search($term: string){
			q(func: regexp(per, $term)) {
				name
			}
		}`, map[string]string{"$term": "/^rea.*$/"})
	check(t, err)
	CompareJSON(t, `
	{
		"q": [
			{
				"name": "read"
			},
			{
				"name": "admin"
			}
		]
	}
	`, string(resp.GetJson()))
}

func GraphQLVarChild(t *testing.T, c *dgo.Dgraph) {
	ctx := context.Background()

	check(t, c.Alter(ctx, &api.Operation{
		Schema: `
			name: string @index(exact) .
		`,
	}))

	txn := c.NewTxn()
	defer txn.Discard(ctx)
	au, err := txn.Mutate(ctx, &api.Mutation{
		CommitNow: true,
		SetNquads: []byte(`
			_:a <name> "Alice"  .
			_:b <name> "Bob" .
			_:c <name> "Charlie" .

			_:a <friend> _:c  .
			_:a <friend> _:b  .
		`),
	})
	check(t, err)

	a := au.Uids["a"]
	b := au.Uids["b"]
	ch := au.Uids["c"]

	// Try GraphQL variable with filter at root.
	resp, err := c.NewTxn().QueryWithVars(context.Background(), `
	query q($alice: string){
		q(func: eq(name, "Alice")) @filter(uid($alice)) {
			name
		}
	}`, map[string]string{"$alice": a})
	check(t, err)
	CompareJSON(t, `
	{
		"q": [
			{
				"name": "Alice"
			}
		]
	}
	`, string(resp.GetJson()))

	// Try GraphQL variable with filter at child.
	resp, err = c.NewTxn().QueryWithVars(context.Background(), `
	query q($bob: string){
		q(func: eq(name, "Alice")) {
			name
			friend @filter(uid($bob)) {
				name
			}
		}
	}`, map[string]string{"$bob": b})
	check(t, err)
	CompareJSON(t, `
	{
		"q": [
			{
				"name": "Alice",
				"friend": [
					{
						"name": "Bob"
					}
				]
			}
		]
	}
	`, string(resp.GetJson()))

	// Try GraphQL array variable with filter at child.
	friends := "[" + b + "," + ch + "]"
	resp, err = c.NewTxn().QueryWithVars(context.Background(), `
	query q($friends: string){
		q(func: eq(name, "Alice")) {
			name
			friend @filter(uid($friends)) {
				name
			}
		}
	}`, map[string]string{"$friends": friends})
	check(t, err)
	CompareJSON(t, `
	{
		"q": [
			{
				"name": "Alice",
				"friend": [
					{
						"name": "Bob"
					},
					{
						"name": "Charlie"
					}
				]
			}
		]
	}
	`, string(resp.GetJson()))

}

func MathGe(t *testing.T, c *dgo.Dgraph) {
	ctx := context.Background()

	check(t, c.Alter(ctx, &api.Operation{
		Schema: `
			name: string @index(exact) .
		`,
	}))

	txn := c.NewTxn()
	defer txn.Discard(ctx)
	_, err := txn.Mutate(ctx, &api.Mutation{
		CommitNow: true,
		SetNquads: []byte(`
			_:root <name> "/" .
			_:root <container> _:c .
			_:c <name> "Foobar" .
		`),
	})
	check(t, err)

	// Try GraphQL variable with filter at root.
	resp, err := c.NewTxn().Query(context.Background(), `
	{
		q(func: eq(name, "/")) {
			containerCount as count(container)
			hasChildren: math(containerCount >= 1)
		}
	}`)
	check(t, err)
	CompareJSON(t, `
		{
		  "q": [
		    {
		      "count(container)": 1,
		      "hasChildren": true
		    }
		  ]
		}
	`, string(resp.GetJson()))
}
