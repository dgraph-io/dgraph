//go:build integration

/*
 * Copyright 2023 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package main

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/dgraph-io/dgraph/dgraphtest"
	"github.com/dgraph-io/dgo/v230/protos/api"
	"github.com/dgraph-io/dgraph/x"
)

var srcDB, dstDB string = "v22.0.2", "v23.0.0-rc1"

type MultitenancyTestSuite struct {
	suite.Suite
	dc dgraphtest.Cluster
	cleanup func()
}

func (suite *MultitenancyTestSuite) SetupTest() {
	suite.dc = dgraphtest.NewComposeCluster()
}

func (suite *MultitenancyTestSuite) TearDownTest() {
	t := suite.T()

	t.Cleanup(suite.cleanup)
}

func (suite *MultitenancyTestSuite) Upgrade(dstDB string, uStrategy dgraphtest.UpgradeStrategy) {
	// Not implemented for integration tests
}

func (suite *MultitenancyTestSuite) prepare() {
	t := suite.T()

	gc, cu, err := suite.dc.Client()
	suite.cleanup = cu
	require.NoError(t, err)
	require.NoError(t, gc.LoginIntoNamespace(context.Background(), "groot", "password", x.GalaxyNamespace), "login with galaxy failed")
	require.NoError(t, gc.Alter(context.Background(), &api.Operation{DropAll: true}))
}

func TestACLSuite(t *testing.T) {
	suite.Run(t, new(MultitenancyTestSuite))
}
