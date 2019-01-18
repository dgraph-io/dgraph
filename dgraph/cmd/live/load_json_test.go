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
	"testing"

	"github.com/dgraph-io/dgo"
	"github.com/dgraph-io/dgo/protos/api"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	"github.com/dgraph-io/dgraph/x"
	"github.com/dgraph-io/dgraph/z"
)

const alphaService = ":9180"

var testDataDir string
var dg *dgo.Dgraph
var tmpDir string

// move to z package?
func dropAll(t *testing.T) {
	err := dg.Alter(context.Background(), &api.Operation{DropAll: true})
	x.Check(err)

	resp, err := dg.NewTxn().Query(context.Background(), `
		{
			q(func: has(name)) {
				nodes : count(uid)
			}
		}
	`)
	x.Check(err)
	z.CompareJSON(t, `{"q":[{"nodes":0}]}`, string(resp.GetJson()))
}

func checkLoadedData(t *testing.T) {
	// just check the first and last entries and assume everything in between is okay

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
	dropAll(t)

	pipeline := [][]string{
		{os.ExpandEnv("$GOPATH/bin/dgraph"), "live",
			"--schema", testDataDir + "/family.schema", "--rdfs", testDataDir + "/family.json",
			"--dgraph", alphaService},
	}
	err := z.Pipeline(pipeline)
	require.NoError(t, err, "live loader ran successfully")

	checkLoadedData(t)
}

func TestLiveLoadCompressedJSONStream(t *testing.T) {
	dropAll(t)

	pipeline := [][]string{
		{"gzip", "-c", testDataDir + "/family.json"},
		{os.ExpandEnv("$GOPATH/bin/dgraph"), "live",
			"--schema", testDataDir + "/family.schema", "--rdfs", "/dev/stdin",
			"--dgraph", alphaService},
	}
	err := z.Pipeline(pipeline)
	require.NoError(t, err, "live loader ran successfully")

	checkLoadedData(t)
}

func TestMain(m *testing.M) {
	_, thisFile, _, _ := runtime.Caller(0)
	testDataDir = path.Dir(thisFile) + "/test_data"

	conn, err := grpc.Dial(alphaService, grpc.WithInsecure())
	x.Check(err)
	defer conn.Close()

	dg = dgo.NewDgraphClient(api.NewDgraphClient(conn))
	err = dg.Alter(context.Background(), &api.Operation{DropAll: true})
	x.Check(err)

	tmpDir, err = ioutil.TempDir("", "test.tmp-")
	x.Check(err)
	os.Chdir(tmpDir)
	defer os.RemoveAll(tmpDir)

	os.Exit(m.Run())
}
