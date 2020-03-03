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
	"github.com/ChainSafe/gossamer/lib/common/optional"
)

func TestBodyToExtrinsics(t *testing.T) {
	exts := []Extrinsic{{1, 2, 3}, {7, 8, 9, 0}, {0xa, 0xb}}

	body, err := NewBodyFromExtrinsics(exts)
	if err != nil {
		t.Fatal(err)
	}

	res, err := body.AsExtrinsics()
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(res, exts) {
		t.Fatalf("Fail: got %x expected %x", res, exts)
	}
}

func TestBlockDataEncodeEmpty(t *testing.T) {
	hash := common.NewHash([]byte{0})

	bd := &BlockData{
		Hash:          hash,
		Header:        optional.NewHeader(false, nil),
		Body:          optional.NewBody(false, nil),
		Receipt:       optional.NewBytes(false, nil),
		MessageQueue:  optional.NewBytes(false, nil),
		Justification: optional.NewBytes(false, nil),
	}

	expected := append([]byte{0}, hash[:]...)
	expected = append(expected, []byte{0, 0, 0, 0}...)

	enc, err := bd.Encode()
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(expected, enc) {
		t.Fatalf("Fail: got %x expected %x", enc, expected)
	}
}

func TestBlockDataEncodeHeader(t *testing.T) {
	hash := common.NewHash([]byte{0})
	testHash := common.NewHash([]byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 0xa, 0xb, 0xc, 0xd, 0xe, 0xf, 0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 0xa, 0xb, 0xc, 0xd, 0xe, 0xf})

	header := &optional.CoreHeader{
		ParentHash:     testHash,
		Number:         big.NewInt(1),
		StateRoot:      testHash,
		ExtrinsicsRoot: testHash,
		Digest:         [][]byte{{0xe, 0xf}},
	}

	bd := &BlockData{
		Hash:          hash,
		Header:        optional.NewHeader(true, header),
		Body:          optional.NewBody(false, nil),
		Receipt:       optional.NewBytes(false, nil),
		MessageQueue:  optional.NewBytes(false, nil),
		Justification: optional.NewBytes(false, nil),
	}

	expected, err := common.HexToBytes("0x000000000000000000000000000000000000000000000000000000000000000001000102030405060708090a0b0c0d0e0f000102030405060708090a0b0c0d0e0f04000102030405060708090a0b0c0d0e0f000102030405060708090a0b0c0d0e0f000102030405060708090a0b0c0d0e0f000102030405060708090a0b0c0d0e0f04080e0f00000000")
	if err != nil {
		t.Fatal(err)
	}

	enc, err := bd.Encode()
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(expected, enc) {
		t.Fatalf("Fail: got %x expected %x", enc, expected)
	}
}

func TestBlockDataEncodeBody(t *testing.T) {
	hash := common.NewHash([]byte{0})
	body := optional.CoreBody{0xa, 0xb, 0xc, 0xd}

	bd := &BlockData{
		Hash:          hash,
		Header:        optional.NewHeader(false, nil),
		Body:          optional.NewBody(true, body),
		Receipt:       optional.NewBytes(false, nil),
		MessageQueue:  optional.NewBytes(false, nil),
		Justification: optional.NewBytes(false, nil),
	}

	expected, err := common.HexToBytes("0x00000000000000000000000000000000000000000000000000000000000000000001100a0b0c0d000000")
	if err != nil {
		t.Fatal(err)
	}

	enc, err := bd.Encode()
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(expected, enc) {
		t.Fatalf("Fail: got %x expected %x", enc, expected)
	}
}

func TestBlockDataEncodeAll(t *testing.T) {
	hash := common.NewHash([]byte{0})
	body := optional.CoreBody{0xa, 0xb, 0xc, 0xd}

	bd := &BlockData{
		Hash:          hash,
		Header:        optional.NewHeader(false, nil),
		Body:          optional.NewBody(true, body),
		Receipt:       optional.NewBytes(true, []byte("asdf")),
		MessageQueue:  optional.NewBytes(true, []byte("ghjkl")),
		Justification: optional.NewBytes(true, []byte("qwerty")),
	}

	expected, err := common.HexToBytes("0x00000000000000000000000000000000000000000000000000000000000000000001100a0b0c0d011061736466011467686a6b6c0118717765727479")
	if err != nil {
		t.Fatal(err)
	}

	enc, err := bd.Encode()
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(expected, enc) {
		t.Fatalf("Fail: got %x expected %x", enc, expected)
	}
}

func TestBlockDataDecodeHeader(t *testing.T) {
	hash := common.NewHash([]byte{0})
	testHash := common.NewHash([]byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 0xa, 0xb, 0xc, 0xd, 0xe, 0xf, 0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 0xa, 0xb, 0xc, 0xd, 0xe, 0xf})

	header := &optional.CoreHeader{
		ParentHash:     testHash,
		Number:         big.NewInt(1),
		StateRoot:      testHash,
		ExtrinsicsRoot: testHash,
		Digest:         [][]byte{{0xe, 0xf}},
	}

	expected := &BlockData{
		Hash:          hash,
		Header:        optional.NewHeader(true, header),
		Body:          optional.NewBody(false, nil),
		Receipt:       optional.NewBytes(false, nil),
		MessageQueue:  optional.NewBytes(false, nil),
		Justification: optional.NewBytes(false, nil),
	}

	enc, err := common.HexToBytes("0x000000000000000000000000000000000000000000000000000000000000000001000102030405060708090a0b0c0d0e0f000102030405060708090a0b0c0d0e0f04000102030405060708090a0b0c0d0e0f000102030405060708090a0b0c0d0e0f000102030405060708090a0b0c0d0e0f000102030405060708090a0b0c0d0e0f04080e0f00000000")

	//enc, err := common.HexToBytes("0x000000000000000000000000000000000000000000000000000000000000000001000102030405060708090a0b0c0d0e0f000102030405060708090a0b0c0d0e0f04000102030405060708090a0b0c0d0e0f000102030405060708090a0b0c0d0e0f000102030405060708090a0b0c0d0e0f000102030405060708090a0b0c0d0e0f0400000000")
	if err != nil {
		t.Fatal(err)
	}

	res := new(BlockData)
	r := &bytes.Buffer{}
	r.Write(enc)

	err = res.Decode(r)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(res, expected) {
		t.Fatalf("Fail: got %v expected %v", res, expected)
	}
}

func TestBlockDataDecodeBody(t *testing.T) {
	hash := common.NewHash([]byte{0})
	body := optional.CoreBody{0xa, 0xb, 0xc, 0xd}

	expected := &BlockData{
		Hash:          hash,
		Header:        optional.NewHeader(false, nil),
		Body:          optional.NewBody(true, body),
		Receipt:       optional.NewBytes(false, nil),
		MessageQueue:  optional.NewBytes(false, nil),
		Justification: optional.NewBytes(false, nil),
	}

	enc, err := common.HexToBytes("0x00000000000000000000000000000000000000000000000000000000000000000001100a0b0c0d000000")
	if err != nil {
		t.Fatal(err)
	}

	res := new(BlockData)
	r := &bytes.Buffer{}
	r.Write(enc)

	err = res.Decode(r)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(res, expected) {
		t.Fatalf("Fail: got %v expected %v", res, expected)
	}
}

func TestBlockDataDecodeAll(t *testing.T) {
	hash := common.NewHash([]byte{0})
	body := optional.CoreBody{0xa, 0xb, 0xc, 0xd}

	expected := &BlockData{
		Hash:          hash,
		Header:        optional.NewHeader(false, nil),
		Body:          optional.NewBody(true, body),
		Receipt:       optional.NewBytes(true, []byte("asdf")),
		MessageQueue:  optional.NewBytes(true, []byte("ghjkl")),
		Justification: optional.NewBytes(true, []byte("qwerty")),
	}

	enc, err := common.HexToBytes("0x00000000000000000000000000000000000000000000000000000000000000000001100a0b0c0d011061736466011467686a6b6c0118717765727479")
	if err != nil {
		t.Fatal(err)
	}

	res := new(BlockData)
	r := &bytes.Buffer{}
	r.Write(enc)

	err = res.Decode(r)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(res, expected) {
		t.Fatalf("Fail: got %v expected %v", res, expected)
	}
}

func TestBlockDataArrayEncodeAndDecode(t *testing.T) {
	hash := common.NewHash([]byte{0})
	testHash := common.NewHash([]byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 0xa, 0xb, 0xc, 0xd, 0xe, 0xf, 0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 0xa, 0xb, 0xc, 0xd, 0xe, 0xf})
	body := optional.CoreBody{0xa, 0xb, 0xc, 0xd}

	header := &optional.CoreHeader{
		ParentHash:     testHash,
		Number:         big.NewInt(1),
		StateRoot:      testHash,
		ExtrinsicsRoot: testHash,
		Digest:         [][]byte{},
	}

	expected := []*BlockData{{
		Hash:          hash,
		Header:        optional.NewHeader(true, header),
		Body:          optional.NewBody(false, nil),
		Receipt:       optional.NewBytes(false, nil),
		MessageQueue:  optional.NewBytes(false, nil),
		Justification: optional.NewBytes(false, nil),
	}, {
		Hash:          hash,
		Header:        optional.NewHeader(false, nil),
		Body:          optional.NewBody(true, body),
		Receipt:       optional.NewBytes(true, []byte("asdf")),
		MessageQueue:  optional.NewBytes(true, []byte("ghjkl")),
		Justification: optional.NewBytes(true, []byte("qwerty")),
	}, {
		Hash:          hash,
		Header:        optional.NewHeader(false, nil),
		Body:          optional.NewBody(true, body),
		Receipt:       optional.NewBytes(false, nil),
		MessageQueue:  optional.NewBytes(false, nil),
		Justification: optional.NewBytes(false, nil),
	}}

	enc, err := EncodeBlockDataArray(expected)
	if err != nil {
		t.Fatal(err)
	}

	r := &bytes.Buffer{}
	r.Write(enc)

	res, err := DecodeBlockDataArray(r)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(res[1], expected[1]) {
		t.Fatalf("Fail: got %v expected %v", res[1], expected[1])
	}
}

func TestDecodeBlock(t *testing.T) {
	// see https://github.com/paritytech/substrate/blob/master/test-utils/runtime/src/system.rs#L376
	data := []byte{69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 4, 39, 71, 171, 124, 13, 195, 139, 127, 42, 251, 168, 43, 213, 226, 214, 172, 239, 140, 49, 224, 152, 0, 246, 96, 183, 94, 200, 74, 112, 5, 9, 159, 3, 23, 10, 46, 117, 151, 183, 183, 227, 216, 76, 5, 57, 29, 19, 154, 98, 177, 87, 231, 135, 134, 216, 192, 130, 242, 157, 207, 76, 17, 19, 20, 0, 0}
	bh := NewEmptyBlock()
	err := bh.Decode(data)
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

func TestEncodeBlock(t *testing.T) {
	// see https://github.com/paritytech/substrate/blob/master/test-utils/runtime/src/system.rs#L376
	expected := []byte{69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69, 69,
		4, 39, 71, 171, 124, 13, 195, 139, 127, 42, 251, 168, 43, 213, 226, 214, 172, 239, 140, 49, 224, 152, 0,
		246, 96, 183, 94, 200, 74, 112, 5, 9, 159, 3, 23, 10, 46, 117, 151, 183, 183, 227, 216, 76, 5, 57, 29, 19,
		154, 98, 177, 87, 231, 135, 134, 216, 192, 130, 242, 157, 207, 76, 17, 19, 20, 0, 0}

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

	block := NewBlock(header, NewBody([]byte{}))
	enc, err := block.Encode()
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(enc, expected) {
		t.Fatalf("Fail: got %x expected %x", enc, expected)
	}
}
