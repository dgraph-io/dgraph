package crypto

import (
	"errors"
)

type KeyType = string

const Ed25519Type KeyType = "ed25519"
const Sr25519Type KeyType = "sr25519"

type Keypair interface {
	Sign(msg []byte) ([]byte, error)
	Public() PublicKey
	Private() PrivateKey
}

type PublicKey interface {
	Verify(msg, sig []byte) bool
	Encode() []byte
	Decode([]byte) error
	Hex() string
}

type PrivateKey interface {
	Sign(msg []byte) ([]byte, error)
	Public() (PublicKey, error)
	Encode() []byte
	Decode([]byte) error
}

func DecodePrivateKey(in []byte, keytype KeyType) (priv PrivateKey, err error) {
	if keytype == Ed25519Type {
		priv, err = NewEd25519PrivateKey(in)
	} else if keytype == Sr25519Type {
		priv, err = NewSr25519PrivateKey(in)
	} else {
		return nil, errors.New("cannot decode key: invalid key type")
	}

	return priv, err
}
