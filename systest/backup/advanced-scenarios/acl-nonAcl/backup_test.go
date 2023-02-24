package main

import (
	"net/http"
	"testing"

	e2eCommon "github.com/dgraph-io/dgraph/graphql/e2e/common"
	utilsCommon "github.com/dgraph-io/dgraph/systest/backup/common"
	"github.com/dgraph-io/dgraph/testutil"
	"github.com/dgraph-io/dgraph/x"
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

func BackupRestore(t *testing.T, jwtTokenBackupAlpha string, jwtTokenRestoreAlpha string, header http.Header, backupAlpha string, restoreAlpha string) { //nolint:lll

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
