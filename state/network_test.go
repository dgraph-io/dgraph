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

	"github.com/ChainSafe/gossamer/p2p"
)

var testHealth = p2p.Health{}
var testNetworkState = p2p.NetworkState{}
var testPeers = []p2p.PeerInfo{}

// test state.Network
func TestNetworkState(t *testing.T) {

	state := newTestService(t)

	// test state.Network.Health()
	health := state.Network.Health()
	if health != testHealth {
		t.Errorf("System.Health - expected %+v got: %+v\n", testHealth, health)
	}

	// test state.Network.NetworkState()
	networkState := state.Network.NetworkState()
	if networkState != testNetworkState {
		t.Errorf("System.NetworkState - expected %+v got: %+v\n", testNetworkState, networkState)
	}

	// test state.Network.Peers()
	peers := state.Network.Peers()
	if len(peers) != len(testPeers) {
		t.Errorf("System.Peers - expected %+v got: %+v\n", testPeers, peers)
	}
}
