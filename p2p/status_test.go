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

	"github.com/ChainSafe/gossamer/common"
)

// wait time for status messages to be exchanged and handled
var TestStatusTimeout = time.Second

// test status message
var TestStatusMessage = &StatusMessage{
	ProtocolVersion:     0,
	MinSupportedVersion: 0,
	Roles:               0,
	BestBlockNumber:     0,
	BestBlockHash:       common.Hash{0x00},
	GenesisHash:         common.Hash{0x00},
	ChainStatus:         []byte{0},
}

// test exchange status messages after peer connected
func TestStatus(t *testing.T) {
	configA := &Config{
		Port:        7001,
		RandSeed:    1,
		NoBootstrap: true,
		NoGossip:    true,
		NoMdns:      true,
	}

	nodeA, _, msgRecA := createTestService(t, configA)
	defer nodeA.Stop()

	// simulate host status message sent from core service on startup
	msgRecA <- TestStatusMessage

	configB := &Config{
		Port:        7002,
		RandSeed:    2,
		NoBootstrap: true,
		NoGossip:    true,
		NoMdns:      true,
	}

	nodeB, _, msgRecB := createTestService(t, configB)
	defer nodeB.Stop()

	// simulate host status message sent from core service on startup
	msgRecB <- TestStatusMessage

	addrInfoB, err := nodeB.host.addrInfo()
	if err != nil {
		t.Fatal(err)
	}

	err = nodeA.host.connect(*addrInfoB)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(TestStatusTimeout)

	if !nodeA.status.confirmed(nodeB.host.h.ID()) {
		t.Error("node A did not confirm status of node B")
	}

	if !nodeB.status.confirmed(nodeA.host.h.ID()) {
		t.Error("node B did not confirm status of node A")
	}
}
