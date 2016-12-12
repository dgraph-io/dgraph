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

// Object represents all types in the schema definition.
type Object struct {
	Name   string
	Id     TypeID
	Fields map[string]string //field to type relationship
}

// Value is the interface that all scalar values need to implement.
type Value interface {
	encoding.TextMarshaler
	encoding.TextUnmarshaler
	encoding.BinaryMarshaler
	encoding.BinaryUnmarshaler
	json.Marshaler
	// Type returns the type of this value
	TypeID() TypeID
	fmt.Stringer
}

// IsScalar returns true if the object is of scalar type.
func (o Object) IsScalar() bool {
	if o.Id == ObjectID {
		return false
	}
	return true
}
