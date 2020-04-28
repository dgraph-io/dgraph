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
	"bytes"

	"github.com/ChainSafe/gossamer/dot/types"
	"github.com/ChainSafe/gossamer/lib/crypto/sr25519"
)

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
	Authorities []*types.AuthorityData
	Randomness  [RandomnessLength]byte // TODO: update to [32]byte when runtime is updated
}

// NextEpochDescriptorRaw contains information about the next epoch.
type NextEpochDescriptorRaw struct {
	Authorities []*types.AuthorityDataRaw
	Randomness  [RandomnessLength]byte
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
// TODO: change to io.Reader
func (n *NextEpochDescriptor) Decode(in []byte) error {
	n.Authorities = []*types.AuthorityData{}

	i := 0
	for i = 0; i < (len(in)-32)/40; i++ {
		auth := new(types.AuthorityData)
		buf := &bytes.Buffer{}
		_, err := buf.Write(in[i*40 : (i+1)*40])
		if err != nil {
			return err
		}
		err = auth.Decode(buf)
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
