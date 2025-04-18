//go:build integration

/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package main

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/hypermodeinc/dgraph/v25/dgraphapi"
	"github.com/hypermodeinc/dgraph/v25/dgraphtest"
)

type SystestTestSuite struct {
	suite.Suite
	dc dgraphapi.Cluster
}

func (ssuite *SystestTestSuite) SetupTest() {
	ssuite.dc = dgraphtest.NewComposeCluster()

	t := ssuite.T()
	gcli, cleanup, err := doGrpcLogin(ssuite)
	defer cleanup()
	require.NoError(t, err)
	require.NoError(t, gcli.DropAll())
}

func (ssuite *SystestTestSuite) CheckAllowedErrorPreUpgrade(err error) bool {
	return false
}

func (ssuite *SystestTestSuite) Upgrade() {
	// Not implemented for integration tests
}

func TestSystestSuite(t *testing.T) {
	suite.Run(t, new(SystestTestSuite))
}
