package keystore

import (
	"sync"

	"github.com/ChainSafe/gossamer/common"
	"github.com/ChainSafe/gossamer/crypto"
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
