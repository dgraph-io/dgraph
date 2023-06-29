//go:build upgrade

/*
 * Copyright 2023 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Dgraph Community License (the "License"); you
 * may not use this file except in compliance with the License. You
 * may obtain a copy of the License at
 *
 *     https://github.com/dgraph-io/dgraph/blob/main/licenses/DCL.txt
 */

package acl

import (
	"log"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/dgraph-io/dgraph/dgraphtest"
	"github.com/dgraph-io/dgraph/x"
)

type AclTestSuite struct {
	suite.Suite
	lc *dgraphtest.LocalCluster
	dc dgraphtest.Cluster
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
	if err := asuite.lc.Upgrade(asuite.uc.After, asuite.uc.Strategy); err != nil {
		asuite.lc.Cleanup(true)
		asuite.T().Fatal(err)
	}
}

func TestACLSuite(t *testing.T) {
	for _, uc := range dgraphtest.AllUpgradeCombos {
		log.Printf("running upgrade tests for confg: %+v", uc)
		aclSuite := AclTestSuite{uc: uc}
		suite.Run(t, &aclSuite)
		if t.Failed() {
			panic("TestACLSuite tests failed")
		}
	}
}
