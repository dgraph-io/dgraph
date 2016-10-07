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
	bytesID    TypeID = 0
	int32ID           = 1
	floatID           = 2
	boolID            = 3
	dateTimeID        = 4
	stringID          = 5
	dateID            = 6
	geoID             = 7
)

// added suffix 'type' to names to distinguish from Go types 'int' and 'string'
var (
	// Int32Type scalar.
	Int32Type = Scalar{
		Name:        "int",
		id:          int32ID,
		Unmarshaler: uInt32,
	}
	// FloatType scalar.
	FloatType = Scalar{
		Name:        "float",
		id:          floatID,
		Unmarshaler: uFloat,
	}
	// StringType scalar.
	StringType = Scalar{
		Name:        "string",
		id:          stringID,
		Unmarshaler: uString,
	}
	// ByteArrayType scalar.
	ByteArrayType = Scalar{
		Name:        "bytes",
		id:          bytesID,
		Unmarshaler: uBytes,
	}
	// BooleanType scalar.
	BooleanType = Scalar{
		Name:        "bool",
		id:          boolID,
		Unmarshaler: uBool,
	}
	// IDType scalar.
	IDType = Scalar{
		Name:        "id",
		id:          stringID,
		Unmarshaler: uString,
	}
	// DateTimeType scalar.
	DateTimeType = Scalar{
		Name:        "datetime",
		id:          dateTimeID,
		Unmarshaler: uTime,
	}
	// DateType scalar.
	DateType = Scalar{
		Name:        "date",
		id:          dateID,
		Unmarshaler: uDate,
	}
	// GeoType scalar.
	GeoType = Scalar{
		Name:        "geo",
		id:          geoID,
		Unmarshaler: uGeo,
	}
)

// stores a mapping between a string name of a type
var typeNameMap = map[string]Type{
	"int":      Int32Type,
	"float":    FloatType,
	"string":   StringType,
	"bool":     BooleanType,
	"id":       IDType,
	"datetime": DateTimeType,
	"date":     DateType,
	"geo":      GeoType,
	"bytes":    ByteArrayType,
}

// stores a mapping between the typeID to a type
var typeIDMap = map[TypeID]Type{
	Int32Type.ID():     Int32Type,
	FloatType.ID():     FloatType,
	StringType.ID():    StringType,
	BooleanType.ID():   BooleanType,
	IDType.ID():        IDType,
	DateTimeType.ID():  DateTimeType,
	ByteArrayType.ID(): ByteArrayType,
	DateType.ID():      DateType,
	GeoType.ID():       GeoType,
}

// TypeForName returns the type corresponding to the given name.
// If name is not recognized, it returns nil.
func TypeForName(name string) (Type, bool) {
	t, ok := typeNameMap[name]
	return t, ok
}

// TypeForID returns the type corresponding to the given TypeID.
// If id is not recognized, it returns nil.
func TypeForID(id TypeID) (Type, bool) {
	t, ok := typeIDMap[id]
	return t, ok
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

// Type returns the type of this value
func (v Int32) Type() Type {
	return typeIDMap[int32ID]
}

type unmarshalInt32 struct{}

func (u unmarshalInt32) FromBinary(data []byte) (Value, error) {
	if len(data) < 4 {
		return nil, x.Errorf("Invalid data for int32 %v", data)
	}
	val := binary.LittleEndian.Uint32(data)
	return Int32(val), nil
}

func (u unmarshalInt32) FromText(text []byte) (Value, error) {
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

// Type returns the type of this value
func (v Float) Type() Type {
	return typeIDMap[floatID]
}

type unmarshalFloat struct{}

func (u unmarshalFloat) FromBinary(data []byte) (Value, error) {
	if len(data) < 8 {
		return nil, x.Errorf("Invalid data for float %v", data)
	}
	v := binary.LittleEndian.Uint64(data)
	val := math.Float64frombits(v)
	return Float(val), nil
}

func (u unmarshalFloat) FromText(text []byte) (Value, error) {
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

// Type returns the type of this value
func (v String) Type() Type {
	return typeIDMap[stringID]
}

type unmarshalString struct{}

func (v unmarshalString) FromBinary(data []byte) (Value, error) {
	return String(data), nil
}

func (v unmarshalString) FromText(text []byte) (Value, error) {
	return v.FromBinary(text)
}

var uString unmarshalString

// Bytes is the scalar type for []byte
type Bytes []byte

// MarshalBinary marshals to binary
func (v Bytes) MarshalBinary() ([]byte, error) {
	return []byte(v), nil
}

// MarshalText marshals to text
func (v Bytes) MarshalText() ([]byte, error) {
	return v.MarshalBinary()
}

// MarshalJSON marshals to json
func (v Bytes) MarshalJSON() ([]byte, error) {
	// TODO: should we encode this somehow if they are are not printable characters.
	return json.Marshal(string(v))
}

// Type returns the type of this value
func (v Bytes) Type() Type {
	return typeIDMap[bytesID]
}

type unmarshalBytes struct{}

func (v unmarshalBytes) FromBinary(data []byte) (Value, error) {
	return Bytes(data), nil
}

func (v unmarshalBytes) FromText(text []byte) (Value, error) {
	return v.FromBinary(text)
}

var uBytes unmarshalBytes

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

// Type returns the type of this value
func (v Bool) Type() Type {
	return typeIDMap[boolID]
}

type unmarshalBool struct{}

func (u unmarshalBool) FromBinary(data []byte) (Value, error) {
	if data[0] == 0 {
		return Bool(false), nil
	} else if data[0] == 1 {
		return Bool(true), nil
	} else {
		return nil, x.Errorf("Invalid value for bool %v", data[0])
	}
}

func (u unmarshalBool) FromText(text []byte) (Value, error) {
	val, err := strconv.ParseBool(string(text))
	if err != nil {
		return nil, err
	}
	return Bool(val), nil
}

var uBool unmarshalBool

// Time wraps time.Time to add the Value interface
type Time struct {
	time.Time
}

// Type returns the type of this value
func (v Time) Type() Type {
	return typeIDMap[dateTimeID]
}

type unmarshalTime struct{}

func (u unmarshalTime) FromBinary(data []byte) (Value, error) {
	var v time.Time
	if err := v.UnmarshalBinary(data); err != nil {
		return nil, err
	}
	return Time{v}, nil
}

func (u unmarshalTime) FromText(text []byte) (Value, error) {
	var v time.Time
	if err := v.UnmarshalText(text); err != nil {
		// Try parsing without timezone since that is a valid format
		if v, err = time.Parse("2006-01-02T15:04:05", string(text)); err != nil {
			return nil, err
		}
	}
	return Time{v}, nil
}

var uTime unmarshalTime

func (u unmarshalInt32) fromFloat(f float64) (Value, error) {
	if f > math.MaxInt32 || f < math.MinInt32 || math.IsNaN(f) {
		return nil, x.Errorf("Float out of int32 range")
	}
	return Int32(f), nil
}

func (u unmarshalInt32) fromBool(b bool) (Value, error) {
	if b {
		return Int32(1), nil
	}
	return Int32(0), nil
}

func (u unmarshalInt32) fromTime(t time.Time) (Value, error) {
	// Represent the unix timestamp as a 32bit int.
	secs := t.Unix()
	if secs > math.MaxInt32 || secs < math.MinInt32 {
		return nil, x.Errorf("Time out of int32 range")
	}
	return Int32(secs), nil
}

func (u unmarshalFloat) fromInt(i int32) (Value, error) {
	return Float(i), nil
}

func (u unmarshalFloat) fromBool(b bool) (Value, error) {
	if b {
		return Float(1), nil
	}
	return Float(0), nil
}

const (
	nanoSecondsInSec = 1000000000
)

func (u unmarshalFloat) fromTime(t time.Time) (Value, error) {
	// Represent the unix timestamp as a float (with fractional seconds)
	secs := float64(t.Unix())
	nano := float64(t.Nanosecond())
	val := secs + nano/nanoSecondsInSec
	return Float(val), nil
}

func (u unmarshalBool) fromInt(i int32) (Value, error) {
	return Bool(i != 0), nil
}

func (u unmarshalBool) fromFloat(f float64) (Value, error) {
	return Bool(f != 0), nil
}

func (u unmarshalTime) fromInt(i int32) (Value, error) {
	t := time.Unix(int64(i), 0).UTC()
	return Time{t}, nil
}

func (u unmarshalTime) fromFloat(f float64) (Value, error) {
	secs := int64(f)
	fracSecs := f - float64(secs)
	nsecs := int64(fracSecs * nanoSecondsInSec)
	t := time.Unix(secs, nsecs).UTC()
	return Time{t}, nil
}
