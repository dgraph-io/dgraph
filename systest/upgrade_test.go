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
	"log"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/dgraph-io/dgraph/dgraphtest"
	"github.com/dgraph-io/dgraph/x"
)

type SystestTestSuite struct {
	suite.Suite
	dc dgraphtest.Cluster
	lc *dgraphtest.LocalCluster
	uc dgraphtest.UpgradeCombo
}

func (ssuite *SystestTestSuite) SetupTest() {
	conf := dgraphtest.NewClusterConfig().WithNumAlphas(1).WithNumZeros(1).WithReplicas(1).
		WithACL(20 * time.Second).WithEncryption().WithVersion(ssuite.uc.Before)
	c, err := dgraphtest.NewLocalCluster(conf)
	x.Panic(err)
	if err := c.Start(); err != nil {
		c.Cleanup(true)
		panic(err)
	}

	ssuite.dc = c
	ssuite.lc = c
}

func (ssuite *SystestTestSuite) TearDownTest() {
	ssuite.lc.Cleanup(ssuite.T().Failed())
}

func (ssuite *SystestTestSuite) Upgrade() {
	t := ssuite.T()

	if err := ssuite.lc.Upgrade(ssuite.uc.After, ssuite.uc.Strategy); err != nil {
		t.Fatal(err)
	}
}

func TestSystestTestSuite(t *testing.T) {
	for _, uc := range dgraphtest.AllUpgradeCombos {
		log.Printf("running: backup in [%v], restore in [%v]", uc.Before, uc.After)
		var ssuite SystestTestSuite
		ssuite.uc = uc
		suite.Run(t, &ssuite)
	}
}
