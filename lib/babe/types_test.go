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

package babe

import (
	"bytes"
	"testing"

	"github.com/ChainSafe/gossamer/lib/crypto/sr25519"
)

func TestDecodeNextEpochDescriptor(t *testing.T) {
	length := 3
	auths := []*AuthorityData{}

	for i := 0; i < length; i++ {
		kp, err := sr25519.GenerateKeypair()
		if err != nil {
			t.Fatal(err)
		}

		auth := &AuthorityData{
			ID:     kp.Public().(*sr25519.PublicKey),
			Weight: 1,
		}

		auths = append(auths, auth)
	}

	ned := &NextEpochDescriptor{
		Authorities: auths,
		Randomness:  [sr25519.VrfOutputLength]byte{1, 2, 3, 4, 5, 6, 1, 2, 3, 4, 5, 6, 1, 2, 3, 4, 5, 6, 1, 2, 3, 4, 5, 6, 1, 2, 3, 4, 5, 6, 1, 2},
	}

	enc := ned.Encode()

	res := new(NextEpochDescriptor)
	err := res.Decode(enc)
	if err != nil {
		t.Fatal(err)
	}

	enc2 := res.Encode()

	if !bytes.Equal(enc2, enc) {
		t.Fatalf("Fail: got %v expected %v", enc2, enc)
	}
}
