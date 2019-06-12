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
	"fmt"
	"math"
	"sort"
	"strconv"
	"unicode"

	"github.com/dgraph-io/dgo/protos/api"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/tok"
	"github.com/dgraph-io/dgraph/types"
	"github.com/pkg/errors"
)

// SortAndValidate sorts And validates the facets.
func SortAndValidate(fs []*api.Facet) error {
	if len(fs) == 0 {
		return nil
	}
	sort.Slice(fs, func(i, j int) bool {
		return fs[i].Key < fs[j].Key
	})
	for i := 1; i < len(fs); i++ {
		if fs[i-1].Key == fs[i].Key {
			return errors.Errorf("Repeated keys are not allowed in facets. But got %s",
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
		return uq, api.Facet_STRING, errors.Wrapf(err, "could not unquote %q:", val)
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
			return nil, api.Facet_FLOAT, fmt.Errorf("Got invalid value: NaN")
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
	return nil, api.Facet_STRING, errors.Errorf("Could not parse the facet value : [%s]", val)
}

// FacetFor returns Facet for given key and val.
func FacetFor(key, val string) (*api.Facet, error) {
	v, vt, err := valAndValType(val)
	if err != nil {
		return nil, err
	}

	facet, err := ToBinary(key, v, vt)
	if err != nil {
		return nil, err
	}

	if vt == api.Facet_STRING {
		// tokenize val.
		facet.Tokens, err = tok.GetTermTokens([]string{v.(string)})
		if err == nil {
			sort.Strings(facet.Tokens)
		}
	}
	return facet, err
}

// ToBinary converts the given value into a binary value.
func ToBinary(key string, value interface{}, sourceType api.Facet_ValType) (
	*api.Facet, error) {
	// convert facet val interface{} to binary
	sourceTid, err := TypeIDFor(&api.Facet{ValType: sourceType})
	if err != nil {
		return nil, err
	}

	targetVal := &types.Val{Tid: types.BinaryID}
	if err = types.Marshal(types.Val{Tid: sourceTid, Value: value}, targetVal); err != nil {
		return nil, err
	}

	return &api.Facet{Key: key, Value: targetVal.Value.([]byte), ValType: sourceType}, nil
}

// TypeIDFor gives TypeID for facet.
func TypeIDFor(f *api.Facet) (types.TypeID, error) {
	switch f.ValType {
	case api.Facet_INT:
		return types.IntID, nil
	case api.Facet_FLOAT:
		return types.FloatID, nil
	case api.Facet_BOOL:
		return types.BoolID, nil
	case api.Facet_DATETIME:
		return types.DateTimeID, nil
	case api.Facet_STRING:
		return types.StringID, nil
	default:
		return types.DefaultID, fmt.Errorf("Unrecognized facet type: %v", f.ValType)
	}
}

// ValFor converts Facet into types.Val.
func ValFor(f *api.Facet) (types.Val, error) {
	val := types.Val{Tid: types.BinaryID, Value: f.Value}
	facetTid, err := TypeIDFor(f)
	if err != nil {
		return types.Val{}, err
	}

	return types.Convert(val, facetTid)
}
