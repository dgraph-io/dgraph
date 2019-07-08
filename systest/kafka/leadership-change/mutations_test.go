package kafka

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
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

	t.Run("source cluster leadership change", wrap(SourceLeadershipChangeTest))
	t.Run("destination cluster leadership change", wrap(DstLeadershipChangeTest))
}

// getLeader parses the state response from a zero, and returns the host name of the
// leader in the specified group
func getLeader(groupID string, stateResponse *z.StateResponse) (string, error) {
	group, ok := stateResponse.Groups[groupID]
	if !ok {
		return "", fmt.Errorf("unable to find the group %s in %v", groupID, stateResponse)
	}
	for _, m := range group.Members {
		if m.Leader {
			parts := strings.Split(m.Addr, ":")
			if len(parts) != 2 {
				return "", fmt.Errorf("found invaled host addr: %s", m.Addr)
			}
			return parts[0], nil
		}
	}
	return "", fmt.Errorf("unable to find a leader in group %s, state response is %v",
		groupID, stateResponse)
}

func restartLeader(t *testing.T, zeroAddr string) {
	// query for the leader in group 1
	// by default, the z package uses the port 5180, which points to the zero server in the source
	// cluster
	stateResponse, err := z.GetStateFrom(zeroAddr)
	require.NoError(t, err, "Unable to get state from zero: %v", err)

	leader, err := getLeader("1", stateResponse)
	require.NoError(t, err, "unable to find leader: %v", err)
	glog.Infof("got leader: %s\n", leader)

	restartLeaderCmd := exec.Command("docker", "restart", leader)
	require.NoError(t, restartLeaderCmd.Run(), "Error while trying to stop %s", leader)
	time.Sleep(5 * time.Second)

	newStateResp, err := z.GetStateFrom(zeroAddr)
	require.NoError(t, err, "Unable to get state from zero: %v", err)
	newLeader, err := getLeader("1", newStateResp)
	require.NoError(t, err, "unable to find leader: %v", err)
	glog.Infof("got new leader: %s\n", newLeader)
}

func SourceLeadershipChangeTest(t *testing.T, dgSrc *dgo.Dgraph, dgsDst []*dgo.Dgraph) {
	ctx := context.Background()
	require.NoError(t, dgSrc.Alter(ctx, &api.Operation{
		Schema: `name: string @index(exact) .`,
	}))

	txn1 := dgSrc.NewTxn()
	assigned1, err := txn1.Mutate(ctx, &api.Mutation{
		SetNquads: []byte(`
			_:michael <name> "Michael" .
		`),
	})
	require.NoError(t, err)
	require.NoError(t, txn1.Commit(ctx))

	restartLeader(t, z.SockAddrZero)

	// make one more mutation in the source cluster after the leadership change
	txn2 := dgSrc.NewTxn()
	assigned2, err := txn2.Mutate(ctx, &api.Mutation{
		SetNquads: []byte(`
			_:bob <name> "Bob" .
		`),
	})
	require.NoError(t, err)
	require.NoError(t, txn2.Commit(ctx))

	delay := 5 * time.Second
	// sleep for some time waiting for the replication to finish
	time.Sleep(delay)

	const query = `
	{
		q(func: has(name)) {
			name
		}
	}`

	for idx, dgDst := range dgsDst {
		glog.Infof("querying the %s host", dstAlphas[idx])
		txn := dgDst.NewReadOnlyTxn().BestEffort()
		resp, err := txn.Query(ctx, query)
		require.NoError(t, err)
		require.True(t, z.CompareJSON(t, `{ "q": [
        {"name": "Bob"},
        {"name": "Michael"}]}`, string(resp.Json)))
	}

	//delete data in the source cluster
	txn3 := dgSrc.NewTxn()
	_, err = txn3.Mutate(ctx, &api.Mutation{
		DelNquads: []byte(fmt.Sprintf(`
			<%s> <name>  * .`,
			assigned1.Uids["michael"])),
	})
	require.NoError(t, err)
	require.NoError(t, txn3.Commit(ctx))

	// force a leadership change again
	restartLeader(t, z.SockAddrZero)
	// delete Bob as well
	txn4 := dgSrc.NewTxn()
	_, err = txn4.Mutate(ctx, &api.Mutation{
		DelNquads: []byte(fmt.Sprintf(`
			<%s> <name>  * .`,
			assigned2.Uids["bob"])),
	})
	require.NoError(t, err)
	require.NoError(t, txn4.Commit(ctx))

	// make sure the data has been deleted in the source cluster
	txn5 := dgSrc.NewReadOnlyTxn()
	resp, err := txn5.Query(ctx, query)
	require.NoError(t, err)
	require.True(t, z.CompareJSON(t, `{ "q": []}`, string(resp.Json)))

	// sleep for some time waiting for the replication to finish
	time.Sleep(delay)

	// run the query again in the dst cluster
	for _, dgDst := range dgsDst {
		txn := dgDst.NewReadOnlyTxn().BestEffort()
		resp, err = txn.Query(ctx, query)
		require.NoError(t, err)
		require.True(t, z.CompareJSON(t, `{ "q": []}`, string(resp.Json)))
	}
}

func DstLeadershipChangeTest(t *testing.T, dgSrc *dgo.Dgraph, dgsDst []*dgo.Dgraph) {
	ctx := context.Background()
	require.NoError(t, dgSrc.Alter(ctx, &api.Operation{
		Schema: `name: string @index(exact) .`,
	}))

	txn1 := dgSrc.NewTxn()
	assigned1, err := txn1.Mutate(ctx, &api.Mutation{
		SetNquads: []byte(`
			_:michael <name> "Michael" .
		`),
	})
	require.NoError(t, err)
	require.NoError(t, txn1.Commit(ctx))

	restartLeader(t, "localhost:6280")

	// make one more mutation in the source cluster after the leadership change
	txn2 := dgSrc.NewTxn()
	assigned2, err := txn2.Mutate(ctx, &api.Mutation{
		SetNquads: []byte(`
			_:bob <name> "Bob" .
		`),
	})
	require.NoError(t, err)
	require.NoError(t, txn2.Commit(ctx))

	delay := 5 * time.Second
	// sleep for some time waiting for the replication to finish
	time.Sleep(delay)

	const query = `
	{
		q(func: has(name)) {
			name
		}
	}`

	for idx, dgDst := range dgsDst {
		glog.Infof("querying the %s host", dstAlphas[idx])
		txn := dgDst.NewReadOnlyTxn().BestEffort()
		resp, err := txn.Query(ctx, query)
		require.NoError(t, err)
		require.True(t, z.CompareJSON(t, `{ "q": [
        {"name": "Bob"},
        {"name": "Michael"}]}`, string(resp.Json)))
	}

	//delete data in the source cluster
	txn3 := dgSrc.NewTxn()
	_, err = txn3.Mutate(ctx, &api.Mutation{
		DelNquads: []byte(fmt.Sprintf(`
			<%s> <name>  * .`,
			assigned1.Uids["michael"])),
	})
	require.NoError(t, err)
	require.NoError(t, txn3.Commit(ctx))

	// force a leadership change again
	restartLeader(t, "localhost:6280")
	// delete Bob as well
	txn4 := dgSrc.NewTxn()
	_, err = txn4.Mutate(ctx, &api.Mutation{
		DelNquads: []byte(fmt.Sprintf(`
			<%s> <name>  * .`,
			assigned2.Uids["bob"])),
	})
	require.NoError(t, err)
	require.NoError(t, txn4.Commit(ctx))

	// make sure the data has been deleted in the source cluster
	txn5 := dgSrc.NewReadOnlyTxn()
	resp, err := txn5.Query(ctx, query)
	require.NoError(t, err)
	require.True(t, z.CompareJSON(t, `{ "q": []}`, string(resp.Json)))

	// sleep for some time waiting for the replication to finish
	time.Sleep(delay)

	// run the query again in the dst cluster
	for _, dgDst := range dgsDst {
		txn := dgDst.NewReadOnlyTxn().BestEffort()
		resp, err = txn.Query(ctx, query)
		require.NoError(t, err)
		require.True(t, z.CompareJSON(t, `{ "q": []}`, string(resp.Json)))
	}

}
