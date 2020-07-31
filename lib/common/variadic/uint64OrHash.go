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

package variadic

import (
	"encoding/binary"
	"errors"
	"io"

	"github.com/ChainSafe/gossamer/lib/common"
)

// Uint64OrHash represents an optional interface type (int,hash).
type Uint64OrHash struct {
	value interface{}
}

// NewUint64OrHash returns a new variadic.Uint64OrHash given an int, uint64, or Hash
func NewUint64OrHash(value interface{}) (*Uint64OrHash, error) {
	switch v := value.(type) {
	case int:
		return &Uint64OrHash{
			value: uint64(v),
		}, nil
	case uint64:
		return &Uint64OrHash{
			value: v,
		}, nil
	case common.Hash:
		return &Uint64OrHash{
			value: v,
		}, nil
	default:
		return nil, errors.New("value is not uint64 or common.Hash")
	}
}

// NewUint64OrHashFromBytes returns a new variadic.Uint64OrHash from an encoded variadic uint64 or hash
func NewUint64OrHashFromBytes(data []byte) *Uint64OrHash {
	firstByte := data[0]
	if firstByte == 0 {
		return &Uint64OrHash{
			value: common.NewHash(data[1:]),
		}
	} else if firstByte == 1 {
		num := data[1:]
		if len(num) < 8 {
			num = common.AppendZeroes(num, 8)
		}
		return &Uint64OrHash{
			value: binary.LittleEndian.Uint64(num),
		}
	} else {
		return nil
	}
}

// Value returns the interface value.
func (x *Uint64OrHash) Value() interface{} {
	if x == nil {
		return nil
	}
	return x.value
}

// Encode will encode a uint64 or hash into the SCALE spec
func (x *Uint64OrHash) Encode() ([]byte, error) {
	var encMsg []byte
	switch c := x.Value().(type) {
	case uint64:
		startingBlockByteArray := make([]byte, 8)
		binary.LittleEndian.PutUint64(startingBlockByteArray, c)

		encMsg = append(encMsg, append([]byte{1}, startingBlockByteArray...)...)
	case common.Hash:
		encMsg = append(encMsg, append([]byte{0}, c.ToBytes()...)...)
	}
	return encMsg, nil
}

// Decode will decode the Uint64OrHash into a hash or uint64
func (x *Uint64OrHash) Decode(r io.Reader) error {
	startingBlockType, err := common.ReadByte(r)
	if err != nil {
		return err
	}
	if startingBlockType == 0 {
		hash := make([]byte, 32)
		_, err = r.Read(hash)
		if err != nil {
			return err
		}
		x.value = common.NewHash(hash)
	} else {
		num := make([]byte, 8)
		_, err = r.Read(num)
		if err != nil {
			return err
		}
		x.value = binary.LittleEndian.Uint64(num)
	}
	return nil
}
