/*
 * Copyright 2022 Dgraph Labs, Inc. and Contributors *
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

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"

	"google.golang.org/grpc/credentials"

	"github.com/dgraph-io/badger/v3/options"
	"github.com/dgraph-io/dgo/v210"
	"github.com/dgraph-io/dgo/v210/protos/api"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	"github.com/dgraph-io/dgraph/testutil"
	"github.com/dgraph-io/dgraph/worker"
	"github.com/dgraph-io/dgraph/x"
)

var (
	backupDir     = "s3://s3.ap-south-1.amazonaws.com/test-the-dgraph-backup"
	restoreDir    = "./data/restore"
	manifestDir   = "./data"
	testDirs      = []string{backupDir, restoreDir}
	bucketName    = "test-the-dgraph-backup"
	backupDst     string
	files_found   int
	fileList      = []string{}
	directoryList = []string{}
)

func TestBackupS3(t *testing.T) {

	MapIGet := getEnvs()

	backupDst = "s3://s3." + MapIGet[3] + ".amazonaws.com/" + MapIGet[2]

	files_found = 0

	conn, err := grpc.Dial(testutil.SockAddr, grpc.WithTransportCredentials(credentials.NewTLS(testutil.GetAlphaClientConfig(t))))
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
	t.Logf("--- Original uid mapping: %+v\n", original.Uids)

	// Move tablet to group 1 to avoid messes later.
	client := testutil.GetHttpsClient(t)
	_, err = client.Get("https://" + testutil.SockAddrZeroHttp + "/moveTablet?tablet=movie&group=1")
	require.NoError(t, err)

	// After the move, we need to pause a bit to give zero a chance to quorum.
	t.Log("Pausing to let zero move tablet...")
	moveOk := false
	for retry := 5; retry > 0; retry-- {
		state, err := testutil.GetStateHttps(testutil.GetAlphaClientConfig(t))
		require.NoError(t, err)
		if _, ok := state.Groups["1"].Tablets[x.NamespaceAttr(x.GalaxyNamespace, "movie")]; ok {
			moveOk = true
			break
		}
		time.Sleep(1 * time.Second)
	}
	require.True(t, moveOk)

	// Setup test directories.
	dirSetup(t)

	// Send backup request.
	// TODO: minio backup request fails when the environment is not ready,
	//       mostly because of a race condition
	//       adding sleep
	time.Sleep(time.Second * 10)

	_ = runBackup(t, 3, 1)

	restored := runRestore(t, "", math.MaxUint64)
	require.Equal(t, "Success", restored)
	testutil.WaitForRestore(t, dg, testutil.R_SockAddrHttp)

	// We check expected Objects vs Received Objects from restoreed db
	checkObjectCount(t, 5, 5)

	// Add more data for the incremental backup.
	incr1, err := dg.NewTxn().Mutate(ctx, &api.Mutation{
		CommitNow: true,
		SetNquads: []byte(`
			<_:x6> <movie> "Inception" .
			<_:x7> <movie> "The Lord of the Rings" .
		`),
	})
	t.Logf("%+v", incr1)
	require.NoError(t, err)

	// Perform first incremental backup.
	_ = runBackup(t, 6, 2)
	restored = runRestore(t, "", incr1.Txn.CommitTs)
	require.Equal(t, "Success", restored)
	testutil.WaitForRestore(t, dg, testutil.R_SockAddrHttp)

	// We check expected Objects vs Received Objects from restoreed db
	checkObjectCount(t, 7, 7)

	// Add more data for a second incremental backup.
	incr2, err := dg.NewTxn().Mutate(ctx, &api.Mutation{
		CommitNow: true,
		SetNquads: []byte(`
		<_:x8> <movie> "The Shawshank Redemption" .
		<_:x9> <movie> "12 Angary Man" .
	`),
	})
	require.NoError(t, err)

	// Perform second incremental backup.
	_ = runBackup(t, 9, 3)
	restored = runRestore(t, "", incr2.Txn.CommitTs)
	require.Equal(t, "Success", restored)
	testutil.WaitForRestore(t, dg, testutil.R_SockAddrHttp)

	// We check expected Objects vs Received Objects from restoreed db
	checkObjectCount(t, 9, 9)

	// Add more data for a second full backup.
	incr3, err := dg.NewTxn().Mutate(ctx, &api.Mutation{
		CommitNow: true,
		SetNquads: []byte(`
		<_:x10> <movie> "Harry Potter Part 1" .
		<_:x11> <movie> "Harry Potter Part 2" .
	`),
	})
	require.NoError(t, err)

	// Perform second full backup.
	_ = runBackupInternal(t, true, 12, 4)
	restored = runRestore(t, "", incr3.Txn.CommitTs)
	require.Equal(t, "Success", restored)
	testutil.WaitForRestore(t, dg, testutil.R_SockAddrHttp)

	// We check expected Objects vs Received Objects from restoreed db
	checkObjectCount(t, 11, 11)

	// Do a DROP_DATA
	require.NoError(t, dg.Alter(ctx, &api.Operation{DropOp: api.Operation_DATA}))

	// add some data
	incr4, err := dg.NewTxn().Mutate(ctx, &api.Mutation{
		CommitNow: true,
		SetNquads: []byte(`
		<_:x12> <movie> "Titanic" .
		<_:x13> <movie> "The Dark Knight" .
	`),
	})
	require.NoError(t, err)

	// perform an incremental backup and then restore
	_ = runBackup(t, 15, 5)
	restored = runRestore(t, "", incr4.Txn.CommitTs)
	require.Equal(t, "Success", restored)
	testutil.WaitForRestore(t, dg, testutil.R_SockAddrHttp)

	// We check expected Objects vs Received Objects from restoreed db
	checkObjectCount(t, 2, 2)

	// Clean up test directories.
	dirCleanup(t)
}

func checkObjectCount(t *testing.T, expectedCount, receivedCount int) {

	checkCountRequest := `query {
		movieCount(func: has(movie)) {
		  count(uid)
		}
	  }`

	//Check object count from newly created restore alpha

	adminUrl := "https://" + testutil.R_SockAddrHttp + "/query"
	params := testutil.GraphQLParams{
		Query: checkCountRequest,
	}
	b, err := json.Marshal(params)
	require.NoError(t, err)

	client := testutil.GetHttpsClient(t)
	resp, err := client.Post(adminUrl, "application/json", bytes.NewBuffer(b))
	require.NoError(t, err)
	defer resp.Body.Close()

	var data interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&data))

	receivedMap := testutil.JsonGet(data, "data", "movieCount").([]interface{})

	receivedNumber := testutil.JsonGet(receivedMap[0], "count").(float64)

	require.Equal(t, expectedCount, int(receivedNumber))
	//Setting files_found value to 0.
	//Flushing the fileList array.
	//Flushing the directoryList array.
	files_found = 0
	fileList = []string{}
	directoryList = []string{}
}

func runBackup(t *testing.T, numExpectedFiles, numExpectedDirs int) []string {
	return runBackupInternal(t, false, numExpectedFiles, numExpectedDirs)
}

func runBackupInternal(t *testing.T, forceFull bool, numExpectedFiles,
	numExpectedDirs int) []string {

	MapIGet := getEnvs()

	backupRequest := `mutation backup($dst: String!, $ak: String!, $sk: String! $ff: Boolean!) {
		backup(input: {destination: $dst, accessKey: $ak, secretKey: $sk, forceFull: $ff}) {
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
			"ak":  MapIGet[0],
			"sk":  MapIGet[1],
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
	testutil.WaitForTask(t, taskId, true, testutil.SockAddrHttp)

	sess, _ := session.NewSession(&aws.Config{
		Region:                        aws.String(MapIGet[3]),
		CredentialsChainVerboseErrors: aws.Bool(true)},
	)
	if err != nil {
		fmt.Println("------------------------------>failed to create a new aws session: ", sess)
	}

	s3client := s3.New(sess)

	// List all the folders in the bucket.
	s3resp, s3err := s3client.ListObjectsV2(&s3.ListObjectsV2Input{Bucket: aws.String(MapIGet[2])})
	if s3err != nil {
		fmt.Println("------------------------>Unable to list items in bucket :-", err)
	}

	for _, item := range s3resp.Contents {
		if *item.Key != "manifest.json" {
			files_found++

			fileList = append(fileList, "s3://"+MapIGet[3]+".amazonaws.com/test-the-dgraph-backup"+*item.Key)

			directoryList = append(directoryList, "s3://"+MapIGet[3]+".amazonaws.com/test-the-dgraph-backup"+strings.Split(*item.Key, "/")[0])
		}
	}

	files := fileList

	require.Equal(t, numExpectedFiles, len(files))

	dirs := RemoveDuplicateDirectories(directoryList)

	require.Equal(t, numExpectedDirs, len(dirs))

	downloader := s3manager.NewDownloader(sess)

	item := manifestDir + "/manifest.json"

	file, err := os.Create(item)
	if err != nil {
		fmt.Println(err)
	}

	defer file.Close()

	numBytes, err := downloader.Download(file,
		&s3.GetObjectInput{
			Bucket: aws.String(MapIGet[2]),
			Key:    aws.String("manifest.json"),
		})
	if err != nil {
		fmt.Println(err)
	}

	fmt.Println("!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!Downloaded", file.Name(), numBytes, "bytes")

	b, err = ioutil.ReadFile(filepath.Join(manifestDir, "manifest.json"))
	require.NoError(t, err)

	var manifest worker.MasterManifest
	err = json.Unmarshal(b, &manifest)
	require.NoError(t, err)
	require.Equal(t, numExpectedDirs, len(manifest.Manifests))

	return dirs
}

func RemoveDuplicateDirectories(s []string) []string {
	m := make(map[string]bool)
	for _, item := range s {
		if _, ok := m[item]; ok {
		} else {
			m[item] = true
		}
	}

	var result []string
	for item := range m {
		result = append(result, item)
	}
	return result
}

func runRestore(t *testing.T, lastDir string, commitTs uint64) string {

	MapIGet := getEnvs()

	restoreRequest := `mutation restore($loc: String!, $ak: String!, $sk: String!) {
		restore(input: {location: $loc, accessKey: $ak, secretKey: $sk}) {
				code
				message
			}
	}`

	// For restore we have to always use newly added restore cluster
	adminUrl := "https://" + testutil.R_SockAddrHttp + "/admin"
	params := testutil.GraphQLParams{
		Query: restoreRequest,
		Variables: map[string]interface{}{
			"loc": backupDst,
			"ak":  MapIGet[0],
			"sk":  MapIGet[1],
		},
	}
	b, err := json.Marshal(params)
	require.NoError(t, err)

	fmt.Println("Admin URL for Restore------------>", adminUrl)

	client := testutil.GetHttpsClient(t)
	resp, err := client.Post(adminUrl, "application/json", bytes.NewBuffer(b))
	require.NoError(t, err)
	defer resp.Body.Close()

	fmt.Println("Response of the post request------------>", resp)

	var data interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&data))

	//require.Equal(t, "Success", testutil.JsonGet(data, "data", "restore", "code").(string))
	receivedcode := testutil.JsonGet(data, "data", "restore", "code").(string)

	fmt.Println("Received code from post request------------>", receivedcode)

	//Waiting for 2 seconds, allowing restore to finish, since restore dosen't give us taskID and data is small
	//For larger data/payload, please increase the waiting time.

	time.Sleep(2 * time.Second)

	return receivedcode
}

// runFailingRestore is like runRestore but expects an error during restore.
func runFailingRestore(t *testing.T, backupLocation, lastDir string, commitTs uint64) {
	// Recreate the restore directory to make sure there's no previous data when
	// calling restore.
	require.NoError(t, os.RemoveAll(restoreDir))

	result := worker.RunRestore("./data/restore", backupLocation, lastDir, x.SensitiveByteSlice(nil), options.Snappy, 0)
	require.Error(t, result.Err)
	require.Contains(t, result.Err.Error(), "expected a BackupNum value of 1")
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
	MapIGet := getEnvs()

	if err := os.RemoveAll("./data"); err != nil {
		t.Fatalf("Error removing direcotory: %s", err.Error())
	}

	//Cleanup the s3 too

	sess, _ := session.NewSession(&aws.Config{
		Region:                        aws.String(MapIGet[3]),
		CredentialsChainVerboseErrors: aws.Bool(true)},
	)
	// Create S3 service client
	svc := s3.New(sess)

	// Setup BatchDeleteIterator to iterate through a list of objects.
	iter := s3manager.NewDeleteListIterator(svc, &s3.ListObjectsInput{
		Bucket: aws.String(MapIGet[2]),
	})

	// Traverse iterator deleting each object
	if err := s3manager.NewBatchDeleteWithClient(svc).Delete(aws.BackgroundContext(), iter); err != nil {
		exitErrorf("Can't delete object(s) from bucket because %v", err)
	}
}

func exitErrorf(msg string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, msg+"\n", args...)
	os.Exit(1)
}

func getEnvs() [4]string {

	S3_ACCESS_KEY := os.Getenv("S3_ACCESS_KEY")
	S3_SECRET_KEY := os.Getenv("S3_SECRET_KEY")
	S3_BUCKET_NAME := os.Getenv("S3_BUCKET_NAME")
	S3_REGION := os.Getenv("S3_REGION")

	if S3_ACCESS_KEY == "" || S3_SECRET_KEY == "" || S3_BUCKET_NAME == "" || S3_REGION == "" {

		exitErrorf("********************************** FAILED TO FETCH S3 CONFIG ************************************")

	}

	ConfigMap := [4]string{0: S3_ACCESS_KEY, 1: S3_SECRET_KEY, 2: S3_BUCKET_NAME, 3: S3_REGION}

	return ConfigMap
}
