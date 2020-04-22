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
		{
			description: "test system_peers",
			method:      "system_peers",
			expected: modules.SystemPeersResponse{
				Peers: []common.PeerInfo{},
			},
		},
	}

	for _, test := range testsCases {
		t.Run(test.description, func(t *testing.T) {

			respBody, err := PostRPC(t, test.method, GOSSAMER_NODE_HOST, "{}")
			require.Nil(t, err)

			target := DecodeRPC(t, respBody, test.method)

			log.Debug("Will assert payload", "target", target)
			switch v := target.(type) {

			case modules.SystemHealthResponse:

				require.Equal(t, test.expected.(modules.SystemHealthResponse).Health.IsSyncing, v.Health.IsSyncing)
				require.Equal(t, test.expected.(modules.SystemHealthResponse).Health.ShouldHavePeers, v.Health.ShouldHavePeers)
				require.GreaterOrEqual(t, test.expected.(modules.SystemHealthResponse).Health.Peers, v.Health.Peers)

			case modules.SystemNetworkStateResponse:

				require.NotNil(t, v.NetworkState)
				require.NotNil(t, v.NetworkState.PeerID)

			case modules.SystemPeersResponse:

				require.NotNil(t, v.Peers)
				require.GreaterOrEqual(t, len(v.Peers), 2)

				for _, vv := range v.Peers {
					require.NotNil(t, vv.PeerID)
					require.NotNil(t, vv.Roles)
					require.NotNil(t, vv.ProtocolVersion)
					require.NotNil(t, vv.BestHash)
					require.NotNil(t, vv.BestNumber)
				}

			}

		})
	}

}
