package types

import (
	"encoding/binary"
	"io"

	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/ChainSafe/gossamer/lib/crypto/sr25519"
)

// BabeConfiguration contains the genesis data for BABE
// see: https://github.com/paritytech/substrate/blob/426c26b8bddfcdbaf8d29f45b128e0864b57de1c/core/consensus/babe/primitives/src/lib.rs#L132
type BabeConfiguration struct {
	SlotDuration       uint64 // milliseconds
	EpochLength        uint64 // duration of epoch in slots
	C1                 uint64 // (1-(c1/c2)) is the probability of a slot being empty
	C2                 uint64
	GenesisAuthorities []*AuthorityDataRaw
	Randomness         [32]byte
	SecondarySlots     bool
}

// AuthorityDataRaw represents a BABE authority where their key is a byte array
type AuthorityDataRaw struct {
	ID     [sr25519.PublicKeyLength]byte
	Weight uint64
}

// Decode will decode the Reader into a AuthorityDataRaw
func (a *AuthorityDataRaw) Decode(r io.Reader) (*AuthorityDataRaw, error) {
	id, err := common.Read32Bytes(r)
	if err != nil {
		return nil, err
	}

	weight, err := common.ReadUint64(r)
	if err != nil {
		return nil, err
	}

	a = new(AuthorityDataRaw)
	a.ID = id
	a.Weight = weight

	return a, nil
}

// AuthorityData represents a BABE authority
type AuthorityData struct {
	ID     *sr25519.PublicKey
	Weight uint64
}

// NewAuthorityData returns AuthorityData with the given id and weight
func NewAuthorityData(pub *sr25519.PublicKey, weight uint64) *AuthorityData {
	return &AuthorityData{
		ID:     pub,
		Weight: weight,
	}
}

// ToRaw returns the AuthorityData as AuthorityDataRaw. It encodes the authority public keys.
func (a *AuthorityData) ToRaw() *AuthorityDataRaw {
	raw := new(AuthorityDataRaw)

	id := a.ID.Encode()
	copy(raw.ID[:], id)

	raw.Weight = a.Weight
	return raw
}

// FromRaw sets the AuthorityData given AuthorityDataRaw. It converts the byte representations of
// the authority public keys into a sr25519.PublicKey.
func (a *AuthorityData) FromRaw(raw *AuthorityDataRaw) error {
	id, err := sr25519.NewPublicKey(raw.ID[:])
	if err != nil {
		return err
	}

	a.ID = id
	a.Weight = raw.Weight
	return nil
}

// Encode returns the SCALE encoding of the AuthorityData.
func (a *AuthorityData) Encode() []byte {
	raw := a.ToRaw()

	enc := raw.ID[:]

	weightBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(weightBytes, raw.Weight)

	return append(enc, weightBytes...)
}

// Decode sets the AuthorityData to the SCALE decoded input.
func (a *AuthorityData) Decode(r io.Reader) error {
	id, err := common.Read32Bytes(r)
	if err != nil {
		return err
	}

	weight, err := common.ReadUint64(r)
	if err != nil {
		return err
	}

	raw := &AuthorityDataRaw{
		ID:     id,
		Weight: weight,
	}

	return a.FromRaw(raw)
}

// AuthorityDataRawToAuthorityData turns a slice of AuthorityDataRaw into a slice of AuthorityData
func AuthorityDataRawToAuthorityData(adr []*AuthorityDataRaw) ([]*AuthorityData, error) {
	ad := make([]*AuthorityData, len(adr))
	for i, r := range adr {
		ad[i] = new(AuthorityData)
		err := ad[i].FromRaw(r)
		if err != nil {
			return nil, err
		}
	}

	return ad, nil
}
