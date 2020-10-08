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
	"testing"

	"github.com/dgraph-io/dgo/v200"
	"github.com/dgraph-io/dgo/v200/protos/api"
	minio "github.com/minio/minio-go/v6"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	"github.com/dgraph-io/dgraph/testutil"
)

var (
	mc             *minio.Client
	bucketName     = "dgraph-backup"
	destination    = "minio://minio1:9001/dgraph-backup?secure=false"
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
	result := requestExport(t)

	require.Equal(t, "Success", getFromJSON(result, "data", "export", "response", "code").(string))
	require.Equal(t, "Export completed.", getFromJSON(result, "data", "export", "response", "message").(string))

	var files []string
	for _, f := range getFromJSON(result, "data", "export", "exportedFiles").([]interface{}) {
		files = append(files, f.(string))
	}
	require.Equal(t, 3, len(files))

	schemaFile := files[1]
	require.Contains(t, schemaFile, ".schema.gz")

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

var expectedSchema = `<movie>:string .` + " " + `
<dgraph.cors>:[string] @index(exact) @upsert .` + " " + `
<dgraph.type>:[string] @index(exact) .` + " " + `
<dgraph.graphql.xid>:string @index(exact) @upsert .` + " " + `
<dgraph.graphql.schema>:string .` + " " + `
<dgraph.graphql.schema_history>:string .` + " " + `
<dgraph.graphql.schema_created_at>:datetime .` + " " + `
type <Node> {
	movie
}
type <dgraph.graphql> {
	dgraph.graphql.schema
	dgraph.graphql.xid
}
type <dgraph.graphql.history> {
	dgraph.graphql.schema_history
	dgraph.graphql.schema_created_at
}
`

func setupDgraph(t *testing.T) {
	conn, err := grpc.Dial(testutil.SockAddr, grpc.WithInsecure())
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

func requestExport(t *testing.T) map[string]interface{} {
	exportRequest := `mutation export($dst: String!) {
		export(input: {destination: $dst}) {
			response {
				code
				message
			}
			exportedFiles
		}
	}`

	adminUrl := "http://localhost:8180/admin"
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
	buf, err := ioutil.ReadAll(resp.Body)
	require.NoError(t, err)

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(buf, &result))

	return result
}

func getFromJSON(j map[string]interface{}, path ...string) interface{} {
	var res interface{} = j
	for _, p := range path {
		res = res.(map[string]interface{})[p]
	}
	return res
}
