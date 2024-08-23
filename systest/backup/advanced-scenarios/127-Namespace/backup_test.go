//go:build integration

/*
 * Copyright 2023 Dgraph Labs, Inc. and Contributors *
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

	e2eCommon "github.com/dgraph-io/dgraph/v24/graphql/e2e/common"
	utilsCommon "github.com/dgraph-io/dgraph/v24/systest/backup/common"
	"github.com/dgraph-io/dgraph/v24/testutil"
	"github.com/dgraph-io/dgraph/v24/x"
)

const (
	accessJwtHeader = "X-Dgraph-AccessToken"
	restoreLocation = "/data/backups/"
	backupDst       = "/data/backups/"
)

func Test127PlusNamespaces(t *testing.T) {
	alpha1Addr := testutil.ContainerAddr("alpha1", 8080)
	alpha2Addr := testutil.ContainerAddr("alpha2", 8080)

	jwtTokenAlpha1Np0, headerAlpha1Np0 := utilsCommon.GetJwtTokenAndHeader(t, "alpha1", 0)
	jwtTokenAlpha2Np0, headerAlpha2Np0 := utilsCommon.GetJwtTokenAndHeader(t, "alpha2", 0)
	_ = e2eCommon.CreateNamespaces(t, headerAlpha1Np0, "alpha1", 50)
	utilsCommon.AddItemSchema(t, headerAlpha1Np0, "alpha1")
	e2eCommon.AssertGetGQLSchema(t, alpha1Addr, headerAlpha1Np0)
	utilsCommon.AddItem(t, 1, 50, jwtTokenAlpha1Np0, "alpha1")
	utilsCommon.CheckItemExists(t, 30, jwtTokenAlpha1Np0, "alpha1")
	utilsCommon.TakeBackup(t, jwtTokenAlpha1Np0, backupDst, "alpha1")
	utilsCommon.RunRestore(t, jwtTokenAlpha2Np0, restoreLocation, "alpha2")
	dg1 := testutil.DgClientWithLogin(t, "groot", "password", x.GalaxyNamespace)
	testutil.WaitForRestore(t, dg1, alpha2Addr)
	e2eCommon.AssertGetGQLSchema(t, alpha2Addr, headerAlpha2Np0)
	utilsCommon.CheckItemExists(t, 30, jwtTokenAlpha2Np0, "alpha2")
	_ = e2eCommon.CreateNamespaces(t, headerAlpha1Np0, "alpha1", 50)
	jwtTokenAlpha1Np51, headerAlpha1Np51 := utilsCommon.GetJwtTokenAndHeader(t, "alpha1", 51)
	utilsCommon.AddItemSchema(t, headerAlpha1Np51, "alpha1")
	e2eCommon.AssertGetGQLSchema(t, alpha1Addr, headerAlpha1Np51)
	utilsCommon.AddItem(t, 51, 100, jwtTokenAlpha1Np51, "alpha1")
	utilsCommon.CheckItemExists(t, 70, jwtTokenAlpha1Np51, "alpha1")
	utilsCommon.TakeBackup(t, jwtTokenAlpha1Np0, backupDst, "alpha1")
	utilsCommon.RunRestore(t, jwtTokenAlpha2Np0, restoreLocation, "alpha2")
	dg2 := testutil.DgClientWithLogin(t, "groot", "password", x.GalaxyNamespace)
	testutil.WaitForRestore(t, dg2, alpha2Addr)
	e2eCommon.AssertGetGQLSchema(t, alpha2Addr, headerAlpha2Np0)
	utilsCommon.CheckItemExists(t, 30, jwtTokenAlpha2Np0, "alpha2")
	jwtTokenAlpha2Np51, headerAlpha2Np51 := utilsCommon.GetJwtTokenAndHeader(t, "alpha1", 51)
	e2eCommon.AssertGetGQLSchema(t, alpha2Addr, headerAlpha2Np51)
	utilsCommon.CheckItemExists(t, 70, jwtTokenAlpha2Np51, "alpha2")
	_ = e2eCommon.CreateNamespaces(t, headerAlpha1Np0, "alpha1", 30)
	jwtTokenAlpha1Np130, headerAlpha1Np130 := utilsCommon.GetJwtTokenAndHeader(t, "alpha1", 130)
	utilsCommon.AddItemSchema(t, headerAlpha1Np130, "alpha1")
	e2eCommon.AssertGetGQLSchema(t, alpha1Addr, headerAlpha1Np130)
	utilsCommon.AddItem(t, 101, 130, jwtTokenAlpha1Np130, "alpha1")
	utilsCommon.CheckItemExists(t, 110, jwtTokenAlpha1Np130, "alpha1")
	utilsCommon.TakeBackup(t, jwtTokenAlpha1Np0, backupDst, "alpha1")
	utilsCommon.RunRestore(t, jwtTokenAlpha2Np0, restoreLocation, "alpha2")
	dg3 := testutil.DgClientWithLogin(t, "groot", "password", x.GalaxyNamespace)
	testutil.WaitForRestore(t, dg3, alpha2Addr)
	e2eCommon.AssertGetGQLSchema(t, alpha2Addr, headerAlpha2Np0)
	utilsCommon.CheckItemExists(t, 30, jwtTokenAlpha2Np0, "alpha2")
	e2eCommon.AssertGetGQLSchema(t, alpha2Addr, headerAlpha2Np51)
	utilsCommon.CheckItemExists(t, 70, jwtTokenAlpha2Np51, "alpha2")
	jwtTokenAlpha2Np130, headerAlpha2Np130 := utilsCommon.GetJwtTokenAndHeader(t, "alpha1", 130)
	e2eCommon.AssertGetGQLSchema(t, alpha2Addr, headerAlpha2Np130)
	utilsCommon.CheckItemExists(t, 110, jwtTokenAlpha2Np130, "alpha2")
}
