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
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/dgraph-io/dgraph/v24/dgraphapi"
	"github.com/dgraph-io/dgraph/v24/dgraphtest"
)

type AclTestSuite struct {
	suite.Suite
	dc dgraphapi.Cluster
}

func (suite *AclTestSuite) SetupTest() {
	suite.dc = dgraphtest.NewComposeCluster()
}

func (suite *AclTestSuite) Upgrade() {
	// not implemented for integration tests
}

func TestACLSuite(t *testing.T) {
	suite.Run(t, new(AclTestSuite))
}
