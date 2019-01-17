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
	"os/exec"
	"path"
	"runtime"
	"testing"

	"github.com/dgraph-io/dgo"
	"github.com/dgraph-io/dgo/protos/api"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	dgtest "github.com/dgraph-io/dgraph/testing"
	"github.com/dgraph-io/dgraph/x"
)

const alphaHost = ":9180"

var testDataDir string
var conn *grpc.ClientConn
var dg *dgo.Dgraph

func mkTempDir() string {
	tmpDir, err := ioutil.TempDir(os.Getenv("TMPDIR"), "test.tmp-")
	x.Check(err)
	return tmpDir
}

func runOn(conn *grpc.ClientConn, fn func(*testing.T, *dgo.Dgraph)) func(*testing.T) {
	return func(t *testing.T) {
		dg := dgo.NewDgraphClient(api.NewDgraphClient(conn))
		fn(t, dg)
	}
}

func TestLiveLoadJSON(t *testing.T) {
	tmpDir := mkTempDir()
	defer os.RemoveAll(tmpDir)
	//fmt.Fprintf(os.Stderr, "TEMP DIR = %s\n", tmpDir)

	liveCmd := exec.Command(os.ExpandEnv("$GOPATH/bin/dgraph"), "live",
		"--schema", testDataDir+"/family.schema", "--rdfs", testDataDir+"/family.json",
		"--dgraph", alphaHost,
	)
	liveCmd.Dir = tmpDir
	liveCmd.Stdout, _ = os.Create(tmpDir + "/live.out")
	liveCmd.Stderr = liveCmd.Stdout
	err := liveCmd.Run()
	require.NoError(t, err, "live loader ran successfully")

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
	dgtest.CompareJSON(t, `
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
	dgtest.CompareJSON(t, `
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

func TestMain(m *testing.M) {
	_, thisFile, _, _ := runtime.Caller(0)
	testDataDir = path.Dir(thisFile) + "/test_data"

	conn, err := grpc.Dial(alphaHost, grpc.WithInsecure())
	x.Check(err)
	defer conn.Close()

	dg = dgo.NewDgraphClient(api.NewDgraphClient(conn))
	err = dg.Alter(context.Background(), &api.Operation{DropAll: true})
	x.Check(err)

	os.Exit(m.Run())
}
