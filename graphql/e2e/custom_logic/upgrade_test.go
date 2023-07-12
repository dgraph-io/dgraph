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

package custom_logic

import (
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/dgraph-io/dgraph/dgraphtest"
	"github.com/dgraph-io/dgraph/x"
)

type CustomLogicTestSuite struct {
	suite.Suite
	c          *dgraphtest.LocalCluster
	dc         dgraphtest.Cluster
	alpha1Port string
}

func (suite *CustomLogicTestSuite) SetupTest() {
	conf := dgraphtest.NewClusterConfig().WithNumAlphas(1).WithNumZeros(1).WithGeneric(1).
		WithACL(20 * time.Second).WithEncryption().WithVersion("0c9f60156").WithReplicas(1)
	c, err := dgraphtest.NewLocalCluster(conf)
	x.Panic(err)
	if err := c.Start(); err != nil {
		suite.T().Fatal(err)
	}
	suite.c = c
	suite.dc = c
}

func (suite *CustomLogicTestSuite) TearDownTest() {
	suite.c.Cleanup(suite.T().Failed())
}

func (suite *CustomLogicTestSuite) Upgrade() {
	x.Panic(suite.c.Upgrade("v23.0.0-rc1", dgraphtest.StopStart))
}

func TestCustomLogicSuite(t *testing.T) {
	suite.Run(t, new(CustomLogicTestSuite))
}
