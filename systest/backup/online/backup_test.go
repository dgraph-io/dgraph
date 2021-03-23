package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os/exec"
	"testing"
	"time"

	"github.com/dgraph-io/dgo/v200/protos/api"
	"github.com/dgraph-io/dgraph/systest/backup/common"
	"github.com/dgraph-io/dgraph/testutil"
	"github.com/dgraph-io/dgraph/x"
	"github.com/stretchr/testify/require"
)

var (
	alphaBackupDir = "/data/backups"
)

func TestBackupWithMoveTablet(t *testing.T) {
	common.DirSetup(t)

	// Call increment on predicate p1 and p2.
	cmd := exec.Command("dgraph", "increment", "--num", "10",
		"--alpha", testutil.SockAddr, "--pred", "p1")
	if out, err := cmd.CombinedOutput(); err != nil {
		fmt.Println(string(out))
		t.Fatal(err)
	}
	cmd = exec.Command("dgraph", "increment", "--num", "10",
		"--alpha", testutil.SockAddr, "--pred", "p2")
	if out, err := cmd.CombinedOutput(); err != nil {
		fmt.Println(string(out))
		t.Fatal(err)
	}
	runBackup(t)

	// Move p1 to group-1 and p2 to group-2
	client := http.Client{}
	_, err := client.Get("http://" + testutil.SockAddrZeroHttp + "/moveTablet?tablet=p1&group=1")
	require.NoError(t, err)
	_, err = client.Get("http://" + testutil.SockAddrZeroHttp + "/moveTablet?tablet=p2&group=2")
	require.NoError(t, err)

	t.Log("Pausing to let zero move tablets...")

	checkTablets := func() bool {
		moveOk := true
		for retry := 60; retry > 0; retry-- {
			time.Sleep(1 * time.Second)
			state, err := testutil.GetState()
			require.NoError(t, err)
			if _, ok := state.Groups["1"].Tablets[x.NamespaceAttr(x.GalaxyNamespace, "p1")]; !ok {
				moveOk = false
			}
			if _, ok := state.Groups["2"].Tablets[x.NamespaceAttr(x.GalaxyNamespace, "p2")]; !ok {
				moveOk = false
			}
			if moveOk {
				break
			}
		}
		return moveOk
	}
	require.True(t, checkTablets())

	// Take an incremental backup
	runBackup(t)

	// Drop everything from the cluster and restore the previously taken backup.
	dg, err := testutil.DgraphClient(testutil.SockAddr)
	require.NoError(t, err)
	testutil.DropAll(t, dg)
	runRestore(t)

	// Get the membership state after restore and verify that p1 and p2 belongs to expected groups.
	require.True(t, checkTablets())

	// Verify the count of p1 and p2
	q1 := `{q(func: has(p1)){ p1 }}`
	q2 := `{q(func: has(p2)){ p2 }}`
	res := testutil.QueryData(t, dg, q1)
	require.NoError(t, err)
	require.JSONEq(t, `{"q":[{"p1":10}]}`, string(res))

	res = testutil.QueryData(t, dg, q2)
	require.NoError(t, err)
	require.JSONEq(t, `{"q":[{"p2":10}]}`, string(res))
	common.DirCleanup(t)
}

func TestBackupBasic(t *testing.T) {
	common.DirSetup(t)
	dg, err := testutil.DgraphClient(testutil.SockAddr)
	require.NoError(t, err)
	testutil.DropAll(t, dg)

	ctx := context.Background()

	_, err = dg.NewTxn().Mutate(ctx, &api.Mutation{
		CommitNow: true,
		SetNquads: []byte(`
			<_:a> <name> "alice" .
			<_:a> <pet> "dog" .
		`),
	})
	require.NoError(t, err)

	runBackup(t)
	testutil.DropAll(t, dg)

	_, err = dg.NewTxn().Mutate(ctx, &api.Mutation{
		CommitNow: true,
		SetNquads: []byte(`
			<_:a> <name> "bob" .
		`),
	})
	runBackup(t)

	testutil.DropAll(t, dg)
	runRestore(t)
	// TODO: Remove this sleep and use retry.
	time.Sleep(5 * time.Second)

	q := `{q(func: has(name)){ name }}`
	res := testutil.QueryData(t, dg, q)
	require.NoError(t, err)
	require.JSONEq(t, `{"q":[{"name": "bob"}]}`, string(res))

	common.DirCleanup(t)
}

func runBackup(t *testing.T) {
	backupRequest := `mutation backup($dst: String!, $ff: Boolean!) {
			backup(input: {destination: $dst, forceFull: $ff}) {
				response {
					code
					message
				}
			}
		}`

	adminUrl := "http://" + testutil.SockAddrHttp + "/admin"
	params := testutil.GraphQLParams{
		Query: backupRequest,
		Variables: map[string]interface{}{
			"dst": alphaBackupDir,
			"ff":  false,
		},
	}
	b, err := json.Marshal(params)
	require.NoError(t, err)

	client := http.Client{}
	resp, err := client.Post(adminUrl, "application/json", bytes.NewBuffer(b))
	require.NoError(t, err)
	defer resp.Body.Close()
	buf, err := ioutil.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Contains(t, string(buf), "Backup completed.")
}

func runRestore(t *testing.T) {
	backupRequest := `mutation restore($location: String!) {
			restore(input: {location: $location}) {
				code
				message
			}
		}`

	adminUrl := "http://" + testutil.SockAddrHttp + "/admin"
	params := testutil.GraphQLParams{
		Query: backupRequest,
		Variables: map[string]interface{}{
			"location": alphaBackupDir,
		},
	}
	b, err := json.Marshal(params)
	require.NoError(t, err)
	client := http.Client{}
	resp, err := client.Post(adminUrl, "application/json", bytes.NewBuffer(b))
	require.NoError(t, err)
	defer resp.Body.Close()
	buf, err := ioutil.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Contains(t, string(buf), "Restore operation started.")

}
