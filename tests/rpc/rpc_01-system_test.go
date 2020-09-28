// Copyright 2020 ChainSafe Systems (ON) Corp.
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
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/ChainSafe/gossamer/dot/rpc/modules"
	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/ChainSafe/gossamer/tests/utils"
	"github.com/stretchr/testify/require"
)

func TestSystemRPC(t *testing.T) {
	if utils.MODE != rpcSuite {
		_, _ = fmt.Fprintln(os.Stdout, "Going to skip RPC suite tests")
		return
	}

	testCases := []*testCase{
		{ //TODO
			description: "test system_name",
			method:      "system_name",
			skip:        true,
		},
		{ //TODO
			description: "test system_version",
			method:      "system_version",
			skip:        true,
		},
		{ //TODO
			description: "test system_chain",
			method:      "system_chain",
			skip:        true,
		},
		{ //TODO
			description: "test system_properties",
			method:      "system_properties",
			skip:        true,
		},
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
			params: "{}",
		},
		{
			description: "test system_peers",
			method:      "system_peers",
			expected: modules.SystemPeersResponse{
				Peers: []common.PeerInfo{},
			},
			params: "{}",
		},
		{
			description: "test system_network_state",
			method:      "system_networkState",
			expected: modules.SystemNetworkStateResponse{
				NetworkState: modules.NetworkStateString{
					PeerID: "",
				},
			},
			params: "{}",
		},
		{ //TODO
			description: "test system_addReservedPeer",
			method:      "system_addReservedPeer",
			skip:        true,
		},
		{ //TODO
			description: "test system_removeReservedPeer",
			method:      "system_removeReservedPeer",
			skip:        true,
		},
		{ //TODO
			description: "test system_nodeRoles",
			method:      "system_nodeRoles",
			skip:        true,
		},
		{ //TODO
			description: "test system_accountNextIndex",
			method:      "system_accountNextIndex",
			skip:        true,
		},
	}

	t.Log("starting gossamer...")
	nodes, err := utils.InitializeAndStartNodes(t, 3, utils.GenesisDefault, utils.ConfigDefault)

	//use only first server for tests
	require.Nil(t, err)

	time.Sleep(time.Second) // give server a second to start

	for _, test := range testCases {
		t.Run(test.description, func(t *testing.T) {
			target := getResponse(t, test)

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

				//TODO: #807
				//this assertion requires more time on init to be enabled
				//require.GreaterOrEqual(t, len(v.Peers), 2)

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

	t.Log("going to tear down gossamer...")

	errList := utils.TearDown(t, nodes)
	require.Len(t, errList, 0)
}
