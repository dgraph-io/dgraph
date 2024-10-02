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

	"github.com/stretchr/testify/suite"

	"github.com/dgraph-io/dgraph/v24/dgraphapi"
	"github.com/dgraph-io/dgraph/v24/dgraphtest"
	"github.com/dgraph-io/dgraph/v24/x"
)

type LicenseTestSuite struct {
	suite.Suite
	dc dgraphapi.Cluster
	lc *dgraphtest.LocalCluster
	uc dgraphtest.UpgradeCombo
}

func (lsuite *LicenseTestSuite) SetupTest() {
	conf := dgraphtest.NewClusterConfig().WithNumAlphas(1).WithNumZeros(1).WithReplicas(1).
		WithEncryption().WithVersion(lsuite.uc.Before)
	c, err := dgraphtest.NewLocalCluster(conf)
	x.Panic(err)
	if err := c.Start(); err != nil {
		c.Cleanup(true)
		panic(err)
	}

	lsuite.dc = c
	lsuite.lc = c
}

func (lsuite *LicenseTestSuite) TearDownTest() {
	lsuite.lc.Cleanup(lsuite.T().Failed())
}

func (lsuite *LicenseTestSuite) Upgrade() {
	t := lsuite.T()

	if err := lsuite.lc.Upgrade(lsuite.uc.After, lsuite.uc.Strategy); err != nil {
		t.Fatal(err)
	}
}

func TestLicenseTestSuite(t *testing.T) {
	for _, uc := range dgraphtest.AllUpgradeCombos(false) {
		log.Printf("running: backup in [%v], restore in [%v]", uc.Before, uc.After)
		var tsuite LicenseTestSuite
		tsuite.uc = uc
		suite.Run(t, &tsuite)
		if t.Failed() {
			t.Fatal("TestLicenseTestSuite tests failed")
		}
	}
}
