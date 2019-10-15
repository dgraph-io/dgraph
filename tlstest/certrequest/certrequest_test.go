package certrequest

import (
	"context"
	"testing"

	"github.com/dgraph-io/dgo/v2/protos/api"
	"github.com/dgraph-io/dgraph/testutil"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func TestAccessOverPlaintext(t *testing.T) {
	dg := testutil.DgraphClient(testutil.SockAddr)
	err := dg.Alter(context.Background(), &api.Operation{DropAll: true})
	require.Error(t, err, "The authentication handshake should have failed")
}

func TestAccessWithCaCert(t *testing.T) {
	conf := viper.New()
	conf.Set("tls_cacert", "../tls/ca.crt")
	conf.Set("tls_server_name", "node")

	dg, err := testutil.DgraphClientWithCerts(testutil.SockAddr, conf)
	require.NoError(t, err, "Unable to get dgraph client: %v", err)
	err = dg.Alter(context.Background(), &api.Operation{DropAll: true})
	require.NoError(t, err, "Unable to perform dropall: %v", err)
}

func TestCurlAccessWithCaCert(t *testing.T) {
	// curl over plaintext should fail
	curlPlainTextArgs := []string{
		"https://localhost:8180/alter",
		"-d", "name: string @index(exact) .",
	}
	testutil.VerifyCurlCmd(t, curlPlainTextArgs, &testutil.CurlFailureConfig{
		ShouldFail: true,
		CurlErrMsg: "SSL certificate problem",
	})

	curlArgs := []string{
		"--cacert", "../tls/ca.crt", "https://localhost:8180/alter",
		"-d", "name: string @index(exact) .",
	}
	testutil.VerifyCurlCmd(t, curlArgs, &testutil.CurlFailureConfig{
		ShouldFail: false,
	})
}
