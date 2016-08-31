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
	"strconv"
)

// As per the GraphQL Spec, Integers are only treated as valid when a valid.
// 32-bit signed integer, providing the broadest support across platforms.
const minInt int32 = -1 << 31
const maxInt int32 = 1<<31 - 1

var (
	Int     GraphQLScalar
	Float   GraphQLScalar
	String  GraphQLScalar
	Boolean GraphQLScalar
	ID      GraphQLScalar
)

// coerceInt coerces the input value to appropriate type according to GraphQL specification.
// Note:
// -although, in most cases, input will be string/boot/int/float types but,
// coercion has been done for all available types for demonstration.
// -byte and rune types are already covered by uint8 and int32, respectively.
// -complex numbers and pointer type (uintptr) are not covered here.
// -if input value cannot be coerced, "nil" is retuned.
func coerceInt(input interface{}) (interface{}, error) {
	// use a 'type switch' to find out the type of the input value
	switch v := input.(type) {
	case bool:
		if v {
			return 1, nil
		}
		return 0, nil
	case string:
		val, err := strconv.ParseFloat(v, 32)
		if err != nil {
			return nil, err
		}
		return coerceInt(val)
	// TODO(akhil): Golang tutorial mentioned it could be int64 on 64 bit systems, verify
	case int:
		return v, nil
	case int8:
		return int32(v), nil
	case int16:
		return int32(v), nil
	case int32:
		return v, nil
	case int64:
		if int64(maxInt) < v || v < int64(minInt) {
			return nil, fmt.Errorf("Value:%v out of int32 range for conversion.", v)
		}
		return int32(v), nil
	// TODO(akhil): check for potential issues here, same as 'int'
	case uint:
		return int32(v), nil
	case uint8:
		return int32(v), nil
	case uint16:
		return int32(v), nil
	case uint32:
		return int32(v), nil
	case uint64:
		if uint64(maxInt) < v {
			return nil, fmt.Errorf("Value:%v out of int32 range for conversion.", v)
		}
		return int32(v), nil
	case float32:
		if float32(maxInt) < v || v < float32(minInt) {
			return nil, fmt.Errorf("Value:%v out of int32 range for conversion.", v)
		}
		return int32(v), nil
	case float64:
		if float64(maxInt) < v || v < float64(minInt) {
			return nil, fmt.Errorf("Value:%v out of int32 range for conversion.", v)
		}
		return int32(v), nil
	default:
		return nil, fmt.Errorf("Value:%v cannot be coerced into an Int.", v)
	}
}

// coerceFloat converts different types to float object type
func coerceFloat(input interface{}) (interface{}, error) {
	switch v := input.(type) {
	case bool:
		if v {
			return 1.0, nil
		}
		return 0.0, nil
	case string:
		val, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return nil, err
		}
		return coerceFloat(val)
	case int:
		return float64(v), nil
	case float32:
		return float64(v), nil
	case float64:
		return v, nil
	default:
		return nil, fmt.Errorf("Value:%v cannot be coerced into a Float.", v)
	}
}

// coerceString converts objects
func coerceString(input interface{}) (interface{}, error) {
	switch v := input.(type) {
	case bool:
		if v {
			return "true", nil
		}
		return "false", nil
	default:
		return fmt.Sprintf("%v", v), nil
	}
}

// coerceBool converts other object types to bool scalar type
func coerceBool(input interface{}) (interface{}, error) {
	switch v := input.(type) {
	case bool:
		return v, nil
	case string:
		if v == "false" || v == "0" {
			return false, nil
		}
		return true, nil
	case int:
		if v == 0 {
			return false, nil
		}
		return true, nil
	case float32:
		if v == 0.0 {
			return false, nil
		}
		return true, nil
	case float64:
		if v == 0.0 {
			return false, nil
		}
		return true, nil
	default:
		return nil, fmt.Errorf("Value:%v cannot be coerced into a Bool.", v)
	}
}

// LoadScalarTypes defines and initializes all scalar types in system and checks for errors
func LoadScalarTypes() error {
	var combinedError error
	var err error

	// Int scalar type.
	Int, err = MakeScalarType(
		&ScalarConfig{
			Name: "Int",
			Description: "The 'Int' scalar type represents non-fractional signed whole" +
				" numeric values. Int can represent values between -(2^31)" +
				" and 2^31 - 1.",
			ParseType: coerceInt,
		},
	)
	if err != nil {
		combinedError = err
	}

	// Float scalar type.
	Float, err = MakeScalarType(
		&ScalarConfig{
			Name: "Float",
			Description: "The 'Float' scalar type represents signed double-precision" +
				" fractional values	as specified by [IEEE 754]" +
				" (http://en.wikipedia.org/wiki/IEEE_floating_point).",
			ParseType: coerceFloat,
		},
	)
	if err != nil {
		combinedError = joinError(combinedError, err)
	}

	// String scalar type.
	String, err = MakeScalarType(
		&ScalarConfig{
			Name: "String",
			Description: "The 'String' scalar type represents textual data, represented" +
				" as UTF-8 character sequences. The String type is most often" +
				" used by GraphQL to represent free-form human-readable text.",
			ParseType: coerceString,
		},
	)
	if err != nil {
		combinedError = joinError(combinedError, err)
	}

	// Boolean scalar type.
	Boolean, err = MakeScalarType(
		&ScalarConfig{
			Name:        "Boolean",
			Description: "The 'Boolean' scalar type represents 'true' or 'false'.",
			ParseType:   coerceBool,
		},
	)
	if err != nil {
		combinedError = joinError(combinedError, err)
	}

	// ID scalar type.
	ID, err = MakeScalarType(
		&ScalarConfig{
			Name: "ID",
			Description: "The 'ID' scalar type represents a unique identifier, often" +
				" used to refetch an object or as key for a cache. The ID type" +
				" appears in a JSON response as a String; however, it is not" +
				" intended to be human-readable. When expected as an input" +
				" type, any string (such as '4') or integer (such as '4')" +
				" input value will be accepted as an ID.",
			ParseType: coerceString,
		},
	)
	if err != nil {
		combinedError = joinError(combinedError, err)
	}
	return combinedError
}

// TODO(akhil): define two more types here: Time and URL.
