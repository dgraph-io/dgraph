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
	headerAlpha1 = http.Header{}
	headerAlpha2 = http.Header{}
)

const (
	accessJwtHeader = "X-Dgraph-AccessToken"
	restoreLocation = "/data/backups/"
	backupDst       = "/data/backups/"
)

func TestDeletedNamespaceID(t *testing.T) {
	jwtTokenAlpha1 := testutil.GrootHttpLogin("http://" + testutil.SockAddrHttp + "/admin").AccessJwt
	headerAlpha1.Set(accessJwtHeader, jwtTokenAlpha1)
	headerAlpha1.Set("Content-Type", "application/json")
	jwtTokenAlpha2 := testutil.GrootHttpLogin("http://" + testutil.ContainerAddr("alpha2", 8080) + "/admin").AccessJwt
	headerAlpha2.Set(accessJwtHeader, jwtTokenAlpha2)
	headerAlpha2.Set("Content-Type", "application/json")
	_ = e2eCommon.CreateNamespace(t, headerAlpha1, e2eCommon.CreateNamespaceParams{CustomGraphAdminURLs: "", NamespaceQuant: 20})
	utilsCommon.AddSchema(t, headerAlpha1, "alpha1")
	e2eCommon.AssertGetGQLSchema(t, testutil.ContainerAddr("alpha1", 8080), headerAlpha1)
	utilsCommon.AddData(t, 1, 10, jwtTokenAlpha1, "alpha1")
	utilsCommon.CheckDataExists(t, 10, jwtTokenAlpha1, "alpha1")
	_ = e2eCommon.CreateNamespace(t, headerAlpha1, e2eCommon.CreateNamespaceParams{CustomGraphAdminURLs: "", NamespaceQuant: 20})
	utilsCommon.AddData(t, 11, 20, jwtTokenAlpha1, "alpha1")
	utilsCommon.CheckDataExists(t, 20, jwtTokenAlpha1, "alpha1")
	_ = e2eCommon.CreateNamespace(t, headerAlpha1, e2eCommon.CreateNamespaceParams{CustomGraphAdminURLs: "", NamespaceQuant: 20})
	utilsCommon.AddData(t, 21, 30, jwtTokenAlpha1, "alpha1")
	utilsCommon.CheckDataExists(t, 30, jwtTokenAlpha1, "alpha1")
	_ = e2eCommon.CreateNamespace(t, headerAlpha1, e2eCommon.CreateNamespaceParams{CustomGraphAdminURLs: "", NamespaceQuant: 20})
	utilsCommon.AddData(t, 31, 40, jwtTokenAlpha1, "alpha1")
	utilsCommon.CheckDataExists(t, 40, jwtTokenAlpha1, "alpha1")
	_ = e2eCommon.CreateNamespace(t, headerAlpha1, e2eCommon.CreateNamespaceParams{CustomGraphAdminURLs: "", NamespaceQuant: 20})
	utilsCommon.AddData(t, 41, 50, jwtTokenAlpha1, "alpha1")
	utilsCommon.CheckDataExists(t, 50, jwtTokenAlpha1, "alpha1")
	_ = e2eCommon.CreateNamespace(t, headerAlpha1, e2eCommon.CreateNamespaceParams{CustomGraphAdminURLs: "", NamespaceQuant: 20})
	utilsCommon.AddData(t, 51, 60, jwtTokenAlpha1, "alpha1")
	utilsCommon.CheckDataExists(t, 60, jwtTokenAlpha1, "alpha1")
	_ = e2eCommon.CreateNamespace(t, headerAlpha1, e2eCommon.CreateNamespaceParams{CustomGraphAdminURLs: "", NamespaceQuant: 30})
	utilsCommon.AddData(t, 61, 70, jwtTokenAlpha1, "alpha1")
	utilsCommon.CheckDataExists(t, 70, jwtTokenAlpha1, "alpha1")
	utilsCommon.TakeBackup(t, jwtTokenAlpha1, backupDst, "alpha1")
	utilsCommon.RunRestore(t, jwtTokenAlpha2, restoreLocation, "alpha2")
	dg := testutil.DgClientWithLogin(t, "groot", "password", x.GalaxyNamespace)
	testutil.WaitForRestore(t, dg, testutil.ContainerAddr("alpha2", 8080))
	e2eCommon.AssertGetGQLSchema(t, testutil.ContainerAddr("alpha2", 8080), headerAlpha2)
	utilsCommon.CheckDataExists(t, 70, jwtTokenAlpha2, "alpha2")
}
