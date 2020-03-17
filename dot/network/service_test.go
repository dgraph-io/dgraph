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
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/ChainSafe/gossamer/lib/common/optional"
	"github.com/ChainSafe/gossamer/lib/common/variadic"
	"github.com/ChainSafe/gossamer/lib/utils"
)

var TestProtocolID = "/gossamer/test/0"

// arbitrary block request message
var TestMessage = &BlockRequestMessage{
	ID:            1,
	RequestedData: 1,
	// TODO: investigate starting block mismatch with different slice length
	StartingBlock: variadic.NewUint64OrHash([]byte{1, 1, 1, 1, 1, 1, 1, 1, 1}),
	EndBlockHash:  optional.NewHash(true, common.Hash{}),
	Direction:     1,
	Max:           optional.NewUint32(true, 1),
}

// maximum wait time for non-status message to be handled
var TestMessageTimeout = 3 * time.Second

// time between connection retries (BackoffBase default 5 seconds)
var TestBackoffTimeout = 5 * time.Second

// failedToDial returns true if "failed to dial" error, otherwise false
func failedToDial(err error) bool {
	return err != nil && strings.Contains(err.Error(), "failed to dial")
}

// helper method to create and start a new network service
func createTestService(t *testing.T, cfg *Config) (node *Service, msgSend chan Message, msgRec chan Message) {
	msgRec = make(chan Message, 4)
	msgSend = make(chan Message, 4)

	// same for all network tests use the createTestService helper method
	cfg.BlockState = &MockBlockState{} // required
	cfg.NetworkState = &MockNetworkState{}
	cfg.ProtocolID = TestProtocolID // default "/gossamer/gssmr/0"

	if cfg.SyncChan == nil {
		cfg.SyncChan = make(chan *big.Int)
	}

	node, err := NewService(cfg, msgSend, msgRec)
	if err != nil {
		t.Fatal(err)
	}

	err = node.Start()
	if err != nil {
		t.Fatal(err)
	}

	return node, msgSend, msgRec
}

// createTestServiceWithBlockState is a helper method to create and start a new network service
func createTestServiceWithBlockState(t *testing.T, cfg *Config, blockState *MockBlockState) (node *Service, msgRec chan Message) {
	msgRec = make(chan Message)
	msgSend := make(chan Message)

	cfg.BlockState = blockState
	cfg.NetworkState = &MockNetworkState{}
	cfg.ProtocolID = TestProtocolID

	if cfg.SyncChan == nil {
		cfg.SyncChan = make(chan *big.Int)
	}

	var err error
	node, err = NewService(cfg, msgSend, msgRec)
	if err != nil {
		t.Fatal(err)
	}

	err = node.Start()
	if err != nil {
		t.Fatal(err)
	}

	return node, msgRec
}

// test network service starts
func TestStartService(t *testing.T) {
	dataDir := utils.NewTestDataDir(t, "node")

	// removes all data directories created within test directory
	defer utils.RemoveTestDir(t)

	config := &Config{
		DataDir:     dataDir,
		Port:        7001,
		RandSeed:    1,
		NoBootstrap: true,
		NoMDNS:      true,
	}
	node, _, _ := createTestService(t, config)
	node.Stop()
}

// test broacast messages from core service
func TestBroadcastMessages(t *testing.T) {
	dataDirA := utils.NewTestDataDir(t, "nodeA")

	// removes all data directories created within test directory
	defer utils.RemoveTestDir(t)

	configA := &Config{
		DataDir:     dataDirA,
		Port:        7001,
		RandSeed:    1,
		NoBootstrap: true,
		NoMDNS:      true,
	}

	nodeA, _, msgRecA := createTestService(t, configA)
	defer nodeA.Stop()

	nodeA.noGossip = true
	nodeA.noStatus = true

	dataDirB := utils.NewTestDataDir(t, "nodeB")

	configB := &Config{
		DataDir:     dataDirB,
		Port:        7002,
		RandSeed:    2,
		NoBootstrap: true,
		NoMDNS:      true,
	}

	nodeB, msgSendB, _ := createTestService(t, configB)
	defer nodeB.Stop()

	nodeB.noGossip = true
	nodeB.noStatus = true

	addrInfosB, err := nodeB.host.addrInfos()
	if err != nil {
		t.Fatal(err)
	}

	err = nodeA.host.connect(*addrInfosB[0])
	// retry connect if "failed to dial" error
	if failedToDial(err) {
		time.Sleep(TestBackoffTimeout)
		err = nodeA.host.connect(*addrInfosB[0])
	}
	if err != nil {
		t.Fatal(err)
	}

	// simulate message sent from core service
	msgRecA <- TestMessage

	select {
	case msg := <-msgSendB:
		if !reflect.DeepEqual(msg, TestMessage) {
			t.Error(
				"node B received unexpected message from node A",
				"\nexpected:", TestMessage,
				"\nreceived:", msg,
			)
		}
	case <-time.After(TestMessageTimeout):
		t.Error("node B timeout waiting for message")
	}
}
