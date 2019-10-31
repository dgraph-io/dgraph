package crypto

import (
	"crypto/rand"
	"fmt"

	ed25519 "golang.org/x/crypto/ed25519"
)

// Ed25519Keypair is a ed25519 public-private keypair
type Ed25519Keypair struct {
	public  ed25519.PublicKey
	private ed25519.PrivateKey
}

// NewEd25519Keypair returns an Ed25519 keypair given a ed25519 private key
func NewEd25519Keypair(priv ed25519.PrivateKey) *Ed25519Keypair {
	return &Ed25519Keypair{
		public:  priv.Public().(ed25519.PublicKey),
		private: priv,
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
func NewEd25519PublicKey(in []byte) (ed25519.PublicKey, error) {
	if len(in) != 32 {
		return nil, fmt.Errorf("cannot create public key: input is not 32 bytes")
	}

	return ed25519.PublicKey(in), nil
}

// NewEd25519PrivateKey returns an ed25519 private key that consists of the input bytes
// Input length must be 64 bytes
func NewEd25519PrivateKey(in []byte) (ed25519.PrivateKey, error) {
	if len(in) != 64 {
		return nil, fmt.Errorf("cannot create private key: input is not 64 bytes")
	}

	return ed25519.PrivateKey(in), nil
}

// Sign uses the keypair to sign the message using the ed25519 signature algorith
func (kp *Ed25519Keypair) Sign(msg []byte) []byte {
	return ed25519.Sign(kp.private, msg)
}

// Public returns the keypair's public key
func (kp *Ed25519Keypair) Public() ed25519.PublicKey {
	return kp.public
}

// Private returns the keypair's private key
func (kp *Ed25519Keypair) Private() ed25519.PrivateKey {
	return kp.private
}

// Verify returns true if the signature is valid for the given message and public key, false otherwise
func Verify(pub ed25519.PublicKey, msg, sig []byte) bool {
	return ed25519.Verify(pub, msg, sig)
}
