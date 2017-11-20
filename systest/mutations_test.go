package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	"github.com/dgraph-io/dgraph/client"
	"github.com/dgraph-io/dgraph/protos"
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
				context.Background(), &protos.Operation{DropAll: true}))
			fn(t, cluster.client)
		}
	}

	t.Run("n-quad mutation", wrap(NQuadMutationTest))
	t.Run("expand all lang test", wrap(ExpandAllLangTest))
	t.Run("list with languages", wrap(ListWithLanguagesTest))
}

func ExpandAllLangTest(t *testing.T, c *client.Dgraph) {
	ctx := context.Background()

	check(t, (c.Alter(ctx, &protos.Operation{
		Schema: `list: [string] .`,
	})))

	txn := c.NewTxn()
	defer txn.Discard(ctx)
	_, err := txn.Mutate(ctx, &protos.Mutation{
		CommitNow: true,
		SetNquads: []byte(`
			<0x1> <name> "abc" .
			<0x1> <name> "abc_en"@en .
			<0x1> <name> "abc_nl"@nl .
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
			q(func: uid(0x1)) {
				expand(_all_)
			}
		}
		`)
	check(t, err)

	CompareJSON(t, `
		{
			"q": [
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

	check(t, (c.Alter(ctx, &protos.Operation{
		Schema: `pred: [string] .`,
	})))

	txn := c.NewTxn()
	defer txn.Discard(ctx)
	_, err := txn.Mutate(ctx, &protos.Mutation{
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

	require.NoError(t, c.Alter(ctx, &protos.Operation{
		Schema: `xid: string @index(exact) .`,
	}))

	txn := c.NewTxn()
	assigned, err := txn.Mutate(ctx, &protos.Mutation{
		SetNquads: []byte(`
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
		]
	}]}`, string(resp.Json))

	txn = c.NewTxn()
	_, err = txn.Mutate(ctx, &protos.Mutation{
		DelNquads: []byte(fmt.Sprintf(`
			<%s> <fruit>  <%s> .
			<%s> <cereal> <%s> .`,
			assigned.Uids["breakfast"], assigned.Uids["banana"],
			assigned.Uids["breakfast"], assigned.Uids["weetbix"])),
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
