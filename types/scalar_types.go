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
const minInt int32 = -1<<31
const maxInt int32 = 1<<31 - 1

/**
 * coerceInt coerces the input value to appropriate type according to GraphQL specification.
 * TODO(akhil): handle error thrown from here.
 * Note:
 * 		-although, in most cases, input will be string/boot/int/float types but,
 * 			coercion has been done for all available types for demonstration.
 * 		-byte and rune types are already covered by uint8 and int32, respectively.
 * 		-complex numbers and pointer type (uintptr) are not covered here.
 * 		-if input value cannot be coerced, "nil" is retuned.
 */
func coerceInt(input interface{}) interface{} {
	// use a 'type switch' to find out the type of the input value
	switch v := input.(type) {
	case bool:
		if v {
			return 1
		}
		return 0
	case string:
		// TODO(akhil): Test if this works for all inputs. HINT: Atoi didn't work here for float input
		val, err := strconv.ParseFloat(v, 32)
		if err != nil {
			return nil
		}
		return coerceInt(val)
	// TODO(akhil): check if this works correctly, Golang tutorial mentioned it could be int64 on 64 bit systems
	case int:
		return v
	case int8:
		return int32(v)
	case int16:
		return int32(v)
	case int32:
		return v
	case int64:
		if int64(maxInt) < v || v < int64(minInt) {
			return nil
		}
		return int32(v)
	// TODO(akhil): check for potential issues here, same as 'int'
	case uint:
		return int32(v)
	case uint8:
		return int32(v)
	case uint16:
		return int32(v)
	case uint32:
		return int32(v)
	case uint64:
		if uint64(maxInt) < v {
			return nil
		}
		return int32(v)
	case float32:
		if float32(maxInt) < v || v < float32(minInt) {
			return nil
		}
		return int32(v)
	case float64:
		if float64(maxInt) < v || v < float64(minInt) {
			return nil
		}
		return int32(v)
	default:
		return nil
	}
}

// Int scalar type.
var Int = MakeScalarType(
	&ScalarConfig{
		Name:			"Int",
		Description:	"The 'Int' scalar type represents non-fractional signed whole" +
						" numeric values. Int can represent values between -(2^31)" +
						" and 2^31 - 1.",
		Serialize:		coerceInt,
		// parseValue:		coerceInt,
		// parseLiteral:	coerceInt
	},
)

func coerceFloat(input interface{}) interface{} {
	switch v := input.(type) {
	case bool:
		if v {
			return 1.0
		}
		return 0.0
	case string:
		val, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return nil
		}
		return coerceFloat(val)
	case int:
		return float64(v)
	case float32:
		return float64(v)
	case float64:
		return v
	default:
		return nil
	}
}

// Float scalar type.
var Float = MakeScalarType(
	&ScalarConfig{
		Name:			"Float",
		Description:	"The 'Float' scalar type represents signed double-precision" +
						" fractional values	as specified by [IEEE 754]" +
						" (http://en.wikipedia.org/wiki/IEEE_floating_point).",
		Serialize:		coerceFloat,
		// parseValue:		coerceFloat,
		// parseLiteral:	coerceFloat
	},
)



func coerceString(input interface{}) interface{} {
	switch v := input.(type) {
	case bool:
		if v {
			return "true"
		}
		return "false"
	default:
		return fmt.Sprintf("%v\n", v)
	}
}

// String scalar type.
var String = MakeScalarType(
	&ScalarConfig{
		Name:			"String",
		Description:	"The 'String' scalar type represents textual data, represented" +
						" as UTF-8 character sequences. The String type is most often" +
						" used by GraphQL to represent free-form human-readable text.",
		Serialize:		coerceString,
		// parseValue:		coerceString,
		// parseLiteral:	coerceString
	},
)



func coerceBool(input interface{}) interface{} {
	switch v := input.(type) {
	case bool:
		return v
	case string:
		if v == "false" {
			return false
		}
		return true
	case int:
		if v == 0 {
			return false
		}
		return true
	case float32:
		if v == 0.0 {
			return false
		}
		return true
	case float64:
		if v == 0.0 {
			return false
		}
		return true
	default:
		return nil
	}
}

// Boolean scalar type.
var Boolean = MakeScalarType(
	&ScalarConfig{
		Name:			"Boolean",
		Description:	"The 'Boolean' scalar type represents 'true' or 'false'.",
		Serialize:		coerceBool,
		// parseValue:		coerceBool,
		// parseLiteral:	coerceBool
	},
)

// ID scalar type.
var ID = MakeScalarType(
	&ScalarConfig{
		Name:			"ID",
		Description:	"The 'ID' scalar type represents a unique identifier, often" +
						" used to refetch an object or as key for a cache. The ID type" +
						" appears in a JSON response as a String; however, it is not" +
						" intended to be human-readable. When expected as an input" +
						" type, any string (such as '4') or integer (such as '4')" +
						" input value will be accepted as an ID.",
		Serialize:		coerceString,
		// parseValue:		coerceString,
		// parseLiteral:	coerceString
	},
)

// TODO(akhil): define two more types here: Time and URL.