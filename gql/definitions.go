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

package gql

import (
	"encoding"
	"encoding/json"
	"fmt"
)

// Type interface is the wrapper interface for all types
type Type interface {
	IsScalar() bool
}

// Scalar type defines concrete structure for scalar types to use.
// Almost all scalar types can also act as input types.
// Scalars (along with Enums) form leaf nodes of request or input values to arguements.
type Scalar struct {
	Name        string // name of scalar type
	Description string // short description
	// to unmarshal the binary/text representation of the type.
	Unmarshaler TypeValueUnmarshaler
}

// This is the interface that all scalar type values need to able
// implement.
type TypeValue interface {
	encoding.TextMarshaler
	encoding.BinaryMarshaler
	json.Marshaler
}

type TypeValueUnmarshaler interface {
	UnmarshalBinary(data []byte) (TypeValue, error)
	UnmarshalText(data []byte) (TypeValue, error)
}

// String function to implement string interface
func (s Scalar) String() string {
	return fmt.Sprint(s.Name)
}

// IsScalar function to assert scalar type
func (s Scalar) IsScalar() bool {
	return true
}
