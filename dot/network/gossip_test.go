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
	"testing"
	"time"

	"github.com/ChainSafe/gossamer/lib/utils"
)

// test gossip messages to connected peers
func TestGossip(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping TestGossip")
	}
	basePathA := utils.NewTestBasePath(t, "nodeA")

	// removes all data directories created within test directory
	defer utils.RemoveTestDir(t)

	mmhA := new(MockMessageHandler)

	configA := &Config{
		BasePath:       basePathA,
		Port:           7001,
		RandSeed:       1,
		NoBootstrap:    true,
		NoMDNS:         true,
		MessageHandler: mmhA,
	}

	nodeA := createTestService(t, configA)
	defer nodeA.Stop()

	nodeA.noStatus = true

	basePathB := utils.NewTestBasePath(t, "nodeB")

	mmhB := new(MockMessageHandler)

	configB := &Config{
		BasePath:       basePathB,
		Port:           7002,
		RandSeed:       2,
		NoBootstrap:    true,
		NoMDNS:         true,
		MessageHandler: mmhB,
	}

	nodeB := createTestService(t, configB)
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

	basePathC := utils.NewTestBasePath(t, "nodeC")

	mmhC := new(MockMessageHandler)

	configC := &Config{
		BasePath:       basePathC,
		Port:           7003,
		RandSeed:       3,
		NoBootstrap:    true,
		NoMDNS:         true,
		MessageHandler: mmhC,
	}

	nodeC := createTestService(t, configC)
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

	err = nodeA.host.send(addrInfosB[0].ID, "", TestMessage)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(TestMessageTimeout)

	// node A sends message to node B
	if mmhB.Message == nil {
		t.Error("node A timeout waiting for message")
	}

	// node B gossips message to node C
	if mmhC.Message == nil {
		t.Error("node A timeout waiting for message")
	}

	// node C gossips message to node A
	if mmhA.Message == nil {
		t.Error("node A timeout waiting for message")
	}

	// node A gossips message to node B
	if mmhB.Message == nil {
		t.Error("node A timeout waiting for message")
	}

	if hasSeenB, ok := nodeB.gossip.hasSeen.Load(TestMessage.IDString()); !ok || hasSeenB.(bool) == false {
		t.Error(
			"node B did not receive block request message from node A",
			"\nreceived:", hasSeenB,
			"\nexpected:", true,
		)
	}

	if hasSeenC, ok := nodeC.gossip.hasSeen.Load(TestMessage.IDString()); !ok || hasSeenC.(bool) == false {
		t.Error(
			"node C did not receive block request message from node B",
			"\nreceived:", hasSeenC,
			"\nexpected:", true,
		)
	}

	if hasSeenA, ok := nodeA.gossip.hasSeen.Load(TestMessage.IDString()); !ok || hasSeenA.(bool) == false {
		t.Error(
			"node A did not receive block request message from node C",
			"\nreceived:", hasSeenA,
			"\nexpected:", true,
		)
	}
}
