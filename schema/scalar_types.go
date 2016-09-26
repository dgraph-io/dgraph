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

import "strconv"

// added suffix 'type' to names to distinguish from Go types 'int' and 'string'
var (
	// Int scalar type.
	intType = Scalar{
		Name: "int",
		Description: "The 'Int' scalar type represents non-fractional signed whole" +
			" numeric values. Int can represent values between -(2^31)" +
			" and 2^31 - 1.",
		ParseType: coerceInt,
	}
	// Float scalar type.
	floatType = Scalar{
		Name: "float",
		Description: "The 'Float' scalar type represents signed double-precision" +
			" fractional values	as specified by [IEEE 754]" +
			" (http://en.wikipedia.org/wiki/IEEE_floating_point).",
		ParseType: coerceFloat,
	}
	// String scalar type.
	stringType = Scalar{
		Name: "string",
		Description: "The 'String' scalar type represents textual data, represented" +
			" as UTF-8 character sequences. The String type is most often" +
			" used by GraphQL to represent free-form human-readable text.",
		ParseType: coerceString,
	}
	// Boolean scalar type.
	booleanType = Scalar{
		Name:        "bool",
		Description: "The 'Boolean' scalar type represents 'true' or 'false'.",
		ParseType:   coerceBool,
	}
	// ID scalar type.
	idType = Scalar{
		Name: "id",
		Description: "The 'ID' scalar type represents a unique identifier, often" +
			" used to refetch an object or as key for a cache. The ID type" +
			" appears in a JSON response as a String; however, it is not" +
			" intended to be human-readable. When expected as an input" +
			" type, any string (such as '4') or integer (such as '4')" +
			" input value will be accepted as an ID.",
		ParseType: coerceString,
	}
)

// coerceInt coerces the input value to int scalar type.
func coerceInt(input []byte) (interface{}, error) {
	val, err := strconv.ParseInt(string(input), 10, 32)
	if err != nil {
		return 0, err
	}
	return int32(val), nil
}

// coerceFloat converts different types to float scalar type
func coerceFloat(input []byte) (interface{}, error) {
	val, err := strconv.ParseFloat(string(input), 64)
	if err != nil {
		return 0.0, err
	}
	return val, nil
}

// coerceString converts input types to string scalar type
func coerceString(input []byte) (interface{}, error) {
	return string(input), nil
}

// coerceBool converts other object types to bool scalar type
func coerceBool(input []byte) (interface{}, error) {
	val, err := strconv.ParseBool(string(input))
	if err != nil {
		return false, err
	}
	return val, nil
}
