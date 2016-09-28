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

// Note: These ids are stored in the posting lists to indicate the type
// of the data. The order *cannot* be changed without breaking existing
// data. When adding a new type *always* add to the end of this list.
// Never delete anything from this list even if it becomes unused.
const (
	stringID TypeID = iota
	int32ID
	floatID
	boolID
	dateTimeID
)

// added suffix 'type' to names to distinguish from Go types 'int' and 'string'
var (
	// Int scalar type.
	intType = Scalar{
		Name: "int",
		id:   int32ID,
		Description: "The 'Int' scalar type represents non-fractional signed whole" +
			" numeric values. Int can represent values between -(2^31)" +
			" and 2^31 - 1.",
		Unmarshaler: uInt32,
	}
	// Float scalar type.
	floatType = Scalar{
		Name: "float",
		id:   floatID,
		Description: "The 'Float' scalar type represents signed double-precision" +
			" fractional values	as specified by [IEEE 754]" +
			" (http://en.wikipedia.org/wiki/IEEE_floating_point).",
		Unmarshaler: uFloat,
	}
	// String scalar type.
	stringType = Scalar{
		Name: "string",
		id:   stringID,
		Description: "The 'String' scalar type represents textual data, represented" +
			" as UTF-8 character sequences. The String type is most often" +
			" used by GraphQL to represent free-form human-readable text.",
		Unmarshaler: uString,
	}
	// Boolean scalar type.
	booleanType = Scalar{
		Name:        "bool",
		id:          boolID,
		Description: "The 'Boolean' scalar type represents 'true' or 'false'.",
		Unmarshaler: uBool,
	}
	// ID scalar type.
	idType = Scalar{
		Name: "id",
		id:   stringID,
		Description: "The 'ID' scalar type represents a unique identifier, often" +
			" used to refetch an object or as key for a cache. The ID type" +
			" appears in a JSON response as a String; however, it is not" +
			" intended to be human-readable. When expected as an input" +
			" type, any string (such as '4') or integer (such as '4')" +
			" input value will be accepted as an ID.",
		Unmarshaler: uString,
	}
	// DateTime scalar type.
	dateTimeType = Scalar{
		Name: "datetime",
		id:   dateTimeID,
		Description: "The 'DateTime' scalar type an instant in time with nanosecond" +
			" precision. Each DateTime is associated with a timezone.",
		Unmarshaler: uTime,
	}
)

// stores a mapping between a string name of a type
var typeMap = map[string]Type{
	intType.Name:      intType,
	floatType.Name:    floatType,
	stringType.Name:   stringType,
	booleanType.Name:  booleanType,
	idType.Name:       idType,
	dateTimeType.Name: dateTimeType,
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

type unmarshalInt32 struct{}

func (u unmarshalInt32) FromBinary(data []byte) (TypeValue, error) {
	if len(data) < 4 {
		return nil, x.Errorf("Invalid data for int32 %v", data)
	}
	val := binary.LittleEndian.Uint32(data)
	return Int32Type(val), nil
}

func (u unmarshalInt32) FromText(text []byte) (TypeValue, error) {
	val, err := strconv.ParseInt(string(text), 10, 32)
	if err != nil {
		return nil, err
	}
	return Int32Type(val), nil
}

var uInt32 unmarshalInt32

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

type unmarshalFloat struct{}

func (u unmarshalFloat) FromBinary(data []byte) (TypeValue, error) {
	if len(data) < 8 {
		return nil, x.Errorf("Invalid data for float %v", data)
	}
	v := binary.LittleEndian.Uint64(data)
	val := math.Float64frombits(v)
	return FloatType(val), nil
}

func (u unmarshalFloat) FromText(text []byte) (TypeValue, error) {
	val, err := strconv.ParseFloat(string(text), 64)
	if err != nil {
		return nil, err
	}
	return FloatType(val), nil
}

var uFloat unmarshalFloat

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

type unmarshalString struct{}

func (v unmarshalString) FromBinary(data []byte) (TypeValue, error) {
	return StringType(data), nil
}

func (v unmarshalString) FromText(text []byte) (TypeValue, error) {
	return v.FromBinary(text)
}

var uString unmarshalString

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

type unmarshalBool struct{}

func (u unmarshalBool) FromBinary(data []byte) (TypeValue, error) {
	if data[0] == 0 {
		return BoolType(false), nil
	} else if data[0] == 1 {
		return BoolType(true), nil
	} else {
		return nil, x.Errorf("Invalid value for bool %v", data[0])
	}
}

func (u unmarshalBool) FromText(text []byte) (TypeValue, error) {
	val, err := strconv.ParseBool(string(text))
	if err != nil {
		return nil, err
	}
	return BoolType(val), nil
}

var uBool unmarshalBool

type unmarshalTime struct{}

func (u unmarshalTime) FromBinary(data []byte) (TypeValue, error) {
	var v time.Time
	if err := v.UnmarshalBinary(data); err != nil {
		return nil, err
	}
	return v, nil
}

func (u unmarshalTime) FromText(text []byte) (TypeValue, error) {
	var v time.Time
	if err := v.UnmarshalText(text); err != nil {
		return nil, err
	}
	return v, nil
}

var uTime unmarshalTime

func (u unmarshalInt32) fromFloat(f float64) (TypeValue, error) {
	if f > math.MaxInt32 || f < math.MinInt32 || math.IsNaN(f) {
		return nil, x.Errorf("Float out of int32 range")
	}
	return Int32Type(f), nil
}

func (u unmarshalInt32) fromBool(b bool) (TypeValue, error) {
	if b {
		return Int32Type(1), nil
	}
	return Int32Type(0), nil
}

func (u unmarshalInt32) fromTime(t time.Time) (TypeValue, error) {
	// Represent the unix timestamp as a 32bit int.
	secs := t.Unix()
	if secs > math.MaxInt32 || secs < math.MinInt32 {
		return nil, x.Errorf("Time out of int32 range")
	}
	return Int32Type(secs), nil
}

func (u unmarshalFloat) fromInt(i int32) (TypeValue, error) {
	return FloatType(i), nil
}

func (u unmarshalFloat) fromBool(b bool) (TypeValue, error) {
	if b {
		return FloatType(1), nil
	}
	return FloatType(0), nil
}

const (
	nanoSecondsInSec = 1000000000
)

func (u unmarshalFloat) fromTime(t time.Time) (TypeValue, error) {
	// Represent the unix timestamp as a float (with fractional seconds)
	secs := float64(t.Unix())
	nano := float64(t.Nanosecond())
	val := secs + nano/nanoSecondsInSec
	return FloatType(val), nil
}

func (u unmarshalBool) fromInt(i int32) (TypeValue, error) {
	return BoolType(i != 0), nil
}

func (u unmarshalBool) fromFloat(f float64) (TypeValue, error) {
	return BoolType(f != 0), nil
}

func (u unmarshalTime) fromInt(i int32) (TypeValue, error) {
	return time.Unix(int64(i), 0).UTC(), nil
}

func (u unmarshalTime) fromFloat(f float64) (TypeValue, error) {
	secs := int64(f)
	fracSecs := f - float64(secs)
	nsecs := int64(fracSecs * nanoSecondsInSec)
	return time.Unix(secs, nsecs).UTC(), nil
}
