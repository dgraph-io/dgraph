//go:build integration

/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package main

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/dgraph-io/dgraph/v25/dgraphapi"
	"github.com/dgraph-io/dgraph/v25/dgraphtest"
)

type PluginTestSuite struct {
	suite.Suite
	dc dgraphapi.Cluster
}

func (psuite *PluginTestSuite) SetupTest() {
	psuite.dc = dgraphtest.NewComposeCluster()
}

func (psuite *PluginTestSuite) TearDownTest() {
	t := psuite.T()
	gcli, cleanup, err := psuite.dc.Client()
	require.NoError(t, err)
	defer cleanup()
	require.NoError(t, gcli.DropAll())
}

func (psuite *PluginTestSuite) Upgrade() {
	// Not implemented for integration tests
}

func TestPluginTestSuite(t *testing.T) {
	suite.Run(t, new(PluginTestSuite))
}
