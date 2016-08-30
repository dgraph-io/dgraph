/*
 * Copyright 2015 DGraph Labs, Inc.
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

// GraphQLScalar type defines concrete structure for scalar types to use.
// Almost all scalar types can also act as input types.
// Scalars (along with Enums) form leaf nodes of request or input values to arguements.
// TODO(akhil): make more underlying interfaces for stricter definitions of types.
type GraphQLScalar struct {
	Name        string       // name of scalar type
	Description string       // short description, could be used for documentation in GraphiQL
	Config      ScalarConfig // config struct to construct a custom scalar
}

// ScalarConfig to pass config params to scalar type constructor function.
type ScalarConfig struct {
	Name        string
	Description string
	ParseType   ParseTypeFunc
}

// ParseTypeFunc is a function type that parses and does coersion for GraphQL Scalar types.
type ParseTypeFunc func(input interface{}) interface{}

// MakeScalarType declares a custom scalar type using the input configuration.
// Custom scalar could directly be defined as well, but this function gives us flexibility
// to do validations and catch errors.
// Basic scalar types supported by GraphQL are:
// - Int
// - Float
// - String
// - Boolean
// - ID
func MakeScalarType(sc *ScalarConfig) (GraphQLScalar, error) {
	scalarType := GraphQLScalar{}

	// check if all essential config is present.
	if sc.Name != "" && validName(sc.Name) {
		scalarType.Name = sc.Name
	} else {
		return scalarType, fmt.Errorf("Type must be named.")
	}

	scalarType.Description = sc.Description

	if sc.ParseType == nil {
		return scalarType, fmt.Errorf("'%v' type must a provide 'ParseType' function"+
			" which will be used for validation and type coercion if required. ", scalarType.Name)
	}
	scalarType.Config = *sc
	return scalarType, nil
}

// String function to implement string interface
func (s *GraphQLScalar) String() string {
	return fmt.Sprintf("ScalarTypeName is:%v\n", s.Name)
}

// GraphQLObject type defines skeleton for basic graphql objects.
// They form the basis for most object in this system.
// Object has a name and a set of fields.
type GraphQLObject struct {
	// TODO(akhil): complete this impl.
	Name   string
	Desc   string
	Fields FieldMap
}

// FieldMap maps field names to their corresponding types.
type FieldMap map[string]*Field

// Field declares the details for a field used for type inference, validation and execution.
// Type field is an interface to account for the fact that fields can be of any object type
// Could make it stricter with a wrapper interface
type Field struct {
	Type    interface{}
	Resolve ResolveFunc
}

// ResolveFunc fetches the result data for the specified field.
type ResolveFunc func(rp ResolveParams) interface{}

// ResolveParams is the set of input params available to fetch query result from backend.
// Could also pass current context (like user info) and source info (about root, field) here.
type ResolveParams struct {
	InputVal string
}

// GraphQLInterface type defines skeleton for basic graphql objects.
// They form the basis for most object in this system.
// Object has a name and a set of fields.
type GraphQLInterface struct {
	// TODO(akhil): complete this impl.
	Name   string
	Desc   string
	Fields FieldMap
}

// TODO(akhil): GraphQLUnion type implementation.

// TODO(akhil): GraphQLEnum type implementation.

// TODO(akhil): GraphQLList type implementation.

// TODO(akhil): GraphQLNonNull type implementation.

// TODO(akhil): GraphQLInputObjects type implementation.

// validName matches valid name strings.
func validName(name string) bool {
	// TODO(akhil): impl using regexp.
	return true
}
