package ed25519

import (
	ed25519 "crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/ChainSafe/gossamer/common"
	"github.com/ChainSafe/gossamer/crypto"
)

const PublicKeyLength int = 32
const SeedLength int = 32
const PrivateKeyLength int = 64
const SignatureLength int = 64

// Keypair is a ed25519 public-private keypair
type Keypair struct {
	public  *PublicKey
	private *PrivateKey
}

type PrivateKey ed25519.PrivateKey
type PublicKey ed25519.PublicKey

// NewKeypair returns an Ed25519 keypair given a ed25519 private key
func NewKeypair(priv ed25519.PrivateKey) *Keypair {
	pubkey := PublicKey(priv.Public().(ed25519.PublicKey))
	privkey := PrivateKey(priv)
	return &Keypair{
		public:  &pubkey,
		private: &privkey,
	}
}

// NewKeypairFromSeed generates a new ed25519 keypair from a 32 byte seed
func NewKeypairFromSeed(seed []byte) (*Keypair, error) {
	if len(seed) != SeedLength {
		return nil, fmt.Errorf("cannot generate key from seed: seed is not 32 bytes long")
	}
	edpriv := ed25519.NewKeyFromSeed(seed)
	return NewKeypair(edpriv), nil
}

// GenerateKeypair returns a new ed25519 keypair
func GenerateKeypair() (*Keypair, error) {
	buf := make([]byte, SeedLength)
	_, err := rand.Read(buf)
	if err != nil {
		return nil, err
	}

	priv := ed25519.NewKeyFromSeed(buf)

	return NewKeypair(priv), nil
}

// NewPublicKey returns an ed25519 public key that consists of the input bytes
// Input length must be 32 bytes
func NewPublicKey(in []byte) (*PublicKey, error) {
	if len(in) != PublicKeyLength {
		return nil, fmt.Errorf("cannot create public key: input is not 32 bytes")
	}

	pub := PublicKey(ed25519.PublicKey(in))
	return &pub, nil
}

// NewPrivateKey returns an ed25519 private key that consists of the input bytes
// Input length must be 64 bytes
func NewPrivateKey(in []byte) (*PrivateKey, error) {
	if len(in) != PrivateKeyLength {
		return nil, fmt.Errorf("cannot create private key: input is not 64 bytes")
	}

	priv := PrivateKey(ed25519.PrivateKey(in))
	return &priv, nil
}

// Verify returns true if the signature is valid for the given message and public key, false otherwise
func Verify(pub *PublicKey, msg, sig []byte) (bool, error) {
	if len(sig) != SignatureLength {
		return false, errors.New("invalid signature length")
	}

	return ed25519.Verify(ed25519.PublicKey(*pub), msg, sig), nil
}

// Sign uses the keypair to sign the message using the ed25519 signature algorithm
func (kp *Keypair) Sign(msg []byte) ([]byte, error) {
	return ed25519.Sign(ed25519.PrivateKey(*kp.private), msg), nil
}

// Public returns the keypair's public key
func (kp *Keypair) Public() crypto.PublicKey {
	return kp.public
}

// Private returns the keypair's private key
func (kp *Keypair) Private() crypto.PrivateKey {
	return kp.private
}

// Sign uses the ed25519 signature algorithm to sign the message
func (k *PrivateKey) Sign(msg []byte) ([]byte, error) {
	return ed25519.Sign(ed25519.PrivateKey(*k), msg), nil
}

// Public returns the public key corresponding to the ed25519 private key
func (k *PrivateKey) Public() (crypto.PublicKey, error) {
	kp := NewKeypair(ed25519.PrivateKey(*k))
	return kp.Public(), nil
}

// Encode returns the bytes underlying the ed25519 PrivateKey
func (k *PrivateKey) Encode() []byte {
	return []byte(ed25519.PrivateKey(*k))
}

// Decode turns the input bytes into a ed25519 PrivateKey
// the input must be 64 bytes, or the function will return an error
func (k *PrivateKey) Decode(in []byte) error {
	priv, err := NewPrivateKey(in)
	if err != nil {
		return err
	}
	*k = *priv
	return nil
}

// Verify checks that Ed25519PublicKey was used to create the signature for the message
func (k *PublicKey) Verify(msg, sig []byte) (bool, error) {
	if len(sig) != SignatureLength {
		return false, errors.New("invalid signature length")
	}
	return ed25519.Verify(ed25519.PublicKey(*k), msg, sig), nil
}

// Encode returns the encoding of the ed25519 PublicKey
func (k *PublicKey) Encode() []byte {
	return []byte(ed25519.PublicKey(*k))
}

// Decode turns the input bytes into an ed25519 PublicKey
// the input must be 32 bytes, or the function will return and error
func (k *PublicKey) Decode(in []byte) error {
	pub, err := NewPublicKey(in)
	if err != nil {
		return err
	}
	*k = *pub
	return nil
}

// Address returns the ss58 address for this public key
func (k *PublicKey) Address() common.Address {
	return crypto.PublicKeyToAddress(k)
}

// Hex returns the public key as a '0x' prefixed hex string
func (k *PublicKey) Hex() string {
	enc := k.Encode()
	h := hex.EncodeToString(enc)
	return "0x" + h
}
