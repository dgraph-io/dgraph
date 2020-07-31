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

package optional

import (
	"fmt"
	"math/big"

	"github.com/ChainSafe/gossamer/lib/common"
)

const none = "None"

// Uint32 represents an optional uint32 type.
type Uint32 struct {
	exists bool
	value  uint32
}

// NewUint32 returns a new optional.Uint32
func NewUint32(exists bool, value uint32) *Uint32 {
	return &Uint32{
		exists: exists,
		value:  value,
	}
}

// Exists returns true if the value is Some, false if it is None.
func (x *Uint32) Exists() bool {
	return x.exists
}

// Value returns the uint32 value. It returns 0 if it is None.
func (x *Uint32) Value() uint32 {
	return x.value
}

// String returns the value as a string.
func (x *Uint32) String() string {
	if x == nil {
		return ""
	}
	if !x.exists {
		return none
	}
	return fmt.Sprintf("%d", x.value)
}

// Set sets the exists and value fields.
func (x *Uint32) Set(exists bool, value uint32) {
	x.exists = exists
	x.value = value
}

// Bytes represents an optional Bytes type.
type Bytes struct {
	exists bool
	value  []byte
}

// NewBytes returns a new optional.Bytes
func NewBytes(exists bool, value []byte) *Bytes {
	return &Bytes{
		exists: exists,
		value:  value,
	}
}

// Exists returns true if the value is Some, false if it is None.
func (x *Bytes) Exists() bool {
	return x.exists
}

// Value returns the []byte value. It returns nil if it is None.
func (x *Bytes) Value() []byte {
	return x.value
}

// String returns the value as a string.
func (x *Bytes) String() string {
	if !x.exists {
		return none
	}
	return fmt.Sprintf("%x", x.value)
}

// Set sets the exists and value fields.
func (x *Bytes) Set(exists bool, value []byte) {
	x.exists = exists
	x.value = value
}

// Hash represents an optional Hash type.
type Hash struct {
	exists bool
	value  common.Hash
}

// NewHash returns a new optional.Hash
func NewHash(exists bool, value common.Hash) *Hash {
	return &Hash{
		exists: exists,
		value:  value,
	}
}

// Exists returns true if the value is Some, false if it is None.
func (x *Hash) Exists() bool {
	if x == nil {
		return false
	}
	return x.exists
}

// Value returns Hash Value
func (x *Hash) Value() common.Hash {
	if x == nil {
		return common.Hash{}
	}
	return x.value
}

// String returns the value as a string.
func (x *Hash) String() string {
	if x == nil {
		return ""
	}

	if !x.exists {
		return none
	}
	return fmt.Sprintf("%x", x.value)
}

// Set sets the exists and value fields.
func (x *Hash) Set(exists bool, value common.Hash) {
	x.exists = exists
	x.value = value
}

// CoreHeader is a state block header
// This is copied from core/types since core/types imports this package, we cannot import core/types.
type CoreHeader struct {
	ParentHash     common.Hash `json:"parentHash"`
	Number         *big.Int    `json:"number"`
	StateRoot      common.Hash `json:"stateRoot"`
	ExtrinsicsRoot common.Hash `json:"extrinsicsRoot"`
	Digest         [][]byte    `json:"digest"`
}

// Header represents an optional header type
type Header struct {
	exists bool
	value  *CoreHeader
}

// NewHeader returns a new optional.Header
func NewHeader(exists bool, value *CoreHeader) *Header {
	return &Header{
		exists: exists,
		value:  value,
	}
}

// Exists returns true if the value is Some, false if it is None.
func (x *Header) Exists() bool {
	if x == nil {
		return false
	}
	return x.exists
}

// Value returns the value of the header. It returns nil if the header is None.
func (x *Header) Value() *CoreHeader {
	if x == nil {
		return nil
	}
	return x.value
}

// String returns the value as a string.
func (x *Header) String() string {
	if !x.exists {
		return none
	}
	return fmt.Sprintf("%v", x.value)
}

// Set sets the exists and value fields.
func (x *Header) Set(exists bool, value *CoreHeader) {
	x.exists = exists
	x.value = value
}

// CoreBody is the extrinsics inside a state block
type CoreBody []byte

// Body represents an optional types.Body.
// The fields need to be exported since it's JSON encoded by the state service.
// TODO: when we change the state service's encoding to SCALE, these fields should become unexported.
type Body struct {
	Exists bool
	Value  CoreBody
}

// NewBody returns a new optional.Body
func NewBody(exists bool, value CoreBody) *Body {
	return &Body{
		Exists: exists,
		Value:  value,
	}
}

// String returns the value as a string.
func (x *Body) String() string {
	if !x.Exists {
		return none
	}
	return fmt.Sprintf("%v", x.Value)
}

// Set sets the exists and value fields.
func (x *Body) Set(exists bool, value CoreBody) {
	x.Exists = exists
	x.Value = value
}
