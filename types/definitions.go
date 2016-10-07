/*
 * Copyright 2016 DGraph Labs, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * 		http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package types

import (
	"encoding"
	"encoding/json"
	"fmt"
)

// Type interface is the wrapper interface for all types
type Type interface {
	IsScalar() bool
}

// TypeID is the id used to identify a type.
type TypeID byte

// Unmarshaler returns the unmarshaler associated with the TypeID
func (t TypeID) Unmarshaler() Unmarshaler {
	return typeIDUnmarshalerMap[t]
}

// Scalar type defines concrete structure for scalar types to use.
// Almost all scalar types can also act as input types.
// Scalars (along with Enums) form leaf nodes of request or input values to arguements.
type Scalar struct {
	Name string // name of scalar type
	id   TypeID // The storage identifier for this type
}

// Object represents all object types in the schema definition.
type Object struct {
	Name   string
	Fields map[string]string //field to type relationship
}

// Value is the interface that all scalar values need to implement.
type Value interface {
	encoding.TextMarshaler
	encoding.BinaryMarshaler
	json.Marshaler
	// Type returns the type of this value
	Type() Scalar
}

// Unmarshaler type is for unmarshaling a Value from binary/text format.
type Unmarshaler interface {
	// FromBinary unmarshals the data from a binary format.
	FromBinary(data []byte) (Value, error)
	// FromText unmarshals the data from a text format.
	FromText(data []byte) (Value, error)
}

// String function to implement string interface
func (s Scalar) String() string {
	return fmt.Sprint(s.Name)
}

// ID function returns the storage identifier of this type
func (s Scalar) ID() TypeID {
	return s.id
}

// IsScalar returns true if the object is of scalar type.
func (s Scalar) IsScalar() bool {
	return true
}

// IsScalar returns true if the object is of scalar type.
func (o Object) IsScalar() bool {
	return false
}
