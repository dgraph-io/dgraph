//go:build integration

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
	"fmt"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/dgraph-io/dgraph/dgraphtest"
	"github.com/dgraph-io/dgraph/testutil"
)

type AclTestSuite struct {
	suite.Suite
}

func (suite *AclTestSuite) SetupTest() {
	cluster := dgraphtest.NewComposeCluster()
	dc = cluster
	sockAddr = testutil.SockAddr
	adminEndpoint = "http://" + testutil.SockAddrHttp + "/admin"
	fmt.Printf("Using adminEndpoint for acl package: %s\n", adminEndpoint)
}

func (suite *AclTestSuite) Upgrade() {
	// not implemented for integration tests
}
func TestSuite(t *testing.T) {
	suite.Run(t, new(AclTestSuite))
}
