//go:build integration

/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package main

import (
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/hypermodeinc/dgraph/v24/dgraphapi"
	"github.com/hypermodeinc/dgraph/v24/dgraphtest"
)

type LicenseTestSuite struct {
	suite.Suite
	dc dgraphapi.Cluster
}

func (lsuite *LicenseTestSuite) SetupTest() {
	lsuite.dc = dgraphtest.NewComposeCluster()
}

func (lsuite *LicenseTestSuite) TearDownTest() {
}

func (lsuite *LicenseTestSuite) Upgrade() {
	// Not implemented for integration tests
}

func TestLicenseTestSuite(t *testing.T) {
	suite.Run(t, new(LicenseTestSuite))
	if t.Failed() {
		t.Fatal("TestLicenseTestSuite tests failed")
	}
}
