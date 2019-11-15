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
	"fmt"
	"io/ioutil"
	"math/big"
	"os"
	"path"
	"reflect"
	"testing"
	"time"

	"github.com/ChainSafe/gossamer/common"
	"github.com/ChainSafe/gossamer/common/optional"
	net "github.com/libp2p/go-libp2p-core/network"
	peer "github.com/libp2p/go-libp2p-core/peer"
	ps "github.com/libp2p/go-libp2p-core/peerstore"
	ma "github.com/multiformats/go-multiaddr"
)

func startNewService(t *testing.T, cfg *Config, msgSendChan chan []byte, msgRecChan chan BlockAnnounceMessage) *Service {
	node, err := NewService(cfg, msgSendChan, msgRecChan)
	if err != nil {
		t.Fatal(err)
	}

	err = node.Start()
	if err != nil {
		t.Fatal(err)
	}

	return node
}

func TestBuildOpts(t *testing.T) {
	tmpPDir, err := ioutil.TempDir(os.TempDir(), "p2p-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpPDir)
	tmpDir := path.Join(tmpPDir, "data")
	err = os.Mkdir(tmpDir, os.ModePerm+os.ModeDir)
	if err != nil {
		t.Fatal(err)
	}

	testCfgA := &Config{
		BootstrapNodes: []string{},
		Port:           7001,
		DataDir:        tmpDir,
	}

	_, err = testCfgA.buildOpts()
	if err != nil {
		t.Fatalf("TestBuildOpts error: %s", err)
	}

	if testCfgA.privateKey == nil {
		t.Error("Private key is nil")
	}

	testCfgB := &Config{
		BootstrapNodes: []string{},
		Port:           7001,
		DataDir:        tmpDir,
	}

	_, err = testCfgB.buildOpts()
	if err != nil {
		t.Fatalf("TestBuildOpts error: %s", err)
	}

	if testCfgA.privateKey == testCfgB.privateKey {
		t.Error("Private key does not match first key generated")
	}
}

func TestBootstrapConnect(t *testing.T) {
	bootnodeCfg := &Config{
		BootstrapNodes: nil,
		Port:           7000,
		RandSeed:       1,
		NoBootstrap:    true,
		NoMdns:         true,
	}

	bootnode := startNewService(t, bootnodeCfg, nil, nil)
	defer bootnode.Stop()
	bootnodeAddr := bootnode.host.fullAddrs()[0]

	nodeCfg := &Config{
		BootstrapNodes: []string{bootnodeAddr.String()},
		Port:           7001,
		RandSeed:       2,
		NoBootstrap:    false,
		NoMdns:         true,
	}

	node := startNewService(t, nodeCfg, nil, nil)
	defer node.Stop()
	// Allow everything to finish connecting
	time.Sleep(1 * time.Second)

	if bootnode.host.peerCount() != 1 {
		t.Errorf("expected peer count: %d got: %d", 1, bootnode.host.peerCount())
	}
}

func TestNoBootstrap(t *testing.T) {
	testServiceConfigA := &Config{
		NoBootstrap: true,
		Port:        7006,
		RandSeed:    1,
	}

	sa := startNewService(t, testServiceConfigA, nil, nil)
	sa.Stop()
}

func TestService_PeerCount(t *testing.T) {
	testServiceConfigA := &Config{
		NoBootstrap: true,
		Port:        7002,
		RandSeed:    1,
	}

	sa := startNewService(t, testServiceConfigA, nil, nil)
	defer sa.Stop()

	testServiceConfigB := &Config{
		NoBootstrap: true,
		Port:        7003,
		RandSeed:    2,
	}

	sb := startNewService(t, testServiceConfigB, nil, nil)
	defer sb.Stop()

	sb.host.h.Peerstore().AddAddrs(sa.host.h.ID(), sa.host.h.Addrs(), ps.PermanentAddrTTL)
	addr, err := ma.NewMultiaddr(fmt.Sprintf("%s/p2p/%s", sa.host.h.Addrs()[0].String(), sa.host.h.ID()))
	if err != nil {
		t.Fatal(err)
	}

	addrInfo, err := peer.AddrInfoFromP2pAddr(addr)
	if err != nil {
		t.Fatal(err)
	}

	err = sb.host.connect(*addrInfo)
	if err != nil {
		t.Fatal(err)
	}

	count := sb.host.peerCount()
	if count == 0 {
		t.Fatalf("incorrect peerCount got %d", count)
	}
}

func TestPing(t *testing.T) {
	testServiceConfigA := &Config{
		NoBootstrap: true,
		Port:        7004,
		RandSeed:    1,
		DataDir:     path.Join(os.TempDir(), "gossamer"),
	}

	sa := startNewService(t, testServiceConfigA, nil, nil)
	defer sa.Stop()

	testServiceConfigB := &Config{
		NoBootstrap: true,
		Port:        7005,
		RandSeed:    2,
		DataDir:     path.Join(os.TempDir(), "gossamer2"),
	}

	msgChan := make(chan []byte)
	sb := startNewService(t, testServiceConfigB, msgChan, nil)
	defer sb.Stop()

	sb.host.h.Peerstore().AddAddrs(sa.host.h.ID(), sa.host.h.Addrs(), ps.PermanentAddrTTL)
	addr, err := ma.NewMultiaddr(fmt.Sprintf("%s/p2p/%s", sa.host.h.Addrs()[0].String(), sa.host.h.ID()))
	if err != nil {
		t.Fatal(err)
	}

	addrInfo, err := peer.AddrInfoFromP2pAddr(addr)
	if err != nil {
		t.Fatal(err)
	}

	err = sb.host.connect(*addrInfo)
	if err != nil {
		t.Fatal(err)
	}

	err = sb.host.ping(addrInfo.ID)
	if err != nil {
		t.Error(err)
	}
}

func TestSend(t *testing.T) {
	testServiceConfigA := &Config{
		NoBootstrap: true,
		Port:        7004,
		RandSeed:    1,
		DataDir:     path.Join(os.TempDir(), "gossamer"),
	}

	sa := startNewService(t, testServiceConfigA, nil, nil)
	defer sa.Stop()

	testServiceConfigB := &Config{
		NoBootstrap: true,
		Port:        7005,
		RandSeed:    2,
		DataDir:     path.Join(os.TempDir(), "gossamer2"),
	}

	msgChan := make(chan []byte)
	sb := startNewService(t, testServiceConfigB, msgChan, nil)
	defer sb.Stop()

	sb.host.h.Peerstore().AddAddrs(sa.host.h.ID(), sa.host.h.Addrs(), ps.PermanentAddrTTL)
	addr, err := ma.NewMultiaddr(fmt.Sprintf("%s/p2p/%s", sa.host.h.Addrs()[0].String(), sa.host.h.ID()))
	if err != nil {
		t.Fatal(err)
	}

	addrInfo, err := peer.AddrInfoFromP2pAddr(addr)
	if err != nil {
		t.Fatal(err)
	}

	err = sb.host.connect(*addrInfo)
	if err != nil {
		t.Fatal(err)
	}

	p, err := sa.host.dht.FindPeer(sa.ctx, sb.host.h.ID())
	if err != nil {
		t.Fatalf("could not find peer: %s", err)
	}

	endBlock, err := common.HexToHash("0xfd19d9ebac759c993fd2e05a1cff9e757d8741c2704c8682c15b5503496b6aa1")
	if err != nil {
		t.Fatal(err)
	}

	bm := &BlockRequestMessage{
		ID:            7,
		RequestedData: 1,
		StartingBlock: []byte{1, 1},
		EndBlockHash:  optional.NewHash(true, endBlock),
		Direction:     1,
		Max:           optional.NewUint32(true, 1),
	}

	encMsg, err := bm.Encode()
	if err != nil {
		t.Fatal(err)
	}

	err = sa.host.send(p, encMsg)
	if err != nil {
		t.Errorf("Send error: %s", err)
	}

	select {
	case <-msgChan:
	case <-time.After(30 * time.Second):
		t.Fatalf("Did not receive message from %s", sa.host.hostAddr)
	}
}

func TestGossiping(t *testing.T) {
	//Start node A
	nodeConfigA := &Config{
		BootstrapNodes: nil,
		Port:           7000,
		NoBootstrap:    true,
		NoMdns:         true,
		RandSeed:       1,
	}

	nodeA := startNewService(t, nodeConfigA, nil, nil)
	defer nodeA.Stop()
	nodeAAddr := nodeA.host.fullAddrs()[0]

	//Start node B
	nodeConfigB := &Config{
		BootstrapNodes: []string{
			nodeAAddr.String(),
		},
		Port:     7001,
		NoMdns:   true,
		RandSeed: 2,
	}

	msgChanB := make(chan []byte)
	nodeB := startNewService(t, nodeConfigB, msgChanB, nil)
	defer nodeB.Stop()
	nodeBAddr := nodeB.host.fullAddrs()[0]

	//Start node C
	nodeConfigC := &Config{
		BootstrapNodes: []string{
			nodeBAddr.String(),
		},
		Port:     7002,
		NoMdns:   true,
		RandSeed: 3,
	}

	msgChanC := make(chan []byte)
	nodeC := startNewService(t, nodeConfigC, msgChanC, nil)
	defer nodeC.Stop()

	// Meaningless hash
	endBlock, err := common.HexToHash("0xfd19d9ebac759c993fd2e05a1cff9e757d8741c2704c8682c15b5503496b6aa1")
	if err != nil {
		t.Errorf("Can't convert hex to hash")
	}

	// Create mock BlockRequestMessage to broadcast
	bm := &BlockRequestMessage{
		ID:            7,
		RequestedData: 1,
		StartingBlock: []byte{1, 1},
		EndBlockHash:  optional.NewHash(true, endBlock),
		Direction:     1,
		Max:           optional.NewUint32(true, 1),
	}

	err = nodeA.Broadcast(bm)
	if err != nil {
		t.Error(err)
	}

	// Check returned values from channels in the 2 other nodes
	select {
	case res := <-msgChanB:
		bmEnc, err := bm.Encode()
		if err != nil {
			t.Fatalf("Can't decode original message")
		}

		// Compare the byte arrays of the original & returned message
		if !reflect.DeepEqual(bmEnc, res) {
			t.Fatalf("Didn't receive the correct message")
		}
	case <-time.After(30 * time.Second):
		t.Fatalf("Did not receive message from %s", nodeA.host.hostAddr)
	}
	select {
	case res := <-msgChanC:
		bmEnc, err := bm.Encode()
		if err != nil {
			t.Fatalf("Can't decode original message")
		}

		// Compare the byte arrays of the original & returned message
		if !reflect.DeepEqual(bmEnc, res) {
			t.Fatalf("Didn't receive the correct message")
		}
	case <-time.After(30 * time.Second):
		t.Fatalf("Did not receive message from %s", nodeB.host.hostAddr)
	}

}

func TestP2pReceiveChan(t *testing.T) {
	//Create nodeA
	testServiceConfigA := &Config{
		BootstrapNodes: nil,
		NoBootstrap:    true,
		NoMdns:         true,
		Port:           7000,
		RandSeed:       1,
	}

	msgRecChan := make(chan BlockAnnounceMessage)

	nodeA := startNewService(t, testServiceConfigA, nil, msgRecChan)
	defer nodeA.Stop()

	// Create nodeB
	nodeAAddr := nodeA.host.fullAddrs()[0]
	testServiceConfigB := &Config{
		BootstrapNodes: []string{
			nodeAAddr.String(),
		},
		Port:     7001,
		NoMdns:   true,
		RandSeed: 2,
	}
	msgSendChan := make(chan []byte)
	nodeB := startNewService(t, testServiceConfigB, msgSendChan, nil)
	defer nodeB.Stop()

	// node B only handles the stream, and doesn't need to rebroadcast it
	nodeB.host.registerStreamHandler(func(stream net.Stream) {

		_, rawMsg, err := parseMessage(stream)

		nodeB.msgSendChan <- rawMsg

		if err != nil {
			return
		}
	})

	//Create a blockAnnounceMessage & send to node A for broadcasting
	blockAnnounceMsg := BlockAnnounceMessage{
		Number: big.NewInt(10),
	}

	encodedBlockAnnounceMsg, err := blockAnnounceMsg.Encode()
	if err != nil {
		t.Fatalf("Can't encode the block announce message")
	}

	// Send message down the nodeA receive channel
	msgRecChan <- blockAnnounceMsg

	// Check that we receive the same message in nodeB
	select {
	case receivedBlockAnnounceMsg := <-msgSendChan:
		if !reflect.DeepEqual(receivedBlockAnnounceMsg, encodedBlockAnnounceMsg) {
			t.Fatalf("Node B service didn't receive the correct block\ngot: %+v\nexpected: %+v", receivedBlockAnnounceMsg, encodedBlockAnnounceMsg)
		}
	case <-time.After(30 * time.Second):
		t.Fatalf("Did not receive message %+v", encodedBlockAnnounceMsg)
	}
}
