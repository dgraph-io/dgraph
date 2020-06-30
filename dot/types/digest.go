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

package types

import (
	"errors"

	"github.com/ChainSafe/gossamer/lib/common"
)

// ConsensusEngineID is a 4-character identifier of the consensus engine that produced the digest.
type ConsensusEngineID [4]byte

// NewConsensusEngineID casts a byte array to ConsensusEngineID
// if the input is longer than 4 bytes, it takes the first 4 bytes
func NewConsensusEngineID(in []byte) (res ConsensusEngineID) {
	res = [4]byte{}
	copy(res[:], in)
	return res
}

// ToBytes turns ConsensusEngineID to a byte array
func (h ConsensusEngineID) ToBytes() []byte {
	b := [4]byte(h)
	return b[:]
}

// BabeEngineID is the hard-coded babe ID
var BabeEngineID = ConsensusEngineID{'B', 'A', 'B', 'E'}

// GrandpaEngineID is the hard-coded grandpa ID
var GrandpaEngineID = ConsensusEngineID{'F', 'R', 'N', 'K'}

// ChangesTrieRootDigestType is the byte representation of ChangesTrieRootDigest
var ChangesTrieRootDigestType = byte(2)

// PreRuntimeDigestType is the byte representation of PreRuntimeDigest
var PreRuntimeDigestType = byte(6)

// ConsensusDigestType is the byte representation of ConsensusDigest
var ConsensusDigestType = byte(4)

// SealDigestType is the byte representation of SealDigest
var SealDigestType = byte(5)

// DecodeDigestItem will decode byte array to DigestItem
func DecodeDigestItem(in []byte) (DigestItem, error) {
	if len(in) < 2 {
		return nil, errors.New("cannot decode invalid digest encoding")
	}

	switch in[0] {
	case ChangesTrieRootDigestType:
		d := new(ChangesTrieRootDigest)
		err := d.Decode(in[1:])
		return d, err
	case PreRuntimeDigestType:
		d := new(PreRuntimeDigest)
		err := d.Decode(in[1:])
		return d, err
	case ConsensusDigestType:
		d := new(ConsensusDigest)
		err := d.Decode(in[1:])
		return d, err
	case SealDigestType:
		d := new(SealDigest)
		err := d.Decode(in[1:])
		return d, err
	}

	return nil, errors.New("invalid digest item type")
}

// DigestItem can be of one of four types of digest: ChangesTrieRootDigest, PreRuntimeDigest, ConsensusDigest, or SealDigest.
// see https://github.com/paritytech/substrate/blob/f548309478da3935f72567c2abc2eceec3978e9f/primitives/runtime/src/generic/digest.rs#L77
type DigestItem interface {
	Type() byte
	Encode() []byte
	Decode([]byte) error // Decode assumes the type byte (first byte) has been removed from the encoding.
}

// ChangesTrieRootDigest contains the root of the changes trie at a given block, if the runtime supports it.
type ChangesTrieRootDigest struct {
	Hash common.Hash
}

// Type returns the type
func (d *ChangesTrieRootDigest) Type() byte {
	return ChangesTrieRootDigestType
}

// Encode will encode the ChangesTrieRootDigestType into byte array
func (d *ChangesTrieRootDigest) Encode() []byte {
	return append([]byte{ChangesTrieRootDigestType}, d.Hash[:]...)
}

// Decode will decode into ChangesTrieRootDigest Hash
func (d *ChangesTrieRootDigest) Decode(in []byte) error {
	if len(in) < 32 {
		return errors.New("input is too short: need 32 bytes")
	}

	copy(d.Hash[:], in)
	return nil
}

// PreRuntimeDigest contains messages from the consensus engine to the runtime.
type PreRuntimeDigest struct {
	ConsensusEngineID ConsensusEngineID
	Data              []byte
}

// Type will return PreRuntimeDigestType
func (d *PreRuntimeDigest) Type() byte {
	return PreRuntimeDigestType
}

// Encode will encode PreRuntimeDigest ConsensusEngineID and Data
func (d *PreRuntimeDigest) Encode() []byte {
	enc := []byte{PreRuntimeDigestType}
	enc = append(enc, d.ConsensusEngineID[:]...)
	return append(enc, d.Data...)
}

// Decode will decode PreRuntimeDigest ConsensusEngineID and Data
func (d *PreRuntimeDigest) Decode(in []byte) error {
	if len(in) < 4 {
		return errors.New("input is too short: need at least 4 bytes")
	}

	copy(d.ConsensusEngineID[:], in[:4])
	d.Data = in[4:]
	return nil
}

// ConsensusDigest contains messages from the runtime to the consensus engine.
type ConsensusDigest struct {
	ConsensusEngineID ConsensusEngineID
	Data              []byte
}

// Type returns the ConsensusDigest type
func (d *ConsensusDigest) Type() byte {
	return ConsensusDigestType
}

// Encode will encode ConsensusDigest ConsensusEngineID and Data
func (d *ConsensusDigest) Encode() []byte {
	enc := []byte{ConsensusDigestType}
	enc = append(enc, d.ConsensusEngineID[:]...)
	return append(enc, d.Data...)
}

// Decode will decode into ConsensusEngineID and Data
func (d *ConsensusDigest) Decode(in []byte) error {
	if len(in) < 4 {
		return errors.New("input is too short: need at least 4 bytes")
	}

	copy(d.ConsensusEngineID[:], in[:4])
	d.Data = in[4:]
	return nil
}

// DataType returns the data type of the runtime-to-consensus engine message
func (d *ConsensusDigest) DataType() byte {
	return d.Data[0]
}

// SealDigest contains the seal or signature. This is only used by native code.
type SealDigest struct {
	ConsensusEngineID ConsensusEngineID
	Data              []byte
}

// Type will return SealDigest type
func (d *SealDigest) Type() byte {
	return SealDigestType
}

// Encode will encode SealDigest ConsensusEngineID and Data
func (d *SealDigest) Encode() []byte {
	enc := []byte{SealDigestType}
	enc = append(enc, d.ConsensusEngineID[:]...)
	return append(enc, d.Data...)
}

// Decode will decode into  SealDigest ConsensusEngineID and Data
func (d *SealDigest) Decode(in []byte) error {
	if len(in) < 4 {
		return errors.New("input is too short: need at least 4 bytes")
	}

	copy(d.ConsensusEngineID[:], in[:4])
	d.Data = in[4:]
	return nil
}
