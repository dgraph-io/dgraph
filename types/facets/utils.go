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
	// TODO(ashish) : strings should be in quotes.. \"\"
	// No need to check for nonNumChar then.
	if intVal, err := strconv.ParseInt(val, 0, 32); err == nil {
		return int32(intVal), facetsp.Facet_INT32, nil
	} else if numErr := err.(*strconv.NumError); numErr.Err == strconv.ErrRange {
		// check if whole string is only of nums or not.
		// comes here for : 11111111111111111111132333uasfk333 ; see test.
		nonNumChar := false
		for _, v := range val {
			if !unicode.IsDigit(v) {
				nonNumChar = true
				break
			}
		}
		if !nonNumChar { // return error
			return nil, facetsp.Facet_INT32, err
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
	return val, facetsp.Facet_STRING, nil
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
	case Int32ID:
		return types.Int32ID
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
