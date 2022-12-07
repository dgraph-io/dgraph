package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dgraph-io/dgraph/testutil"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

type TestCases struct {
	Tag   string `yaml:"tag"`
	Query string `yaml:"query"`
	Resp  string `yaml:"resp"`
}

func TestQueries(t *testing.T) {
	// dg, err := testutil.DgraphClient(testutil.ContainerAddr("alpha1", 9080))
	dg, err := testutil.DgraphClient("127.0.0.1:9080")

	if err != nil {
		t.Fatalf("Error while getting a dgraph client: %v", err)
	}

	yfile, _ := ioutil.ReadFile("test_cases.yaml")

	tc := make(map[string]TestCases)

	err = yaml.Unmarshal(yfile, &tc)

	if err != nil {
		t.Fatalf("Error while greading test cases yaml: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	for _, tt := range tc {
		desc := tt.Tag
		// TODO(anurag): IC06 and IC10 have non-deterministic results because of dataset.
		// Find a way to modify the queries to include them in the tests
		if desc == "IC06" || desc == "IC10" || desc != "IC12" {
			continue
		}
		t.Run(desc, func(t *testing.T) {
			resp, err := dg.NewTxn().Query(ctx, tt.Query)
			require.NoError(t, err)
			testutil.CompareJSON(t, tt.Resp, string(resp.Json))
		})
		if ctx.Err() == context.DeadlineExceeded {
			t.Fatal("aborting test due to query timeout")
		}
	}
	cancel()
}

func TestMain(m *testing.M) {
	noschemaFile := filepath.Join(testutil.TestDataDirectory, "ldbcTypes.schema")
	rdfFile := testutil.TestDataDirectory
	if err := testutil.MakeDirEmpty([]string{"out/0"}); err != nil {
		os.Exit(1)
	}

	start := time.Now()
	fmt.Println("Bulkupload started")
	if err := testutil.BulkLoad(testutil.BulkOpts{
		Zero:       testutil.SockAddrZero,
		Shards:     1,
		RdfFile:    rdfFile,
		SchemaFile: noschemaFile,
	}); err != nil {
		fmt.Println(err)
		cleanupAndExit(1)
	}

	fmt.Printf("Took %s to bulkupload LDBC dataset\n", time.Since(start))

	if err := testutil.StartAlphas("./alpha.yml"); err != nil {
		fmt.Printf("Error while bringin up alphas. Error: %v\n", err)
		cleanupAndExit(1)
	}

	exitCode := m.Run()
	// cleanupAndExit(exitCode)
	os.Exit(exitCode)
}

func cleanupAndExit(exitCode int) {
	if testutil.StopAlphasAndDetectRace("./alpha.yml") {
		// if there is race fail the test
		exitCode = 1
	}
	_ = os.RemoveAll("out")
	os.Exit(exitCode)
}
