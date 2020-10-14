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
	"bytes"
	"math/big"
	"reflect"
	"testing"

	"github.com/ChainSafe/gossamer/lib/common"

	"github.com/stretchr/testify/require"
)

func TestEncodeBlock(t *testing.T) {
	// see https://github.com/paritytech/substrate/blob/master/test-utils/runtime/src/system.rs#L376
	expected := []byte{69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69,
		4, 39, 71, 171, 124, 13, 195, 139, 127, 42, 251, 168, 43, 213, 226, 214, 172, 239, 140, 49, 224, 152, 0,
		246, 96, 183, 94, 200, 74, 112, 5, 9, 159, 3, 23, 10, 46, 117, 151, 183, 183, 227, 216, 76, 5, 57, 29, 19,
		154, 98, 177, 87, 231, 135, 134, 216, 192, 130, 242, 157, 207, 76, 17, 19, 20, 0, 0, 4, 1}

	parentHash, err := common.HexToHash("0x4545454545454545454545454545454545454545454545454545454545454545")
	if err != nil {
		t.Fatal(err)
	}

	stateRoot, err := common.HexToHash("0x2747ab7c0dc38b7f2afba82bd5e2d6acef8c31e09800f660b75ec84a7005099f")
	if err != nil {
		t.Fatal(err)
	}

	extrinsicsRoot, err := common.HexToHash("0x03170a2e7597b7b7e3d84c05391d139a62b157e78786d8c082f29dcf4c111314")
	if err != nil {
		t.Fatal(err)
	}

	header := &Header{
		ParentHash:     parentHash,
		Number:         big.NewInt(1),
		StateRoot:      stateRoot,
		ExtrinsicsRoot: extrinsicsRoot,
		Digest:         [][]byte{},
	}

	block := NewBlock(header, NewBody([]byte{4, 1}))
	enc, err := block.Encode()
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(enc, expected) {
		t.Fatalf("Fail: got %x expected %x", enc, expected)
	}
}

func TestDecodeBlock(t *testing.T) {
	// see https://github.com/paritytech/substrate/blob/master/test-utils/runtime/src/system.rs#L376
	data := []byte{69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 4, 39, 71, 171, 124, 13, 195, 139, 127, 42, 251, 168, 43, 213, 226, 214, 172, 239, 140, 49, 224, 152, 0, 246, 96, 183, 94, 200, 74, 112, 5, 9, 159, 3, 23, 10, 46, 117, 151, 183, 183, 227, 216, 76, 5, 57, 29, 19, 154, 98, 177, 87, 231, 135, 134, 216, 192, 130, 242, 157, 207, 76, 17, 19, 20, 0, 0}
	bh := NewEmptyBlock()

	rw := &bytes.Buffer{}
	rw.Write(data)
	err := bh.Decode(rw)
	if err != nil {
		t.Fatal(err)
	}

	parentHash, err := common.HexToHash("0x4545454545454545454545454545454545454545454545454545454545454545")
	if err != nil {
		t.Fatal(err)
	}

	stateRoot, err := common.HexToHash("0x2747ab7c0dc38b7f2afba82bd5e2d6acef8c31e09800f660b75ec84a7005099f")
	if err != nil {
		t.Fatal(err)
	}

	extrinsicsRoot, err := common.HexToHash("0x03170a2e7597b7b7e3d84c05391d139a62b157e78786d8c082f29dcf4c111314")
	if err != nil {
		t.Fatal(err)
	}

	header := &Header{
		ParentHash:     parentHash,
		Number:         big.NewInt(1),
		StateRoot:      stateRoot,
		ExtrinsicsRoot: extrinsicsRoot,
		Digest:         [][]byte{},
	}
	expected := NewBlock(header, NewBody(nil))

	if !reflect.DeepEqual(bh, expected) {
		t.Fatalf("Fail: got %v, %v expected %v, %v", bh.Header, bh.Body, expected.Header, expected.Body)
	}
}

func TestDeepCopyBlock(t *testing.T) {
	data := []byte{69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 4, 39, 71, 171, 124, 13, 195, 139, 127, 42, 251, 168, 43, 213, 226, 214, 172, 239, 140, 49, 224, 152, 0, 246, 96, 183, 94, 200, 74, 112, 5, 9, 159, 3, 23, 10, 46, 117, 151, 183, 183, 227, 216, 76, 5, 57, 29, 19, 154, 98, 177, 87, 231, 135, 134, 216, 192, 130, 242, 157, 207, 76, 17, 19, 20, 0, 0}
	block := NewEmptyBlock()

	rw := &bytes.Buffer{}
	rw.Write(data)
	err := block.Decode(rw)
	if err != nil {
		t.Fatal(err)
	}

	bc := block.DeepCopy()
	bc.Header.ParentHash = common.Hash{}
	require.NotEqual(t, block.Header.ParentHash, bc.Header.ParentHash)
}

func TestMustEncodeBlock(t *testing.T) {
	h1, err := NewHeader(common.Hash{}, big.NewInt(0), common.Hash{}, common.Hash{}, [][]byte{{}})
	require.NoError(t, err)
	b1 := NewBlock(h1, NewBody([]byte{}))
	enc, err := b1.Encode()
	require.NoError(t, err)

	h2, err := NewHeader(common.Hash{0x1, 0x2}, big.NewInt(0), common.Hash{}, common.Hash{}, [][]byte{{}})
	require.NoError(t, err)
	b2 := NewBlock(h2, NewBody([]byte{0xa, 0xb}))
	enc2, err := b2.Encode()
	require.NoError(t, err)

	tests := []struct {
		name string
		take *Block
		want []byte
	}{
		{
			name: "correct",
			take: b1,
			want: enc,
		},
		{
			name: "correct2",
			take: b2,
			want: enc2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.take.MustEncode(); !bytes.Equal(got, tt.want) {
				t.Errorf("MustEncode() = %v, want %v", got, tt.want)
			}
		})
	}
}
