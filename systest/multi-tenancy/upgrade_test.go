//go:build upgrade

/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package main

import (
	"log"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/hypermodeinc/dgraph/v25/dgraphapi"
	"github.com/hypermodeinc/dgraph/v25/dgraphtest"
	"github.com/hypermodeinc/dgraph/v25/x"
)

type MultitenancyTestSuite struct {
	suite.Suite
	dc dgraphapi.Cluster
	lc *dgraphtest.LocalCluster
	uc dgraphtest.UpgradeCombo
}

func (msuite *MultitenancyTestSuite) SetupTest() {
	conf := dgraphtest.NewClusterConfig().WithNumAlphas(1).WithNumZeros(1).WithReplicas(1).
		WithACL(20 * time.Second).WithEncryption().WithVersion(msuite.uc.Before)
	c, err := dgraphtest.NewLocalCluster(conf)
	x.Panic(err)
	if err := c.Start(); err != nil {
		c.Cleanup(true)
		panic(err)
	}

	msuite.dc = c
	msuite.lc = c
}

func (msuite *MultitenancyTestSuite) TearDownTest() {
	msuite.lc.Cleanup(msuite.T().Failed())
}

func (msuite *MultitenancyTestSuite) Upgrade() {
	require.NoError(msuite.T(), msuite.lc.Upgrade(msuite.uc.After, msuite.uc.Strategy))
}

func TestMultitenancySuite(t *testing.T) {
	for _, uc := range dgraphtest.AllUpgradeCombos(false) {
		log.Printf("running upgrade tests for config: %+v", uc)
		var msuite MultitenancyTestSuite
		msuite.uc = uc
		suite.Run(t, &msuite)
		if t.Failed() {
			panic("TestMultitenancySuite tests failed")
		}
	}
}
