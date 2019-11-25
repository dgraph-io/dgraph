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
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math/big"

	scale "github.com/ChainSafe/gossamer/codec"

	"github.com/ChainSafe/gossamer/common"
	"github.com/ChainSafe/gossamer/common/optional"
	"github.com/ChainSafe/gossamer/core/types"
)

const (
	StatusMsgType = iota
	BlockRequestMsgType
	BlockResponseMsgType
	BlockAnnounceMsgType
	TransactionMsgType
	ConsensusMsgType
	RemoteCallRequestType
	RemoteCallResponseType
	RemoteReadRequestType
	RemoteReadResponseType
	RemoteHeaderRequestType
	RemoteHeaderResponseType
	RemoteChangesRequestType
	RemoteChangesResponseType
	ChainSpecificMsgType = 255
)

type Message interface {
	Encode() ([]byte, error)
	Decode(io.Reader) error
	String() string
	GetType() int
	Id() string
}

// DecodeMessage accepts a raw message including the type indicator byte and decodes it to its specific message type
func DecodeMessage(r io.Reader) (m Message, err error) {
	msgType := make([]byte, 1)
	_, err = r.Read(msgType)
	if err != nil {
		return nil, err
	}

	switch msgType[0] {
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
	default:
		return nil, errors.New("unsupported message type")
	}

	return m, err
}

type StatusMessage struct {
	ProtocolVersion     uint32
	MinSupportedVersion uint32
	Roles               byte
	BestBlockNumber     uint64
	BestBlockHash       common.Hash
	GenesisHash         common.Hash
	ChainStatus         []byte
}

func (sm *StatusMessage) GetType() int {
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

// Decodes the message into a StatusMessage, it assumes the type byte has been removed
func (sm *StatusMessage) Decode(r io.Reader) error {
	sd := scale.Decoder{Reader: r}
	_, err := sd.Decode(sm)
	return err
}

// Returns an empty string to ensure we don't rebroadcast it
func (sm *StatusMessage) Id() string {
	return ""
}

// for optionals, if first byte is 0, then it is None
// otherwise it is Some
type BlockRequestMessage struct {
	ID            uint64
	RequestedData byte
	StartingBlock []byte // first byte 0 = block hash (32 byte), first byte 1 = block number (int64)
	EndBlockHash  *optional.Hash
	Direction     byte
	Max           *optional.Uint32
}

func (bm *BlockRequestMessage) GetType() int {
	return BlockRequestMsgType
}

// String formats a BlockRequestMessage as a string
func (bm *BlockRequestMessage) String() string {
	return fmt.Sprintf("BlockRequestMessage Id=%d RequestedData=%d StartingBlock=0x%x EndBlockHash=0x%s Direction=%d Max=%s",
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

	encId := make([]byte, 8)
	binary.LittleEndian.PutUint64(encId, bm.ID)
	encMsg = append(encMsg, encId...)

	encMsg = append(encMsg, bm.RequestedData)

	if bm.StartingBlock[0] == 1 {
		encMsg = append(encMsg, bm.StartingBlock[0])
		num := bm.StartingBlock[1:]
		if len(num) < 8 {
			num = common.AppendZeroes(num, 8)
		}
		encMsg = append(encMsg, num...)
	} else {
		encMsg = append(encMsg, bm.StartingBlock...)
	}

	if !bm.EndBlockHash.Exists() {
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

// Decodes the message into a BlockRequestMessage, it assumes the type byte has been removed
func (bm *BlockRequestMessage) Decode(r io.Reader) error {
	var err error

	bm.ID, err = readUint64(r)
	if err != nil {
		return err
	}

	bm.RequestedData, err = readByte(r)
	if err != nil {
		return err
	}

	// starting block is a variable type; if next byte is 0 it is Hash, if next byte is 1 it is uint64
	startingBlockType, err := readByte(r)
	if err != nil {
		return err
	}

	if startingBlockType == 0 {
		hash := make([]byte, 32)
		_, err = r.Read(hash)
		if err != nil {
			return err
		}
		bm.StartingBlock = append([]byte{startingBlockType}, hash...)
	} else {
		num := make([]byte, 8)
		_, err = r.Read(num)
		if err != nil {
			return err
		}
		bm.StartingBlock = append([]byte{startingBlockType}, num...)
	}

	// EndBlockHash is an optional type, if next byte is 0 it doesn't exist
	endBlockHashExists, err := readByte(r)
	if err != nil {
		return err
	}

	// if endBlockHash was None, then just set Direction and Max
	if endBlockHashExists == 0 {
		bm.EndBlockHash = optional.NewHash(false, common.Hash{})
	} else {
		var endBlockHash common.Hash
		endBlockHash, err = readHash(r)
		if err != nil {
			return err
		}
		bm.EndBlockHash = optional.NewHash(true, endBlockHash)
	}
	dir, err := readByte(r)
	if err != nil {
		return err
	}

	bm.Direction = dir

	// Max is an optional type, if next byte is 0 it doesn't exist
	maxExists, err := readByte(r)
	if err != nil {
		return err
	}

	if maxExists == 0 {
		bm.Max = optional.NewUint32(false, 0)
	} else {
		max, err := readUint32(r)
		if err != nil {
			return err
		}
		bm.Max = optional.NewUint32(true, max)
	}

	return nil
}

// Id Returns the ID of the block
func (bm *BlockRequestMessage) Id() string {
	return string(bm.ID)
}

// BlockAnnounceMessage is a state block header
type BlockAnnounceMessage struct {
	ParentHash     common.Hash
	Number         *big.Int
	StateRoot      common.Hash
	ExtrinsicsRoot common.Hash
	Digest         []byte // any additional block info eg. logs, seal
}

func (bm *BlockAnnounceMessage) GetType() int {
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

func (bm *BlockAnnounceMessage) Encode() ([]byte, error) {
	enc, err := scale.Encode(bm)
	if err != nil {
		return enc, err
	}
	return append([]byte{BlockAnnounceMsgType}, enc...), nil
}

// Decodes the message into a BlockAnnounceMessage, it assumes the type byte has been removed
func (bm *BlockAnnounceMessage) Decode(r io.Reader) error {
	sd := scale.Decoder{Reader: r}
	_, err := sd.Decode(bm)
	return err
}

// Id returns the hash of the block
func (bm *BlockAnnounceMessage) Id() string {
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

type BlockResponseMessage struct {
	ID   uint64
	Data []byte // TODO: change this to BlockData type
}

func (bm *BlockResponseMessage) GetType() int {
	return BlockResponseMsgType
}

// String formats a BlockResponseMessage as a string
func (bm *BlockResponseMessage) String() string {
	return fmt.Sprintf("BlockResponseMessage Id=%d Data=%x", bm.ID, bm.Data)
}

// Encode encodes a block response message using SCALE and appends the type byte to the start
func (bm *BlockResponseMessage) Encode() ([]byte, error) {
	encMsg := []byte{BlockResponseMsgType}

	encId := make([]byte, 8)
	binary.LittleEndian.PutUint64(encId, bm.ID)
	encMsg = append(encMsg, encId...)

	return append(encMsg, bm.Data...), nil
}

// Decodes the message into a BlockResponseMessage, it assumes the type byte has been removed
func (bm *BlockResponseMessage) Decode(r io.Reader) error {
	var err error
	bm.ID, err = readUint64(r)
	if err != nil {
		return err
	}

	for {
		b, err := readByte(r)
		if err != nil {
			break
		}

		bm.Data = append(bm.Data, b)
	}

	return nil
}

// Id returns the Id of BlockResponseMessage
func (bm *BlockResponseMessage) Id() string {
	return string(bm.ID)
}

type TransactionMessage struct {
	Extrinsics []types.Extrinsic
}

func (tm *TransactionMessage) GetType() int {
	return TransactionMsgType
}

func (tm *TransactionMessage) String() string {
	return fmt.Sprintf("TransactionMessage extrinsics=%x", tm.Extrinsics)
}

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

// Decodes the message into a TransactionMessage, it assumes the type byte han been removed
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

// Id returns the Hash of TransactionMessage
func (tm *TransactionMessage) Id() string {
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

func readByte(r io.Reader) (byte, error) {
	buf := make([]byte, 1)
	_, err := r.Read(buf)
	if err != nil {
		return 0, err
	}
	return buf[0], nil
}

func readUint32(r io.Reader) (uint32, error) {
	buf := make([]byte, 4)
	_, err := r.Read(buf)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint32(buf), nil
}

func readUint64(r io.Reader) (uint64, error) {
	buf := make([]byte, 8)
	_, err := r.Read(buf)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint64(buf), nil
}

func readHash(r io.Reader) (common.Hash, error) {
	buf := make([]byte, 32)
	_, err := r.Read(buf)
	if err != nil {
		return common.Hash{}, err
	}
	h := [32]byte{}
	copy(h[:], buf)
	return common.Hash(h), nil

}
