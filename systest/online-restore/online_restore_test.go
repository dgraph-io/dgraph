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
	"net/http"
	"path"
	"runtime"
	"strings"
	"testing"

	"github.com/dgraph-io/dgo/v200"
	"github.com/dgraph-io/dgo/v200/protos/api"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	"github.com/dgraph-io/dgraph/chunker"
	"github.com/dgraph-io/dgraph/testutil"
)

func sendRestoreRequest(t *testing.T, backupId string) {
	restoreRequest := fmt.Sprintf(`mutation restore() {
		 restore(input: {location: "/data/backup", backupId: "%s",
		 	encryptionKeyFile: "/data/keys/enc_key"}) {
			response {
				code
				message
			}
		}
	}`, backupId)

	adminUrl := "http://localhost:8180/admin"
	params := testutil.GraphQLParams{
		Query: restoreRequest,
	}
	b, err := json.Marshal(params)
	require.NoError(t, err)

	resp, err := http.Post(adminUrl, "application/json", bytes.NewBuffer(b))
	require.NoError(t, err)
	buf, err := ioutil.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Contains(t, string(buf), "Restore completed.")
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
			require.Error(t, err)
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
	conn, err := grpc.Dial(testutil.SockAddr, grpc.WithInsecure())
	require.NoError(t, err)
	dg := dgo.NewDgraphClient(api.NewDgraphClient(conn))

	ctx := context.Background()
	require.NoError(t, dg.Alter(ctx, &api.Operation{DropAll: true}))

	sendRestoreRequest(t, "cranky_bartik8")
	runQueries(t, dg, false)
	runMutations(t, dg)
}

func TestMoveTablets(t *testing.T) {
	conn, err := grpc.Dial(testutil.SockAddr, grpc.WithInsecure())
	require.NoError(t, err)
	dg := dgo.NewDgraphClient(api.NewDgraphClient(conn))

	ctx := context.Background()
	require.NoError(t, dg.Alter(ctx, &api.Operation{DropAll: true}))

	sendRestoreRequest(t, "cranky_bartik8")
	runQueries(t, dg, false)

	// Send another restore request with a different backup. This backup has some of the
	// same predicates as the previous one but they are stored in different groups.
	sendRestoreRequest(t, "awesome_dirac9")

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
			response {
				code
				message
			}
		}
	}`

	adminUrl := "http://localhost:8180/admin"
	params := testutil.GraphQLParams{
		Query: restoreRequest,
	}
	b, err := json.Marshal(params)
	require.NoError(t, err)

	resp, err := http.Post(adminUrl, "application/json", bytes.NewBuffer(b))
	require.NoError(t, err)
	buf, err := ioutil.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Contains(t, string(buf), "failed to verify backup")
}

func TestListBackups(t *testing.T) {
	query := `query backup() {
		listBackups(input: {location: "/data/backup"}) {
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

	adminUrl := "http://localhost:8180/admin"
	params := testutil.GraphQLParams{
		Query: query,
	}
	b, err := json.Marshal(params)
	require.NoError(t, err)

	resp, err := http.Post(adminUrl, "application/json", bytes.NewBuffer(b))
	require.NoError(t, err)
	buf, err := ioutil.ReadAll(resp.Body)
	require.NoError(t, err)
	sbuf := string(buf)
	require.Contains(t, sbuf, `"backupId":"cranky_bartik8"`)
	require.Contains(t, sbuf, `"backupNum":1`)
	require.Contains(t, sbuf, `"backupNum":2`)
	require.Contains(t, sbuf, "initial_release_date")
}
