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

package online_restore

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"path"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/dgraph-io/dgraph/x"
	"google.golang.org/grpc/credentials"

	"github.com/dgraph-io/dgo/v200"
	"github.com/dgraph-io/dgo/v200/protos/api"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	"github.com/dgraph-io/dgraph/chunker"
	"github.com/dgraph-io/dgraph/testutil"
)

func sendRestoreRequest(t *testing.T, backupId string, backupNum int) int {
	restoreRequest := fmt.Sprintf(`mutation restore() {
		 restore(input: {location: "/data/backup", backupId: "%s", backupNum: %d,
		 	encryptionKeyFile: "/data/keys/enc_key"}) {
			code
			message
			restoreId
		}
	}`, backupId, backupNum)
	tlsConf := getAlphaClient(t)
	client := http.Client{
		Timeout: time.Second * 3,
		Transport: &http.Transport{
			TLSClientConfig: tlsConf,
		},
	}

	adminUrl := "https://" + testutil.SockAddrHttp + "/admin"
	params := testutil.GraphQLParams{
		Query: restoreRequest,
	}
	b, err := json.Marshal(params)
	require.NoError(t, err)

	resp, err := client.Post(adminUrl, "application/json", bytes.NewBuffer(b))
	require.NoError(t, err)
	buf, err := ioutil.ReadAll(resp.Body)
	bufString := string(buf)
	require.NoError(t, err)
	require.Contains(t, bufString, "Success")
	jsonMap := make(map[string]map[string]interface{})
	require.NoError(t, json.Unmarshal([]byte(bufString), &jsonMap))
	restoreId := int(jsonMap["data"]["restore"].(map[string]interface{})["restoreId"].(float64))
	require.NotEqual(t, "", restoreId)
	return restoreId
}

func waitForRestore(t *testing.T, restoreId int, dg *dgo.Dgraph) {
	query := fmt.Sprintf(`query status() {
		 restoreStatus(restoreId: %d) {
			status
			errors
		}
	}`, restoreId)
	tlsConf := getAlphaClient(t)
	client := http.Client{
		Timeout: time.Second * 3,
		Transport: &http.Transport{
			TLSClientConfig: tlsConf,
		},
	}

	adminUrl := "https://" + testutil.SockAddrHttp + "/admin"
	params := testutil.GraphQLParams{
		Query: query,
	}
	b, err := json.Marshal(params)
	require.NoError(t, err)

	restoreDone := false
	for i := 0; i < 15; i++ {
		resp, err := client.Post(adminUrl, "application/json", bytes.NewBuffer(b))
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

	tlsConf := getAlphaClient(t)
	client := http.Client{
		Timeout: time.Second * 3,
		Transport: &http.Transport{
			TLSClientConfig: tlsConf,
		},
	}
	adminUrl := "https://" + testutil.SockAddrHttp + "/admin"
	params := testutil.GraphQLParams{
		Query: drainRequest,
	}
	b, err := json.Marshal(params)
	require.NoError(t, err)
	resp, err := client.Post(adminUrl, "application/json", bytes.NewBuffer(b))
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
	tlsConf := getAlphaClient(t)
	conn, err := grpc.Dial(testutil.SockAddr, grpc.WithTransportCredentials(credentials.NewTLS(tlsConf)))
	require.NoError(t, err)
	dg := dgo.NewDgraphClient(api.NewDgraphClient(conn))

	ctx := context.Background()
	require.NoError(t, dg.Alter(ctx, &api.Operation{DropAll: true}))

	restoreId := sendRestoreRequest(t, "youthful_rhodes3", 0)
	waitForRestore(t, restoreId, dg)
	runQueries(t, dg, false)
	runMutations(t, dg)
}

func TestRestoreBackupNum(t *testing.T) {
	disableDraining(t)
	tlsConf := getAlphaClient(t)
	conn, err := grpc.Dial(testutil.SockAddr, grpc.WithTransportCredentials(credentials.NewTLS(tlsConf)))
	require.NoError(t, err)
	dg := dgo.NewDgraphClient(api.NewDgraphClient(conn))

	ctx := context.Background()
	require.NoError(t, dg.Alter(ctx, &api.Operation{DropAll: true}))
	runQueries(t, dg, true)

	restoreId := sendRestoreRequest(t, "youthful_rhodes3", 1)
	waitForRestore(t, restoreId, dg)
	runQueries(t, dg, true)
	runMutations(t, dg)
}

func TestRestoreBackupNumInvalid(t *testing.T) {
	disableDraining(t)
	tlsConf := getAlphaClient(t)
	conn, err := grpc.Dial(testutil.SockAddr, grpc.WithTransportCredentials(credentials.NewTLS(tlsConf)))
	require.NoError(t, err)
	dg := dgo.NewDgraphClient(api.NewDgraphClient(conn))

	ctx := context.Background()
	require.NoError(t, dg.Alter(ctx, &api.Operation{DropAll: true}))
	runQueries(t, dg, true)
	client := http.Client{
		Timeout: time.Second * 3,
		Transport: &http.Transport{
			TLSClientConfig: tlsConf,
		},
	}
	// Send a request with a backupNum greater than the number of manifests.
	adminUrl := "https://" + testutil.SockAddrHttp + "/admin"
	restoreRequest := fmt.Sprintf(`mutation restore() {
		 restore(input: {location: "/data/backup", backupId: "%s", backupNum: %d,
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

	resp, err := client.Post(adminUrl, "application/json", bytes.NewBuffer(b))
	require.NoError(t, err)
	buf, err := ioutil.ReadAll(resp.Body)
	bufString := string(buf)
	require.NoError(t, err)
	require.Contains(t, bufString, "not enough backups")

	// Send a request with a negative backupNum value.
	restoreRequest = fmt.Sprintf(`mutation restore() {
		 restore(input: {location: "/data/backup", backupId: "%s", backupNum: %d,
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

	resp, err = client.Post(adminUrl, "application/json", bytes.NewBuffer(b))
	require.NoError(t, err)
	buf, err = ioutil.ReadAll(resp.Body)
	bufString = string(buf)
	require.NoError(t, err)
	require.Contains(t, bufString, "backupNum value should be equal or greater than zero")
}

func TestMoveTablets(t *testing.T) {
	disableDraining(t)
	tlsConf := getAlphaClient(t)
	conn, err := grpc.Dial(testutil.SockAddr, grpc.WithTransportCredentials(credentials.NewTLS(tlsConf)))
	require.NoError(t, err)
	dg := dgo.NewDgraphClient(api.NewDgraphClient(conn))

	ctx := context.Background()
	require.NoError(t, dg.Alter(ctx, &api.Operation{DropAll: true}))

	restoreId := sendRestoreRequest(t, "youthful_rhodes3", 0)
	waitForRestore(t, restoreId, dg)
	runQueries(t, dg, false)

	// Send another restore request with a different backup. This backup has some of the
	// same predicates as the previous one but they are stored in different groups.
	restoreId = sendRestoreRequest(t, "blissful_hermann1", 0)
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

	tlsConf := getAlphaClient(t)
	client := http.Client{
		Timeout: time.Second * 3,
		Transport: &http.Transport{
			TLSClientConfig: tlsConf,
		},
	}
	adminUrl := "https://" + testutil.SockAddrHttp + "/admin"
	params := testutil.GraphQLParams{
		Query: restoreRequest,
	}
	b, err := json.Marshal(params)
	require.NoError(t, err)

	resp, err := client.Post(adminUrl, "application/json", bytes.NewBuffer(b))
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

	tlsConf := getAlphaClient(t)
	client := http.Client{
		Timeout: time.Second * 3,
		Transport: &http.Transport{
			TLSClientConfig: tlsConf,
		},
	}
	adminUrl := "https://" + testutil.SockAddrHttp + "/admin"
	params := testutil.GraphQLParams{
		Query: query,
	}
	b, err := json.Marshal(params)
	require.NoError(t, err)

	resp, err := client.Post(adminUrl, "application/json", bytes.NewBuffer(b))
	require.NoError(t, err)
	buf, err := ioutil.ReadAll(resp.Body)
	require.NoError(t, err)
	sbuf := string(buf)
	require.Contains(t, sbuf, `"backupId":"youthful_rhodes3"`)
	require.Contains(t, sbuf, `"backupNum":1`)
	require.Contains(t, sbuf, `"backupNum":2`)
	require.Contains(t, sbuf, "initial_release_date")
}

func getAlphaClient(t *testing.T) *tls.Config {
	c := &x.TLSHelperConfig{
		CertRequired:     true,
		Cert:             "../tls/alpha1/client.alpha1.crt",
		Key:              "../tls/alpha1/client.alpha1.key",
		ServerName:       "alpha1",
		RootCACert:       "../tls/alpha1/ca.crt",
		UseSystemCACerts: true,
	}
	tlsConf, err := x.GenerateClientTLSConfig(c)
	require.NoError(t, err)
	return tlsConf
}
