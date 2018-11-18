/*
 * Copyright 2017-2018 Dgraph Labs, Inc. and Contributors
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

package facets

import (
	"bytes"
	"fmt"
	"math"
	"sort"
	"strconv"
	"unicode"

	"github.com/dgraph-io/dgo/protos/api"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/tok"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
)

// Sorts And validates the facets.
func SortAndValidate(fs []*api.Facet) error {
	if len(fs) == 0 {
		return nil
	}
	sort.Slice(fs, func(i, j int) bool {
		return fs[i].Key < fs[j].Key
	})
	for i := 1; i < len(fs); i++ {
		if fs[i-1].Key == fs[i].Key {
			return x.Errorf("Repeated keys are not allowed in facets. But got %s",
				fs[i].Key)
		}
	}
	return nil
}

// CopyFacets makes a copy of facets of the posting which are requested in param.Keys.
func CopyFacets(fcs []*api.Facet, param *pb.FacetParams) (fs []*api.Facet) {
	if param == nil || fcs == nil {
		return nil
	}
	// facets and param.keys are both sorted,
	// We also need all keys if param.AllKeys is true.
	numKeys := len(param.Param)
	numFacets := len(fcs)
	for kidx, fidx := 0, 0; (param.AllKeys || kidx < numKeys) && fidx < numFacets; {
		f := fcs[fidx]
		if param.AllKeys || param.Param[kidx].Key == f.Key {
			fcopy := &api.Facet{
				Key:     f.Key,
				Value:   nil,
				ValType: f.ValType,
			}
			if !param.AllKeys {
				fcopy.Alias = param.Param[kidx].Alias
			}
			fcopy.Value = make([]byte, len(f.Value))
			copy(fcopy.Value, f.Value)
			fs = append(fs, fcopy)
			kidx++
			fidx++
		} else if f.Key > param.Param[kidx].Key {
			kidx++
		} else {
			fidx++
		}
	}
	return fs
}

// valAndValType returns interface val and valtype for facet.
func valAndValType(val string) (interface{}, api.Facet_ValType, error) {
	if len(val) == 0 { // empty string case
		return "", api.Facet_STRING, nil
	}
	// strings should be in quotes.
	if len(val) >= 2 && val[0] == '"' && val[len(val)-1] == '"' {
		uq, err := strconv.Unquote(val)
		return uq, api.Facet_STRING, x.Wrapf(err, "could not unquote %q:", val)
	}
	if intVal, err := strconv.ParseInt(val, 0, 64); err == nil {
		return int64(intVal), api.Facet_INT, nil
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
			return nil, api.Facet_INT, err
		}
	}
	if floatVal, err := strconv.ParseFloat(val, 64); err == nil {
		// We can't store NaN as it is because it serializes into invalid JSON.
		if math.IsNaN(floatVal) {
			return nil, api.Facet_FLOAT, fmt.Errorf("Got invalid value: NaN.")
		}

		return floatVal, api.Facet_FLOAT, nil
	} else if numErr := err.(*strconv.NumError); numErr.Err == strconv.ErrRange {
		return nil, api.Facet_FLOAT, err
	}
	if val == "true" || val == "false" {
		return val == "true", api.Facet_BOOL, nil
	}
	if t, err := types.ParseTime(val); err == nil {
		return t, api.Facet_DATETIME, nil
	}
	return nil, api.Facet_STRING, x.Errorf("Could not parse the facet value : [%s]", val)
}

// FacetFor returns Facet for given key and val.
func FacetFor(key, val string) (*api.Facet, error) {
	v, vt, err := valAndValType(val)
	if err != nil {
		return nil, err
	}

	// convert facet val interface{} to binary
	tid := TypeIDFor(&api.Facet{ValType: vt})
	fVal := &types.Val{Tid: types.BinaryID}
	if err = types.Marshal(types.Val{Tid: tid, Value: v}, fVal); err != nil {
		return nil, err
	}

	fval, ok := fVal.Value.([]byte)
	if !ok {
		return nil, x.Errorf("Error while marshalling types.Val into binary.")
	}
	res := &api.Facet{Key: key, Value: fval, ValType: vt}
	if vt == api.Facet_STRING {
		// tokenize val.
		res.Tokens, err = tok.GetTermTokens([]string{v.(string)})
		if err == nil {
			sort.Strings(res.Tokens)
		}
	}
	return res, err
}

// SameFacets returns whether two facets are same or not.
// both should be sorted by key.
func SameFacets(a []*api.Facet, b []*api.Facet) bool {
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

// TypeIDFor gives TypeID for facet.
func TypeIDFor(f *api.Facet) types.TypeID {
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

// TryValFor tries to convert the facet to the its type from binary format. We use it to validate
// the facets set directly by the user during mutation.
func TryValFor(f *api.Facet) error {
	val := types.Val{Tid: types.BinaryID, Value: f.Value}
	typId := TypeIDFor(f)
	_, err := types.Convert(val, typId)
	return x.Wrapf(err, "Error while parsing facet: [%v]", f)
}

// ValFor converts Facet into types.Val.
func ValFor(f *api.Facet) types.Val {
	val := types.Val{Tid: types.BinaryID, Value: f.Value}
	typId := TypeIDFor(f)
	v, err := types.Convert(val, typId)
	x.AssertTruef(err == nil,
		"We should always be able to covert facet into val. %v %v", f.Value, typId)
	return v
}
