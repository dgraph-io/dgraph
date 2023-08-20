//go:build integration || upgrade

package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"

	"github.com/dgraph-io/dgraph/dgraphtest"
)

var baseUrl = "https://github.com/dgraph-io/benchmarks/blob/master/ldbc/sf0.3/ldbc_rdf_0.3/"
var suffix = "?raw=true"

var ldbcDataFiles = map[string]string{
	"ldbcTypes.schema": "https://github.com/dgraph-io/benchmarks/blob/master/ldbc/sf0.3/ldbcTypes.schema?raw=true",
}

var rdfFileNames = [...]string{
	"Deltas.rdf",
	"comment_0.rdf",
	"containerOf_0.rdf",
	"forum_0.rdf",
	"hasCreator_0.rdf",
	"hasInterest_0.rdf",
	"hasMember_0.rdf",
	"hasModerator_0.rdf",
	"hasTag_0.rdf",
	"hasType_0.rdf",
	"isLocatedIn_0.rdf",
	"isPartOf_0.rdf",
	"isSubclassOf_0.rdf",
	"knows_0.rdf",
	"likes_0.rdf",
	"organisation_0.rdf",
	"person_0.rdf",
	"place_0.rdf",
	"post_0.rdf",
	"replyOf_0.rdf",
	"studyAt_0.rdf",
	"tag_0.rdf",
	"tagclass_0.rdf",
	"workAt_0.rdf"}

type TestCases struct {
	Tag   string `yaml:"tag"`
	Query string `yaml:"query"`
	Resp  string `yaml:"resp"`
}

var (
	COVERAGE_FLAG         = "COVERAGE_OUTPUT"
	EXPECTED_COVERAGE_ENV = "--test.coverprofile=coverage.out"
)

func (lsuite *LdbcTestSuite) TestQueries() {
	t := lsuite.T()

	yfile, _ := os.ReadFile("test_cases.yaml")

	tc := make(map[string]TestCases)
	if err := yaml.Unmarshal(yfile, &tc); err != nil {
		t.Fatalf("Error while greading test cases yaml: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	for _, tt := range tc {
		desc := tt.Tag
		if cc := os.Getenv(COVERAGE_FLAG); cc == EXPECTED_COVERAGE_ENV {
			// LDBC test (IC05) times out for test-binaries (code coverage enabled)
			if desc == "IC05" {
				continue
			}
		}
		// TODO(anurag): IC06 and IC10 have non-deterministic results because of dataset.
		// Find a way to modify the queries to include them in the tests
		if desc == "IC06" || desc == "IC10" {
			continue
		}
		lsuite.Run(desc, func() {
			t := lsuite.T()
			require.NoError(t, lsuite.bulkLoader())

			require.NoError(t, lsuite.StartAlpha())

			// Upgrade
			lsuite.Upgrade()

			dg, cleanup, err := lsuite.dc.Client()
			defer cleanup()
			require.NoError(t, err)

			resp, err := dg.Query(tt.Query)
			require.NoError(t, err)
			dgraphtest.CompareJSON(tt.Resp, string(resp.Json))
		})
		if ctx.Err() == context.DeadlineExceeded {
			t.Fatal("aborting test due to query timeout")
		}
	}
	cancel()

	if cc := os.Getenv(COVERAGE_FLAG); cc == EXPECTED_COVERAGE_ENV {
		lsuite.StopAlphasForCoverage()
	}
}

func downloadLDBCFiles(ldbcDataDir string) {
	for _, name := range rdfFileNames {
		filepath := baseUrl + name + suffix
		ldbcDataFiles[name] = filepath
	}

	start := time.Now()
	var wg sync.WaitGroup
	for fname, link := range ldbcDataFiles {
		wg.Add(1)
		go func(fname, link string, wg *sync.WaitGroup) {
			defer wg.Done()
			start := time.Now()
			cmd := exec.Command("wget", "-O", fname, link)
			cmd.Dir = ldbcDataDir
			if out, err := cmd.CombinedOutput(); err != nil {
				panic(fmt.Sprintf("error downloading a file: %s", string(out)))
			}
			log.Printf("Downloaded %s to %s in %s \n", fname, ldbcDataDir, time.Since(start))
		}(fname, link, &wg)
	}
	wg.Wait()
	log.Printf("Downloaded %d files in %s \n", len(ldbcDataFiles), time.Since(start))
}
