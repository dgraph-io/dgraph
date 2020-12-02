/*
 * Copyright 2020 Dgraph Labs, Inc. and Contributors *
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
	"os"
	"path"
	"runtime"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc/credentials"

	"github.com/dgraph-io/dgo/v200"
	"github.com/dgraph-io/dgo/v200/protos/api"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	"github.com/dgraph-io/dgraph/chunker"
	"github.com/dgraph-io/dgraph/testutil"
)

func sendRestoreRequest(t *testing.T, location, backupId string, backupNum int) int {
	if location == "" {
		location = "/data/backup2"
	}
	params := testutil.GraphQLParams{
		Query: `mutation restore($location: String!, $backupId: String, $backupNum: Int) {
			restore(input: {location: $location, backupId: $backupId, backupNum: $backupNum,
				encryptionKeyFile: "/data/keys/enc_key"}) {
				code
				message
				restoreId
			}
		}`,
		Variables: map[string]interface{}{
			"location":  location,
			"backupId":  backupId,
			"backupNum": backupNum,
		},
	}
	resp := testutil.MakeGQLRequestWithTLS(t, &params, testutil.GetAlphaClientConfig(t))
	resp.RequireNoGraphQLErrors(t)

	var restoreResp struct {
		Restore struct {
			Code      string
			Message   string
			RestoreId int
		}
	}
	require.NoError(t, json.Unmarshal(resp.Data, &restoreResp))
	require.Equal(t, restoreResp.Restore.Code, "Success")
	require.Greater(t, restoreResp.Restore.RestoreId, 0)
	return restoreResp.Restore.RestoreId
}

func waitForRestore(t *testing.T, restoreId int, dg *dgo.Dgraph) {
	query := fmt.Sprintf(`query status() {
		 restoreStatus(restoreId: %d) {
			status
			errors
		}
	}`, restoreId)
	params := testutil.GraphQLParams{
		Query: query,
	}
	b, err := json.Marshal(params)
	require.NoError(t, err)

	restoreDone := false
	client := testutil.GetHttpsClient(t)
	for i := 0; i < 15; i++ {
		resp, err := client.Post(testutil.AdminUrlHttps(), "application/json", bytes.NewBuffer(b))
		require.NoError(t, err)
		buf, err := ioutil.ReadAll(resp.Body)
		require.NoError(t, err)
		sbuf := string(buf)
		if strings.Contains(sbuf, "OK") {
			restoreDone = true
			break
		}
		time.Sleep(4 * time.Second)
	}
	require.True(t, restoreDone)

	// Wait for the client to exit draining mode. This is needed because the client might
	// be connected to a follower and might be behind the leader in applying the restore.
	// Waiting for three consecutive successful queries is done to prevent a situation in
	// which the query succeeds at the first attempt because the follower is behind and
	// has not started to apply the restore proposal.
	numSuccess := 0
	for {
		// This is a dummy query that returns no results.
		_, err = dg.NewTxn().Query(context.Background(), `{
		q(func: has(invalid_pred)) {
			invalid_pred
		}}`)

		if err == nil {
			numSuccess += 1
		} else {
			require.Contains(t, err.Error(), "the server is in draining mode")
			numSuccess = 0
		}

		if numSuccess == 3 {
			// The server has been responsive three times in a row.
			break
		}
		time.Sleep(1 * time.Second)
	}
}

// disableDraining disables draining mode before each test for increased reliability.
func disableDraining(t *testing.T) {
	drainRequest := `mutation draining {
 		draining(enable: false) {
    		response {
        		code
        		message
      		}
  		}
	}`

	params := testutil.GraphQLParams{
		Query: drainRequest,
	}
	b, err := json.Marshal(params)
	require.NoError(t, err)
	client := testutil.GetHttpsClient(t)
	resp, err := client.Post(testutil.AdminUrlHttps(), "application/json", bytes.NewBuffer(b))
	require.NoError(t, err)
	buf, err := ioutil.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Contains(t, string(buf), "draining mode has been set to false")
}

func runQueries(t *testing.T, dg *dgo.Dgraph, shouldFail bool) {
	_, thisFile, _, _ := runtime.Caller(0)
	queryDir := path.Join(path.Dir(thisFile), "queries")

	files, err := ioutil.ReadDir(queryDir)
	require.NoError(t, err)

	for _, file := range files {
		if !strings.HasPrefix(file.Name(), "query-") {
			continue
		}
		filename := path.Join(queryDir, file.Name())
		reader, cleanup := chunker.FileReader(filename, nil)
		bytes, err := ioutil.ReadAll(reader)
		require.NoError(t, err)
		contents := string(bytes)
		cleanup()

		// The test query and expected result are separated by a delimiter.
		bodies := strings.SplitN(contents, "\n---\n", 2)

		resp, err := dg.NewTxn().Query(context.Background(), bodies[0])
		if shouldFail {
			if err != nil {
				continue
			}
			require.False(t, testutil.EqualJSON(t, bodies[1], string(resp.GetJson()), "", true))
		} else {
			require.NoError(t, err)
			require.True(t, testutil.EqualJSON(t, bodies[1], string(resp.GetJson()), "", true))
		}
	}
}

func runMutations(t *testing.T, dg *dgo.Dgraph) {
	// Mutate existing data to verify uid leas was set correctly.
	_, err := dg.NewTxn().Mutate(context.Background(), &api.Mutation{
		SetNquads: []byte(`
			<0x1> <test> "aaa" .
		`),
		CommitNow: true,
	})
	require.NoError(t, err)

	resp, err := dg.NewTxn().Query(context.Background(), `{
	  q(func: has(test), orderasc: test) {
		test
	  }
	}`)
	require.NoError(t, err)
	require.JSONEq(t, `{"q":[{"test":"aaa"}]}`, string(resp.Json))

	// Send mutation with blank nodes to verify new UIDs are generated.
	_, err = dg.NewTxn().Mutate(context.Background(), &api.Mutation{
		SetNquads: []byte(`
			_:b <test> "bbb" .
			_:c <test> "ccc" .
		`),
		CommitNow: true,
	})
	require.NoError(t, err)

	resp, err = dg.NewTxn().Query(context.Background(), `{
	  q(func: has(test), orderasc: test) {
		test
	  }
	}`)
	require.NoError(t, err)
	require.JSONEq(t, `{"q":[{"test":"aaa"}, {"test": "bbb"}, {"test": "ccc"}]}`, string(resp.Json))
}

func TestBasicRestore(t *testing.T) {
	disableDraining(t)

	conn, err := grpc.Dial(testutil.SockAddr, grpc.WithTransportCredentials(credentials.NewTLS(testutil.GetAlphaClientConfig(t))))
	require.NoError(t, err)
	dg := dgo.NewDgraphClient(api.NewDgraphClient(conn))

	ctx := context.Background()
	require.NoError(t, dg.Alter(ctx, &api.Operation{DropAll: true}))

	restoreId := sendRestoreRequest(t, "", "youthful_rhodes3", 0)
	waitForRestore(t, restoreId, dg)
	runQueries(t, dg, false)
	runMutations(t, dg)
}

func TestRestoreBackupNum(t *testing.T) {
	disableDraining(t)

	conn, err := grpc.Dial(testutil.SockAddr, grpc.WithTransportCredentials(credentials.NewTLS(testutil.GetAlphaClientConfig(t))))
	require.NoError(t, err)
	dg := dgo.NewDgraphClient(api.NewDgraphClient(conn))

	ctx := context.Background()
	require.NoError(t, dg.Alter(ctx, &api.Operation{DropAll: true}))
	runQueries(t, dg, true)

	restoreId := sendRestoreRequest(t, "", "youthful_rhodes3", 1)
	waitForRestore(t, restoreId, dg)
	runQueries(t, dg, true)
	runMutations(t, dg)
}

func TestRestoreBackupNumInvalid(t *testing.T) {
	disableDraining(t)

	conn, err := grpc.Dial(testutil.SockAddr, grpc.WithTransportCredentials(credentials.NewTLS(testutil.GetAlphaClientConfig(t))))
	require.NoError(t, err)
	dg := dgo.NewDgraphClient(api.NewDgraphClient(conn))

	ctx := context.Background()
	require.NoError(t, dg.Alter(ctx, &api.Operation{DropAll: true}))
	runQueries(t, dg, true)

	// Send a request with a backupNum greater than the number of manifests.
	restoreRequest := fmt.Sprintf(`mutation restore() {
		 restore(input: {location: "/data/backup2", backupId: "%s", backupNum: %d,
		 	encryptionKeyFile: "/data/keys/enc_key"}) {
			code
			message
			restoreId
		}
	}`, "youthful_rhodes3", 1000)

	params := testutil.GraphQLParams{
		Query: restoreRequest,
	}
	b, err := json.Marshal(params)
	require.NoError(t, err)

	client := testutil.GetHttpsClient(t)
	resp, err := client.Post(testutil.AdminUrlHttps(), "application/json", bytes.NewBuffer(b))
	require.NoError(t, err)
	buf, err := ioutil.ReadAll(resp.Body)
	bufString := string(buf)
	require.NoError(t, err)
	require.Contains(t, bufString, "not enough backups")

	// Send a request with a negative backupNum value.
	restoreRequest = fmt.Sprintf(`mutation restore() {
		 restore(input: {location: "/data/backup2", backupId: "%s", backupNum: %d,
		 	encryptionKeyFile: "/data/keys/enc_key"}) {
			code
			message
			restoreId
		}
	}`, "youthful_rhodes3", -1)

	params = testutil.GraphQLParams{
		Query: restoreRequest,
	}
	b, err = json.Marshal(params)
	require.NoError(t, err)

	resp, err = client.Post(testutil.AdminUrlHttps(), "application/json", bytes.NewBuffer(b))
	require.NoError(t, err)
	buf, err = ioutil.ReadAll(resp.Body)
	bufString = string(buf)
	require.NoError(t, err)
	require.Contains(t, bufString, "backupNum value should be equal or greater than zero")
}

func TestMoveTablets(t *testing.T) {
	disableDraining(t)

	conn, err := grpc.Dial(testutil.SockAddr, grpc.WithTransportCredentials(credentials.NewTLS(testutil.GetAlphaClientConfig(t))))
	require.NoError(t, err)
	dg := dgo.NewDgraphClient(api.NewDgraphClient(conn))

	ctx := context.Background()
	require.NoError(t, dg.Alter(ctx, &api.Operation{DropAll: true}))

	restoreId := sendRestoreRequest(t, "", "youthful_rhodes3", 0)
	waitForRestore(t, restoreId, dg)
	runQueries(t, dg, false)

	// Send another restore request with a different backup. This backup has some of the
	// same predicates as the previous one but they are stored in different groups.
	restoreId = sendRestoreRequest(t, "", "blissful_hermann1", 0)
	waitForRestore(t, restoreId, dg)

	resp, err := dg.NewTxn().Query(context.Background(), `{
	  q(func: has(name), orderasc: name) {
		name
	  }
	}`)
	require.NoError(t, err)
	require.JSONEq(t, `{"q":[{"name":"Person 1"}, {"name": "Person 2"}]}`, string(resp.Json))

	resp, err = dg.NewTxn().Query(context.Background(), `{
	  q(func: has(tagline), orderasc: tagline) {
		tagline
	  }
	}`)
	require.NoError(t, err)
	require.JSONEq(t, `{"q":[{"tagline":"Tagline 1"}]}`, string(resp.Json))

	// Run queries based on the first restored backup and verify none of the old data
	// is still accessible.
	runQueries(t, dg, true)
}

func TestInvalidBackupId(t *testing.T) {
	restoreRequest := `mutation restore() {
		 restore(input: {location: "/data/backup", backupId: "bad-backup-id",
			encryptionKeyFile: "/data/keys/enc_key"}) {
				code
				message
				restoreId
		}
	}`

	params := testutil.GraphQLParams{
		Query: restoreRequest,
	}
	b, err := json.Marshal(params)
	require.NoError(t, err)
	client := testutil.GetHttpsClient(t)
	resp, err := client.Post(testutil.AdminUrlHttps(), "application/json", bytes.NewBuffer(b))
	require.NoError(t, err)
	buf, err := ioutil.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Contains(t, string(buf), "failed to verify backup")
}

func TestListBackups(t *testing.T) {
	query := `query backup() {
		listBackups(input: {location: "/data/backup2"}) {
			backupId
			backupNum
			encrypted
			groups {
				groupId
				predicates
			}
			path
			since
			type
		}
	}`

	params := testutil.GraphQLParams{
		Query: query,
	}
	b, err := json.Marshal(params)
	require.NoError(t, err)

	client := testutil.GetHttpsClient(t)
	resp, err := client.Post(testutil.AdminUrlHttps(), "application/json", bytes.NewBuffer(b))
	require.NoError(t, err)
	buf, err := ioutil.ReadAll(resp.Body)
	require.NoError(t, err)
	sbuf := string(buf)
	require.Contains(t, sbuf, `"backupId":"youthful_rhodes3"`)
	require.Contains(t, sbuf, `"backupNum":1`)
	require.Contains(t, sbuf, `"backupNum":2`)
	require.Contains(t, sbuf, "initial_release_date")
}

func TestRestoreWithDropOperations(t *testing.T) {
	conn, err := grpc.Dial(testutil.SockAddr, grpc.WithTransportCredentials(credentials.NewTLS(testutil.GetAlphaClientConfig(t))))
	require.NoError(t, err)
	dg := dgo.NewDgraphClient(api.NewDgraphClient(conn))
	for {
		// keep retrying until we succeed or receive a non-retriable error
		err = dg.Alter(context.Background(), &api.Operation{DropAll: true})
		if err == nil || !strings.Contains(err.Error(), "Please retry") {
			break
		}
		time.Sleep(time.Second)
	}
	require.NoError(t, err)

	// apply initial schema
	initialSchema := `
		name: string .
		age: int .
		type Person {
			name
			age
		}`
	require.NoError(t, dg.Alter(context.Background(), &api.Operation{Schema: initialSchema}))

	// add some data
	alice := `
		_:alice <name> "Alice" .
		_:alice <age> "20" .
		_:alice <dgraph.type> "Person" .
	`
	_, err = dg.NewTxn().Mutate(context.Background(), &api.Mutation{SetNquads: []byte(alice),
		CommitNow: true})
	require.NoError(t, err)

	backupDir := "/data/backup"
	// create a full backup in backupDir
	backup(t, backupDir)

	// add some more data
	bob := `
		_:bob <name> "Bob" .
		_:bob <age> "25" .
		_:bob <dgraph.type> "Person" .
	`
	_, err = dg.NewTxn().Mutate(context.Background(), &api.Mutation{SetNquads: []byte(bob),
		CommitNow: true})
	require.NoError(t, err)

	// create another backup (incremental) in backupDir
	backup(t, backupDir)

	// Now start testing
	queryPersonCount := `
	{
		persons(func: type(Person)) {
			count(uid)
		}
	}`
	queryPerson := `
	{
		persons(func: type(Person)) {
			name
			age
		}
	}`
	initialPreds := `
	{"predicate":"name", "type": "string"},
	{"predicate":"age", "type": "int"}`
	initialTypes := `
	{
		"fields": [{"name": "name"},{"name": "age"}],
		"name": "Person"
	}`

	/* STEP-1: DROP_ATTR, reapply schema for that attr then add some new values for the same attr.
	Then backup and restore.
	Verify that dropped ATTR is not there for old nodes and other data is intact.
	Also verify that the schema is intact after restore.
	*/
	require.NoError(t, dg.Alter(context.Background(), &api.Operation{
		DropOp:    api.Operation_ATTR,
		DropValue: "age",
	}))
	require.NoError(t, dg.Alter(context.Background(), &api.Operation{Schema: `age: int .`}))
	charlie := `
		_:charlie <name> "Charlie" .
		_:charlie <age> "30" .
		_:charlie <dgraph.type> "Person" .
	`
	_, err = dg.NewTxn().Mutate(context.Background(), &api.Mutation{SetNquads: []byte(charlie),
		CommitNow: true})
	require.NoError(t, err)
	backupRestoreAndVerify(t, dg, backupDir,
		queryPerson,
		`{
		  "persons": [
			{
			  "name": "Alice"
			},
			{
			  "name": "Bob"
			},
			{
			  "name": "Charlie",
			  "age": 30
			}
		  ]
		}`,
		testutil.SchemaOptions{
			UserPreds: initialPreds,
			UserTypes: initialTypes,
		})

	/* STEP-2: DROP_DATA, then backup and restore.
	Verify that there is no user data, and the schema is intact after restore.
	*/
	require.NoError(t, dg.Alter(context.Background(), &api.Operation{DropOp: api.Operation_DATA}))
	backupRestoreAndVerify(t, dg, backupDir,
		queryPersonCount,
		`{"persons":[{"count":0}]}`,
		testutil.SchemaOptions{
			UserPreds: initialPreds,
			UserTypes: initialTypes,
		})

	/* STEP-3: DROP_ALL, then backup and restore.
	Verify that there is no user data, and no user schema.
	*/
	require.NoError(t, dg.Alter(context.Background(), &api.Operation{DropOp: api.Operation_ALL}))
	backupRestoreAndVerify(t, dg, backupDir,
		queryPersonCount,
		`{"persons":[{"count":0}]}`,
		testutil.SchemaOptions{})

	/* STEP-4: Apply a schema, and backup to backupDir. Then add some data, and do a DROP_ALL.
	Now, add different schema and data, and do a DROP_DATA.
	Then, backup and restore using backupDir.
	Verify that the schema is latest and there is no data.
	*/
	require.NoError(t, dg.Alter(context.Background(), &api.Operation{Schema: initialSchema}))
	backup(t, backupDir)
	_, err = dg.NewTxn().Mutate(context.Background(), &api.Mutation{SetNquads: []byte(alice),
		CommitNow: true})
	require.NoError(t, err)
	require.NoError(t, dg.Alter(context.Background(), &api.Operation{DropOp: api.Operation_ALL}))
	require.NoError(t, dg.Alter(context.Background(), &api.Operation{
		Schema: `
		color: String .
		type Flower {
			color
		}`}))
	_, err = dg.NewTxn().Mutate(context.Background(), &api.Mutation{SetNquads: []byte(`
		_:flower <color> "yellow" .
	`),
		CommitNow: true})
	require.NoError(t, err)
	require.NoError(t, dg.Alter(context.Background(), &api.Operation{DropOp: api.Operation_DATA}))
	backupRestoreAndVerify(t, dg, backupDir,
		`{
			flowers(func: has(color)) {
				count(uid)
			}
		}`,
		`{"flowers":[{"count":0}]}`,
		testutil.SchemaOptions{
			UserPreds: `{"predicate":"color", "type": "string"}`,
			UserTypes: `
			{
				"fields": [{"name": "color"}],
				"name": "Flower"
			}`,
		})
}

func setupDirs(t *testing.T, dirs []string) {
	// first, clean them up
	cleanupDirs(t, dirs)

	// then create them
	for _, dir := range dirs {
		require.NoError(t, os.MkdirAll(dir, os.ModePerm))
	}
}

func cleanupDirs(t *testing.T, dirs []string) {
	for _, dir := range dirs {
		if err := os.RemoveAll(dir); err != nil {
			t.Logf("Got error while removing: %s: %v\n", dir, err)
		}
	}
}

func backup(t *testing.T, backupDir string) {
	backupParams := &testutil.GraphQLParams{
		Query: `mutation($backupDir: String!) {
		  backup(input: {
			destination: $backupDir
		  }) {
			response {
			  code
			  message
			}
		  }
		}`,
		Variables: map[string]interface{}{"backupDir": backupDir},
	}
	testutil.MakeGQLRequestWithTLS(t, backupParams, testutil.GetAlphaClientConfig(t)).RequireNoGraphQLErrors(t)
}

func backupRestoreAndVerify(t *testing.T, dg *dgo.Dgraph, backupDir, queryToVerify,
	expectedResponse string, schemaVerificationOpts testutil.SchemaOptions) {
	schemaVerificationOpts.ExcludeAclSchema = true
	backup(t, backupDir)
	waitForRestore(t, sendRestoreRequest(t, backupDir, "", 0), dg)
	testutil.VerifyQueryResponse(t, dg, queryToVerify, expectedResponse)
	testutil.VerifySchema(t, dg, schemaVerificationOpts)
}
