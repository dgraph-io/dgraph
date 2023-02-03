package all_routes_tls

import (
	"crypto/tls"
	"crypto/x509"
	"io/ioutil"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/testutil"
)

type testCase struct {
	url        string
	statusCode int
	response   string
}

var testCasesHttp = []testCase{
	{
		url:        "/health",
		response:   "OK",
		statusCode: 200,
	},
	{
		url:        "/state",
		response:   "Client sent an HTTP request to an HTTPS server.\n",
		statusCode: 400,
	},
	{
		url:        "/removeNode?id=2&group=0",
		response:   "Client sent an HTTP request to an HTTPS server.\n",
		statusCode: 400,
	},
}

func TestZeroWithAllRoutesTLSWithHTTPClient(t *testing.T) {
	client := http.Client{
		Timeout: time.Second * 10,
	}
	defer client.CloseIdleConnections()
	for _, test := range testCasesHttp {
		request, err := http.NewRequest("GET", "http://"+testutil.SockAddrZeroHttp+test.url, nil)
		require.NoError(t, err)
		do, err := client.Do(request)
		require.NoError(t, err)
		if do != nil && do.StatusCode != test.statusCode {
			t.Fatalf("status code is not same. Got: %d Expected: %d", do.StatusCode, test.statusCode)
		}

		body := readResponseBody(t, do)
		if test.response != string(body) {
			t.Fatalf("response is not same. Got: %s Expected: %s", string(body), test.response)
		}
	}
}

var testCasesHttps = []testCase{
	{
		url:        "/health",
		response:   "OK",
		statusCode: 200,
	},
	{
		url:        "/state",
		response:   "\"id\":\"1\",\"groupId\":0,\"addr\":\"zero1:5080\",\"leader\":true,\"amDead\":false",
		statusCode: 200,
	},
}

func TestZeroWithAllRoutesTLSWithTLSClient(t *testing.T) {
	pool, err := generateCertPool("../../tls/ca.crt", true)
	require.NoError(t, err)

	tlsCfg := &tls.Config{RootCAs: pool, ServerName: "localhost", InsecureSkipVerify: true}
	tr := &http.Transport{
		IdleConnTimeout:    30 * time.Second,
		DisableCompression: true,
		TLSClientConfig:    tlsCfg,
	}
	client := http.Client{
		Transport: tr,
	}

	defer client.CloseIdleConnections()
	for _, test := range testCasesHttps {
		request, err := http.NewRequest("GET", "https://"+testutil.SockAddrZeroHttp+test.url, nil)
		require.NoError(t, err)
		do, err := client.Do(request)
		require.NoError(t, err)
		if do != nil && do.StatusCode != test.statusCode {
			t.Fatalf("status code is not same. Got: %d Expected: %d", do.StatusCode, test.statusCode)
		}

		body := readResponseBody(t, do)
		if !strings.Contains(string(body), test.response) {
			t.Fatalf("response is not same. Got: %s Expected: %s", string(body), test.response)
		}
	}
}

func readResponseBody(t *testing.T, do *http.Response) []byte {
	defer func() { _ = do.Body.Close() }()
	body, err := ioutil.ReadAll(do.Body)
	require.NoError(t, err)
	return body
}

func generateCertPool(certPath string, useSystemCA bool) (*x509.CertPool, error) {
	var pool *x509.CertPool
	if useSystemCA {
		var err error
		if pool, err = x509.SystemCertPool(); err != nil {
			return nil, err
		}
	} else {
		pool = x509.NewCertPool()
	}

	if len(certPath) > 0 {
		caFile, err := ioutil.ReadFile(certPath)
		if err != nil {
			return nil, err
		}
		if !pool.AppendCertsFromPEM(caFile) {
			return nil, errors.Errorf("error reading CA file %q", certPath)
		}
	}

	return pool, nil
}
