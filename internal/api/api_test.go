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
)

// -------------- Mock Apis ------------------
var (
	testPeerCount   = 1
	testVersion     = "0.0.1"
	name            = "Gossamer"
	peerID          = "Qmc85Ephxa3sR7xaTzTq2UpCJ4a4HWAfxxaV6TarXHWVVh"
	noBootstrapping = false
	peers           = []string{"QmeQeqpf3fz3CG2ckQq3CUWwUnyT2cqxJepHpjji7ehVtX"}
)

// Creating a mock peer
type MockP2pApi struct{}

func (a *MockP2pApi) PeerCount() int {
	return testPeerCount
}

func (a *MockP2pApi) Peers() []string {
	return peers
}

func (b *MockP2pApi) ID() string {
	return peerID
}

func (b *MockP2pApi) NoBootstrapping() bool {
	return noBootstrapping
}

// Creating a mock runtime API
type MockRuntimeApi struct{}

func (a *MockRuntimeApi) Name() string {
	//TODO: Replace with dynamic name
	return name
}

func (a *MockRuntimeApi) Version() string {
	return testVersion
}

// func (a *MockRuntimeApi) Chain() string {
// 	return Chain
// }

// // System properties not implemented yet
// func (b *MockRuntimeApi) properties() string {
// 	return properties
// }

// -------------------------------------------

func TestSystemModule(t *testing.T) {
	srvc := NewApiService(&MockP2pApi{}, &MockRuntimeApi{})

	// System.Name
	n := srvc.Api.RuntimeModule.Name()
	if n != name {
		t.Fatalf("System.Name - expected %+v got: %+v\n", name, n)
	}

	// System.networkState
	s := srvc.Api.P2pModule.ID()
	if s != peerID {
		t.Fatalf("System.NetworkState - expected %+v got: %+v\n", peerID, s)
	}

	// System.peers
	p := srvc.Api.P2pModule.Peers()
	if s != peerID {
		t.Fatalf("System.NetworkState - expected %+v got: %+v\n", peers, p)
	}

	// System.PeerCount
	c := srvc.Api.P2pModule.PeerCount()
	if c != testPeerCount {
		t.Fatalf("System.PeerCount - expected: %d got: %d\n", testPeerCount, c)
	}

	// System.Version
	v := srvc.Api.RuntimeModule.Version()
	if v != testVersion {
		t.Fatalf("System.Version - expected: %s got: %s\n", testVersion, v)
	}
}
