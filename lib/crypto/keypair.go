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

package crypto

import (
	"github.com/ChainSafe/gossamer/lib/common"

	"github.com/btcsuite/btcutil/base58"
	"golang.org/x/crypto/blake2b"
)

// KeyType str
type KeyType = string

// Ed25519Type ed25519
const Ed25519Type KeyType = "ed25519"

//Sr25519Type sr25519
const Sr25519Type KeyType = "sr25519"

//Secp256k1Type secp256k1
const Secp256k1Type KeyType = "secp256k1"

// UnknownType is used by the GenericKeystore
const UnknownType KeyType = "unknown"

// Keypair interface
type Keypair interface {
	Type() KeyType
	Sign(msg []byte) ([]byte, error)
	Public() PublicKey
	Private() PrivateKey
}

// PublicKey interface
type PublicKey interface {
	Verify(msg, sig []byte) (bool, error)
	Encode() []byte
	Decode([]byte) error
	Address() common.Address
	Hex() string
}

// PrivateKey interface
type PrivateKey interface {
	Sign(msg []byte) ([]byte, error)
	Public() (PublicKey, error)
	Encode() []byte
	Decode([]byte) error
	Hex() string
}

var ss58Prefix = []byte("SS58PRE")

// PublicKeyToAddress returns an ss58 address given a PublicKey
// see: https://github.com/paritytech/substrate/wiki/External-Address-Format-(SS58)
// also see: https://github.com/paritytech/substrate/blob/master/primitives/core/src/crypto.rs#L275
func PublicKeyToAddress(pub PublicKey) common.Address {
	enc := append([]byte{42}, pub.Encode()...)
	hasher, err := blake2b.New(64, nil)
	if err != nil {
		return ""
	}
	_, err = hasher.Write(append(ss58Prefix, enc...))
	if err != nil {
		return ""
	}
	checksum := hasher.Sum(nil)
	return common.Address(base58.Encode(append(enc, checksum[:2]...)))
}

// PublicAddressToByteArray returns []byte address for given PublicKey Address
func PublicAddressToByteArray(add common.Address) []byte {
	k := base58.Decode(string(add))
	return k[1:33]
}
