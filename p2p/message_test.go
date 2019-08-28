package p2p

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/ChainSafe/gossamer/common"
)

func TestDecodeMessage_Status(t *testing.T) {
	// from Alexander testnet
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

	expected := &StatusMessage{
		ProtocolVersion:     uint32(2),
		MinSupportedVersion: uint32(2),
		Roles:               byte(4),
		BestBlockNumber:     uint64(2418625),
		BestBlockHash:       bestBlockHash,
		GenesisHash:         genesisHash,
		ChainStatus:         []byte{0},
	}

	if !reflect.DeepEqual(sm, expected) {
		t.Fatalf("Fail: got %v expected %v", sm, expected)
	}
}

func TestDecodeStatusMessage(t *testing.T) {
	// from Alexander testnet
	encStatus, err := common.HexToBytes("0x020000000200000004c1e72400000000008dac4bd53582976cd2834b47d3c7b3a9c8c708db84b3bae145753547ec9ee4dadcd1346701ca8396496e52aa2785b1748deb6db09551b72159dcb3e08991025b0400")
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

	sm := new(StatusMessage)
	err = sm.Decode(encStatus)
	if err != nil {
		t.Fatal(err)
	}

	expected := &StatusMessage{
		ProtocolVersion:     uint32(2),
		MinSupportedVersion: uint32(2),
		Roles:               byte(4),
		BestBlockNumber:     uint64(2418625),
		BestBlockHash:       bestBlockHash,
		GenesisHash:         genesisHash,
		ChainStatus:         []byte{0},
	}

	if !reflect.DeepEqual(sm, expected) {
		t.Fatalf("Fail: got %v expected %v", sm, expected)
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

func TestEncodeBlockRequestMessage(t *testing.T) {
	// this value is a concatenation of:
	// message type: byte(1)
	// message ID: uint32(7)
	// requested data: byte(1)
	// starting block: 1 byte to identify type + 32 byte hash: byte(0) + hash(dcd1346701ca8396496e52aa2785b1748deb6db09551b72159dcb3e08991025b)
	// end block hash: 32 bytes: hash(fd19d9ebac759c993fd2e05a1cff9e757d8741c2704c8682c15b5503496b6aa1)
	// direction: byte(1)
	// max: uint32(1)
	expected, err := common.HexToBytes("0x01070000000100dcd1346701ca8396496e52aa2785b1748deb6db09551b72159dcb3e08991025bfd19d9ebac759c993fd2e05a1cff9e757d8741c2704c8682c15b5503496b6aa10101000000")
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
		EndBlockHash:  endBlock,
		Direction:     1,
		Max:           1,
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
	expected, err := common.HexToBytes("0x010700000001010100000000000000fd19d9ebac759c993fd2e05a1cff9e757d8741c2704c8682c15b5503496b6aa10101000000")
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
		EndBlockHash:  endBlock,
		Direction:     1,
		Max:           1,
	}

	encMsg, err := bm.Encode()
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(encMsg, expected) {
		t.Fatalf("Fail: got %x expected %x", encMsg, expected)
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
	expected, err := common.HexToBytes("0x01070000000100dcd1346701ca8396496e52aa2785b1748deb6db09551b72159dcb3e08991025b000100")
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
		Direction:     1,
	}

	encMsg, err := bm.Encode()
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(encMsg, expected) {
		t.Fatalf("Fail: got %x expected %x", encMsg, expected)
	}
}
