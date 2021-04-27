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
	"os"
	"path/filepath"
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
	minioDest      = "minio://minio:9001/dgraph-backup?secure=false"
	localBackupDst = "minio://localhost:9001/dgraph-backup?secure=false"
	copyExportDir  = "./data/export-copy"
)

// TestExportSchemaToMinio. This test does an export, then verifies that the
// schema file has been exported to minio. The unit tests test the actual data
// exported, so it's not tested here again
func TestExportSchemaToMinio(t *testing.T) {
	mc, err := testutil.NewMinioClient()
	require.NoError(t, err)
	mc.MakeBucket(bucketName, "")

	setupDgraph(t, moviesData, movieSchema)
	result := requestExport(t, minioDest, "rdf")

	require.Equal(t, "Success", getFromJSON(result, "data", "export", "response", "code").(string))
	require.Equal(t, "Export completed.",
		getFromJSON(result, "data", "export", "response", "message").(string))

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
var moviesData = `<_:x1> <movie> "BIRDS MAN OR (THE UNEXPECTED VIRTUE OF IGNORANCE)" .
	<_:x2> <movie> "Spotlight" .
	<_:x3> <movie> "Moonlight" .
	<_:x4> <movie> "THE SHAPE OF WATERLOO" .
	<_:x5> <movie> "BLACK PUNTER" .`

var movieSchema = `
	movie: string .
	type Node {
			movie
	}`

func TestExportAndLoadJson(t *testing.T) {
	setupDgraph(t, moviesData, movieSchema)

	// Run export
	result := requestExport(t, "/data/export-data", "json")
	require.Equal(t, "Success", getFromJSON(result, "data", "export", "response", "code").(string))
	require.Equal(t, "Export completed.",
		getFromJSON(result, "data", "export", "response", "message").(string))

	var files []string
	for _, f := range getFromJSON(result, "data", "export", "exportedFiles").([]interface{}) {
		files = append(files, f.(string))
	}
	require.Equal(t, 3, len(files))
	copyToLocalFs(t)

	q := `{ q(func:has(movie)) { count(uid) } }`

	res := runQuery(t, q)
	require.JSONEq(t, `{"data":{"q":[{"count": 5}]}}`, res)

	// Drop all data
	dg, err := testutil.DgraphClient(testutil.SockAddr)
	require.NoError(t, err)
	err = dg.Alter(context.Background(), &api.Operation{DropAll: true})
	require.NoError(t, err)

	res = runQuery(t, q)
	require.JSONEq(t, `{"data": {"q": [{"count":0}]}}`, res)

	// Live load the exported data
	base := filepath.Dir(files[0])
	dir := filepath.Join(copyExportDir, base)
	loadData(t, dir, "json")

	res = runQuery(t, q)
	require.JSONEq(t, `{"data":{"q":[{"count": 5}]}}`, res)

	dirCleanup(t)
}

var facetsData = `
	_:blank-0 <name> "Carol" .
	_:blank-0 <friend> _:blank-1 (close="yes") .
	_:blank-1 <name> "Daryl" .

	_:a <pred> "test" (f="test") .
	_:a <predlist> "London" (cont="England") .
	_:a <predlist> "Paris" (cont="France") .
	_:a <name> "alice" .

	_:b <refone> _:a (f="something") .
	_:b <name> "bob" .
	`

var facetsSchema = `
	<name>: string @index(exact) .
	<friend>: [uid] .
	<refone>: uid .
	<predlist>: [string] .
`

func TestExportAndLoadJsonFacets(t *testing.T) {
	setupDgraph(t, facetsData, facetsSchema)

	// Run export
	result := requestExport(t, "/data/export-data", "json")
	require.Equal(t, "Success", getFromJSON(result, "data", "export", "response", "code").(string))
	require.Equal(t, "Export completed.",
		getFromJSON(result, "data", "export", "response", "message").(string))

	var files []string
	for _, f := range getFromJSON(result, "data", "export", "exportedFiles").([]interface{}) {
		files = append(files, f.(string))
	}
	require.Equal(t, 3, len(files))
	copyToLocalFs(t)

	checkRes := func() {
		// Check value posting.
		q := `{ q(func:has(name)) { pred @facets } }`
		res := runQuery(t, q)
		require.JSONEq(t, `{"data": {"q": [{"pred": "test", "pred|f": "test"}]}}`, res)

		// Check value postings of list type.
		q = `{ q(func:has(name)) {	predlist @facets } }`
		res = runQuery(t, q)
		require.JSONEq(t, `{"data": {"q": [{
		      "predlist|cont": {"0": "England","1": "France"},
		      "predlist": ["London","Paris" ]}]}}`, res)

		// Check reference posting.
		q = `{ q(func:has(name)) { refone @facets {name} } }`
		res = runQuery(t, q)
		require.JSONEq(t,
			`{"data":{"q":[{"refone":{"name":"alice","refone|f":"something"}}]}}`, res)

		// Check reference postings of list type.
		q = `{ q(func:has(name)) { friend @facets {name} } }`
		res = runQuery(t, q)
		require.JSONEq(t,
			`{"data":{"q":[{"friend":[{"name":"Daryl","friend|close":"yes"}]}]}}`, res)
	}

	checkRes()

	// Drop all data
	dg, err := testutil.DgraphClient(testutil.SockAddr)
	require.NoError(t, err)
	err = dg.Alter(context.Background(), &api.Operation{DropAll: true})
	require.NoError(t, err)

	res := runQuery(t, `{ q(func:has(name)) { name } }`)
	require.JSONEq(t, `{"data": {"q": []}}`, res)

	// Live load the exported data and verify that exported data is loaded correctly.
	base := filepath.Dir(files[0])
	dir := filepath.Join(copyExportDir, base)
	loadData(t, dir, "json")

	// verify that the state after loading the exported data as same.
	checkRes()
	dirCleanup(t)
}

func runQuery(t *testing.T, q string) string {
	dg, err := testutil.DgraphClient(testutil.SockAddr)
	require.NoError(t, err)

	resp, err := testutil.RetryQuery(dg, q)
	require.NoError(t, err)
	response := map[string]interface{}{}
	response["data"] = json.RawMessage(string(resp.Json))

	jsonResponse, err := json.Marshal(response)
	require.NoError(t, err)
	return string(jsonResponse)
}

func copyToLocalFs(t *testing.T) {
	require.NoError(t, os.RemoveAll(copyExportDir))
	srcPath := testutil.DockerPrefix + "_alpha1_1:/data/export-data"
	require.NoError(t, testutil.DockerCp(srcPath, copyExportDir))
}

func loadData(t *testing.T, dir, format string) {
	schemaFile := dir + "/g01.schema.gz"
	dataFile := dir + "/g01." + format + ".gz"

	pipeline := [][]string{
		{testutil.DgraphBinaryPath(), "live",
			"-s", schemaFile, "-f", dataFile, "--alpha",
			testutil.SockAddr, "--zero", testutil.SockAddrZero,
		},
	}
	_, err := testutil.Pipeline(pipeline)
	require.NoErrorf(t, err, "Got error while loading data: %v", err)

}

func dirCleanup(t *testing.T) {
	require.NoError(t, os.RemoveAll("./t"))
	require.NoError(t, os.RemoveAll("./data"))

	cmd := []string{"bash", "-c", "rm -rf /data/export-data/*"}
	require.NoError(t, testutil.DockerExec("alpha1", cmd...))
}

func setupDgraph(t *testing.T, nquads, schema string) {

	require.NoError(t, os.MkdirAll("./data", os.ModePerm))
	conn, err := grpc.Dial(testutil.SockAddr, grpc.WithInsecure())
	require.NoError(t, err)
	dg := dgo.NewDgraphClient(api.NewDgraphClient(conn))

	ctx := context.Background()
	require.NoError(t, testutil.RetryAlter(dg, &api.Operation{DropAll: true}))

	// Add schema and types.
	// this is because Alters are always blocked until the indexing is finished.
	require.NoError(t, testutil.RetryAlter(dg, &api.Operation{Schema: schema}))

	// Add initial data.
	_, err = dg.NewTxn().Mutate(ctx, &api.Mutation{
		CommitNow: true,
		SetNquads: []byte(nquads),
	})
	require.NoError(t, err)
}

func requestExport(t *testing.T, dest string, format string) map[string]interface{} {
	exportRequest := `mutation export($dst: String!, $f: String!) {
		export(input: {destination: $dst, format: $f}) {
			response {
				code
				message
			}
			exportedFiles
		}
	}`

	adminUrl := "http://" + testutil.SockAddrHttp + "/admin"
	params := testutil.GraphQLParams{
		Query: exportRequest,
		Variables: map[string]interface{}{
			"dst": dest,
			"f":   format,
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
