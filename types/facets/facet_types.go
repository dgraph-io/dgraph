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
	"bytes"
	"strconv"
	"time"
	"unicode"
)

const (
	Int32ID    = TypeID(Facet_INT32)
	FloatID    = TypeID(Facet_FLOAT)
	BoolID     = TypeID(Facet_BOOL)
	DateTimeID = TypeID(Facet_DATETIME)
	StringID   = TypeID(Facet_STRING)
)

type TypeID Facet_ValType

// ValTypeForTypeID gives Facet_ValType for given TypeID
func ValTypeForTypeID(typId TypeID) Facet_ValType {
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
	panic("Unhandled case in ValTypeForTypeID.")
}

// TypeIDForValType gives TypeID for Facet_ValType
func TypeIDForValType(valType Facet_ValType) TypeID {
	switch valType {
	case Facet_INT32:
		return Int32ID
	case Facet_FLOAT:
		return FloatID
	case Facet_BOOL:
		return BoolID
	case Facet_DATETIME:
		return DateTimeID
	case Facet_STRING:
		return StringID
	}
	panic("Unhandled case in TypeIDForValType.")
}

// ValType gives Facet's TypeID for given facet value str.
func ValType(val string) (Facet_ValType, error) {
	if _, err := strconv.ParseInt(val, 10, 32); err == nil {
		return Facet_INT32, nil
	} else if nume := err.(*strconv.NumError); nume.Err == strconv.ErrRange {
		// check if whole string is only of nums or not.
		// comes here for : 11111111111111111111132333uasfk333 ; see test.
		nonNumChar := false
		for _, v := range val {
			if !unicode.IsDigit(v) {
				nonNumChar = true
				break
			}
		}
		if !nonNumChar {
			return Facet_INT32, err
		}
	}
	if _, err := strconv.ParseFloat(val, 64); err == nil {
		return Facet_FLOAT, nil
	} else if nume := err.(*strconv.NumError); nume.Err == strconv.ErrRange {
		return Facet_FLOAT, err
	}
	if val == "true" || val == "false" {
		return Facet_BOOL, nil
	}
	if _, err := parseTime(val); err == nil {
		return Facet_DATETIME, nil
	}
	return Facet_STRING, nil
}

// FacetFor returns Facet for given key and val.
func FacetFor(key, val string) (*Facet, error) {
	vt, err := ValType(val)
	if err != nil {
		return nil, err
	}
	return &Facet{Key: key, Value: []byte(val), ValType: vt}, nil
}

// Move to types/parse namespace.
func parseTime(val string) (time.Time, error) {
	var t time.Time
	if err := t.UnmarshalText([]byte(val)); err == nil {
		return t, err
	}
	if t, err := time.Parse(dateTimeFormat, val); err == nil {
		return t, err
	}
	return time.Parse(dateFormatYMD, val)
}

// OnlyDate returns whether val has format of only Year-Month-Day
func OnlyDate(val string) bool {
	_, err := time.Parse(dateFormatYMD, val)
	return err == nil
}

const dateFormatYMD = "2006-01-02"
const dateTimeFormat = "2006-01-02T15:04:05"

// SameFacets returns whether two facets are same or not.
// both should be sorted by key.
func SameFacets(a []*Facet, b []*Facet) bool {
	if len(a) != len(b) {
		return false
	}
	la := len(a)
	for i := 0; i < la; i++ {
		if (a[i].Key != b[i].Key) ||
			!bytes.Equal(a[i].Value, b[i].Value) ||
			(a[i].ValType != b[i].ValType) {
			return false
		}
	}
	return true
}
