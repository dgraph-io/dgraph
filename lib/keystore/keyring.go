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

package keystore

import (
	"reflect"

	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/ChainSafe/gossamer/lib/crypto"
	"github.com/ChainSafe/gossamer/lib/crypto/ed25519"
	"github.com/ChainSafe/gossamer/lib/crypto/sr25519"
)

// private keys generated using `subkey inspect //Name`
var privateKeys = []string{
	"0xe5be9a5092b81bca64be81d212e7f2f9eba183bb7a90954f7b76361f6edb5c0a",
	"0x398f0c28f98885e046333d4a41c19cee4c37368a9832c6502f6cfd182e2aef89",
	"0xbc1ede780f784bb6991a585e4f6e61522c14e1cae6ad0895fb57b9a205a8f938",
	"0x868020ae0687dda7d57565093a69090211449845a7e11453612800b663307246",
	"0x786ad0e2df456fe43dd1f91ebca22e235bc162e0bb8d53c633e8c85b2af68b7a",
	"0x42438b7883391c05512a938e36c2df0131e088b3756d6aa7a755fbff19d2f842",
	"0xcdb035129162df39b70e604ab75162084e176f48897cdafb7d72c4a542a86dda",
	"0x51079fc9e1817f8d4f245d66b325a94d9cafdb8691acbfe85415dce3ae7a62b9",
	"0x7c04eea9d31ce0d9ee256d7c561dc29f20d1119a125e95713c967dcd8d14f22d",
}

// Keyring represents a test keyring
type Keyring interface {
	Alice() crypto.Keypair
	Bob() crypto.Keypair
	Charlie() crypto.Keypair
	Dave() crypto.Keypair
	Eve() crypto.Keypair
	Ferdie() crypto.Keypair
	George() crypto.Keypair
	Heather() crypto.Keypair
	Ian() crypto.Keypair
}

// Sr25519Keyring represents a test keyring
type Sr25519Keyring struct {
	KeyAlice   *sr25519.Keypair
	KeyBob     *sr25519.Keypair
	KeyCharlie *sr25519.Keypair
	KeyDave    *sr25519.Keypair
	KeyEve     *sr25519.Keypair
	KeyFerdie  *sr25519.Keypair
	KeyGeorge  *sr25519.Keypair
	KeyHeather *sr25519.Keypair
	KeyIan     *sr25519.Keypair
}

// NewSr25519Keyring returns an initialized sr25519 Keyring
func NewSr25519Keyring() (*Sr25519Keyring, error) {
	kr := new(Sr25519Keyring)

	v := reflect.ValueOf(kr).Elem()

	for i := 0; i < v.NumField(); i++ {
		who := v.Field(i)
		h, err := common.HexToBytes(privateKeys[i])
		if err != nil {
			return nil, err
		}

		kp, err := sr25519.NewKeypairFromSeed(h)
		if err != nil {
			return nil, err
		}

		who.Set(reflect.ValueOf(kp))
	}

	return kr, nil
}

// Alice returns Alice's key
func (kr *Sr25519Keyring) Alice() crypto.Keypair {
	return kr.KeyAlice
}

// Bob returns Bob's key
func (kr *Sr25519Keyring) Bob() crypto.Keypair {
	return kr.KeyBob
}

// Charlie returns Charlie's key
func (kr *Sr25519Keyring) Charlie() crypto.Keypair {
	return kr.KeyCharlie
}

// Dave returns Dave's key
func (kr *Sr25519Keyring) Dave() crypto.Keypair {
	return kr.KeyDave
}

// Eve returns Eve's key
func (kr *Sr25519Keyring) Eve() crypto.Keypair {
	return kr.KeyEve
}

// Ferdie returns Ferdie's key
func (kr *Sr25519Keyring) Ferdie() crypto.Keypair {
	return kr.KeyFerdie
}

// George returns George's key
func (kr *Sr25519Keyring) George() crypto.Keypair {
	return kr.KeyGeorge
}

// Heather returns Heather's key
func (kr *Sr25519Keyring) Heather() crypto.Keypair {
	return kr.KeyHeather
}

// Ian returns Ian's key
func (kr *Sr25519Keyring) Ian() crypto.Keypair {
	return kr.KeyIan
}

// Ed25519Keyring represents a test ed25519 keyring
type Ed25519Keyring struct {
	KeyAlice   *ed25519.Keypair
	KeyBob     *ed25519.Keypair
	KeyCharlie *ed25519.Keypair
	KeyDave    *ed25519.Keypair
	KeyEve     *ed25519.Keypair
	KeyFerdie  *ed25519.Keypair
	KeyGeorge  *ed25519.Keypair
	KeyHeather *ed25519.Keypair
	KeyIan     *ed25519.Keypair

	Keys []*ed25519.Keypair
}

// NewEd25519Keyring returns an initialized ed25519 Keyring
func NewEd25519Keyring() (*Ed25519Keyring, error) {
	kr := new(Ed25519Keyring)
	kr.Keys = []*ed25519.Keypair{}

	v := reflect.ValueOf(kr).Elem()

	for i := 0; i < v.NumField()-1; i++ {
		who := v.Field(i)
		kp, err := ed25519.NewKeypairFromPrivateKeyString(privateKeys[i])
		if err != nil {
			return nil, err
		}
		who.Set(reflect.ValueOf(kp))

		kr.Keys = append(kr.Keys, kp)
	}

	return kr, nil
}

// Alice returns Alice's key
func (kr *Ed25519Keyring) Alice() crypto.Keypair {
	return kr.KeyAlice
}

// Bob returns Bob's key
func (kr *Ed25519Keyring) Bob() crypto.Keypair {
	return kr.KeyBob
}

// Charlie returns Charlie's key
func (kr *Ed25519Keyring) Charlie() crypto.Keypair {
	return kr.KeyCharlie
}

// Dave returns Dave's key
func (kr *Ed25519Keyring) Dave() crypto.Keypair {
	return kr.KeyDave
}

// Eve returns Eve's key
func (kr *Ed25519Keyring) Eve() crypto.Keypair {
	return kr.KeyEve
}

// Ferdie returns Ferdie's key
func (kr *Ed25519Keyring) Ferdie() crypto.Keypair {
	return kr.KeyFerdie
}

// George returns George's key
func (kr *Ed25519Keyring) George() crypto.Keypair {
	return kr.KeyGeorge
}

// Heather returns Heather's key
func (kr *Ed25519Keyring) Heather() crypto.Keypair {
	return kr.KeyHeather
}

// Ian returns Ian's key
func (kr *Ed25519Keyring) Ian() crypto.Keypair {
	return kr.KeyIan
}
