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

package schema

import "fmt"

// Type interface is the wrapper interface for all types
type Type interface {
	Type() Type
	IsScalar() bool
}

// Scalar type defines concrete structure for scalar types to use.
// Almost all scalar types can also act as input types.
// Scalars (along with Enums) form leaf nodes of request or input values to arguements.
type Scalar struct {
	Name        string // name of scalar type
	Description string // short description
	ParseType   ParseTypeFunc
}

type Object struct {
	Name   string
	Fields map[string]string //field to type relationship
}

// ParseTypeFunc is a function that parses and does coercion for Scalar types.
type ParseTypeFunc func(input []byte) (interface{}, error)

// String function to implement string interface
func (s Scalar) String() string {
	return fmt.Sprint(s.Name)
}

func (s Scalar) Type() Type {
	typ, _ := getScalar(s.Name)
	return typ
}

func (o Object) Type() Type {
	return o
}

func (s Scalar) IsScalar() bool {
	return true
}

func (o Object) IsScalar() bool {
	return false
}
