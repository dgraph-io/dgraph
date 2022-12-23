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
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"

	"google.golang.org/grpc/credentials"

	"github.com/dgraph-io/dgo/v210"
	"github.com/dgraph-io/dgo/v210/protos/api"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	"github.com/dgraph-io/dgraph/testutil"
	"github.com/dgraph-io/dgraph/x"
)

var (
	backupDir                = "s3://s3.ap-south-1.amazonaws.com/test-the-dgraph-backup"
	restoreDir               = "./data/restore"
	manifestDir              = "./data"
	testDirs                 = []string{restoreDir}
	bucketName               = "test-the-dgraph-backup"
	backupDst                string
	files_found              int
	fileList                 = []string{}
	directoryList            = []string{}
	folderNameForCurrentTest string
)

func TestBackupHAClust(t *testing.T) {

	fmt.Println("*********************** Testing for HA cluster ************************")

	BackupAlphaSocketAddr := testutil.SockAddr
	BackupAlphaSocketAddrHttp := testutil.SockAddrHttp

	BackupZeroSockerAddr := testutil.SockAddrZeroHttp
	RestoreAlphaSocketAddr := testutil.R_SockAddrHttp

	tests3(t, BackupAlphaSocketAddr, RestoreAlphaSocketAddr, BackupZeroSockerAddr, BackupAlphaSocketAddrHttp)

}

func TestBackupNonHAClust(t *testing.T) {

	fmt.Println("*********************** Testing for Non-HA cluster ************************")

	BackupAlphaSocketAddr := testutil.SockAddrAlpha7
	BackupAlphaSocketAddrHttp := testutil.SockAddrAlpha7Http

	BackupZeroSockerAddr := testutil.SockAddrZero7Http
	RestoreAlphaSocketAddr := testutil.R_SockAddrAlpha8Http
	tests3(t, BackupAlphaSocketAddr, RestoreAlphaSocketAddr, BackupZeroSockerAddr, BackupAlphaSocketAddrHttp)

}
func tests3(t *testing.T, backupAlphaSocketAddr string, restoreAlphaAddr string, backupZeroAddr string, backupAlphaSocketAddrHttp string) {

	files_found = 0

	conn, err := grpc.Dial(backupAlphaSocketAddr, grpc.WithTransportCredentials(credentials.NewTLS(testutil.GetAlphaClientConfig(t))))
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
	_, err = client.Get("https://" + backupZeroAddr + "/moveTablet?tablet=movie&group=1")
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

	folderNameForCurrentTest = strconv.FormatInt(time.Now().UnixNano(), 10)

	createBackupFolder(t, folderNameForCurrentTest)

	S3Config := getEnvs()

	backupDst = "s3://s3." + S3Config[3] + ".amazonaws.com/" + S3Config[2] + "/" + folderNameForCurrentTest

	// Send backup request.
	// TODO: minio backup request fails when the environment is not ready,
	//       mostly because of a race condition
	//       adding sleep
	time.Sleep(time.Second * 10)

	_ = runBackup(t, 1, 1, backupAlphaSocketAddrHttp)

	restored := runRestore(t, "", restoreAlphaAddr, false, 1)
	require.Equal(t, "Success", restored)
	testutil.WaitForRestore(t, dg, restoreAlphaAddr)

	// We check expected Objects vs Received Objects from restoreed db
	checkObjectCount(t, 5, 5, restoreAlphaAddr)

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
	_ = runBackup(t, 2, 2, backupAlphaSocketAddrHttp)

	// Add more data for a second incremental backup.
	_, err = dg.NewTxn().Mutate(ctx, &api.Mutation{
		CommitNow: true,
		SetNquads: []byte(`
		 <_:x8> <movie> "The Shawshank Redemption" .
		 <_:x9> <movie> "12 Angary Man" .
	 `),
	})
	require.NoError(t, err)

	// Perform second incremental backup.
	_ = runBackup(t, 3, 3, backupAlphaSocketAddrHttp)

	//Now we are performing 2 restores with backup num 2 & 3

	//1st restore with backupNum=2
	restored = runRestore(t, "", restoreAlphaAddr, true, 2)
	require.Equal(t, "Success", restored)
	testutil.WaitForRestore(t, dg, restoreAlphaAddr)

	// We check expected Objects vs Received Objects from restoreed db
	checkObjectCount(t, 7, 7, restoreAlphaAddr)

	//1st restore with backupNum=3
	restored = runRestore(t, "", restoreAlphaAddr, true, 3)
	require.Equal(t, "Success", restored)
	testutil.WaitForRestore(t, dg, restoreAlphaAddr)

	// We check expected Objects vs Received Objects from restoreed db
	checkObjectCount(t, 9, 9, restoreAlphaAddr)

	// Add more data for a second full backup.
	_, err = dg.NewTxn().Mutate(ctx, &api.Mutation{
		CommitNow: true,
		SetNquads: []byte(`
		 <_:x10> <movie> "Harry Potter Part 1" .
		 <_:x11> <movie> "Harry Potter Part 2" .
	 `),
	})
	require.NoError(t, err)

	// Perform second full backup.
	_ = runBackupInternal(t, true, 4, 4, backupAlphaSocketAddrHttp)
	restored = runRestore(t, "", restoreAlphaAddr, false, 4)
	require.Equal(t, "Success", restored)
	testutil.WaitForRestore(t, dg, restoreAlphaAddr)

	// We check expected Objects vs Received Objects from restoreed db
	checkObjectCount(t, 11, 11, restoreAlphaAddr)

	// Do a DROP_DATA
	require.NoError(t, dg.Alter(ctx, &api.Operation{DropOp: api.Operation_DATA}))

	// add some data
	_, err = dg.NewTxn().Mutate(ctx, &api.Mutation{
		CommitNow: true,
		SetNquads: []byte(`
		 <_:x12> <movie> "Titanic" .
		 <_:x13> <movie> "The Dark Knight" .
	 `),
	})
	require.NoError(t, err)

	// perform an incremental backup and then restore
	_ = runBackup(t, 5, 5, backupAlphaSocketAddrHttp)
	restored = runRestore(t, "", restoreAlphaAddr, false, 5)
	require.Equal(t, "Success", restored)
	testutil.WaitForRestore(t, dg, restoreAlphaAddr)

	// We check expected Objects vs Received Objects from restoreed db
	checkObjectCount(t, 2, 2, restoreAlphaAddr)

	// Clean up test directories.
	dirCleanup(t)
}

func checkObjectCount(t *testing.T, expectedCount, receivedCount int, restoreAlphaAddr string) {

	checkCountRequest := `query {
		  movieCount(func: has(movie)) {
			count(uid)
		  }
		}`

	//Check object count from newly created restore alpha

	adminUrl := "https://" + restoreAlphaAddr + "/query"
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

}

func runBackup(t *testing.T, numExpectedFiles, numExpectedDirs int, backupAlphaSocketAddrHttp string) []string {
	return runBackupInternal(t, false, numExpectedFiles, numExpectedDirs, backupAlphaSocketAddrHttp)
}

func runBackupInternal(t *testing.T, forceFull bool, numExpectedFiles,
	numExpectedDirs int, backupAlphaSocketAddrHttp string) []string {

	//Flushing the fileList array.
	//Flushing the directoryList array.
	fileList = []string{}
	directoryList = []string{}

	S3Config := getEnvs()
	fmt.Print(S3Config)

	backupRequest := `mutation backup($dst: String!, $ak: String!, $sk: String! $ff: Boolean!) {
		 backup(input: {destination: $dst, accessKey: $ak, secretKey: $sk, forceFull: $ff}) {
			 response {
				 code
			 }
			 taskId
		 }
	 }`

	adminUrl := "https://" + backupAlphaSocketAddrHttp + "/admin"
	params := testutil.GraphQLParams{
		Query: backupRequest,
		Variables: map[string]interface{}{
			"dst": backupDst,
			"ak":  S3Config[0],
			"sk":  S3Config[1],
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
	testutil.WaitForTask(t, taskId, true, backupAlphaSocketAddrHttp)

	sess, _ := session.NewSession(&aws.Config{
		Region:                        aws.String(S3Config[3]),
		CredentialsChainVerboseErrors: aws.Bool(true)},
	)

	if err != nil {
		fmt.Println("------------------------------>failed to create a new aws session: ", sess)
	}

	s3client := s3.New(sess)

	// List all the folders in the bucket.
	s3resp, s3err := s3client.ListObjectsV2(&s3.ListObjectsV2Input{Bucket: aws.String(S3Config[2]), Prefix: aws.String(folderNameForCurrentTest + "/")})
	if s3err != nil {
		fmt.Println("------------------------>Unable to list items in bucket :-", s3err)
	}

	for _, item := range s3resp.Contents {
		if *item.Key != "manifest.json" {

			fileList = append(fileList, "s3://"+S3Config[3]+".amazonaws.com/"+S3Config[2]+"/"+*item.Key)

			directoryList = append(directoryList, "s3://"+S3Config[3]+".amazonaws.com/"+S3Config[2]+"/"+strings.Split(*item.Key, "/")[1])
		}
	}

	files := RemoveDuplicateDirectories(fileList)
	dirs := RemoveDuplicateDirectories(directoryList)

	require.Equal(t, numExpectedFiles, len(files)-2)

	require.Equal(t, numExpectedDirs, len(dirs)-2)

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
	for item, _ := range m {
		result = append(result, item)
	}
	return result
}

func runRestore(t *testing.T, lastDir string, restoreAlphaAddr string, isIncremental bool, backupNum int) string {

	var restoreRequest string
	S3Config := getEnvs()
	if isIncremental == false {
		restoreRequest = `mutation restore($loc: String!, $ak: String!, $sk: String!) {
			 restore(input: {location: $loc, accessKey: $ak, secretKey: $sk}) {
					 code
					 message
				 }
		 }`

	} else {
		restoreRequest = `mutation restore($loc: String!, $ak: String!, $sk: String!, $bn: Int) {
			 restore(input: {location: $loc, accessKey: $ak, secretKey: $sk, backupNum: $bn}) {
					 code
					 message
				 }
		 }`

	}

	// For restore we always have to use newly added restore cluster
	adminUrl := "https://" + restoreAlphaAddr + "/admin"
	params := testutil.GraphQLParams{
		Query: restoreRequest,
		Variables: map[string]interface{}{
			"loc": backupDst,
			"ak":  S3Config[0],
			"sk":  S3Config[1],
			"bn":  backupNum,
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

	//require.Equal(t, "Success", testutil.JsonGet(data, "data", "restore", "code").(string))
	receivedcode := testutil.JsonGet(data, "data", "restore", "code").(string)

	//Waiting for 2 seconds, allowing restore to finish, since restore dosen't give us taskID and data is small
	//For larger data/payload, please increase the waiting time.

	time.Sleep(5 * time.Second)

	return receivedcode
}

func dirSetup(t *testing.T) {
	// Clean up data from previous runs.

	for _, dir := range testDirs {
		if err := os.MkdirAll(dir, os.ModePerm); err != nil {
			t.Fatalf("Error while creaing directory: %s", err.Error())
		}
	}
}

func dirCleanup(t *testing.T) {

	S3Config := getEnvs()

	if err := os.RemoveAll("./data"); err != nil {
		t.Fatalf("Error removing direcotory: %s", err.Error())
	}

	//Cleanup the s3 too

	sess, _ := session.NewSession(&aws.Config{
		Region:                        aws.String(S3Config[3]),
		CredentialsChainVerboseErrors: aws.Bool(true)},
	)

	// Create S3 service client
	svc := s3.New(sess)

	resp, err := svc.ListObjects(&s3.ListObjectsInput{
		Bucket: aws.String(S3Config[2]),
		Prefix: aws.String(folderNameForCurrentTest + "/"),
	})

	for _, key := range resp.Contents {
		_, err = svc.DeleteObject(&s3.DeleteObjectInput{Bucket: aws.String(S3Config[2]), Key: aws.String(*key.Key)})
		if err != nil {
			exitErrorf("Unable to delete object %q from bucket %q, %v", folderNameForCurrentTest, S3Config[2], err)
		}
	}

	err = svc.WaitUntilObjectNotExists(&s3.HeadObjectInput{
		Bucket: aws.String(S3Config[2]),
		Key:    aws.String(folderNameForCurrentTest),
	})

	fmt.Println("*****Any Problem while deleting the s3 folder "+folderNameForCurrentTest+"? ", err)
}

func exitErrorf(msg string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, msg+"\n", args...)
	os.Exit(1)
}

func createBackupFolder(t *testing.T, dir string) {

	S3Config := getEnvs()

	sess, _ := session.NewSession(&aws.Config{
		Region:                        aws.String(S3Config[3]),
		CredentialsChainVerboseErrors: aws.Bool(true)},
	)

	upFile, err := os.Open("backup_test.go")
	if err != nil {
		fmt.Println("Cant open file")
	}
	defer upFile.Close()

	upFileInfo, _ := upFile.Stat()
	var fileSize int64 = upFileInfo.Size()
	fileBuffer := make([]byte, fileSize)
	upFile.Read(fileBuffer)

	_, err = s3.New(sess).PutObject(&s3.PutObjectInput{
		Bucket:               aws.String(S3Config[2]),
		Key:                  aws.String(dir + "/test.txt"),
		Body:                 bytes.NewReader(fileBuffer),
		ContentLength:        aws.Int64(fileSize),
		ContentType:          aws.String(http.DetectContentType(fileBuffer)),
		ContentDisposition:   aws.String("attachment"),
		ServerSideEncryption: aws.String("AES256"),
	})

	if err != nil {
		fmt.Println("Error while creating folder ", err)
	}
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
