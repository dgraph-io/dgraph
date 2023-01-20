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

package common

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
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
	namespaceId uint64
)

const (
	accessJwtHeader = "X-Dgraph-AccessToken"
	ShellToUse      = "bash"
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

func RemoveContentsOfPerticularDir(t *testing.T, dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	names, err := d.Readdirnames(-1)
	if err != nil {
		return err
	}
	for _, name := range names {
		err = os.RemoveAll(filepath.Join(dir, name))
		if err != nil {
			return err
		}
	}
	return nil
}

func AddNamespaces(t *testing.T, namespaceQuant int, headerAlpha1 http.Header) uint64 {
	for index := 1; index <= namespaceQuant; index++ {
		namespaceId = common.CreateNamespace(t, headerAlpha1)

		fmt.Printf("\nSucessfully added Namespace with Id: %d ", namespaceId)
	}

	return namespaceId
}

func DeleteNamespace(t *testing.T, id uint64, jwtTokenAlpha1 string) {
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
	adminUrl := "http://" + testutil.SockAddrHttp + "/admin"
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, adminUrl, bytes.NewBuffer(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(accessJwtHeader, jwtTokenAlpha1)
	client := &http.Client{}
	resp, err := client.Do(req)

	var data interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&data))

	deletedNamespaceID := testutil.JsonGet(data, "data", "deleteNamespace", "namespaceId").(float64)
	fmt.Println("")
	fmt.Println("..................................................>Deleted Schema id? ", deletedNamespaceID)
}

func AddSchema(t *testing.T, jwtTokenAlpha1 string) {
	query := `mutation {
        updateGQLSchema(
          input: { set: { schema: "type Item {id: ID!, name: String! @search(by: [hash]), price: String!}, type Post { postID: ID!, title: String! @search(by: [term, fulltext]), text: String @search(by: [fulltext, term]), datePublished: DateTime }"}})
        {
          gqlSchema {
			schema
          }
        }
      }`
	params := testutil.GraphQLParams{Query: query}
	b, err := json.Marshal(params)
	adminUrl := "http://" + testutil.SockAddrHttp + "/admin"
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, adminUrl, bytes.NewBuffer(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(accessJwtHeader, jwtTokenAlpha1)
	client := &http.Client{}
	resp, err := client.Do(req)

	var data interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&data))

	fmt.Println("")
	fmt.Println("..................................................>Add Schema ", data)
}

func CheckSchemaExists(t *testing.T, SockAddrHttp string, header http.Header) {
	resp := common.AssertGetGQLSchema(t, SockAddrHttp, header)
	require.NotNil(t, resp)
	fmt.Println("????????? Schema Exists? ", resp)
}

func AddData(t *testing.T, minSuffixVal int, maxSuffixVal int, jwtTokenAlpha1 string) {

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
		adminUrl := "http://" + testutil.SockAddrHttp + "/graphql"
		require.NoError(t, err)

		req, err := http.NewRequest(http.MethodPost, adminUrl, bytes.NewBuffer(b))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set(accessJwtHeader, jwtTokenAlpha1)
		client := &http.Client{}
		resp, err := client.Do(req)

		var data interface{}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&data))
	}
}

func CheckDataExists(t *testing.T, sockAdder string, desriedSuffix int, jwtTokenAlpha1 string) {
	checkData := `query queryItem($name: String!){
		queryItem(filter: {
			name: {eq: $name}
		})
		{
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
	adminUrl := "http://" + sockAdder + "/graphql"
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, adminUrl, bytes.NewBuffer(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(accessJwtHeader, jwtTokenAlpha1)
	client := &http.Client{}
	resp, err := client.Do(req)

	var data interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&data))

	lastInsertDetails := testutil.JsonGet(data, "data", "queryItem")
	fmt.Println("")
	fmt.Println("Details of the recently added record", lastInsertDetails)
}

func TakeBackup(t *testing.T, jwtTokenAlpha1 string, backupDst string) {

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
	adminUrl := "http://" + testutil.SockAddrHttp + "/admin"
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, adminUrl, bytes.NewBuffer(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(accessJwtHeader, jwtTokenAlpha1)
	client := &http.Client{}
	resp, err := client.Do(req)

	var data interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&data))

	fmt.Println("")
	fmt.Println("********************** BACKUP OUTPUT **********************", data)
	require.Equal(t, "Success", testutil.JsonGet(data, "data", "backup", "response", "code").(string))
	taskId := testutil.JsonGet(data, "data", "backup", "taskId").(string)
	testutil.WaitForTask(t, taskId, false)

	changeFolderPermission()
}

func changeFolderPermission() {

	out, errout, err := Shellout("cd data/backup")
	if err != nil {
		log.Printf("error: %v\n", err)
	}
	fmt.Println("--- stdout ---")
	fmt.Println(out)
	fmt.Println("--- stderr ---")
	fmt.Println(errout)

	out1, errout1, err1 := Shellout("echo 'Darksiders@1997' | sudo -S chmod -R a+rwx *")
	if err1 != nil {
		log.Printf("error: %v\n", err1)
	}
	fmt.Println("--- stdout ---")
	fmt.Println(out1)
	fmt.Println("--- stderr ---")
	fmt.Println(errout1)

}

func Shellout(command string) (string, string, error) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd := exec.Command(ShellToUse, "-c", command)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

func RunRestore(t *testing.T, jwtTokenAlpha2 string, restoreLocation string) string {
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
	adminUrl := "http://" + testutil.ContainerAddr("alpha2", 8080) + "/admin"
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, adminUrl, bytes.NewBuffer(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(accessJwtHeader, jwtTokenAlpha2)
	client := &http.Client{}
	resp, err := client.Do(req)

	var data interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&data))

	fmt.Println("")
	fmt.Println("********************** RESTORE OUTPUT **********************", data)

	//require.Equal(t, "Success", testutil.JsonGet(data, "data", "restore", "code").(string))
	receivedcode := testutil.JsonGet(data, "data", "restore", "code").(string)
	return receivedcode
}

func WaitForRestore(t *testing.T) {
	restoreDone := false
	for {
		resp, err := http.Get("http://" + testutil.ContainerAddr("alpha2", 8080) + "/health")
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
