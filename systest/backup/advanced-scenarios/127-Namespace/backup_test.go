package main

import (
	"fmt"
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
	headerAlpha2    = http.Header{}
)

const (
	accessJwtHeader = "X-Dgraph-AccessToken"
)

func TestDeletedNamespaceID(t *testing.T) {
	err := common.RemoveContentsOfPerticularDir(t, "./data/backups/")
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	jwtTokenAlpha1 := testutil.GrootHttpLogin("http://" + testutil.SockAddrHttp + "/admin").AccessJwt
	headerAlpha1.Set(accessJwtHeader, jwtTokenAlpha1)

	jwtTokenAlpha2 := testutil.GrootHttpLogin("http://" + testutil.ContainerAddr("alpha2", 8080) + "/admin").AccessJwt
	headerAlpha2.Set(accessJwtHeader, jwtTokenAlpha2)

	common.AddNamespaces(t, 20, headerAlpha1, "")

	common.AddSchema(t, headerAlpha1, "alpha1")

	common.CheckSchemaExists(t, headerAlpha1, "alpha1")

	common.AddData(t, 1, 10, jwtTokenAlpha1, "alpha1")

	common.CheckDataExists(t, 10, jwtTokenAlpha1, "alpha1")

	common.AddNamespaces(t, 20, headerAlpha1, "")

	common.AddData(t, 11, 20, jwtTokenAlpha1, "alpha1")

	common.CheckDataExists(t, 20, jwtTokenAlpha1, "alpha1")

	common.AddNamespaces(t, 20, headerAlpha1, "")

	common.AddData(t, 21, 30, jwtTokenAlpha1, "alpha1")

	common.CheckDataExists(t, 30, jwtTokenAlpha1, "alpha1")

	common.AddNamespaces(t, 20, headerAlpha1, "")

	common.AddData(t, 31, 40, jwtTokenAlpha1, "alpha1")

	common.CheckDataExists(t, 40, jwtTokenAlpha1, "alpha1")

	common.AddNamespaces(t, 20, headerAlpha1, "")

	common.AddData(t, 41, 50, jwtTokenAlpha1, "alpha1")

	common.CheckDataExists(t, 50, jwtTokenAlpha1, "alpha1")

	common.AddNamespaces(t, 20, headerAlpha1, "")

	common.AddData(t, 51, 60, jwtTokenAlpha1, "alpha1")

	common.CheckDataExists(t, 60, jwtTokenAlpha1, "alpha1")

	common.AddNamespaces(t, 30, headerAlpha1, "")

	common.AddData(t, 61, 70, jwtTokenAlpha1, "alpha1")

	common.CheckDataExists(t, 70, jwtTokenAlpha1, "alpha1")

	common.TakeBackup(t, jwtTokenAlpha1, backupDst, "alpha1")
	common.RunRestore(t, jwtTokenAlpha2, restoreLocation, "alpha2")
	common.WaitForRestore(t, "alpha2")

	common.CheckSchemaExists(t, headerAlpha2, "alpha2")

	common.CheckDataExists(t, 70, jwtTokenAlpha2, "alpha2")
}
