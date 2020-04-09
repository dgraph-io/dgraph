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
	"bytes"
	"encoding/hex"
	"math/big"
	"testing"

	"github.com/ChainSafe/gossamer/dot/types"
	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/ChainSafe/gossamer/lib/common/optional"
	"github.com/ChainSafe/gossamer/lib/common/variadic"

	"github.com/stretchr/testify/require"
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
	require.Nil(t, err)

	buf := &bytes.Buffer{}
	buf.Write(encMsg)

	m, err := decodeMessage(buf)
	require.Nil(t, err)

	genesisHash, err := common.HexToHash("0xdcd1346701ca8396496e52aa2785b1748deb6db09551b72159dcb3e08991025b")
	require.Nil(t, err)

	bestBlockHash, err := common.HexToHash("0x8dac4bd53582976cd2834b47d3c7b3a9c8c708db84b3bae145753547ec9ee4da")
	require.Nil(t, err)

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

	m, err := decodeMessage(buf)
	require.Nil(t, err)

	genesisHash, err := common.HexToBytes("0xdcd1346701ca8396496e52aa2785b1748deb6db09551b72159dcb3e08991025b")
	require.Nil(t, err)

	endBlock, err := common.HexToHash("0xfd19d9ebac759c993fd2e05a1cff9e757d8741c2704c8682c15b5503496b6aa1")
	require.Nil(t, err)

	expected := &BlockRequestMessage{
		ID:            7,
		RequestedData: 1,
		StartingBlock: variadic.NewUint64OrHashFromBytes(append([]byte{0}, genesisHash...)),
		EndBlockHash:  optional.NewHash(true, endBlock),
		Direction:     1,
		Max:           optional.NewUint32(true, 1),
	}

	bm := m.(*BlockRequestMessage)
	require.Equal(t, expected, bm)
}

func TestDecodeMessageBlockResponse(t *testing.T) {
	encMsg, err := common.HexToBytes("0x02070000000000000001000000000000000000000000000000000000000000000000000000000000000000000001000102030405060708090a0b0c0d0e0f000102030405060708090a0b0c0d0e0f04000102030405060708090a0b0c0d0e0f000102030405060708090a0b0c0d0e0f000102030405060708090a0b0c0d0e0f000102030405060708090a0b0c0d0e0f04080e0f00000000")
	require.Nil(t, err)

	buf := &bytes.Buffer{}
	buf.Write(encMsg)

	m, err := decodeMessage(buf)
	require.Nil(t, err)

	hash := common.NewHash([]byte{0})
	testHash := common.NewHash([]byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 0xa, 0xb, 0xc, 0xd, 0xe, 0xf, 0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 0xa, 0xb, 0xc, 0xd, 0xe, 0xf})

	header := &optional.CoreHeader{
		ParentHash:     testHash,
		Number:         big.NewInt(1),
		StateRoot:      testHash,
		ExtrinsicsRoot: testHash,
		Digest:         [][]byte{{0xe, 0xf}},
	}

	bd := &types.BlockData{
		Hash:          hash,
		Header:        optional.NewHeader(true, header),
		Body:          optional.NewBody(false, nil),
		Receipt:       optional.NewBytes(false, nil),
		MessageQueue:  optional.NewBytes(false, nil),
		Justification: optional.NewBytes(false, nil),
	}

	expected := &BlockResponseMessage{
		ID:        7,
		BlockData: []*types.BlockData{bd},
	}

	bm := m.(*BlockResponseMessage)
	require.Equal(t, expected, bm)
}

func TestEncodeStatusMessage(t *testing.T) {
	genesisHash, err := common.HexToHash("0xdcd1346701ca8396496e52aa2785b1748deb6db09551b72159dcb3e08991025b")
	require.Nil(t, err)

	bestBlockHash, err := common.HexToHash("0x829de6be9a35b55c794c609c060698b549b3064c183504c18ab7517e41255569")
	require.Nil(t, err)

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
	require.Nil(t, err)

	// from Alexander testnet
	expected, err := common.HexToBytes("0x000200000002000000047125250000000000829de6be9a35b55c794c609c060698b549b3064c183504c18ab7517e41255569dcd1346701ca8396496e52aa2785b1748deb6db09551b72159dcb3e08991025b0400")
	require.Nil(t, err)

	require.Equal(t, expected, encStatus)
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
	require.Nil(t, err)

	genesisHash, err := common.HexToBytes("0xdcd1346701ca8396496e52aa2785b1748deb6db09551b72159dcb3e08991025b")
	require.Nil(t, err)

	endBlock, err := common.HexToHash("0xfd19d9ebac759c993fd2e05a1cff9e757d8741c2704c8682c15b5503496b6aa1")
	require.Nil(t, err)

	bm := &BlockRequestMessage{
		ID:            7,
		RequestedData: 1,
		StartingBlock: variadic.NewUint64OrHashFromBytes(append([]byte{0}, genesisHash...)),
		EndBlockHash:  optional.NewHash(true, endBlock),
		Direction:     1,
		Max:           optional.NewUint32(true, 1),
	}

	encMsg, err := bm.Encode()
	require.Nil(t, err)

	require.Equal(t, expected, encMsg)
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
	require.Nil(t, err)

	endBlock, err := common.HexToHash("0xfd19d9ebac759c993fd2e05a1cff9e757d8741c2704c8682c15b5503496b6aa1")
	require.Nil(t, err)

	bm := &BlockRequestMessage{
		ID:            7,
		RequestedData: 1,
		StartingBlock: variadic.NewUint64OrHashFromBytes([]byte{1, 1}),
		EndBlockHash:  optional.NewHash(true, endBlock),
		Direction:     1,
		Max:           optional.NewUint32(true, 1),
	}

	encMsg, err := bm.Encode()
	require.Nil(t, err)

	require.Equal(t, expected, encMsg)
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
	require.Nil(t, err)

	buf := &bytes.Buffer{}
	buf.Write(encMsg)

	m, err := decodeMessage(buf)
	require.Nil(t, err)

	endBlock, err := common.HexToHash("0xfd19d9ebac759c993fd2e05a1cff9e757d8741c2704c8682c15b5503496b6aa1")
	require.Nil(t, err)

	expected := &BlockRequestMessage{
		ID:            7,
		RequestedData: 1,
		StartingBlock: variadic.NewUint64OrHashFromBytes([]byte{1, 1, 0, 0, 0, 0, 0, 0, 0}),
		EndBlockHash:  optional.NewHash(true, endBlock),
		Direction:     1,
		Max:           optional.NewUint32(true, 1),
	}

	bm := m.(*BlockRequestMessage)
	require.Equal(t, expected, bm)
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
	require.Nil(t, err)

	genesisHash, err := common.HexToBytes("0xdcd1346701ca8396496e52aa2785b1748deb6db09551b72159dcb3e08991025b")
	require.Nil(t, err)

	bm := &BlockRequestMessage{
		ID:            7,
		RequestedData: 1,
		StartingBlock: variadic.NewUint64OrHashFromBytes(append([]byte{0}, genesisHash...)),
		EndBlockHash:  optional.NewHash(false, common.Hash{}),
		Direction:     1,
		Max:           optional.NewUint32(false, 0),
	}

	encMsg, err := bm.Encode()
	require.Nil(t, err)

	require.Equal(t, expected, encMsg)
}

func TestDecodeBlockRequestMessage_NoOptionals(t *testing.T) {
	encMsg, err := common.HexToBytes("0x0107000000000000000100dcd1346701ca8396496e52aa2785b1748deb6db09551b72159dcb3e08991025b000100")
	require.Nil(t, err)

	buf := &bytes.Buffer{}
	buf.Write(encMsg)

	m, err := decodeMessage(buf)
	require.Nil(t, err)

	genesisHash, err := common.HexToBytes("0xdcd1346701ca8396496e52aa2785b1748deb6db09551b72159dcb3e08991025b")
	require.Nil(t, err)

	expected := &BlockRequestMessage{
		ID:            7,
		RequestedData: 1,
		StartingBlock: variadic.NewUint64OrHashFromBytes(append([]byte{0}, genesisHash...)),
		EndBlockHash:  optional.NewHash(false, common.Hash{}),
		Direction:     1,
		Max:           optional.NewUint32(false, 0),
	}

	bm := m.(*BlockRequestMessage)
	require.Equal(t, expected, bm)
}

func TestEncodeBlockResponseMessage(t *testing.T) {
	expected, err := common.HexToBytes("0x02070000000000000001000000000000000000000000000000000000000000000000000000000000000000000001000102030405060708090a0b0c0d0e0f000102030405060708090a0b0c0d0e0f04000102030405060708090a0b0c0d0e0f000102030405060708090a0b0c0d0e0f000102030405060708090a0b0c0d0e0f000102030405060708090a0b0c0d0e0f04080e0f00000000")
	require.Nil(t, err)

	hash := common.NewHash([]byte{0})
	testHash := common.NewHash([]byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 0xa, 0xb, 0xc, 0xd, 0xe, 0xf, 0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 0xa, 0xb, 0xc, 0xd, 0xe, 0xf})

	header := &optional.CoreHeader{
		ParentHash:     testHash,
		Number:         big.NewInt(1),
		StateRoot:      testHash,
		ExtrinsicsRoot: testHash,
		Digest:         [][]byte{{0xe, 0xf}},
	}

	bd := &types.BlockData{
		Hash:          hash,
		Header:        optional.NewHeader(true, header),
		Body:          optional.NewBody(false, nil),
		Receipt:       optional.NewBytes(false, nil),
		MessageQueue:  optional.NewBytes(false, nil),
		Justification: optional.NewBytes(false, nil),
	}

	bm := &BlockResponseMessage{
		ID:        7,
		BlockData: []*types.BlockData{bd},
	}

	encMsg, err := bm.Encode()
	require.Nil(t, err)

	require.Equal(t, expected, encMsg)
}

func TestDecodeBlockResponseMessage(t *testing.T) {
	encMsg, err := common.HexToBytes("0x070000000000000001000000000000000000000000000000000000000000000000000000000000000000000001000102030405060708090a0b0c0d0e0f000102030405060708090a0b0c0d0e0f04000102030405060708090a0b0c0d0e0f000102030405060708090a0b0c0d0e0f000102030405060708090a0b0c0d0e0f000102030405060708090a0b0c0d0e0f04080e0f00000000")
	require.Nil(t, err)

	buf := &bytes.Buffer{}
	buf.Write(encMsg)

	m := new(BlockResponseMessage)
	err = m.Decode(buf)
	require.Nil(t, err)

	hash := common.NewHash([]byte{0})
	testHash := common.NewHash([]byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 0xa, 0xb, 0xc, 0xd, 0xe, 0xf, 0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 0xa, 0xb, 0xc, 0xd, 0xe, 0xf})

	header := &optional.CoreHeader{
		ParentHash:     testHash,
		Number:         big.NewInt(1),
		StateRoot:      testHash,
		ExtrinsicsRoot: testHash,
		Digest:         [][]byte{{0xe, 0xf}},
	}

	bd := &types.BlockData{
		Hash:          hash,
		Header:        optional.NewHeader(true, header),
		Body:          optional.NewBody(false, nil),
		Receipt:       optional.NewBytes(false, nil),
		MessageQueue:  optional.NewBytes(false, nil),
		Justification: optional.NewBytes(false, nil),
	}

	expected := &BlockResponseMessage{
		ID:        7,
		BlockData: []*types.BlockData{bd},
	}

	require.Equal(t, expected, m)
}

func TestEncodeBlockAnnounceMessage(t *testing.T) {
	// this value is a concatenation of:
	//  message type:  byte(3)
	//  ParentHash: Hash: 0x4545454545454545454545454545454545454545454545454545454545454545
	//	Number: *big.Int // block number: 1
	//	StateRoot:  Hash: 0xb3266de137d20a5d0ff3a6401eb57127525fd9b2693701f0bf5a8a853fa3ebe0
	//	ExtrinsicsRoot: Hash: 0x03170a2e7597b7b7e3d84c05391d139a62b157e78786d8c082f29dcf4c111314
	//	Digest: []byte

	//                                        mtparenthash                                                      bnstateroot                                                       extrinsicsroot                                                  di
	expected, err := common.HexToBytes("0x03454545454545454545454545454545454545454545454545454545454545454504b3266de137d20a5d0ff3a6401eb57127525fd9b2693701f0bf5a8a853fa3ebe003170a2e7597b7b7e3d84c05391d139a62b157e78786d8c082f29dcf4c1113140400")
	require.Nil(t, err)

	parentHash, err := common.HexToHash("0x4545454545454545454545454545454545454545454545454545454545454545")
	require.Nil(t, err)

	stateRoot, err := common.HexToHash("0xb3266de137d20a5d0ff3a6401eb57127525fd9b2693701f0bf5a8a853fa3ebe0")
	require.Nil(t, err)

	extrinsicsRoot, err := common.HexToHash("0x03170a2e7597b7b7e3d84c05391d139a62b157e78786d8c082f29dcf4c111314")
	require.Nil(t, err)

	bhm := &BlockAnnounceMessage{
		ParentHash:     parentHash,
		Number:         big.NewInt(1),
		StateRoot:      stateRoot,
		ExtrinsicsRoot: extrinsicsRoot,
		Digest:         [][]byte{{}},
	}
	encMsg, err := bhm.Encode()
	require.Nil(t, err)

	require.Equal(t, expected, encMsg)

}

func TestDecodeBlockAnnounceMessage(t *testing.T) {
	announceMessage, err := common.HexToBytes("0x454545454545454545454545454545454545454545454545454545454545454504b3266de137d20a5d0ff3a6401eb57127525fd9b2693701f0bf5a8a853fa3ebe003170a2e7597b7b7e3d84c05391d139a62b157e78786d8c082f29dcf4c11131400")
	require.Nil(t, err)

	buf := &bytes.Buffer{}
	buf.Write(announceMessage)

	bhm := new(BlockAnnounceMessage)
	err = bhm.Decode(buf)
	require.Nil(t, err)

	parentHash, err := common.HexToHash("0x4545454545454545454545454545454545454545454545454545454545454545")
	require.Nil(t, err)

	stateRoot, err := common.HexToHash("0xb3266de137d20a5d0ff3a6401eb57127525fd9b2693701f0bf5a8a853fa3ebe0")
	require.Nil(t, err)

	extrinsicsRoot, err := common.HexToHash("0x03170a2e7597b7b7e3d84c05391d139a62b157e78786d8c082f29dcf4c111314")
	require.Nil(t, err)

	expected := &BlockAnnounceMessage{
		ParentHash:     parentHash,
		Number:         big.NewInt(1),
		StateRoot:      stateRoot,
		ExtrinsicsRoot: extrinsicsRoot,
		Digest:         [][]byte{},
	}

	require.Equal(t, expected, bhm)
}

func TestEncodeTransactionMessageSingleExtrinsic(t *testing.T) {
	// expected:
	// 0x04 - Message Type
	// 0x14 - byte array (of all extrinsics encoded) - len 5
	// 0x10 - btye array (first extrinsic) - len 4
	// 0x01020304 - value of array extrinsic array
	expected, err := common.HexToBytes("0x04141001020304")
	require.Nil(t, err)

	extrinsic := types.Extrinsic{0x01, 0x02, 0x03, 0x04}

	transactionMessage := TransactionMessage{Extrinsics: []types.Extrinsic{extrinsic}}

	encMsg, err := transactionMessage.Encode()
	require.Nil(t, err)

	require.Equal(t, expected, encMsg)
}

func TestEncodeTransactionMessageTwoExtrinsics(t *testing.T) {
	// expected:
	// 0x04 - Message Type
	// 0x24 - byte array (of all extrinsics encoded) - len 9
	// 0x0C - btye array (first extrinsic) len 3
	// 0x010203 - value of array first extrinsic array
	// 0x10 - byte array (second extrinsic) len 4
	// 0x04050607 - value of second extrinsic array
	expected, err := common.HexToBytes("0x04240C0102031004050607")
	require.Nil(t, err)

	extrinsic1 := types.Extrinsic{0x01, 0x02, 0x03}
	extrinsic2 := types.Extrinsic{0x04, 0x05, 0x06, 0x07}

	transactionMessage := TransactionMessage{Extrinsics: []types.Extrinsic{extrinsic1, extrinsic2}}

	encMsg, err := transactionMessage.Encode()
	require.Nil(t, err)

	require.Equal(t, expected, encMsg)
}

func TestDecodeTransactionMessageOneExtrinsic(t *testing.T) {
	originalMessage, err := common.HexToBytes("0x141001020304") // (without message type byte prepended)
	require.Nil(t, err)

	buf := &bytes.Buffer{}
	buf.Write(originalMessage)

	decodedMessage := new(TransactionMessage)
	err = decodedMessage.Decode(buf)
	require.Nil(t, err)

	extrinsic := types.Extrinsic{0x01, 0x02, 0x03, 0x04}
	expected := TransactionMessage{[]types.Extrinsic{extrinsic}}

	require.Equal(t, expected, *decodedMessage)

}

func TestDecodeTransactionMessageTwoExtrinsics(t *testing.T) {
	originalMessage, err := common.HexToBytes("0x240C0102031004050607") // (without message type byte prepended)
	require.Nil(t, err)

	buf := &bytes.Buffer{}
	buf.Write(originalMessage)

	decodedMessage := new(TransactionMessage)
	err = decodedMessage.Decode(buf)
	require.Nil(t, err)

	extrinsic1 := types.Extrinsic{0x01, 0x02, 0x03}
	extrinsic2 := types.Extrinsic{0x04, 0x05, 0x06, 0x07}
	expected := TransactionMessage{[]types.Extrinsic{extrinsic1, extrinsic2}}

	require.Equal(t, expected, *decodedMessage)
}

func TestDecodeConsensusMessage(t *testing.T) {
	ConsensusEngineID := types.BabeEngineID

	testID := hex.EncodeToString(types.BabeEngineID.ToBytes())
	testData := "03100405"

	msg := "0x" + testID + testData // 0x4241424503100405

	encMsg, err := common.HexToBytes(msg)
	require.Nil(t, err)

	buf := &bytes.Buffer{}
	buf.Write(encMsg)

	m := new(ConsensusMessage)
	err = m.Decode(buf)
	require.Nil(t, err)

	out, err := hex.DecodeString(testData)
	require.Nil(t, err)

	expected := &ConsensusMessage{
		ConsensusEngineID: ConsensusEngineID,
		Data:              out,
	}

	require.Equal(t, expected, m)

	encodedMessage, err := expected.Encode()
	require.Nil(t, err)

	require.Equal(t, encMsg, encodedMessage[1:])

}
