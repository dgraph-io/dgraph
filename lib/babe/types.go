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
	"fmt"
	"math/big"

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

// NewSlot returns a new Slot
func NewSlot(start, duration, number uint64) *Slot {
	return &Slot{
		start:    start,
		duration: duration,
		number:   number,
	}
}

// AuthorityData is an alias for []*types.Authority
type AuthorityData []*types.Authority

// String returns the AuthorityData as a formatted string
func (d AuthorityData) String() string {
	str := ""
	for _, di := range []*types.Authority(d) {
		str = str + fmt.Sprintf("[key=0x%x idx=%d] ", di.Key.Encode(), di.Weight)
	}
	return str
}

// Descriptor contains the information needed to verify blocks
type Descriptor struct {
	AuthorityData []*types.Authority
	Randomness    [types.RandomnessLength]byte
	Threshold     *big.Int
}
