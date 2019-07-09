package kafka

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/dgraph-io/dgo"
	"github.com/dgraph-io/dgo/protos/api"
	"github.com/dgraph-io/dgraph/z"
	"github.com/golang/glog"
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

	// create 3 predicates
	require.NoError(t, dgSrc.Alter(ctx, &api.Operation{
		Schema: `pred1: string @index(exact) .
pred2: string @index(exact) .
pred3: string @index(exact) .
link: uid .`,
	}))

	var err error
	// and move the 3 predicates into different groups
	_, err = http.Get("http://" + z.SockAddrZeroHttp + "/moveTablet?tablet=pred1&group=1")
	require.NoError(t, err)
	_, err = http.Get("http://" + z.SockAddrZeroHttp + "/moveTablet?tablet=pred2&group=2")
	require.NoError(t, err)
	_, err = http.Get("http://" + z.SockAddrZeroHttp + "/moveTablet?tablet=pred3&group=3")
	require.NoError(t, err)

	// sleep for a short while for the tablet movement to finish
	time.Sleep(5 * time.Second)

	txn := dgSrc.NewTxn()
	assigned, err := txn.Mutate(ctx, &api.Mutation{
		SetNquads: []byte(`
			_:e1 <pred1> "Pred1 Value" .
            _:e2 <pred2> "Pred2 Value" .
            _:e3 <pred3> "Pred3 Value" .
            _:e1 <link> _:e2 .
            _:e2 <link> _:e3 .
		`),
	})
	require.NoError(t, err)
	require.NoError(t, txn.Commit(ctx))
	delay := 5 * time.Second

	// sleep for some time waiting for the replication to finish
	time.Sleep(delay)
	query := fmt.Sprintf(`
	{
		q(func: uid(%s)) {
			pred1
            link {
              pred2
              link {
                pred3
              }
            }
		}
	}`, assigned.Uids["e1"])

	for idx, dgDst := range dgsDst {
		glog.Infof("querying the %s host", dstAlphas[idx])
		txn = dgDst.NewReadOnlyTxn().BestEffort()
		resp, err := txn.Query(ctx, query)
		require.NoError(t, err)
		require.True(t, z.CompareJSON(t, `{"q": [
  {
   "pred1": "Pred1 Value",
   "link": {
      "pred2": "Pred2 Value",
      "link": {
        "pred3": "Pred3 Value"
      }
    }
  }
]}`, string(resp.Json)))
	}

	//delete data in the source cluster
	txn = dgSrc.NewTxn()
	_, err = txn.Mutate(ctx, &api.Mutation{
		DelNquads: []byte(fmt.Sprintf(`
			<%s> <pred1>  * .
			<%s> <pred2>  * .
            <%s> <pred3>  * .
            <%s> <link>  * .
            <%s> <link>  * .
 `, assigned.Uids["e1"], assigned.Uids["e2"], assigned.Uids["e3"],
			assigned.Uids["e1"], assigned.Uids["e2"])),
	})
	require.NoError(t, err)
	require.NoError(t, txn.Commit(ctx))
	// make sure the data has been deleted in the source cluster
	txn = dgSrc.NewReadOnlyTxn()
	resp, err := txn.Query(ctx, query)
	require.NoError(t, err)
	require.True(t, z.CompareJSON(t, `{ "q": []}`, string(resp.Json)))

	// sleep for some time waiting for the replication to finish
	time.Sleep(delay)

	// run the query again in the dst cluster
	preds := []string{"pred1", "pred2", "pred3"}
	for _, dgDst := range dgsDst {

		for _, pred := range preds {
			txn = dgDst.NewReadOnlyTxn().BestEffort()
			resp, err = txn.Query(ctx,
				fmt.Sprintf(`{q(func:has(%s)){}}`, pred))
			require.NoError(t, err)
			require.True(t, z.CompareJSON(t, `{ "q": []}`, string(resp.Json)))
		}
	}
}
