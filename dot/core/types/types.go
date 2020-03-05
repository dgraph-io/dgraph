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
	"errors"
	"io"
	"math/big"

	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/ChainSafe/gossamer/lib/common/optional"
	"github.com/ChainSafe/gossamer/lib/scale"
)

// Extrinsic is a generic transaction whose format is verified in the runtime
type Extrinsic []byte

// NewExtrinsic creates a new Extrinsic given a byte slice
func NewExtrinsic(e []byte) Extrinsic {
	return Extrinsic(e)
}

// Block defines a state block
type Block struct {
	Header *Header
	Body   *Body
}

// NewBlock returns a new Block
func NewBlock(header *Header, body *Body) *Block {
	return &Block{
		Header: header,
		Body:   body,
	}
}

// NewEmptyBlock returns a new Block with an initialized but empty Header and Body
func NewEmptyBlock() *Block {
	return &Block{
		Header: new(Header),
		Body:   new(Body),
	}
}

// Encode returns the SCALE encoding of a block
func (b *Block) Encode() ([]byte, error) {
	enc, err := scale.Encode(b.Header)
	if err != nil {
		return nil, err
	}

	// fix since scale doesn't handle *types.Body types, but does handle []byte
	encBody, err := scale.Encode([]byte(*b.Body))
	if err != nil {
		return nil, err
	}

	return append(enc, encBody...), nil
}

// Decode decodes the SCALE encoded input into this block
func (b *Block) Decode(in []byte) error {
	_, err := scale.Decode(in, b)
	return err
}

// Header is a state block header
type Header struct {
	ParentHash     common.Hash `json:"parentHash"`
	Number         *big.Int    `json:"number"`
	StateRoot      common.Hash `json:"stateRoot"`
	ExtrinsicsRoot common.Hash `json:"extrinsicsRoot"`
	Digest         [][]byte    `json:"digest"`
	hash           common.Hash
}

// NewHeader creates a new block header and sets its hash field
func NewHeader(parentHash common.Hash, number *big.Int, stateRoot common.Hash, extrinsicsRoot common.Hash, digest [][]byte) (*Header, error) {
	if number == nil {
		// Hash() will panic if number is nil
		return nil, errors.New("cannot have nil block number")
	}

	bh := &Header{
		ParentHash:     parentHash,
		Number:         number,
		StateRoot:      stateRoot,
		ExtrinsicsRoot: extrinsicsRoot,
		Digest:         digest,
	}

	bh.Hash()
	return bh, nil
}

// DeepCopy returns a deep copy of the header to prevent side effects down the road
func (bh *Header) DeepCopy() *Header {
	//copy everything but pointers / array
	safeCopyHeader := *bh
	//copy number ptr
	if bh.Number != nil {
		safeCopyHeader.Number = new(big.Int).Set(bh.Number)
	}
	//copy digest byte array
	if len(bh.Digest) > 0 {
		safeCopyHeader.Digest = make([][]byte, len(bh.Digest))
		copy(safeCopyHeader.Digest, bh.Digest)
	}

	return &safeCopyHeader
}

// Hash returns the hash of the block header
// If the internal hash field is nil, it hashes the block and sets the hash field.
// If hashing the header errors, this will panic.
func (bh *Header) Hash() common.Hash {
	if bh.hash == [32]byte{} {
		enc, err := scale.Encode(bh)
		if err != nil {
			panic(err)
		}

		hash, err := common.Blake2bHash(enc)
		if err != nil {
			panic(err)
		}

		bh.hash = hash
	}

	return bh.hash
}

// Encode returns the SCALE encoding of a header
func (bh *Header) Encode() ([]byte, error) {
	return scale.Encode(bh)
}

// Decode decodes the SCALE encoded input into this header
func (bh *Header) Decode(in []byte) error {
	_, err := scale.Decode(in, bh)
	return err
}

// AsOptional returns the Header as an optional.Header
func (bh *Header) AsOptional() *optional.Header {
	return optional.NewHeader(true, &optional.CoreHeader{
		ParentHash:     bh.ParentHash,
		Number:         bh.Number,
		StateRoot:      bh.StateRoot,
		ExtrinsicsRoot: bh.ExtrinsicsRoot,
		Digest:         bh.Digest,
	})
}

// NewHeaderFromOptional returns a Header given an optional.Header. If the optional.Header is None, an error is returned.
func NewHeaderFromOptional(oh *optional.Header) (*Header, error) {
	if !oh.Exists() {
		return nil, errors.New("header is None")
	}

	h := oh.Value()

	if h.Number == nil {
		// Hash() will panic if number is nil
		return nil, errors.New("cannot have nil block number")
	}

	bh := &Header{
		ParentHash:     h.ParentHash,
		Number:         h.Number,
		StateRoot:      h.StateRoot,
		ExtrinsicsRoot: h.ExtrinsicsRoot,
		Digest:         h.Digest,
	}

	bh.Hash()
	return bh, nil
}

// Body is the encoded extrinsics inside a state block
type Body []byte

// NewBody returns a Body from a byte array
func NewBody(b []byte) *Body {
	body := Body(b)
	return &body
}

// NewBodyFromExtrinsics creates a block body given an array of extrinsics.
func NewBodyFromExtrinsics(exts []Extrinsic) (*Body, error) {
	enc, err := scale.Encode(ExtrinsicsArrayToBytesArray(exts))
	if err != nil {
		return nil, err
	}

	body := Body(enc)
	return &body, nil
}

// AsExtrinsics decodes the body into an array of extrinsics
func (b *Body) AsExtrinsics() ([]Extrinsic, error) {
	exts := [][]byte{}

	dec, err := scale.Decode(*b, exts)
	if err != nil {
		return nil, err
	}

	return BytesArrayToExtrinsics(dec.([][]byte)), nil
}

// NewBodyFromOptional returns a Body given an optional.Body. If the optional.Body is None, an error is returned.
func NewBodyFromOptional(ob *optional.Body) (*Body, error) {
	if !ob.Exists {
		return nil, errors.New("body is None")
	}

	b := ob.Value
	res := Body([]byte(b))
	return &res, nil
}

// AsOptional returns the Body as an optional.Body
func (b *Body) AsOptional() *optional.Body {
	ob := optional.CoreBody([]byte(*b))
	return optional.NewBody(true, ob)
}

// ExtrinsicsArrayToBytesArray converts an array of extrinsics into an array of byte arrays
func ExtrinsicsArrayToBytesArray(exts []Extrinsic) [][]byte {
	b := make([][]byte, len(exts))
	for i, ext := range exts {
		b[i] = []byte(ext)
	}
	return b
}

// BytesArrayToExtrinsics converts an array of byte arrays into an array of extrinsics
func BytesArrayToExtrinsics(b [][]byte) []Extrinsic {
	exts := make([]Extrinsic, len(b))
	for i, be := range b {
		exts[i] = Extrinsic(be)
	}
	return exts
}

// BlockData is stored within the BlockDB
type BlockData struct {
	Hash          common.Hash
	Header        *optional.Header
	Body          *optional.Body
	Receipt       *optional.Bytes
	MessageQueue  *optional.Bytes
	Justification *optional.Bytes
}

// Encode performs SCALE encoding of the BlockData
func (bd *BlockData) Encode() ([]byte, error) {
	enc := bd.Hash[:]

	if bd.Header.Exists() {
		venc, err := scale.Encode(bd.Header.Value())
		if err != nil {
			return nil, err
		}
		enc = append(enc, byte(1)) // Some
		enc = append(enc, venc...)
	} else {
		enc = append(enc, byte(0)) // None
	}

	if bd.Body.Exists {
		venc, err := scale.Encode([]byte(bd.Body.Value))
		if err != nil {
			return nil, err
		}
		enc = append(enc, byte(1)) // Some
		enc = append(enc, venc...)
	} else {
		enc = append(enc, byte(0)) // None
	}

	if bd.Receipt != nil && bd.Receipt.Exists() {
		venc, err := scale.Encode(bd.Receipt.Value())
		if err != nil {
			return nil, err
		}
		enc = append(enc, byte(1)) // Some
		enc = append(enc, venc...)
	} else {
		enc = append(enc, byte(0)) // None
	}

	if bd.MessageQueue != nil && bd.MessageQueue.Exists() {
		venc, err := scale.Encode(bd.MessageQueue.Value())
		if err != nil {
			return nil, err
		}
		enc = append(enc, byte(1)) // Some
		enc = append(enc, venc...)
	} else {
		enc = append(enc, byte(0)) // None
	}

	if bd.Justification != nil && bd.Justification.Exists() {
		venc, err := scale.Encode(bd.Justification.Value())
		if err != nil {
			return nil, err
		}
		enc = append(enc, byte(1)) // Some
		enc = append(enc, venc...)
	} else {
		enc = append(enc, byte(0)) // None
	}

	return enc, nil
}

// Decode decodes the SCALE encoded input to BlockData
func (bd *BlockData) Decode(r io.Reader) error {
	hash, err := common.ReadHash(r)
	if err != nil {
		return err
	}
	bd.Hash = hash

	bd.Header, err = decodeOptionalHeader(r)
	if err != nil {
		return err
	}

	bd.Body, err = decodeOptionalBody(r)
	if err != nil {
		return err
	}

	bd.Receipt, err = decodeOptionalBytes(r)
	if err != nil {
		return err
	}

	bd.MessageQueue, err = decodeOptionalBytes(r)
	if err != nil {
		return err
	}

	bd.Justification, err = decodeOptionalBytes(r)
	if err != nil {
		return err
	}

	return nil
}

// EncodeBlockDataArray encodes an array of BlockData using SCALE
func EncodeBlockDataArray(bds []*BlockData) ([]byte, error) {
	enc, err := scale.Encode(int32(len(bds)))
	if err != nil {
		return nil, err
	}

	for _, bd := range bds {
		benc, err := bd.Encode()
		if err != nil {
			return nil, err
		}
		enc = append(enc, benc...)
	}

	return enc, nil
}

// DecodeBlockDataArray decodes a SCALE encoded BlockData array
func DecodeBlockDataArray(r io.Reader) ([]*BlockData, error) {
	sd := scale.Decoder{Reader: r}

	l, err := sd.Decode(int32(0))
	if err != nil {
		return nil, err
	}

	length := int(l.(int32))
	bds := make([]*BlockData, length)

	for i := 0; i < length; i++ {
		bd := new(BlockData)
		err = bd.Decode(r)
		if err != nil {
			return bds, err
		}

		bds[i] = bd
	}

	return bds, err
}

// decodeOptionalHeader decodes a SCALE encoded optional Header into an *optional.Header
func decodeOptionalHeader(r io.Reader) (*optional.Header, error) {
	sd := scale.Decoder{Reader: r}

	exists, err := common.ReadByte(r)
	if err != nil {
		return nil, err
	}

	if exists == 1 {
		header := &Header{
			ParentHash:     common.Hash{},
			Number:         big.NewInt(0),
			StateRoot:      common.Hash{},
			ExtrinsicsRoot: common.Hash{},
			Digest:         [][]byte{},
		}
		_, err = sd.Decode(header)
		if err != nil {
			return nil, err
		}

		header.Hash()
		return header.AsOptional(), nil
	}

	return optional.NewHeader(false, nil), nil
}

// decodeOptionalBody decodes a SCALE encoded optional Body into an *optional.Body
func decodeOptionalBody(r io.Reader) (*optional.Body, error) {
	sd := scale.Decoder{Reader: r}

	exists, err := common.ReadByte(r)
	if err != nil {
		return nil, err
	}

	if exists == 1 {
		b, err := sd.Decode([]byte{})
		if err != nil {
			return nil, err
		}

		body := Body(b.([]byte))
		return body.AsOptional(), nil
	}

	return optional.NewBody(false, nil), nil
}

// decodeOptionalBytes decodes SCALE encoded optional bytes into an *optional.Bytes
func decodeOptionalBytes(r io.Reader) (*optional.Bytes, error) {
	sd := scale.Decoder{Reader: r}

	exists, err := common.ReadByte(r)
	if err != nil {
		return nil, err
	}

	if exists == 1 {
		b, err := sd.Decode([]byte{})
		if err != nil {
			return nil, err
		}

		return optional.NewBytes(true, b.([]byte)), nil
	}

	return optional.NewBytes(false, nil), nil
}
