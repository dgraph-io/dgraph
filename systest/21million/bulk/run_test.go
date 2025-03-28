//go:build integration

/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
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

	"github.com/hypermodeinc/dgraph/v24/chunker"
	"github.com/hypermodeinc/dgraph/v24/dgraphtest"
	"github.com/hypermodeinc/dgraph/v24/systest/21million/common"
	"github.com/hypermodeinc/dgraph/v24/testutil"
	"github.com/hypermodeinc/dgraph/v24/x"
)

func TestQueries(t *testing.T) {
	t.Run("Run queries", common.TestQueriesFor21Million)
}

func BenchmarkQueries(b *testing.B) {
	_, thisFile, _, _ := runtime.Caller(0)
	queryDir := filepath.Join(filepath.Dir(thisFile), "../queries")

	// For this test we DON'T want to start with an empty database.

	cacheHeatTries := 4
	meanCheckTries := 10

	c, err := dgraphtest.NewDCloudCluster()
	x.Panic(err)

	dg, cleanup, err := c.Client()
	x.Panic(err)
	defer cleanup()

	//ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	//defer cancel()
	//x.Panic(dg.Login(ctx, dgraphapi.DefaultUser, dgraphapi.DefaultPassword))

	files, err := os.ReadDir(queryDir)
	if err != nil {
		panic(fmt.Sprintf("Error reading directory: %s", err.Error()))
	}

	for _, file := range files {
		if !strings.HasPrefix(file.Name(), "query-") {
			continue
		}

		filename := filepath.Join(queryDir, file.Name())
		reader, cleanupi := chunker.FileReader(filename, nil)
		bytes, _ := io.ReadAll(reader)
		contents := string(bytes[:])
		cleanupi()

		// The test query and expected result are separated by a delimiter.
		bodies := strings.SplitN(contents, "\n---\n", 2)
		// Dgraph can get into unhealthy state sometime. So, add retry for every query.
		// If a query takes too long to run, it probably means dgraph is stuck and there's
		// no point in waiting longer or trying more tests.
		runQuery := func() uint64 {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			resp, err := dg.NewTxn().Query(ctx, bodies[0])
			if err != nil {
				panic(err)
			}
			cancel()
			return resp.Latency.TotalNs
		}
		for retry := 0; retry < cacheHeatTries; retry++ {
			runQuery()
		}
		dbTime := uint64(0)
		startTime := time.Now()
		for retry := 0; retry < meanCheckTries; retry++ {
			dbTime += runQuery()
		}

		dbDuration := time.Duration(dbTime) * time.Nanosecond
		duration := time.Since(startTime) / time.Duration(meanCheckTries)
		fmt.Println("QUERY: ", file.Name(), " took ", duration, dbDuration)
	}
}

func TestMain(m *testing.M) {
	//schemaFile := filepath.Join(testutil.TestDataDirectory, "21million.schema")
	//rdfFile := filepath.Join(testutil.TestDataDirectory, "21million.rdf.gz")
	//if err := testutil.MakeDirEmpty([]string{"out/0", "out/1", "out/2"}); err != nil {
	//	os.Exit(1)
	//}

	//if err := testutil.BulkLoad(testutil.BulkOpts{
	//	Zero:       testutil.SockAddrZero,
	//	Shards:     1,
	//	RdfFile:    rdfFile,
	//	SchemaFile: schemaFile,
	//}); err != nil {
	//	cleanupAndExit(1)
	//}

	//if err := testutil.StartAlphas("./alpha.yml"); err != nil {
	//	cleanupAndExit(1)
	//}
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
