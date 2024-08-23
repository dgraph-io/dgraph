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
	"errors"
	"log"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/dgraph-io/dgraph/v24/dgraphapi"
	"github.com/dgraph-io/dgraph/v24/dgraphtest"
	"github.com/dgraph-io/dgraph/v24/x"
)

type PluginTestSuite struct {
	suite.Suite
	dc dgraphapi.Cluster
	lc *dgraphtest.LocalCluster
	uc dgraphtest.UpgradeCombo
}

func (psuite *PluginTestSuite) SetupSubTest() {
	// The TestPlugins() invokes subtest function, hence using
	// SetupSubTest() instead of SetupTest().
	conf := dgraphtest.NewClusterConfig().WithNumAlphas(1).WithNumZeros(1).WithReplicas(1).
		WithVersion(psuite.uc.Before).WithCustomPlugins()
	c, err := dgraphtest.NewLocalCluster(conf)
	x.Panic(err)
	if err := c.Start(); err != nil {
		c.Cleanup(true)
		panic(err)
	}

	psuite.dc = c
	psuite.lc = c
}

func (psuite *PluginTestSuite) TearDownTest() {
	psuite.lc.Cleanup(psuite.T().Failed())
}

func (psuite *PluginTestSuite) Upgrade() {
	require.NoError(psuite.T(), psuite.lc.Upgrade(psuite.uc.After, psuite.uc.Strategy))
}

func TestPluginTestSuite(t *testing.T) {
	for _, uc := range dgraphtest.AllUpgradeCombos(false) {
		log.Printf("running: backup in [%v], restore in [%v]", uc.Before, uc.After)
		var psuite PluginTestSuite
		psuite.uc = uc
		suite.Run(t, &psuite)
		if t.Failed() {
			x.Panic(errors.New("TestPluginTestSuite tests failed"))
		}
	}
}
