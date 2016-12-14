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
	"time"

	stype "github.com/dgraph-io/dgraph/posting/types"
	geom "github.com/twpayne/go-geom"
)

// Note: These ids are stored in the posting lists to indicate the type
// of the data. The order *cannot* be changed without breaking existing
// data. When adding a new type *always* add to the end of this list.
// Never delete anything from this list even if it becomes unused.
const (
	BinaryID   = TypeID(stype.Posting_BINARY)
	Int32ID    = TypeID(stype.Posting_INT32)
	FloatID    = TypeID(stype.Posting_FLOAT)
	BoolID     = TypeID(stype.Posting_BOOL)
	DateTimeID = TypeID(stype.Posting_DATETIME)
	StringID   = TypeID(stype.Posting_STRING)
	DateID     = TypeID(stype.Posting_DATE)
	GeoID      = TypeID(stype.Posting_GEO)
)

var typeNameMap = map[string]TypeID{
	"int":      Int32ID,
	"float":    FloatID,
	"string":   StringID,
	"bool":     BoolID,
	"id":       StringID,
	"datetime": DateTimeID,
	"date":     DateID,
	"geo":      GeoID,
}

type TypeID stype.Posting_ValType

func (t TypeID) Name() String {
	switch t {
	case Int32ID:
		return "int"
	case FloatID:
		return "float"
	case BoolID:
		return "bool"
	case StringID:
		return "string"
	case DateID:
		return "date"
	case DateTimeID:
		return "dateTime"
	case GeoID:
		return "geo"
	}
	return ""
}

// TypeForName returns the type corresponding to the given name.
// If name is not recognized, it returns nil.
func TypeForName(name string) (TypeID, bool) {
	t, ok := typeNameMap[name]
	return t, ok
}

// ValueForType returns the zero value for a type id
func ValueForType(id TypeID) interface{} {
	switch id {
	case BinaryID:
		var b Binary
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

type Binary []byte

// Int32 is the scalar type for int32
type Int32 int32

// Float is the scalar type for float64
type Float float64

// String is the scalar type for string
type String string

// Bool is the scalar type for bool
type Bool bool

// Time wraps time.Time to add the Value interface
type Time struct {
	time.Time
}

// Geo represents geo-spatial data.
type Geo struct {
	geom.T
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
