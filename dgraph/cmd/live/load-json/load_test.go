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

var alphaService = z.SockAddr
var zeroService = z.SockAddrZero

var (
	testDataDir string
	dg          *dgo.Dgraph
	tmpDir      string
)

// Just check the first and last entries and assumes everything in between is okay.
func checkLoadedData(t *testing.T) {
	resp, err := dg.NewTxn().Query(context.Background(), `
		{
			q(func: anyofterms(name, "Homer")) {
				name
				age
				role @facets(gender,generation)
				role@es
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
					"role": "father",
					"role@es": "padre",
					"role|gender": "male"
				}
			]
		}
	`, string(resp.GetJson()))

	resp, err = dg.NewTxn().Query(context.Background(), `
		{
			q(func: anyofterms(name, "Maggie")) {
				name
				role @facets(gender,generation)
				role@es
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
					"role@es": "hija",
					"carries": "pacifier",
					"role|gender": "female",
					"role|generation": 3
				}
			]
		}
	`, string(resp.GetJson()))
}

func TestLiveLoadJSONFileEmpty(t *testing.T) {
	z.DropAll(t, dg)

	pipeline := [][]string{
		{"echo", "[]"},
		{os.ExpandEnv("$GOPATH/bin/dgraph"), "live",
			"--schema", testDataDir + "/family.schema", "--files", "/dev/stdin",
			"--alpha", alphaService, "--zero", zeroService},
	}
	err := z.Pipeline(pipeline)
	require.NoError(t, err, "live loading JSON file ran successfully")
}

func TestLiveLoadJSONFile(t *testing.T) {
	z.DropAll(t, dg)

	pipeline := [][]string{
		{os.ExpandEnv("$GOPATH/bin/dgraph"), "live",
			"--schema", testDataDir + "/family.schema", "--files", testDataDir + "/family.json",
			"--alpha", alphaService, "--zero", zeroService},
	}
	err := z.Pipeline(pipeline)
	require.NoError(t, err, "live loading JSON file exited with error")

	checkLoadedData(t)
}

func TestLiveLoadJSONCompressedStream(t *testing.T) {
	z.DropAll(t, dg)

	pipeline := [][]string{
		{"gzip", "-c", testDataDir + "/family.json"},
		{os.ExpandEnv("$GOPATH/bin/dgraph"), "live",
			"--schema", testDataDir + "/family.schema", "--files", "/dev/stdin",
			"--alpha", alphaService, "--zero", zeroService},
	}
	err := z.Pipeline(pipeline)
	require.NoError(t, err, "live loading JSON stream exited with error")

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
			"--schema", testDataDir + "/family.schema", "--files", fileList,
			"--alpha", alphaService, "--zero", zeroService},
	}
	err := z.Pipeline(pipeline)
	require.NoError(t, err, "live loading multiple JSON files exited with error")

	checkLoadedData(t)
}

func TestMain(m *testing.M) {
	_, thisFile, _, _ := runtime.Caller(0)
	testDataDir = path.Dir(thisFile)

	dg = z.DgraphClientWithGroot(z.SockAddr)

	// Try to create any files in a dedicated temp directory that gets cleaned up
	// instead of all over /tmp or the working directory.
	tmpDir, err := ioutil.TempDir("", "test.tmp-")
	x.Check(err)
	os.Chdir(tmpDir)
	defer os.RemoveAll(tmpDir)

	os.Exit(m.Run())
}
