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

package module

import (
	"github.com/ChainSafe/gossamer/common"
	log "github.com/ChainSafe/log15"
)

// NetworkModule holds the fields for manipulating the API
type NetworkModule struct {
	NetworkAPI NetworkAPI
}

// NetworkAPI is the interface for the network package
type NetworkAPI interface {
	Health() common.Health
	NetworkState() common.NetworkState
	Peers() []common.PeerInfo
}

// NewNetworkModule implements NetworkAPI
func NewNetworkModule(networkAPI NetworkAPI) *NetworkModule {
	return &NetworkModule{networkAPI}
}

// Health returns network service Health()
func (m *NetworkModule) Health() common.Health {
	log.Debug("[rpc] Executing System.Health", "params", nil)
	return m.NetworkAPI.Health()
}

// NetworkState returns network service NetworkState()
func (m *NetworkModule) NetworkState() common.NetworkState {
	log.Debug("[rpc] Executing System.NetworkState", "params", nil)
	return m.NetworkAPI.NetworkState()
}

// Peers returns network service Peers()
func (m *NetworkModule) Peers() []common.PeerInfo {
	log.Debug("[rpc] Executing System.Peers", "params", nil)
	return m.NetworkAPI.Peers()
}
