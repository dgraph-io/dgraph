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

	"github.com/dgraph-io/dgo/v230/protos/api"
	"github.com/dgraph-io/dgraph/dgraphtest"
	"github.com/dgraph-io/dgraph/x"
)

type PluginTestSuite struct {
	suite.Suite
	dc dgraphtest.Cluster
}

func (psuite *PluginTestSuite) SetupTest() {
	psuite.dc = dgraphtest.NewComposeCluster()
}

func (psuite *PluginTestSuite) TearDownTest() {
	t := psuite.T()
	gcli, cleanup, err := psuite.dc.Client()
	defer cleanup()
	require.NoError(t, err)
	require.NoError(t, gcli.LoginIntoNamespace(context.Background(),
		dgraphtest.DefaultUser, dgraphtest.DefaultPassword, x.GalaxyNamespace))
	require.NoError(t, gcli.Alter(context.Background(), &api.Operation{DropAll: true}))
}

func (psuite *PluginTestSuite) Upgrade() {
	// Not implemented for integration tests
}

func TestPluginTestSuite(t *testing.T) {
	for _, e := range pfiArray {
		psuite.pfiEntry := e
		suite.Run(t, new(PluginTestSuite))
	}
}
