/*
 * Copyright 2018 Dgraph Labs, Inc. and Contributors *
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
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dgraph-io/badger/v3/options"
	"github.com/dgraph-io/dgo/v210/protos/api"
	"github.com/dgraph-io/dgraph/ee"
	"github.com/dgraph-io/dgraph/testutil"
	"github.com/dgraph-io/dgraph/worker"
	"github.com/dgraph-io/dgraph/x"
	minio "github.com/minio/minio-go/v6"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

var (
	backupDir  = "./data/backups"
	restoreDir = "./data/restore"
	testDirs   = []string{backupDir, restoreDir}

	mc             *minio.Client
	bucketName     = "dgraph-backup"
	backupDst      string
	localBackupDst string
)

func TestBackupMinioE(t *testing.T) {
	backupDst = "minio://minio:9001/dgraph-backup?secure=false"
	addr := testutil.ContainerAddr("minio", 9001)
	localBackupDst = "minio://" + addr + "/dgraph-backup?secure=false"

	conf := viper.GetViper()
	conf.Set("tls", fmt.Sprintf("ca-cert=%s; server-name=%s; internal-port=%v;",
		// ca-cert
		"../../../tlstest/mtls_internal/tls/live/ca.crt",
		// server-name
		"alpha1",
		// internal-port
		true))

	dg, err := testutil.DgraphClientWithCerts(testutil.SockAddr, conf)
	require.NoError(t, err)
	mc, err = testutil.NewMinioClient()
	require.NoError(t, err)
	require.NoError(t, mc.MakeBucket(bucketName, ""))

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
	t.Logf("--- Original uid mapping: %+v\n", original.Uids)

	client := testutil.GetHttpsClient(t)
	tabletName := x.NamespaceAttr(x.GalaxyNamespace, "movie")
	// Move tablet to group 1 to avoid messes later.
	_, err = client.Get("https://" + testutil.SockAddrZeroHttp + "/moveTablet?tablet=movie&group=1")
	require.NoError(t, err)

	// After the move, we need to pause a bit to give zero a chance to quorum.
	t.Log("Pausing to let zero move tablet...")
	moveOk := false
	for retry := 5; retry > 0; retry-- {
		state, err := testutil.GetStateHttps(testutil.GetAlphaClientConfig(t))
		require.NoError(t, err)
		if _, ok := state.Groups["1"].Tablets[tabletName]; ok {
			moveOk = true
			break
		}
		time.Sleep(1 * time.Second)
	}
	require.True(t, moveOk)

	// Setup environmental variables for use during restore.
	os.Setenv("MINIO_ACCESS_KEY", "accesskey")
	os.Setenv("MINIO_SECRET_KEY", "secretkey")

	// Setup test directories.
	dirSetup(t)

	// Send backup request.
	_ = runBackup(t, 3, 1)
	restored := runRestore(t, "", math.MaxUint64)

	checks := []struct {
		blank, expected string
	}{
		{blank: "x1", expected: "BIRDS MAN OR (THE UNEXPECTED VIRTUE OF IGNORANCE)"},
		{blank: "x2", expected: "Spotlight"},
		{blank: "x3", expected: "Moonlight"},
		{blank: "x4", expected: "THE SHAPE OF WATERLOO"},
		{blank: "x5", expected: "BLACK PUNTER"},
	}
	for _, check := range checks {
		require.EqualValues(t, check.expected, restored[original.Uids[check.blank]])
	}

	// Add more data for the incremental backup.
	incr1, err := dg.NewTxn().Mutate(ctx, &api.Mutation{
		CommitNow: true,
		SetNquads: []byte(fmt.Sprintf(`
			<%s> <movie> "Birdman or (The Unexpected Virtue of Ignorance)" .
			<%s> <movie> "The Shape of Waterloo" .
		`, original.Uids["x1"], original.Uids["x4"])),
	})
	t.Logf("%+v", incr1)
	require.NoError(t, err)

	// Perform first incremental backup.
	_ = runBackup(t, 6, 2)
	restored = runRestore(t, "", incr1.Txn.CommitTs)

	checks = []struct {
		blank, expected string
	}{
		{blank: "x1", expected: "Birdman or (The Unexpected Virtue of Ignorance)"},
		{blank: "x4", expected: "The Shape of Waterloo"},
	}
	for _, check := range checks {
		require.EqualValues(t, check.expected, restored[original.Uids[check.blank]])
	}

	// Add more data for a second incremental backup.
	incr2, err := dg.NewTxn().Mutate(ctx, &api.Mutation{
		CommitNow: true,
		SetNquads: []byte(fmt.Sprintf(`
				<%s> <movie> "The Shape of Water" .
				<%s> <movie> "The Black Panther" .
			`, original.Uids["x4"], original.Uids["x5"])),
	})
	require.NoError(t, err)

	// Perform second incremental backup.
	_ = runBackup(t, 9, 3)
	restored = runRestore(t, "", incr2.Txn.CommitTs)

	checks = []struct {
		blank, expected string
	}{
		{blank: "x4", expected: "The Shape of Water"},
		{blank: "x5", expected: "The Black Panther"},
	}
	for _, check := range checks {
		require.EqualValues(t, check.expected, restored[original.Uids[check.blank]])
	}

	// Add more data for a second full backup.
	incr3, err := dg.NewTxn().Mutate(ctx, &api.Mutation{
		CommitNow: true,
		SetNquads: []byte(fmt.Sprintf(`
				<%s> <movie> "El laberinto del fauno" .
				<%s> <movie> "Black Panther 2" .
			`, original.Uids["x4"], original.Uids["x5"])),
	})
	require.NoError(t, err)

	// Perform second full backup.
	_ = runBackupInternal(t, true, 12, 4)
	restored = runRestore(t, "", incr3.Txn.CommitTs)

	// Check all the values were restored to their most recent value.
	checks = []struct {
		blank, expected string
	}{
		{blank: "x1", expected: "Birdman or (The Unexpected Virtue of Ignorance)"},
		{blank: "x2", expected: "Spotlight"},
		{blank: "x3", expected: "Moonlight"},
		{blank: "x4", expected: "El laberinto del fauno"},
		{blank: "x5", expected: "Black Panther 2"},
	}
	for _, check := range checks {
		require.EqualValues(t, check.expected, restored[original.Uids[check.blank]])
	}

	// Do a DROP_DATA
	require.NoError(t, dg.Alter(ctx, &api.Operation{DropOp: api.Operation_DATA}))

	// add some data
	incr4, err := dg.NewTxn().Mutate(ctx, &api.Mutation{
		CommitNow: true,
		SetNquads: []byte(`
				<_:x1> <movie> "El laberinto del fauno" .
				<_:x2> <movie> "Black Panther 2" .
			`),
	})
	require.NoError(t, err)

	// perform an incremental backup and then restore
	dirs := runBackup(t, 15, 5)
	restored = runRestore(t, "", incr4.Txn.CommitTs)

	// Check that the newly added data is the only data for the movie predicate
	require.Len(t, restored, 2)
	checks = []struct {
		blank, expected string
	}{
		{blank: "x1", expected: "El laberinto del fauno"},
		{blank: "x2", expected: "Black Panther 2"},
	}
	for _, check := range checks {
		require.EqualValues(t, check.expected, restored[incr4.Uids[check.blank]])
	}

	// Remove the full backup dirs and verify restore catches the error.
	require.NoError(t, os.RemoveAll(dirs[0]))
	require.NoError(t, os.RemoveAll(dirs[3]))
	runFailingRestore(t, backupDir, "", incr4.Txn.CommitTs)

	// Clean up test directories.
	dirCleanup(t)
}

func runBackup(t *testing.T, numExpectedFiles, numExpectedDirs int) []string {
	return runBackupInternal(t, false, numExpectedFiles, numExpectedDirs)
}

func runBackupInternal(t *testing.T, forceFull bool, numExpectedFiles,
	numExpectedDirs int) []string {

	backupRequest := `mutation backup($dst: String!, $ff: Boolean!) {
		backup(input: {destination: $dst, forceFull: $ff}) {
			response {
				code
			}
			taskId
		}
	}`

	adminUrl := "https://" + testutil.SockAddrHttp + "/admin"
	params := testutil.GraphQLParams{
		Query: backupRequest,
		Variables: map[string]interface{}{
			"dst": backupDst,
			"ff":  forceFull,
		},
	}
	b, err := json.Marshal(params)
	require.NoError(t, err)
	client := testutil.GetHttpsClient(t)
	resp, err := client.Post(adminUrl, "application/json", bytes.NewBuffer(b))
	require.NoError(t, err)
	defer resp.Body.Close()

	var data interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&data))
	require.Equal(t, "Success", testutil.JsonGet(data, "data", "backup", "response", "code").(string))
	taskId := testutil.JsonGet(data, "data", "backup", "taskId").(string)
	testutil.WaitForTask(t, taskId, true)

	// Verify that the right amount of files and directories were created.
	copyToLocalFs(t)

	files := x.WalkPathFunc(backupDir, func(path string, isdir bool) bool {
		return !isdir && strings.HasSuffix(path, ".backup")
	})
	require.Equal(t, numExpectedFiles, len(files))

	dirs := x.WalkPathFunc(backupDir, func(path string, isdir bool) bool {
		return isdir && strings.HasPrefix(path, "data/backups/dgraph.")
	})
	require.Equal(t, numExpectedDirs, len(dirs))

	b, err = ioutil.ReadFile(filepath.Join(backupDir, "manifest.json"))
	require.NoError(t, err)

	var manifest worker.MasterManifest
	err = json.Unmarshal(b, &manifest)
	require.NoError(t, err)
	require.Equal(t, numExpectedDirs, len(manifest.Manifests))
	return dirs
}

func runRestore(t *testing.T, lastDir string, commitTs uint64) map[string]string {
	// Recreate the restore directory to make sure there's no previous data when
	// calling restore.
	require.NoError(t, os.RemoveAll(restoreDir))

	t.Logf("--- Restoring from: %q", localBackupDst)
	testutil.KeyFile = "../../../ee/enc/test-fixtures/enc-key"
	result := worker.RunOfflineRestore("./data/restore", localBackupDst, lastDir,
		testutil.KeyFile, options.Snappy, 0)
	require.NoError(t, result.Err)

	for i, pdir := range []string{"p1", "p2", "p3"} {
		pdir = filepath.Join("./data/restore", pdir)
		groupId, err := x.ReadGroupIdFile(pdir)
		require.NoError(t, err)
		require.Equal(t, uint32(i+1), groupId)
	}
	pdir := "./data/restore/p1"
	restored, err := testutil.GetPredicateValues(pdir, x.GalaxyAttr("movie"), commitTs)
	require.NoError(t, err)

	restoredPreds, err := testutil.GetPredicateNames(pdir)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"dgraph.graphql.schema", "dgraph.graphql.xid", "dgraph.type",
		"movie", "dgraph.graphql.p_query", "dgraph.drop.op"},
		restoredPreds)

	restoredTypes, err := testutil.GetTypeNames(pdir)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"Node", "dgraph.graphql",
		"dgraph.graphql.persisted_query"}, restoredTypes)

	require.NoError(t, err)
	t.Logf("--- Restored values: %+v\n", restored)

	return restored
}

// runFailingRestore is like runRestore but expects an error during restore.
func runFailingRestore(t *testing.T, backupLocation, lastDir string, commitTs uint64) {
	// Recreate the restore directory to make sure there's no previous data when
	// calling restore.
	require.NoError(t, os.RemoveAll(restoreDir))
	keyFile := "../../../ee/enc/test-fixtures/enc-key"

	result := worker.RunOfflineRestore("./data/restore", backupLocation, lastDir, keyFile,
		options.Snappy, 0)
	require.Error(t, result.Err)
	require.Contains(t, result.Err.Error(), "expected a BackupNum value of 1")
}

func getEncConfig() *viper.Viper {
	config := viper.New()
	flags := &pflag.FlagSet{}
	ee.RegisterEncFlag(flags)
	config.BindPFlags(flags)
	return config
}

func dirSetup(t *testing.T) {
	// Clean up data from previous runs.
	dirCleanup(t)

	for _, dir := range testDirs {
		if err := os.MkdirAll(dir, os.ModePerm); err != nil {
			t.Fatalf("Error while creaing directory: %s", err.Error())
		}
	}
}

func dirCleanup(t *testing.T) {
	if err := os.RemoveAll("./data"); err != nil {
		t.Fatalf("Error removing direcotory: %s", err.Error())
	}
}

func copyToLocalFs(t *testing.T) {
	// List all the folders in the bucket.
	lsCh1 := make(chan struct{})
	defer close(lsCh1)
	objectCh1 := mc.ListObjectsV2(bucketName, "", false, lsCh1)
	for object := range objectCh1 {
		require.NoError(t, object.Err)
		if object.Key != "manifest.json" {
			dstDir := backupDir + "/" + object.Key
			require.NoError(t, os.MkdirAll(dstDir, os.ModePerm))
		}

		// Get all the files in that folder and copy them to the local filesystem.
		lsCh2 := make(chan struct{})
		objectCh2 := mc.ListObjectsV2(bucketName, "", true, lsCh2)
		for object := range objectCh2 {
			require.NoError(t, object.Err)
			dstFile := backupDir + "/" + object.Key
			mc.FGetObject(bucketName, object.Key, dstFile, minio.GetObjectOptions{})
		}
		close(lsCh2)
	}
}
