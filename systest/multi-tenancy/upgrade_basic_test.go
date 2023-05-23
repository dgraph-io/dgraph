//go:build upgrade

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
	"time"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/dgraph-io/dgraph/dgraphtest"
	"github.com/dgraph-io/dgo/v230/protos/api"
	"github.com/dgraph-io/dgraph/x"
)

type MultitenancyTestSuite struct {
	suite.Suite
	dc dgraphtest.Cluster
	lc *dgraphtest.LocalCluster
	cleanup func()
	backupIn string
	restoreIn string
}

func (suite *MultitenancyTestSuite) SetupTest() {
	conf := dgraphtest.NewClusterConfig().WithNumAlphas(1).WithNumZeros(1).WithReplicas(1).
		WithACL(20 * time.Second).WithEncryption().WithVersion(suite.backupIn)

	c, err := dgraphtest.NewLocalCluster(conf)
	x.Panic(err)

	x.Panic(c.Start())

	suite.dc = c
	suite.lc = c
}

func (suite *MultitenancyTestSuite) TearDownTest() {
	t := suite.T()

	
	t.Cleanup(func() {suite.lc.Cleanup(t.Failed())})
	t.Cleanup(suite.cleanup)
}

func (suite *MultitenancyTestSuite) prepare() {
	t := suite.T()

	gc, cleanup, err := suite.dc.Client()
	suite.cleanup = cleanup
	require.NoError(t, err)
	err = gc.LoginIntoNamespace(context.Background(), "groot", "password", x.GalaxyNamespace)
	require.NoError(t, err, "login with galaxy failed")
	require.NoError(t, gc.Alter(context.Background(), &api.Operation{DropAll: true}))
}

func (suite *MultitenancyTestSuite) Upgrade(uStrategy dgraphtest.UpgradeStrategy) {
	t := suite.T()

	if err := suite.lc.Upgrade(suite.restoreIn, uStrategy); err != nil {
		t.Fatal(err)
	}
}

func TestMultitenancyTestSuite(t *testing.T) {
	for _, br := range dgraphtest.UpgradeCombos {
		m := new(MultitenancyTestSuite)
		m.backupIn = br[0]
		m.restoreIn = br[1]
		suite.Run(t, m)
	}
}
