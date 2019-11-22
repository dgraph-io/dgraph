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
	"reflect"
	"testing"
	"time"

	"github.com/ChainSafe/gossamer/common"
	"github.com/ChainSafe/gossamer/common/optional"
	peer "github.com/libp2p/go-libp2p-core/peer"
)

func startNewService(t *testing.T, cfg *Config, msgSend chan []byte, msgRec chan BlockAnnounceMessage) *Service {
	node, err := NewService(cfg, msgSend, msgRec)
	if err != nil {
		t.Fatal(err)
	}

	err = node.Start()
	if err != nil {
		t.Fatal(err)
	}

	return node
}

func TestStartService(t *testing.T) {
	config := &Config{
		Port:        7001,
		RandSeed:    1,
		NoBootstrap: true, // TODO: fix no bootstrap, this should be required
		NoMdns:      true, // TODO: investigate failed dials, disable for now
	}
	node := startNewService(t, config, nil, nil)
	node.Stop()
}

func TestBootstrap(t *testing.T) {
	configA := &Config{
		Port:        7001,
		RandSeed:    1,
		NoBootstrap: true, // TODO: fix no bootstrap, this should be required
		NoMdns:      true, // TODO: investigate failed dials, disable for now
	}

	nodeA := startNewService(t, configA, nil, nil)
	defer nodeA.Stop()

	addrA := nodeA.host.fullAddrs()[0]

	configB := &Config{
		BootstrapNodes: []string{addrA.String()},
		Port:           7002,
		RandSeed:       2,
		NoMdns:         true, // TODO: investigate failed dials, disable for now
	}

	nodeB := startNewService(t, configB, nil, nil)
	defer nodeB.Stop()

	peerCountA := nodeA.host.peerCount()

	if peerCountA != 1 {
		t.Errorf("Expected peer count: 1, got peer count: %d", peerCountA)
	}
}

func TestConnect(t *testing.T) {
	configA := &Config{
		Port:        7001,
		RandSeed:    1,
		NoBootstrap: true, // TODO: fix no bootstrap, this should be required
		NoMdns:      true, // TODO: investigate failed dials, disable for now
	}

	nodeA := startNewService(t, configA, nil, nil)
	defer nodeA.Stop()

	configB := &Config{
		Port:        7002,
		RandSeed:    2,
		NoBootstrap: true, // TODO: fix no bootstrap, this should be required
		NoMdns:      true, // TODO: investigate failed dials, disable for now
	}

	nodeB := startNewService(t, configB, nil, nil)
	defer nodeB.Stop()

	addrA := nodeA.host.fullAddrs()[0]

	addrInfoA, err := peer.AddrInfoFromP2pAddr(addrA)
	if err != nil {
		t.Fatal(err)
	}

	err = nodeB.host.connect(*addrInfoA)
	if err != nil {
		t.Fatal(err)
	}

	peerCountB := nodeB.host.peerCount()

	if peerCountB != 1 {
		t.Errorf("Expected peer count: 1, got peer count: %d", peerCountB)
	}
}

func TestPing(t *testing.T) {
	configA := &Config{
		Port:        7001,
		RandSeed:    1,
		NoBootstrap: true, // TODO: fix no bootstrap, this should be required
		NoMdns:      true, // TODO: investigate failed dials, disable for now
	}

	nodeA := startNewService(t, configA, nil, nil)
	defer nodeA.Stop()

	configB := &Config{
		Port:        7002,
		RandSeed:    2,
		NoBootstrap: true, // TODO: fix no bootstrap, this should be required
		NoMdns:      true, // TODO: investigate failed dials, disable for now
	}

	msgSendB := make(chan []byte)

	nodeB := startNewService(t, configB, msgSendB, nil)
	defer nodeB.Stop()

	addrA := nodeA.host.fullAddrs()[0]

	addrInfoA, err := peer.AddrInfoFromP2pAddr(addrA)
	if err != nil {
		t.Fatal(err)
	}

	err = nodeB.host.connect(*addrInfoA)
	if err != nil {
		t.Fatal(err)
	}

	err = nodeB.host.ping(addrInfoA.ID)
	if err != nil {
		t.Fatal(err)
	}
}

func TestSendRequest(t *testing.T) {
	configA := &Config{
		Port:        7001,
		RandSeed:    1,
		NoBootstrap: true, // TODO: fix no bootstrap, this should be required
		NoMdns:      true, // TODO: investigate failed dials, disable for now
	}

	nodeA := startNewService(t, configA, nil, nil)
	defer nodeA.Stop()

	configB := &Config{
		Port:        7002,
		RandSeed:    2,
		NoBootstrap: true, // TODO: fix no bootstrap, this should be required
		NoMdns:      true, // TODO: investigate failed dials, disable for now
	}

	msgSendB := make(chan []byte)

	nodeB := startNewService(t, configB, msgSendB, nil)
	defer nodeB.Stop()

	addrA := nodeA.host.fullAddrs()[0]

	addrInfoA, err := peer.AddrInfoFromP2pAddr(addrA)
	if err != nil {
		t.Fatal(err)
	}

	err = nodeB.host.connect(*addrInfoA)
	if err != nil {
		t.Fatal(err)
	}

	addrB := nodeB.host.fullAddrs()[0]

	addrInfoB, err := peer.AddrInfoFromP2pAddr(addrB)
	if err != nil {
		t.Fatal(err)
	}

	// Create end block hash (arbitrary block hash)
	endBlock, err := common.HexToHash("0xfd19d9ebac759c993fd2e05a1cff9e757d8741c2704c8682c15b5503496b6aa1")
	if err != nil {
		t.Fatal(err)
	}

	// Create block request message (RequestedData: 1 = request header)
	blockRequest := &BlockRequestMessage{
		ID:            1,
		RequestedData: 1,
		StartingBlock: []byte{1, 1},
		EndBlockHash:  optional.NewHash(true, endBlock),
		Direction:     1,
		Max:           optional.NewUint32(true, 1),
	}

	encBlockRequest, err := blockRequest.Encode()
	if err != nil {
		t.Fatal(err)
	}

	err = nodeA.host.send(*addrInfoB, encBlockRequest)
	if err != nil {
		t.Fatal(err)
	}

	select {
	case message := <-msgSendB:
		// Compare received message to original message
		if !reflect.DeepEqual(message, encBlockRequest) {
			t.Error("Did not receive the correct message")
		}
	case <-time.After(30 * time.Second):
		t.Errorf("Did not receive message from %s", nodeA.host.hostAddr)
	}
}

func TestGossiping(t *testing.T) {
	configA := &Config{
		Port:        7001,
		RandSeed:    1,
		NoBootstrap: true, // TODO: fix no bootstrap, this should be required
		NoMdns:      true, // TODO: investigate failed dials, disable for now
	}

	nodeA := startNewService(t, configA, nil, nil)
	defer nodeA.Stop()

	addrA := nodeA.host.fullAddrs()[0]

	configB := &Config{
		BootstrapNodes: []string{addrA.String()}, // Bootstrap node with node A
		Port:           7002,
		RandSeed:       2,
		NoMdns:         true, // TODO: investigate failed dials, disable for now
	}

	msgSendB := make(chan []byte)

	nodeB := startNewService(t, configB, msgSendB, nil)
	defer nodeB.Stop()

	configC := &Config{
		BootstrapNodes: []string{addrA.String()}, // Bootstrap node with node A
		Port:           7003,
		RandSeed:       3,
		NoMdns:         true, // TODO: investigate failed dials, disable for now
	}

	msgSendC := make(chan []byte)

	nodeC := startNewService(t, configC, msgSendC, nil)
	defer nodeC.Stop()

	// Create end block hash (arbitrary block hash)
	endBlock, err := common.HexToHash("0xfd19d9ebac759c993fd2e05a1cff9e757d8741c2704c8682c15b5503496b6aa1")
	if err != nil {
		t.Fatal(err)
	}

	// Create block request message (RequestedData: 1 = request header)
	blockRequest := &BlockRequestMessage{
		ID:            1,
		RequestedData: 1,
		StartingBlock: []byte{1, 1},
		EndBlockHash:  optional.NewHash(true, endBlock),
		Direction:     1,
		Max:           optional.NewUint32(true, 1),
	}

	// Broadcast block request message
	err = nodeA.Broadcast(blockRequest)
	if err != nil {
		t.Fatal(err)
	}

	encBlockRequest, err := blockRequest.Encode()
	if err != nil {
		t.Fatal(err)
	}

	select {
	case message := <-msgSendB:
		// Compare received message to original message
		if !reflect.DeepEqual(message, encBlockRequest) {
			t.Error("Did not receive the correct message")
		}
	case <-time.After(30 * time.Second):
		t.Errorf("Did not receive message from %s", nodeA.host.hostAddr)
	}

	select {
	case message := <-msgSendC:
		// Compare received message to original message
		if !reflect.DeepEqual(encBlockRequest, message) {
			t.Error("Did not receive the correct message")
		}
	case <-time.After(30 * time.Second):
		t.Errorf("Did not receive message from %s", nodeB.host.hostAddr)
	}
}

func TestReceiveChannel(t *testing.T) {
	configA := &Config{
		Port:        7001,
		RandSeed:    1,
		NoBootstrap: true, // TODO: fix no bootstrap, this should be required
		NoMdns:      true, // TODO: investigate failed dials, disable for now
	}

	msgRecA := make(chan BlockAnnounceMessage)

	nodeA := startNewService(t, configA, nil, msgRecA)
	defer nodeA.Stop()

	addrA := nodeA.host.fullAddrs()[0]

	configB := &Config{
		BootstrapNodes: []string{addrA.String()}, // Bootstrap node with node A
		Port:           7002,
		RandSeed:       2,
		NoMdns:         true, // TODO: investigate failed dials, disable for now
	}

	msgSendB := make(chan []byte)

	nodeB := startNewService(t, configB, msgSendB, nil)
	defer nodeB.Stop()

	blockAnnounce := BlockAnnounceMessage{
		Number: big.NewInt(1),
	}

	msgRecA <- blockAnnounce

	encBlockAnnounce, err := blockAnnounce.Encode()
	if err != nil {
		t.Fatal(err)
	}

	select {
	case message := <-msgSendB:
		// Compare received message to original message
		if !reflect.DeepEqual(message, encBlockAnnounce) {
			t.Error("Did not receive the correct message")
		}
	case <-time.After(30 * time.Second):
		t.Errorf("Did not receive message from %s", nodeB.host.hostAddr)
	}
}
