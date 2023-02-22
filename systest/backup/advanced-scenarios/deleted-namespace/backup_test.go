package main

import (
	"testing"

	e2eCommon "github.com/dgraph-io/dgraph/graphql/e2e/common"
	utilsCommon "github.com/dgraph-io/dgraph/systest/backup/common"
	"github.com/dgraph-io/dgraph/testutil"
	"github.com/dgraph-io/dgraph/x"
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
	_ = e2eCommon.CreateNamespaces(t, headerAlpha1Np0, "alpha1", 2)
	ns2 := e2eCommon.CreateNamespaces(t, headerAlpha1Np0, "alpha1", 2)
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
	e2eCommon.DeleteNamespace(t, ns2[1], headerAlpha1Np0, "alpha1")
	utilsCommon.TakeBackup(t, jwtTokenAlpha1Np0, backupDst, "alpha1")
	utilsCommon.RunRestore(t, jwtTokenAlpha2Np0, restoreLocation, "alpha2")
	dg1 := testutil.DgClientWithLogin(t, "groot", "password", x.GalaxyNamespace)
	testutil.WaitForRestore(t, dg1, testutil.ContainerAddr("alpha2", 8080))
	lastAddedNamespaceId := e2eCommon.CreateNamespace(t, headerAlpha2Np0, "alpha2")
	require.Equal(t, lastAddedNamespaceId > ns2[1], true)
	require.Greater(t, lastAddedNamespaceId, ns2[1])
	jwtTokenAlpha2Np1, _ := utilsCommon.GetJwtTokenAndHeader(t, "alpha2", 1)
	utilsCommon.CheckItemExists(t, 15, jwtTokenAlpha2Np1, "alpha2")
	jwtTokenAlpha2Np2, _ := utilsCommon.GetJwtTokenAndHeader(t, "alpha2", 2)
	utilsCommon.CheckItemExists(t, 25, jwtTokenAlpha2Np2, "alpha2")
	jwtTokenAlpha2Np3, _ := utilsCommon.GetJwtTokenAndHeader(t, "alpha2", 3)
	utilsCommon.CheckItemExists(t, 35, jwtTokenAlpha2Np3, "alpha2")
	nsl := e2eCommon.ListNamespaces(t, jwtTokenAlpha2Np0, headerAlpha2Np0, "alpha2")
	for _, ns := range nsl {
		require.NotEqual(t, ns, ns2[1])
	}
	require.Contains(t, nsl, lastAddedNamespaceId)

}
