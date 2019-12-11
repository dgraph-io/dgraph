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
	"reflect"
	"testing"
	"time"

	"github.com/ChainSafe/gossamer/common"
	"github.com/ChainSafe/gossamer/common/optional"
)

// wait time to discover and connect using mdns discovery
var TestDiscoveryTimeout = 3 * time.Second

// wait time for status messages to be exchanged and handled
var TestStatusTimeout = time.Second

// maximum wait time for non-status message to be handled
var TestMessageTimeout = 10 * time.Second

// arbitrary block request message
var testMessage = &BlockRequestMessage{
	ID:            1,
	RequestedData: 1,
	// TODO: investigate starting block mismatch with different slice length
	StartingBlock: []byte{1, 1, 1, 1, 1, 1, 1, 1, 1},
	EndBlockHash:  optional.NewHash(true, common.Hash{}),
	Direction:     1,
	Max:           optional.NewUint32(true, 1),
}

// helper method to create and start a new p2p service
func createTestService(t *testing.T, cfg *Config) (node *Service, msgSend chan Message, msgRec chan Message) {

	msgRec = make(chan Message)
	msgSend = make(chan Message)

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

// test p2p service starts
func TestStartService(t *testing.T) {
	config := &Config{
		Port:        7001,
		RandSeed:    1,
		NoBootstrap: true,
		NoGossip:    true,
		NoMdns:      true,
	}
	node, _, _ := createTestService(t, config)
	node.Stop()
}

// test mdns discovery service (discovers and connects)
func TestDiscovery(t *testing.T) {
	configA := &Config{
		Port:        7001,
		RandSeed:    1,
		NoBootstrap: true,
		NoGossip:    true,
	}

	nodeA, _, _ := createTestService(t, configA)
	defer nodeA.Stop()

	nodeA.host.noStatus = true

	configB := &Config{
		Port:        7002,
		RandSeed:    2,
		NoBootstrap: true,
		NoGossip:    true,
	}

	nodeB, _, _ := createTestService(t, configB)
	defer nodeB.Stop()

	nodeB.host.noStatus = true

	time.Sleep(TestDiscoveryTimeout)

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

// test broacast messages from core service
func TestBroadcastMessages(t *testing.T) {
	configA := &Config{
		Port:        7001,
		RandSeed:    1,
		NoBootstrap: true,
		NoGossip:    true,
		NoMdns:      true,
	}

	nodeA, _, msgRecA := createTestService(t, configA)
	defer nodeA.Stop()

	nodeA.host.noStatus = true

	configB := &Config{
		Port:        7002,
		RandSeed:    2,
		NoBootstrap: true,
		NoGossip:    true,
		NoMdns:      true,
	}

	nodeB, msgSendB, _ := createTestService(t, configB)
	defer nodeB.Stop()

	nodeB.host.noStatus = true

	addrInfoB, err := nodeB.host.addrInfo()
	if err != nil {
		t.Fatal(err)
	}

	err = nodeA.host.connect(*addrInfoB)
	if err != nil {
		t.Fatal(err)
	}

	// simulate message sent from core service
	msgRecA <- testMessage

	select {
	case msg := <-msgSendB:
		if !reflect.DeepEqual(msg, testMessage) {
			t.Error(
				"node B received unexpected message from node A",
				"\nexpected:", testMessage,
				"\nreceived:", msg,
			)
		}
	case <-time.After(TestMessageTimeout):
		t.Error("node B timeout waiting for message")
	}
}

// test exchange status messages after peer connected
func TestExchangeStatusMessages(t *testing.T) {
	configA := &Config{
		Port:        7001,
		RandSeed:    1,
		NoBootstrap: true,
		NoGossip:    true,
		NoMdns:      true,
	}

	nodeA, _, _ := createTestService(t, configA)
	defer nodeA.Stop()

	configB := &Config{
		Port:        7002,
		RandSeed:    2,
		NoBootstrap: true,
		NoGossip:    true,
		NoMdns:      true,
	}

	nodeB, _, _ := createTestService(t, configB)
	defer nodeB.Stop()

	addrInfoB, err := nodeB.host.addrInfo()
	if err != nil {
		t.Fatal(err)
	}

	err = nodeA.host.connect(*addrInfoB)
	if err != nil {
		t.Fatal(err)
	}

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

// test gossip messages to connected peers
func TestGossipMessages(t *testing.T) {
	configA := &Config{
		Port:        7001,
		RandSeed:    1,
		NoBootstrap: true,
		NoMdns:      true,
	}

	nodeA, msgSendA, _ := createTestService(t, configA)
	defer nodeA.Stop()

	nodeA.host.noStatus = true

	configB := &Config{
		Port:        7002,
		RandSeed:    2,
		NoBootstrap: true,
		NoMdns:      true,
	}

	nodeB, msgSendB, _ := createTestService(t, configB)
	defer nodeB.Stop()

	nodeB.host.noStatus = true

	addrInfoA, err := nodeA.host.addrInfo()
	if err != nil {
		t.Fatal(err)
	}

	err = nodeB.host.connect(*addrInfoA)
	if err != nil {
		t.Fatal(err)
	}

	configC := &Config{
		Port:        7003,
		RandSeed:    3,
		NoBootstrap: true,
		NoMdns:      true,
	}

	nodeC, msgSendC, _ := createTestService(t, configC)
	defer nodeC.Stop()

	nodeC.host.noStatus = true

	err = nodeC.host.connect(*addrInfoA)
	if err != nil {
		t.Fatal(err)
	}

	addrInfoB, err := nodeB.host.addrInfo()
	if err != nil {
		t.Fatal(err)
	}

	err = nodeC.host.connect(*addrInfoB)
	if err != nil {
		t.Fatal(err)
	}

	err = nodeA.host.send(addrInfoB.ID, testMessage)
	if err != nil {
		t.Fatal(err)
	}

	// node A sends message to node B
	select {
	case <-msgSendB:
	case <-time.After(TestMessageTimeout):
		t.Error("node A timeout waiting for message")
	}

	// node B gossips message to node A and node C
	for i := 0; i < 2; i++ {
		select {
		case <-msgSendA:
		case <-msgSendC:
		case <-time.After(TestMessageTimeout):
			t.Error("node A timeout waiting for message")
		}
	}

	// node A gossips message to node B and node C
	// node C gossips message to node A and node B
	for i := 0; i < 4; i++ {
		select {
		case <-msgSendA:
		case <-msgSendB:
		case <-msgSendC:
		case <-time.After(TestMessageTimeout):
			t.Error("timeout waiting for messages")
		}
	}

	hasSeenB := nodeB.gossip.hasSeen[testMessage.Id()]
	if hasSeenB == false {
		t.Error(
			"node B did not receive block request message from node A",
			"\nreceived:", hasSeenB,
			"\nexpected:", true,
		)
	}

	hasSeenA := nodeA.gossip.hasSeen[testMessage.Id()]
	if hasSeenA == false {
		t.Error(
			"node A did not receive block request message from node B or node C",
			"\nreceived:", hasSeenA,
			"\nexpected:", true,
		)
	}

	hasSeenC := nodeC.gossip.hasSeen[testMessage.Id()]
	if hasSeenC == false {
		t.Error(
			"node C did not receive block request message from node A or node B",
			"\nreceived:", hasSeenC,
			"\nexpected:", true,
		)
	}
}
