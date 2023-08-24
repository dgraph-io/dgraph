//go:build upgrade

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
	"log"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/dgraph-io/dgraph/dgraphtest"
	"github.com/dgraph-io/dgraph/x"
)

type BulkTestSuite struct {
	suite.Suite
	dc          dgraphtest.Cluster
	lc          *dgraphtest.LocalCluster
	uc          dgraphtest.UpgradeCombo
	bulkDataDir string
}

func (bsuite *BulkTestSuite) SetupTest() {
	t := bsuite.T()
	bsuite.bulkDataDir = t.TempDir()
	require.NoError(t, downloadDataFiles(bsuite.bulkDataDir))

	conf := dgraphtest.NewClusterConfig().WithNumAlphas(1).WithNumZeros(1).WithReplicas(1).
		WithVersion(bsuite.uc.Before).WithBulkLoadOutDir(t.TempDir()).
		WithDetectRace()
	c, err := dgraphtest.NewLocalCluster(conf)
	x.Panic(err)

	// start zero
	if err := c.StartZero(0); err != nil {
		c.Cleanup(true)
		panic(err)
	}

	if err := c.HealthCheck(true); err != nil {
		c.Cleanup(true)
		panic(err)
	}

	bsuite.dc = c
	bsuite.lc = c
}

func (bsuite *BulkTestSuite) TearDownTest() {
	bsuite.lc.Cleanup(bsuite.T().Failed())
}

func (bsuite *BulkTestSuite) Upgrade() {
	require.NoError(bsuite.T(), bsuite.lc.Upgrade(bsuite.uc.After, bsuite.uc.Strategy))
}

func TestBulkTestSuite(t *testing.T) {
	for _, uc := range dgraphtest.AllUpgradeCombos(false) {
		log.Printf("running upgrade tests for confg: %+v", uc)
		var bsuite BulkTestSuite
		bsuite.uc = uc
		suite.Run(t, &bsuite)
		if t.Failed() {
			panic("TestBulkTestSuite tests failed")
		}
	}
}

func (bsuite *BulkTestSuite) bulkLoader() error {
	dataDir := bsuite.bulkDataDir
	rdfFile := filepath.Join(dataDir, "21million.rdf.gz")
	schemaFile := filepath.Join(dataDir, "21million.schema")
	return bsuite.lc.BulkLoad(dgraphtest.BulkOpts{
		DataFiles:   []string{rdfFile},
		SchemaFiles: []string{schemaFile},
	})
}

func (lsuite *BulkTestSuite) StartAlpha() error {
	return lsuite.lc.Start()
}
