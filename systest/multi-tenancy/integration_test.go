//go:build integration

/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package main

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/dgraph-io/dgo/v250/protos/api"
	"github.com/hypermodeinc/dgraph/v25/dgraphapi"
	"github.com/hypermodeinc/dgraph/v25/dgraphtest"
	"github.com/hypermodeinc/dgraph/v25/x"
)

type MultitenancyTestSuite struct {
	suite.Suite
	dc dgraphapi.Cluster
}

func (msuite *MultitenancyTestSuite) SetupTest() {
	msuite.dc = dgraphtest.NewComposeCluster()
}

func (msuite *MultitenancyTestSuite) TearDownTest() {
	t := msuite.T()
	gcli, cleanup, err := msuite.dc.Client()
	defer cleanup()
	require.NoError(t, err)
	require.NoError(t, gcli.LoginIntoNamespace(context.Background(),
		dgraphapi.DefaultUser, dgraphapi.DefaultPassword, x.RootNamespace))
	require.NoError(t, gcli.Alter(context.Background(), &api.Operation{DropAll: true}))
}

func (msuite *MultitenancyTestSuite) Upgrade() {
	// Not implemented for integration tests
}

func TestACLSuite(t *testing.T) {
	suite.Run(t, new(MultitenancyTestSuite))
}
