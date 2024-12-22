//go:build integration

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

package bulk

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/dgraph-io/dgraph/v24/chunker"
	"github.com/dgraph-io/dgraph/v24/systest/21million/common"
	"github.com/dgraph-io/dgraph/v24/testutil"
)

func TestQueries(t *testing.T) {
	t.Run("Run queries", common.TestQueriesFor21Million)
}

func BenchmarkQueries(b *testing.B) {

	_, thisFile, _, _ := runtime.Caller(0)
	queryDir := filepath.Join(filepath.Dir(thisFile), "../queries")

	// For this test we DON'T want to start with an empty database.
	dg, err := testutil.DgraphClient(testutil.ContainerAddr("alpha1", 9080))
	if err != nil {
		panic(fmt.Sprintf("Error while getting a dgraph client: %v", err))
	}

	files, err := os.ReadDir(queryDir)
	if err != nil {
		panic(fmt.Sprintf("Error reading directory: %s", err.Error()))
	}

	//savepath := ""
	//diffs := 0
	for _, file := range files {
		if !strings.HasPrefix(file.Name(), "query-") {
			continue
		}
		b.Run(file.Name(), func(b *testing.B) {
			filename := filepath.Join(queryDir, file.Name())
			reader, cleanup := chunker.FileReader(filename, nil)
			bytes, _ := io.ReadAll(reader)
			contents := string(bytes[:])
			cleanup()

			// The test query and expected result are separated by a delimiter.
			bodies := strings.SplitN(contents, "\n---\n", 2)
			// Dgraph can get into unhealthy state sometime. So, add retry for every query.
			// If a query takes too long to run, it probably means dgraph is stuck and there's
			// no point in waiting longer or trying more tests.
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			_, err := dg.NewTxn().Query(ctx, bodies[0])
			if err != nil {
				panic(err)
			}
			cancel()
		})
	}
}

func TestMain(m *testing.M) {
	exitCode := m.Run()
	cleanupAndExit(exitCode)
}

func cleanupAndExit(exitCode int) {
	if testutil.StopAlphasAndDetectRace([]string{"alpha1"}) {
		// if there is race fail the test
		exitCode = 1
	}
	_ = os.RemoveAll("out")
	os.Exit(exitCode)
}
