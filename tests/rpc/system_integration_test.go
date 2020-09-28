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
	"reflect"
	"strconv"
	"testing"

	"github.com/ChainSafe/gossamer/dot/rpc/modules"
	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/ChainSafe/gossamer/tests/utils"

	log "github.com/ChainSafe/log15"
	"github.com/stretchr/testify/require"
)

func TestStableNetworkRPC(t *testing.T) {
	if utils.MODE != "stable" {
		t.Skip("Integration tests are disabled, going to skip.")
	}
	log.Info("Going to run NetworkAPI tests",
		"HOSTNAME", utils.HOSTNAME,
		"PORT", utils.PORT,
	)

	networkSize, err := strconv.Atoi(utils.NETWORK_SIZE)
	if err != nil {
		networkSize = 0
	}

	testsCases := []*testCase{
		{
			description: "test system_health",
			method:      "system_health",
			expected: modules.SystemHealthResponse{
				Health: common.Health{
					Peers:           networkSize - 1,
					IsSyncing:       false,
					ShouldHavePeers: true,
				},
			},
		},
		{
			description: "test system_network_state",
			method:      "system_networkState",
			expected: modules.SystemNetworkStateResponse{
				NetworkState: modules.NetworkStateString{
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
			respBody, err := utils.PostRPC(test.method, "http://"+utils.HOSTNAME+":"+utils.PORT, "{}")
			require.Nil(t, err)

			target := reflect.New(reflect.TypeOf(test.expected)).Interface()
			err = utils.DecodeRPC(t, respBody, target)
			require.Nil(t, err)

			switch v := target.(type) {
			case *modules.SystemHealthResponse:
				t.Log("Will assert SystemHealthResponse", "target", target)

				require.Equal(t, test.expected.(modules.SystemHealthResponse).Health.IsSyncing, v.Health.IsSyncing)
				require.Equal(t, test.expected.(modules.SystemHealthResponse).Health.ShouldHavePeers, v.Health.ShouldHavePeers)
				require.GreaterOrEqual(t, v.Health.Peers, test.expected.(modules.SystemHealthResponse).Health.Peers)

			case *modules.SystemNetworkStateResponse:
				t.Log("Will assert SystemNetworkStateResponse", "target", target)

				require.NotNil(t, v.NetworkState)
				require.NotNil(t, v.NetworkState.PeerID)

			case *modules.SystemPeersResponse:
				t.Log("Will assert SystemPeersResponse", "target", target)

				require.NotNil(t, v.Peers)
				require.GreaterOrEqual(t, len(v.Peers), networkSize-2)

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
