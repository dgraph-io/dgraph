// Copyright 2019 ChainSafe Systems (ON) Corp.
// This file is part of gossamer.
//
// The gossamer library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The gossamer library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the gossamer library. If not, see <http://www.gnu.org/licenses/>.

package rpc

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/ChainSafe/gossamer/dot/rpc/modules"
	"github.com/ChainSafe/gossamer/lib/common"

	log "github.com/ChainSafe/log15"
	"github.com/stretchr/testify/require"
)

var (
	GOSSAMER_INTEGRATION_TEST_MODE = os.Getenv("GOSSAMER_INTEGRATION_TEST_MODE")

	GOSSAMER_NODE_HOST = os.Getenv("GOSSAMER_NODE_HOST")

	ContentTypeJSON   = "application/json"
	dialTimeout       = 60 * time.Second
	httpClientTimeout = 120 * time.Second
)

//TODO: json2.serverResponse should be exported and re-used instead
type serverResponse struct {
	// JSON-RPC Version
	Version string `json:"jsonrpc"`
	// Resulting values
	Result json.RawMessage `json:"result"`
	// Any generated errors
	Error *Error `json:"error"`
	// Request id
	ID *json.RawMessage `json:"id"`
}

// ErrCode is a int type used for the rpc error codes
type ErrCode int

// Error is a struct that holds the error message and the error code for a error
type Error struct {
	Message   string
	ErrorCode ErrCode
}

// Error returns the error Message string
func (e *Error) Error() string {
	return e.Message
}

func TestStableRPC(t *testing.T) {
	if GOSSAMER_INTEGRATION_TEST_MODE != "stable" {
		t.Skip("Integration tests are disabled, going to skip.")
	}
	log.Info("Going to run tests",
		"GOSSAMER_INTEGRATION_TEST_MODE", GOSSAMER_INTEGRATION_TEST_MODE,
		"GOSSAMER_NODE_HOST", GOSSAMER_NODE_HOST)

	testsCases := []struct {
		description string
		method      string
		expected    interface{}
	}{
		{
			description: "test system_Health",
			method:      "system_health",
			expected: modules.SystemHealthResponse{
				Health: common.Health{
					Peers:           0,
					IsSyncing:       false,
					ShouldHavePeers: true,
				},
			},
		},
	}

	transport := &http.Transport{
		Dial: (&net.Dialer{
			Timeout: dialTimeout,
		}).Dial,
	}
	httpClient := &http.Client{
		Transport: transport,
		Timeout:   httpClientTimeout,
	}

	for _, test := range testsCases {
		t.Run(test.description, func(t *testing.T) {

			data := []byte(`{"jsonrpc":"2.0","method":"` + test.method + `","params":{},"id":1}`)
			buf := &bytes.Buffer{}
			_, err := buf.Write(data)
			require.Nil(t, err)

			r, err := http.NewRequest("POST", GOSSAMER_NODE_HOST, buf)
			require.Nil(t, err)

			r.Header.Set("Content-Type", ContentTypeJSON)
			r.Header.Set("Accept", ContentTypeJSON)

			resp, err := httpClient.Do(r)
			require.Nil(t, err)
			require.Equal(t, resp.StatusCode, http.StatusOK)

			defer func() {
				_ = resp.Body.Close()
			}()

			respBody, err := ioutil.ReadAll(resp.Body)
			require.Nil(t, err)

			decoder := json.NewDecoder(bytes.NewReader(respBody))
			decoder.DisallowUnknownFields()

			var response serverResponse
			err = decoder.Decode(&response)
			require.Nil(t, err)
			log.Debug("Got payload from RPC request", "serverResponse", response)

			require.Nil(t, response.Error)
			require.Equal(t, response.Version, "2.0")

			decoder = json.NewDecoder(bytes.NewReader(response.Result))
			decoder.DisallowUnknownFields()

			switch test.method {

			case "system_Health":
				var target modules.SystemHealthResponse
				err = decoder.Decode(&target)
				require.Nil(t, err)
				require.Equal(t, target, test.expected)
			}

		})
	}

}
