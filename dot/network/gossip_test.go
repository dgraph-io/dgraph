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
	"os"
	"testing"
	"time"
)

// test gossip messages to connected peers
func TestGossip(t *testing.T) {
	dataDirA := newTestDataDir(t, "nodeA")
	defer os.RemoveAll(dataDirA)

	configA := &Config{
		DataDir:     dataDirA,
		Port:        7001,
		RandSeed:    1,
		NoBootstrap: true,
		NoMDNS:      true,
	}

	nodeA, msgSendA, _ := createTestService(t, configA)
	defer nodeA.Stop()

	nodeA.noStatus = true

	dataDirB := newTestDataDir(t, "nodeB")
	defer os.RemoveAll(dataDirB)

	configB := &Config{
		DataDir:     dataDirB,
		Port:        7002,
		RandSeed:    2,
		NoBootstrap: true,
		NoMDNS:      true,
	}

	nodeB, msgSendB, _ := createTestService(t, configB)
	defer nodeB.Stop()

	nodeB.noStatus = true

	addrInfosA, err := nodeA.host.addrInfos()
	if err != nil {
		t.Fatal(err)
	}

	err = nodeB.host.connect(*addrInfosA[0])
	// retry connect if "failed to dial" error
	if failedToDial(err) {
		time.Sleep(TestBackoffTimeout)
		err = nodeB.host.connect(*addrInfosA[0])
	}
	if err != nil {
		t.Fatal(err)
	}

	dataDirC := newTestDataDir(t, "nodeC")
	defer os.RemoveAll(dataDirC)

	configC := &Config{
		DataDir:     dataDirC,
		Port:        7003,
		RandSeed:    3,
		NoBootstrap: true,
		NoMDNS:      true,
	}

	nodeC, msgSendC, _ := createTestService(t, configC)
	defer nodeC.Stop()

	nodeC.noStatus = true

	err = nodeC.host.connect(*addrInfosA[0])
	// retry connect if "failed to dial" error
	if failedToDial(err) {
		time.Sleep(TestBackoffTimeout)
		err = nodeC.host.connect(*addrInfosA[0])
	}
	if err != nil {
		t.Fatal(err)
	}

	addrInfosB, err := nodeB.host.addrInfos()
	if err != nil {
		t.Fatal(err)
	}

	err = nodeC.host.connect(*addrInfosB[0])
	// retry connect if "failed to dial" error
	if failedToDial(err) {
		time.Sleep(TestBackoffTimeout)
		err = nodeC.host.connect(*addrInfosB[0])
	}
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

	// node B gossips message to node C
	select {
	case <-msgSendC:
	case <-time.After(TestMessageTimeout):
		t.Error("node A timeout waiting for message")
	}

	// node C gossips message to node A
	select {
	case <-msgSendA:
	case <-time.After(TestMessageTimeout):
		t.Error("node A timeout waiting for message")
	}

	hasSeenB := nodeB.gossip.hasSeen[TestMessage.IDString()]
	if hasSeenB == false {
		t.Error(
			"node B did not receive block request message from node A",
			"\nreceived:", hasSeenB,
			"\nexpected:", true,
		)
	}

	hasSeenA := nodeA.gossip.hasSeen[TestMessage.IDString()]
	if hasSeenA == false {
		t.Error(
			"node A did not receive block request message from node B or node C",
			"\nreceived:", hasSeenA,
			"\nexpected:", true,
		)
	}

	hasSeenC := nodeC.gossip.hasSeen[TestMessage.IDString()]
	if hasSeenC == false {
		t.Error(
			"node C did not receive block request message from node A or node B",
			"\nreceived:", hasSeenC,
			"\nexpected:", true,
		)
	}
}
