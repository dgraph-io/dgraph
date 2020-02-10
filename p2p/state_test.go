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

package p2p

import (
	"math/big"

	"github.com/ChainSafe/gossamer/common"
	"github.com/ChainSafe/gossamer/core/types"
)

// MockBlockState
type MockBlockState struct{}

// AddBlock for MockBlockState
func (mbs *MockBlockState) AddBlock(*types.Block) error {
	return nil
}

// SetBlock for MockBlockState
func (mbs *MockBlockState) SetBlock(*types.Block) error {
	return nil
}

// LatestHeader for MockBlockState
func (mbs *MockBlockState) LatestHeader() *types.Header {
	parentHash, err := common.HexToHash("0x4545454545454545454545454545454545454545454545454545454545454545")
	if err != nil {
		return nil
	}
	stateRoot, err := common.HexToHash("0xb3266de137d20a5d0ff3a6401eb57127525fd9b2693701f0bf5a8a853fa3ebe0")
	if err != nil {
		return nil
	}
	extrinsicsRoot, err := common.HexToHash("0x03170a2e7597b7b7e3d84c05391d139a62b157e78786d8c082f29dcf4c111314")
	if err != nil {
		return nil
	}

	return &types.Header{
		ParentHash:     parentHash,
		Number:         big.NewInt(1),
		StateRoot:      stateRoot,
		ExtrinsicsRoot: extrinsicsRoot,
		Digest:         [][]byte{{}},
	}
}

// MockNetworkState
type MockNetworkState struct{}

// GetHealth for MockNetworkState
func (mbs *MockNetworkState) GetHealth() (*common.Health, error) {
	return &common.Health{}, nil
}

// GetNetworkState for MockNetworkState
func (mbs *MockNetworkState) GetNetworkState() (*common.NetworkState, error) {
	return &common.NetworkState{}, nil
}

// GetPeers for MockNetworkState
func (mbs *MockNetworkState) GetPeers() (*[]common.PeerInfo, error) {
	return &[]common.PeerInfo{}, nil
}

// SetHealth for MockNetworkState
func (mbs *MockNetworkState) SetHealth(val *common.Health) error {
	return nil
}

// SetNetworkState for MockNetworkState
func (mbs *MockNetworkState) SetNetworkState(val *common.NetworkState) error {
	return nil
}

// SetPeers for MockNetworkState
func (mbs *MockNetworkState) SetPeers(val *[]common.PeerInfo) error {
	return nil
}

// MockStorageState
type MockStorageState struct{}

// StorageRoot for MockStorageState
func (mbs *MockStorageState) StorageRoot() (common.Hash, error) {
	return common.Hash{}, nil
}
