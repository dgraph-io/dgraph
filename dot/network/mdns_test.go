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

// wait time to discover and connect using mdns discovery
var TestMdnsTimeout = time.Second

// test mdns discovery service (discovers and connects)
func TestMdns(t *testing.T) {
	dataDirA := newTestDataDir(t, "nodeA")
	defer os.RemoveAll(dataDirA)

	configA := &Config{
		DataDir:     dataDirA,
		Port:        7001,
		RandSeed:    1,
		NoBootstrap: true,
	}

	nodeA, _, _ := createTestService(t, configA)
	defer nodeA.Stop()

	nodeA.noGossip = true
	nodeA.noStatus = true

	dataDirB := newTestDataDir(t, "nodeB")
	defer os.RemoveAll(dataDirB)

	configB := &Config{
		DataDir:     dataDirB,
		Port:        7002,
		RandSeed:    2,
		NoBootstrap: true,
	}

	nodeB, _, _ := createTestService(t, configB)
	defer nodeB.Stop()

	nodeB.noGossip = true
	nodeB.noStatus = true

	time.Sleep(TestMdnsTimeout)

	peerCountA := nodeA.host.peerCount()
	peerCountB := nodeB.host.peerCount()

	if peerCountA == 0 {
		// check peerstore for disconnected peers
		peerCountA := len(nodeA.host.h.Peerstore().Peers())
		if peerCountA == 0 {
			t.Error(
				"node A does not have expected peer count",
				"\nexpected:", "not zero",
				"\nreceived:", peerCountA,
			)
		}
	}

	if peerCountB == 0 {
		// check peerstore for disconnected peers
		peerCountB := len(nodeB.host.h.Peerstore().Peers())
		if peerCountB == 0 {
			t.Error(
				"node B does not have expected peer count",
				"\nexpected:", "not zero",
				"\nreceived:", peerCountB,
			)
		}
	}
}
