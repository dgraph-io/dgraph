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

	"github.com/ChainSafe/gossamer/lib/crypto/sr25519"
)

var privateKeys = []string{
	"0xb7e9185065667390d2ad952a5324e8c365c9bf503dcf97c67a5ce861afe97309",
	"0x00e5029dd32ed973146bc60131d929552b087d0376a8c284c5703a7a305f9009",
	"0x2176a403c253c126b6f97d066604987e2cebad374155a51a4eb6baf29fd4050e",
	"0x1eee623155d1fbe860ae9de9bb3e1b853af54f37cefc58bca6013f36214ddd08",
	"0x4eca8fca08a08c3e295a64dda72078efc71f305f600548a0b404e7806bd2bb09",
	"0xacb6c03db1f04d23da738ff16d69153c3104e8e2d8a3572a894ee1df3b06520c",
	"0x85e1562da2878744a30d62b5a44e694a3ad587ccde20b5f8c5796cf90f5df309",
	"0x1655133c8a0339b2456ea1ee7f2adca6015b5c56109b854ccf88ca4150d8bd0f",
}

// Keyring represents a test keyring
type Keyring struct {
	Alice   *sr25519.Keypair
	Bob     *sr25519.Keypair
	Charlie *sr25519.Keypair
	Dave    *sr25519.Keypair
	Eve     *sr25519.Keypair
	Fred    *sr25519.Keypair
	George  *sr25519.Keypair
	Heather *sr25519.Keypair
}

// NewKeyring returns an initialized Keyring
func NewKeyring() (*Keyring, error) {
	kr := new(Keyring)

	v := reflect.ValueOf(kr).Elem()

	for i := 0; i < v.NumField(); i++ {
		who := v.Field(i)
		kp, err := sr25519.NewKeypairFromPrivateKeyString(privateKeys[i])
		if err != nil {
			return nil, err
		}
		who.Set(reflect.ValueOf(kp))
	}

	return kr, nil
}
