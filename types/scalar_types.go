/*
 * Copyright 2016-2023 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package types

import (
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/twpayne/go-geom"

	"github.com/dgraph-io/dgraph/protos/pb"
)

const nanoSecondsInSec = 1000000000
const dateFormatY = "2006" // time.longYear
const dateFormatYM = "2006-01"
const dateFormatYMD = "2006-01-02"
const dateFormatYMDZone = "2006-01-02 15:04:05 -0700 MST"
const dateTimeFormat = "2006-01-02T15:04:05"

// Note: These ids are stored in the posting lists to indicate the type
// of the data. The order *cannot* be changed without breaking existing
// data. When adding a new type *always* add to the end of this list.
// Never delete anything from this list even if it becomes unused.
const (
	// DefaultID represents the default type.
	DefaultID = TypeID(pb.Posting_DEFAULT)
	// BinaryID represents the binary data type.
	BinaryID = TypeID(pb.Posting_BINARY)
	// IntID represents the integer type.
	IntID = TypeID(pb.Posting_INT)
	// FloatID represents the floating-point number type.
	FloatID = TypeID(pb.Posting_FLOAT)
	// BoolID represents the boolean type.
	BoolID = TypeID(pb.Posting_BOOL)
	// DateTimeID represents the datetime type.
	DateTimeID = TypeID(pb.Posting_DATETIME)
	// GeoID represents the geo-location data type.
	GeoID = TypeID(pb.Posting_GEO)
	// UidID represents the uid type.
	UidID = TypeID(pb.Posting_UID)
	// PasswordID represents the password type.
	PasswordID = TypeID(pb.Posting_PASSWORD)
	// StringID represents the string type.
	StringID = TypeID(pb.Posting_STRING)
	// TODO: Add constant for pb.Posting_OBJECT ??
	//       Not clear if it belongs here, but if it does,
	//       we should add it, if not, we should document here
	//       why it does not belong here.
	// VFloatID represents a vector of IEEE754 64-bit floats.
	VFloatID = TypeID(pb.Posting_VFLOAT)
	// UndefinedID represents the undefined type.
	UndefinedID = TypeID(100)
)

var typeNameMap = map[string]TypeID{
	"default":  DefaultID,
	"binary":   BinaryID,
	"int":      IntID,
	"float":    FloatID,
	"bool":     BoolID,
	"datetime": DateTimeID,
	"geo":      GeoID,
	"uid":      UidID,
	"string":   StringID,
	"password": PasswordID,
	"vfloat":   VFloatID,
}

// TypeID represents the type of the data.
type TypeID pb.Posting_ValType

// Enum takes a TypeID value and returns the corresponding ValType enum value.
func (t TypeID) Enum() pb.Posting_ValType {
	return pb.Posting_ValType(t)
}

// Name returns the name of the type.
func (t TypeID) Name() string {
	switch t {
	case DefaultID:
		return "default"
	case BinaryID:
		return "binary"
	case IntID:
		return "int"
	case FloatID:
		return "float"
	case BoolID:
		return "bool"
	case DateTimeID:
		return "datetime"
	case GeoID:
		return "geo"
	case UidID:
		return "uid"
	case StringID:
		return "string"
	case PasswordID:
		return "password"
	case VFloatID:
		return "vfloat"
	}
	return ""
}

// Val is a value with type information.
type Val struct {
	Tid   TypeID
	Value interface{}
}

// Safe ensures that Val's Value is not nil. This is useful when doing type
// assertions and default values might be involved.
// This function won't change the original v.Value, may it be nil.
// See: "Default value vars" in `fillVars()`
// Returns a safe v.Value suitable for type assertions.
func (v Val) Safe() interface{} {
	if v.Value == nil {
		// get zero value for this v.Tid
		va := ValueForType(v.Tid)
		return va.Value
	}
	return v.Value
}

// TypeForName returns the type corresponding to the given name.
// If name is not recognized, it returns nil.
func TypeForName(name string) (TypeID, bool) {
	t, ok := typeNameMap[name]
	return t, ok
}

// IsScalar returns whether the type is a scalar type.
func (t TypeID) IsScalar() bool {
	return t != UidID
}

// IsNumber returns whether the type is a number type.
func (t TypeID) IsNumber() bool {
	return t == IntID || t == FloatID
}

// ValueForType returns the zero value for a type id
func ValueForType(id TypeID) Val {
	switch id {
	case BinaryID:
		var b []byte
		return Val{BinaryID, &b}

	case IntID:
		var i int64
		return Val{IntID, &i}

	case FloatID:
		var f float64
		return Val{FloatID, &f}

	case BoolID:
		var b bool
		return Val{BoolID, &b}

	case DateTimeID:
		var t time.Time
		return Val{DateTimeID, &t}

	case StringID:
		var s string
		return Val{StringID, s}

	case DefaultID:
		var s string
		return Val{DefaultID, s}

	case GeoID:
		var g geom.T
		return Val{GeoID, &g}

	case UidID:
		var i uint64
		return Val{UidID, &i}

	case PasswordID:
		var p string
		return Val{PasswordID, p}
	case VFloatID:
		var v []float64
		return Val{VFloatID, &v}
	default:
		return Val{}
	}
}

// GoodTimeZone returns true if timezone (provided in offset
// format in seconds) is valid according to RFC3339.
func GoodTimeZone(offset int) bool {
	const boundary = 23*60*60 + 59*60
	return offset <= boundary && offset >= -1*boundary
}

// ParseTime parses the time from string trying various datetime formats.
// By default, Go parses time in UTC unless specified in the data itself.
func ParseTime(val string) (time.Time, error) {
	t, err := parseTimeNonStrict(val)
	if err != nil {
		return t, err
	}

	_, offset := t.Zone()
	if !GoodTimeZone(offset) {
		return time.Time{}, errors.New("timezone outside of range [-23:59,23:59]")
	}

	return t, nil
}

func parseTimeNonStrict(val string) (time.Time, error) {
	if len(val) == len(dateFormatY) {
		return time.Parse(dateFormatY, val)
	}
	if len(val) == len(dateFormatYM) {
		return time.Parse(dateFormatYM, val)
	}
	if len(val) == len(dateFormatYMD) {
		return time.Parse(dateFormatYMD, val)
	}
	if len(val) > len(dateTimeFormat) && val[len(dateFormatYMD)] == 'T' &&
		(val[len(val)-1] == 'Z' || val[len(val)-3] == ':') {
		// https://tools.ietf.org/html/rfc3339#section-5.6
		return time.Parse(time.RFC3339, val)
	}
	if t, err := time.Parse(dateFormatYMDZone, val); err == nil {
		return t, err
	}
	// Try without timezone.
	return time.Parse(dateTimeFormat, val)
}

// parseVFloat(s) will generate a slice of float64 values,
// as long as s is either an empty string, or if it is formatted
// according to the following ebnf:
//
//	floatArray ::= "[" [floatList] [whitespace] "]"
//	floatList := float64Val |
//	             float64Val floatSpaceList |
//	             float64Val floatCommaList
//	floatSpaceList := (whitespace float64Val)+
//	floatCommaList := ([whitespace] "," [whitespace] float64Val)+
//	float64Val := < a string rep of a float64 value >
func ParseVFloat(s string) ([]float64, error) {
	s = strings.TrimSpace(s)
	if len(s) == 0 {
		return []float64{}, nil
	}
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\t", " ")
	trimmedPre := strings.TrimPrefix(s, "[")
	if len(trimmedPre) == len(s) {
		return nil, cannotConvertToVFloat(s)
	}
	trimmed := strings.TrimRight(trimmedPre, "]")
	if len(trimmed) == len(trimmedPre) {
		return nil, cannotConvertToVFloat(s)
	}
	if len(trimmed) == 0 {
		return []float64{}, nil
	}
	if strings.Index(trimmed, ",") != -1 {
		// Splitting based on comma-separation.
		values := strings.Split(trimmed, ",")
		result := make([]float64, len(values))
		for i := 0; i < len(values); i++ {
			trimmedVal := strings.TrimSpace(values[i])
			val, err := strconv.ParseFloat(trimmedVal, 64)
			if err != nil {
				return nil, cannotConvertToVFloat(s)
			}
			result[i] = val
		}
		return result, nil
	}
	values := strings.Split(trimmed, " ")
	result := make([]float64, 0, len(values))
	for i := 0; i < len(values); i++ {
		if len(values[i]) == 0 {
			// skip if we have an empty string. This can naturally
			// occur if input s was "[1.0     2.0]"
			// notice the extra whitespace in separation!
			continue
		}
		if len(values[i]) > 0 {
			val, err := strconv.ParseFloat(values[i], 64)
			if err != nil {
				return nil, cannotConvertToVFloat(s)
			}
			result = append(result, val)
		}
	}
	return result, nil
}
