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
	"github.com/ChainSafe/gossamer/common"
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

var ChangesTrieRootDigestType = byte(0)
var PreRuntimeDigestType = byte(1)
var ConsensusDigestType = byte(2)
var SealDigestType = byte(4)

// DigestItem can be of one of four types of digest: ChangesTrieRootDigest, PreRuntimeDigest, ConsensusDigest, or SealDigest.
// see https://github.com/paritytech/substrate/blob/f548309478da3935f72567c2abc2eceec3978e9f/primitives/runtime/src/generic/digest.rs#L77
type DigestItem interface {
	Type() byte
	Encode() []byte
	//Decode([]byte)
}

// ChangesTrieRootDigest contains the root of the changes trie at a given block, if the runtime supports it.
type ChangesTrieRootDigest struct {
	Hash common.Hash
}

func (d *ChangesTrieRootDigest) Type() byte {
	return ChangesTrieRootDigestType
}

func (d *ChangesTrieRootDigest) Encode() []byte {
	return d.Hash[:]
}

// PreRuntimeDigest contains messages from the consensus engine to the runtime.
type PreRuntimeDigest struct {
	ConsensusEngineID ConsensusEngineID
	Data              []byte
}

func (d *PreRuntimeDigest) Type() byte {
	return PreRuntimeDigestType
}

func (d *PreRuntimeDigest) Encode() []byte {
	enc := []byte{PreRuntimeDigestType}
	enc = append(enc, d.ConsensusEngineID[:]...)
	return append(enc, d.Data...)
}

// ConsensusDigest contains messages from the runtime to the consensus engine.
type ConsensusDigest struct {
	ConsensusEngineID ConsensusEngineID
	Data              []byte
}

func (d *ConsensusDigest) Type() byte {
	return ConsensusDigestType
}

func (d *ConsensusDigest) Encode() []byte {
	enc := []byte{ConsensusDigestType}
	enc = append(enc, d.ConsensusEngineID[:]...)
	return append(enc, d.Data...)
}

// SealDigest contains the seal or signature. This is only used by native code.
type SealDigest struct {
	ConsensusEngineID ConsensusEngineID
	Data              []byte
}

func (d *SealDigest) Type() byte {
	return SealDigestType
}

func (d *SealDigest) Encode() []byte {
	enc := []byte{SealDigestType}
	enc = append(enc, d.ConsensusEngineID[:]...)
	return append(enc, d.Data...)
}
