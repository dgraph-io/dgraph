/*
 * Copyright 2019 Dgraph Labs, Inc. and Contributors *
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
	"context"
	"fmt"
	"io/ioutil"
	"math"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc/credentials"

	"github.com/dgraph-io/badger/v3/options"
	"github.com/dgraph-io/dgo/v200"
	"github.com/dgraph-io/dgo/v200/protos/api"
	minio "github.com/minio/minio-go/v6"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	"github.com/dgraph-io/dgraph/testutil"
	"github.com/dgraph-io/dgraph/worker"
	"github.com/dgraph-io/dgraph/x"
)

var (
	backupDir  = "./data/backups"
	restoreDir = "./data/restore"
	testDirs   = []string{backupDir, restoreDir}

	mc                *minio.Client
	bucketName        = "dgraph-backup"
	backupDestination = "minio://minio:9001/dgraph-backup?secure=false"
	uidCounter        = 0
	batchSize         = 100
	totalTriples      = 20000
)

// Test to add a large database and verify backup and restore work as expected.
func TestBackupMinioLarge(t *testing.T) {
	// backupDestination = "minio://" + testutil.DockerPrefix + "_minio_1:9001/dgraph-backup?secure=false"
	conn, err := grpc.Dial(testutil.SockAddr, grpc.WithTransportCredentials(credentials.NewTLS(testutil.GetAlphaClientConfig(t))))
	require.NoError(t, err)
	dg := dgo.NewDgraphClient(api.NewDgraphClient(conn))
	ctx := context.Background()
	require.NoError(t, dg.Alter(ctx, &api.Operation{DropAll: true}))

	mc, err = testutil.NewMinioClient()
	require.NoError(t, err)
	require.NoError(t, mc.MakeBucket(bucketName, ""))

	// Setup the schema and make sure each group is assigned one predicate.
	setupTablets(t, dg)
	// Setup test directories.
	dirSetup(t)

	// Add half the total triples to create the first full backup.
	addTriples(t, dg, totalTriples/2)

	// Send backup request.
	runBackup(t)
	restored := runRestore(t, backupDir, "", math.MaxUint64)
	require.Equal(t, totalTriples/2, len(restored))

	// Add the other half to create an incremental backup.
	addTriples(t, dg, totalTriples/2)

	// Perform incremental backup.
	runBackup(t)
	restored = runRestore(t, backupDir, "", math.MaxUint64)
	require.Equal(t, totalTriples, len(restored))

	// Clean up test directories.
	dirCleanup(t)
}

func setupTablets(t *testing.T, dg *dgo.Dgraph) {
	require.NoError(t, dg.Alter(context.Background(), &api.Operation{
		Schema: `name1: string .
				 name2: string .
				 name3: string .`}))
	client := testutil.GetHttpsClient(t)
	_, err := client.Get("https://" + testutil.SockAddrZeroHttp + "/moveTablet?tablet=name1&group=1")
	require.NoError(t, err)
	time.Sleep(time.Second)
	_, err = client.Get("https://" + testutil.SockAddrZeroHttp + "/moveTablet?tablet=name2&group=2")
	require.NoError(t, err)
	time.Sleep(time.Second)
	_, err = client.Get("https://" + testutil.SockAddrZeroHttp + "/moveTablet?tablet=name3&group=3")
	require.NoError(t, err)

	// After the move, we need to pause a bit to give zero a chance to quorum.
	t.Log("Pausing to let zero move tablets...")
	moveOk := false
	for retry := 5; retry > 0; retry-- {
		time.Sleep(3 * time.Second)
		state, err := testutil.GetStateHttps(testutil.GetAlphaClientConfig(t))
		require.NoError(t, err)
		_, ok1 := state.Groups["1"].Tablets["name1"]
		_, ok2 := state.Groups["2"].Tablets["name2"]
		_, ok3 := state.Groups["3"].Tablets["name3"]
		if ok1 && ok2 && ok3 {
			moveOk = true
			break
		}
	}
	require.True(t, moveOk)
}

func addTriples(t *testing.T, dg *dgo.Dgraph, numTriples int) {
	// For simplicity, assume numTriples is divisible by batchSize.
	for i := 0; i < numTriples/batchSize; i++ {
		triples := strings.Builder{}
		for j := 0; j < 100; j++ {
			// Send triplets to different predicates so that they are stored uniformly
			// across the different groups.
			predNum := (uidCounter % 3) + 1
			triples.WriteString(fmt.Sprintf("<_:x%d> <name%d> \"%d\" .\n", uidCounter, predNum,
				uidCounter))
			uidCounter++
		}
		_, err := dg.NewTxn().Mutate(context.Background(), &api.Mutation{
			CommitNow: true,
			SetNquads: []byte(triples.String())})
		require.NoError(t, err)
	}
}

func runBackup(t *testing.T) {
	// Using the old /admin/backup endpoint to ensure it works. Change back to using
	// the GraphQL endpoint at /admin once this endpoint is deprecated.
	client := testutil.GetHttpsClient(t)
	resp, err := client.PostForm("https://"+testutil.SockAddrHttp+"/admin/backup", url.Values{
		"destination": []string{backupDestination},
	})
	require.NoError(t, err)
	defer resp.Body.Close()
	buf, err := ioutil.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Contains(t, string(buf), "Backup completed.")

	// Verify that the right amount of files and directories were created.
	copyToLocalFs(t)
}

func runRestore(t *testing.T, backupLocation, lastDir string, commitTs uint64) map[string]string {
	// Recreate the restore directory to make sure there's no previous data when
	// calling restore.
	require.NoError(t, os.RemoveAll(restoreDir))
	require.NoError(t, os.MkdirAll(restoreDir, os.ModePerm))

	t.Logf("--- Restoring from: %q", backupLocation)
	result := worker.RunRestore("./data/restore", backupLocation, lastDir, x.SensitiveByteSlice(nil), options.Snappy, 0)
	require.NoError(t, result.Err)

	restored1, err := testutil.GetPredicateValues("./data/restore/p1", "name1", commitTs)
	require.NoError(t, err)
	restored2, err := testutil.GetPredicateValues("./data/restore/p2", "name2", commitTs)
	require.NoError(t, err)
	restored3, err := testutil.GetPredicateValues("./data/restore/p3", "name3", commitTs)
	require.NoError(t, err)

	restored := make(map[string]string)
	mergeMap(&restored1, &restored)
	mergeMap(&restored2, &restored)
	mergeMap(&restored3, &restored)
	return restored
}

func mergeMap(in, out *map[string]string) {
	for k, v := range *in {
		(*out)[k] = v
	}
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
		dstDir := backupDir + "/" + object.Key
		require.NoError(t, os.MkdirAll(dstDir, os.ModePerm))

		// Get all the files in that folder and copy them to the local filesystem.
		lsCh2 := make(chan struct{})
		objectCh2 := mc.ListObjectsV2(bucketName, "", true, lsCh2)
		for object := range objectCh2 {
			require.NoError(t, object.Err)
			dstFile := backupDir + "/" + object.Key
			err := mc.FGetObject(bucketName, object.Key, dstFile, minio.GetObjectOptions{})
			require.NoError(t, err)
		}
		close(lsCh2)
	}
}
