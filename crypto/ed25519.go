package crypto

import (
	"crypto/rand"
	"fmt"

	ed25519 "crypto/ed25519"
)

// Ed25519Keypair is a ed25519 public-private keypair
type Ed25519Keypair struct {
	public  *Ed25519PublicKey
	private *Ed25519PrivateKey
}

type Ed25519PrivateKey ed25519.PrivateKey
type Ed25519PublicKey ed25519.PublicKey

// NewEd25519Keypair returns an Ed25519 keypair given a ed25519 private key
func NewEd25519Keypair(priv ed25519.PrivateKey) *Ed25519Keypair {
	pubkey := Ed25519PublicKey(priv.Public().(ed25519.PublicKey))
	privkey := Ed25519PrivateKey(priv)
	return &Ed25519Keypair{
		public:  &pubkey,
		private: &privkey,
	}
}

// GenerateEd25519Keypair returns a new ed25519 keypair
func GenerateEd25519Keypair() (*Ed25519Keypair, error) {
	buf := make([]byte, 32)
	_, err := rand.Read(buf)
	if err != nil {
		return nil, err
	}

	priv := ed25519.NewKeyFromSeed(buf)

	return NewEd25519Keypair(priv), nil
}

// NewEd25519PublicKey returns an ed25519 public key that consists of the input bytes
// Input length must be 32 bytes
func NewEd25519PublicKey(in []byte) (*Ed25519PublicKey, error) {
	if len(in) != 32 {
		return nil, fmt.Errorf("cannot create public key: input is not 32 bytes")
	}

	pub := Ed25519PublicKey(ed25519.PublicKey(in))
	return &pub, nil
}

// NewEd25519PrivateKey returns an ed25519 private key that consists of the input bytes
// Input length must be 64 bytes
func NewEd25519PrivateKey(in []byte) (*Ed25519PrivateKey, error) {
	if len(in) != 64 {
		return nil, fmt.Errorf("cannot create private key: input is not 64 bytes")
	}

	priv := Ed25519PrivateKey(ed25519.PrivateKey(in))
	return &priv, nil
}

// Verify returns true if the signature is valid for the given message and public key, false otherwise
func Ed25519Verify(pub *Ed25519PublicKey, msg, sig []byte) bool {
	return ed25519.Verify(ed25519.PublicKey(*pub), msg, sig)
}

// Sign uses the keypair to sign the message using the ed25519 signature algorithm
func (kp *Ed25519Keypair) Sign(msg []byte) ([]byte, error) {
	return ed25519.Sign(ed25519.PrivateKey(*kp.private), msg), nil
}

// Public returns the keypair's public key
func (kp *Ed25519Keypair) Public() PublicKey {
	return kp.public
}

// Private returns the keypair's private key
func (kp *Ed25519Keypair) Private() PrivateKey {
	return kp.private
}

// Sign uses the ed25519 signature algorithm to sign the message
func (k *Ed25519PrivateKey) Sign(msg []byte) ([]byte, error) {
	return ed25519.Sign(ed25519.PrivateKey(*k), msg), nil
}

// Public returns the public key corresponding to the ed25519 private key
func (k *Ed25519PrivateKey) Public() (PublicKey, error) {
	kp := NewEd25519Keypair(ed25519.PrivateKey(*k))
	return kp.Public(), nil
}

// Encode returns the bytes underlying the Ed25519PrivateKey
func (k *Ed25519PrivateKey) Encode() []byte {
	return []byte(ed25519.PrivateKey(*k))
}

// Decode turns the input bytes into a Ed25519PrivateKey
// the input must be 64 bytes, or the function will return an error
func (k *Ed25519PrivateKey) Decode(in []byte) error {
	priv, err := NewEd25519PrivateKey(in)
	if err != nil {
		return err
	}
	*k = *priv
	return nil
}

// Verify checks that Ed25519PublicKey was used to create the signature for the message
func (k *Ed25519PublicKey) Verify(msg, sig []byte) bool {
	return ed25519.Verify(ed25519.PublicKey(*k), msg, sig)
}

// Encode returns the encoding of the Ed25519PublicKey
func (k *Ed25519PublicKey) Encode() []byte {
	return []byte(ed25519.PublicKey(*k))
}

// Decode turns the input bytes into an Ed25519PublicKey
// the input must be 32 bytes, or the function will return and error
func (k *Ed25519PublicKey) Decode(in []byte) error {
	pub, err := NewEd25519PublicKey(in)
	if err != nil {
		return err
	}
	*k = *pub
	return nil
}
