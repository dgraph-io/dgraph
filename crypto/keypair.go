package crypto

import (
	"errors"

	"github.com/ChainSafe/gossamer/common"
	"github.com/btcsuite/btcutil/base58"
	"golang.org/x/crypto/blake2b"
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
	Address() common.Address
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

var ss58Prefix = []byte("SS58PRE")

// PublicKeyToAddress returns an ss58 address given a PublicKey
// see: https://github.com/paritytech/substrate/wiki/External-Address-Format-(SS58)
// also see: https://github.com/paritytech/substrate/blob/master/primitives/core/src/crypto.rs#L275
func PublicKeyToAddress(pub PublicKey) common.Address {
	enc := pub.Encode()
	hasher, err := blake2b.New(64, nil)
	if err != nil {
		return ""
	}
	_, err = hasher.Write(append(ss58Prefix, enc...))
	if err != nil {
		return ""
	}
	checksum := hasher.Sum(nil)
	return common.Address(base58.Encode(append(enc, checksum[:2]...)))
}
