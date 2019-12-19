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
	"testing"
	"time"
)

// test gossip messages to connected peers
func TestGossip(t *testing.T) {
	configA := &Config{
		Port:        7001,
		RandSeed:    1,
		NoBootstrap: true,
		NoMdns:      true,
	}

	nodeA, msgSendA, _ := createTestService(t, configA)
	defer nodeA.Stop()

	nodeA.noStatus = true

	configB := &Config{
		Port:        7002,
		RandSeed:    2,
		NoBootstrap: true,
		NoMdns:      true,
	}

	nodeB, msgSendB, _ := createTestService(t, configB)
	defer nodeB.Stop()

	nodeB.noStatus = true

	addrInfosA, err := nodeA.host.addrInfos()
	if err != nil {
		t.Fatal(err)
	}

	err = nodeB.host.connect(*addrInfosA[0])
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

	nodeC.noStatus = true

	err = nodeC.host.connect(*addrInfosA[0])
	if err != nil {
		t.Fatal(err)
	}

	addrInfosB, err := nodeB.host.addrInfos()
	if err != nil {
		t.Fatal(err)
	}

	err = nodeC.host.connect(*addrInfosB[0])
	if err != nil {
		t.Fatal(err)
	}

	err = nodeA.host.send(addrInfosB[0].ID, TestMessage)
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

	hasSeenB := nodeB.gossip.hasSeen[TestMessage.Id()]
	if hasSeenB == false {
		t.Error(
			"node B did not receive block request message from node A",
			"\nreceived:", hasSeenB,
			"\nexpected:", true,
		)
	}

	hasSeenA := nodeA.gossip.hasSeen[TestMessage.Id()]
	if hasSeenA == false {
		t.Error(
			"node A did not receive block request message from node B or node C",
			"\nreceived:", hasSeenA,
			"\nexpected:", true,
		)
	}

	hasSeenC := nodeC.gossip.hasSeen[TestMessage.Id()]
	if hasSeenC == false {
		t.Error(
			"node C did not receive block request message from node A or node B",
			"\nreceived:", hasSeenC,
			"\nexpected:", true,
		)
	}
}
