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

package api

import (
	"testing"

	"github.com/ChainSafe/gossamer/p2p"
)

var (
	testChain        = "Chain"
	testName         = "Gossamer"
	testProperties   = "Properties"
	testVersion      = "0.0.1"
	testHealth       = p2p.Health{}
	testNetworkState = p2p.NetworkState{}
	testPeers        = append([]p2p.PeerInfo{}, p2p.PeerInfo{})
)

// Mock RuntimeApi
type MockRuntimeApi struct{}

func (r *MockRuntimeApi) Chain() string {
	return testChain
}

func (r *MockRuntimeApi) Name() string {
	return testName
}

func (r *MockRuntimeApi) Properties() string {
	return testProperties
}

func (r *MockRuntimeApi) Version() string {
	return testVersion
}

// Mock P2pApi
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

func TestSystemModule(t *testing.T) {
	s := NewApiService(&MockP2pApi{}, &MockRuntimeApi{})

	// Runtime Module

	// System.Chain
	chain := s.Api.RuntimeModule.Chain()
	if chain != testChain {
		t.Fatalf("System.Chain - expected %+v got: %+v\n", testChain, chain)
	}

	// System.Name
	name := s.Api.RuntimeModule.Name()
	if name != testName {
		t.Fatalf("System.Name - expected %+v got: %+v\n", testName, name)
	}

	// System.Properties
	properties := s.Api.RuntimeModule.Properties()
	if properties != testProperties {
		t.Fatalf("System.Properties - expected: %s got: %s\n", testProperties, properties)
	}

	// System.Version
	version := s.Api.RuntimeModule.Version()
	if version != testVersion {
		t.Fatalf("System.Version - expected: %s got: %s\n", testVersion, version)
	}

	// Network Module

	// System.Health
	health := s.Api.P2pModule.Health()
	if health != testHealth {
		t.Fatalf("System.Health - expected %+v got: %+v\n", testHealth, health)
	}

	// System.NetworkState
	networkState := s.Api.P2pModule.NetworkState()
	if networkState != testNetworkState {
		t.Fatalf("System.NetworkState - expected %+v got: %+v\n", testNetworkState, networkState)
	}

	// System.Peers
	peers := s.Api.P2pModule.Peers()
	if len(peers) != len(testPeers) {
		t.Fatalf("System.Peers - expected %+v got: %+v\n", testPeers, peers)
	}
}
