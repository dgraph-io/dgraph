/*
 * Copyright 2017-2018 Dgraph Labs, Inc. and Contributors
 *
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
	"context"
	"io/ioutil"
	"log"
	"os"
	"path"
	"runtime"
	"testing"
	"time"

	"github.com/dgraph-io/dgo/v200"
	"github.com/dgraph-io/dgo/v200/protos/api"
	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/testutil"
	"github.com/dgraph-io/dgraph/x"
)

var alphaService = testutil.SockAddr
var zeroService = testutil.SockAddrZero

var (
	testDataDir string
	dg          *dgo.Dgraph
)

const (
	alphaName       = "alpha1"
	alphaExportPath = alphaName + ":/data/" + alphaName + "/export"
	localExportPath = "./export_copy"
)

func checkDifferentUid(t *testing.T, wantMap, gotMap map[string]interface{}) {
	require.NotEqual(t, gotMap["q"].([]interface{})[0].(map[string]interface{})["uid"],
		wantMap["q"].([]interface{})[0].(map[string]interface{})["uid"],
		"new uid was assigned")

	gotMap["q"].([]interface{})[0].(map[string]interface{})["uid"] = -1
	wantMap["q"].([]interface{})[0].(map[string]interface{})["uid"] = -1
	testutil.CompareJSONMaps(t, wantMap, gotMap)
}

func checkLoadedData(t *testing.T, newUids bool) {
	resp, err := dg.NewTxn().Query(context.Background(), `
		{
			q(func: anyofterms(name, "Homer")) {
				uid
				name
				age
				role
			}
		}
	`)
	require.NoError(t, err)

	gotMap := testutil.UnmarshalJSON(t, string(resp.GetJson()))
	wantMap := testutil.UnmarshalJSON(t, `
		{
		    "q": [
					{
					"uid": "0x2001",
					"name": "Homer",
					"age": 38,
					"role": "father"
			    }
			]
		}
	`)
	if newUids {
		checkDifferentUid(t, wantMap, gotMap)
	} else {
		testutil.CompareJSONMaps(t, wantMap, gotMap)
	}

	resp, err = dg.NewTxn().Query(context.Background(), `
		{
			q(func: anyofterms(name, "Maggie")) {
				uid
				name
				role
				carries
			}
		}
	`)
	require.NoError(t, err)

	gotMap = testutil.UnmarshalJSON(t, string(resp.GetJson()))
	wantMap = testutil.UnmarshalJSON(t, `
		{
		    "q": [
				{
					"uid": "0x3003",
					"name": "Maggie",
					"role": "daughter",
					"carries": "pacifier"
			    }
			]
		}
	`)
	if newUids {
		checkDifferentUid(t, wantMap, gotMap)
	} else {
		testutil.CompareJSONMaps(t, wantMap, gotMap)
	}
}

func TestLiveLoadJsonUidKeep(t *testing.T) {
	testutil.DropAll(t, dg)

	pipeline := [][]string{
		{testutil.DgraphBinaryPath(), "live",
			"--schema", testDataDir + "/family.schema", "--files", testDataDir + "/family.json",
			"--alpha", alphaService, "--zero", zeroService, "-u", "groot", "-p", "password"},
	}
	err := testutil.Pipeline(pipeline)
	require.NoError(t, err, "live loading JSON file exited with error")

	checkLoadedData(t, false)
}

func TestLiveLoadJsonUidDiscard(t *testing.T) {
	testutil.DropAll(t, dg)

	pipeline := [][]string{
		{testutil.DgraphBinaryPath(), "live", "--new_uids",
			"--schema", testDataDir + "/family.schema", "--files", testDataDir + "/family.json",
			"--alpha", alphaService, "--zero", zeroService, "-u", "groot", "-p", "password"},
	}
	err := testutil.Pipeline(pipeline)
	require.NoError(t, err, "live loading JSON file exited with error")

	checkLoadedData(t, true)
}

func TestLiveLoadRdfUidKeep(t *testing.T) {
	testutil.DropAll(t, dg)

	pipeline := [][]string{
		{testutil.DgraphBinaryPath(), "live",
			"--schema", testDataDir + "/family.schema", "--files", testDataDir + "/family.rdf",
			"--alpha", alphaService, "--zero", zeroService, "-u", "groot", "-p", "password"},
	}
	err := testutil.Pipeline(pipeline)
	require.NoError(t, err, "live loading JSON file exited with error")

	checkLoadedData(t, false)
}

func TestLiveLoadRdfUidDiscard(t *testing.T) {
	testutil.DropAll(t, dg)

	pipeline := [][]string{
		{testutil.DgraphBinaryPath(), "live", "--new_uids",
			"--schema", testDataDir + "/family.schema", "--files", testDataDir + "/family.rdf",
			"--alpha", alphaService, "--zero", zeroService, "-u", "groot", "-p", "password"},
	}
	err := testutil.Pipeline(pipeline)
	require.NoError(t, err, "live loading JSON file exited with error")

	checkLoadedData(t, true)
}

func TestLiveLoadExportedSchema(t *testing.T) {
	testutil.DropAll(t, dg)

	// initiate export
	params := &testutil.GraphQLParams{
		Query: `
			mutation {
			  export(input: {format: "rdf"}) {
				response {
				  code
				  message
				}
			  }
			}`,
	}
	accessJwt, _ := testutil.GrootHttpLogin("http://" + testutil.SockAddrHttp + "/admin")
	resp := testutil.MakeGQLRequestWithAccessJwt(t, params, accessJwt)
	require.Nilf(t, resp.Errors, resp.Errors.Error())

	// wait a bit to be sure export is complete
	time.Sleep(time.Second)

	// copy the unzipped export files from docker
	exportId := unzipAndCopyExportToLocalFs(t)

	// then load the exported files
	pipeline := [][]string{
		{testutil.DgraphBinaryPath(), "live",
			"--schema", localExportPath + "/" + exportId + "/g01.schema",
			"--files", localExportPath + "/" + exportId + "/g01.rdf",
			"--alpha", alphaService, "--zero", zeroService, "-u", "groot", "-p", "password"},
	}
	err := testutil.Pipeline(pipeline)
	require.NoError(t, err, "live loading exported schema exited with error")

	// cleanup copied export files
	if err := os.RemoveAll(localExportPath); err != nil {
		t.Fatalf("Error removing export copy directory: %s", err.Error())
	}
}

func unzipAndCopyExportToLocalFs(t *testing.T) string {
	if err := os.RemoveAll(localExportPath); err != nil {
		t.Fatalf("Error removing directory: %s", err.Error())
	}
	if err := testutil.DockerExec(alphaName, "gunzip", "-rf", "export"); err != nil {
		t.Fatalf("Error unzipping files in docker container: %s", err.Error())
	}
	if err := testutil.DockerCp(alphaExportPath, localExportPath); err != nil {
		t.Fatalf("Error copying files from docker container: %s", err.Error())
	}
	childDirs, err := ioutil.ReadDir(localExportPath)
	if err != nil {
		t.Fatalf("Couldn't read local export copy directory: %v", err)
	}
	if len(childDirs) == 0 {
		t.Fatalf("Local export copy directory is empty!!!")
	}
	return childDirs[0].Name()
}

func TestMain(m *testing.M) {
	_, thisFile, _, _ := runtime.Caller(0)
	testDataDir = path.Dir(thisFile)

	var err error
	dg, err = testutil.DgraphClientWithGroot(testutil.SockAddr)
	if err != nil {
		log.Fatalf("Error while getting a dgraph client: %v", err)
	}
	x.Check(dg.Alter(
		context.Background(), &api.Operation{DropAll: true}))

	// Try to create any files in a dedicated temp directory that gets cleaned up
	// instead of all over /tmp or the working directory.
	tmpDir, err := ioutil.TempDir("", "test.tmp-")
	x.Check(err)
	os.Chdir(tmpDir)
	defer os.RemoveAll(tmpDir)

	os.Exit(m.Run())
}
