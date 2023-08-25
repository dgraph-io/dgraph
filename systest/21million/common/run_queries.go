/*
 * Copyright 2023 Dgraph Labs, Inc. and Contributors
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

package common

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/chunker"
	"github.com/dgraph-io/dgraph/dgraphtest"
)

var datafiles = map[string]string{
	"1million-noindex.schema": "https://github.com/dgraph-io/benchmarks/blob/master/data/1million-noindex.schema?raw=true",
	"1million.schema":         "https://github.com/dgraph-io/benchmarks/blob/master/data/1million.schema?raw=true",
	"1million.rdf.gz":         "https://github.com/dgraph-io/benchmarks/blob/master/data/1million.rdf.gz?raw=true",
	"21million.schema":        "https://github.com/dgraph-io/benchmarks/blob/master/data/21million.schema?raw=true",
	"21million.rdf.gz":        "https://github.com/dgraph-io/benchmarks/blob/master/data/21million.rdf.gz?raw=true",
}

func QueriesFor21Million(t *testing.T, dc dgraphtest.Cluster) error {
	// For this test we DON'T want to start with an empty database.
	dg, cleanup, err := dc.Client()
	defer cleanup()
	if err != nil {
		return errors.Wrapf(err, "error creating grpc client")
	}

	_, thisFile, _, _ := runtime.Caller(0)
	queryDir := filepath.Join(filepath.Dir(thisFile), "../queries")

	files, err := os.ReadDir(queryDir)
	if err != nil {
		return errors.Wrapf(err, "error reading directory")
	}

	for _, file := range files {
		if !strings.HasPrefix(file.Name(), "query-") {
			continue
		}

		t.Run(file.Name(), func(t *testing.T) {
			filename := filepath.Join(queryDir, file.Name())
			reader, cleanup := chunker.FileReader(filename, nil)
			bytes, err := io.ReadAll(reader)
			if err != nil {
				t.Fatalf("Error reading file: %s", err.Error())
			}
			contents := string(bytes[:])
			cleanup()

			// The test query and expected result are separated by a delimiter.
			bodies := strings.SplitN(contents, "\n---\n", 2)
			// Dgraph can get into unhealthy state sometime. So, add retry for every query.
			for retry := 0; retry < 3; retry++ {
				// If a query takes too long to run, it probably means dgraph is stuck and there's
				// no point in waiting longer or trying more tests.
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
				resp, err := dg.NewTxn().Query(ctx, bodies[0])
				cancel()

				if retry < 2 && (err != nil || ctx.Err() == context.DeadlineExceeded) {
					continue
				}

				if ctx.Err() == context.DeadlineExceeded {
					t.Fatal("aborting test due to query timeout")
				}

				t.Logf("running %s", file.Name())

				require.NoError(t, dgraphtest.CompareJSON(bodies[1], string(resp.GetJson())))
			}
		})
	}
	return nil
}

func DownloadDataFiles(testDir string) error {
	for fname, link := range datafiles {
		cmd := exec.Command("wget", "-O", fname, link)
		cmd.Dir = testDir

		if out, err := cmd.CombinedOutput(); err != nil {
			return errors.Wrapf(err, fmt.Sprintf("error downloading a file: %s", string(out)))
		}
	}
	return nil
}
