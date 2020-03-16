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
	"github.com/ChainSafe/gossamer/lib/common"
)

// NetworkState defines fields for manipulating the state of network
type NetworkState struct {
	Health       common.Health
	NetworkState common.NetworkState
	Peers        []common.PeerInfo
}

// NewNetworkState creates NetworkState
func NewNetworkState() *NetworkState {
	return &NetworkState{
		Health:       common.Health{},
		NetworkState: common.NetworkState{},
		Peers:        []common.PeerInfo{},
	}
}

// GetHealth retrieves network health from the database
func (ns *NetworkState) GetHealth() common.Health {
	return ns.Health
}

// SetHealth sets network health in the database
func (ns *NetworkState) SetHealth(health common.Health) {
	ns.Health = health
}

// GetNetworkState retrieves network state from the database
func (ns *NetworkState) GetNetworkState() common.NetworkState {
	return ns.NetworkState
}

// SetNetworkState sets network state in the database
func (ns *NetworkState) SetNetworkState(networkState common.NetworkState) {
	ns.NetworkState = networkState
}

// GetPeers retrieves network state from the database
func (ns *NetworkState) GetPeers() []common.PeerInfo {
	return ns.Peers
}

// SetPeers sets network state in the database
func (ns *NetworkState) SetPeers(peers []common.PeerInfo) {
	ns.Peers = peers
}
