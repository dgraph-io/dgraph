package keystore

import (
	"bytes"
	"sync"

	"github.com/ChainSafe/gossamer/common"
	"github.com/ChainSafe/gossamer/crypto"
	"github.com/ChainSafe/gossamer/crypto/ed25519"
	"github.com/ChainSafe/gossamer/crypto/secp256k1"
	"github.com/ChainSafe/gossamer/crypto/sr25519"
)

type Keystore struct {
	// map of public key encodings to keypairs
	keys map[common.Address]crypto.Keypair
	lock sync.RWMutex
}

func NewKeystore() *Keystore {
	return &Keystore{
		keys: make(map[common.Address]crypto.Keypair),
	}
}

func (ks *Keystore) Insert(kp crypto.Keypair) {
	ks.lock.Lock()
	defer ks.lock.Unlock()
	pub := kp.Public()
	addr := crypto.PublicKeyToAddress(pub)
	ks.keys[addr] = kp
}

func (ks *Keystore) Get(pub common.Address) crypto.Keypair {
	ks.lock.RLock()
	defer ks.lock.RUnlock()
	return ks.keys[pub]
}

func (ks *Keystore) Ed25519PublicKeys() []crypto.PublicKey {
	edkeys := []crypto.PublicKey{}
	for _, key := range ks.keys {
		if _, ok := key.(*ed25519.Keypair); ok {
			edkeys = append(edkeys, key.Public())
		}
	}
	return edkeys
}

func (ks *Keystore) Ed25519Keypairs() []crypto.Keypair {
	edkeys := []crypto.Keypair{}
	for _, key := range ks.keys {
		if _, ok := key.(*ed25519.Keypair); ok {
			edkeys = append(edkeys, key)
		}
	}
	return edkeys
}

func (ks *Keystore) Sr25519PublicKeys() []crypto.PublicKey {
	srkeys := []crypto.PublicKey{}
	for _, key := range ks.keys {
		if _, ok := key.(*sr25519.Keypair); ok {
			srkeys = append(srkeys, key.Public())
		}
	}
	return srkeys
}

func (ks *Keystore) Sr25519Keypairs() []crypto.Keypair {
	srkeys := []crypto.Keypair{}
	for _, key := range ks.keys {
		if _, ok := key.(*sr25519.Keypair); ok {
			srkeys = append(srkeys, key)
		}
	}
	return srkeys
}

func (ks *Keystore) Secp256k1PublicKeys() []crypto.PublicKey {
	sckeys := []crypto.PublicKey{}
	for _, key := range ks.keys {
		if _, ok := key.(*secp256k1.Keypair); ok {
			sckeys = append(sckeys, key.Public())
		}
	}
	return sckeys
}

func (ks *Keystore) Secp256k1Keypairs() []crypto.Keypair {
	sckeys := []crypto.Keypair{}
	for _, key := range ks.keys {
		if _, ok := key.(*secp256k1.Keypair); ok {
			sckeys = append(sckeys, key)
		}
	}
	return sckeys
}

func (ks *Keystore) GetKeypair(pub crypto.PublicKey) crypto.Keypair {
	for _, key := range ks.keys {
		if bytes.Equal(key.Public().Encode(), pub.Encode()) {
			return key
		}
	}
	return nil
}
