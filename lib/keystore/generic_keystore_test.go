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
	"sort"
	"strings"
	"testing"

	"github.com/ChainSafe/gossamer/lib/crypto"
	"github.com/ChainSafe/gossamer/lib/crypto/ed25519"
	"github.com/ChainSafe/gossamer/lib/crypto/secp256k1"
	"github.com/ChainSafe/gossamer/lib/crypto/sr25519"
)

func TestGetSr25519PublicKeys(t *testing.T) {
	ks := NewGenericKeystore("test")

	expectedPubkeys := []crypto.PublicKey{}
	numKps := 12

	for i := 0; i < numKps; i++ {
		kp, err := sr25519.GenerateKeypair()
		if err != nil {
			t.Fatal(err)
		}
		ks.Insert(kp)
		expectedPubkeys = append(expectedPubkeys, kp.Public())
	}

	for i := 0; i < numKps; i++ {
		kp, err := ed25519.GenerateKeypair()
		if err != nil {
			t.Fatal(err)
		}
		ks.Insert(kp)
	}

	pubkeys := ks.Sr25519PublicKeys()
	sort.Slice(pubkeys, func(i, j int) bool {
		return strings.Compare(string(pubkeys[i].Address()), string(pubkeys[j].Address())) < 0
	})
	sort.Slice(expectedPubkeys, func(i, j int) bool {
		return strings.Compare(string(expectedPubkeys[i].Address()), string(expectedPubkeys[j].Address())) < 0
	})

	if !reflect.DeepEqual(pubkeys, expectedPubkeys) {
		t.Fatalf("Fail: got %v expected %v", pubkeys, expectedPubkeys)
	}
}

func TestGetEd25519PublicKeys(t *testing.T) {
	ks := NewGenericKeystore("test")

	expectedPubkeys := []crypto.PublicKey{}
	numKps := 10

	for i := 0; i < numKps; i++ {
		kp, err := ed25519.GenerateKeypair()
		if err != nil {
			t.Fatal(err)
		}
		ks.Insert(kp)
		expectedPubkeys = append(expectedPubkeys, kp.Public())
	}

	for i := 0; i < numKps; i++ {
		kp, err := secp256k1.GenerateKeypair()
		if err != nil {
			t.Fatal(err)
		}
		ks.Insert(kp)
	}

	pubkeys := ks.Ed25519PublicKeys()
	sort.Slice(pubkeys, func(i, j int) bool {
		return strings.Compare(string(pubkeys[i].Address()), string(pubkeys[j].Address())) < 0
	})
	sort.Slice(expectedPubkeys, func(i, j int) bool {
		return strings.Compare(string(expectedPubkeys[i].Address()), string(expectedPubkeys[j].Address())) < 0
	})

	if !reflect.DeepEqual(pubkeys, expectedPubkeys) {
		t.Fatalf("Fail: got %v expected %v", pubkeys, expectedPubkeys)
	}
}

func TestGetSecp256k1PublicKeys(t *testing.T) {
	ks := NewGenericKeystore("test")

	expectedPubkeys := []crypto.PublicKey{}
	numKps := 10

	for i := 0; i < numKps; i++ {
		kp, err := secp256k1.GenerateKeypair()
		if err != nil {
			t.Fatal(err)
		}
		ks.Insert(kp)
		expectedPubkeys = append(expectedPubkeys, kp.Public())
	}

	for i := 0; i < numKps; i++ {
		kp, err := sr25519.GenerateKeypair()
		if err != nil {
			t.Fatal(err)
		}
		ks.Insert(kp)
	}

	pubkeys := ks.Secp256k1PublicKeys()
	sort.Slice(pubkeys, func(i, j int) bool {
		return strings.Compare(string(pubkeys[i].Address()), string(pubkeys[j].Address())) < 0
	})
	sort.Slice(expectedPubkeys, func(i, j int) bool {
		return strings.Compare(string(expectedPubkeys[i].Address()), string(expectedPubkeys[j].Address())) < 0
	})

	if !reflect.DeepEqual(pubkeys, expectedPubkeys) {
		t.Fatalf("Fail: got %v expected %v", pubkeys, expectedPubkeys)
	}
}
