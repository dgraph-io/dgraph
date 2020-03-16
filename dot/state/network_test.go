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

package state

import (
	"testing"

	"github.com/ChainSafe/gossamer/lib/common"

	"github.com/stretchr/testify/require"
)

var testHealth = common.Health{}
var testNetworkState = common.NetworkState{}
var testPeers = []common.PeerInfo{
	{
		PeerID:          "alice",
		BestHash:        common.Hash{},
		BestNumber:      1,
		ProtocolVersion: 2,
		Roles:           0x03,
	},
	{
		PeerID:          "bob",
		BestHash:        common.Hash{},
		BestNumber:      50,
		ProtocolVersion: 60,
		Roles:           0x70,
	},
}

// test state.Network
func TestNetworkState(t *testing.T) {
	ns := NewNetworkState()

	ns.SetHealth(testHealth)

	health := ns.GetHealth()
	require.NotNil(t, health)

	require.Equal(t, health, testHealth)

	ns.SetNetworkState(testNetworkState)

	networkState := ns.GetNetworkState()
	require.NotNil(t, networkState)

	require.Equal(t, networkState, testNetworkState)

	ns.SetPeers(testPeers)

	peers := ns.GetPeers()
	require.NotNil(t, peers)

	require.Equal(t, peers, testPeers)
}
