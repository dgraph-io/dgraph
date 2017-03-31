/*
 * Copyright (C) 2017 Dgraph Labs, Inc. and Contributors
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 */

package facets

import (
	"bytes"
	"sort"
	"strconv"
	"time"
	"unicode"

	"github.com/dgraph-io/dgraph/protos/facetsp"
	"github.com/dgraph-io/dgraph/tok"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
)

// Sorts the facets.
func SortFacets(fs []*facetsp.Facet) {
	sort.Slice(fs, func(i, j int) bool {
		return fs[i].Key < fs[j].Key
	})
}

// CopyFacets makes a copy of facets of the posting which are requested in param.Keys.
func CopyFacets(fcs []*facetsp.Facet, param *facetsp.Param) (fs []*facetsp.Facet) {
	if param == nil || fcs == nil {
		return nil
	}
	// facets and param.keys are both sorted,
	// We also need all keys if param.AllKeys is true.
	numKeys := len(param.Keys)
	numFacets := len(fcs)
	for kidx, fidx := 0, 0; (param.AllKeys || kidx < numKeys) && fidx < numFacets; {
		f := fcs[fidx]
		if param.AllKeys || param.Keys[kidx] == f.Key {
			fcopy := &facetsp.Facet{Key: f.Key, Value: nil, ValType: f.ValType}
			fcopy.Value = make([]byte, len(f.Value))
			copy(fcopy.Value, f.Value)
			fs = append(fs, fcopy)
			kidx++
			fidx++
		} else if f.Key > param.Keys[kidx] {
			kidx++
		} else {
			fidx++
		}
	}
	return fs
}

// valAndValType returns interface val and valtype for facet.
func valAndValType(val string) (interface{}, facetsp.Facet_ValType, error) {
	if len(val) == 0 { // empty string case
		return "", facetsp.Facet_STRING, nil
	}
	// strings should be in quotes.
	if len(val) >= 2 && val[0] == '"' && val[len(val)-1] == '"' {
		return val[1 : len(val)-1], facetsp.Facet_STRING, nil
	}
	if intVal, err := strconv.ParseInt(val, 0, 64); err == nil {
		return int64(intVal), facetsp.Facet_INT, nil
	} else if numErr := err.(*strconv.NumError); numErr.Err == strconv.ErrRange {
		// if we have only digits in val, then val is a big integer : return error
		// otherwise try to parse as float.
		allNumChars := true
		for _, v := range val {
			if !unicode.IsDigit(v) {
				allNumChars = false
				break
			}
		}
		if allNumChars {
			return nil, facetsp.Facet_INT, err
		}
	}
	if floatVal, err := strconv.ParseFloat(val, 64); err == nil {
		return floatVal, facetsp.Facet_FLOAT, nil
	} else if numErr := err.(*strconv.NumError); numErr.Err == strconv.ErrRange {
		return nil, facetsp.Facet_FLOAT, err
	}
	if val == "true" || val == "false" {
		return val == "true", facetsp.Facet_BOOL, nil
	}
	if t, err := parseTime(val); err == nil {
		return t, facetsp.Facet_DATETIME, nil
	}
	return nil, facetsp.Facet_STRING, x.Errorf("Could not parse the facet value : [%s]", val)
}

// FacetFor returns Facet for given key and val.
func FacetFor(key, val string) (*facetsp.Facet, error) {
	v, vt, err := valAndValType(val)
	if err != nil {
		return nil, err
	}

	// convert facet val interface{} to binary
	tid := TypeIDFor(&facetsp.Facet{ValType: vt})
	fVal := &types.Val{Tid: types.BinaryID}
	if err = types.Marshal(types.Val{Tid: tid, Value: v}, fVal); err != nil {
		return nil, err
	}

	fval, ok := fVal.Value.([]byte)
	if !ok {
		return nil, x.Errorf("Error while marshalling types.Val into binary.")
	}
	res := &facetsp.Facet{Key: key, Value: fval, ValType: vt}
	if vt == facetsp.Facet_STRING {
		// tokenize val.
		res.Tokens, err = tok.GetTokens([]string{val})
		if err == nil {
			sort.Strings(res.Tokens)
		}
	}
	return res, err
}

// SameFacets returns whether two facets are same or not.
// both should be sorted by key.
func SameFacets(a []*facetsp.Facet, b []*facetsp.Facet) bool {
	if len(a) != len(b) {
		return false
	}
	la := len(a)
	for i := 0; i < la; i++ {
		if (a[i].ValType != b[i].ValType) ||
			(a[i].Key != b[i].Key) ||
			!bytes.Equal(a[i].Value, b[i].Value) {
			return false
		}
	}
	return true
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

const dateFormatYMD = "2006-01-02"
const dateTimeFormat = "2006-01-02T15:04:05"

// TypeIDFor gives TypeID for facet.
func TypeIDFor(f *facetsp.Facet) types.TypeID {
	switch TypeIDForValType(f.ValType) {
	case IntID:
		return types.IntID
	case StringID:
		return types.StringID
	case BoolID:
		return types.BoolID
	case DateTimeID:
		return types.DateTimeID
	case FloatID:
		return types.FloatID
	default:
		panic("unhandled case in facetValToTypeVal")
	}
}

// ValFor converts Facet into types.Val.
func ValFor(f *facetsp.Facet) types.Val {
	val := types.Val{Tid: types.BinaryID, Value: f.Value}
	typId := TypeIDFor(f)
	v, err := types.Convert(val, typId)
	x.AssertTruef(err == nil,
		"We should always be able to covert facet into val. %v %v", f.Value, typId)
	return v
}
