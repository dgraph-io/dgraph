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
)

// BasicKeystore holds keys of a certain type
type BasicKeystore struct {
	name Name
	typ  crypto.KeyType
	keys map[common.Address]crypto.Keypair // map of public key encodings to keypairs
	lock sync.RWMutex
}

// NewBasicKeystore creates a new BasicKeystore with the given key type
func NewBasicKeystore(name Name, typ crypto.KeyType) *BasicKeystore {
	return &BasicKeystore{
		name: name,
		typ:  typ,
		keys: make(map[common.Address]crypto.Keypair),
	}
}

// Name returns the keystore's name
func (ks *BasicKeystore) Name() Name {
	return ks.name
}

// Type returns the keystore's key type
func (ks *BasicKeystore) Type() crypto.KeyType {
	return ks.typ
}

// Size returns the number of keys in the keystore
func (ks *BasicKeystore) Size() int {
	return len(ks.Keypairs())
}

// Insert adds a keypair to the keystore
func (ks *BasicKeystore) Insert(kp crypto.Keypair) {
	ks.lock.Lock()
	defer ks.lock.Unlock()

	if kp.Type() != ks.typ {
		return
	}

	pub := kp.Public()
	addr := crypto.PublicKeyToAddress(pub)
	ks.keys[addr] = kp
}

// GetKeypair returns a keypair corresponding to the given public key, or nil if it doesn't exist
func (ks *BasicKeystore) GetKeypair(pub crypto.PublicKey) crypto.Keypair {
	for _, key := range ks.keys {
		if bytes.Equal(key.Public().Encode(), pub.Encode()) {
			return key
		}
	}
	return nil
}

// GetKeypairFromAddress returns a keypair corresponding to the given address, or nil if it doesn't exist
func (ks *BasicKeystore) GetKeypairFromAddress(pub common.Address) crypto.Keypair {
	ks.lock.RLock()
	defer ks.lock.RUnlock()
	return ks.keys[pub]
}

// PublicKeys returns all public keys in the keystore
func (ks *BasicKeystore) PublicKeys() []crypto.PublicKey {
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
func (ks *BasicKeystore) Keypairs() []crypto.Keypair {
	srkeys := []crypto.Keypair{}
	if ks.keys == nil {
		return srkeys
	}

	for _, key := range ks.keys {
		srkeys = append(srkeys, key)
	}
	return srkeys
}
