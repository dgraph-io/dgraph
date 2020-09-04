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

package keystore

import (
	"bytes"
	"sync"

	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/ChainSafe/gossamer/lib/crypto"
	"github.com/ChainSafe/gossamer/lib/crypto/ed25519"
	"github.com/ChainSafe/gossamer/lib/crypto/secp256k1"
	"github.com/ChainSafe/gossamer/lib/crypto/sr25519"
)

// GenericKeystore holds keys of any type
type GenericKeystore struct {
	name Name
	keys map[common.Address]crypto.Keypair // map of public key encodings to keypairs
	lock sync.RWMutex
}

// NewGenericKeystore creates a new GenericKeystore
func NewGenericKeystore(name Name) *GenericKeystore {
	return &GenericKeystore{
		name: name,
		keys: make(map[common.Address]crypto.Keypair),
	}
}

// Name returns the keystore's name
func (ks *GenericKeystore) Name() Name {
	return ks.name
}

// Type returns UnknownType since the keystore may contain keys of any type
func (ks *GenericKeystore) Type() crypto.KeyType {
	return crypto.UnknownType
}

// Size returns the number of keys in the keystore
func (ks *GenericKeystore) Size() int {
	return len(ks.Keypairs())
}

// Insert adds a keypair to the keystore
func (ks *GenericKeystore) Insert(kp crypto.Keypair) {
	ks.lock.Lock()
	defer ks.lock.Unlock()

	pub := kp.Public()
	addr := crypto.PublicKeyToAddress(pub)
	ks.keys[addr] = kp
}

// GetKeypair returns a keypair corresponding to the given public key, or nil if it doesn't exist
func (ks *GenericKeystore) GetKeypair(pub crypto.PublicKey) crypto.Keypair {
	for _, key := range ks.keys {
		if bytes.Equal(key.Public().Encode(), pub.Encode()) {
			return key
		}
	}
	return nil
}

// GetKeypairFromAddress returns a keypair corresponding to the given address, or nil if it doesn't exist
func (ks *GenericKeystore) GetKeypairFromAddress(pub common.Address) crypto.Keypair {
	ks.lock.RLock()
	defer ks.lock.RUnlock()
	return ks.keys[pub]
}

// PublicKeys returns all public keys in the keystore
func (ks *GenericKeystore) PublicKeys() []crypto.PublicKey {
	srkeys := []crypto.PublicKey{}
	if ks.keys == nil {
		return srkeys
	}

	for _, key := range ks.keys {
		srkeys = append(srkeys, key.Public())
	}

	return srkeys
}

// Keypairs returns all keypairs in the keystore
func (ks *GenericKeystore) Keypairs() []crypto.Keypair {
	srkeys := []crypto.Keypair{}
	if ks.keys == nil {
		return srkeys
	}

	for _, key := range ks.keys {
		srkeys = append(srkeys, key)
	}
	return srkeys
}

// NumSr25519Keys returns the number of sr25519 keys in the keystore
func (ks *GenericKeystore) NumSr25519Keys() int {
	return len(ks.Sr25519Keypairs())
}

// NumEd25519Keys returns the number of ed25519 keys in the keystore
func (ks *GenericKeystore) NumEd25519Keys() int {
	return len(ks.Ed25519Keypairs())
}

// Ed25519PublicKeys keys
func (ks *GenericKeystore) Ed25519PublicKeys() []crypto.PublicKey {
	edkeys := []crypto.PublicKey{}
	if ks.keys == nil {
		return edkeys
	}

	for _, key := range ks.keys {
		if _, ok := key.(*ed25519.Keypair); ok {
			edkeys = append(edkeys, key.Public())
		}
	}
	return edkeys
}

// Ed25519Keypairs Keypair
func (ks *GenericKeystore) Ed25519Keypairs() []crypto.Keypair {
	edkeys := []crypto.Keypair{}
	if ks.keys == nil {
		return edkeys
	}

	for _, key := range ks.keys {
		if _, ok := key.(*ed25519.Keypair); ok {
			edkeys = append(edkeys, key)
		}
	}
	return edkeys
}

// Sr25519PublicKeys PublicKey
func (ks *GenericKeystore) Sr25519PublicKeys() []crypto.PublicKey {
	srkeys := []crypto.PublicKey{}
	if ks.keys == nil {
		return srkeys
	}

	for _, key := range ks.keys {
		if _, ok := key.(*sr25519.Keypair); ok {
			srkeys = append(srkeys, key.Public())
		}
	}
	return srkeys
}

// Sr25519Keypairs Keypair
func (ks *GenericKeystore) Sr25519Keypairs() []crypto.Keypair {
	srkeys := []crypto.Keypair{}
	if ks.keys == nil {
		return srkeys
	}

	for _, key := range ks.keys {
		if _, ok := key.(*sr25519.Keypair); ok {
			srkeys = append(srkeys, key)
		}
	}
	return srkeys
}

// Secp256k1PublicKeys PublicKey
func (ks *GenericKeystore) Secp256k1PublicKeys() []crypto.PublicKey {
	sckeys := []crypto.PublicKey{}
	if ks.keys == nil {
		return sckeys
	}

	for _, key := range ks.keys {
		if _, ok := key.(*secp256k1.Keypair); ok {
			sckeys = append(sckeys, key.Public())
		}
	}
	return sckeys
}

// Secp256k1Keypairs Keypair
func (ks *GenericKeystore) Secp256k1Keypairs() []crypto.Keypair {
	sckeys := []crypto.Keypair{}
	if ks.keys == nil {
		return sckeys
	}

	for _, key := range ks.keys {
		if _, ok := key.(*secp256k1.Keypair); ok {
			sckeys = append(sckeys, key)
		}
	}
	return sckeys
}
