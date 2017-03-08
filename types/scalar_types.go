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

	"github.com/dgraph-io/dgraph/protos/typesp"
	geom "github.com/twpayne/go-geom"
)

const (
	nanoSecondsInSec = 1000000000
)

// Note: These ids are stored in the posting lists to indicate the type
// of the data. The order *cannot* be changed without breaking existing
// data. When adding a new type *always* add to the end of this list.
// Never delete anything from this list even if it becomes unused.
const (
	BinaryID   = TypeID(typesp.Posting_BINARY)
	Int32ID    = TypeID(typesp.Posting_INT32)
	FloatID    = TypeID(typesp.Posting_FLOAT)
	BoolID     = TypeID(typesp.Posting_BOOL)
	DateTimeID = TypeID(typesp.Posting_DATETIME)
	StringID   = TypeID(typesp.Posting_STRING)
	DateID     = TypeID(typesp.Posting_DATE)
	GeoID      = TypeID(typesp.Posting_GEO)
	UidID      = TypeID(typesp.Posting_UID)
	PasswordID = TypeID(typesp.Posting_PASSWORD)
	DefaultID  = TypeID(typesp.Posting_DEFAULT)
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
	"uid":      UidID,
	"password": PasswordID,
	"default":  DefaultID,
}

type TypeID typesp.Posting_ValType

func (t TypeID) Name() string {
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
	case UidID:
		return "uid"
	case PasswordID:
		return "password"
	case DefaultID:
		return "default"
	}
	return ""
}

// Val is a value with type information.
type Val struct {
	Tid   TypeID
	Value interface{}
}

// TypeForName returns the type corresponding to the given name.
// If name is not recognized, it returns nil.
func TypeForName(name string) (TypeID, bool) {
	t, ok := typeNameMap[name]
	return t, ok
}

func (t TypeID) IsScalar() bool {
	return t != UidID
}

// ValueForType returns the zero value for a type id
func ValueForType(id TypeID) Val {
	switch id {
	case BinaryID:
		var b []byte
		return Val{BinaryID, &b}

	case Int32ID:
		var i int32
		return Val{Int32ID, &i}

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

	case DateID:
		var d time.Time
		return Val{DateID, &d}

	case GeoID:
		var g geom.T
		return Val{GeoID, &g}

	case UidID:
		var i uint64
		return Val{UidID, &i}

	case PasswordID:
		var p string
		return Val{PasswordID, p}

	default:
		return Val{}
	}
}

func createDate(y int, m time.Month, d int) time.Time {
	var dt time.Time
	dt = time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	return dt
}

const dateFormatYMD = "2006-01-02"
const dateFormatYM = "2006-01"
const dateFormatY = "2006"
const dateTimeFormat = "2006-01-02T15:04:05"
