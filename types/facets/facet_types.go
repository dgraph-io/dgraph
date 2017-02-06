/*
 * Copyright 2017 Dgraph Labs, Inc.
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

package facets

import (
	"strconv"
	"time"
)

const (
	Int32ID    = TypeID(Facet_INT32)
	FloatID    = TypeID(Facet_FLOAT)
	BoolID     = TypeID(Facet_BOOL)
	DateTimeID = TypeID(Facet_DATETIME)
	StringID   = TypeID(Facet_STRING)
)

type TypeID Facet_ValType

// TypeIDToValType gives Facet_ValType for given TypeID
func TypeIDToValType(typId TypeID) Facet_ValType {
	switch typId {
	case Int32ID:
		return Facet_INT32
	case FloatID:
		return Facet_FLOAT
	case BoolID:
		return Facet_BOOL
	case DateTimeID:
		return Facet_DATETIME
	case StringID:
		return Facet_STRING
	}
	panic("Unhandled case in TypeIDToValType.")
}

// ValStrToTypeID gives Facet's TypeID for given facet value
func ValStrToValType(val string) Facet_ValType {
	if _, err := strconv.ParseBool(val); err == nil {
		return Facet_BOOL
	}
	if _, err := strconv.ParseInt(val, 0, 32); err == nil {
		return Facet_INT32
	}
	if _, err := strconv.ParseFloat(val, 64); err == nil {
		return Facet_FLOAT
	}
	if _, err := time.Parse(dateTimeFormat, val); err != nil {
		if _, err = time.Parse(dateFormatYMD, val); err == nil {
			return Facet_DATETIME
		}
	} else {
		return Facet_DATETIME
	}
	return Facet_STRING
}

const dateFormatYMD = "2006-01-02"
const dateTimeFormat = "2006-01-02T15:04:05"
