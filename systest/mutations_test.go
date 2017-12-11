package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	"github.com/dgraph-io/dgraph/client"
	"github.com/dgraph-io/dgraph/protos/api"
	"github.com/stretchr/testify/require"
)

func TestSystem(t *testing.T) {
	dir, err := ioutil.TempDir("", "")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	cluster := NewDgraphCluster(dir)
	require.NoError(t, cluster.Start())
	defer cluster.Close()

	wrap := func(fn func(*testing.T, *client.Dgraph)) func(*testing.T) {
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
}

func ExpandAllLangTest(t *testing.T, c *client.Dgraph) {
	ctx := context.Background()

	check(t, (c.Alter(ctx, &api.Operation{
		Schema: `list: [string] .`,
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

func ListWithLanguagesTest(t *testing.T, c *client.Dgraph) {
	ctx := context.Background()

	check(t, (c.Alter(ctx, &api.Operation{
		Schema: `pred: [string] .`,
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

func NQuadMutationTest(t *testing.T, c *client.Dgraph) {
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

func DeleteAllReverseIndex(t *testing.T, c *client.Dgraph) {
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

func ExpandAllReversePredicatesTest(t *testing.T, c *client.Dgraph) {
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

func NormalizeEdgeCasesTest(t *testing.T, c *client.Dgraph) {
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

func FacetOrderTest(t *testing.T, c *client.Dgraph) {
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
		`),
	})
	require.NoError(t, err)
	require.NoError(t, txn.Commit(ctx))

	const breakfastQuery = `
	{
		q(func: eq(name, "Alice")) {
			name
			friend(orderdesc: name) @facets {
				name
			}
		}
	}`

	txn = c.NewTxn()
	resp, err := txn.Query(ctx, breakfastQuery)
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
		          "friend|age": 13,
		          "friend|car": "Honda",
		          "name": "Bob"
		        }
		      ],
		      "name": "Alice"
		    }
		  ]
		}`, string(resp.Json))

}
