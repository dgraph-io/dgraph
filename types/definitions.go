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

// TODO(akhil): make underlying interfaces for stricter definitions of types.


/**
 * GraphQLScalar type defines concrete structure for scalar types to use.
 * Almost all scalar types can also act as input types.
 * Scalars (along with Enums) form leaf nodes of request or input values to arguements. 
 */
type GraphQLScalar struct {
	Name 		string 		// name of scalar type
	Description 	string 		// short description, could be used for documentation in GraphiQL
	Config 		ScalarConfig 	// config struct to construct a custom scalar
	Err		error 		// error object
}

// SerializeFunc is a function type that serializes GraphQL Scalar types.
type SerializeFunc func(input interface{}) interface{}


// ParseValueFunc is a function type that parses the value of GraphQL Scalar types.
type ParseValueFunc func(input interface{}) interface{}
																																																			

// ParseLiteralFunc is a function type that parses literal value of GraphQL Scalar types.
type ParseLiteralFunc func(input interface{}) interface{}


// ScalarConfig to pass config params to scalar type constructor function.
type ScalarConfig struct {
	Name			string				
	Description		string				
	Serialize		SerializeFunc		
	// ParseValue		ParseValueFunc
	// ParseLiteral	ParseLiteralFunc
}

/**
 * MakeScalarType declares a custom scalar type using the input configuration.
 * Custom scalar could directly be defined as well, but this function gives us flexibility
 * to do validations and catch errors. 
 * Basic scalar types supported by GraphQL are:
 * 		- Int
 * 		- Float
 * 		- String
 * 		- Boolean
 * 		- ID 
 */
func MakeScalarType(sc *ScalarConfig) (*GraphQLScalar) {
	scalarType := &GraphQLScalar{}

	// check if all essential config is present.
	if sc.Name != "" && validName(sc.Name){
		scalarType.Name = sc.Name
	} else {
		scalarType.Err = makeTypeError("Type must be named.")
		return scalarType
	}

	scalarType.Description = sc.Description

	if sc.Serialize == nil {
		scalarType.Err = makeTypeError(
				fmt.Sprintf("%v must provide 'serialize' function. If this custom Scalar " +
					"is also used as an input type, ensure 'parseValue' and " +
					"'parseLiteral' functions are also provided.", scalarType),
			)
	}
/*
	// TODO(akhil): uncomment when implementing input types.
	// throw error if any one of parseValue or parseLiteral functions are not provided.
	// (in the case that this scalar type is an input type)
	if (sc.parseValue != nil && sc.parseLiteral == nil) || 
		 (sc.parseValue == nil && sc.parseLiteral != nil) {
		 	scalarType.err = makeTypeError(
		 			fmt.Sprintf("%v must provide none (non-input type) or both (input type) 
		 				'parseValue' and 'parseLiteral' functions.",scalarType)
		 		)
	}
*/
	scalarType.Config = *sc
	return scalarType
}



func (s *GraphQLScalar) String() string {
	return s.Name
}

func (s *GraphQLScalar) Error() error {
	return s.Err
}

/**
 * GraphQLObject type defines skeleton for basic graphql objects.
 * They form the basis for most object in this system.
 * Object has a name and a set of fields.
 */
type GraphQLObject struct {
	// TODO(akhil): complete this impl.
	// TODO(akhil): find out if not exporting struct fields causes issues (same for GraphQLScalar).
	Name			string
	Desc			string
	Fields			FieldMap
}

// FieldMap maps field names to their corresponding types.
type FieldMap map[string]*Field

// Field declares the details for a field used for type inference, validation and execution.
type Field struct {
	Type 		interface{}
	Resolve		ResolveFunc
}

/**
 * ResolveFunc fetches the result data for the specified field.
 */
type ResolveFunc func (rp ResolveParams) interface{}

/**
 * ResolveParams is the set of input params available to fetch query result from backend.
 * Could also pass current context (like user info) and source info (about root, field) here.
 */
type ResolveParams struct {
	InputVal string
}


// TODO(akhil): GraphQLInterface type implementation.

// TODO(akhil): GraphQLUnion type implementation.

// TODO(akhil): GraphQLEnum type implementation.

// TODO(akhil): GraphQLList type implementation.

// TODO(akhil): GraphQLNonNull type implementation.

// TODO(akhil): GraphQLInputObjects type implementation.

/**
 * validName matches valid name strings.
 */
func validName(name string) bool {
	// TODO(akhil): impl using regexp.
	return true
}