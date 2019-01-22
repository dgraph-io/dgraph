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

package live

import (
	"context"
	"io/ioutil"
	"os"
	"path"
	"runtime"
	"strings"
	"testing"

	"github.com/dgraph-io/dgo"
	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/x"
	"github.com/dgraph-io/dgraph/z"
)

const alphaService = ":9180"

var testDataDir string
var dg *dgo.Dgraph
var tmpDir string

// Just check the first and last entries and assumes everything in between is okay.
func checkLoadedData(t *testing.T) {
	resp, err := dg.NewTxn().Query(context.Background(), `
		{
			q(func: anyofterms(name, "Homer")) {
				name
				age
				role
			}
		}
	`)
	require.NoError(t, err)
	z.CompareJSON(t, `
		{
		    "q": [
					{
					"name": "Homer",
					"age": 38,
					"role": "father"
			    }
			]
		}
	`, string(resp.GetJson()))

	resp, err = dg.NewTxn().Query(context.Background(), `
		{
			q(func: anyofterms(name, "Maggie")) {
				name
				role
				carries
			}
		}
	`)
	require.NoError(t, err)
	z.CompareJSON(t, `
		{
		    "q": [
				{
					"name": "Maggie",
					"role": "daughter",
					"carries": "pacifier"
			    }
			]
		}
	`, string(resp.GetJson()))
}

func TestLiveLoadJSONFile(t *testing.T) {
	z.DropAll(t, dg)

	pipeline := [][]string{
		{os.ExpandEnv("$GOPATH/bin/dgraph"), "live",
			"--schema", testDataDir + "/family.schema", "--rdfs", testDataDir + "/family.json",
			"--dgraph", alphaService},
	}
	err := z.Pipeline(pipeline)
	require.NoError(t, err, "live loading JSON file ran successfully")

	checkLoadedData(t)
}

func TestLiveLoadJSONCompressedStream(t *testing.T) {
	z.DropAll(t, dg)

	pipeline := [][]string{
		{"gzip", "-c", testDataDir + "/family.json"},
		{os.ExpandEnv("$GOPATH/bin/dgraph"), "live",
			"--schema", testDataDir + "/family.schema", "--rdfs", "/dev/stdin",
			"--dgraph", alphaService},
	}
	err := z.Pipeline(pipeline)
	require.NoError(t, err, "live loading JSON stream ran successfully")

	checkLoadedData(t)
}

func TestLiveLoadJSONMultipleFiles(t *testing.T) {
	z.DropAll(t, dg)

	files := []string{
		testDataDir + "/family1.json",
		testDataDir + "/family2.json",
		testDataDir + "/family3.json",
	}
	fileList := strings.Join(files, ",")

	pipeline := [][]string{
		{os.ExpandEnv("$GOPATH/bin/dgraph"), "live",
			"--schema", testDataDir + "/family.schema", "--rdfs", fileList,
			"--dgraph", alphaService},
	}
	err := z.Pipeline(pipeline)
	require.NoError(t, err, "live loading multiple JSON files ran successfully")

	checkLoadedData(t)
}

func TestLiveLoadJSONAutoUID(t *testing.T) {
	z.DropAll(t, dg)

	// Remove UID from test data to verify that live loading it fails.
	pipeline := [][]string{
		{"grep", "-v", "uid", testDataDir + "/family.json"},
		{os.ExpandEnv("$GOPATH/bin/dgraph"), "live",
			"--schema", testDataDir + "/family.schema", "--rdfs", "/dev/stdin",
			"--dgraph", alphaService},
	}
	err := z.Pipeline(pipeline)
	require.Error(t, err, "live loading JSON file without uid failed")
	require.Equal(t, 0, z.DbNodeCount(t, dg), "no data was loaded")

	// Remove UID and use "role" as the key to create UIDs from. Only 4 of the 6 family members
	// should be in the final database, one for each of "father", "mother", "son", and "daughter".
	pipeline = [][]string{
		{"grep", "-v", "uid", testDataDir + "/family.json"},
		{os.ExpandEnv("$GOPATH/bin/dgraph"), "live",
			"--schema", testDataDir + "/family.schema", "--rdfs", "/dev/stdin",
			"--dgraph", alphaService, "--key", "role"},
	}
	err = z.Pipeline(pipeline)
	require.NoError(t, err, "live loading JSON file with auto-uid ran successfully")
	require.Equal(t, 4, z.DbNodeCount(t, dg), "not all data was loaded")

	z.DropAll(t, dg)

	// Remove UID and use "role" and "age" as the key to create UIDs from. That should be enough
	// to make all members unique, so all should be in final database.
	pipeline = [][]string{
		{"grep", "-v", "uid", testDataDir + "/family.json"},
		{os.ExpandEnv("$GOPATH/bin/dgraph"), "live",
			"--schema", testDataDir + "/family.schema", "--rdfs", "/dev/stdin",
			"--dgraph", alphaService, "--key", "role,age"},
	}
	err = z.Pipeline(pipeline)
	require.NoError(t, err, "live loading JSON file with auto-uid ran successfully")
	require.Equal(t, 6, z.DbNodeCount(t, dg), "all data was loaded")
	checkLoadedData(t)
}

func TestMain(m *testing.M) {
	_, thisFile, _, _ := runtime.Caller(0)
	testDataDir = path.Dir(thisFile) + "/test_data"

	dg = z.DgraphClient(alphaService)

	// Try to create any files in a dedicated temp directory that gets cleaned up
	// instead of all over /tmp or the working directory.
	tmpDir, err := ioutil.TempDir("", "test.tmp-")
	x.Check(err)
	os.Chdir(tmpDir)
	defer os.RemoveAll(tmpDir)

	os.Exit(m.Run())
}
