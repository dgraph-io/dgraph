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
	"testing"
	"time"

	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/ChainSafe/gossamer/lib/utils"

	"github.com/stretchr/testify/require"
)

// TestRequestedBlockIDs tests adding and removing block ids from requestedBlockIDs
func TestRequestedBlockIDs(t *testing.T) {
	basePath := utils.NewTestBasePath(t, "node")

	// removes all data directories created within test directory
	defer utils.RemoveTestDir(t)

	config := &Config{
		BasePath:    basePath,
		Port:        7001,
		RandSeed:    1,
		NoBootstrap: true,
		NoMDNS:      true,
	}

	node := createTestService(t, config)

	hasRequestedBlockID := node.requestTracker.hasRequestedBlockID(1)
	require.Equal(t, false, hasRequestedBlockID)

	node.requestTracker.addRequestedBlockID(1)

	hasRequestedBlockID = node.requestTracker.hasRequestedBlockID(1)
	require.Equal(t, true, hasRequestedBlockID)

	node.requestTracker.removeRequestedBlockID(1)

	hasRequestedBlockID = node.requestTracker.hasRequestedBlockID(1)
	require.Equal(t, false, hasRequestedBlockID)
}

// have a peer send a message status with a block ahead
// test exchanged messages after peer connected are correct
func TestHandleStatusMessage(t *testing.T) {
	basePathA := utils.NewTestBasePath(t, "nodeA")

	// removes all data directories created within test directory
	defer utils.RemoveTestDir(t)

	heightA := big.NewInt(3)
	mmhA := new(MockMessageHandler)
	configA := &Config{
		BasePath:    basePathA,
		BlockState:  newMockBlockState(heightA),
		Port:        7001,
		RandSeed:    1,
		NoBootstrap: true,
		NoMDNS:      true,
		Syncer:      newMockSyncer(),
	}

	nodeA := createTestService(t, configA)
	defer nodeA.Stop()

	nodeA.noGossip = true

	genesisHash, err := common.HexToHash("0xdcd1346701ca8396496e52aa2785b1748deb6db09551b72159dcb3e08991025b")
	require.Nil(t, err)

	bestBlockHash, err := common.HexToHash("0x829de6be9a35b55c794c609c060698b549b3064c183504c18ab7517e41255569")
	require.Nil(t, err)

	testStatusMessage := &StatusMessage{
		ProtocolVersion:     uint32(2),
		MinSupportedVersion: uint32(2),
		Roles:               byte(4),
		BestBlockNumber:     uint64(2434417),
		BestBlockHash:       bestBlockHash,
		GenesisHash:         genesisHash,
		ChainStatus:         []byte{0},
	}

	// simulate host status message sent from core service on startup
	mmhA.HandleMessage(testStatusMessage)

	basePathB := utils.NewTestBasePath(t, "nodeB")

	heightB := big.NewInt(1)
	mmhB := new(MockMessageHandler)

	configB := &Config{
		BasePath:    basePathB,
		BlockState:  newMockBlockState(heightB),
		Port:        7002,
		RandSeed:    2,
		NoBootstrap: true,
		NoMDNS:      true,
		Syncer:      newMockSyncer(),
	}

	nodeB := createTestService(t, configB)
	defer nodeB.Stop()

	nodeB.noGossip = true

	// simulate host status message sent from core service on startup
	mmhB.HandleMessage(testStatusMessage)

	addrInfosB, err := nodeB.host.addrInfos()
	require.Nil(t, err)

	err = nodeA.host.connect(*addrInfosB[0])
	require.Nil(t, err)

	time.Sleep(TestStatusTimeout)

	if !nodeA.status.confirmed(nodeB.host.h.ID()) {
		t.Error("node A did not confirm status of node B")
	}

	if !nodeB.status.confirmed(nodeA.host.h.ID()) {
		t.Error("node B did not confirm status of node A")
	}

	num := nodeB.syncer.(*mockSyncer).highestSeen
	require.NotNil(t, num)

	if num.Cmp(heightA) != 0 {
		t.Fatalf("Fail: got %d expected %d", num, heightA)
	}
}
