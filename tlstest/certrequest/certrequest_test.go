package certrequest

import (
	"context"
	"fmt"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgo/v210/protos/api"
	"github.com/dgraph-io/dgraph/testutil"
)

func TestAccessOverPlaintext(t *testing.T) {
	dg, err := testutil.DgraphClient(testutil.SockAddr)
	if err != nil {
		t.Fatalf("Error while getting a dgraph client: %v", err)
	}
	err = dg.Alter(context.Background(), &api.Operation{DropAll: true})
	require.Error(t, err, "The authentication handshake should have failed")
}

func TestAccessWithCaCert(t *testing.T) {
	conf := viper.New()
	conf.Set("tls", fmt.Sprintf("ca-cert=%s; server-name=%s;",
		// ca-cert
		"../tls/ca.crt",
		// server-name
		"node"))

	dg, err := testutil.DgraphClientWithCerts(testutil.SockAddr, conf)
	require.NoError(t, err, "Unable to get dgraph client: %v", err)
	err = dg.Alter(context.Background(), &api.Operation{DropAll: true})
	require.NoError(t, err, "Unable to perform dropall: %v", err)
}

func TestCurlAccessWithCaCert(t *testing.T) {
	// curl over plaintext should fail
	curlPlainTextArgs := []string{
		"https://" + testutil.SockAddrHttp + "/alter",
		"-d", "name: string @index(exact) .",
	}
	testutil.VerifyCurlCmd(t, curlPlainTextArgs, &testutil.CurlFailureConfig{
		ShouldFail: true,
		CurlErrMsg: "SSL certificate problem",
	})

	curlArgs := []string{
		"--cacert", "../tls/ca.crt", "https://" + testutil.SockAddrHttp + "/alter",
		"-d", "name: string @index(exact) .",
	}
	testutil.VerifyCurlCmd(t, curlArgs, &testutil.CurlFailureConfig{
		ShouldFail: false,
	})
}
