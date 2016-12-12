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
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/dgraph-io/dgraph/x"
	geom "github.com/twpayne/go-geom"
	"github.com/twpayne/go-geom/encoding/geojson"
	"github.com/twpayne/go-geom/encoding/wkb"
)

// Note: These ids are stored in the posting lists to indicate the type
// of the data. The order *cannot* be changed without breaking existing
// data. When adding a new type *always* add to the end of this list.
// Never delete anything from this list even if it becomes unused.
const (
	BytesID  TypeID = 0
	Int32ID  TypeID = 1
	FloatID  TypeID = 2
	BoolID   TypeID = 3
	TimeID   TypeID = 4
	StringID TypeID = 5
	DateID   TypeID = 6
	GeoID    TypeID = 7
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
	timeType = Scalar{
		Name: "time",
		id:   TimeID,
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
	timeType.Name:      timeType,
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

	case TimeID:
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
	return timeType
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

func (v *Int32) fromInt(t Int32) error {
	if t > math.MaxInt32 || t < math.MinInt32 {
		return x.Errorf("Int out of int32 range")
	}
	*v = Int32(t)
	return nil
}

func (v *Geo) fromInt(t Int32) error {
	return cantConvert(t.Type(), v)
}

func (v *Geo) fromFloat(t Float) error {
	return cantConvert(t.Type(), v)
}

func (v *Geo) fromBool(t Bool) error {
	return cantConvert(t.Type(), v)
}

func (v *Int32) fromFloat(t Float) error {
	if t > math.MaxInt32 || t < math.MinInt32 || math.IsNaN(float64(t)) {
		return x.Errorf("Float out of int32 range")
	}
	*v = Int32(t)
	return nil
}

func (v *Float) fromFloat(t Float) error {
	*v = t
	return nil
}

func (v *Geo) fromGeo(t Geo) error {
	*v = t
	return nil
}

func (v *Bool) fromBool(t Bool) error {
	*v = t
	return nil
}

func (v *Int32) fromBool(t Bool) error {
	if t {
		*v = 1
	} else {
		*v = 0
	}
	return nil
}

func (v *Int32) fromTime(t Time) error {
	// Represent the unix timestamp as a 32bit int.
	secs := t.Time.Unix()
	if secs > math.MaxInt32 || secs < math.MinInt32 {
		return x.Errorf("Time out of int32 range")
	}
	*v = Int32(secs)
	return nil
}

func (v *Int32) fromGeo(t Geo) error {
	return cantConvert(t.Type(), v)
}

func (v *Bool) fromGeo(t Geo) error {
	return cantConvert(t.Type(), v)
}

func (v *Float) fromGeo(t Geo) error {
	return cantConvert(t.Type(), v)
}

func (v *Time) fromGeo(t Geo) error {
	return cantConvert(t.Type(), v)
}

func (v *Float) fromInt(t Int32) error {
	*v = Float(int32(t))
	return nil
}

func (v *Float) fromBool(t Bool) error {
	if bool(t) {
		*v = 1
	} else {
		*v = 0
	}
	return nil
}

const (
	nanoSecondsInSec = 1000000000
)

func (v *Float) fromTime(t Time) error {
	// Represent the unix timestamp as a float (with fractional seconds)
	secs := float64(t.Time.Unix())
	nano := float64(t.Nanosecond())
	val := secs + nano/nanoSecondsInSec
	*v = Float(val)
	return nil
}

func (v *Bool) fromInt(t Int32) error {
	*v = int32(t) != 0
	return nil
}

func (v *Bool) fromFloat(t Float) error {
	*v = float64(t) != 0
	return nil
}

func (v *Time) fromInt(t Int32) error {
	v.Time = time.Unix(int64(t), 0).UTC()
	return nil
}

func (v *Time) fromFloat(t Float) error {
	secs := int64(t)
	fracSecs := float64(t) - float64(secs)
	nsecs := int64(fracSecs * nanoSecondsInSec)
	v.Time = time.Unix(secs, nsecs).UTC()
	return nil
}

// Geo represents geo-spatial data.
type Geo struct {
	geom.T
}

// MarshalBinary marshals to binary
func (v Geo) MarshalBinary() ([]byte, error) {
	return wkb.Marshal(v.T, binary.LittleEndian)
}

// MarshalText marshals to text
func (v Geo) MarshalText() ([]byte, error) {
	// The text format is geojson
	res, err := geojson.Marshal(v.T)
	return bytes.Replace(res, []byte("\""), []byte("'"), -1), err
}

// MarshalJSON marshals to json
func (v Geo) MarshalJSON() ([]byte, error) {
	// this same as MarshalText
	return geojson.Marshal(v.T)
}

// Type returns the type of this value
func (v Geo) Type() Scalar {
	return geoType
}

// UnmarshalBinary unmarshals the data from WKB
func (v *Geo) UnmarshalBinary(data []byte) error {
	w, err := wkb.Unmarshal(data)
	if err != nil {
		return err
	}
	v.T = w
	return nil
}

// UnmarshalText parses the data from a Geojson
func (v *Geo) UnmarshalText(text []byte) error {
	var g geom.T
	text = bytes.Replace(text, []byte("'"), []byte("\""), -1)
	if err := geojson.Unmarshal(text, &g); err != nil {
		return err
	}
	v.T = g
	return nil
}

func (v Geo) String() string {
	return "<geodata>"
}

// Date represents a date (YYYY-MM-DD). There is no timezone information
// attached.
type Date struct {
	time.Time
}

func createDate(y int, m time.Month, d int) Date {
	var dt Date
	dt.Time = time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	return dt
}

const dateFormatYMD = "2006-01-02"
const dateFormatYM = "2006-01"
const dateFormatY = "2006"

// MarshalBinary marshals to binary
func (v Date) MarshalBinary() ([]byte, error) {
	var bs [8]byte
	binary.LittleEndian.PutUint64(bs[:], uint64(v.Time.Unix()))
	return bs[:], nil
}

// MarshalText marshals to text
func (v Date) MarshalText() ([]byte, error) {
	s := v.Time.Format(dateFormatYMD)
	return []byte(s), nil
}

// MarshalJSON marshals to json
func (v Date) MarshalJSON() ([]byte, error) {
	s := v.Time.Format(dateFormatYMD)
	return json.Marshal(s)
}

// Type returns the type of this value
func (v Date) Type() Scalar {
	return dateType
}

func (v Date) String() string {
	str, _ := v.MarshalText()
	return string(str)
}

// UnmarshalBinary unmarshals the data from a binary format.
func (v *Date) UnmarshalBinary(data []byte) error {
	if len(data) < 8 {
		return x.Errorf("Invalid data for date %v", data)
	}
	val := binary.LittleEndian.Uint64(data)
	tm := time.Unix(int64(val), 0)
	return v.fromTime(Time{tm})
}

// UnmarshalText unmarshals the data from a text format.
func (v *Date) UnmarshalText(text []byte) error {
	val, err := time.Parse(dateFormatYMD, string(text))
	if err != nil {
		val, err = time.Parse(dateFormatYM, string(text))
		if err != nil {
			val, err = time.Parse(dateFormatY, string(text))
			if err != nil {
				return err
			}
		}
	}
	return v.fromTime(Time{val})
}

func (v *Date) fromFloat(f Float) error {
	var t Time
	err := t.fromFloat(f)
	if err != nil {
		return err
	}
	return v.fromTime(t)
}

func (v *Date) fromTime(t Time) error {
	// truncate the time to just a date.
	*v = createDate(t.Date())
	return nil
}

func (v *Time) fromTime(t Time) error {
	// truncate the time to just a date.
	*v = t
	return nil
}

func (v *Bool) fromTime(t Time) error {
	return cantConvert(t.Type(), v)
}

func (v *Geo) fromTime(t Time) error {
	return cantConvert(t.Type(), v)
}

func (v *Date) fromGeo(t Geo) error {
	return cantConvert(t.Type(), v)
}

func (v *Date) fromBool(t Bool) error {
	return cantConvert(t.Type(), v)
}

func (v *Time) fromBool(t Bool) error {
	return cantConvert(t.Type(), v)
}

func (v *Date) fromInt(i Int32) error {
	var t Time
	err := t.fromInt(i)
	if err != nil {
		return err
	}
	return v.fromTime(Time(t))
}

func (v *Date) fromDate(t Date) error {
	*v = t
	return nil
}

func (v *Time) fromDate(d Date) error {
	v.Time = d.Time
	return nil
}

func (v *Float) fromDate(d Date) error {
	return v.fromTime(Time{d.Time})
}

func (v *Int32) fromDate(d Date) error {
	return v.fromTime(Time{d.Time})
}

func (v *Bool) fromDate(d Date) error {
	return cantConvert(d.Type(), v)
}

func (v *Geo) fromDate(d Date) error {
	return cantConvert(d.Type(), v)
}
