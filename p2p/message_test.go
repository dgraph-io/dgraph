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
	"bytes"
	"math/big"
	"reflect"
	"testing"

	"github.com/ChainSafe/gossamer/common"
	"github.com/ChainSafe/gossamer/common/optional"
)

func TestDecodeMessageStatus(t *testing.T) {
	// this value is a concatenation of:
	// message type: byte(0)
	// protocol version uint32(2)
	// minimum supported version: uint32(2)
	// roles: byte(4)
	// best block number: uint64(2418625)
	// best block hash: 32 bytes hash 0x8dac4bd53582976cd2834b47d3c7b3a9c8c708db84b3bae145753547ec9ee4da
	// genesis hash: 32 byte hash 0xdcd1346701ca8396496e52aa2785b1748deb6db09551b72159dcb3e08991025b
	// chain status: []byte{0}
	encMsg, err := common.HexToBytes("0x00020000000200000004c1e72400000000008dac4bd53582976cd2834b47d3c7b3a9c8c708db84b3bae145753547ec9ee4dadcd1346701ca8396496e52aa2785b1748deb6db09551b72159dcb3e08991025b0400")
	if err != nil {
		t.Fatal(err)
	}

	buf := &bytes.Buffer{}
	buf.Write(encMsg)

	m, err := DecodeMessage(buf)
	if err != nil {
		t.Fatal(err)
	}

	genesisHash, err := common.HexToHash("0xdcd1346701ca8396496e52aa2785b1748deb6db09551b72159dcb3e08991025b")
	if err != nil {
		t.Fatal(err)
	}

	bestBlockHash, err := common.HexToHash("0x8dac4bd53582976cd2834b47d3c7b3a9c8c708db84b3bae145753547ec9ee4da")
	if err != nil {
		t.Fatal(err)
	}

	sm := m.(*StatusMessage)
	if sm.ProtocolVersion != 2 {
		t.Error("did not get correct ProtocolVersion")
	} else if sm.MinSupportedVersion != 2 {
		t.Error("did not get correct MinSupportedVersion")
	} else if sm.Roles != byte(4) {
		t.Error("did not get correct Roles")
	} else if sm.BestBlockNumber != 2418625 {
		t.Error("did not get correct BestBlockNumber")
	} else if !bytes.Equal(sm.BestBlockHash.ToBytes(), bestBlockHash.ToBytes()) {
		t.Error("did not get correct BestBlockHash")
	} else if !bytes.Equal(sm.GenesisHash.ToBytes(), genesisHash.ToBytes()) {
		t.Error("did not get correct BestBlockHash")
	} else if !bytes.Equal(sm.ChainStatus, []byte{0}) {
		t.Error("did not get correct ChainStatus")
	}
}

func TestDecodeMessageBlockRequest(t *testing.T) {
	encMsg, err := common.HexToBytes("0x0107000000000000000100dcd1346701ca8396496e52aa2785b1748deb6db09551b72159dcb3e08991025b01fd19d9ebac759c993fd2e05a1cff9e757d8741c2704c8682c15b5503496b6aa1010101000000")
	if err != nil {
		t.Fatal(err)
	}

	buf := &bytes.Buffer{}
	buf.Write(encMsg)

	m, err := DecodeMessage(buf)
	if err != nil {
		t.Fatal(err)
	}

	genesisHash, err := common.HexToBytes("0xdcd1346701ca8396496e52aa2785b1748deb6db09551b72159dcb3e08991025b")
	if err != nil {
		t.Fatal(err)
	}

	endBlock, err := common.HexToHash("0xfd19d9ebac759c993fd2e05a1cff9e757d8741c2704c8682c15b5503496b6aa1")
	if err != nil {
		t.Fatal(err)
	}

	expected := &BlockRequestMessage{
		Id:            7,
		RequestedData: 1,
		StartingBlock: append([]byte{0}, genesisHash...),
		EndBlockHash:  optional.NewHash(true, endBlock),
		Direction:     1,
		Max:           optional.NewUint32(true, 1),
	}

	bm := m.(*BlockRequestMessage)
	if !reflect.DeepEqual(bm, expected) {
		t.Fatalf("Fail: got %v expected %v", bm, expected)
	}
}

func TestDecodeMessageBlockResponse(t *testing.T) {
	encMsg, err := common.HexToBytes("0x02070000000000000000")
	if err != nil {
		t.Fatal(err)
	}

	buf := &bytes.Buffer{}
	buf.Write(encMsg)

	m, err := DecodeMessage(buf)
	if err != nil {
		t.Fatal(err)
	}

	expected := &BlockResponseMessage{
		Id:   7,
		Data: []byte{0},
	}

	bm := m.(*BlockResponseMessage)
	if !reflect.DeepEqual(bm, expected) {
		t.Fatalf("Fail: got %v expected %v", bm, expected)
	}
}

func TestEncodeStatusMessage(t *testing.T) {
	genesisHash, err := common.HexToHash("0xdcd1346701ca8396496e52aa2785b1748deb6db09551b72159dcb3e08991025b")
	if err != nil {
		t.Fatal(err)
	}

	bestBlockHash, err := common.HexToHash("0x829de6be9a35b55c794c609c060698b549b3064c183504c18ab7517e41255569")
	if err != nil {
		t.Fatal(err)
	}

	testStatusMessage := &StatusMessage{
		ProtocolVersion:     uint32(2),
		MinSupportedVersion: uint32(2),
		Roles:               byte(4),
		BestBlockNumber:     uint64(2434417),
		BestBlockHash:       bestBlockHash,
		GenesisHash:         genesisHash,
		ChainStatus:         []byte{0},
	}

	encStatus, err := testStatusMessage.Encode()
	if err != nil {
		t.Fatal(err)
	}

	// from Alexander testnet
	expected, err := common.HexToBytes("0x000200000002000000047125250000000000829de6be9a35b55c794c609c060698b549b3064c183504c18ab7517e41255569dcd1346701ca8396496e52aa2785b1748deb6db09551b72159dcb3e08991025b0400")
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(encStatus, expected) {
		t.Errorf("Fail: got %x expected %x", encStatus, expected)
	}
}

func TestEncodeBlockRequestMessage_BlockHash(t *testing.T) {
	// this value is a concatenation of:
	// message type: byte(1)
	// message ID: uint32(7)
	// requested data: byte(1)
	// starting block: 1 byte to identify type + 32 byte hash: byte(0) + hash(dcd1346701ca8396496e52aa2785b1748deb6db09551b72159dcb3e08991025b)
	// end block hash: 32 bytes: hash(fd19d9ebac759c993fd2e05a1cff9e757d8741c2704c8682c15b5503496b6aa1)
	// direction: byte(1)
	// max: uint32(1)
	expected, err := common.HexToBytes("0x0107000000000000000100dcd1346701ca8396496e52aa2785b1748deb6db09551b72159dcb3e08991025b01fd19d9ebac759c993fd2e05a1cff9e757d8741c2704c8682c15b5503496b6aa1010101000000")
	if err != nil {
		t.Fatal(err)
	}

	genesisHash, err := common.HexToBytes("0xdcd1346701ca8396496e52aa2785b1748deb6db09551b72159dcb3e08991025b")
	if err != nil {
		t.Fatal(err)
	}

	endBlock, err := common.HexToHash("0xfd19d9ebac759c993fd2e05a1cff9e757d8741c2704c8682c15b5503496b6aa1")
	if err != nil {
		t.Fatal(err)
	}

	bm := &BlockRequestMessage{
		Id:            7,
		RequestedData: 1,
		StartingBlock: append([]byte{0}, genesisHash...),
		EndBlockHash:  optional.NewHash(true, endBlock),
		Direction:     1,
		Max:           optional.NewUint32(true, 1),
	}

	encMsg, err := bm.Encode()
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(encMsg, expected) {
		t.Fatalf("Fail: got %x expected %x", encMsg, expected)
	}
}

func TestEncodeBlockRequestMessage_BlockNumber(t *testing.T) {
	// this value is a concatenation of:
	// message type: byte(1)
	// message ID: uint32(7)
	// requested data: byte(1)
	// starting block: 1 byte to identify type + 8 byte number: byte(1) + uint64(1)
	// end block hash: 32 bytes: hash(fd19d9ebac759c993fd2e05a1cff9e757d8741c2704c8682c15b5503496b6aa1)
	// direction: byte(1)
	// max: uint32(1)
	expected, err := common.HexToBytes("0x0107000000000000000101010000000000000001fd19d9ebac759c993fd2e05a1cff9e757d8741c2704c8682c15b5503496b6aa1010101000000")
	if err != nil {
		t.Fatal(err)
	}

	endBlock, err := common.HexToHash("0xfd19d9ebac759c993fd2e05a1cff9e757d8741c2704c8682c15b5503496b6aa1")
	if err != nil {
		t.Fatal(err)
	}

	bm := &BlockRequestMessage{
		Id:            7,
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

	if !bytes.Equal(encMsg, expected) {
		t.Fatalf("Fail: got %x expected %x", encMsg, expected)
	}
}

func TestDecodeBlockRequestMessage_BlockNumber(t *testing.T) {
	// this value is a concatenation of:
	// message type: byte(1)
	// message ID: uint32(7)
	// requested data: byte(1)
	// starting block: 1 byte to identify type + 8 byte number: byte(1) + uint64(1)
	// end block hash: 32 bytes: hash(fd19d9ebac759c993fd2e05a1cff9e757d8741c2704c8682c15b5503496b6aa1)
	// direction: byte(1)
	// max: uint32(1)
	encMsg, err := common.HexToBytes("0x0107000000000000000101010000000000000001fd19d9ebac759c993fd2e05a1cff9e757d8741c2704c8682c15b5503496b6aa1010101000000")
	if err != nil {
		t.Fatal(err)
	}

	buf := &bytes.Buffer{}
	buf.Write(encMsg)

	m, err := DecodeMessage(buf)
	if err != nil {
		t.Fatal(err)
	}

	endBlock, err := common.HexToHash("0xfd19d9ebac759c993fd2e05a1cff9e757d8741c2704c8682c15b5503496b6aa1")
	if err != nil {
		t.Fatal(err)
	}

	expected := &BlockRequestMessage{
		Id:            7,
		RequestedData: 1,
		StartingBlock: []byte{1, 1, 0, 0, 0, 0, 0, 0, 0},
		EndBlockHash:  optional.NewHash(true, endBlock),
		Direction:     1,
		Max:           optional.NewUint32(true, 1),
	}

	bm := m.(*BlockRequestMessage)
	if !reflect.DeepEqual(bm, expected) {
		t.Fatalf("Fail: got %v expected %v", bm, expected)
	}
}

func TestEncodeBlockRequestMessage_NoOptionals(t *testing.T) {
	// this value is a concatenation of:
	// message type: byte(1)
	// message ID: uint32(7)
	// requested data: byte(1)
	// starting block: 1 byte to identify type + 8 byte number: byte(1) + uint64(1)
	// end block hash: byte(0)
	// direction: byte(1)
	// max: byte(0)
	expected, err := common.HexToBytes("0x0107000000000000000100dcd1346701ca8396496e52aa2785b1748deb6db09551b72159dcb3e08991025b0000010000")
	if err != nil {
		t.Fatal(err)
	}

	genesisHash, err := common.HexToBytes("0xdcd1346701ca8396496e52aa2785b1748deb6db09551b72159dcb3e08991025b")
	if err != nil {
		t.Fatal(err)
	}

	bm := &BlockRequestMessage{
		Id:            7,
		RequestedData: 1,
		StartingBlock: append([]byte{0}, genesisHash...),
		EndBlockHash:  optional.NewHash(false, common.Hash{}),
		Direction:     1,
		Max:           optional.NewUint32(false, 0),
	}

	encMsg, err := bm.Encode()
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(encMsg, expected) {
		t.Fatalf("Fail: got %x expected %x", encMsg, expected)
	}
}

func TestDecodeBlockRequestMessage_NoOptionals(t *testing.T) {
	encMsg, err := common.HexToBytes("0x0107000000000000000100dcd1346701ca8396496e52aa2785b1748deb6db09551b72159dcb3e08991025b000100")

	if err != nil {
		t.Fatal(err)
	}

	buf := &bytes.Buffer{}
	buf.Write(encMsg)

	m, err := DecodeMessage(buf)
	if err != nil {
		t.Fatal(err)
	}

	genesisHash, err := common.HexToBytes("0xdcd1346701ca8396496e52aa2785b1748deb6db09551b72159dcb3e08991025b")
	if err != nil {
		t.Fatal(err)
	}

	expected := &BlockRequestMessage{
		Id:            7,
		RequestedData: 1,
		StartingBlock: append([]byte{0}, genesisHash...),
		EndBlockHash:  optional.NewHash(false, common.Hash{}),
		Direction:     1,
		Max:           optional.NewUint32(false, 0),
	}

	bm := m.(*BlockRequestMessage)
	if !reflect.DeepEqual(bm, expected) {
		t.Fatalf("Fail: got %v expected %v", bm, expected)
	}
}

func TestEncodeBlockResponseMessage(t *testing.T) {
	expected, err := common.HexToBytes("0x02070000000000000000")

	if err != nil {
		t.Fatal(err)
	}

	bm := &BlockResponseMessage{
		Id:   7,
		Data: []byte{0},
	}

	encMsg, err := bm.Encode()
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(encMsg, expected) {
		t.Fatalf("Fail: got %x expected %x", encMsg, expected)
	}
}

func TestDecodeBlockResponseMessage(t *testing.T) {
	encMsg, err := common.HexToBytes("0x070000000000000000")
	if err != nil {
		t.Fatal(err)
	}

	buf := &bytes.Buffer{}
	buf.Write(encMsg)

	m := new(BlockResponseMessage)
	err = m.Decode(buf)
	if err != nil {
		t.Fatal(err)
	}

	expected := &BlockResponseMessage{
		Id:   7,
		Data: []byte{0},
	}

	if !reflect.DeepEqual(m, expected) {
		t.Fatalf("Fail: got %v expected %v", m, expected)
	}
}

func TestEncodeBlockAnnounceMessage(t *testing.T) {
	// this value is a concatenation of:
	//  message type:  byte(3)
	//  ParentHash: Hash: 0x4545454545454545454545454545454545454545454545454545454545454545
	//	Number: *big.Int // block number: 1
	//	StateRoot:  Hash: 0xb3266de137d20a5d0ff3a6401eb57127525fd9b2693701f0bf5a8a853fa3ebe0
	//	ExtrinsicsRoot: Hash: 0x03170a2e7597b7b7e3d84c05391d139a62b157e78786d8c082f29dcf4c111314
	//	Digest: []byte

	//                                    mtparenthash                                                      bnstateroot                                                       extrinsicsroot                                                  di
	expected, err := common.HexToBytes("0x03454545454545454545454545454545454545454545454545454545454545454504b3266de137d20a5d0ff3a6401eb57127525fd9b2693701f0bf5a8a853fa3ebe003170a2e7597b7b7e3d84c05391d139a62b157e78786d8c082f29dcf4c11131400")
	if err != nil {
		t.Fatal(err)
	}

	parentHash, err := common.HexToHash("0x4545454545454545454545454545454545454545454545454545454545454545")
	if err != nil {
		t.Fatal(err)
	}
	stateRoot, err := common.HexToHash("0xb3266de137d20a5d0ff3a6401eb57127525fd9b2693701f0bf5a8a853fa3ebe0")
	if err != nil {
		t.Fatal(err)
	}
	extrinsicsRoot, err := common.HexToHash("0x03170a2e7597b7b7e3d84c05391d139a62b157e78786d8c082f29dcf4c111314")
	if err != nil {
		t.Fatal(err)
	}

	bhm := &BlockHeaderMessage{
		ParentHash:     parentHash,
		Number:         big.NewInt(1),
		StateRoot:      stateRoot,
		ExtrinsicsRoot: extrinsicsRoot,
		Digest:         []byte{},
	}
	encMsg, err := bhm.Encode()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(encMsg, expected) {
		t.Fatalf("Fail: got %x expected %x", encMsg, expected)
	}
}

func TestDecodeBlockAnnounceMessage(t *testing.T) {
	announceMessage, err := common.HexToBytes("0x454545454545454545454545454545454545454545454545454545454545454504b3266de137d20a5d0ff3a6401eb57127525fd9b2693701f0bf5a8a853fa3ebe003170a2e7597b7b7e3d84c05391d139a62b157e78786d8c082f29dcf4c11131400")
	if err != nil {
		t.Fatal(err)
	}

	bhm := new(BlockHeaderMessage)
	err = bhm.Decode(announceMessage)
	if err != nil {
		t.Fatal(err)
	}

	parentHash, err := common.HexToHash("0x4545454545454545454545454545454545454545454545454545454545454545")
	if err != nil {
		t.Fatal(err)
	}
	stateRoot, err := common.HexToHash("0xb3266de137d20a5d0ff3a6401eb57127525fd9b2693701f0bf5a8a853fa3ebe0")
	if err != nil {
		t.Fatal(err)
	}
	extrinsicsRoot, err := common.HexToHash("0x03170a2e7597b7b7e3d84c05391d139a62b157e78786d8c082f29dcf4c111314")
	if err != nil {
		t.Fatal(err)
	}
	expected := &BlockHeaderMessage{
		ParentHash:     parentHash,
		Number:         big.NewInt(1),
		StateRoot:      stateRoot,
		ExtrinsicsRoot: extrinsicsRoot,
		Digest:         []byte{},
	}

	if !reflect.DeepEqual(bhm, expected) {
		t.Fatalf("Fail: got %v expected %v", bhm, expected)
	}
}
