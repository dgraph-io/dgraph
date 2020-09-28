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
	"math/big"
	"os"
	"path"
	"testing"

	"github.com/ChainSafe/gossamer/dot/network"
	"github.com/ChainSafe/gossamer/dot/state"
	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/stretchr/testify/require"
)

var (
	testHealth = common.Health{
		Peers:           0,
		IsSyncing:       false,
		ShouldHavePeers: true,
	}
	testPeers = []common.PeerInfo{}
)

type mockSyncer struct{}

func (s *mockSyncer) CreateBlockResponse(msg *network.BlockRequestMessage) (*network.BlockResponseMessage, error) {
	return nil, nil
}

func (s *mockSyncer) HandleBlockResponse(msg *network.BlockResponseMessage) *network.BlockRequestMessage {
	return nil
}

func (s *mockSyncer) HandleBlockAnnounce(msg *network.BlockAnnounceMessage) *network.BlockRequestMessage {
	return nil
}

func (s *mockSyncer) HandleSeenBlocks(num *big.Int) *network.BlockRequestMessage {
	return nil
}

func newNetworkService(t *testing.T) *network.Service {
	testDir := path.Join(os.TempDir(), "test_data")

	cfg := &network.Config{
		NoStatus:     true,
		NetworkState: &state.NetworkState{},
		BasePath:     testDir,
		Syncer:       &mockSyncer{},
	}

	srv, err := network.NewService(cfg)
	if err != nil {
		t.Fatal(err)
	}

	return srv
}

// Test RPC's System.Health() response
func TestSystemModule_Health(t *testing.T) {
	net := newNetworkService(t)
	sys := NewSystemModule(net, nil)

	res := &SystemHealthResponse{}
	err := sys.Health(nil, nil, res)
	require.NoError(t, err)

	if res.Health != testHealth {
		t.Errorf("System.Health.: expected: %+v got: %+v\n", testHealth, res.Health)
	}
}

// Test RPC's System.NetworkState() response
func TestSystemModule_NetworkState(t *testing.T) {
	net := newNetworkService(t)
	sys := NewSystemModule(net, nil)

	res := &SystemNetworkStateResponse{}
	err := sys.NetworkState(nil, nil, res)
	require.NoError(t, err)

	testNetworkState := net.NetworkState()

	if res.NetworkState.PeerID != testNetworkState.PeerID {
		t.Errorf("System.NetworkState: expected: %+v got: %+v\n", testNetworkState, res.NetworkState)
	}
}

// Test RPC's System.Peers() response
func TestSystemModule_Peers(t *testing.T) {
	net := newNetworkService(t)
	sys := NewSystemModule(net, nil)

	res := &SystemPeersResponse{}
	err := sys.Peers(nil, nil, res)
	require.NoError(t, err)

	if len(res.Peers) != len(testPeers) {
		t.Errorf("System.Peers: expected: %+v got: %+v\n", testPeers, res.Peers)
	}
}

func TestSystemModule_NodeRoles(t *testing.T) {
	net := newNetworkService(t)
	sys := NewSystemModule(net, nil)
	expected := []interface{}{"Full"}

	var res []interface{}
	err := sys.NodeRoles(nil, nil, &res)
	require.NoError(t, err)
	require.Equal(t, expected, res)
}
