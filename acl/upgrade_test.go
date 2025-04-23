//go:build upgrade

/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package acl

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

type AclTestSuite struct {
	suite.Suite
	lc *dgraphtest.LocalCluster
	dc dgraphapi.Cluster
	uc dgraphtest.UpgradeCombo
}

func (asuite *AclTestSuite) SetupTest() {
	conf := dgraphtest.NewClusterConfig().WithNumAlphas(1).WithNumZeros(1).
		WithReplicas(1).WithACL(20 * time.Second).WithEncryption().WithVersion(asuite.uc.Before)
	c, err := dgraphtest.NewLocalCluster(conf)
	x.Panic(err)
	if err := c.Start(); err != nil {
		c.Cleanup(true)
		asuite.T().Fatal(err)
	}
	asuite.lc = c
	asuite.dc = c
}

func (asuite *AclTestSuite) TearDownTest() {
	asuite.lc.Cleanup(asuite.T().Failed())
}

func (asuite *AclTestSuite) Upgrade() {
	require.NoError(asuite.T(), asuite.lc.Upgrade(asuite.uc.After, asuite.uc.Strategy))
}

func TestACLSuite(t *testing.T) {
	for _, uc := range dgraphtest.AllUpgradeCombos(false) {
		log.Printf("running upgrade tests for config: %+v", uc)
		aclSuite := AclTestSuite{uc: uc}
		suite.Run(t, &aclSuite)
		if t.Failed() {
			panic("TestACLSuite tests failed")
		}
	}
}
