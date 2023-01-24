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

	common.AddNamespaces(t, 20, headerAlpha1)

	common.AddSchema(t, jwtTokenAlpha1)

	common.CheckSchemaExists(t, testutil.SockAddrHttp, headerAlpha1)

	common.AddData(t, 1, 10, jwtTokenAlpha1)

	common.CheckDataExists(t, testutil.SockAddrHttp, 10, jwtTokenAlpha1)

	common.AddNamespaces(t, 20, headerAlpha1)

	common.AddData(t, 11, 20, jwtTokenAlpha1)

	common.CheckDataExists(t, testutil.SockAddrHttp, 20, jwtTokenAlpha1)

	common.AddNamespaces(t, 20, headerAlpha1)

	common.AddData(t, 21, 30, jwtTokenAlpha1)

	common.CheckDataExists(t, testutil.SockAddrHttp, 30, jwtTokenAlpha1)

	common.AddNamespaces(t, 20, headerAlpha1)

	common.AddData(t, 31, 40, jwtTokenAlpha1)

	common.CheckDataExists(t, testutil.SockAddrHttp, 40, jwtTokenAlpha1)

	common.AddNamespaces(t, 20, headerAlpha1)

	common.AddData(t, 41, 50, jwtTokenAlpha1)

	common.CheckDataExists(t, testutil.SockAddrHttp, 50, jwtTokenAlpha1)

	common.AddNamespaces(t, 20, headerAlpha1)

	common.AddData(t, 51, 60, jwtTokenAlpha1)

	common.CheckDataExists(t, testutil.SockAddrHttp, 60, jwtTokenAlpha1)

	common.AddNamespaces(t, 30, headerAlpha1)

	common.AddData(t, 61, 70, jwtTokenAlpha1)

	common.CheckDataExists(t, testutil.SockAddrHttp, 70, jwtTokenAlpha1)

	common.TakeBackup(t, jwtTokenAlpha1, backupDst)

	restored := common.RunRestore(t, jwtTokenAlpha2, restoreLocation)
	require.Equal(t, "Success", restored)
	common.WaitForRestore(t)

	common.CheckSchemaExists(t, testutil.ContainerAddr("alpha2", 8080), headerAlpha2)

	common.CheckDataExists(t, testutil.ContainerAddr("alpha2", 8080), 70, jwtTokenAlpha2)
}
