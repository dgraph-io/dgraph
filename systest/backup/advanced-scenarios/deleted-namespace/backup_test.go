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

	common.AddNamespaces(t, 2, headerAlpha1, "")

	common.AddSchema(t, headerAlpha1, "alpha1")

	common.CheckSchemaExists(t, headerAlpha1, "alpha1")

	common.AddData(t, 1, 5, jwtTokenAlpha1, "alpha1")

	common.CheckDataExists(t, 4, jwtTokenAlpha1, "alpha1")

	ns := common.AddNamespaces(t, 2, headerAlpha1, "")

	common.AddData(t, 6, 10, jwtTokenAlpha1, "alpha1")

	common.CheckDataExists(t, 7, jwtTokenAlpha1, "alpha1")

	common.DeleteNamespace(t, ns, jwtTokenAlpha1, "alpha1")

	common.TakeBackup(t, jwtTokenAlpha1, backupDst, "alpha1")
	common.RunRestore(t, jwtTokenAlpha2, restoreLocation, "alpha2")
	common.WaitForRestore(t, "alpha2")
	alpha2AdminUrl := "http://" + testutil.ContainerAddr("alpha2", 8080) + "/admin"
	lastAddedNamespaceId := common.AddNamespaces(t, 1, headerAlpha2, alpha2AdminUrl)

	require.Equal(t, lastAddedNamespaceId > ns, true)
}
