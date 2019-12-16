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

package modules

import (
	"testing"

	"github.com/ChainSafe/gossamer/internal/api"
	module "github.com/ChainSafe/gossamer/internal/api/modules"
	"github.com/ChainSafe/gossamer/p2p"
)

var (
	testRuntimeChain      = "Chain"
	testRuntimeName       = "Gossamer"
	testRuntimeProperties = "Properties"
	testRuntimeVersion    = "0.0.1"
	testHealth            = p2p.Health{}
	testNetworkState      = p2p.NetworkState{}
	testPeers             = append([]p2p.PeerInfo{}, p2p.PeerInfo{})
)

// Mock runtime API
type MockRuntimeApi struct{}

func (r *MockRuntimeApi) Chain() string {
	return testRuntimeChain
}

func (r *MockRuntimeApi) Name() string {
	return testRuntimeName
}

func (r *MockRuntimeApi) Properties() string {
	return testRuntimeProperties
}

func (r *MockRuntimeApi) Version() string {
	return testRuntimeVersion
}

// Mock network API
type MockP2pApi struct{}

func (n *MockP2pApi) Health() p2p.Health {
	return testHealth
}

func (n *MockP2pApi) NetworkState() p2p.NetworkState {
	return testNetworkState
}

func (n *MockP2pApi) Peers() []p2p.PeerInfo {
	return testPeers
}

func newMockApi() *api.Api {
	p2pApi := &MockP2pApi{}
	runtimeApi := &MockRuntimeApi{}

	return &api.Api{
		P2pModule:     module.NewP2pModule(p2pApi),
		RuntimeModule: module.NewRuntimeModule(runtimeApi),
	}
}

// Test RPC's System.Health() response
func TestSystemModule_Health(t *testing.T) {
	sys := NewSystemModule(newMockApi())

	res := &SystemHealthResponse{}
	sys.Health(nil, nil, res)

	if res.Health != testHealth {
		t.Errorf("System.Health.: expected: %+v got: %+v\n", testHealth, res.Health)
	}
}

// Test RPC's System.NetworkState() response
func TestSystemModule_NetworkState(t *testing.T) {
	sys := NewSystemModule(newMockApi())

	res := &SystemNetworkStateResponse{}
	sys.NetworkState(nil, nil, res)

	if res.NetworkState != testNetworkState {
		t.Errorf("System.NetworkState: expected: %+v got: %+v\n", testNetworkState, res.NetworkState)
	}
}

// Test RPC's System.Peers() response
func TestSystemModule_Peers(t *testing.T) {
	sys := NewSystemModule(newMockApi())

	res := &SystemPeersResponse{}
	sys.Peers(nil, nil, res)

	if len(res.Peers) != len(testPeers) {
		t.Errorf("System.Peers: expected: %+v got: %+v\n", testPeers, res.Peers)
	}
}
