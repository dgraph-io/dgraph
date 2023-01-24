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
	headerAlpha2    = http.Header{}
)

const (
	accessJwtHeader = "X-Dgraph-AccessToken"
)

func TestDeletedNamespaceID(t *testing.T) {
	err := common.RemoveContentsOfPerticularDir(t, "./data/backup/")
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	jwtTokenAlpha1 := testutil.GrootHttpLogin("http://" + testutil.SockAddrHttp + "/admin").AccessJwt
	headerAlpha1.Set(accessJwtHeader, jwtTokenAlpha1)

	jwtTokenAlpha2 := testutil.GrootHttpLogin("http://" + testutil.ContainerAddr("alpha2", 8080) + "/admin").AccessJwt
	headerAlpha2.Set(accessJwtHeader, jwtTokenAlpha2)

	common.AddNamespaces(t, 2, headerAlpha1)

	common.AddSchema(t, jwtTokenAlpha1)

	common.CheckSchemaExists(t, testutil.SockAddrHttp, headerAlpha1)

	common.AddData(t, 1, 5, jwtTokenAlpha1)

	common.CheckDataExists(t, testutil.SockAddrHttp, 4, jwtTokenAlpha1)

	common.AddNamespaces(t, 2, headerAlpha1)

	common.AddData(t, 6, 10, jwtTokenAlpha1)

	common.CheckDataExists(t, testutil.SockAddrHttp, 7, jwtTokenAlpha1)

	common.DeleteNamespace(t, 4, jwtTokenAlpha1)

	common.TakeBackup(t, jwtTokenAlpha1, backupDst)

	restored := common.RunRestore(t, jwtTokenAlpha2, restoreLocation)
	require.Equal(t, "Success", restored)
	common.WaitForRestore(t)
	lastAddedNamespaceId := common.AddNamespaces(t, 1, headerAlpha2)

	require.Equal(t, lastAddedNamespaceId > 4, true)
}
