package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/dgraph-io/dgraph/protos"
	"github.com/stretchr/testify/require"
)

func TestNQuadMutation(t *testing.T) {
	dir, err := ioutil.TempDir("", "")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	cluster := NewDgraphCluster(dir)
	require.NoError(t, cluster.Start())
	defer cluster.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	require.NoError(t, cluster.client.Alter(ctx, &protos.Operation{
		Schema: `xid: string @index(exact) .`,
	}))

	txn := cluster.client.NewTxn()
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

	txn = cluster.client.NewTxn()
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

	txn = cluster.client.NewTxn()
	_, err = txn.Mutate(ctx, &protos.Mutation{
		DelNquads: []byte(fmt.Sprintf(`
			<%s> <fruit>  <%s> .
			<%s> <cereal> <%s> .`,
			assigned.Uids["breakfast"], assigned.Uids["banana"],
			assigned.Uids["breakfast"], assigned.Uids["weetbix"])),
	})
	require.NoError(t, err)
	require.NoError(t, txn.Commit(ctx))

	txn = cluster.client.NewTxn()
	resp, err = txn.Query(ctx, breakfastQuery)
	require.NoError(t, err)
	CompareJSON(t, `{ "q": [ {
		"fruit": [
			{ "xid": "apple" }
		]
	}]}`, string(resp.Json))
}
