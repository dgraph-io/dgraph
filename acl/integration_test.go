//go:build integration

/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package acl

import (
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/hypermodeinc/dgraph/v25/dgraphapi"
	"github.com/hypermodeinc/dgraph/v25/dgraphtest"
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
