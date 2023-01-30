package main

import (
	"fmt"
	"net/http"
	"os"
	"testing"

	"github.com/dgraph-io/dgraph/systest/backup/common"
	"github.com/dgraph-io/dgraph/testutil"
	"github.com/stretchr/testify/require"
)

var (
	restoreLocation = "/data/backup/"
	backupDst       = "/data/backup/"
	headerAlpha1    = http.Header{}
	headerAlpha3    = http.Header{}
)

const (
	accessJwtHeader = "X-Dgraph-AccessToken"
)

func TestAclToNonAclBackupRestore(t *testing.T) {
	// Backup on acl cluster and Restore on non-acl

	fmt.Println("--------------------------------------> acl(backup cluster) to non-Acl(restore cluster) Test")
	jwtTokenAlpha1 := testutil.GrootHttpLogin("http://" + testutil.SockAddrHttp + "/admin").AccessJwt
	headerAlpha1.Set(accessJwtHeader, jwtTokenAlpha1)
	BackupRestore(t, jwtTokenAlpha1, "", headerAlpha1, "alpha1", "alpha2")
}

func TestNonAclToAclBackupRestore(t *testing.T) {
	// Backup on acl cluster and Restore on non-acl

	fmt.Println("--------------------------------------> non-acl(backup cluster) to acl(restore cluster) Test")
	jwtTokenAlpha3 := testutil.GrootHttpLogin("http://" + testutil.ContainerAddr("alpha3", 8080) + "/admin").AccessJwt
	headerAlpha3.Set(accessJwtHeader, jwtTokenAlpha3)
	BackupRestore(t, "", jwtTokenAlpha3, headerAlpha3, "alpha4", "alpha3")
}

func BackupRestore(t *testing.T, jwtTokenBackupAlpha string, jwtTokenRestoreAlpha string, headerjwtTokenBackupAlpha http.Header, backupAlpha string, restoreAlpha string) {
	err := common.RemoveContentsOfPerticularDir(t, "./data/backup/")
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	// *****************Get JWT-Token

	// jwtTokenAlpha1 := testutil.GrootHttpLogin("http://" + testutil.SockAddrHttp + "/admin").AccessJwt
	// headerAlpha1.Set(accessJwtHeader, jwtTokenAlpha1)
	// fmt.Println("Header1: ", jwtTokenAlpha1)

	// jwtTokenAlpha2 := testutil.GrootHttpLogin("http://" + testutil.ContainerAddr("alpha2", 8080) + "/admin").AccessJwt
	// headerAlpha2.Set(accessJwtHeader, jwtTokenAlpha2)
	// fmt.Println("Header2: ", jwtTokenAlpha2)

	// // *****************Add 20 Namespaces
	// common.AddNamespaces(t, 20, headerAlpha1)

	// *****************Add Initial data
	common.AddSchema(t, headerjwtTokenBackupAlpha, backupAlpha)

	common.CheckSchemaExists(t, headerjwtTokenBackupAlpha, backupAlpha)

	common.AddData(t, 1, 10, jwtTokenBackupAlpha, backupAlpha)

	common.CheckDataExists(t, 10, jwtTokenBackupAlpha, backupAlpha)

	// *****************Take Backup
	common.TakeBackup(t, jwtTokenBackupAlpha, backupDst, backupAlpha)

	// *****************Restore
	restored := common.RunRestore(t, jwtTokenRestoreAlpha, restoreLocation, restoreAlpha)
	require.Equal(t, "Success", restored)
	common.WaitForRestore(t, restoreAlpha)

	common.CheckDataExists(t, 10, jwtTokenRestoreAlpha, restoreAlpha)
}
