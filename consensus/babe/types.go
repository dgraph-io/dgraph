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
	Duration       uint64
	Authorities    AuthorityId // Schnorrkel public key of authority
	Randomness     byte
	SecondarySlots bool
}
