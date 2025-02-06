//go:build integration

/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 */

package acl

import (
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/hypermodeinc/dgraph/v24/dgraphapi"
	"github.com/hypermodeinc/dgraph/v24/dgraphtest"
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
