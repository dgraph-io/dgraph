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
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/dgraph-io/dgraph/dgraphtest"
	"github.com/dgraph-io/dgraph/testutil"
)

type BulkTestSuite struct {
	suite.Suite
	dc          dgraphtest.Cluster
}

func (bsuite *BulkTestSuite) SetupTest() {
	bsuite.dc = dgraphtest.NewComposeCluster()
}

func (bsuite *BulkTestSuite) TearDownTest() {
	testutil.DetectRaceInAlphas(testutil.DockerPrefix)
}

func (bsuite *BulkTestSuite) Upgrade() {
	// Not implemented for integration tests
}

func TestBulkTestSuite(t *testing.T) {
	suite.Run(t, new(BulkTestSuite))
}

func (bsuite *BulkTestSuite) bulkLoader() error {
	bulkDataDir := testutil.TestDataDirectory
	rdfFile := filepath.Join(bulkDataDir, "21million.rdf.gz")
	schemaFile := filepath.Join(bulkDataDir, "21million.schema")
	require.NoError(bsuite.T(), testutil.MakeDirEmpty([]string{"out/0", "out/1", "out/2"}))
	return testutil.BulkLoad(testutil.BulkOpts{
		Zero:       testutil.SockAddrZero,
		Shards:     1,
		RdfFile:    rdfFile,
		SchemaFile: schemaFile,
	})
}

func (bsuite *BulkTestSuite) StartAlpha() error {
	return testutil.StartAlphas("./alpha.yml")
}
