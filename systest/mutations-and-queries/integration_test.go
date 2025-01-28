//go:build integration

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
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/hypermodeinc/dgraph/v24/dgraphapi"
	"github.com/hypermodeinc/dgraph/v24/dgraphtest"
)

type SystestTestSuite struct {
	suite.Suite
	dc dgraphapi.Cluster
}

func (ssuite *SystestTestSuite) SetupTest() {
	ssuite.dc = dgraphtest.NewComposeCluster()
}

func (ssuite *SystestTestSuite) SetupSubTest() {
	t := ssuite.T()
	gcli, cleanup, err := doGrpcLogin(ssuite)
	defer cleanup()
	require.NoError(t, err)
	require.NoError(t, gcli.DropAll())
}

func (ssuite *SystestTestSuite) CheckAllowedErrorPreUpgrade(err error) bool {
	return false
}

func (ssuite *SystestTestSuite) Upgrade() {
	// Not implemented for integration tests
}

func TestSystestTestSuite(t *testing.T) {
	suite.Run(t, new(SystestTestSuite))
}
