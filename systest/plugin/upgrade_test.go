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

type PluginTestSuite struct {
	suite.Suite
	dc dgraphtest.Cluster
	lc *dgraphtest.LocalCluster
	uc dgraphtest.UpgradeCombo
	pfiEntry PluginFuncInp
}

func (psuite *PluginTestSuite) SetupTest() {
	conf := dgraphtest.NewClusterConfig().WithNumAlphas(1).WithNumZeros(1).WithReplicas(1).
		WithACL(20 * time.Second).WithEncryption().WithVersion(psuite.uc.Before)
	c, err := dgraphtest.NewLocalCluster(conf)
	x.Panic(err)
	if err := c.Start(); err != nil {
		c.Cleanup(true)
		panic(err)
	}

	psuite.dc = c
	psuite.lc = c
}

func (psuite *PluginTestSuite) TearDownTest() {
	psuite.lc.Cleanup(psuite.T().Failed())
}

func (psuite *PluginTestSuite) Upgrade() {
	t := psuite.T()

	if err := psuite.lc.Upgrade(psuite.uc.After, psuite.uc.Strategy); err != nil {
		t.Fatal(err)
	}
}

// For testing purpose
var comboEntry = []dgraphtest.UpgradeCombo{
			{"v21.03.0", "v23.0.0", dgraphtest.BackupRestore},
}

func TestPluginTestSuite(t *testing.T) {
	//for _, uc := range dgraphtest.AllUpgradeCombos {
	//}
	for _, uc := range comboEntry {
		log.Printf("running: backup in [%v], restore in [%v]", uc.Before, uc.After)
		var psuite PluginTestSuite
		psuite.uc = uc
		for _, e := range pfiArray {
			psuite.pfiEntry = e
			suite.Run(t, &psuite)
		}
	}
}
