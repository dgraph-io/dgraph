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

// TODO: change to Schnorrkel keys
type VrfPublicKey [32]byte
type VrfPrivateKey [64]byte

// BabeConfiguration contains the starting data needed for Babe
type BabeConfiguration struct {
	SlotDuration         uint64
	C1                   uint64 // (1-(c1/c2)) is the probability of a slot being empty
	C2                   uint64
	MedianRequiredBlocks uint64
}

// TODO: change to Schnorrkel public key
type AuthorityId [32]byte

// Epoch contains the data for an epoch
type Epoch struct {
	EpochIndex     uint64
	StartSlot      uint64
	Duration       uint64      // Slot duration in milliseconds
	Authorities    AuthorityId // Schnorrkel public key of authority
	Randomness     byte
	SecondarySlots bool
}
