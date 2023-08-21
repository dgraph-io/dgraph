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

package main

import (
	"log"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/dgraph-io/dgraph/dgraphtest"
	"github.com/dgraph-io/dgraph/testutil"
)

type LdbcTestSuite struct {
	suite.Suite
	dc dgraphtest.Cluster
}

func (lsuite *LdbcTestSuite) SetupTest() {
	lsuite.dc = dgraphtest.NewComposeCluster()
}

func (lsuite *LdbcTestSuite) TearDownTest() {
	testutil.DetectRaceInAlphas(testutil.DockerPrefix)
}

func (lsuite *LdbcTestSuite) Upgrade() {
	// Not implemented for integration tests
}

func TestLdbcTestSuite(t *testing.T) {
	suite.Run(t, new(LdbcTestSuite))
}

func (lsuite *LdbcTestSuite) bulkLoader() error {
	dataDir := testutil.TestDataDirectory
	noschemaFile := filepath.Join(dataDir, "ldbcTypes.schema")
	require.NoError(lsuite.T(), testutil.MakeDirEmpty([]string{"./out/0/p"}))
	start := time.Now()
	err := testutil.BulkLoad(testutil.BulkOpts{
		Zero:       testutil.SockAddrZero,
		Shards:     1,
		RdfFile:    dataDir,
		SchemaFile: noschemaFile,
	})
	log.Printf("took %s to bulkupload LDBC dataset", time.Since(start))
	return err
}

func (lsuite *LdbcTestSuite) StartAlpha() error {
	return testutil.StartAlphas("./alpha.yml")
}

func (lsuite *LdbcTestSuite) StopAlphasForCoverage() {
	testutil.StopAlphasForCoverage("./alpha.yml")
}
