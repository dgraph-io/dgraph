package no_tls

import (
	"github.com/stretchr/testify/require"
	"io/ioutil"
	"net/http"
	"strings"
	"testing"
	"time"
)

type testCase struct {
	url        string
	statusCode int
	response   string
}

var testCasesHttp = []testCase{
	{
		url:        "http://localhost:6180/health",
		response:   "OK",
		statusCode: 200,
	},
	{
		url:        "http://localhost:6180/state",
		response:   "\"id\":\"1\",\"groupId\":0,\"addr\":\"zero1:5180\",\"leader\":true,\"amDead\":false",
		statusCode: 200,
	},
}

func TestZeroWithNoTLS(t *testing.T) {
	client := http.Client{
		Timeout: time.Second * 10,
	}
	defer client.CloseIdleConnections()
	for _, test := range testCasesHttp {
		request, err := http.NewRequest("GET", test.url, nil)
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
