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

	"github.com/ChainSafe/gossamer/common"
)

var (
	testChain        = "Chain"
	testName         = "Gossamer"
	testProperties   = "Properties"
	testVersion      = "0.0.1"
	testHealth       = common.Health{}
	testNetworkState = common.NetworkState{}
	testPeers        = append([]common.PeerInfo{}, common.PeerInfo{})
)

// MockRuntimeAPI is the Mock for RuntimeAPI
type MockRuntimeAPI struct{}

// Chain is the Mock for Chain
func (r *MockRuntimeAPI) Chain() string {
	return testChain
}

// Name is the Mock for Name
func (r *MockRuntimeAPI) Name() string {
	return testName
}

// Properties is the Mock for Properties
func (r *MockRuntimeAPI) Properties() string {
	return testProperties
}

// Version is the Mock for Version
func (r *MockRuntimeAPI) Version() string {
	return testVersion
}

// MockP2pAPI Mock P2pAPI
type MockP2pAPI struct{}

func (n *MockP2pAPI) Health() common.Health {
	return testHealth
}

func (n *MockP2pAPI) NetworkState() common.NetworkState {
	return testNetworkState
}

func (n *MockP2pAPI) Peers() []common.PeerInfo {
	return testPeers
}

func TestSystemModule(t *testing.T) {
	s := NewAPIService(&MockP2pAPI{}, &MockRuntimeAPI{})

	// Runtime Module

	// System.Chain
	chain := s.API.RuntimeModule.Chain()
	if chain != testChain {
		t.Fatalf("System.Chain - expected %+v got: %+v\n", testChain, chain)
	}

	// System.Name
	name := s.API.RuntimeModule.Name()
	if name != testName {
		t.Fatalf("System.Name - expected %+v got: %+v\n", testName, name)
	}

	// System.Properties
	properties := s.API.RuntimeModule.Properties()
	if properties != testProperties {
		t.Fatalf("System.Properties - expected: %s got: %s\n", testProperties, properties)
	}

	// System.Version
	version := s.API.RuntimeModule.Version()
	if version != testVersion {
		t.Fatalf("System.Version - expected: %s got: %s\n", testVersion, version)
	}

	// Network Module

	// System.Health
	health := s.API.P2pModule.Health()
	if health != testHealth {
		t.Fatalf("System.Health - expected %+v got: %+v\n", testHealth, health)
	}

	// System.NetworkState
	networkState := s.API.P2pModule.NetworkState()
	if networkState != testNetworkState {
		t.Fatalf("System.NetworkState - expected %+v got: %+v\n", testNetworkState, networkState)
	}

	// System.Peers
	peers := s.API.P2pModule.Peers()
	if len(peers) != len(testPeers) {
		t.Fatalf("System.Peers - expected %+v got: %+v\n", testPeers, peers)
	}
}
