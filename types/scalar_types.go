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
	"encoding/binary"
	"encoding/json"
	"math"
	"strconv"
	"time"

	"github.com/dgraph-io/dgraph/x"
)

// added suffix 'type' to names to distinguish from Go types 'int' and 'string'
var (
	// Int scalar type.
	intType = Scalar{
		Name: "Int",
		Description: "The 'Int' scalar type represents non-fractional signed whole" +
			" numeric values. Int can represent values between -(2^31)" +
			" and 2^31 - 1.",
		Unmarshaler: int32Unmarsh,
	}
	// Float scalar type.
	floatType = Scalar{
		Name: "Float",
		Description: "The 'Float' scalar type represents signed double-precision" +
			" fractional values	as specified by [IEEE 754]" +
			" (http://en.wikipedia.org/wiki/IEEE_floating_point).",
		Unmarshaler: floatUnmarsh,
	}
	// String scalar type.
	stringType = Scalar{
		Name: "String",
		Description: "The 'String' scalar type represents textual data, represented" +
			" as UTF-8 character sequences. The String type is most often" +
			" used by GraphQL to represent free-form human-readable text.",
		Unmarshaler: stringUnmarsh,
	}
	// Boolean scalar type.
	booleanType = Scalar{
		Name:        "Boolean",
		Description: "The 'Boolean' scalar type represents 'true' or 'false'.",
		Unmarshaler: boolUnmarsh,
	}
	// ID scalar type.
	idType = Scalar{
		Name: "ID",
		Description: "The 'ID' scalar type represents a unique identifier, often" +
			" used to refetch an object or as key for a cache. The ID type" +
			" appears in a JSON response as a String; however, it is not" +
			" intended to be human-readable. When expected as an input" +
			" type, any string (such as '4') or integer (such as '4')" +
			" input value will be accepted as an ID.",
		Unmarshaler: stringUnmarsh,
	}
	// DateTime scalar type.
	dateTimeType = Scalar{
		Name: "DateTime",
		Description: "The 'DateTime' scalar type an instant in time with nanosecond" +
			" precision. Each DateTime is associated with a timezone.",
		Unmarshaler: timeUnmarsh,
	}
)

// stores a mapping between a string name of a type
var typeMap map[string]Type = map[string]Type{
	"int":      intType,
	"float":    floatType,
	"string":   stringType,
	"bool":     booleanType,
	"id":       idType,
	"datetime": dateTimeType,
}

// TypeForName returns the type corresponding to the given name.
// If name is not recognized, it returns nil.
func TypeForName(name string) Type {
	return typeMap[name]
}

// Int32Type is the scalar type for int32
type Int32Type int32

// MarshalBinary marshals to binary
func (v Int32Type) MarshalBinary() ([]byte, error) {
	var bs [4]byte
	binary.LittleEndian.PutUint32(bs[:], uint32(v))
	return bs[:], nil
}

// MarshalText marshals to text
func (v Int32Type) MarshalText() ([]byte, error) {
	s := strconv.FormatInt(int64(v), 10)
	return []byte(s), nil
}

// MarshalJSON marshals to json
func (v Int32Type) MarshalJSON() ([]byte, error) {
	return json.Marshal(int32(v))
}

type int32Unmarshaler struct{}

func (v int32Unmarshaler) UnmarshalBinary(data []byte) (TypeValue, error) {
	if len(data) < 4 {
		return nil, x.Errorf("Invalid data for int32 %v", data)
	}
	val := binary.LittleEndian.Uint32(data)
	return Int32Type(val), nil
}

func (v int32Unmarshaler) UnmarshalText(text []byte) (TypeValue, error) {
	val, err := strconv.ParseInt(string(text), 10, 32)
	if err != nil {
		return nil, err
	}
	return Int32Type(val), nil
}

var int32Unmarsh int32Unmarshaler

// FloatType is the scalar type for float64
type FloatType float64

// MarshalBinary marshals to binary
func (v FloatType) MarshalBinary() ([]byte, error) {
	var bs [8]byte
	u := math.Float64bits(float64(v))
	binary.LittleEndian.PutUint64(bs[:], u)
	return bs[:], nil
}

// MarshalText marshals to text
func (v FloatType) MarshalText() ([]byte, error) {
	s := strconv.FormatFloat(float64(v), 'E', -1, 64)
	return []byte(s), nil
}

// MarshalJSON marshals to json
func (v FloatType) MarshalJSON() ([]byte, error) {
	return json.Marshal(float64(v))
}

type floatUnmarshaler struct{}

func (v floatUnmarshaler) UnmarshalBinary(data []byte) (TypeValue, error) {
	if len(data) < 8 {
		return nil, x.Errorf("Invalid data for float %v", data)
	}
	u := binary.LittleEndian.Uint64(data)
	val := math.Float64frombits(u)
	return FloatType(val), nil
}

func (v floatUnmarshaler) UnmarshalText(text []byte) (TypeValue, error) {
	val, err := strconv.ParseFloat(string(text), 64)
	if err != nil {
		return nil, err
	}
	return FloatType(val), nil
}

var floatUnmarsh floatUnmarshaler

// StringType is the scalar type for string
type StringType string

// MarshalBinary marshals to binary
func (v StringType) MarshalBinary() ([]byte, error) {
	return []byte(v), nil
}

// MarshalText marshals to text
func (v StringType) MarshalText() ([]byte, error) {
	return v.MarshalBinary()
}

// MarshalJSON marshals to json
func (v StringType) MarshalJSON() ([]byte, error) {
	return json.Marshal(string(v))
}

type stringUnmarshaler struct{}

func (v stringUnmarshaler) UnmarshalBinary(data []byte) (TypeValue, error) {
	return StringType(data), nil
}

func (v stringUnmarshaler) UnmarshalText(text []byte) (TypeValue, error) {
	return v.UnmarshalBinary(text)
}

var stringUnmarsh stringUnmarshaler

// BoolType is the scalar type for bool
type BoolType bool

// MarshalBinary marshals to binary
func (v BoolType) MarshalBinary() ([]byte, error) {
	var bs [1]byte
	if v {
		bs[0] = 1
	} else {
		bs[0] = 0
	}
	return bs[:], nil
}

// MarshalText marshals to text
func (v BoolType) MarshalText() ([]byte, error) {
	s := strconv.FormatBool(bool(v))
	return []byte(s), nil
}

// MarshalJSON marshals to json
func (v BoolType) MarshalJSON() ([]byte, error) {
	return json.Marshal(bool(v))
}

type boolUnmarshaler struct{}

func (v boolUnmarshaler) UnmarshalBinary(data []byte) (TypeValue, error) {
	if data[0] == 0 {
		return BoolType(false), nil
	} else if data[0] == 1 {
		return BoolType(true), nil
	} else {
		return nil, x.Errorf("Invalid value for bool %v", data[0])
	}
}

func (v boolUnmarshaler) UnmarshalText(text []byte) (TypeValue, error) {
	val, err := strconv.ParseBool(string(text))
	if err != nil {
		return nil, err
	}
	return BoolType(val), nil
}

var boolUnmarsh boolUnmarshaler

type timeUnmarshaler struct{}

func (u timeUnmarshaler) UnmarshalBinary(data []byte) (TypeValue, error) {
	var v time.Time
	if err := v.UnmarshalBinary(data); err != nil {
		return nil, err
	}
	return v, nil
}

func (u timeUnmarshaler) UnmarshalText(text []byte) (TypeValue, error) {
	var v time.Time
	if err := v.UnmarshalText(text); err != nil {
		return nil, err
	}
	return v, nil
}

var timeUnmarsh timeUnmarshaler
