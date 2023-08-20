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
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/dgraph-io/dgraph/dgraphtest"
	"github.com/dgraph-io/dgraph/x"
)

type LiveTestSuite struct {
	suite.Suite
	dc          dgraphtest.Cluster
	lc          *dgraphtest.LocalCluster
	uc          dgraphtest.UpgradeCombo
	liveDataDir string
}

func (lsuite *LiveTestSuite) SetupTest() {
	t := lsuite.T()
	var err error
	lsuite.liveDataDir, err = os.MkdirTemp(os.TempDir(), "21millionLive")
	require.NoError(t, err)
	require.NoError(t, downloadDataFiles(lsuite.liveDataDir))
}

func (lsuite *LiveTestSuite) SetupSubTest() {
	conf := dgraphtest.NewClusterConfig().WithNumAlphas(1).WithNumZeros(1).WithReplicas(1).
		WithVersion(lsuite.uc.Before)
	c, err := dgraphtest.NewLocalCluster(conf)
	x.Panic(err)
	if err := c.Start(); err != nil {
		c.Cleanup(true)
		panic(err)
	}

	lsuite.dc = c
	lsuite.lc = c
}

func (lsuite *LiveTestSuite) TearDownSubTest() {
	lsuite.lc.Cleanup(lsuite.T().Failed())
}

func (lsuite *LiveTestSuite) TearDownTest() {
	require.NoError(lsuite.T(), os.RemoveAll(lsuite.liveDataDir))
}

func (lsuite *LiveTestSuite) Upgrade() {
	require.NoError(lsuite.T(), lsuite.lc.Upgrade(lsuite.uc.After, lsuite.uc.Strategy))
}

func TestLiveTestSuite(t *testing.T) {
	for _, uc := range dgraphtest.AllUpgradeCombos(false) {
		log.Printf("running upgrade tests for confg: %+v", uc)
		var lsuite LiveTestSuite
		lsuite.uc = uc
		suite.Run(t, &lsuite)
		if t.Failed() {
			panic("TestLiveTestSuite tests failed")
		}
	}
}

func (lsuite *LiveTestSuite) liveLoader() error {
	dataDir := lsuite.liveDataDir
	rdfFile := filepath.Join(dataDir, "21million.rdf.gz")
	schemaFile := filepath.Join(dataDir, "21million.schema")
	return lsuite.lc.LiveLoad(dgraphtest.LiveOpts{
		DataFiles:   []string{rdfFile},
		SchemaFiles: []string{schemaFile},
	})
}
