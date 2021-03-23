package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os/exec"
	"testing"
	"time"

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
	require.True(t, moveOk)

	// Take an incremental backup
	runBackup(t)

	// Drop everything from the cluster and restore the previously taken backup.
	dg, err := testutil.DgraphClient(testutil.SockAddr)
	require.NoError(t, err)
	testutil.DropAll(t, dg)
	runRestore(t)

	time.Sleep(5 * time.Second)
	// Get the membership state after restore and verify that p1 and p2 belongs to expected groups.
	state, err := testutil.GetState()
	_, ok := state.Groups["1"].Tablets[x.NamespaceAttr(x.GalaxyNamespace, "p1")]
	require.True(t, ok)
	_, ok = state.Groups["2"].Tablets[x.NamespaceAttr(x.GalaxyNamespace, "p2")]
	require.True(t, ok)

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
