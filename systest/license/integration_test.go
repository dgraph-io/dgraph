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
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/dgraph-io/dgraph/v24/dgraphapi"
	"github.com/dgraph-io/dgraph/v24/dgraphtest"
)

type LicenseTestSuite struct {
	suite.Suite
	dc dgraphapi.Cluster
}

func (lsuite *LicenseTestSuite) SetupTest() {
	lsuite.dc = dgraphtest.NewComposeCluster()
}

func (lsuite *LicenseTestSuite) TearDownTest() {
}

func (lsuite *LicenseTestSuite) Upgrade() {
	// Not implemented for integration tests
}

func TestLicenseTestSuite(t *testing.T) {
	suite.Run(t, new(LicenseTestSuite))
	if t.Failed() {
		t.Fatal("TestLicenseTestSuite tests failed")
	}
}
