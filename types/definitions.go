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
	"fmt"
)

// Type interface is the wrapper interface for all types
type Type interface {
	// TODO(akhil): discuss and add a method to implement this interface (didn't want to add dummy method)
}

// Scalar type defines concrete structure for scalar types to use.
// Almost all scalar types can also act as input types.
// Scalars (along with Enums) form leaf nodes of request or input values to arguements.
type Scalar struct {
	Name        string // name of scalar type
	Description string // short description
	ParseType   ParseTypeFunc
}

// ParseTypeFunc is a function that parses and does coercion for Scalar types.
type ParseTypeFunc func(input []byte) (interface{}, error)

// String function to implement string interface
func (s *Scalar) String() string {
	return fmt.Sprintf("ScalarTypeName is:%v\n", s.Name)
}

// Object type defines skeleton for basic objects.
// They form the basis for most object in this system.
// Object has a name and a set of fields.
type Object struct {
	Name        string
	Description string
	Fields      FieldMap
}

// FieldMap maps field names to their corresponding types.
type FieldMap map[string]Type

// String function to implement string interface
func (o *Object) String() string {
	return fmt.Sprintf("ObjectTypeName is:%v\n", o.Name)
}

// List type defines a wrapper for other grapql object types.
// Mostly used while defining object fields
type List struct {
	HasType Type
}
