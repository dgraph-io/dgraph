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

	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/ChainSafe/gossamer/lib/common/optional"
	"github.com/ChainSafe/gossamer/lib/scale"
)

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

// NewBodyFromExtrinsicStrings creates a block body given an array of hex-encoded 0x-prefixed strings.
func NewBodyFromExtrinsicStrings(ss []string) (*Body, error) {
	exts := [][]byte{}
	for _, s := range ss {
		b, err := common.HexToBytes(s)
		if err == common.ErrNoPrefix {
			b = []byte(s)
		} else if err != nil {
			return nil, err
		}
		exts = append(exts, b)
	}

	enc, err := scale.Encode(exts)
	if err != nil {
		return nil, err
	}

	body := Body(enc)
	return &body, nil
}

// AsExtrinsics decodes the body into an array of extrinsics
func (b *Body) AsExtrinsics() ([]Extrinsic, error) {
	exts := [][]byte{}

	if len(*b) == 0 {
		return []Extrinsic{}, nil
	}

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
