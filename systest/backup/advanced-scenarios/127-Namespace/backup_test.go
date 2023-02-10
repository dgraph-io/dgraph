package main

import (
	"net/http"
	"testing"

	e2eCommon "github.com/dgraph-io/dgraph/graphql/e2e/common"
	utilsCommon "github.com/dgraph-io/dgraph/systest/backup/common"
	"github.com/dgraph-io/dgraph/testutil"
	"github.com/dgraph-io/dgraph/x"
)

var (
	headerAlpha1Np0   = http.Header{}
	headerAlpha1Np51  = http.Header{}
	headerAlpha1Np130 = http.Header{}

	headerAlpha2Np0   = http.Header{}
	headerAlpha2Np51  = http.Header{}
	headerAlpha2Np130 = http.Header{}
)

const (
	accessJwtHeader = "X-Dgraph-AccessToken"
	restoreLocation = "/data/backups/"
	backupDst       = "/data/backups/"
)

func Test127PlusNamespaces(t *testing.T) {
	jwtTokenAlpha1Np0 := testutil.GrootHttpLoginNamespace("http://"+testutil.SockAddrHttp+"/admin", 0).AccessJwt
	headerAlpha1Np0.Set(accessJwtHeader, jwtTokenAlpha1Np0)
	headerAlpha1Np0.Set("Content-Type", "application/json")

	jwtTokenAlpha2Np0 := testutil.GrootHttpLoginNamespace("http://"+testutil.ContainerAddr("alpha2", 8080)+"/admin", 0).AccessJwt
	headerAlpha2Np0.Set(accessJwtHeader, jwtTokenAlpha2Np0)
	headerAlpha2Np0.Set("Content-Type", "application/json")
	_ = e2eCommon.CreateNamespace(t, headerAlpha1Np0, e2eCommon.CreateNamespaceParams{CustomGraphAdminURLs: "", Count: 50})
	utilsCommon.AddSchema(t, headerAlpha1Np0, "alpha1")
	e2eCommon.AssertGetGQLSchema(t, testutil.ContainerAddr("alpha1", 8080), headerAlpha1Np0)
	utilsCommon.AddData(t, 1, 50, jwtTokenAlpha1Np0, "alpha1")
	utilsCommon.CheckDataExists(t, 30, jwtTokenAlpha1Np0, "alpha1")
	utilsCommon.TakeBackup(t, jwtTokenAlpha1Np0, backupDst, "alpha1")
	utilsCommon.RunRestore(t, jwtTokenAlpha2Np0, restoreLocation, "alpha2")
	dg1 := testutil.DgClientWithLogin(t, "groot", "password", x.GalaxyNamespace)
	testutil.WaitForRestore(t, dg1, testutil.ContainerAddr("alpha2", 8080))
	e2eCommon.AssertGetGQLSchema(t, testutil.ContainerAddr("alpha2", 8080), headerAlpha2Np0)
	utilsCommon.CheckDataExists(t, 30, jwtTokenAlpha2Np0, "alpha2")
	_ = e2eCommon.CreateNamespace(t, headerAlpha1Np0, e2eCommon.CreateNamespaceParams{CustomGraphAdminURLs: "", Count: 50})
	jwtTokenAlpha1Np51 := testutil.GrootHttpLoginNamespace("http://"+testutil.SockAddrHttp+"/admin", 51).AccessJwt
	headerAlpha1Np51.Set(accessJwtHeader, jwtTokenAlpha1Np51)
	headerAlpha1Np51.Set("Content-Type", "application/json")
	utilsCommon.AddSchema(t, headerAlpha1Np51, "alpha1")
	e2eCommon.AssertGetGQLSchema(t, testutil.ContainerAddr("alpha1", 8080), headerAlpha1Np51)
	utilsCommon.AddData(t, 51, 100, jwtTokenAlpha1Np51, "alpha1")
	utilsCommon.CheckDataExists(t, 70, jwtTokenAlpha1Np51, "alpha1")
	utilsCommon.TakeBackup(t, jwtTokenAlpha1Np0, backupDst, "alpha1")
	utilsCommon.RunRestore(t, jwtTokenAlpha2Np0, restoreLocation, "alpha2")
	dg2 := testutil.DgClientWithLogin(t, "groot", "password", x.GalaxyNamespace)
	testutil.WaitForRestore(t, dg2, testutil.ContainerAddr("alpha2", 8080))
	e2eCommon.AssertGetGQLSchema(t, testutil.ContainerAddr("alpha2", 8080), headerAlpha2Np0)
	utilsCommon.CheckDataExists(t, 30, jwtTokenAlpha2Np0, "alpha2")
	jwtTokenAlpha2Np51 := testutil.GrootHttpLoginNamespace("http://"+testutil.ContainerAddr("alpha2", 8080)+"/admin", 51).AccessJwt
	headerAlpha2Np51.Set(accessJwtHeader, jwtTokenAlpha2Np51)
	headerAlpha2Np51.Set("Content-Type", "application/json")
	e2eCommon.AssertGetGQLSchema(t, testutil.ContainerAddr("alpha2", 8080), headerAlpha2Np51)
	utilsCommon.CheckDataExists(t, 70, jwtTokenAlpha2Np51, "alpha2")
	_ = e2eCommon.CreateNamespace(t, headerAlpha1Np0, e2eCommon.CreateNamespaceParams{CustomGraphAdminURLs: "", Count: 30})
	jwtTokenAlpha1Np130 := testutil.GrootHttpLoginNamespace("http://"+testutil.SockAddrHttp+"/admin", 130).AccessJwt
	headerAlpha1Np130.Set(accessJwtHeader, jwtTokenAlpha1Np130)
	headerAlpha1Np130.Set("Content-Type", "application/json")
	utilsCommon.AddSchema(t, headerAlpha1Np130, "alpha1")
	e2eCommon.AssertGetGQLSchema(t, testutil.ContainerAddr("alpha1", 8080), headerAlpha1Np130)
	utilsCommon.AddData(t, 101, 130, jwtTokenAlpha1Np130, "alpha1")
	utilsCommon.CheckDataExists(t, 110, jwtTokenAlpha1Np130, "alpha1")
	utilsCommon.TakeBackup(t, jwtTokenAlpha1Np0, backupDst, "alpha1")
	utilsCommon.RunRestore(t, jwtTokenAlpha2Np0, restoreLocation, "alpha2")
	dg3 := testutil.DgClientWithLogin(t, "groot", "password", x.GalaxyNamespace)
	testutil.WaitForRestore(t, dg3, testutil.ContainerAddr("alpha2", 8080))
	e2eCommon.AssertGetGQLSchema(t, testutil.ContainerAddr("alpha2", 8080), headerAlpha2Np0)
	utilsCommon.CheckDataExists(t, 30, jwtTokenAlpha2Np0, "alpha2")
	e2eCommon.AssertGetGQLSchema(t, testutil.ContainerAddr("alpha2", 8080), headerAlpha2Np51)
	utilsCommon.CheckDataExists(t, 70, jwtTokenAlpha2Np51, "alpha2")
	jwtTokenAlpha2Np130 := testutil.GrootHttpLoginNamespace("http://"+testutil.ContainerAddr("alpha2", 8080)+"/admin", 130).AccessJwt
	headerAlpha2Np130.Set(accessJwtHeader, jwtTokenAlpha2Np130)
	headerAlpha2Np130.Set("Content-Type", "application/json")
	e2eCommon.AssertGetGQLSchema(t, testutil.ContainerAddr("alpha2", 8080), headerAlpha2Np130)
	utilsCommon.CheckDataExists(t, 110, jwtTokenAlpha2Np130, "alpha2")
}
