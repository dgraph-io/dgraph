//go:build integration || upgrade

package main

import (
	"context"
	"os"
	"time"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"

	"github.com/dgraph-io/dgraph/dgraphtest"
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

			resp, err := dg.NewTxn().Query(ctx, tt.Query)
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
