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
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/badger/v3/options"
	"github.com/dgraph-io/dgraph/graphql/e2e/common"
	"github.com/dgraph-io/dgraph/testutil"
	"github.com/dgraph-io/dgraph/worker"
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
	shellToUse      = "bash"
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

//***************************************New Adds

func RemoveContentsOfPerticularDir(t *testing.T, dir string) {
	d, err := os.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	names, err := d.Readdirnames(-1)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		if name != ".gitkeep" {
			err = os.RemoveAll(filepath.Join(dir, name))
			if err != nil {
				t.Fatal(err)
			}
		}
	}
}

func AddNamespaces(t *testing.T, namespaceQuant int, header http.Header, customAdminURL string) uint64 {
	var namespaceId uint64
	for index := 1; index <= namespaceQuant; index++ {
		if customAdminURL != "" {
			namespaceId = common.CreateNamespace(t, header, customAdminURL)
		} else {
			namespaceId = common.CreateNamespace(t, header)
		}
	}
	t.Logf("Sucessfully added Namespace with Id: %d ", namespaceId)

	return namespaceId
}

func DeleteNamespace(t *testing.T, id uint64, jwtToken string, whichAlpha string) {
	query := `mutation deleteNamespace($id:Int!){
					deleteNamespace(input:{namespaceId:$id}){
						namespaceId
					}
				}`
	params := testutil.GraphQLParams{Query: query,
		Variables: map[string]interface{}{
			"id": id,
		}}
	b, err := json.Marshal(params)
	adminUrl := "http://" + testutil.ContainerAddr(whichAlpha, 8080) + "/admin"
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, adminUrl, bytes.NewBuffer(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(accessJwtHeader, jwtToken)
	client := &http.Client{}
	resp, err := client.Do(req)

	var data interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&data))
}

func AddSchema(t *testing.T, header http.Header, whichAlpha string) {
	updateSchemaParams := &common.GraphQLParams{
		Query: `mutation {
			    updateGQLSchema(
			      input: { set: { schema: "type Item {id: ID!, name: String! @search(by: [hash]), price: String!}, type Post { postID: ID!, title: String! @search(by: [term, fulltext]), text: String @search(by: [fulltext, term]), datePublished: DateTime }"}})
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

func CheckSchemaExists(t *testing.T, header http.Header, whichAlpha string) {
	resp := common.AssertGetGQLSchema(t, testutil.ContainerAddr(whichAlpha, 8080), header)
	require.NotNil(t, resp)
}

func AddData(t *testing.T, minSuffixVal int, maxSuffixVal int, jwtToken string, whichAlpha string) {

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
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set(accessJwtHeader, jwtToken)
		client := &http.Client{}
		resp, err := client.Do(req)

		var data interface{}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&data))
	}
}

func CheckDataExists(t *testing.T, desriedSuffix int, jwtToken string, whichAlpha string) {
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

	req, err := http.NewRequest(http.MethodPost, adminUrl, bytes.NewBuffer(b))
	req.Header.Set("Content-Type", "application/json")
	if jwtToken != "" {
		req.Header.Set(accessJwtHeader, jwtToken)
	}
	client := &http.Client{}
	resp, err := client.Do(req)

	var data interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&data))
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
	req.Header.Set("Content-Type", "application/json")
	if jwtToken != "" {
		req.Header.Set(accessJwtHeader, jwtToken)
	}
	client := &http.Client{}
	resp, err := client.Do(req)

	var data interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&data))

	require.Equal(t, "Success", testutil.JsonGet(data, "data", "backup", "response", "code").(string))
	taskId := testutil.JsonGet(data, "data", "backup", "taskId").(string)
	testutil.WaitForTask(t, taskId, false, testutil.ContainerAddr(whichAlpha, 8080))

	defer changeFolderPermission(t, whichAlpha)
}

func changeFolderPermission(t *testing.T, whichAlpha string) {

	cmd := []string{"bash", "-c", "chmod -R 777 /data/backups"}
	require.NoError(t, testutil.DockerExec(whichAlpha, cmd...))

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
	req.Header.Set("Content-Type", "application/json")
	if jwtToken != "" {
		req.Header.Set(accessJwtHeader, jwtToken)
	}
	client := &http.Client{}
	resp, err := client.Do(req)

	var data interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&data))

	require.Equal(t, "Success", testutil.JsonGet(data, "data", "restore", "code").(string))
}

func WaitForRestore(t *testing.T, whichAlpha string) {
	restoreDone := false
	for {
		resp, err := http.Get("http://" + testutil.ContainerAddr(whichAlpha, 8080) + "/health")
		require.NoError(t, err)
		buf, err := ioutil.ReadAll(resp.Body)
		require.NoError(t, err)
		sbuf := string(buf)
		if !strings.Contains(sbuf, "opRestore") {
			restoreDone = true
			break
		}
		time.Sleep(4 * time.Second)
	}
	require.True(t, restoreDone)

	time.Sleep(5 * time.Second)
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
