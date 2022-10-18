package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/dgraph-io/dgo/v210/protos/api"
	"github.com/dgraph-io/dgraph/testutil"
)

func TestMain(m *testing.M) {
	noschemaFile := filepath.Join(testutil.TestDataDirectory, "ldbcTypes.schema")
	rdfFile := filepath.Join(testutil.TestDataDirectory, "ldbcData")
	if err := testutil.MakeDirEmpty([]string{"out/0", "out/1", "out/2"}); err != nil {
		os.Exit(1)
	}

	if err := testutil.BulkLoad(testutil.BulkOpts{
		Zero:       testutil.SockAddrZero,
		Shards:     1,
		RdfFile:    rdfFile,
		SchemaFile: noschemaFile,
	}); err != nil {
		cleanupAndExit(1)
	}

	if err := testutil.StartAlphas("./alpha.yml"); err != nil {
		fmt.Printf("Error while bringin up alphas. Error: %v\n", err)
		cleanupAndExit(1)
	}
	schemaFile := filepath.Join(testutil.TestDataDirectory, "1million.schema")
	client, err := testutil.DgraphClient(testutil.ContainerAddr("alpha1", 9080))
	if err != nil {
		fmt.Printf("Error while creating client. Error: %v\n", err)
		cleanupAndExit(1)
	}

	file, err := ioutil.ReadFile(schemaFile)
	if err != nil {
		fmt.Printf("Error while reading schema file. Error: %v\n", err)
		cleanupAndExit(1)
	}

	if err = client.Alter(context.Background(), &api.Operation{
		Schema: string(file),
	}); err != nil {
		fmt.Printf("Error while indexing. Error: %v\n", err)
		cleanupAndExit(1)
	}

	exitCode := m.Run()
	cleanupAndExit(exitCode)
}

func cleanupAndExit(exitCode int) {
	if testutil.StopAlphasAndDetectRace("./alpha.yml") {
		// if there is race fail the test
		exitCode = 1
	}
	_ = os.RemoveAll("out")
	os.Exit(exitCode)
}
