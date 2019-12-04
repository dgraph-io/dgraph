package crypto

import (
	"encoding/hex"
	"errors"

	sr25519 "github.com/ChainSafe/go-schnorrkel"
	"github.com/ChainSafe/gossamer/common"
)

const Sr25519PublicKeyLength int = 32
const Sr25519SeedLength int = 32
const Sr25519PrivateKeyLength int = 32
const Sr25519SignatureLength int = 64

// SigningContext is the context for signatures used or created with substrate
var SigningContext = []byte("substrate")

// Sr25519Keypair is a sr25519 public-private keypair
type Sr25519Keypair struct {
	public  *Sr25519PublicKey
	private *Sr25519PrivateKey
}

type Sr25519PublicKey struct {
	key *sr25519.PublicKey
}

type Sr25519PrivateKey struct {
	key *sr25519.SecretKey
}

// NewSr25519Keypair returns a Sr25519Keypair given a schnorrkel secret key
func NewSr25519Keypair(priv *sr25519.SecretKey) (*Sr25519Keypair, error) {
	pub, err := priv.Public()
	if err != nil {
		return nil, err
	}

	return &Sr25519Keypair{
		public:  &Sr25519PublicKey{key: pub},
		private: &Sr25519PrivateKey{key: priv},
	}, nil
}

// NewSr25519KeypairFromSeed returns a new Sr25519Keypair given a seed
func NewSr25519KeypairFromSeed(seed []byte) (*Sr25519Keypair, error) {
	buf := [Sr25519SeedLength]byte{}
	msc, err := sr25519.NewMiniSecretKeyFromRaw(buf)
	if err != nil {
		return nil, err
	}

	priv := msc.ExpandEd25519()
	pub := msc.Public()

	return &Sr25519Keypair{
		public:  &Sr25519PublicKey{key: pub},
		private: &Sr25519PrivateKey{key: priv},
	}, nil
}

// NewSr25519PrivateKey creates a new private key using the input bytes
func NewSr25519PrivateKey(in []byte) (*Sr25519PrivateKey, error) {
	if len(in) != Sr25519PrivateKeyLength {
		return nil, errors.New("input to create sr25519 private key is not 32 bytes")
	}
	priv := new(Sr25519PrivateKey)
	err := priv.Decode(in)
	return priv, err
}

// GenerateSr25519Keypair returns a new sr25519 keypair
func GenerateSr25519Keypair() (*Sr25519Keypair, error) {
	priv, pub, err := sr25519.GenerateKeypair()
	if err != nil {
		return nil, err
	}

	return &Sr25519Keypair{
		public:  &Sr25519PublicKey{key: pub},
		private: &Sr25519PrivateKey{key: priv},
	}, nil
}

func NewSr25519PublicKey(in []byte) (*Sr25519PublicKey, error) {
	if len(in) != Sr25519PublicKeyLength {
		return nil, errors.New("cannot create public key: input is not 32 bytes")
	}

	buf := [Sr25519PublicKeyLength]byte{}
	copy(buf[:], in)
	return &Sr25519PublicKey{key: sr25519.NewPublicKey(buf)}, nil
}

// Sign uses the keypair to sign the message using the sr25519 signature algorithm
func (kp *Sr25519Keypair) Sign(msg []byte) ([]byte, error) {
	return kp.private.Sign(msg)
}

// Public returns the public key corresponding to this keypair
func (kp *Sr25519Keypair) Public() PublicKey {
	return kp.public
}

// Private returns the private key corresponding to this keypair
func (kp *Sr25519Keypair) Private() PrivateKey {
	return kp.private
}

// Sign uses the private key to sign the message using the sr25519 signature algorithm
func (k *Sr25519PrivateKey) Sign(msg []byte) ([]byte, error) {
	if k.key == nil {
		return nil, errors.New("key is nil")
	}
	t := sr25519.NewSigningContext(SigningContext, msg)
	sig, err := k.key.Sign(t)
	if err != nil {
		return nil, err
	}
	enc := sig.Encode()
	return enc[:], nil
}

// Public returns the public key corresponding to this private key
func (k *Sr25519PrivateKey) Public() (PublicKey, error) {
	if k.key == nil {
		return nil, errors.New("key is nil")
	}
	pub, err := k.key.Public()
	if err != nil {
		return nil, err
	}
	return &Sr25519PublicKey{key: pub}, nil
}

// Encode returns the 32-byte encoding of the private key
func (k *Sr25519PrivateKey) Encode() []byte {
	if k.key == nil {
		return nil
	}
	enc := k.key.Encode()
	return enc[:]
}

// Decode decodes the input bytes into a private key and sets the receiver the decoded key
// Input must be 32 bytes, or else this function will error
func (k *Sr25519PrivateKey) Decode(in []byte) error {
	if len(in) != Sr25519PrivateKeyLength {
		return errors.New("input to sr25519 private key decode is not 32 bytes")
	}
	b := [Sr25519PrivateKeyLength]byte{}
	copy(b[:], in)
	k.key = &sr25519.SecretKey{}
	return k.key.Decode(b)
}

// Verify uses the sr25519 signature algorithm to verify that the message was signed by
// this public key; it returns true if this key created the signature for the message,
// false otherwise
func (k *Sr25519PublicKey) Verify(msg, sig []byte) bool {
	if k.key == nil {
		return false
	}

	if len(sig) != Sr25519SignatureLength {
		return false
	}

	b := [Sr25519SignatureLength]byte{}
	copy(b[:], sig)

	s := &sr25519.Signature{}
	err := s.Decode(b)
	if err != nil {
		return false
	}

	t := sr25519.NewSigningContext(SigningContext, msg)
	return k.key.Verify(s, t)
}

// Encode returns the 32-byte encoding of the public key
func (k *Sr25519PublicKey) Encode() []byte {
	if k.key == nil {
		return nil
	}

	enc := k.key.Encode()
	return enc[:]
}

// Decode decodes the input bytes into a public key and sets the receiver the decoded key
// Input must be 32 bytes, or else this function will error
func (k *Sr25519PublicKey) Decode(in []byte) error {
	if len(in) != Sr25519PublicKeyLength {
		return errors.New("input to sr25519 public key decode is not 32 bytes")
	}
	b := [Sr25519PublicKeyLength]byte{}
	copy(b[:], in)
	k.key = &sr25519.PublicKey{}
	return k.key.Decode(b)
}

// Address returns the ss58 address for this public key
func (k *Sr25519PublicKey) Address() common.Address {
	return PublicKeyToAddress(k)
}

// Hex returns the public key as a '0x' prefixed hex string
func (k *Sr25519PublicKey) Hex() string {
	enc := k.Encode()
	h := hex.EncodeToString(enc)
	return "0x" + h
}
