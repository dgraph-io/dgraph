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
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/dgraph-io/dgo/test"
	"github.com/pkg/errors"
)

var rootDir = filepath.Join(os.TempDir(), "dgraph_systest")

type suite struct {
	t *testing.T

	liveCluster *test.DgraphCluster
	bulkCluster *test.DgraphCluster
}

func newSuite(t *testing.T, schema, rdfs string) *suite {
	if testing.Short() {
		t.Skip("Skipping system test with long runtime.")
	}
	s := &suite{t: t}
	s.checkFatal(makeDirEmpty(rootDir))
	rdfFile := filepath.Join(rootDir, "rdfs.rdf")
	s.checkFatal(ioutil.WriteFile(rdfFile, []byte(rdfs), 0644))
	schemaFile := filepath.Join(rootDir, "schema.txt")
	s.checkFatal(ioutil.WriteFile(schemaFile, []byte(schema), 0644))
	s.setup(schemaFile, rdfFile)
	return s
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
	)

	s.bulkCluster = test.NewDgraphCluster(bulkDir)
	s.checkFatal(s.bulkCluster.StartZeroOnly())

	bulkCmd := exec.Command(os.ExpandEnv("$GOPATH/bin/dgraph"), "bulk",
		"-r", rdfFile,
		"-s", schemaFile,
		"--http", ":"+strconv.Itoa(test.FreePort(0)),
		"-z", ":"+s.bulkCluster.ZeroPort,
		"-j=1",
		"-x=true",
	)
	bulkCmd.Stdout = os.Stdout
	bulkCmd.Stderr = os.Stdout
	bulkCmd.Dir = bulkDir
	if err := bulkCmd.Run(); err != nil {
		s.cleanup()
		s.t.Fatalf("Bulkloader didn't run: %v\n", err)
	}
	s.bulkCluster.Zero.Process.Kill()
	s.bulkCluster.Zero.Wait()
	s.checkFatal(os.Rename(
		filepath.Join(bulkDir, "out", "0", "p"),
		filepath.Join(bulkDir, "p"),
	))

	s.liveCluster = test.NewDgraphCluster(liveDir)
	s.checkFatal(s.liveCluster.Start())
	s.checkFatal(s.bulkCluster.Start())

	liveCmd := exec.Command(os.ExpandEnv("$GOPATH/bin/dgraph"), "live",
		"--rdfs", rdfFile,
		"--schema", schemaFile,
		"--dgraph", ":"+s.liveCluster.DgraphPort,
		"--zero", ":"+s.liveCluster.ZeroPort,
	)
	liveCmd.Dir = liveDir
	liveCmd.Stdout = os.Stdout
	liveCmd.Stderr = os.Stdout
	if err := liveCmd.Run(); err != nil {
		s.cleanup()
		s.t.Fatalf("Live Loader didn't run: %v\n", err)
	}
}

func (s *suite) cleanup() {
	// NOTE: Shouldn't raise any errors here or fail a test, since this is
	// called when we detect an error (don't want to mask the original problem).
	if s.liveCluster != nil {
		s.liveCluster.Close()
	}
	if s.bulkCluster != nil {
		s.bulkCluster.Close()
	}
	_ = os.RemoveAll(rootDir)
}

func (s *suite) testCase(query, wantResult string) func(*testing.T) {
	return func(t *testing.T) {
		for _, cluster := range []*test.DgraphCluster{s.bulkCluster, s.liveCluster} {
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			txn := cluster.Client.NewTxn()
			resp, err := txn.Query(ctx, query)
			if err != nil {
				t.Fatalf("Could not query: %v", err)
			}
			CompareJSON(t, wantResult, string(resp.GetJson()))
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
