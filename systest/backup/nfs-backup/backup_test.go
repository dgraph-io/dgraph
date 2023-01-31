/*
 * Copyright 2023 Dgraph Labs, Inc. and Contributors *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dgraph-io/dgraph/systest/backup/common"
	"github.com/dgraph-io/dgraph/testutil"
	"github.com/dgraph-io/dgraph/worker"
	"github.com/dgraph-io/dgraph/x"

	"github.com/dgraph-io/dgo/v210"
	"github.com/dgraph-io/dgo/v210/protos/api"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

var (
	copyBackupDir  = "./data/backups_copy"
	backupDir      = "./data/backup"
	restoreDir     = "./data/restore"
	testDirs       = []string{backupDir, restoreDir}
	backupDstHA    = "/ha_backup"
	backupDstNonHA = "/non_ha_backup"
)

func TestBackupHAClust(t *testing.T) {

	backupRestoreTest(t, testutil.SockAddr, testutil.SockAddrAlpha4Http, testutil.SockAddrZeroHttp, backupDstHA, testutil.SockAddrHttp)
}

func TestBackupNonHAClust(t *testing.T) {

	backupRestoreTest(t, testutil.SockAddrAlpha7, testutil.SockAddrAlpha8Http, testutil.SockAddrZero7Http, backupDstNonHA, testutil.SockAddrAlpha7Http)
}

func backupRestoreTest(t *testing.T, backupAlphaSocketAddr string, restoreAlphaAddr string, backupZeroAddr string, backupDst string, backupAlphaSocketAddrHttp string) {
	conn, err := grpc.Dial(backupAlphaSocketAddr, grpc.WithInsecure())
	require.NoError(t, err)
	dg := dgo.NewDgraphClient(api.NewDgraphClient(conn))
	ctx := context.Background()
	require.NoError(t, dg.Alter(ctx, &api.Operation{DropAll: true}))
	// Add schema and types.
	require.NoError(t, dg.Alter(ctx, &api.Operation{Schema: `movie: string .
		 type Node {
			 movie
		 }`}))

	// Add initial data.
	original, err := dg.NewTxn().Mutate(ctx, &api.Mutation{
		CommitNow: true,
		SetNquads: []byte(`
			 <_:x1> <movie> "BIRDS MAN OR (THE UNEXPECTED VIRTUE OF IGNORANCE)" .
			 <_:x2> <movie> "Spotlight" .
			 <_:x3> <movie> "Moonlight" .
			 <_:x4> <movie> "THE SHAPE OF WATERLOO" .
			 <_:x5> <movie> "BLACK PUNTER" .
		 `),
	})
	require.NoError(t, err)
	totalObjectCount := 5
	t.Logf("--- Original uid mapping: %+v\n", original.Uids)
	// Move tablet to group 1 to avoid messes later.
	_, err = http.Get("http://" + backupZeroAddr + "/moveTablet?tablet=movie&group=1")
	require.NoError(t, err)
	// After the move, we need to pause a bit to give zero a chance to quorum.
	t.Log("Pausing to let zero move tablet...")
	moveOk := false
	for !moveOk {
		state, err := testutil.GetState()
		require.NoError(t, err)
		if _, ok := state.Groups["1"].Tablets[x.NamespaceAttr(x.GalaxyNamespace, "movie")]; ok {
			moveOk = true
			break
		}
	}
	require.True(t, moveOk)
	// Setup test directories.
	dirSetup(t)
	// //mostly because of a race condition adding sleep
	// time.Sleep(time.Second * 10)
	_ = runBackup(t, 1, 1, backupAlphaSocketAddrHttp, backupDst)
	restoreStatusCode := restore(t, "", restoreAlphaAddr, backupDst, "non-incremantal", 1)
	require.Equal(t, "Success", restoreStatusCode)
	testutil.WaitForRestore(t, dg, restoreAlphaAddr)
	// We check expected Objects vs Received Objects from restoreed db
	checkObjectCount(t, totalObjectCount, restoreAlphaAddr)
	// Add more data for the incremental backup.
	incr1, err := dg.NewTxn().Mutate(ctx, &api.Mutation{
		CommitNow: true,
		SetNquads: []byte(`
			 <_:x6> <movie> "Harry Potter Part 1" .
			 <_:x7> <movie> "Harry Potter Part 2" .
		 `),
	})
	t.Logf("%+v", incr1)
	require.NoError(t, err)
	// Perform first incremental backup.
	totalObjectCount = totalObjectCount + 2
	_ = runBackup(t, 2, 2, backupAlphaSocketAddrHttp, backupDst)
	// Add more data for a second incremental backup.
	_, err = dg.NewTxn().Mutate(ctx, &api.Mutation{
		CommitNow: true,
		SetNquads: []byte(`
			 <_:x8> <movie> "The Shape of Water" .
			 <_:x9> <movie> "The Black Panther" .
		 `),
	})
	t.Logf("%+v", incr1)
	require.NoError(t, err)
	_ = runBackup(t, 3, 3, backupAlphaSocketAddrHttp, backupDst)
	restoreStatusCode = restore(t, "", restoreAlphaAddr, backupDst, "incremental", 2)
	require.Equal(t, "Success", restoreStatusCode)
	testutil.WaitForRestore(t, dg, restoreAlphaAddr)
	checkObjectCount(t, totalObjectCount, restoreAlphaAddr)
	restoreStatusCode = restore(t, "", restoreAlphaAddr, backupDst, "incremental", 3)
	require.Equal(t, "Success", restoreStatusCode)
	testutil.WaitForRestore(t, dg, restoreAlphaAddr)
	totalObjectCount = totalObjectCount + 2
	checkObjectCount(t, totalObjectCount, restoreAlphaAddr)
	// Add more data for a second full backup.
	_, err = dg.NewTxn().Mutate(ctx, &api.Mutation{
		CommitNow: true,
		SetNquads: []byte(`
			 <_:x10> <movie> "El laberinto del fauno" .
			 <_:x11> <movie> "Black Panther 2" .
		 `),
	})
	t.Logf("%+v", incr1)
	require.NoError(t, err)
	totalObjectCount = totalObjectCount + 2
	// Perform second full backup.
	_ = runBackupInternal(t, true, 4, 4, backupAlphaSocketAddrHttp, backupDst)
	restoreStatusCode = restore(t, "", restoreAlphaAddr, backupDst, "non-incremental", 4)
	require.Equal(t, "Success", restoreStatusCode)
	testutil.WaitForRestore(t, dg, restoreAlphaAddr)
	checkObjectCount(t, totalObjectCount, restoreAlphaAddr)
	// Do a DROP_DATA
	require.NoError(t, dg.Alter(ctx, &api.Operation{DropOp: api.Operation_DATA}))
	// add some data
	_, err = dg.NewTxn().Mutate(ctx, &api.Mutation{
		CommitNow: true,
		SetNquads: []byte(`
				 <_:x12> <movie> "El laberinto del fauno" .
				 <_:x13> <movie> "Black Panther 2" .
			 `),
	})
	require.NoError(t, err)
	totalObjectCount = 2
	// perform an incremental backup and then restore
	_ = runBackup(t, 5, 5, backupAlphaSocketAddrHttp, backupDst)
	restoreStatusCode = restore(t, "", restoreAlphaAddr, backupDst, "non-incremental", 5)
	require.Equal(t, "Success", restoreStatusCode)
	testutil.WaitForRestore(t, dg, restoreAlphaAddr)
	checkObjectCount(t, totalObjectCount, restoreAlphaAddr)
	// Clean up test directories.
	dirCleanup(t)
}

// function to check object count
func checkObjectCount(t *testing.T, expectedCount int, restoreAlphaAddr string) {
	checkCountRequest := `query {
		 movieCount(func: has(movie)) {
		   count(uid)
		 }
	   }`

	//Check object count from newly created restore alpha
	adminUrl := "http://" + restoreAlphaAddr + "/query"
	params := testutil.GraphQLParams{
		Query: checkCountRequest,
	}
	b, err := json.Marshal(params)
	require.NoError(t, err)
	resp, err := http.Post(adminUrl, "application/json", bytes.NewBuffer(b))
	require.NoError(t, err)
	defer resp.Body.Close()
	var data interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&data))
	receivedMap := testutil.JsonGet(data, "data", "movieCount").([]interface{})
	movieCount := testutil.JsonGet(receivedMap[0], "count").(float64)
	require.Equal(t, expectedCount, int(movieCount))
}

func runBackup(t *testing.T, numExpectedFiles, numExpectedDirs int, backupAlphaSocketAddrHttp string, backupDst string) []string {
	return runBackupInternal(t, false, numExpectedFiles, numExpectedDirs, backupAlphaSocketAddrHttp, backupDst)
}

func runBackupInternal(t *testing.T, forceFull bool, numExpectedFiles,
	numExpectedDirs int, backupAlphaSocketAddrHttp string, backupDst string) []string {
	backupRequest := `mutation backup($dst: String!, $ff: Boolean!) {
		backup(input: {destination: $dst, forceFull: $ff}) {
			response {
				code
			}
			taskId
		}
	}`
	adminUrl := "http://" + backupAlphaSocketAddrHttp + "/admin"
	params := testutil.GraphQLParams{
		Query: backupRequest,
		Variables: map[string]interface{}{
			"dst": "/mnt" + backupDst,
			"ff":  forceFull,
		},
	}
	b, err := json.Marshal(params)
	require.NoError(t, err)
	resp, err := http.Post(adminUrl, "application/json", bytes.NewBuffer(b))
	require.NoError(t, err)
	defer resp.Body.Close()
	var data interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&data))
	require.Equal(t, "Success", testutil.JsonGet(data, "data", "backup", "response", "code").(string))
	taskId := testutil.JsonGet(data, "data", "backup", "taskId").(string)
	testutil.WaitForTask(t, taskId, false, backupAlphaSocketAddrHttp)
	// Verify that the right amount of files and directories were created.
	// We are not using local folder to back up.
	common.CopyToLocalFsFromNFS(t, backupDst)
	// List all the folders in the NFS mounted directory.
	files := x.WalkPathFunc(copyBackupDir, func(path string, isdir bool) bool {
		return !isdir && strings.HasSuffix(path, ".backup") && strings.HasPrefix(path, "data/backups_copy/dgraph.")
	})
	require.Equal(t, numExpectedFiles, len(files))
	dirs := x.WalkPathFunc(copyBackupDir, func(path string, isdir bool) bool {
		return isdir && strings.HasPrefix(path, "data/backups_copy/dgraph.")
	})
	require.Equal(t, numExpectedDirs, len(dirs))
	b, err = ioutil.ReadFile(filepath.Join(copyBackupDir, "manifest.json"))
	require.NoError(t, err)
	var manifest worker.MasterManifest
	err = json.Unmarshal(b, &manifest)
	require.NoError(t, err)
	require.Equal(t, numExpectedDirs, len(manifest.Manifests))
	return dirs
}

func restore(t *testing.T, lastDir string, restoreAlphaAddr string, backupDst string, backupType string, backupNum int) string {
	var restoreRequest string
	if backupType != "incremental" {
		restoreRequest = `mutation restore($loc: String!) {
			restore(input: {location: $loc}) {
					code
					message
				}
		}`

	} else {
		restoreRequest = `mutation restore($loc: String!, $bn: Int) {
			restore(input: {location: $loc, backupNum: $bn}) {
					code
					message
				}
		}`

	}
	// For restore we have to always use newly added restore cluster
	adminUrl := "http://" + restoreAlphaAddr + "/admin"
	params := testutil.GraphQLParams{
		Query: restoreRequest,
		Variables: map[string]interface{}{
			"loc": "/mnt" + backupDst,
			"bn":  backupNum,
		},
	}
	b, err := json.Marshal(params)
	require.NoError(t, err)
	resp, err := http.Post(adminUrl, "application/json", bytes.NewBuffer(b))
	require.NoError(t, err)
	defer resp.Body.Close()
	var data interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&data))
	restoreStatusCode := testutil.JsonGet(data, "data", "restore", "code").(string)
	return restoreStatusCode
}

func dirSetup(t *testing.T) {
	// Clean up data from previous runs.
	dirCleanup(t)
	nfsContainerName := testutil.DockerPrefix + "_nfs_1"
	cmdStr := "docker exec -d " + nfsContainerName + " mkdir -p" + " /data" + backupDstHA + " /data" + backupDstNonHA
	cmd := exec.Command("/bin/sh", "-c", cmdStr)
	_, err := cmd.Output()
	if err != nil {
		// if there was any error, print it here
		fmt.Println("could not run command: ", err)
	}
	for _, dir := range testDirs {
		if err := os.MkdirAll(dir, os.ModePerm); err != nil {
			t.Fatalf("Error while creating directory: %s", err.Error())
		}
	}
}

func dirCleanup(t *testing.T) {
	if err := os.RemoveAll("./data"); err != nil {
		t.Fatalf("Error removing direcotory: %s", err.Error())
	}
	nfsContainerName := testutil.DockerPrefix + "_nfs_1"
	cmdStr := "docker exec -d " + nfsContainerName + " rm -rf" + " /data" + backupDstHA + " /data" + backupDstNonHA
	cmd := exec.Command("/bin/sh", "-c", cmdStr)
	_, err := cmd.Output()
	if err != nil {
		// if there was any error, print it here
		fmt.Println("could not run command: ", err)
	}
}
