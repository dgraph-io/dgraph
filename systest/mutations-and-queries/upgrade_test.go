//go:build upgrade

/*
 * Copyright 2025 Hypermode Inc. and Contributors
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
	"errors"
	"log"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/hypermodeinc/dgraph/v24/dgraphapi"
	"github.com/hypermodeinc/dgraph/v24/dgraphtest"
	"github.com/hypermodeinc/dgraph/v24/x"
)

type SystestTestSuite struct {
	suite.Suite
	dc dgraphapi.Cluster
	lc *dgraphtest.LocalCluster
	uc dgraphtest.UpgradeCombo
}

func (ssuite *SystestTestSuite) SetupSubTest() {
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

func (ssuite *SystestTestSuite) TearDownSubTest() {
	ssuite.lc.Cleanup(ssuite.T().Failed())
}

func (ssuite *SystestTestSuite) CheckAllowedErrorPreUpgrade(err error) bool {
	if val, checkErr := dgraphtest.IsHigherVersion(
		ssuite.dc.GetVersion(),
		"315747a19e9d5c5b98055c8b943a6e6462153bb3"); checkErr == nil && val || checkErr != nil {
		return false
	}
	return strings.Contains(err.Error(), "cannot initialize iterator when calling List.iterate: deleteBelowTs")
}

func (ssuite *SystestTestSuite) Upgrade() {
	require.NoError(ssuite.T(), ssuite.lc.Upgrade(ssuite.uc.After, ssuite.uc.Strategy))
}

func TestSystestTestSuite(t *testing.T) {
	for _, uc := range dgraphtest.AllUpgradeCombos(false) {
		log.Printf("running: backup in [%v], restore in [%v]", uc.Before, uc.After)
		var ssuite SystestTestSuite
		ssuite.uc = uc
		suite.Run(t, &ssuite)
		if t.Failed() {
			x.Panic(errors.New("TestSystestTestSuite tests failed"))
		}
	}
}
