package kafka

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/dgraph-io/dgo"
	"github.com/dgraph-io/dgo/protos/api"
	"github.com/dgraph-io/dgraph/z"
	"github.com/stretchr/testify/require"
)

const (
	sourceAlpha = "localhost:9180"
	dstAlpha    = "localhost:9280"
)

func TestSystem(t *testing.T) {
	wrap := func(fn func(*testing.T, *dgo.Dgraph, *dgo.Dgraph)) func(*testing.T) {
		return func(t *testing.T) {
			dgSrc := z.DgraphClient(sourceAlpha)
			dgDst := z.DgraphClient(dstAlpha)
			require.NoError(t, dgSrc.Alter(
				context.Background(), &api.Operation{DropAll: true}))
			fn(t, dgSrc, dgDst)
		}
	}

	t.Run("n-quad mutation", wrap(NQuadMutationTest))
}

func NQuadMutationTest(t *testing.T, dgSrc *dgo.Dgraph, dgDst *dgo.Dgraph) {
	ctx := context.Background()

	require.NoError(t, dgSrc.Alter(ctx, &api.Operation{
		Schema: `xid: string @index(exact) .`,
	}))

	txn := dgSrc.NewTxn()
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

	// sleep for 2 seconds for the replication to finish
	time.Sleep(2 * time.Second)

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

	txn = dgDst.NewReadOnlyTxn().BestEffort()
	resp, err := txn.Query(ctx, breakfastQuery)
	require.NoError(t, err)
	z.CompareJSON(t, `{ "q": [ {
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

	// delete data in the source cluster
	txn = dgSrc.NewTxn()
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
	// sleep for 2 seconds for the replication to finish
	time.Sleep(2 * time.Second)

	// run the query again in the dst cluster
	txn = dgDst.NewReadOnlyTxn().BestEffort()
	resp, err = txn.Query(ctx, breakfastQuery)
	require.NoError(t, err)
	z.CompareJSON(t, `{ "q": [ {
		"fruit": [
			{ "xid": "apple" }
		]
	}]}`, string(resp.Json))
}
