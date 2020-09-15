package types

// BabeConfiguration contains the genesis data for BABE
// see: https://github.com/paritytech/substrate/blob/426c26b8bddfcdbaf8d29f45b128e0864b57de1c/core/consensus/babe/primitives/src/lib.rs#L132
type BabeConfiguration struct {
	SlotDuration       uint64 // milliseconds
	EpochLength        uint64 // duration of epoch in slots
	C1                 uint64 // (1-(c1/c2)) is the probability of a slot being empty
	C2                 uint64
	GenesisAuthorities []*AuthorityRaw
	Randomness         [32]byte
	SecondarySlots     bool
}

// BABEAuthorityRawToAuthority turns a slice of BABE AuthorityRaw into a slice of Authority
func BABEAuthorityRawToAuthority(adr []*AuthorityRaw) ([]*Authority, error) {
	ad := make([]*Authority, len(adr))
	for i, r := range adr {
		ad[i] = new(Authority)
		err := ad[i].FromRawSr25519(r)
		if err != nil {
			return nil, err
		}
	}

	return ad, nil
}

// RandomnessLength is the length of the epoch randomness (32 bytes)
const RandomnessLength = 32

// EpochInfo is internal BABE information for a given epoch
type EpochInfo struct {
	Duration   uint64 // number of slots in the epoch
	FirstBlock uint64 // number of the first block in the epoch
	Randomness [RandomnessLength]byte
}
