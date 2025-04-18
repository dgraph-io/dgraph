//go:build upgrade

/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package main

import (
	"errors"
	"log"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/hypermodeinc/dgraph/v25/dgraphapi"
	"github.com/hypermodeinc/dgraph/v25/dgraphtest"
	"github.com/hypermodeinc/dgraph/v25/x"
)

type SystestTestSuite struct {
	suite.Suite
	dc dgraphapi.Cluster
	lc *dgraphtest.LocalCluster
	uc dgraphtest.UpgradeCombo
}

func (ssuite *SystestTestSuite) SetupTest() {
	conf := dgraphtest.NewClusterConfig().WithNumAlphas(1).WithNumZeros(1).WithReplicas(1).
		WithACL(20 * time.Second).WithEncryption().WithVersion(ssuite.uc.Before)
	c, err := dgraphtest.NewLocalCluster(conf)
	x.Panic(err)
	if err := c.Start(); err != nil {
		c.Cleanup(true)
		panic(err)
	}

	ssuite.dc = c
	ssuite.lc = c
}

func (ssuite *SystestTestSuite) TearDownTest() {
	ssuite.lc.Cleanup(ssuite.T().Failed())
}

func (ssuite *SystestTestSuite) CheckAllowedErrorPreUpgrade(err error) bool {
	if val, checkErr := dgraphtest.IsHigherVersion(
		ssuite.dc.GetVersion(),
		"315747a19e9d5c5b98055c8b943a6e6462153bb3"); checkErr == nil && val || checkErr != nil {
		return false
	}
	return strings.Contains(err.Error(), "cannot initialize iterator when calling List.iterate: deleteBelowTs")
}

func (ssuite *SystestTestSuite) Upgrade() {
	require.NoError(ssuite.T(), ssuite.lc.Upgrade(ssuite.uc.After, ssuite.uc.Strategy))
}

func TestSystestSuite(t *testing.T) {
	for _, uc := range dgraphtest.AllUpgradeCombos(false) {
		log.Printf("running: backup in [%v], restore in [%v]", uc.Before, uc.After)
		var ssuite SystestTestSuite
		ssuite.uc = uc
		suite.Run(t, &ssuite)
		if t.Failed() {
			x.Panic(errors.New("TestSystestSuite tests failed"))
		}
	}
}
