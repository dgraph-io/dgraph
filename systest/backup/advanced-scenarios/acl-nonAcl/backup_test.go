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
	"net/http"
	"testing"

	e2eCommon "github.com/dgraph-io/dgraph/v24/graphql/e2e/common"
	utilsCommon "github.com/dgraph-io/dgraph/v24/systest/backup/common"
	"github.com/dgraph-io/dgraph/v24/testutil"
	"github.com/dgraph-io/dgraph/v24/x"
)

const (
	restoreLocation = "/data/backups/"
	backupDst       = "/data/backups/"
)

func TestAclToNonAclBackupRestore(t *testing.T) {
	t.Logf("acl(backup cluster) to non-Acl(restore cluster) Test")
	jwtTokenAlpha1, headerAlpha1 := utilsCommon.GetJwtTokenAndHeader(t, "alpha1", 0)
	BackupRestore(t, jwtTokenAlpha1, "", headerAlpha1, "alpha1", "alpha2")
}

func TestNonAclToAclBackupRestore(t *testing.T) {
	t.Logf("non-acl(backup cluster) to acl(restore cluster) Test")
	jwtTokenAlpha3, headerAlpha3 := utilsCommon.GetJwtTokenAndHeader(t, "alpha3", 0)
	BackupRestore(t, "", jwtTokenAlpha3, headerAlpha3, "alpha4", "alpha3")
}

func BackupRestore(t *testing.T, jwtTokenBackupAlpha string, jwtTokenRestoreAlpha string,
	header http.Header, backupAlpha string, restoreAlpha string) {

	utilsCommon.AddItemSchema(t, header, backupAlpha)
	e2eCommon.AssertGetGQLSchema(t, testutil.ContainerAddr(backupAlpha, 8080), header)
	utilsCommon.AddItem(t, 1, 10, jwtTokenBackupAlpha, backupAlpha)
	utilsCommon.CheckItemExists(t, 5, jwtTokenBackupAlpha, backupAlpha)
	utilsCommon.TakeBackup(t, jwtTokenBackupAlpha, backupDst, backupAlpha)
	utilsCommon.RunRestore(t, jwtTokenRestoreAlpha, restoreLocation, restoreAlpha)
	dg := testutil.DgClientWithLogin(t, "groot", "password", x.GalaxyNamespace)
	testutil.WaitForRestore(t, dg, testutil.ContainerAddr(restoreAlpha, 8080))
	utilsCommon.CheckItemExists(t, 5, jwtTokenRestoreAlpha, restoreAlpha)
}
