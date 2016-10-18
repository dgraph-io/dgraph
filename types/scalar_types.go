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
	"fmt"
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
	BytesID    TypeID = 0
	Int32ID    TypeID = 1
	FloatID    TypeID = 2
	BoolID     TypeID = 3
	DateTimeID TypeID = 4
	StringID   TypeID = 5
	DateID     TypeID = 6
	GeoID      TypeID = 7
)

// added suffix 'type' to names to distinguish from Go types 'int' and 'string'
var (
	int32Type = Scalar{
		Name: "int",
		id:   Int32ID,
	}
	floatType = Scalar{
		Name: "float",
		id:   FloatID,
	}
	stringType = Scalar{
		Name: "string",
		id:   StringID,
	}
	byteArrayType = Scalar{
		Name: "bytes",
		id:   BytesID,
	}
	booleanType = Scalar{
		Name: "bool",
		id:   BoolID,
	}
	idType = Scalar{
		Name: "id",
		id:   StringID,
	}
	dateTimeType = Scalar{
		Name: "datetime",
		id:   DateTimeID,
	}
	dateType = Scalar{
		Name: "date",
		id:   DateID,
	}
	geoType = Scalar{
		Name: "geo",
		id:   GeoID,
	}
)

// stores a mapping between a string name of a type
var typeNameMap = map[string]Type{
	int32Type.Name:     int32Type,
	floatType.Name:     floatType,
	stringType.Name:    stringType,
	booleanType.Name:   booleanType,
	idType.Name:        idType,
	dateTimeType.Name:  dateTimeType,
	dateType.Name:      dateType,
	geoType.Name:       geoType,
	byteArrayType.Name: byteArrayType,
}

// TypeForName returns the type corresponding to the given name.
// If name is not recognized, it returns nil.
func TypeForName(name string) (Type, bool) {
	t, ok := typeNameMap[name]
	return t, ok
}

// ValueForType returns the zero value for a type id
func ValueForType(id TypeID) Value {
	switch id {
	case BytesID:
		var b Bytes
		return &b

	case Int32ID:
		var i Int32
		return &i

	case FloatID:
		var f Float
		return &f

	case BoolID:
		var b Bool
		return &b

	case DateTimeID:
		var t Time
		return &t

	case StringID:
		var s String
		return &s

	case DateID:
		var d Date
		return &d

	case GeoID:
		var g Geo
		return &g

	default:
		return nil
	}
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
func (v Int32) Type() Scalar {
	return int32Type
}

func (v Int32) String() string {
	return fmt.Sprintf("%v", int32(v))
}

// UnmarshalBinary unmarshals the data from a binary format.
func (v *Int32) UnmarshalBinary(data []byte) error {
	if len(data) < 4 {
		return x.Errorf("Invalid data for int32 %v", data)
	}
	*v = Int32(binary.LittleEndian.Uint32(data))
	return nil
}

// UnmarshalText unmarshals the data from a text format.
func (v *Int32) UnmarshalText(text []byte) error {
	val, err := strconv.ParseInt(string(text), 10, 32)
	if err != nil {
		return err
	}
	*v = Int32(val)
	return nil
}

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
func (v Float) Type() Scalar {
	return floatType
}

// UnmarshalBinary unmarshals the data from a binary format.
func (v *Float) UnmarshalBinary(data []byte) error {
	if len(data) < 8 {
		return x.Errorf("Invalid data for float %v", data)
	}
	i := binary.LittleEndian.Uint64(data)
	val := math.Float64frombits(i)
	*v = Float(val)
	return nil
}

// UnmarshalText unmarshals the data from a text format.
func (v *Float) UnmarshalText(text []byte) error {
	val, err := strconv.ParseFloat(string(text), 64)
	if err != nil {
		return err
	}
	*v = Float(val)
	return nil
}

func (v Float) String() string {
	return fmt.Sprintf("%v", float64(v))
}

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
func (v String) Type() Scalar {
	return stringType
}

// UnmarshalBinary unmarshals the data from a binary format.
func (v *String) UnmarshalBinary(data []byte) error {
	*v = String(data)
	return nil
}

// UnmarshalText unmarshals the data from a text format.
func (v *String) UnmarshalText(text []byte) error {
	return v.UnmarshalBinary(text)
}

func (v String) String() string {
	return string(v)
}

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
func (v Bytes) Type() Scalar {
	return byteArrayType
}

// UnmarshalBinary unmarshals the data from a binary format.
func (v *Bytes) UnmarshalBinary(data []byte) error {
	*v = Bytes(data)
	return nil
}

// UnmarshalText unmarshals the data from a text format.
func (v *Bytes) UnmarshalText(text []byte) error {
	return v.UnmarshalBinary(text)
}

func (v Bytes) String() string {
	return string(v)
}

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
func (v Bool) Type() Scalar {
	return booleanType
}

// UnmarshalBinary unmarshals the data from a binary format.
func (v *Bool) UnmarshalBinary(data []byte) error {
	if data[0] == 0 {
		*v = Bool(false)
		return nil
	} else if data[0] == 1 {
		*v = Bool(true)
		return nil
	} else {
		return x.Errorf("Invalid value for bool %v", data[0])
	}
}

// UnmarshalText unmarshals the data from a text format.
func (v *Bool) UnmarshalText(text []byte) error {
	val, err := strconv.ParseBool(string(text))
	if err != nil {
		return err
	}
	*v = Bool(val)
	return nil
}

func (v Bool) String() string {
	return fmt.Sprintf("%v", bool(v))
}

// Time wraps time.Time to add the Value interface
type Time struct {
	time.Time
}

// Type returns the type of this value
func (v Time) Type() Scalar {
	return dateTimeType
}

// UnmarshalText unmarshals the data from a text format.
func (v *Time) UnmarshalText(text []byte) error {
	var t time.Time
	if err := t.UnmarshalText(text); err != nil {
		// Try parsing without timezone since that is a valid format
		if t, err = time.Parse("2006-01-02T15:04:05", string(text)); err != nil {
			return err
		}
	}
	v.Time = t
	return nil
}

func (v *Int32) fromFloat(f float64) error {
	if f > math.MaxInt32 || f < math.MinInt32 || math.IsNaN(f) {
		return x.Errorf("Float out of int32 range")
	}
	*v = Int32(f)
	return nil
}

func (v *Int32) fromBool(b bool) error {
	if b {
		*v = 1
	} else {
		*v = 0
	}
	return nil
}

func (v *Int32) fromTime(t time.Time) error {
	// Represent the unix timestamp as a 32bit int.
	secs := t.Unix()
	if secs > math.MaxInt32 || secs < math.MinInt32 {
		return x.Errorf("Time out of int32 range")
	}
	*v = Int32(secs)
	return nil
}

func (v *Float) fromInt(i int32) error {
	*v = Float(i)
	return nil
}

func (v *Float) fromBool(b bool) error {
	if b {
		*v = 1
	} else {
		*v = 0
	}
	return nil
}

const (
	nanoSecondsInSec = 1000000000
)

func (v *Float) fromTime(t time.Time) error {
	// Represent the unix timestamp as a float (with fractional seconds)
	secs := float64(t.Unix())
	nano := float64(t.Nanosecond())
	val := secs + nano/nanoSecondsInSec
	*v = Float(val)
	return nil
}

func (v *Bool) fromInt(i int32) error {
	*v = i != 0
	return nil
}

func (v *Bool) fromFloat(f float64) error {
	*v = f != 0
	return nil
}

func (v *Time) fromInt(i int32) error {
	v.Time = time.Unix(int64(i), 0).UTC()
	return nil
}

func (v *Time) fromFloat(f float64) error {
	secs := int64(f)
	fracSecs := f - float64(secs)
	nsecs := int64(fracSecs * nanoSecondsInSec)
	v.Time = time.Unix(secs, nsecs).UTC()
	return nil
}
