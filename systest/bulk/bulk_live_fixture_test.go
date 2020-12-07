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
	"fmt"
	"github.com/dgraph-io/dgo/v200/protos/api"
	"github.com/stretchr/testify/require"
	"io/ioutil"
	"log"
	"math/rand"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/dgraph-io/dgraph/testutil"
	"github.com/pkg/errors"
)

var rootDir = "./data"

type suite struct {
	t           *testing.T
	opts        suiteOpts
}

type suiteOpts struct {
	schema         string
	gqlSchema      string
	rdfs           string
	skipBulkLoader bool
	//skipLiveLoader bool
}

func newSuiteInternal(t *testing.T, opts suiteOpts) *suite {
	if testing.Short() {
		t.Skip("Skipping system test with long runtime.")
	}

	s := &suite{
		t:    t,
		opts: opts,
	}

	s.checkFatal(makeDirEmpty(rootDir))
	rdfFile := filepath.Join(rootDir, "rdfs.rdf")
	s.checkFatal(ioutil.WriteFile(rdfFile, []byte(opts.rdfs), 0644))
	schemaFile := filepath.Join(rootDir, "schema.txt")
	s.checkFatal(ioutil.WriteFile(schemaFile, []byte(opts.schema), 0644))
	gqlSchemaFile := filepath.Join(rootDir, "gql_schema.txt")
	s.checkFatal(ioutil.WriteFile(gqlSchemaFile, []byte(opts.gqlSchema), 0644))
	s.setup( "schema.txt", "rdfs.rdf", "gql_schema.txt")
	return s
}

func newSuite(t *testing.T, schema, rdfs string) *suite {
	opts := suiteOpts{
		schema: schema,
		rdfs:   rdfs,
	}
	return newSuiteInternal(t, opts)
}

func newBulkOnlySuite(t *testing.T, schema, rdfs, gqlSchema string) *suite {
	opts := suiteOpts{
		schema:         schema,
		gqlSchema:      gqlSchema,
		rdfs:           rdfs,
		//skipLiveLoader: true,
	}
	return newSuiteInternal(t, opts)
}

func newSuiteFromFile(t *testing.T, schemaFile, rdfFile, gqlSchemaFile string) *suite {
	if testing.Short() {
		t.Skip("Skipping system test with long runtime.")
	}
	s := &suite{t: t}

	s.setup(schemaFile, rdfFile, gqlSchemaFile)
	return s
}

func (s *suite) setup(schemaFile, rdfFile, gqlSchemaFile string) {
	s.checkFatal(
		makeDirEmpty(filepath.Join(rootDir, "out", "0")),
	)

	if !s.opts.skipBulkLoader {
		bulkCmd := exec.Command(testutil.DgraphBinaryPath(), "bulk",
			"-f", rdfFile,
			"-s", schemaFile,
			"-g", gqlSchemaFile,
			"--http", "localhost:"+strconv.Itoa(freePort(0)),
			"-j=1",
			"--store-xids=true",
			"--zero", testutil.SockAddrZero,
		)
		bulkCmd.Dir = rootDir
		log.Print(bulkCmd.String())
		if out, err := bulkCmd.CombinedOutput(); err != nil {
			s.cleanup()
			s.t.Logf("%s", out)
			s.t.Fatalf("Bulkloader didn't run: %v\n", err)
		}

		cmd := exec.CommandContext(context.Background(), "docker-compose", "-f", "./alpha.yml", "-p", testutil.DockerPrefix,
			"up", "-d", "--force-recreate", "alpha1")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Env = os.Environ()
		if err := cmd.Run(); err != nil {
			fmt.Printf("Error while bringing up alpha node. Prefix: %s. Error: %v\n", testutil.DockerPrefix, err)
			s.cleanup()
			s.t.Fatalf("Couldn't start alpha in Dgraph cluster: %v\n", err)
		}
		for i := 0; i < 30; i++ {
			time.Sleep(time.Second)
			resp, err := http.Get(testutil.ContainerAddr("alpha1", 8080) + "/health")
			if resp != nil && resp.Body != nil {
				resp.Body.Close()
			}
			if err == nil && resp.StatusCode == http.StatusOK {
				return
			}
		}
	}
	//
	//if !s.opts.skipLiveLoader {
	//	liveCmd := exec.Command(testutil.DgraphBinaryPath(), "live",
	//		"--files", rdfFile,
	//		"--schema", schemaFile,
	//		"--alpha", testutil.ContainerAddr("alpha1", 9080),
	//		"--zero", testutil.SockAddrZero,
	//	)
	//	liveCmd.Dir = rootDir
	//	if out, err := liveCmd.Output(); err != nil {
	//		s.cleanup()
	//		s.t.Logf("%s", out)
	//		s.t.Fatalf("Live Loader didn't run: %v\n", err)
	//	}
	//}
}

func makeDirEmpty(dir string) error {
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	return os.MkdirAll(dir, 0755)
}

func (s *suite) cleanup() {
	// NOTE: Shouldn't raise any errors here or fail a test, since this is
	// called when we detect an error (don't want to mask the original problem).
	dg, err := testutil.DgraphClient(testutil.ContainerAddr("alpha1", 9080))
	if err == nil {
		_ = dg.Alter(context.Background(), &api.Operation{
			DropAll: true,
		})
	}

	_ = os.RemoveAll(rootDir)

	cmd := exec.CommandContext(context.Background(), "docker-compose", "-f", "./alpha.yml",
		"-p", testutil.DockerPrefix, "rm", "-f", "-s", "-v")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()
	if err := cmd.Run(); err != nil {
		fmt.Printf("Error while bringing down cluster. Prefix: %s. Error: %v\n", testutil.DockerPrefix, err)
	}
}

func (s *suite) testCase(query, wantResult string) func(*testing.T) {
	return func(t *testing.T) {
		//if !s.opts.skipLiveLoader {
		//	// Check results of the live loader.
		//	dg, err := testutil.DgraphClient(testutil.ContainerAddr("alpha1", 9080))
		//	if err != nil {
		//		t.Fatalf("Error while getting a dgraph client: %v", err)
		//	}
		//	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		//	defer cancel()
		//
		//	txn := dg.NewTxn()
		//	resp, err := txn.Query(ctx, query)
		//	if err != nil {
		//		t.Fatalf("Could not query: %v", err)
		//	}
		//	testutil.CompareJSON(t, wantResult, string(resp.GetJson()))
		//}

		if !s.opts.skipBulkLoader {
			// Check results of the bulk loader.
			dg, err := testutil.DgraphClient(testutil.ContainerAddr("alpha1", 9080))
			require.NoError(t, err)
			ctx2, cancel2 := context.WithTimeout(context.Background(), time.Minute)
			defer cancel2()

			txn := dg.NewTxn()
			resp, err := txn.Query(ctx2, query)
			require.NoError(t, err)
			testutil.CompareJSON(t, wantResult, string(resp.GetJson()))
		}
	}
}

func (s *suite) checkFatal(errs ...error) {
	for _, err := range errs {
		err = errors.Wrapf(err, "") // Add a stack.
		if err != nil {
			s.cleanup()
			s.t.Fatalf("%+v", err)
		}
	}
}

func init() {
	rand.Seed(int64(time.Now().Nanosecond()))
}

func freePort(port int) int {
	// Linux reuses ports in FIFO order. So a port that we listen on and then
	// release will be free for a long time.
	for {
		// p + 5080 and p + 9080 must lie within [20000, 60000]
		offset := 15000 + rand.Intn(30000)
		p := port + offset
		listener, err := net.Listen("tcp", fmt.Sprintf(":%d", p))
		if err == nil {
			listener.Close()
			return offset
		}
	}
}
