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
	// Int32Type scalar.
	Int32Type = Scalar{
		Name: "int",
		id:   int32ID,
		Description: "The 'Int' scalar type represents non-fractional signed whole" +
			" numeric values. Int can represent values between -(2^31)" +
			" and 2^31 - 1.",
		Unmarshaler: uInt32,
	}
	// FloatType scalar.
	FloatType = Scalar{
		Name: "float",
		id:   floatID,
		Description: "The 'Float' scalar type represents signed double-precision" +
			" fractional values	as specified by [IEEE 754]" +
			" (http://en.wikipedia.org/wiki/IEEE_floating_point).",
		Unmarshaler: uFloat,
	}
	// StringType scalar.
	StringType = Scalar{
		Name: "string",
		id:   stringID,
		Description: "The 'String' scalar type represents textual data, represented" +
			" as UTF-8 character sequences. The String type is most often" +
			" used by GraphQL to represent free-form human-readable text.",
		Unmarshaler: uString,
	}
	// BooleanType scalar.
	BooleanType = Scalar{
		Name:        "bool",
		id:          boolID,
		Description: "The 'Boolean' scalar type represents 'true' or 'false'.",
		Unmarshaler: uBool,
	}
	// IDType scalar.
	IDType = Scalar{
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
	// DateTimeType scalar.
	DateTimeType = Scalar{
		Name: "datetime",
		id:   dateTimeID,
		Description: "The 'DateTime' scalar type an instant in time with nanosecond" +
			" precision. Each DateTime is associated with a timezone.",
		Unmarshaler: uTime,
	}
)

// stores a mapping between a string name of a type
var typeNameMap = map[string]Type{
	Int32Type.Name:    Int32Type,
	FloatType.Name:    FloatType,
	StringType.Name:   StringType,
	BooleanType.Name:  BooleanType,
	IDType.Name:       IDType,
	DateTimeType.Name: DateTimeType,
}

// stores a mapping between the typeID to a type
var typeIDMap = map[TypeID]Type{
	Int32Type.ID():    Int32Type,
	FloatType.ID():    FloatType,
	StringType.ID():   StringType,
	BooleanType.ID():  BooleanType,
	IDType.ID():       IDType,
	DateTimeType.ID(): DateTimeType,
}

// TypeForName returns the type corresponding to the given name.
// If name is not recognized, it returns nil.
func TypeForName(name string) Type {
	return typeNameMap[name]
}

// TypeForID returns the type corresponding to the given TypeID.
// If id is not recognized, it returns nil.
func TypeForID(id TypeID) Type {
	return typeIDMap[id]
}

// Int32 is the scalar type for int32
type Int32 int32

// MarshalBinary marshals to binary
func (v Int32) MarshalBinary() ([]byte, error) {
	var bs [4]byte
	binary.LittleEndian.PutUint32(bs[:], uint32(v))
	return bs[:], nil
}

// MarshalText marshals to text
func (v Int32) MarshalText() ([]byte, error) {
	s := strconv.FormatInt(int64(v), 10)
	return []byte(s), nil
}

// MarshalJSON marshals to json
func (v Int32) MarshalJSON() ([]byte, error) {
	return json.Marshal(int32(v))
}

type unmarshalInt32 struct{}

func (u unmarshalInt32) FromBinary(data []byte) (TypeValue, error) {
	if len(data) < 4 {
		return nil, x.Errorf("Invalid data for int32 %v", data)
	}
	val := binary.LittleEndian.Uint32(data)
	return Int32(val), nil
}

func (u unmarshalInt32) FromText(text []byte) (TypeValue, error) {
	val, err := strconv.ParseInt(string(text), 10, 32)
	if err != nil {
		return nil, err
	}
	return Int32(val), nil
}

var uInt32 unmarshalInt32

// Float is the scalar type for float64
type Float float64

// MarshalBinary marshals to binary
func (v Float) MarshalBinary() ([]byte, error) {
	var bs [8]byte
	u := math.Float64bits(float64(v))
	binary.LittleEndian.PutUint64(bs[:], u)
	return bs[:], nil
}

// MarshalText marshals to text
func (v Float) MarshalText() ([]byte, error) {
	s := strconv.FormatFloat(float64(v), 'E', -1, 64)
	return []byte(s), nil
}

// MarshalJSON marshals to json
func (v Float) MarshalJSON() ([]byte, error) {
	return json.Marshal(float64(v))
}

type unmarshalFloat struct{}

func (u unmarshalFloat) FromBinary(data []byte) (TypeValue, error) {
	if len(data) < 8 {
		return nil, x.Errorf("Invalid data for float %v", data)
	}
	v := binary.LittleEndian.Uint64(data)
	val := math.Float64frombits(v)
	return Float(val), nil
}

func (u unmarshalFloat) FromText(text []byte) (TypeValue, error) {
	val, err := strconv.ParseFloat(string(text), 64)
	if err != nil {
		return nil, err
	}
	return Float(val), nil
}

var uFloat unmarshalFloat

// String is the scalar type for string
type String string

// MarshalBinary marshals to binary
func (v String) MarshalBinary() ([]byte, error) {
	return []byte(v), nil
}

// MarshalText marshals to text
func (v String) MarshalText() ([]byte, error) {
	return v.MarshalBinary()
}

// MarshalJSON marshals to json
func (v String) MarshalJSON() ([]byte, error) {
	return json.Marshal(string(v))
}

type unmarshalString struct{}

func (v unmarshalString) FromBinary(data []byte) (TypeValue, error) {
	return String(data), nil
}

func (v unmarshalString) FromText(text []byte) (TypeValue, error) {
	return v.FromBinary(text)
}

var uString unmarshalString

// Bool is the scalar type for bool
type Bool bool

// MarshalBinary marshals to binary
func (v Bool) MarshalBinary() ([]byte, error) {
	var bs [1]byte
	if v {
		bs[0] = 1
	} else {
		bs[0] = 0
	}
	return bs[:], nil
}

// MarshalText marshals to text
func (v Bool) MarshalText() ([]byte, error) {
	s := strconv.FormatBool(bool(v))
	return []byte(s), nil
}

// MarshalJSON marshals to json
func (v Bool) MarshalJSON() ([]byte, error) {
	return json.Marshal(bool(v))
}

type unmarshalBool struct{}

func (u unmarshalBool) FromBinary(data []byte) (TypeValue, error) {
	if data[0] == 0 {
		return Bool(false), nil
	} else if data[0] == 1 {
		return Bool(true), nil
	} else {
		return nil, x.Errorf("Invalid value for bool %v", data[0])
	}
}

func (u unmarshalBool) FromText(text []byte) (TypeValue, error) {
	val, err := strconv.ParseBool(string(text))
	if err != nil {
		return nil, err
	}
	return Bool(val), nil
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
		// Try parsing without timezone since that is a valid format
		if v, err = time.Parse("2006-01-02T15:04:05", string(text)); err != nil {
			return nil, err
		}
	}
	return v, nil
}

var uTime unmarshalTime

func (u unmarshalInt32) fromFloat(f float64) (TypeValue, error) {
	if f > math.MaxInt32 || f < math.MinInt32 || math.IsNaN(f) {
		return nil, x.Errorf("Float out of int32 range")
	}
	return Int32(f), nil
}

func (u unmarshalInt32) fromBool(b bool) (TypeValue, error) {
	if b {
		return Int32(1), nil
	}
	return Int32(0), nil
}

func (u unmarshalInt32) fromTime(t time.Time) (TypeValue, error) {
	// Represent the unix timestamp as a 32bit int.
	secs := t.Unix()
	if secs > math.MaxInt32 || secs < math.MinInt32 {
		return nil, x.Errorf("Time out of int32 range")
	}
	return Int32(secs), nil
}

func (u unmarshalFloat) fromInt(i int32) (TypeValue, error) {
	return Float(i), nil
}

func (u unmarshalFloat) fromBool(b bool) (TypeValue, error) {
	if b {
		return Float(1), nil
	}
	return Float(0), nil
}

const (
	nanoSecondsInSec = 1000000000
)

func (u unmarshalFloat) fromTime(t time.Time) (TypeValue, error) {
	// Represent the unix timestamp as a float (with fractional seconds)
	secs := float64(t.Unix())
	nano := float64(t.Nanosecond())
	val := secs + nano/nanoSecondsInSec
	return Float(val), nil
}

func (u unmarshalBool) fromInt(i int32) (TypeValue, error) {
	return Bool(i != 0), nil
}

func (u unmarshalBool) fromFloat(f float64) (TypeValue, error) {
	return Bool(f != 0), nil
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
