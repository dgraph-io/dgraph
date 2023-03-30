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
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/dgraph-io/dgraph/dgraphtest"
	"github.com/dgraph-io/dgraph/x"
)

type AclTestSuite struct {
	suite.Suite
	c *dgraphtest.LocalCluster
}

func (suite *AclTestSuite) SetupTest() {
	conf := dgraphtest.NewClusterConfig().WithNumAlphas(3).WithNumZeros(3).
		WithACL(20 * time.Second).WithEncryption().WithVersion("0c9f60156")
	c, err := dgraphtest.NewLocalCluster(conf)
	x.Panic(err)
	c.Start()
	suite.c = c
	dc = c
	grpcPort, err := c.GetAlphaPublicPort("9080")
	if err != nil {
		panic(err)
	}
	sockAddrHttp, err := c.GetAlphaPublicPort("8080")
	if err != nil {
		panic(err)
	}
	sockAddr = "localhost:" + grpcPort
	adminEndpoint = "http://localhost:" + sockAddrHttp + "/admin"

}

func (suite *AclTestSuite) TearDownTest() {
	// cleanup function here
	suite.T().Cleanup(suite.c.Cleanup)
}

func (suite *AclTestSuite) Upgrade() {
	x.Panic(suite.c.Upgrade("v23.0.0-beta1", dgraphtest.StopStart))
	// need to set admin endpoind after every upgrade
	sockAddrHttp, err := suite.c.GetAlphaPublicPort("8080")
	if err != nil {
		panic(err)
	}
	port, err := suite.c.GetAlphaPublicPort("9080")
	if err != nil {
		panic(err)
	}
	sockAddr = "localhost:" + port
	adminEndpoint = "http://localhost:" + sockAddrHttp + "/admin"
}

func TestSuite(t *testing.T) {
	suite.Run(t, new(AclTestSuite))
}
