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
	"testing"

	"github.com/ChainSafe/gossamer/dot/rpc/modules"
	"github.com/ChainSafe/gossamer/lib/common"

	log "github.com/ChainSafe/log15"
	"github.com/stretchr/testify/require"
)

func TestStableNetworkRPC(t *testing.T) {
	if GOSSAMER_INTEGRATION_TEST_MODE != "stable" {
		t.Skip("Integration tests are disabled, going to skip.")
	}
	log.Info("Going to run NetworkAPI tests",
		"GOSSAMER_INTEGRATION_TEST_MODE", GOSSAMER_INTEGRATION_TEST_MODE,
		"GOSSAMER_NODE_HOST", GOSSAMER_NODE_HOST)

	testsCases := []struct {
		description string
		method      string
		expected    interface{}
	}{
		{
			description: "test system_health",
			method:      "system_health",
			expected: modules.SystemHealthResponse{
				Health: common.Health{
					Peers:           2,
					IsSyncing:       false,
					ShouldHavePeers: true,
				},
			},
		},
		{
			description: "test system_network_state",
			method:      "system_networkState",
			expected: modules.SystemNetworkStateResponse{
				NetworkState: common.NetworkState{
					PeerID: "",
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
			require.Nil(t, err, "respBody", string(respBody))
			log.Debug("Got payload from RPC request", "serverResponse", response, "string(respBody)", string(respBody))

			require.Nil(t, response.Error)
			require.Equal(t, response.Version, "2.0")

			decoder = json.NewDecoder(bytes.NewReader(response.Result))
			decoder.DisallowUnknownFields()

			switch test.method {

			case "system_health":
				var target modules.SystemHealthResponse
				err = decoder.Decode(&target)
				require.Nil(t, err)

				log.Debug("Will assert payload", "target", target)

				require.Equal(t, test.expected.(modules.SystemHealthResponse).Health.IsSyncing, target.Health.IsSyncing)
				require.Equal(t, test.expected.(modules.SystemHealthResponse).Health.ShouldHavePeers, target.Health.ShouldHavePeers)

				require.GreaterOrEqual(t, test.expected.(modules.SystemHealthResponse).Health.Peers, target.Health.Peers)
			case "system_networkState":
				var target modules.SystemNetworkStateResponse
				err = decoder.Decode(&target)
				require.Nil(t, err)

				log.Debug("Will assert payload", "target", target)

				require.NotNil(t, target.NetworkState)
				require.NotNil(t, target.NetworkState.PeerID)
			}

		})
	}

}
