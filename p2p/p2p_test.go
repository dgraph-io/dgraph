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
	"github.com/libp2p/go-libp2p-core/peer"
)

// maximum wait time for non-status message to be handled
var TestMessageTimeout = 10 * time.Second

// wait time for status messages to be exchanged and handled
var TestStatusTimeout = time.Second

func startNewService(t *testing.T, cfg *Config, msgSend chan Message, msgRec chan Message) *Service {
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
		NoBootstrap: true,
		NoGossip:    true,
		NoMdns:      true,
	}
	node := startNewService(t, config, nil, nil)

	node.host.noStatus = true

	node.Stop()
}

func TestConnect(t *testing.T) {
	configA := &Config{
		Port:        7001,
		RandSeed:    1,
		NoBootstrap: true,
		NoGossip:    true,
		NoMdns:      true,
	}

	nodeA := startNewService(t, configA, nil, nil)
	defer nodeA.Stop()

	nodeA.host.noStatus = true

	configB := &Config{
		Port:        7002,
		RandSeed:    2,
		NoBootstrap: true,
		NoGossip:    true,
		NoMdns:      true,
	}

	nodeB := startNewService(t, configB, nil, nil)
	defer nodeB.Stop()

	nodeB.host.noStatus = true

	addrB := nodeB.host.fullAddrs()[0]
	addrInfoB, err := peer.AddrInfoFromP2pAddr(addrB)
	if err != nil {
		t.Fatal(err)
	}

	err = nodeA.host.connect(*addrInfoB)
	if err != nil {
		t.Fatal(err)
	}

	peerCountA := nodeA.host.peerCount()
	peerCountB := nodeB.host.peerCount()

	if peerCountA != 1 {
		t.Error(
			"node A does not have expected peer count",
			"\nexpected:", 1,
			"\nreceived:", peerCountA,
		)
	}

	if peerCountB != 1 {
		t.Error(
			"node B does not have expected peer count",
			"\nexpected:", 1,
			"\nreceived:", peerCountB,
		)
	}
}

func TestBootstrap(t *testing.T) {
	configA := &Config{
		Port:        7001,
		RandSeed:    1,
		NoBootstrap: true,
		NoGossip:    true,
		NoMdns:      true,
	}

	nodeA := startNewService(t, configA, nil, nil)
	defer nodeA.Stop()

	nodeA.host.noStatus = true

	addrA := nodeA.host.fullAddrs()[0]

	configB := &Config{
		BootstrapNodes: []string{addrA.String()},
		Port:           7002,
		RandSeed:       2,
		NoGossip:       true,
		NoMdns:         true,
	}

	nodeB := startNewService(t, configB, nil, nil)
	defer nodeB.Stop()

	nodeB.host.noStatus = true

	peerCountA := nodeA.host.peerCount()
	peerCountB := nodeB.host.peerCount()

	if peerCountA != 1 {
		t.Error(
			"node A does not have expected peer count",
			"\nexpected:", 1,
			"\nreceived:", peerCountA,
		)
	}

	if peerCountB != 1 {
		t.Error(
			"node B does not have expected peer count",
			"\nexpected:", 1,
			"\nreceived:", peerCountB,
		)
	}
}

func TestPing(t *testing.T) {
	configA := &Config{
		Port:        7001,
		RandSeed:    1,
		NoBootstrap: true,
		NoGossip:    true,
		NoMdns:      true,
	}

	nodeA := startNewService(t, configA, nil, nil)
	defer nodeA.Stop()

	nodeA.host.noStatus = true

	configB := &Config{
		Port:        7002,
		RandSeed:    2,
		NoBootstrap: true,
		NoGossip:    true,
		NoMdns:      true,
	}

	nodeB := startNewService(t, configB, nil, nil)
	defer nodeB.Stop()

	nodeB.host.noStatus = true

	addrB := nodeB.host.fullAddrs()[0]
	addrInfoB, err := peer.AddrInfoFromP2pAddr(addrB)
	if err != nil {
		t.Fatal(err)
	}

	err = nodeA.host.connect(*addrInfoB)
	if err != nil {
		t.Fatal(err)
	}

	err = nodeA.host.ping(addrInfoB.ID)
	if err != nil {
		t.Fatal(err)
	}
}

func TestExchangeStatus(t *testing.T) {
	configA := &Config{
		Port:        7001,
		RandSeed:    1,
		NoBootstrap: true,
		NoGossip:    true,
		NoMdns:      true,
	}

	msgSendA := make(chan Message)
	nodeA := startNewService(t, configA, msgSendA, nil)
	defer nodeA.Stop()

	addrA := nodeA.host.fullAddrs()[0]

	configB := &Config{
		BootstrapNodes: []string{addrA.String()},
		Port:           7002,
		RandSeed:       2,
		NoGossip:       true,
		NoMdns:         true,
	}

	msgSendB := make(chan Message)
	nodeB := startNewService(t, configB, msgSendB, nil)
	defer nodeB.Stop()

	time.Sleep(TestStatusTimeout)

	statusB := nodeA.host.peerStatus[nodeB.host.h.ID()]
	if statusB == false {
		t.Error(
			"node A did not receive status message from node B",
			"\nreceived:", statusB,
			"\nexpected:", true,
		)
	}

	statusA := nodeB.host.peerStatus[nodeA.host.h.ID()]
	if statusA == false {
		t.Error(
			"node B did not receive status message from node A",
			"\nreceived:", statusA,
			"\nexpected:", true,
		)
	}
}

func TestSendRequest(t *testing.T) {
	configA := &Config{
		Port:        7001,
		RandSeed:    1,
		NoBootstrap: true,
		NoGossip:    true,
		NoMdns:      true,
	}

	msgSendA := make(chan Message)
	nodeA := startNewService(t, configA, msgSendA, nil)
	defer nodeA.Stop()

	nodeA.host.noStatus = true

	addrA := nodeA.host.fullAddrs()[0]

	configB := &Config{
		BootstrapNodes: []string{addrA.String()},
		Port:           7002,
		RandSeed:       2,
		NoGossip:       true,
		NoMdns:         true,
	}

	msgSendB := make(chan Message)
	nodeB := startNewService(t, configB, msgSendB, nil)
	defer nodeB.Stop()

	nodeB.host.noStatus = true

	blockRequest := &BlockRequestMessage{
		ID:            1,
		RequestedData: 1,
		// TODO: investigate starting block mismatch with different slice length
		StartingBlock: []byte{1, 1, 1, 1, 1, 1, 1, 1, 1},
		EndBlockHash:  optional.NewHash(true, common.Hash{}),
		Direction:     1,
		Max:           optional.NewUint32(true, 1),
	}

	addrB := nodeB.host.fullAddrs()[0]
	addrInfoB, err := peer.AddrInfoFromP2pAddr(addrB)
	if err != nil {
		t.Fatal(err)
	}

	err = nodeA.host.send(addrInfoB.ID, blockRequest)
	if err != nil {
		t.Fatal(err)
	}

	select {
	case msg := <-msgSendB:
		if !reflect.DeepEqual(msg, blockRequest) {
			t.Error(
				"node B received unexpected message from node A",
				"\nexpected:", blockRequest,
				"\nreceived:", msg,
			)
		}
	case <-time.After(TestMessageTimeout):
		t.Error("node B timeout waiting for message from node A")
	}
}

func TestBroadcastRequest(t *testing.T) {
	configA := &Config{
		Port:        7001,
		RandSeed:    1,
		NoBootstrap: true,
		NoGossip:    true,
		NoMdns:      true,
	}

	msgSendA := make(chan Message)
	nodeA := startNewService(t, configA, msgSendA, nil)
	defer nodeA.Stop()

	nodeA.host.noStatus = true

	addrA := nodeA.host.fullAddrs()[0]

	configB := &Config{
		BootstrapNodes: []string{addrA.String()},
		Port:           7002,
		RandSeed:       2,
		NoGossip:       true,
		NoMdns:         true,
	}

	msgSendB := make(chan Message)
	nodeB := startNewService(t, configB, msgSendB, nil)
	defer nodeB.Stop()

	nodeB.host.noStatus = true

	configC := &Config{
		BootstrapNodes: []string{addrA.String()},
		Port:           7003,
		RandSeed:       3,
		NoGossip:       true,
		NoMdns:         true,
	}

	msgSendC := make(chan Message)
	nodeC := startNewService(t, configC, msgSendC, nil)
	defer nodeC.Stop()

	nodeC.host.noStatus = true

	blockRequest := &BlockRequestMessage{
		ID:            1,
		RequestedData: 1,
		// TODO: investigate starting block mismatch with different slice length
		StartingBlock: []byte{1, 1, 1, 1, 1, 1, 1, 1, 1},
		EndBlockHash:  optional.NewHash(true, common.Hash{}),
		Direction:     1,
		Max:           optional.NewUint32(true, 1),
	}

	nodeA.host.broadcast(blockRequest)

	select {
	case msg := <-msgSendB:
		if !reflect.DeepEqual(msg, blockRequest) {
			t.Error(
				"node B received unexpected message from node A",
				"\nexpected:", blockRequest,
				"\nreceived:", msg,
			)
		}
	case <-time.After(TestMessageTimeout):
		t.Error("node B timeout waiting for message")
	}

	select {
	case msg := <-msgSendC:
		if !reflect.DeepEqual(msg, blockRequest) {
			t.Error(
				"node C received unexpected message from node A",
				"\nexpected:", blockRequest,
				"\nreceived:", msg,
			)
		}
	case <-time.After(TestMessageTimeout):
		t.Error("node C timeout waiting for message")
	}

}

func TestBlockAnnounce(t *testing.T) {
	configA := &Config{
		Port:        7001,
		RandSeed:    1,
		NoBootstrap: true,
		NoGossip:    true,
		NoMdns:      true,
	}

	msgRecA := make(chan Message)
	msgSendA := make(chan Message)
	nodeA := startNewService(t, configA, msgSendA, msgRecA)
	defer nodeA.Stop()

	nodeA.host.noStatus = true

	addrA := nodeA.host.fullAddrs()[0]

	configB := &Config{
		BootstrapNodes: []string{addrA.String()},
		Port:           7002,
		RandSeed:       2,
		NoGossip:       true,
		NoMdns:         true,
	}

	msgSendB := make(chan Message)
	nodeB := startNewService(t, configB, msgSendB, nil)
	defer nodeB.Stop()

	nodeB.host.noStatus = true

	blockAnnounce := &BlockAnnounceMessage{
		Number: big.NewInt(1),
	}

	// simulate block announce message received from core service
	msgRecA <- blockAnnounce

	select {
	case msg := <-msgSendB:
		// TODO: investigate error when using deep equal
		if msg.String() != blockAnnounce.String() {
			t.Error(
				"node B received unexpected message from node A",
				"\nexpected:", blockAnnounce,
				"\nreceived:", msg,
			)
		}
	case <-time.After(TestMessageTimeout):
		t.Error("node B timeout waiting for message")
	}
}

func TestGossip(t *testing.T) {
	configA := &Config{
		Port:        7001,
		RandSeed:    1,
		NoBootstrap: true,
		NoMdns:      true,
	}

	msgSendA := make(chan Message)
	nodeA := startNewService(t, configA, msgSendA, nil)
	defer nodeA.Stop()

	nodeA.host.noStatus = true

	addrA := nodeA.host.fullAddrs()[0]

	configB := &Config{
		BootstrapNodes: []string{addrA.String()},
		Port:           7002,
		RandSeed:       2,
		NoMdns:         true,
	}

	msgSendB := make(chan Message)
	nodeB := startNewService(t, configB, msgSendB, nil)
	defer nodeB.Stop()

	nodeB.host.noStatus = true

	addrB := nodeB.host.fullAddrs()[0]

	configC := &Config{
		BootstrapNodes: []string{addrB.String()},
		Port:           7003,
		RandSeed:       3,
		NoMdns:         true,
	}

	msgSendC := make(chan Message)
	nodeC := startNewService(t, configC, msgSendC, nil)
	defer nodeC.Stop()

	nodeC.host.noStatus = true

	blockRequest := &BlockRequestMessage{
		ID:            1,
		RequestedData: 1,
		// TODO: investigate starting block mismatch with different slice length
		StartingBlock: []byte{1, 1, 1, 1, 1, 1, 1, 1, 1},
		EndBlockHash:  optional.NewHash(true, common.Hash{}),
		Direction:     1,
		Max:           optional.NewUint32(true, 1),
	}

	addrInfoB, err := peer.AddrInfoFromP2pAddr(addrB)
	if err != nil {
		t.Fatal(err)
	}

	err = nodeA.host.send(addrInfoB.ID, blockRequest)
	if err != nil {
		t.Fatal(err)
	}

	select {
	case msg := <-msgSendB:
		if !reflect.DeepEqual(msg, blockRequest) {
			t.Error(
				"node B received unexpected message from node A",
				"\nexpected:", blockRequest,
				"\nreceived:", msg,
			)
		}
	case <-time.After(TestMessageTimeout):
		t.Error("node A timeout waiting for message")
	}

	// Expecting 4 messages to be gossiped in an unpredictable order. We need
	// to wait for all 4 messages to be sent to their respective core services
	// or else send on closed channel error.
	for i := 0; i < 4; i++ {
		select {
		case msg := <-msgSendA:
			if !reflect.DeepEqual(msg, blockRequest) {
				t.Error(
					"node A received unexpected message from node B",
					"\nexpected:", blockRequest,
					"\nreceived:", msg,
				)
			}
		case msg := <-msgSendC:
			if !reflect.DeepEqual(msg, blockRequest) {
				t.Error(
					"node C received unexpected message from node B",
					"\nexpected:", blockRequest,
					"\nreceived:", msg,
				)
			}
		case msg := <-msgSendB:
			if !reflect.DeepEqual(msg, blockRequest) {
				t.Error(
					"node B received unexpected message from node A or node C",
					"\nexpected:", blockRequest,
					"\nreceived:", msg,
				)
			}
		case <-time.After(TestMessageTimeout):
			t.Error("timeout waiting for messages")
		}
	}

	hasSeenB := nodeB.gossip.hasSeen[blockRequest.Id()]
	if hasSeenB == false {
		t.Error(
			"node B did not receive block request message from node A",
			"\nreceived:", hasSeenB,
			"\nexpected:", true,
		)
	}

	hasSeenA := nodeA.gossip.hasSeen[blockRequest.Id()]
	if hasSeenA == false {
		t.Error(
			"node A did not receive block request message from node B or node C",
			"\nreceived:", hasSeenA,
			"\nexpected:", true,
		)
	}

	hasSeenC := nodeC.gossip.hasSeen[blockRequest.Id()]
	if hasSeenC == false {
		t.Error(
			"node C did not receive block request message from node A or node B",
			"\nreceived:", hasSeenC,
			"\nexpected:", true,
		)
	}
}
