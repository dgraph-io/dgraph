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
	"strconv"
)

// As per the GraphQL Spec, Integers as 32-bit signed integer are treated as valid
// That provides the broadest support across platforms.
const minInt int32 = -1 << 31
const maxInt int32 = 1<<31 - 1

var (
	// Int scalar type declaration
	Int GraphQLScalar
	// Float scalar type declaration
	Float GraphQLScalar
	// String scalar type declaration
	String GraphQLScalar
	// Boolean scalar type declaration
	Boolean GraphQLScalar
	// ID scalar type declaration
	ID GraphQLScalar
)

// coerceInt coerces the input value to appropriate type according to GraphQL specification.
// NOTE:
// - Although, currently, input is of string type, later it will change to []byte
func coerceInt(input string) (interface{}, error) {
	val, err := strconv.ParseFloat(input, 32)
	if err != nil {
		return 0, err
	}
	return int32(val), nil
}

// coerceFloat converts different types to float object type
func coerceFloat(input string) (interface{}, error) {
	val, err := strconv.ParseFloat(input, 64)
	if err != nil {
		return 0.0, err
	}
	return val, nil
}

// coerceString converts objects
// ideally, not needed now but kept for two reasons
//  - to maintain consistency, we just call parentType.ParseType to invoke respective coerce funtions
//  - will be required later when we do direct type inference and coerse []byte
func coerceString(input string) (interface{}, error) {
	return input, nil
}

// coerceBool converts other object types to bool scalar type
func coerceBool(input string) (interface{}, error) {
	if input == "false" || input == "0" {
		return false, nil
	}
	return true, nil
}

// loadScalarTypes defines and initializes all scalar types in system and checks for errors
func loadScalarTypes() {

	// Int scalar type.
	Int = GraphQLScalar{
		Name: "Int",
		Description: "The 'Int' scalar type represents non-fractional signed whole" +
			" numeric values. Int can represent values between -(2^31)" +
			" and 2^31 - 1.",
		ParseType: coerceInt,
	}

	// Float scalar type.
	Float = GraphQLScalar{
		Name: "Float",
		Description: "The 'Float' scalar type represents signed double-precision" +
			" fractional values	as specified by [IEEE 754]" +
			" (http://en.wikipedia.org/wiki/IEEE_floating_point).",
		ParseType: coerceFloat,
	}

	// String scalar type.
	String = GraphQLScalar{
		Name: "String",
		Description: "The 'String' scalar type represents textual data, represented" +
			" as UTF-8 character sequences. The String type is most often" +
			" used by GraphQL to represent free-form human-readable text.",
		ParseType: coerceString,
	}

	// Boolean scalar type.
	Boolean = GraphQLScalar{
		Name:        "Boolean",
		Description: "The 'Boolean' scalar type represents 'true' or 'false'.",
		ParseType:   coerceBool,
	}

	// ID scalar type.
	ID = GraphQLScalar{
		Name: "ID",
		Description: "The 'ID' scalar type represents a unique identifier, often" +
			" used to refetch an object or as key for a cache. The ID type" +
			" appears in a JSON response as a String; however, it is not" +
			" intended to be human-readable. When expected as an input" +
			" type, any string (such as '4') or integer (such as '4')" +
			" input value will be accepted as an ID.",
		ParseType: coerceString,
	}
}
