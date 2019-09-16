package babe

import (
	"github.com/ChainSafe/gossamer/runtime"
)

// BabeSession contains the VRF keys for the validator
type BabeSession struct {
	vrfPublicKey  VrfPublicKey
	vrfPrivateKey VrfPrivateKey
	rt            *runtime.Runtime

	// currentEpoch uint64
	// currentSlot  uint64

	// TODO: TransactionQueue
}

// NewBabeSession returns a new Babe session using the provided VRF keys and runtime
func NewBabeSession(pubkey VrfPublicKey, privkey VrfPrivateKey, rt *runtime.Runtime) *BabeSession {
	return &BabeSession{
		vrfPublicKey:  pubkey,
		vrfPrivateKey: privkey,
		rt:            rt,
	}
}
