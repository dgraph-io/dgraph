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
	"io/ioutil"
	"log"
	"math/rand"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/dgraph-io/dgo/v2/protos/api"
	"github.com/dgraph-io/dgraph/testutil"
	"github.com/pkg/errors"
)

func init() {
	cmd := exec.Command("go", "install", "github.com/dgraph-io/dgraph/dgraph")
	cmd.Env = os.Environ()
	if out, err := cmd.CombinedOutput(); err != nil {
		log.Fatalf("Could not run %q: %s", cmd.Args, string(out))
	}
}

var rootDir = filepath.Join(os.TempDir(), "dgraph_systest")

type suite struct {
	t           *testing.T
	bulkCluster *DgraphCluster
	opts        suiteOpts
}

type suiteOpts struct {
	schema         string
	rdfs           string
	skipBulkLoader bool
	skipLiveLoader bool
}

func newSuiteInternal(t *testing.T, opts suiteOpts) *suite {
	dg := testutil.DgraphClientWithGroot(testutil.SockAddr)
	err := dg.Alter(context.Background(), &api.Operation{
		DropAll: true,
	})
	if err != nil {
		t.Fatalf("Could not drop old data: %v", err)
	}

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
	s.setup(schemaFile, rdfFile)
	return s
}

func newSuite(t *testing.T, schema, rdfs string) *suite {
	opts := suiteOpts{
		schema: schema,
		rdfs:   rdfs,
	}
	return newSuiteInternal(t, opts)
}

func newBulkOnlySuite(t *testing.T, schema, rdfs string) *suite {
	opts := suiteOpts{
		schema:         schema,
		rdfs:           rdfs,
		skipLiveLoader: true,
	}
	return newSuiteInternal(t, opts)
}

func newSuiteFromFile(t *testing.T, schemaFile, rdfFile string) *suite {
	if testing.Short() {
		t.Skip("Skipping system test with long runtime.")
	}
	s := &suite{t: t}

	s.setup(schemaFile, rdfFile)
	return s
}

func (s *suite) setup(schemaFile, rdfFile string) {
	var (
		bulkDir = filepath.Join(rootDir, "bulk")
		liveDir = filepath.Join(rootDir, "live")
	)
	s.checkFatal(
		makeDirEmpty(bulkDir),
		makeDirEmpty(liveDir),
		makeDirEmpty(filepath.Join(bulkDir, "out", "0")),
	)

	if !s.opts.skipBulkLoader {
		s.bulkCluster = NewDgraphCluster(filepath.Join(bulkDir, "out", "0"))
		if err := s.bulkCluster.StartZeroOnly(); err != nil {
			s.cleanup()
			s.t.Fatalf("Couldn't start zero in Dgraph cluster: %v\n", err)
		}

		bulkCmd := exec.Command(os.ExpandEnv("$GOPATH/bin/dgraph"), "bulk",
			"-f", rdfFile,
			"-s", schemaFile,
			"--http", "localhost:"+strconv.Itoa(freePort(0)),
			"-j=1",
			"-x=true",
			"-z", "localhost:"+s.bulkCluster.zeroPort,
		)
		bulkCmd.Dir = bulkDir
		if out, err := bulkCmd.Output(); err != nil {
			s.cleanup()
			s.t.Logf("%s", out)
			s.t.Fatalf("Bulkloader didn't run: %v\n", err)
		}

		if err := s.bulkCluster.StartAlphaOnly(); err != nil {
			s.cleanup()
			s.t.Fatalf("Couldn't start alpha in Dgraph cluster: %v\n", err)
		}
	}

	if !s.opts.skipLiveLoader {
		liveCmd := exec.Command(os.ExpandEnv("$GOPATH/bin/dgraph"), "live",
			"--files", rdfFile,
			"--schema", schemaFile,
			"--alpha", testutil.SockAddr,
			"--zero", testutil.SockAddrZero,
		)
		liveCmd.Dir = liveDir
		if out, err := liveCmd.Output(); err != nil {
			s.cleanup()
			s.t.Logf("%s", out)
			s.t.Fatalf("Live Loader didn't run: %v\n", err)
		}
	}
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
	_ = os.RemoveAll(rootDir)
	s.bulkCluster.Close()
}

func (s *suite) testCase(query, wantResult string) func(*testing.T) {
	return func(t *testing.T) {
		if !s.opts.skipLiveLoader {
			// Check results of the live loader.
			dg := testutil.DgraphClientWithGroot(testutil.SockAddr)
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()

			txn := dg.NewTxn()
			resp, err := txn.Query(ctx, query)
			if err != nil {
				t.Fatalf("Could not query: %v", err)
			}
			testutil.CompareJSON(t, wantResult, string(resp.GetJson()))
		}

		if !s.opts.skipBulkLoader {
			// Check results of the bulk loader.
			dg := testutil.DgraphClient("localhost:" + s.bulkCluster.alphaPort)
			ctx2, cancel2 := context.WithTimeout(context.Background(), time.Minute)
			defer cancel2()

			txn := dg.NewTxn()
			resp, err := txn.Query(ctx2, query)
			if err != nil {
				t.Fatalf("Could not query: %v", err)
			}
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

func check(t *testing.T, err error) {
	err = errors.Wrapf(err, "") // Add a stack.
	if err != nil {
		t.Fatalf("%+v", err)
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
