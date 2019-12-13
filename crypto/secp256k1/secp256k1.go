package secp256k1

import (
	"crypto/ecdsa"
	"encoding/hex"
	"errors"

	"github.com/ChainSafe/gossamer/common"
	"github.com/ChainSafe/gossamer/crypto"
	secp256k1 "github.com/ethereum/go-ethereum/crypto"
)

const PrivateKeyLength = 32
const SignatureLength = 64
const MessageLength = 32

type Keypair struct {
	public  *PublicKey
	private *PrivateKey
}

type PublicKey struct {
	key ecdsa.PublicKey
}

type PrivateKey struct {
	key ecdsa.PrivateKey
}

func NewKeypair(pk ecdsa.PrivateKey) *Keypair {
	pub := pk.Public()

	return &Keypair{
		public:  &PublicKey{key: *pub.(*ecdsa.PublicKey)},
		private: &PrivateKey{key: pk},
	}
}

func NewKeypairFromPrivate(priv *PrivateKey) (*Keypair, error) {
	pub, err := priv.Public()
	if err != nil {
		return nil, err
	}

	return &Keypair{
		public:  pub.(*PublicKey),
		private: priv,
	}, nil
}

func NewPrivateKey(in []byte) (*PrivateKey, error) {
	if len(in) != PrivateKeyLength {
		return nil, errors.New("input to create secp256k1 private key is not 32 bytes")
	}
	priv := new(PrivateKey)
	err := priv.Decode(in)
	return priv, err
}

func GenerateKeypair() (*Keypair, error) {
	priv, err := secp256k1.GenerateKey()
	if err != nil {
		return nil, err
	}

	return NewKeypair(*priv), nil
}

func (kp *Keypair) Sign(msg []byte) ([]byte, error) {
	if len(msg) != MessageLength {
		return nil, errors.New("invalid message length: not 32 byte hash")
	}

	return secp256k1.Sign(msg, &kp.private.key)
}

func (kp *Keypair) Public() crypto.PublicKey {
	return kp.public
}
func (kp *Keypair) Private() crypto.PrivateKey {
	return kp.private
}

func (k *PublicKey) Verify(msg, sig []byte) (bool, error) {
	if len(sig) != SignatureLength {
		return false, errors.New("invalid signature length")
	}

	if len(msg) != MessageLength {
		return false, errors.New("invalid message length: not 32 byte hash")
	}

	return secp256k1.VerifySignature(k.Encode(), msg, sig), nil
}

func (k *PublicKey) Encode() []byte {
	return secp256k1.CompressPubkey(&k.key)
}

func (k *PublicKey) Decode(in []byte) error {
	pub, err := secp256k1.DecompressPubkey(in)
	if err != nil {
		return err
	}
	k.key = *pub
	return nil
}

func (k *PublicKey) Address() common.Address {
	return crypto.PublicKeyToAddress(k)
}

func (k *PublicKey) Hex() string {
	enc := k.Encode()
	h := hex.EncodeToString(enc)
	return "0x" + h
}

func (pk *PrivateKey) Sign(msg []byte) ([]byte, error) {
	if len(msg) != MessageLength {
		return nil, errors.New("invalid message length: not 32 byte hash")
	}

	return secp256k1.Sign(msg, &pk.key)
}

func (pk *PrivateKey) Public() (crypto.PublicKey, error) {
	return &PublicKey{
		key: *(pk.key.Public().(*ecdsa.PublicKey)),
	}, nil
}

func (pk *PrivateKey) Encode() []byte {
	return secp256k1.FromECDSA(&pk.key)
}

func (pk *PrivateKey) Decode(in []byte) error {
	key := secp256k1.ToECDSAUnsafe(in)
	pk.key = *key
	return nil
}
