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

package secp256k1

import (
	"reflect"
	"testing"

	"github.com/ChainSafe/gossamer/lib/common"
)

func TestSignAndVerify(t *testing.T) {
	kp, err := GenerateKeypair()
	if err != nil {
		t.Fatal(err)
	}

	msg := []byte("borkbork")
	hash, err := common.Blake2bHash(msg)
	if err != nil {
		t.Fatal(err)
	}

	sig, err := kp.private.Sign(hash[:])
	if err != nil {
		t.Fatal(err)
	}

	t.Log(sig)
	t.Log(len(sig))

	ok, err := kp.public.Verify(hash[:], sig[:64])
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("did not verify :(")
	}
}

func TestPrivateKeys(t *testing.T) {
	kp, err := GenerateKeypair()
	if err != nil {
		t.Fatal(err)
	}

	privbytes := kp.private.Encode()

	priv, err := NewPrivateKey(privbytes)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(kp.private, priv) {
		t.Fatalf("Fail: got %x expected %x", kp.private.Encode(), priv.Encode())
	}
}

func TestPublicKeys(t *testing.T) {
	kp, err := GenerateKeypair()
	if err != nil {
		t.Fatal(err)
	}

	kp2 := NewKeypair(kp.private.key)

	if !reflect.DeepEqual(kp.Public(), kp2.Public()) {
		t.Fatalf("Fail: pubkeys do not match got %x expected %x", kp2.Public(), kp.Public())
	}

	pub, err := kp.private.Public()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(pub, kp.Public()) {
		t.Fatalf("Fail: pubkeys do not match got %x expected %x", kp2.Public(), kp.Public())
	}
}

func TestEncodeAndDecodePriv(t *testing.T) {
	kp, err := GenerateKeypair()
	if err != nil {
		t.Fatal(err)
	}

	enc := kp.Private().Encode()
	res := new(PrivateKey)
	err = res.Decode(enc)
	if err != nil {
		t.Fatal(err)
	}

	exp := kp.Private().(*PrivateKey).Encode()
	if !reflect.DeepEqual(res.Encode(), exp) {
		t.Fatalf("Fail: got %x expected %x", res.Encode(), exp)
	}
}

func TestEncodeAndDecodePub(t *testing.T) {
	kp, err := GenerateKeypair()
	if err != nil {
		t.Fatal(err)
	}

	enc := kp.Public().Encode()
	res := new(PublicKey)
	err = res.Decode(enc)
	if err != nil {
		t.Fatal(err)
	}

	exp := kp.Public().(*PublicKey).Encode()
	if !reflect.DeepEqual(res.Encode(), exp) {
		t.Fatalf("Fail: got %v expected %v", res, exp)
	}
}
