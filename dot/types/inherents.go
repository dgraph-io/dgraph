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
	"encoding/binary"
	"errors"
	"fmt"
	"math/big"

	"github.com/ChainSafe/gossamer/lib/scale"
)

//nolint
var (
	Timstap0 = []byte("timstap0")
	Babeslot = []byte("babeslot")
	Finalnum = []byte("finalnum")
	Uncles00 = []byte("uncles00")
)

// InherentsData contains a mapping of inherent keys to values
// keys must be 8 bytes, values are a scale-encoded byte array
type InherentsData struct {
	data map[[8]byte]([]byte)
}

// NewInherentsData returns InherentsData
func NewInherentsData() *InherentsData {
	return &InherentsData{
		data: make(map[[8]byte]([]byte)),
	}
}

func (d *InherentsData) String() string {
	str := ""
	for k, v := range d.data {
		str = str + fmt.Sprintf("key=%v\tvalue=%v\n", k, v)
	}
	return str
}

// SetInt64Inherent set the Int64 scale.Encode for a given data
func (d *InherentsData) SetInt64Inherent(key []byte, data uint64) error {
	if len(key) != 8 {
		return errors.New("inherent key must be 8 bytes")
	}

	val := make([]byte, 8)
	binary.LittleEndian.PutUint64(val, data)

	venc, err := scale.Encode(val)
	if err != nil {
		return err
	}

	kb := [8]byte{}
	copy(kb[:], key)

	d.data[kb] = venc
	return nil
}

// SetBigIntInherent set as a big.Int (compact int) inherent
func (d *InherentsData) SetBigIntInherent(key []byte, data *big.Int) error {
	if len(key) != 8 {
		return errors.New("inherent key must be 8 bytes")
	}

	venc, err := scale.Encode(data)
	if err != nil {
		return err
	}

	lenc, err := scale.Encode(big.NewInt(int64(len(venc))))
	if err != nil {
		return err
	}

	kb := [8]byte{}
	copy(kb[:], key)

	d.data[kb] = append(lenc, venc...)
	return nil
}

// Encode will encode a given []byte using scale.Encode
func (d *InherentsData) Encode() ([]byte, error) {
	length := big.NewInt(int64(len(d.data)))

	buffer := bytes.Buffer{}
	se := scale.Encoder{Writer: &buffer}

	_, err := se.Encode(length)
	if err != nil {
		return nil, err
	}

	for k, v := range d.data {
		_, err = buffer.Write(k[:])
		if err != nil {
			return nil, err
		}
		_, err = buffer.Write(v)
		if err != nil {
			return nil, err
		}
	}
	return buffer.Bytes(), nil
}
