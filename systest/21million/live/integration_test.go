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
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/suite"
	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/dgraphtest"
	"github.com/dgraph-io/dgraph/testutil"
)

type LiveTestSuite struct {
	suite.Suite
	dc          dgraphtest.Cluster
	liveDataDir string
}

func (lsuite *LiveTestSuite) SetupTest() {
	lsuite.dc = dgraphtest.NewComposeCluster()
	t := lsuite.T()
	var err error
	lsuite.liveDataDir, err = os.MkdirTemp(os.TempDir(), "21millionLive")
	require.NoError(t, err)
	require.NoError(t, downloadDataFiles(lsuite.liveDataDir))
}

func (lsuite *LiveTestSuite) TearDownTest() {
	require.NoError(lsuite.T(), os.RemoveAll(lsuite.liveDataDir))
}

func (lsuite *LiveTestSuite) Upgrade() {
	// Not implemented for integration tests
}

func TestLiveTestSuite(t *testing.T) {
	suite.Run(t, new(LiveTestSuite))
}

func (lsuite *LiveTestSuite) liveLoader() error {
	liveDataDir := lsuite.liveDataDir
	rdfFile := filepath.Join(liveDataDir, "21million.rdf.gz")
	schemaFile := filepath.Join(liveDataDir, "21million.schema")
	return testutil.LiveLoad(testutil.LiveOpts{
		Alpha:      testutil.ContainerAddr("alpha1", 9080),
		Zero:       testutil.SockAddrZero,
		RdfFile:    rdfFile,
		SchemaFile: schemaFile,
	})
}
