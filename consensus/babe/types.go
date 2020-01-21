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

package babe

import (
	"encoding/binary"
	"errors"

	"github.com/ChainSafe/gossamer/crypto/sr25519"
)

// BabeConfiguration contains the starting data needed for Babe
// see: https://github.com/paritytech/substrate/blob/426c26b8bddfcdbaf8d29f45b128e0864b57de1c/core/consensus/babe/primitives/src/lib.rs#L132
type BabeConfiguration struct {
	SlotDuration       uint64 // milliseconds
	EpochLength        uint64 // duration of epoch in slots
	C1                 uint64 // (1-(c1/c2)) is the probability of a slot being empty
	C2                 uint64
	GenesisAuthorities []AuthorityDataRaw
	Randomness         byte
	SecondarySlots     bool
}

type AuthorityDataRaw struct {
	Id     [sr25519.PublicKeyLength]byte
	Weight uint64
}

//nolint:structcheck
type AuthorityData struct {
	id     *sr25519.PublicKey
	weight uint64
}

// BabeHeader as defined in Polkadot RE Spec, definition 5.10 in section 5.1.4
type BabeHeader struct {
	VrfOutput          [sr25519.VrfOutputLength]byte
	VrfProof           [sr25519.VrfProofLength]byte
	BlockProducerIndex uint64
	SlotNumber         uint64
}

func (bh *BabeHeader) Encode() []byte {
	enc := []byte{}
	enc = append(enc, bh.VrfOutput[:]...)
	enc = append(enc, bh.VrfProof[:]...)
	buf := make([]byte, 8)
	binary.LittleEndian.PutUint64(buf, bh.BlockProducerIndex)
	enc = append(enc, buf...)
	binary.LittleEndian.PutUint64(buf, bh.SlotNumber)
	enc = append(enc, buf...)
	return enc
}

func (bh *BabeHeader) Decode(in []byte) error {
	if len(in) < sr25519.VrfOutputLength+sr25519.VrfProofLength+16 {
		return errors.New("input is too short: need at least VrfOutputLength (32) + VrfProofLength (64) + 16")
	}

	copy(bh.VrfOutput[:], in[:sr25519.VrfOutputLength])
	copy(bh.VrfProof[:], in[sr25519.VrfOutputLength:sr25519.VrfOutputLength+sr25519.VrfProofLength])
	bh.BlockProducerIndex = binary.LittleEndian.Uint64(in[sr25519.VrfOutputLength+sr25519.VrfProofLength : sr25519.VrfOutputLength+sr25519.VrfProofLength+8])
	bh.SlotNumber = binary.LittleEndian.Uint64(in[sr25519.VrfOutputLength+sr25519.VrfProofLength+8 : sr25519.VrfOutputLength+sr25519.VrfProofLength+16])
	return nil
}

type VrfOutputAndProof struct {
	output [sr25519.VrfOutputLength]byte
	proof  [sr25519.VrfProofLength]byte
}

// Slot represents a BABE slot
type Slot struct {
	start    uint64
	duration uint64
	number   uint64
}
