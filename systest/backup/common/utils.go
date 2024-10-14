//go:build !oss
// +build !oss

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

package common

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/badger/v4/options"
	"github.com/dgraph-io/dgraph/v24/graphql/e2e/common"
	"github.com/dgraph-io/dgraph/v24/testutil"
	"github.com/dgraph-io/dgraph/v24/worker"
)

var (
	copyBackupDir   = "./data/backups_copy"
	restoreDir      = "./data/restore"
	testDirs        = []string{restoreDir}
	alphaBackupDir  = "/data/backups"
	oldBackupDir    = "/data/to_restore"
	alphaContainers = []string{
		"alpha1",
		"alpha2",
		"alpha3",
	}
)

const (
	accessJwtHeader = "X-Dgraph-AccessToken"
)

// RunFailingRestore is like runRestore but expects an error during restore.
func RunFailingRestore(t *testing.T, backupLocation, lastDir string, commitTs uint64) {
	// Recreate the restore directory to make sure there's no previous data when
	// calling restore.
	require.NoError(t, os.RemoveAll(restoreDir))

	result := worker.RunOfflineRestore(restoreDir, backupLocation, lastDir,
		"", nil, options.Snappy, 0)
	require.Error(t, result.Err)
	require.Contains(t, result.Err.Error(), "expected a BackupNum value of 1")
}

func DirSetup(t *testing.T) {
	// Clean up data from previous runs.
	DirCleanup(t)

	for _, dir := range testDirs {
		require.NoError(t, os.MkdirAll(dir, os.ModePerm))
	}

	for _, alpha := range alphaContainers {
		cmd := []string{"mkdir", "-p", alphaBackupDir}
		require.NoError(t, testutil.DockerExec(alpha, cmd...))
	}
}

func DirCleanup(t *testing.T) {
	require.NoError(t, os.RemoveAll(restoreDir))
	require.NoError(t, os.RemoveAll(copyBackupDir))

	cmd := []string{"bash", "-c", "rm -rf /data/backups/*"}
	require.NoError(t, testutil.DockerExec(alphaContainers[0], cmd...))
}

func CopyOldBackupDir(t *testing.T) {
	for i := 1; i < 4; i++ {
		destPath := fmt.Sprintf("%s_alpha%d_1:/data", testutil.DockerPrefix, i)
		srchPath := "." + oldBackupDir
		require.NoError(t, testutil.DockerCp(srchPath, destPath))
	}
}

func CopyToLocalFs(t *testing.T) {
	// The original backup files are not accessible because docker creates all files in
	// the shared volume as the root user. This restriction is circumvented by using
	// "docker cp" to create a copy that is not owned by the root user.
	require.NoError(t, os.RemoveAll(copyBackupDir))
	srcPath := testutil.DockerPrefix + "_alpha1_1:/data/backups"
	require.NoError(t, testutil.DockerCp(srcPath, copyBackupDir))
}

func AddItemSchema(t *testing.T, header http.Header, whichAlpha string) {
	updateSchemaParams := &common.GraphQLParams{
		Query: `mutation {
			    updateGQLSchema(
			      input: { set: { schema: "type Item {id: ID!, name: String! @search(by: [\"hash\"]), price: String!}"}})
			    {
			      gqlSchema {
					schema
			      }
			    }
			  }`,
		Variables: map[string]interface{}{},
		Headers:   header,
	}

	updateSchemaResp := updateSchemaParams.ExecuteAsPost(t, "http://"+testutil.ContainerAddr(whichAlpha, 8080)+"/admin")

	if len(updateSchemaResp.Errors) > 0 {
		t.Log("Failed to add Schema, Error: ", updateSchemaResp.Errors)
	}
}

func AddItem(t *testing.T, minSuffixVal int, maxSuffixVal int, jwtToken string, whichAlpha string) {
	query := `mutation addItem($name: String!, $price: String!){
		addItem(input: [{ name: $name, price: $price}]) {
		  item {
			id
			name
			price
		  }
		}
	  }`

	for i := minSuffixVal; i <= maxSuffixVal; i++ {
		params := testutil.GraphQLParams{Query: query,
			Variables: map[string]interface{}{
				"name":  "Item" + strconv.Itoa(i),
				"price": strconv.Itoa(i) + strconv.Itoa(i) + strconv.Itoa(i),
			}}
		b, err := json.Marshal(params)
		adminUrl := "http://" + testutil.ContainerAddr(whichAlpha, 8080) + "/graphql"
		require.NoError(t, err)

		req, err := http.NewRequest(http.MethodPost, adminUrl, bytes.NewBuffer(b))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set(accessJwtHeader, jwtToken)
		client := &http.Client{}
		resp, err := client.Do(req)
		require.NoError(t, err)
		var data interface{}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&data))
		require.NoError(t, resp.Body.Close())
	}
}

func CheckItemExists(t *testing.T, desriedSuffix int, jwtToken string, whichAlpha string) {
	checkData := `query queryItem($name: String!){
		queryItem(filter: {
			name: {eq: $name}
		}) {
			id
			name
			price
		}
	}`

	params := testutil.GraphQLParams{
		Query: checkData,
		Variables: map[string]interface{}{
			"name": "Item" + strconv.Itoa(desriedSuffix),
		},
	}

	b, err := json.Marshal(params)
	adminUrl := "http://" + testutil.ContainerAddr(whichAlpha, 8080) + "/graphql"
	require.NoError(t, err)

	req, _ := http.NewRequest(http.MethodPost, adminUrl, bytes.NewBuffer(b))
	req.Header.Set("Content-Type", "application/json")
	if jwtToken != "" {
		req.Header.Set(accessJwtHeader, jwtToken)
	}
	client := &http.Client{}
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer func() { require.NoError(t, resp.Body.Close()) }()

	var data interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&data))
	if strings.Contains(fmt.Sprint(data), "errors") {
		t.Fatalf("Item%v is not available in database", desriedSuffix)
	}
}

func TakeBackup(t *testing.T, jwtToken string, backupDst string, whichAlpha string) {

	backupRequest := `mutation backup($dst: String!) {
		backup(input: {destination: $dst}) {
			response {
				code
			}
			taskId
		}
	}`
	params := testutil.GraphQLParams{
		Query: backupRequest,
		Variables: map[string]interface{}{
			"dst": backupDst,
		},
	}

	b, err := json.Marshal(params)
	adminUrl := "http://" + testutil.ContainerAddr(whichAlpha, 8080) + "/admin"
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, adminUrl, bytes.NewBuffer(b))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	if jwtToken != "" {
		req.Header.Set(accessJwtHeader, jwtToken)
	}
	client := &http.Client{}
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer func() { require.NoError(t, resp.Body.Close()) }()

	var data interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&data))

	require.Equal(t, "Success", testutil.JsonGet(data, "data", "backup", "response", "code").(string))
	taskId := testutil.JsonGet(data, "data", "backup", "taskId").(string)
	testutil.WaitForTask(t, taskId, false, testutil.ContainerAddr(whichAlpha, 8080))
}

func RunRestore(t *testing.T, jwtToken string, restoreLocation string, whichAlpha string) {
	restoreRequest := `mutation restore($loc: String!) {
		restore(input: {location: $loc}) {
				code
				message
			}
	}`
	params := testutil.GraphQLParams{
		Query: restoreRequest,
		Variables: map[string]interface{}{
			"loc": restoreLocation,
		},
	}

	b, err := json.Marshal(params)
	adminUrl := "http://" + testutil.ContainerAddr(whichAlpha, 8080) + "/admin"
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, adminUrl, bytes.NewBuffer(b))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	if jwtToken != "" {
		req.Header.Set(accessJwtHeader, jwtToken)
	}
	client := &http.Client{}
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer func() { require.NoError(t, resp.Body.Close()) }()

	var data interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&data))

	require.Equal(t, "Success", testutil.JsonGet(data, "data", "restore", "code").(string))
}

func GetJwtTokenAndHeader(t *testing.T, whichAlpha string, namespaceId uint64) (string, http.Header) {
	var header = http.Header{}
	adminUrl := "http://" + testutil.ContainerAddr(whichAlpha, 8080) + "/admin"
	jwtToken := testutil.GrootHttpLoginNamespace(adminUrl, namespaceId).AccessJwt
	header.Set(accessJwtHeader, jwtToken)
	header.Set("Content-Type", "application/json")

	return jwtToken, header
}

// to copy files fron nfs server
func CopyToLocalFsFromNFS(t *testing.T, backupDst string, copyBackupDirectory string) {
	// The original backup files are not accessible because docker creates all files in
	// the shared volume as the root user. This restriction is circumvented by using
	// "docker cp" to create a copy that is not owned by the root user.
	require.NoError(t, os.RemoveAll(copyBackupDirectory))
	srcPath := testutil.DockerPrefix + "_nfs_1:/data" + backupDst
	require.NoError(t, testutil.DockerCp(srcPath, copyBackupDirectory))
}
