// Copyright 2019 ChainSafe Systems (ON) Corp.
// This file is part of gossamer.
//
// The gossamer library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The gossamer library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the gossamer library. If not, see <http://www.gnu.org/licenses/>.

package sr25519

import (
	"encoding/hex"
	"errors"

	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/ChainSafe/gossamer/lib/crypto"

	sr25519 "github.com/ChainSafe/go-schnorrkel"
)

//nolint
const (
	PublicKeyLength  int = 32
	SeedLength       int = 32
	PrivateKeyLength int = 32
	SignatureLength  int = 64
	VrfOutputLength  int = 32
	VrfProofLength   int = 64
)

// SigningContext is the context for signatures used or created with substrate
var SigningContext = []byte("substrate")

// Keypair is a sr25519 public-private keypair
type Keypair struct {
	public  *PublicKey
	private *PrivateKey
}

// PublicKey holds reference to a sr25519.PublicKey
type PublicKey struct {
	key *sr25519.PublicKey
}

// PrivateKey holds reference to a sr25519.SecretKey
type PrivateKey struct {
	key *sr25519.SecretKey
}

// NewKeypair returns a sr25519 Keypair given a schnorrkel secret key
func NewKeypair(priv *sr25519.SecretKey) (*Keypair, error) {
	pub, err := priv.Public()
	if err != nil {
		return nil, err
	}

	return &Keypair{
		public:  &PublicKey{key: pub},
		private: &PrivateKey{key: priv},
	}, nil
}

// NewKeypairFromPrivate returns a sr25519 Keypair given a *sr25519.PrivateKey
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

// NewKeypairFromSeed returns a new sr25519 Keypair given a seed
func NewKeypairFromSeed(seed []byte) (*Keypair, error) {
	buf := [SeedLength]byte{}
	copy(buf[:], seed)
	msc, err := sr25519.NewMiniSecretKeyFromRaw(buf)
	if err != nil {
		return nil, err
	}

	priv := msc.ExpandEd25519()
	pub := msc.Public()

	return &Keypair{
		public:  &PublicKey{key: pub},
		private: &PrivateKey{key: priv},
	}, nil
}

// NewKeypairFromPrivateKeyString returns a Keypair given a 0x prefixed private key string
func NewKeypairFromPrivateKeyString(in string) (*Keypair, error) {
	privBytes, err := common.HexToBytes(in)
	if err != nil {
		return nil, err
	}

	return NewKeypairFromPrivateKeyBytes(privBytes)
}

// NewKeypairFromPrivateKeyBytes returns a Keypair given a private key byte slice
func NewKeypairFromPrivateKeyBytes(in []byte) (*Keypair, error) {
	priv, err := NewPrivateKey(in)
	if err != nil {
		return nil, err
	}

	pub, err := priv.Public()
	if err != nil {
		return nil, err
	}

	return &Keypair{
		private: priv,
		public:  pub.(*PublicKey),
	}, nil
}

// NewPrivateKey creates a new private key using the input bytes
func NewPrivateKey(in []byte) (*PrivateKey, error) {
	if len(in) != PrivateKeyLength {
		return nil, errors.New("input to create sr25519 private key is not 32 bytes")
	}
	priv := new(PrivateKey)
	err := priv.Decode(in)
	return priv, err
}

// GenerateKeypair returns a new sr25519 keypair
func GenerateKeypair() (*Keypair, error) {
	priv, pub, err := sr25519.GenerateKeypair()
	if err != nil {
		return nil, err
	}

	return &Keypair{
		public:  &PublicKey{key: pub},
		private: &PrivateKey{key: priv},
	}, nil
}

// NewPublicKey returns a sr25519 public key from 32 byte input
func NewPublicKey(in []byte) (*PublicKey, error) {
	if len(in) != PublicKeyLength {
		return nil, errors.New("cannot create public key: input is not 32 bytes")
	}

	buf := [PublicKeyLength]byte{}
	copy(buf[:], in)
	return &PublicKey{key: sr25519.NewPublicKey(buf)}, nil
}

// Type returns Sr25519Type
func (kp *Keypair) Type() crypto.KeyType {
	return crypto.Sr25519Type
}

// Sign uses the keypair to sign the message using the sr25519 signature algorithm
func (kp *Keypair) Sign(msg []byte) ([]byte, error) {
	return kp.private.Sign(msg)
}

// Public returns the public key corresponding to this keypair
func (kp *Keypair) Public() crypto.PublicKey {
	return kp.public
}

// Private returns the private key corresponding to this keypair
func (kp *Keypair) Private() crypto.PrivateKey {
	return kp.private
}

// VrfSign creates a VRF output and proof from a message and private key
func (kp *Keypair) VrfSign(msg []byte) ([]byte, []byte, error) {
	return kp.private.VrfSign(msg)
}

// Sign uses the private key to sign the message using the sr25519 signature algorithm
func (k *PrivateKey) Sign(msg []byte) ([]byte, error) {
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

// VrfSign creates a VRF output and proof from a message and private key
func (k *PrivateKey) VrfSign(msg []byte) ([]byte, []byte, error) {
	t := sr25519.NewSigningContext(SigningContext, msg)
	inout, proof, err := k.key.VrfSign(t)
	if err != nil {
		return nil, nil, err
	}
	out := inout.Output().Encode()
	proofb := proof.Encode()
	return out[:], proofb[:], nil
}

// Public returns the public key corresponding to this private key
func (k *PrivateKey) Public() (crypto.PublicKey, error) {
	if k.key == nil {
		return nil, errors.New("key is nil")
	}
	pub, err := k.key.Public()
	if err != nil {
		return nil, err
	}
	return &PublicKey{key: pub}, nil
}

// Encode returns the 32-byte encoding of the private key
func (k *PrivateKey) Encode() []byte {
	if k.key == nil {
		return nil
	}
	enc := k.key.Encode()
	return enc[:]
}

// Decode decodes the input bytes into a private key and sets the receiver the decoded key
// Input must be 32 bytes, or else this function will error
func (k *PrivateKey) Decode(in []byte) error {
	if len(in) != PrivateKeyLength {
		return errors.New("input to sr25519 private key decode is not 32 bytes")
	}
	b := [PrivateKeyLength]byte{}
	copy(b[:], in)
	k.key = &sr25519.SecretKey{}
	return k.key.Decode(b)
}

// Hex returns the private key as a '0x' prefixed hex string
func (k *PrivateKey) Hex() string {
	enc := k.Encode()
	h := hex.EncodeToString(enc)
	return "0x" + h
}

// Verify uses the sr25519 signature algorithm to verify that the message was signed by
// this public key; it returns true if this key created the signature for the message,
// false otherwise
func (k *PublicKey) Verify(msg, sig []byte) (bool, error) {
	if k.key == nil {
		return false, errors.New("nil public key")
	}

	if len(sig) != SignatureLength {
		return false, errors.New("invalid signature length")
	}

	b := [SignatureLength]byte{}
	copy(b[:], sig)

	s := &sr25519.Signature{}
	err := s.Decode(b)
	if err != nil {
		return false, err
	}

	t := sr25519.NewSigningContext(SigningContext, msg)
	return k.key.Verify(s, t), nil
}

// VrfVerify confirms that the output and proof are valid given a message and public key
func (k *PublicKey) VrfVerify(msg []byte, out []byte, proof []byte) (bool, error) {
	if len(out) != VrfOutputLength {
		return false, errors.New("invalid output length")
	}

	if len(proof) != VrfProofLength {
		return false, errors.New("invalid proof length")
	}

	outb := [32]byte{}
	copy(outb[:], out)
	proofb := [64]byte{}
	copy(proofb[:], proof)

	t := sr25519.NewSigningContext(SigningContext, msg)
	o := new(sr25519.VrfOutput)
	err := o.Decode(outb)
	if err != nil {
		return false, err
	}

	p := new(sr25519.VrfProof)
	err = p.Decode(proofb)
	if err != nil {
		return false, err
	}

	inout := o.AttachInput(k.key, t)

	return k.key.VrfVerify(t, inout, p)
}

// Encode returns the 32-byte encoding of the public key
func (k *PublicKey) Encode() []byte {
	if k.key == nil {
		return nil
	}

	enc := k.key.Encode()
	return enc[:]
}

// Decode decodes the input bytes into a public key and sets the receiver the decoded key
// Input must be 32 bytes, or else this function will error
func (k *PublicKey) Decode(in []byte) error {
	if len(in) != PublicKeyLength {
		return errors.New("input to sr25519 public key decode is not 32 bytes")
	}
	b := [PublicKeyLength]byte{}
	copy(b[:], in)
	k.key = &sr25519.PublicKey{}
	return k.key.Decode(b)
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

// AsBytes returns the key as a [PublicKeyLength]byte
func (k *PublicKey) AsBytes() [PublicKeyLength]byte {
	return k.key.Encode()
}
