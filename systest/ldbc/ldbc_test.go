//go:build integration

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"

	"github.com/dgraph-io/dgraph/dgraphtest"
	"github.com/dgraph-io/dgraph/testutil"
)

type TestCases struct {
	Tag   string `yaml:"tag"`
	Query string `yaml:"query"`
	Resp  string `yaml:"resp"`
}

var (
	COVERAGE_FLAG         = "COVERAGE_OUTPUT"
	EXPECTED_COVERAGE_ENV = "--test.coverprofile=coverage.out"
)

func TestQueries(t *testing.T) {
	


func TestMain(m *testing.M) {
	if os.Getenv("LDBC_QUERY_ONLY") == "false" {
		noschemaFile := filepath.Join(dgraphtest.TestDataDirectory, "ldbcTypes.schema")
		rdfFile := dgraphtest.TestDataDirectory
		if err := dgraphtest.MakeDirEmpty([]string{"out/0"}); err != nil {
			os.Exit(1)
		}

		start := time.Now()
		fmt.Println("Bulkupload started")
		if err := dgraphtest.BulkLoad(dgraphtest.BulkOpts{
			Zero:       testutil.SockAddrZero,
			Shards:     1,
			RdfFile:    rdfFile,
			SchemaFile: noschemaFile,
		}); err != nil {
			fmt.Println(err)
			
		}

		fmt.Printf("Took %s to bulkupload LDBC dataset\n", time.Since(start))

	
	}
	if os.Getenv("LDBC_BULK_LOAD_ONLY") == "false" {
		if err := testutil.StartAlphas("./alpha.yml"); err != nil {
			fmt.Printf("Error while bringin up alphas. Error: %v\n", err)
		}
		exitCode := m.Run()
		
	
}




