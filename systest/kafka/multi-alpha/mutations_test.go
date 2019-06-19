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

var sourceAlpha = "localhost:9180"
var dstAlphas = []string{"localhost:9280", "localhost:9281", "localhost:9282"}

func TestSystem(t *testing.T) {
	wrap := func(fn func(*testing.T, *dgo.Dgraph, []*dgo.Dgraph)) func(*testing.T) {
		return func(t *testing.T) {
			dgSrc := z.DgraphClient(sourceAlpha)
			var dgsDst []*dgo.Dgraph
			for _, alpha := range dstAlphas {
				dgsDst = append(dgsDst, z.DgraphClient(alpha))
			}

			require.NoError(t, dgSrc.Alter(
				context.Background(), &api.Operation{DropAll: true}))
			fn(t, dgSrc, dgsDst)
		}
	}

	t.Run("n-quad mutation", wrap(NQuadMutationTest))
}

func NQuadMutationTest(t *testing.T, dgSrc *dgo.Dgraph, dgsDst []*dgo.Dgraph) {
	ctx := context.Background()

	require.NoError(t, dgSrc.Alter(ctx, &api.Operation{
		Schema: `name: string @index(exact) .`,
	}))

	txn := dgSrc.NewTxn()
	assigned, err := txn.Mutate(ctx, &api.Mutation{
		SetNquads: []byte(`
			_:michael <name> "Michael" .
		`),
	})
	require.NoError(t, err)
	require.NoError(t, txn.Commit(ctx))
	delay := 2 * time.Second

	// sleep for 2 seconds for the replication to finish
	time.Sleep(delay)

	const query = `
	{
		q(func: eq(name, "Michael")) {
			name
		}
	}`

	for _, dgDst := range dgsDst {
		txn = dgDst.NewReadOnlyTxn().BestEffort()
		resp, err := txn.Query(ctx, query)
		require.NoError(t, err)
		z.CompareJSON(t, `{ "q": [ {
		"name": "Michael"
	}]}`, string(resp.Json))
	}

	//delete data in the source cluster
	txn = dgSrc.NewTxn()
	_, err = txn.Mutate(ctx, &api.Mutation{
		DelNquads: []byte(fmt.Sprintf(`
			<%s> <name>  * .`,
			assigned.Uids["michael"])),
	})
	require.NoError(t, err)
	require.NoError(t, txn.Commit(ctx))
	// make sure the data has been deleted in the source cluster
	txn = dgSrc.NewReadOnlyTxn()
	resp, err := txn.Query(ctx, query)
	require.NoError(t, err)
	require.True(t, z.CompareJSON(t, `{ "q": []}`, string(resp.Json)))

	// sleep for 2 seconds for the replication to finish
	time.Sleep(delay)

	// run the query again in the dst cluster
	for _, dgDst := range dgsDst {
		txn = dgDst.NewReadOnlyTxn().BestEffort()
		resp, err = txn.Query(ctx, query)
		require.NoError(t, err)
		require.True(t, z.CompareJSON(t, `{ "q": []}`, string(resp.Json)))
	}
}
