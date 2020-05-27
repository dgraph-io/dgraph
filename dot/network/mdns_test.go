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

// wait time to discover and connect using mdns discovery
var TestMDNSTimeout = time.Second

// test mdns discovery service (discovers and connects)
func TestMDNS(t *testing.T) {
	basePathA := utils.NewTestBasePath(t, "nodeA")

	// removes all data directories created within test directory
	defer utils.RemoveTestDir(t)

	configA := &Config{
		BasePath:    basePathA,
		Port:        7001,
		RandSeed:    1,
		NoBootstrap: true,
	}

	nodeA := createTestService(t, configA)
	defer nodeA.Stop()

	nodeA.noGossip = true
	nodeA.noStatus = true

	basePathB := utils.NewTestBasePath(t, "nodeB")

	configB := &Config{
		BasePath:    basePathB,
		Port:        7002,
		RandSeed:    2,
		NoBootstrap: true,
	}

	nodeB := createTestService(t, configB)
	defer nodeB.Stop()

	nodeB.noGossip = true
	nodeB.noStatus = true

	time.Sleep(TestMDNSTimeout)

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
