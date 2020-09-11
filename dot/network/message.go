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
	"encoding/binary"
	"fmt"
	"io"
	"math/big"

	"github.com/ChainSafe/gossamer/dot/types"
	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/ChainSafe/gossamer/lib/common/optional"
	"github.com/ChainSafe/gossamer/lib/common/variadic"
	"github.com/ChainSafe/gossamer/lib/scale"
)

//nolint
const (
	StatusMsgType             = 0
	BlockRequestMsgType       = 1
	BlockResponseMsgType      = 2
	BlockAnnounceMsgType      = 3
	TransactionMsgType        = 4
	ConsensusMsgType          = 5
	RemoteCallRequestType     = 6
	RemoteCallResponseType    = 7
	RemoteReadRequestType     = 8
	RemoteReadResponseType    = 9
	RemoteHeaderRequestType   = 10
	RemoteHeaderResponseType  = 11
	RemoteChangesRequestType  = 12
	RemoteChangesResponseType = 13
	ChainSpecificMsgType      = 255
)

// Message interface
type Message interface {
	Encode() ([]byte, error)
	Decode(io.Reader) error
	String() string
	Type() int
	IDString() string
}

// decodeMessage decodes the message based on message type
func decodeMessage(r io.Reader) (m Message, err error) {
	msgType, err := common.ReadByte(r)
	if err != nil {
		return nil, err
	}

	switch msgType {
	case StatusMsgType:
		m = new(StatusMessage)
		err = m.Decode(r)
	case BlockRequestMsgType:
		m = new(BlockRequestMessage)
		err = m.Decode(r)
	case BlockResponseMsgType:
		m = new(BlockResponseMessage)
		err = m.Decode(r)
	case BlockAnnounceMsgType:
		m = new(BlockAnnounceMessage)
		err = m.Decode(r)
	case TransactionMsgType:
		m = new(TransactionMessage)
		err = m.Decode(r)
	case ConsensusMsgType:
		m = new(ConsensusMessage)
		err = m.Decode(r)
	default:
		return nil, fmt.Errorf("unsupported message type %d", msgType)
	}

	return m, err
}

// decodeMessageBytes decodes the message based on message type
func decodeMessageBytes(in []byte) (m Message, err error) {
	r := &bytes.Buffer{}
	_, err = r.Write(in)
	if err != nil {
		return nil, err
	}

	return decodeMessage(r)
}

// StatusMessage struct
type StatusMessage struct {
	ProtocolVersion     uint32
	MinSupportedVersion uint32
	Roles               byte
	BestBlockNumber     uint64
	BestBlockHash       common.Hash
	GenesisHash         common.Hash
	ChainStatus         []byte
}

// Type returns StatusMsgType
func (sm *StatusMessage) Type() int {
	return StatusMsgType
}

// String formats a StatusMessage as a string
func (sm *StatusMessage) String() string {
	return fmt.Sprintf("StatusMessage ProtocolVersion=%d MinSupportedVersion=%d Roles=%d BestBlockNumber=%d BestBlockHash=0x%x GenesisHash=0x%x ChainStatus=0x%x",
		sm.ProtocolVersion,
		sm.MinSupportedVersion,
		sm.Roles,
		sm.BestBlockNumber,
		sm.BestBlockHash,
		sm.GenesisHash,
		sm.ChainStatus)
}

// Encode encodes a status message using SCALE and appends the type byte to the start
func (sm *StatusMessage) Encode() ([]byte, error) {
	enc, err := scale.Encode(sm)
	if err != nil {
		return enc, err
	}
	return append([]byte{StatusMsgType}, enc...), nil
}

// Decode the message into a StatusMessage, it assumes the type byte has been removed
func (sm *StatusMessage) Decode(r io.Reader) error {
	sd := scale.Decoder{Reader: r}
	_, err := sd.Decode(sm)
	return err
}

// IDString Returns an empty string to ensure we don't rebroadcast it
func (sm *StatusMessage) IDString() string {
	return ""
}

// BlockRequestMessage for optionals, if first byte is 0, then it is None
// otherwise it is Some
type BlockRequestMessage struct {
	ID            uint64
	RequestedData byte
	StartingBlock *variadic.Uint64OrHash // first byte 0 = block hash (32 byte), first byte 1 = block number (int64)
	EndBlockHash  *optional.Hash
	Direction     byte
	Max           *optional.Uint32
}

// RequestedDataHeader flag for requesting header data
const RequestedDataHeader = byte(1)

// RequestedDataBody flag for requesting body data
const RequestedDataBody = byte(2)

// RequestedDataReceipt flag for requesting receipt data
const RequestedDataReceipt = byte(4)

// RequestedDataMessageQueue flag for requesting message queue data
const RequestedDataMessageQueue = byte(8)

// RequestedDataJustification flag for requesting justification data
const RequestedDataJustification = byte(16)

// Type returns BlockRequestMsgType
func (bm *BlockRequestMessage) Type() int {
	return BlockRequestMsgType
}

// String formats a BlockRequestMessage as a string
func (bm *BlockRequestMessage) String() string {
	return fmt.Sprintf("BlockRequestMessage ID=%d RequestedData=%d StartingBlock=0x%x EndBlockHash=0x%s Direction=%d Max=%s",
		bm.ID,
		bm.RequestedData,
		bm.StartingBlock,
		bm.EndBlockHash.String(),
		bm.Direction,
		bm.Max.String())
}

// Encode encodes a block request message using SCALE and appends the type byte to the start
func (bm *BlockRequestMessage) Encode() ([]byte, error) {
	encMsg := []byte{BlockRequestMsgType}

	encID := make([]byte, 8)
	binary.LittleEndian.PutUint64(encID, bm.ID)
	encMsg = append(encMsg, encID...)

	encMsg = append(encMsg, bm.RequestedData)

	startingBlockArray, err := bm.StartingBlock.Encode()
	if err != nil || len(startingBlockArray) == 0 {
		return nil, fmt.Errorf("invalid BlockRequestMessage")
	}
	encMsg = append(encMsg, startingBlockArray...)

	if bm.EndBlockHash == nil || !bm.EndBlockHash.Exists() {
		encMsg = append(encMsg, []byte{0, 0}...)
	} else {
		val := bm.EndBlockHash.Value()
		encMsg = append(encMsg, append([]byte{1}, val[:]...)...)
	}

	encMsg = append(encMsg, bm.Direction)

	if !bm.Max.Exists() {
		encMsg = append(encMsg, []byte{0, 0}...)
	} else {
		max := make([]byte, 4)
		binary.LittleEndian.PutUint32(max, bm.Max.Value())
		encMsg = append(encMsg, append([]byte{1}, max...)...)
	}

	return encMsg, nil
}

// Decode the message into a BlockRequestMessage, it assumes the type byte has been removed
func (bm *BlockRequestMessage) Decode(r io.Reader) error {
	var err error

	bm.ID, err = common.ReadUint64(r)
	if err != nil {
		return err
	}

	bm.RequestedData, err = common.ReadByte(r)
	if err != nil {
		return err
	}

	bm.StartingBlock = &variadic.Uint64OrHash{}
	err = bm.StartingBlock.Decode(r)
	if err != nil {
		return err
	}

	// EndBlockHash is an optional type, if next byte is 0 it doesn't exist
	endBlockHashExists, err := common.ReadByte(r)
	if err != nil {
		return err
	}

	// if endBlockHash was None, then just set Direction and Max
	if endBlockHashExists == 0 {
		bm.EndBlockHash = optional.NewHash(false, common.Hash{})
	} else {
		var endBlockHash common.Hash
		endBlockHash, err = common.ReadHash(r)
		if err != nil {
			return err
		}
		bm.EndBlockHash = optional.NewHash(true, endBlockHash)
	}
	dir, err := common.ReadByte(r)
	if err != nil {
		return err
	}

	bm.Direction = dir

	// Max is an optional type, if next byte is 0 it doesn't exist
	maxExists, err := common.ReadByte(r)
	if err != nil {
		return err
	}

	if maxExists == 0 {
		bm.Max = optional.NewUint32(false, 0)
	} else {
		max, err := common.ReadUint32(r)
		if err != nil {
			return err
		}
		bm.Max = optional.NewUint32(true, max)
	}

	return nil
}

// IDString Returns the ID of the block
func (bm *BlockRequestMessage) IDString() string {
	return fmt.Sprintf("%d", bm.ID)
}

// BlockAnnounceMessage is a state block header
type BlockAnnounceMessage struct {
	ParentHash     common.Hash
	Number         *big.Int
	StateRoot      common.Hash
	ExtrinsicsRoot common.Hash
	Digest         [][]byte // any additional block info eg. logs, seal
}

// Type returns BlockAnnounceMsgType
func (bm *BlockAnnounceMessage) Type() int {
	return BlockAnnounceMsgType
}

// string formats a BlockAnnounceMessage as a string
func (bm *BlockAnnounceMessage) String() string {
	return fmt.Sprintf("BlockAnnounceMessage ParentHash=0x%x Number=%d StateRoot=0x%x ExtrinsicsRoot=0x%x Digest=0x%x",
		bm.ParentHash,
		bm.Number,
		bm.StateRoot,
		bm.ExtrinsicsRoot,
		bm.Digest)
}

// Encode a BlockAnnounce Msg Type containing the BlockAnnounceMessage using scale.Encode
func (bm *BlockAnnounceMessage) Encode() ([]byte, error) {
	enc, err := scale.Encode(bm)
	if err != nil {
		return enc, err
	}
	return append([]byte{BlockAnnounceMsgType}, enc...), nil
}

// Decode the message into a BlockAnnounceMessage, it assumes the type byte has been removed
func (bm *BlockAnnounceMessage) Decode(r io.Reader) error {
	sd := scale.Decoder{Reader: r}
	_, err := sd.Decode(bm)
	return err
}

// IDString returns the hash of the block
func (bm *BlockAnnounceMessage) IDString() string {
	// scale encode each extrinsic
	encMsg, err := bm.Encode()
	if err != nil {
		return ""
	}
	hash, err := common.Blake2bHash(encMsg)
	if err != nil {
		return ""
	}
	return hash.String()
}

// BlockResponseMessage struct
type BlockResponseMessage struct {
	ID        uint64
	BlockData []*types.BlockData
}

// Type returns BlockResponseMsgType
func (bm *BlockResponseMessage) Type() int {
	return BlockResponseMsgType
}

// String formats a BlockResponseMessage as a string
func (bm *BlockResponseMessage) String() string {
	return fmt.Sprintf("BlockResponseMessage ID=%d BlockData=%v", bm.ID, bm.BlockData)
}

// Encode encodes a block response message using SCALE and appends the type byte to the start
func (bm *BlockResponseMessage) Encode() ([]byte, error) {
	encMsg := []byte{BlockResponseMsgType}

	encID := make([]byte, 8)
	binary.LittleEndian.PutUint64(encID, bm.ID)
	encMsg = append(encMsg, encID...)

	encData, err := types.EncodeBlockDataArray(bm.BlockData)
	if err != nil {
		return nil, err
	}

	return append(encMsg, encData...), nil
}

// Decode the message into a BlockResponseMessage, it assumes the type byte has been removed
func (bm *BlockResponseMessage) Decode(r io.Reader) error {
	var err error
	bm.ID, err = common.ReadUint64(r)
	if err != nil {
		return err
	}

	bm.BlockData, err = types.DecodeBlockDataArray(r)
	return err
}

// IDString returns the ID of BlockResponseMessage
func (bm *BlockResponseMessage) IDString() string {
	return fmt.Sprintf("%d", bm.ID)
}

// TransactionMessage is a struct that holds reference to Extrinsics
type TransactionMessage struct {
	Extrinsics []types.Extrinsic
}

// Type returns TransactionMsgType
func (tm *TransactionMessage) Type() int {
	return TransactionMsgType
}

// String returns the TransactionMessage extrinsics
func (tm *TransactionMessage) String() string {
	return fmt.Sprintf("TransactionMessage extrinsics=%x", tm.Extrinsics)
}

// Encode will encode TransactionMessage using scale.Encode
func (tm *TransactionMessage) Encode() ([]byte, error) {
	// scale encode each extrinsic
	var encodedExtrinsics = make([]byte, 0)
	for _, extrinsic := range tm.Extrinsics {
		encExt, err := scale.Encode([]byte(extrinsic))
		if err != nil {
			return nil, err
		}
		encodedExtrinsics = append(encodedExtrinsics, encExt...)
	}

	// scale encode the set of all extrinsics
	encodedMessage, err := scale.Encode(encodedExtrinsics)

	// prepend message type to message
	return append([]byte{TransactionMsgType}, encodedMessage...), err
}

// Decode the message into a TransactionMessage, it assumes the type byte han been removed
func (tm *TransactionMessage) Decode(r io.Reader) error {
	sd := scale.Decoder{Reader: r}
	decodedMessage, err := sd.Decode([]byte{})
	if err != nil {
		return err
	}
	messageSize := len(decodedMessage.([]byte))
	bytesProcessed := 0
	// loop through the message decoding extrinsics until they have all been decoded
	for bytesProcessed < messageSize {
		decodedExtrinsic, err := scale.Decode(decodedMessage.([]byte)[bytesProcessed:], []byte{})
		if err != nil {
			return err
		}
		bytesProcessed = bytesProcessed + len(decodedExtrinsic.([]byte)) + 1 // add 1 to processed since the first decode byte is consumed during decoding
		tm.Extrinsics = append(tm.Extrinsics, decodedExtrinsic.([]byte))
	}

	return nil
}

// IDString returns the Hash of TransactionMessage
func (tm *TransactionMessage) IDString() string {
	// scale encode each extrinsic
	encMsg, err := tm.Encode()
	if err != nil {
		return ""
	}
	hash, err := common.Blake2bHash(encMsg)
	if err != nil {
		return ""
	}
	return hash.String()
}

// ConsensusMessage is mostly opaque to us
type ConsensusMessage struct {
	// Identifies consensus engine.
	ConsensusEngineID types.ConsensusEngineID
	// Message payload.
	Data []byte
}

// Type returns ConsensusMsgType
func (cm *ConsensusMessage) Type() int {
	return ConsensusMsgType
}

// String is the string
func (cm *ConsensusMessage) String() string {
	return fmt.Sprintf("ConsensusMessage ConsensusEngineID=%d, DATA=%x", cm.ConsensusEngineID, cm.Data)
}

// Encode encodes a block response message using SCALE and appends the type byte to the start
func (cm *ConsensusMessage) Encode() ([]byte, error) {
	encMsg := []byte{ConsensusMsgType}
	encMsg = append(encMsg, cm.ConsensusEngineID.ToBytes()...)
	return append(encMsg, cm.Data...), nil
}

// Decode the message into a ConsensusMessage, it assumes the type byte has been removed
func (cm *ConsensusMessage) Decode(r io.Reader) error {
	buf := make([]byte, 4)
	_, err := r.Read(buf)
	if err != nil {
		return err
	}
	cm.ConsensusEngineID = types.NewConsensusEngineID(buf)
	for {
		b, err := common.ReadByte(r)
		if err != nil {
			break
		}

		cm.Data = append(cm.Data, b)
	}

	return nil
}

// IDString returns the Hash of ConsensusMessage
func (cm *ConsensusMessage) IDString() string {
	// scale encode each extrinsic
	encMsg, err := cm.Encode()
	if err != nil {
		return ""
	}
	hash, err := common.Blake2bHash(encMsg)
	if err != nil {
		return ""
	}
	return hash.String()
}
