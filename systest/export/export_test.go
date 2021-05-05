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
	"compress/gzip"
	"context"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"strings"
	"testing"

	"github.com/dgraph-io/dgo/v210"
	"github.com/dgraph-io/dgo/v210/protos/api"
	minio "github.com/minio/minio-go/v6"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	"github.com/dgraph-io/dgraph/testutil"
)

var (
	mc             *minio.Client
	bucketName     = "dgraph-backup"
	destination    = "minio://minio:9001/dgraph-backup?secure=false"
	localBackupDst = "minio://localhost:9001/dgraph-backup?secure=false"
)

// TestExportSchemaToMinio. This test does an export, then verifies that the
// schema file has been exported to minio. The unit tests test the actual data
// exported, so it's not tested here again
func TestExportSchemaToMinio(t *testing.T) {
	mc, err := testutil.NewMinioClient()
	require.NoError(t, err)
	mc.MakeBucket(bucketName, "")

	setupDgraph(t)
	requestExport(t)

	schemaFile := ""
	doneCh := make(chan struct{})
	defer close(doneCh)
	for obj := range mc.ListObjectsV2(bucketName, "dgraph.", true, doneCh) {
		if strings.Contains(obj.Key, ".schema.gz") {
			schemaFile = obj.Key
		}
	}
	require.NotEmpty(t, schemaFile)

	object, err := mc.GetObject(bucketName, schemaFile, minio.GetObjectOptions{})
	require.NoError(t, err)
	defer object.Close()

	reader, err := gzip.NewReader(object)
	require.NoError(t, err)
	defer reader.Close()

	bytes, err := ioutil.ReadAll(reader)
	require.NoError(t, err)
	require.Equal(t, expectedSchema, string(bytes))
}

var expectedSchema = `[0x0] <movie>:string .` + " " + `
[0x0] <dgraph.type>:[string] @index(exact) .` + " " + `
[0x0] <dgraph.drop.op>:string .` + " " + `
[0x0] <dgraph.graphql.xid>:string @index(exact) @upsert .` + " " + `
[0x0] <dgraph.graphql.schema>:string .` + " " + `
[0x0] <dgraph.graphql.p_query>:string @index(sha256) .` + " " + `
[0x0] type <Node> {
	movie
}
[0x0] type <dgraph.graphql> {
	dgraph.graphql.schema
	dgraph.graphql.xid
}
[0x0] type <dgraph.graphql.persisted_query> {
	dgraph.graphql.p_query
}
`

func setupDgraph(t *testing.T) {
	conn, err := grpc.Dial(testutil.SockAddr, grpc.WithInsecure())
	require.NoError(t, err)
	dg := dgo.NewDgraphClient(api.NewDgraphClient(conn))

	ctx := context.Background()
	require.NoError(t, testutil.RetryAlter(dg, &api.Operation{DropAll: true}))

	// Add schema and types.
	// this is because Alters are always blocked until the indexing is finished.
	require.NoError(t, testutil.RetryAlter(dg, &api.Operation{Schema: `movie: string .
		type Node {
			movie
		}`}))

	// Add initial data.
	_, err = dg.NewTxn().Mutate(ctx, &api.Mutation{
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
}

func requestExport(t *testing.T) {
	exportRequest := `mutation export($dst: String!) {
		export(input: {destination: $dst}) {
			response {
				code
			}
			taskId
		}
	}`

	adminUrl := "http://" + testutil.SockAddrHttp + "/admin"
	params := testutil.GraphQLParams{
		Query: exportRequest,
		Variables: map[string]interface{}{
			"dst": destination,
		},
	}
	b, err := json.Marshal(params)
	require.NoError(t, err)

	resp, err := http.Post(adminUrl, "application/json", bytes.NewBuffer(b))
	require.NoError(t, err)

	var data interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&data))
	require.Equal(t, "Success", testutil.JsonGet(data, "data", "export", "response", "code").(string))
	taskId := testutil.JsonGet(data, "data", "export", "taskId").(string)
	testutil.WaitForTask(t, taskId, false)
}
