package certrequireandverify

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"net/http"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgo/v210/protos/api"
	"github.com/dgraph-io/dgraph/testutil"
)

func TestAccessWithoutClientCert(t *testing.T) {
	conf := viper.New()
	conf.Set("tls", fmt.Sprintf("ca-cert=%s; server-name=%s;",
		// ca-cert
		"../tls/ca.crt",
		// server-name
		"node"))

	dg, err := testutil.DgraphClientWithCerts(testutil.SockAddr, conf)
	require.NoError(t, err, "Unable to get dgraph client: %v", err)
	err = dg.Alter(context.Background(), &api.Operation{DropAll: true})
	require.Error(t, err, "The authentication handshake should have failed")
}

func TestAccessWithClientCert(t *testing.T) {
	conf := viper.New()
	conf.Set("tls", fmt.Sprintf("ca-cert=%s; server-name=%s; client-cert=%s; client-key=%s;",
		// ca-cert
		"../tls/ca.crt",
		// server-name
		"node",
		// client-cert
		"../tls/client.acl.crt",
		// client-key
		"../tls/client.acl.key"))

	dg, err := testutil.DgraphClientWithCerts(testutil.SockAddr, conf)
	require.NoError(t, err, "Unable to get dgraph client: %v", err)
	err = dg.Alter(context.Background(), &api.Operation{DropAll: true})
	require.NoError(t, err, "Unable to perform dropall: %v", err)
}

func TestCurlAccessWithoutClientCert(t *testing.T) {
	curlArgs := []string{
		"--cacert", "../tls/ca.crt", "https://" + testutil.SockAddrHttp + "/alter",
		"-d", "name: string @index(exact) .",
	}
	testutil.VerifyCurlCmd(t, curlArgs, &testutil.CurlFailureConfig{
		ShouldFail: true,
		CurlErrMsg: "alert bad certificate",
	})
}

func TestCurlAccessWithClientCert(t *testing.T) {
	curlArgs := []string{
		"--cacert", "../tls/ca.crt",
		"--cert", "../tls/client.acl.crt",
		"--key", "../tls/client.acl.key",
		"https://" + testutil.SockAddrHttp + "/alter",
		"-d", "name: string @index(exact) .",
	}
	testutil.VerifyCurlCmd(t, curlArgs, &testutil.CurlFailureConfig{
		ShouldFail: false,
	})
}

func TestGQLAdminHealthWithClientCert(t *testing.T) {
	// Read the root cert file.
	caCert, err := ioutil.ReadFile("../tls/ca.crt")
	require.NoError(t, err, "Unable to read root cert file : %v", err)
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(caCert)

	// Load the client cert file.
	clientCert, err := tls.LoadX509KeyPair("../tls/client.acl.crt", "../tls/client.acl.key")
	require.NoError(t, err, "Unable to read client cert file : %v", err)
	tlsConfig := tls.Config{
		RootCAs:      pool,
		Certificates: []tls.Certificate{clientCert},
	}
	transport := http.Transport{
		TLSClientConfig: &tlsConfig,
	}
	client := http.Client{
		Timeout:   time.Second * 10,
		Transport: &transport,
	}

	healthCheckQuery := []byte(`{"query":"query {\n health {\n status\n }\n}"}`)
	gqlAdminEndpoint := "https://" + testutil.SockAddrHttp + "/admin"
	req, err := http.NewRequest("POST", gqlAdminEndpoint, bytes.NewBuffer(healthCheckQuery))
	require.NoError(t, err, "Failed to create request : %v", err)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	require.NoError(t, err, "Https request failed: %v", err)
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	require.NoError(t, err, "Error while reading http response: %v", err)
	require.Contains(t, string(body), `"status":"healthy"`)
}

func TestGQLAdminHealthWithoutClientCert(t *testing.T) {
	// Read the root cert file.
	caCert, err := ioutil.ReadFile("../tls/ca.crt")
	require.NoError(t, err, "Unable to read root cert file : %v", err)
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(caCert)

	tlsConfig := tls.Config{
		RootCAs: pool,
	}
	transport := http.Transport{
		TLSClientConfig: &tlsConfig,
	}
	client := http.Client{
		Timeout:   time.Second * 10,
		Transport: &transport,
	}

	healthCheckQuery := []byte(`{"query":"query {\n health {\n message\n status\n }\n}"}`)
	gqlAdminEndpoint := "https://" + testutil.SockAddrHttp + "/admin"
	req, err := http.NewRequest("POST", gqlAdminEndpoint, bytes.NewBuffer(healthCheckQuery))
	require.NoError(t, err, "Failed to create request : %v", err)
	req.Header.Set("Content-Type", "application/json")

	_, err = client.Do(req)
	require.Contains(t, err.Error(), "remote error: tls: bad certificate")
}
