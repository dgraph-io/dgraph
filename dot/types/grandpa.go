package types

import (
	"io"

	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/ChainSafe/gossamer/lib/crypto/ed25519"
)

// GrandpaAuthorityDataRaw represents a GRANDPA authority where their key is a byte array
type GrandpaAuthorityDataRaw struct {
	Key [ed25519.PublicKeyLength]byte
	ID  uint64
}

// Decode will decode the Reader into a GrandpaAuthorityDataRaw
func (a *GrandpaAuthorityDataRaw) Decode(r io.Reader) (*GrandpaAuthorityDataRaw, error) {
	key, err := common.Read32Bytes(r)
	if err != nil {
		return nil, err
	}

	id, err := common.ReadUint64(r)
	if err != nil {
		return nil, err
	}

	a = new(GrandpaAuthorityDataRaw)
	a.Key = key
	a.ID = id

	return a, nil
}

// GrandpaAuthorityData represents a GRANDPA authority
type GrandpaAuthorityData struct {
	Key *ed25519.PublicKey
	ID  uint64
}

// NewGrandpaAuthorityData returns AuthorityData with the given key and ID
func NewGrandpaAuthorityData(pub *ed25519.PublicKey, id uint64) *GrandpaAuthorityData {
	return &GrandpaAuthorityData{
		Key: pub,
		ID:  id,
	}
}

// ToRaw returns the GrandpaAuthorityData as GrandpaAuthorityDataRaw. It encodes the authority public keys.
func (a *GrandpaAuthorityData) ToRaw() *GrandpaAuthorityDataRaw {
	raw := new(GrandpaAuthorityDataRaw)

	raw.Key = a.Key.AsBytes()
	raw.ID = a.ID
	return raw
}

// FromRaw sets the GrandpaAuthorityData given GrandpaAuthorityDataRaw. It converts the byte representations of
// the authority public keys into a ed25519.PublicKey.
func (a *GrandpaAuthorityData) FromRaw(raw *GrandpaAuthorityDataRaw) error {
	key, err := ed25519.NewPublicKey(raw.Key[:])
	if err != nil {
		return err
	}

	a.Key = key
	a.ID = raw.ID
	return nil
}

// GrandpaAuthorityDataRawToAuthorityData turns a slice of AuthorityDataRaw into a slice of AuthorityData
func GrandpaAuthorityDataRawToAuthorityData(adr []*GrandpaAuthorityDataRaw) ([]*GrandpaAuthorityData, error) {
	ad := make([]*GrandpaAuthorityData, len(adr))
	for i, r := range adr {
		ad[i] = new(GrandpaAuthorityData)
		err := ad[i].FromRaw(r)
		if err != nil {
			return nil, err
		}
	}

	return ad, nil
}
