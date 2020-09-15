// Copyright 2020 ChainSafe Systems (ON) Corp.
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
	"encoding/binary"
	"io"

	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/ChainSafe/gossamer/lib/crypto"
	"github.com/ChainSafe/gossamer/lib/crypto/sr25519"
)

// Authority struct to hold authority data
type Authority struct {
	Key    crypto.PublicKey
	Weight uint64
}

// NewAuthority function to create Authority object
func NewAuthority(pub crypto.PublicKey, weight uint64) *Authority {
	return &Authority{
		Key:    pub,
		Weight: weight,
	}
}

// Encode returns the SCALE encoding of the BABEAuthorityData.
func (a *Authority) Encode() []byte {
	raw := a.ToRaw()

	enc := raw.Key[:]

	weightBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(weightBytes, raw.Weight)

	return append(enc, weightBytes...)
}

// DecodeSr25519 sets the Authority to the SCALE decoded input for Authority containing SR25519 Keys.
func (a *Authority) DecodeSr25519(r io.Reader) error {
	id, err := common.Read32Bytes(r)
	if err != nil {
		return err
	}

	weight, err := common.ReadUint64(r)
	if err != nil {
		return err
	}

	raw := &AuthorityRaw{
		Key:    id,
		Weight: weight,
	}

	return a.FromRawSr25519(raw)
}

// ToRaw returns the BABEAuthorityData as BABEAuthorityDataRaw. It encodes the authority public keys.
func (a *Authority) ToRaw() *AuthorityRaw {
	raw := new(AuthorityRaw)

	id := a.Key.Encode()
	copy(raw.Key[:], id)

	raw.Weight = a.Weight
	return raw
}

// FromRawSr25519 sets the Authority given AuthorityRaw. It converts the byte representations of
// the authority public keys into a sr25519.PublicKey.
func (a *Authority) FromRawSr25519(raw *AuthorityRaw) error {
	id, err := sr25519.NewPublicKey(raw.Key[:])
	if err != nil {
		return err
	}

	a.Key = id
	a.Weight = raw.Weight
	return nil
}

// AuthorityRaw struct to hold raw authority data
type AuthorityRaw struct {
	Key    [sr25519.PublicKeyLength]byte
	Weight uint64
}

// Decode will decode the Reader into a AuthorityRaw
func (a *AuthorityRaw) Decode(r io.Reader) (*AuthorityRaw, error) {
	id, err := common.Read32Bytes(r)
	if err != nil {
		return nil, err
	}

	weight, err := common.ReadUint64(r)
	if err != nil {
		return nil, err
	}

	a = new(AuthorityRaw)
	a.Key = id
	a.Weight = weight

	return a, nil
}
