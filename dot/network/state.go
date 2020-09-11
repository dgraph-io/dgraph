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

package network

import (
	"math/big"

	"github.com/ChainSafe/gossamer/dot/types"
	"github.com/ChainSafe/gossamer/lib/common"
)

// BlockState interface for block state methods
type BlockState interface {
	BestBlockHeader() (*types.Header, error)
	GenesisHash() common.Hash
}

// NetworkState interface for network state methods
//nolint:golint
type NetworkState interface {
	SetHealth(common.Health)
	SetNetworkState(common.NetworkState)
	SetPeers([]common.PeerInfo)
}

// MessageHandler interface for handling message passing
type MessageHandler interface {
	HandleMessage(Message)
}

// MockNetworkState for testing purposes
type MockNetworkState struct {
	Health       common.Health
	NetworkState common.NetworkState
	Peers        []common.PeerInfo
}

// SetHealth sets network health in the database
func (ns *MockNetworkState) SetHealth(health common.Health) {
	ns.Health = health
}

// SetNetworkState sets network state in the database
func (ns *MockNetworkState) SetNetworkState(networkState common.NetworkState) {
	ns.NetworkState = networkState
}

// SetPeers sets network state in the database
func (ns *MockNetworkState) SetPeers(peers []common.PeerInfo) {
	ns.Peers = peers
}

// Syncer is implemented by the syncing service
type Syncer interface {
	// CreateBlockResponse is called upon receipt of a BlockRequestMessage to create the response
	CreateBlockResponse(*BlockRequestMessage) (*BlockResponseMessage, error)

	// HandleBlockResponse is called upon receipt of BlockResponseMessage to process it.
	// If another request needs to be sent to the peer, this function will return it.
	HandleBlockResponse(*BlockResponseMessage) *BlockRequestMessage

	// HandleBlockAnnounce is called upon receipt of a BlockAnnounceMessage to process it.
	// If a request needs to be sent to the peer to retrieve the full block, this function will return it.
	HandleBlockAnnounce(*BlockAnnounceMessage) *BlockRequestMessage

	// HandleSeenBlocks is called upon receiving a StatusMessage from a peer that has a higher chain head than us
	HandleSeenBlocks(*big.Int) *BlockRequestMessage
}
