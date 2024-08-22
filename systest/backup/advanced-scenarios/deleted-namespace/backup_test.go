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
	"github.com/stretchr/testify/require"
)

const (
	accessJwtHeader = "X-Dgraph-AccessToken"
	restoreLocation = "/data/backups/"
	backupDst       = "/data/backups/"
)

func TestDeletedNamespaceID(t *testing.T) {
	jwtTokenAlpha1Np0, headerAlpha1Np0 := utilsCommon.GetJwtTokenAndHeader(t, "alpha1", 0)
	jwtTokenAlpha2Np0, headerAlpha2Np0 := utilsCommon.GetJwtTokenAndHeader(t, "alpha2", 0)
	ns1 := e2eCommon.CreateNamespaces(t, headerAlpha1Np0, "alpha1", 4)
	jwtTokenAlpha1Np1, headerAlpha1Np1 := utilsCommon.GetJwtTokenAndHeader(t, "alpha1", 1)
	utilsCommon.AddItemSchema(t, headerAlpha1Np1, "alpha1")
	utilsCommon.AddItem(t, 11, 20, jwtTokenAlpha1Np1, "alpha1")
	jwtTokenAlpha1Np2, headerAlpha1Np2 := utilsCommon.GetJwtTokenAndHeader(t, "alpha1", 2)
	utilsCommon.AddItemSchema(t, headerAlpha1Np2, "alpha1")
	utilsCommon.AddItem(t, 21, 30, jwtTokenAlpha1Np2, "alpha1")
	jwtTokenAlpha1Np3, headerAlpha1Np3 := utilsCommon.GetJwtTokenAndHeader(t, "alpha1", 3)
	utilsCommon.AddItemSchema(t, headerAlpha1Np3, "alpha1")
	utilsCommon.AddItem(t, 31, 40, jwtTokenAlpha1Np3, "alpha1")
	jwtTokenAlpha1Np4, headerAlpha1Np4 := utilsCommon.GetJwtTokenAndHeader(t, "alpha1", 4)
	utilsCommon.AddItemSchema(t, headerAlpha1Np4, "alpha1")
	utilsCommon.AddItem(t, 41, 50, jwtTokenAlpha1Np4, "alpha1")
	e2eCommon.DeleteNamespace(t, ns1[2], headerAlpha1Np0, "alpha1")
	utilsCommon.TakeBackup(t, jwtTokenAlpha1Np0, backupDst, "alpha1")
	utilsCommon.RunRestore(t, jwtTokenAlpha2Np0, restoreLocation, "alpha2")
	dg1 := testutil.DgClientWithLogin(t, "groot", "password", x.GalaxyNamespace)
	testutil.WaitForRestore(t, dg1, testutil.ContainerAddr("alpha2", 8080))
	lastAddedNamespaceId := e2eCommon.CreateNamespace(t, headerAlpha2Np0, "alpha2")
	require.Equal(t, lastAddedNamespaceId > ns1[3], true)
	require.Greater(t, lastAddedNamespaceId, ns1[3])
	jwtTokenAlpha2Np1, _ := utilsCommon.GetJwtTokenAndHeader(t, "alpha2", 1)
	utilsCommon.CheckItemExists(t, 15, jwtTokenAlpha2Np1, "alpha2")
	jwtTokenAlpha2Np2, _ := utilsCommon.GetJwtTokenAndHeader(t, "alpha2", 2)
	utilsCommon.CheckItemExists(t, 35, jwtTokenAlpha2Np2, "alpha2")
	jwtTokenAlpha2Np3, _ := utilsCommon.GetJwtTokenAndHeader(t, "alpha2", 4)
	utilsCommon.CheckItemExists(t, 35, jwtTokenAlpha2Np3, "alpha2")
	nsl := e2eCommon.ListNamespaces(t, jwtTokenAlpha2Np0, headerAlpha2Np0, "alpha2")
	for _, ns := range nsl {
		require.NotEqual(t, ns, ns1[2])
	}
	require.Contains(t, nsl, lastAddedNamespaceId)

}
