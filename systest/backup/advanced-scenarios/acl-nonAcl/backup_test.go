package main

import (
	"net/http"
	"os"
	"testing"

	"github.com/dgraph-io/dgraph/systest/backup/common"
	"github.com/dgraph-io/dgraph/testutil"
)

var (
	restoreLocation = "/data/backups/"
	backupDst       = "/data/backups/"
	localBackupDest = "./data/backups"
	headerAlpha1    = http.Header{}
	headerAlpha3    = http.Header{}
)

const (
	accessJwtHeader = "X-Dgraph-AccessToken"
)

func TestAclToNonAclBackupRestore(t *testing.T) {
	t.Logf("acl(backup cluster) to non-Acl(restore cluster) Test")
	jwtTokenAlpha1 := testutil.GrootHttpLogin("http://" + testutil.SockAddrHttp + "/admin").AccessJwt
	headerAlpha1.Set(accessJwtHeader, jwtTokenAlpha1)
	BackupRestore(t, jwtTokenAlpha1, "", headerAlpha1, "alpha1", "alpha2")
}

func TestNonAclToAclBackupRestore(t *testing.T) {
	t.Logf("non-acl(backup cluster) to acl(restore cluster) Test")
	jwtTokenAlpha3 := testutil.GrootHttpLogin("http://" + testutil.ContainerAddr("alpha3", 8080) + "/admin").AccessJwt
	headerAlpha3.Set(accessJwtHeader, jwtTokenAlpha3)
	BackupRestore(t, "", jwtTokenAlpha3, headerAlpha3, "alpha4", "alpha3")
}

func BackupRestore(t *testing.T, jwtTokenBackupAlpha string, jwtTokenRestoreAlpha string, headerjwtTokenBackupAlpha http.Header, backupAlpha string, restoreAlpha string) {
	err := common.RemoveContentsOfPerticularDir(t, "./data/backups/")
	if err != nil {
		t.Logf("%s", err)
		os.Exit(1)
	}

	common.AddSchema(t, headerjwtTokenBackupAlpha, backupAlpha)

	common.CheckSchemaExists(t, headerjwtTokenBackupAlpha, backupAlpha)

	common.AddData(t, 1, 10, jwtTokenBackupAlpha, backupAlpha)

	common.CheckDataExists(t, 10, jwtTokenBackupAlpha, backupAlpha)

	common.TakeBackup(t, jwtTokenBackupAlpha, backupDst, backupAlpha)
	common.RunRestore(t, jwtTokenRestoreAlpha, restoreLocation, restoreAlpha)
	common.WaitForRestore(t, restoreAlpha)

	common.CheckDataExists(t, 10, jwtTokenRestoreAlpha, restoreAlpha)
}
