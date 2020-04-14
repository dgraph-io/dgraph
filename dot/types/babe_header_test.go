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

package types

import (
	"reflect"
	"testing"

	"github.com/ChainSafe/gossamer/lib/crypto/sr25519"
	"github.com/ChainSafe/gossamer/lib/scale"
	"github.com/stretchr/testify/require"
)

func TestBabeHeader_EncodeAndDecode(t *testing.T) {
	// arbitrary test data
	bh := &BabeHeader{
		VrfOutput:          [sr25519.VrfOutputLength]byte{0, 91, 50, 25, 214, 94, 119, 36, 71, 216, 33, 152, 85, 184, 34, 120, 61, 161, 164, 223, 76, 53, 40, 246, 76, 38, 235, 204, 43, 31, 179, 28},
		VrfProof:           [sr25519.VrfProofLength]byte{120, 23, 235, 159, 115, 122, 207, 206, 123, 232, 75, 243, 115, 255, 131, 181, 219, 241, 200, 206, 21, 22, 238, 16, 68, 49, 86, 99, 76, 139, 39, 0, 102, 106, 181, 136, 97, 141, 187, 1, 234, 183, 241, 28, 27, 229, 133, 8, 32, 246, 245, 206, 199, 142, 134, 124, 226, 217, 95, 30, 176, 246, 5, 3},
		BlockProducerIndex: 17,
		SlotNumber:         420,
	}

	enc := bh.Encode()
	bh2 := new(BabeHeader)
	err := bh2.Decode(enc)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(bh, bh2) {
		t.Fatalf("Fail: got %v expected %v", bh2, bh)
	}
}

// test decoding with DecodeCustom on BabeHeader type
func TestDecodeCustom_DecodeBabeHeader(t *testing.T) {
	// arbitrary test data
	expected := &BabeHeader{
		VrfOutput:          [sr25519.VrfOutputLength]byte{0, 91, 50, 25, 214, 94, 119, 36, 71, 216, 33, 152, 85, 184, 34, 120, 61, 161, 164, 223, 76, 53, 40, 246, 76, 38, 235, 204, 43, 31, 179, 28},
		VrfProof:           [sr25519.VrfProofLength]byte{120, 23, 235, 159, 115, 122, 207, 206, 123, 232, 75, 243, 115, 255, 131, 181, 219, 241, 200, 206, 21, 22, 238, 16, 68, 49, 86, 99, 76, 139, 39, 0, 102, 106, 181, 136, 97, 141, 187, 1, 234, 183, 241, 28, 27, 229, 133, 8, 32, 246, 245, 206, 199, 142, 134, 124, 226, 217, 95, 30, 176, 246, 5, 3},
		BlockProducerIndex: 17,
		SlotNumber:         420,
	}
	encoded := []byte{0, 91, 50, 25, 214, 94, 119, 36, 71, 216, 33, 152, 85, 184, 34, 120, 61, 161, 164, 223, 76, 53, 40, 246, 76, 38, 235, 204, 43, 31, 179, 28, 120, 23, 235, 159, 115, 122, 207, 206, 123, 232, 75, 243, 115, 255, 131, 181, 219, 241, 200, 206, 21, 22, 238, 16, 68, 49, 86, 99, 76, 139, 39, 0, 102, 106, 181, 136, 97, 141, 187, 1, 234, 183, 241, 28, 27, 229, 133, 8, 32, 246, 245, 206, 199, 142, 134, 124, 226, 217, 95, 30, 176, 246, 5, 3, 17, 0, 0, 0, 0, 0, 0, 0, 164, 1, 0, 0, 0, 0, 0, 0}
	decodedBabeHeader := new(BabeHeader)

	err := scale.DecodeCustom(encoded, decodedBabeHeader)
	require.Nil(t, err)
	require.Equal(t, expected, decodedBabeHeader)
}
