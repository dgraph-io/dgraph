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

package main

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

type LdbcTestSuite struct {
	suite.Suite
	dc          dgraphtest.Cluster
	lc          *dgraphtest.LocalCluster
	uc          dgraphtest.UpgradeCombo
	ldbcDataDir string
}

func (lsuite *LdbcTestSuite) SetupTest() {
	t := lsuite.T()
	var err error
	lsuite.ldbcDataDir, err = os.MkdirTemp(os.TempDir(), "Ldbc")
	require.NoError(t, err)
	downloadLDBCFiles(lsuite.ldbcDataDir)
}

func (lsuite *LdbcTestSuite) SetupSubTest() {
	t := lsuite.T()
	lsuite.lc.Cleanup(t.Failed())

	conf := dgraphtest.NewClusterConfig().WithNumAlphas(1).WithNumZeros(1).WithReplicas(1).
		WithVersion(lsuite.uc.Before).WithBulkLoadOutDir(t.TempDir())
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

	lsuite.dc = c
	lsuite.lc = c
}

func (lsuite *LdbcTestSuite) TearDownSubTest() {
	lsuite.lc.Cleanup(lsuite.T().Failed())
}

func (lsuite *LdbcTestSuite) TearDownTest() {
	require.NoError(lsuite.T(), os.RemoveAll(lsuite.ldbcDataDir))
}

func (lsuite *LdbcTestSuite) Upgrade() {
	require.NoError(lsuite.T(), lsuite.lc.Upgrade(lsuite.uc.After, lsuite.uc.Strategy))
}

func TestLdbcTestSuite(t *testing.T) {
	for _, uc := range dgraphtest.AllUpgradeCombos(false) {
		log.Printf("running upgrade tests for confg: %+v", uc)
		var lsuite LdbcTestSuite
		lsuite.uc = uc
		suite.Run(t, &lsuite)
		if t.Failed() {
			panic("TestLdbcTestSuite tests failed")
		}
	}
}

func (lsuite *LdbcTestSuite) bulkLoader() error {
	schemaFile := filepath.Join(lsuite.ldbcDataDir, "ldbcTypes.schema")
	return lsuite.lc.BulkLoad(dgraphtest.BulkOpts{
		DataFiles:   []string{lsuite.ldbcDataDir},
		SchemaFiles: []string{schemaFile},
	})
}

func (lsuite *LdbcTestSuite) StartAlpha() error {
	c := lsuite.lc
	if err := c.Start(); err != nil {
		return err
	}
	return c.HealthCheck(false)
}

func (lsuite *LdbcTestSuite) StopAlphasForCoverage() {
	lsuite.lc.StopAlpha(0)
}
