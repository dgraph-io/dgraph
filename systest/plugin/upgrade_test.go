//go:build upgrade

/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package main

import (
	"errors"
	"log"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/hypermodeinc/dgraph/v25/dgraphapi"
	"github.com/hypermodeinc/dgraph/v25/dgraphtest"
	"github.com/hypermodeinc/dgraph/v25/x"
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
