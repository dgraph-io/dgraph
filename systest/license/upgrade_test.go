//go:build upgrade

/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package main

import (
	"log"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/hypermodeinc/dgraph/v24/dgraphapi"
	"github.com/hypermodeinc/dgraph/v24/dgraphtest"
	"github.com/hypermodeinc/dgraph/v24/x"
)

type LicenseTestSuite struct {
	suite.Suite
	dc dgraphapi.Cluster
	lc *dgraphtest.LocalCluster
	uc dgraphtest.UpgradeCombo
}

func (lsuite *LicenseTestSuite) SetupTest() {
	conf := dgraphtest.NewClusterConfig().WithNumAlphas(1).WithNumZeros(1).WithReplicas(1).
		WithEncryption().WithVersion(lsuite.uc.Before)
	c, err := dgraphtest.NewLocalCluster(conf)
	x.Panic(err)
	if err := c.Start(); err != nil {
		c.Cleanup(true)
		panic(err)
	}

	lsuite.dc = c
	lsuite.lc = c
}

func (lsuite *LicenseTestSuite) TearDownTest() {
	lsuite.lc.Cleanup(lsuite.T().Failed())
}

func (lsuite *LicenseTestSuite) Upgrade() {
	t := lsuite.T()

	if err := lsuite.lc.Upgrade(lsuite.uc.After, lsuite.uc.Strategy); err != nil {
		t.Fatal(err)
	}
}

func TestLicenseTestSuite(t *testing.T) {
	for _, uc := range dgraphtest.AllUpgradeCombos(false) {
		log.Printf("running: backup in [%v], restore in [%v]", uc.Before, uc.After)
		var tsuite LicenseTestSuite
		tsuite.uc = uc
		suite.Run(t, &tsuite)
		if t.Failed() {
			t.Fatal("TestLicenseTestSuite tests failed")
		}
	}
}
