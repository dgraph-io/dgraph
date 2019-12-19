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
	"github.com/ChainSafe/gossamer/common"
)

// Health is network information about host needed for the rpc server
type Health struct {
	Peers           int
	IsSyncing       bool
	ShouldHavePeers bool
}

// NetworkState is network information about host needed for the rpc server and the runtime
type NetworkState struct {
	PeerId string
}

// PeerInfo is network information about peers needed for the rpc server
type PeerInfo struct {
	PeerId          string
	Roles           byte
	ProtocolVersion uint32
	BestHash        common.Hash
	BestNumber      uint64
}
