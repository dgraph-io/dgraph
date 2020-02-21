package babe

import (
	"bytes"
	"encoding/binary"
	"errors"
	"math/big"

	scale "github.com/ChainSafe/gossamer/codec"
)

//nolint
var (
	Timstap0 = []byte("timstap0")
	Babeslot = []byte("babeslot")
)

// InherentsData contains a mapping of inherent keys to values
// keys must be 8 bytes, values are a variable-length byte array
type InherentsData struct {
	data map[[8]byte]([]byte)
}

// NewInherentsData returns InherentsData
func NewInherentsData() *InherentsData {
	return &InherentsData{
		data: make(map[[8]byte]([]byte)),
	}
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
