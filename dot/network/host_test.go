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
	"reflect"
	"testing"
	"time"

	"github.com/ChainSafe/gossamer/lib/utils"
	log "github.com/ChainSafe/log15"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/stretchr/testify/require"
)

func TestMessageSize(t *testing.T) {
	size := 100000
	msg := make([]byte, size)
	msg[0] = 77

	nodes := make([]*Service, 2)
	errs := make([]chan error, 2)

	for i := range nodes {
		errCh := make(chan error)

		config := &Config{
			Port:        7000 + uint32(i),
			RandSeed:    1 + int64(i),
			NoBootstrap: true,
			NoMDNS:      true,
			ErrChan:     errCh,
			LogLvl:      log.LvlTrace,
		}
		node := createTestService(t, config)
		defer node.Stop()
		nodes[i] = node
		errs[i] = errCh
	}

	addrs := nodes[0].host.multiaddrs()
	ainfo, err := peer.AddrInfoFromP2pAddr(addrs[1])
	require.NoError(t, err)

	for i, n := range nodes {
		if i == 0 {
			// connect other nodes to first node
			continue
		}

		err = n.host.connect(*ainfo)
		require.NoError(t, err, i)
	}

	err = nodes[0].host.sendBytes(nodes[1].host.id(), "", msg)
	require.NoError(t, err)
	time.Sleep(time.Second)
	err = nodes[1].host.sendBytes(nodes[0].host.id(), "", msg)
	require.NoError(t, err)
	time.Sleep(time.Second)

	select {
	case err := <-errs[0]:
		t.Fatal("got error", err)
	case err := <-errs[1]:
		t.Fatal("got error", err)
	case <-time.After(time.Millisecond * 100):
	}
}

// test host connect method
func TestConnect(t *testing.T) {
	basePathA := utils.NewTestBasePath(t, "nodeA")

	configA := &Config{
		BasePath:    basePathA,
		Port:        7001,
		RandSeed:    1,
		NoBootstrap: true,
		NoMDNS:      true,
	}

	nodeA := createTestService(t, configA)

	nodeA.noGossip = true
	nodeA.noStatus = true

	basePathB := utils.NewTestBasePath(t, "nodeB")

	configB := &Config{
		BasePath:    basePathB,
		Port:        7002,
		RandSeed:    2,
		NoBootstrap: true,
		NoMDNS:      true,
	}

	nodeB := createTestService(t, configB)

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

// test host bootstrap method on start
func TestBootstrap(t *testing.T) {
	basePathA := utils.NewTestBasePath(t, "nodeA")

	configA := &Config{
		BasePath:    basePathA,
		Port:        7001,
		RandSeed:    1,
		NoBootstrap: true,
		NoMDNS:      true,
	}

	nodeA := createTestService(t, configA)

	nodeA.noGossip = true
	nodeA.noStatus = true

	addrA := nodeA.host.multiaddrs()[0]

	basePathB := utils.NewTestBasePath(t, "nodeB")

	configB := &Config{
		BasePath:  basePathB,
		Port:      7002,
		RandSeed:  2,
		Bootnodes: []string{addrA.String()},
		NoMDNS:    true,
	}

	nodeB := createTestService(t, configB)

	nodeB.noGossip = true
	nodeB.noStatus = true

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

// test host send method
func TestSend(t *testing.T) {
	basePathA := utils.NewTestBasePath(t, "nodeA")

	configA := &Config{
		BasePath:    basePathA,
		Port:        7001,
		RandSeed:    1,
		NoBootstrap: true,
		NoMDNS:      true,
	}

	nodeA := createTestService(t, configA)

	nodeA.noGossip = true
	nodeA.noStatus = true

	basePathB := utils.NewTestBasePath(t, "nodeB")

	msgSendB := make(chan Message)

	configB := &Config{
		BasePath:    basePathB,
		Port:        7002,
		RandSeed:    2,
		NoBootstrap: true,
		NoMDNS:      true,
		MsgSend:     msgSendB,
	}

	nodeB := createTestService(t, configB)

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

	err = nodeA.host.send(addrInfosB[0].ID, "", TestMessage)
	if err != nil {
		t.Fatal(err)
	}

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
		t.Error("node B timeout waiting for message from node A")
	}
}

// test host broadcast method
func TestBroadcast(t *testing.T) {
	basePathA := utils.NewTestBasePath(t, "nodeA")

	configA := &Config{
		BasePath:    basePathA,
		Port:        7001,
		RandSeed:    1,
		NoBootstrap: true,
		NoMDNS:      true,
	}

	nodeA := createTestService(t, configA)

	nodeA.noGossip = true
	nodeA.noStatus = true

	basePathB := utils.NewTestBasePath(t, "nodeB")

	msgSendB := make(chan Message)

	configB := &Config{
		BasePath:    basePathB,
		Port:        7002,
		RandSeed:    2,
		NoBootstrap: true,
		NoMDNS:      true,
		MsgSend:     msgSendB,
	}

	nodeB := createTestService(t, configB)

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

	basePathC := utils.NewTestBasePath(t, "")

	msgSendC := make(chan Message)

	configC := &Config{
		BasePath:    basePathC,
		Port:        7003,
		RandSeed:    3,
		NoBootstrap: true,
		NoMDNS:      true,
		MsgSend:     msgSendC,
	}

	nodeC := createTestService(t, configC)

	nodeC.noGossip = true
	nodeC.noStatus = true

	addrInfosC, err := nodeC.host.addrInfos()
	if err != nil {
		t.Fatal(err)
	}

	err = nodeA.host.connect(*addrInfosC[0])
	// retry connect if "failed to dial" error
	if failedToDial(err) {
		time.Sleep(TestBackoffTimeout)
		err = nodeA.host.connect(*addrInfosC[0])
	}
	if err != nil {
		t.Fatal(err)
	}

	nodeA.host.broadcast(TestMessage)

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

	select {
	case msg := <-msgSendC:
		if !reflect.DeepEqual(msg, TestMessage) {
			t.Error(
				"node C received unexpected message from node A",
				"\nexpected:", TestMessage,
				"\nreceived:", msg,
			)
		}
	case <-time.After(TestMessageTimeout):
		t.Error("node C timeout waiting for message")
	}

}

// test host send method with existing stream
func TestExistingStream(t *testing.T) {
	basePathA := utils.NewTestBasePath(t, "nodeA")

	msgSendA := make(chan Message)

	configA := &Config{
		BasePath:    basePathA,
		Port:        7001,
		RandSeed:    1,
		NoBootstrap: true,
		NoMDNS:      true,
		MsgSend:     msgSendA,
	}

	nodeA := createTestService(t, configA)

	nodeA.noGossip = true
	nodeA.noStatus = true

	addrInfosA, err := nodeA.host.addrInfos()
	if err != nil {
		t.Fatal(err)
	}

	basePathB := utils.NewTestBasePath(t, "nodeB")

	msgSendB := make(chan Message)

	configB := &Config{
		BasePath:    basePathB,
		Port:        7002,
		RandSeed:    2,
		NoBootstrap: true,
		NoMDNS:      true,
		MsgSend:     msgSendB,
	}

	nodeB := createTestService(t, configB)

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

	stream := nodeA.host.getStream(nodeB.host.id(), "")
	if stream != nil {
		t.Error("node A should not have an outbound stream")
	}

	// node A opens the stream to send the first message
	err = nodeA.host.send(addrInfosB[0].ID, "", TestMessage)
	if err != nil {
		t.Fatal(err)
	}

	select {
	case <-msgSendB:
	case <-time.After(TestMessageTimeout):
		t.Error("node B timeout waiting for message from node A")
	}

	stream = nodeA.host.getStream(nodeB.host.id(), "")
	if stream == nil {
		t.Error("node A should have an outbound stream")
	}

	// node A uses the stream to send a second message
	err = nodeA.host.send(addrInfosB[0].ID, "", TestMessage)
	if err != nil {
		t.Fatal(err)
	}

	select {
	case <-msgSendB:
	case <-time.After(TestMessageTimeout):
		t.Error("node B timeout waiting for message from node A")
	}

	stream = nodeA.host.getStream(nodeB.host.id(), "")
	if stream == nil {
		t.Error("node A should have an outbound stream")
	}

	stream = nodeB.host.getStream(nodeA.host.id(), "")
	if stream != nil {
		t.Error("node B should not have an outbound stream")
	}

	// node B opens the stream to send the first message
	err = nodeB.host.send(addrInfosA[0].ID, "", TestMessage)
	if err != nil {
		t.Fatal(err)
	}

	select {
	case <-msgSendA:
	case <-time.After(TestMessageTimeout):
		t.Error("node A timeout waiting for message from node B")
	}

	stream = nodeB.host.getStream(nodeA.host.id(), "")
	if stream == nil {
		t.Error("node B should have an outbound stream")
	}

	// node B uses the stream to send a second message
	err = nodeB.host.send(addrInfosA[0].ID, "", TestMessage)
	if err != nil {
		t.Fatal(err)
	}

	select {
	case <-msgSendA:
	case <-time.After(TestMessageTimeout):
		t.Error("node A timeout waiting for message from node B")
	}

	stream = nodeB.host.getStream(nodeA.host.id(), "")
	if stream == nil {
		t.Error("node B should have an outbound stream")
	}
}
