package certrequireandverify

import (
	"context"
	"testing"

	"github.com/dgraph-io/dgo/protos/api"
	"github.com/dgraph-io/dgraph/z"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func TestAccessWithoutClientCert(t *testing.T) {
	conf := viper.New()
	conf.Set("tls_cacert", "../tls/ca.crt")
	conf.Set("tls_server_name", "node")

	dg, err := z.DgraphClientWithCerts(z.SockAddr, conf)
	require.NoError(t, err, "Unable to get dgraph client: %v", err)
	err = dg.Alter(context.Background(), &api.Operation{DropAll: true})
	require.Error(t, err, "The authentication handshake should have failed")
}

func TestAccessWithClientCert(t *testing.T) {
	conf := viper.New()
	conf.Set("tls_cacert", "../tls/ca.crt")
	conf.Set("tls_server_name", "node")
	conf.Set("tls_cert", "../tls/client.acl.crt")
	conf.Set("tls_key", "../tls/client.acl.key")

	dg, err := z.DgraphClientWithCerts(z.SockAddr, conf)
	require.NoError(t, err, "Unable to get dgraph client: %v", err)
	err = dg.Alter(context.Background(), &api.Operation{DropAll: true})
	require.NoError(t, err, "Unable to perform dropall: %v", err)
}

func TestCurlAccessWithoutClientCert(t *testing.T) {
	curlArgs := []string{
		"--cacert", "../tls/ca.crt", "https://localhost:8180/alter",
		"-d", "name: string @index(exact) .",
	}
	z.VerifyCurlCmd(t, curlArgs, &z.FailureConfig{
		ShouldFail: true,
		CurlErrMsg: "alert bad certificate",
	})
}

func TestCurlAccessWithClientCert(t *testing.T) {
	curlArgs := []string{
		"--cacert", "../tls/ca.crt",
		"--cert", "../tls/client.acl.crt",
		"--key", "../tls/client.acl.key",
		"https://localhost:8180/alter",
		"-d", "name: string @index(exact) .",
	}
	z.VerifyCurlCmd(t, curlArgs, &z.FailureConfig{
		ShouldFail: false,
	})
}
