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
	Randomness         byte // TODO: change to [VrfOutputLength]byte when updating to new runtime
	SecondarySlots     bool
}

// AuthorityDataRaw represents the fields for the Authority Data
type AuthorityDataRaw struct {
	ID     [sr25519.PublicKeyLength]byte
	Weight uint64
}

//AuthorityData struct
type AuthorityData struct {
	id     *sr25519.PublicKey
	weight uint64
}

// NewAuthorityData returns AuthorityData with the given id and weight
func NewAuthorityData(pub *sr25519.PublicKey, weight uint64) *AuthorityData {
	return &AuthorityData{
		id:     pub,
		weight: weight,
	}
}

// ToRaw returns the AuthorityData as AuthorityDataRaw. It encodes the authority public keys.
func (a *AuthorityData) ToRaw() *AuthorityDataRaw {
	raw := new(AuthorityDataRaw)

	id := a.id.Encode()
	copy(raw.ID[:], id)

	raw.Weight = a.weight
	return raw
}

// FromRaw sets the AuthorityData given AuthorityDataRaw. It converts the byte representations of
// the authority public keys into a sr25519.PublicKey.
func (a *AuthorityData) FromRaw(raw *AuthorityDataRaw) error {
	id, err := sr25519.NewPublicKey(raw.ID[:])
	if err != nil {
		return err
	}

	a.id = id
	a.weight = raw.Weight
	return nil
}

// Encode returns the SCALE encoding of the AuthorityData.
func (a *AuthorityData) Encode() []byte {
	raw := a.ToRaw()

	enc := raw.ID[:]

	weightBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(weightBytes, raw.Weight)

	return append(enc, weightBytes...)
}

// Decode sets the AuthorityData to the SCALE decoded input.
func (a *AuthorityData) Decode(in []byte) error {
	if len(in) < 40 {
		return errors.New("length of input <40 bytes")
	}

	weight := binary.LittleEndian.Uint64(in[32:40])

	id := [32]byte{}
	copy(id[:], in[:32])

	raw := &AuthorityDataRaw{
		ID:     id,
		Weight: weight,
	}

	return a.FromRaw(raw)
}

// VrfOutputAndProof represents the fields for VRF output and proof
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

// NextEpochDescriptor contains information about the next epoch.
// It is broadcast as part of the consensus digest in the first block of the epoch.
type NextEpochDescriptor struct {
	Authorities []*AuthorityData
	Randomness  [sr25519.VrfOutputLength]byte // TODO: discrepancy between current BabeConfiguration from runtime and this
}

// NextEpochDescriptorRaw contains information about the next epoch.
type NextEpochDescriptorRaw struct {
	Authorities []*AuthorityDataRaw
	Randomness  [sr25519.VrfOutputLength]byte
}

// Encode returns the SCALE encoding of the NextEpochDescriptor.
func (n *NextEpochDescriptor) Encode() []byte {
	enc := []byte{}

	for _, a := range n.Authorities {
		enc = append(enc, a.Encode()...)
	}

	return append(enc, n.Randomness[:]...)
}

// Decode sets the NextEpochDescriptor to the SCALE decoded input.
func (n *NextEpochDescriptor) Decode(in []byte) error {
	n.Authorities = []*AuthorityData{}

	i := 0
	for i = 0; i < (len(in)-32)/40; i++ {
		auth := new(AuthorityData)
		err := auth.Decode(in[i*40 : (i+1)*40])
		if err != nil {
			return err
		}

		n.Authorities = append(n.Authorities, auth)
	}

	rand := [32]byte{}
	copy(rand[:], in[i*40:])
	n.Randomness = rand

	return nil
}
