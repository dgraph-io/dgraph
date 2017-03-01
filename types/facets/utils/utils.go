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

package utils

import (
	"bytes"
	"sort"
	"strconv"
	"time"
	"unicode"

	"github.com/dgraph-io/dgraph/tok"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/types/facets"
	"github.com/dgraph-io/dgraph/x"
)

// Sorts the facets.
func SortFacets(fs []*facets.Facet) {
	sort.Sort(facets.Facets{fs})
}

// CopyFacets makes a copy of facets of the posting which are requested in param.Keys.
func CopyFacets(fcs []*facets.Facet, param *facets.Param) (fs []*facets.Facet) {
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
			fcopy := &facets.Facet{Key: f.Key, Value: nil, ValType: f.ValType}
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

// FacetFor returns Facet for given key and val.
func FacetFor(key, val string) (*facets.Facet, error) {
	v, vt, err := valAndValType(val)
	if err != nil {
		return nil, err
	}

	// convert facet val interface{} to binary
	tid := types.TypeIDFor(&facets.Facet{ValType: vt})
	fVal := &types.Val{Tid: types.BinaryID}
	if err = types.Marshal(types.Val{Tid: tid, Value: v}, fVal); err != nil {
		return nil, err
	}

	fval, ok := fVal.Value.([]byte)
	if !ok {
		return nil, x.Errorf("Error while marshalling types.Val into binary.")
	}
	res := &facets.Facet{Key: key, Value: fval, ValType: vt}
	if vt == facets.Facet_STRING {
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
func SameFacets(a []*facets.Facet, b []*facets.Facet) bool {
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

// valAndValType returns interface val and valtype for facet.
func valAndValType(val string) (interface{}, facets.Facet_ValType, error) {
	if fint, err := strconv.ParseInt(val, 10, 32); err == nil {
		return int32(fint), facets.Facet_INT32, nil
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
		if !nonNumChar { // return error
			return nil, facets.Facet_INT32, err
		}
	}
	if ffloat, err := strconv.ParseFloat(val, 64); err == nil {
		return ffloat, facets.Facet_FLOAT, nil
	} else if nume := err.(*strconv.NumError); nume.Err == strconv.ErrRange {
		return nil, facets.Facet_FLOAT, err
	}
	if val == "true" || val == "false" {
		return val == "true", facets.Facet_BOOL, nil
	}
	if t, err := parseTime(val); err == nil {
		return t, facets.Facet_DATETIME, nil
	}
	return val, facets.Facet_STRING, nil
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
