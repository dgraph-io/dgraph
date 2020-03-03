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
	"path"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/ChainSafe/gossamer/lib/common/optional"
)

var TestProtocolID = "/gossamer/test/0"

// arbitrary block request message
var TestMessage = &BlockRequestMessage{
	ID:            1,
	RequestedData: 1,
	// TODO: investigate starting block mismatch with different slice length
	StartingBlock: []byte{1, 1, 1, 1, 1, 1, 1, 1, 1},
	EndBlockHash:  optional.NewHash(true, common.Hash{}),
	Direction:     1,
	Max:           optional.NewUint32(true, 1),
}

// maximum wait time for non-status message to be handled
var TestMessageTimeout = 3 * time.Second

// time between connection retries (BackoffBase default 5 seconds)
var TestBackoffTimeout = 5 * time.Second

// newTestDataDir returns the path of a test directory
func newTestDataDir(t *testing.T, name string) string {
	return path.Join(os.TempDir(), "gossamer-test", t.Name(), name)
}

// failedToDial returns true if "failed to dial" error, otherwise false
func failedToDial(err error) bool {
	return err != nil && strings.Contains(err.Error(), "failed to dial")
}

// helper method to create and start a new network service
func createTestService(t *testing.T, cfg *Config) (node *Service, msgSend chan Message, msgRec chan Message) {
	msgRec = make(chan Message)
	msgSend = make(chan Message)

	// same for all network tests use the createTestService helper method
	cfg.BlockState = &MockBlockState{} // required
	cfg.NetworkState = &MockNetworkState{}
	cfg.ProtocolID = TestProtocolID // default "/gossamer/dot/0"

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

// test network service starts
func TestStartService(t *testing.T) {
	dataDir := path.Join(os.TempDir(), "gossamer-test", "node")
	defer os.RemoveAll(dataDir)

	config := &Config{
		DataDir:     dataDir,
		Port:        7001,
		RandSeed:    1,
		NoBootstrap: true,
		NoMdns:      true,
	}
	node, _, _ := createTestService(t, config)
	node.Stop()
}

// test broacast messages from core service
func TestBroadcastMessages(t *testing.T) {
	dataDirA := newTestDataDir(t, "nodeA")
	defer os.RemoveAll(dataDirA)

	configA := &Config{
		DataDir:     dataDirA,
		Port:        7001,
		RandSeed:    1,
		NoBootstrap: true,
		NoMdns:      true,
	}

	nodeA, _, msgRecA := createTestService(t, configA)
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
		NoMdns:      true,
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
