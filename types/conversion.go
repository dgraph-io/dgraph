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
	"bytes"
	"encoding/binary"
	"encoding/json"
	"math"
	"strconv"
	"strings"
	"time"
	"unsafe"

	"github.com/pkg/errors"
	geom "github.com/twpayne/go-geom"
	"github.com/twpayne/go-geom/encoding/geojson"
	"github.com/twpayne/go-geom/encoding/wkb"

	"github.com/dgraph-io/dgo/v230/protos/api"
)

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
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\t", " ")
	s = strings.TrimSpace(s)
	if len(s) == 0 {
		return []float64{}, nil
	}
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

func cannotConvertToVFloat(s string) error {
	return errors.Errorf("Cannot convert %s to vfloat", s)
}

// Convert converts the value to given scalar type.
func Convert(from Val, toID TypeID) (Val, error) {
	to := Val{Tid: toID}

	// sanity: we expect a value
	data, ok := from.Value.([]byte)
	if !ok {
		return to, errors.Errorf("Invalid data to convert to %s", toID.Name())
	}

	fromID := from.Tid
	res := &to.Value

	// Convert from-type to to-type and store in the result interface.
	switch fromID {
	case BinaryID:
		{
			// Unmarshal from Binary to type interfaces.
			switch toID {
			case BinaryID:
				*res = data
			case StringID, DefaultID:
				// We never modify from Val, so this should be safe.
				*res = *(*string)(unsafe.Pointer(&data))
			case IntID:
				if len(data) < 8 {
					return to, errors.Errorf("Invalid data for int64 %v", data)
				}
				*res = int64(binary.LittleEndian.Uint64(data))
			case FloatID:
				if len(data) < 8 {
					return to, errors.Errorf("Invalid data for float %v", data)
				}
				i := binary.LittleEndian.Uint64(data)
				*res = math.Float64frombits(i)
			case BoolID:
				if len(data) == 0 || data[0] == 0 {
					*res = false
					return to, nil
				} else if data[0] == 1 {
					*res = true
					return to, nil
				}
				return to, errors.Errorf("Invalid value for bool %v", data[0])
			case DateTimeID:
				var t time.Time
				if err := t.UnmarshalBinary(data); err != nil {
					return to, err
				}
				*res = t
			case GeoID:
				w, err := wkb.Unmarshal(data)
				if err != nil {
					return to, err
				}
				*res = w
			case PasswordID:
				*res = string(data)
			case VFloatID:
				if len(data)%8 != 0 {
					return to, errors.Errorf("Invalid data for vector of floats: %v", data)
				}
				*res = BytesAsFloatArray(data)
			default:
				return to, cantConvert(fromID, toID)
			}
		}
	case StringID, DefaultID:
		{
			vc := string(data)
			switch toID {
			case BinaryID:
				*res = []byte(vc)
			case IntID:
				val, err := strconv.ParseInt(vc, 10, 64)
				if err != nil {
					return to, err
				}
				*res = val
			case FloatID:
				val, err := strconv.ParseFloat(vc, 64)
				if err != nil {
					return to, err
				}
				if math.IsNaN(val) {
					return to, errors.Errorf("Got invalid value: NaN")
				}
				*res = val
			case StringID, DefaultID:
				*res = vc
			case BoolID:
				val, err := strconv.ParseBool(vc)
				if err != nil {
					return to, err
				}
				*res = val
			case DateTimeID:
				t, err := ParseTime(vc)
				if err != nil {
					return to, err
				}
				*res = t
			case GeoID:
				var g geom.T
				text := bytes.Replace([]byte(vc), []byte("'"), []byte("\""), -1)
				if err := geojson.Unmarshal(text, &g); err != nil {
					return to,
						errors.Wrapf(err, "Error while unmarshalling: [%s] as geojson", vc)
				}
				*res = g
			case PasswordID:
				p, err := Encrypt(vc)
				if err != nil {
					return to, err
				}
				*res = p
			case VFloatID:
				vf, err := ParseVFloat(vc)
				if err != nil {
					return to, err
				}
				*res = vf
			default:
				return to, cantConvert(fromID, toID)
			}
		}
	case IntID:
		{
			if len(data) < 8 {
				return to, errors.Errorf("Invalid data for int64 %v", data)
			}
			vc := int64(binary.LittleEndian.Uint64(data))
			switch toID {
			case IntID:
				*res = vc
			case BinaryID:
				var bs [8]byte
				binary.LittleEndian.PutUint64(bs[:], uint64(vc))
				*res = bs[:]
			case FloatID:
				*res = float64(vc)
			case BoolID:
				*res = vc != 0
			case StringID, DefaultID:
				*res = strconv.FormatInt(vc, 10)
			case DateTimeID:
				*res = time.Unix(vc, 0).UTC()
			case VFloatID:
				*res = []float64{float64(vc)}
			default:
				return to, cantConvert(fromID, toID)
			}
		}
	case FloatID:
		{
			if len(data) < 8 {
				return to, errors.Errorf("Invalid data for float %v", data)
			}
			i := binary.LittleEndian.Uint64(data)
			vc := math.Float64frombits(i)
			switch toID {
			case FloatID:
				*res = vc
			case BinaryID:
				var bs [8]byte
				u := math.Float64bits(vc)
				binary.LittleEndian.PutUint64(bs[:], u)
				*res = bs[:]
			case IntID:
				if vc > math.MaxInt64 || vc < math.MinInt64 || math.IsNaN(vc) {
					return to, errors.Errorf("Float out of int64 range")
				}
				*res = int64(vc)
			case BoolID:
				*res = vc != 0
			case StringID, DefaultID:
				*res = strconv.FormatFloat(vc, 'G', -1, 64)
			case DateTimeID:
				secs := int64(vc)
				fracSecs := vc - float64(secs)
				nsecs := int64(fracSecs * nanoSecondsInSec)
				*res = time.Unix(secs, nsecs).UTC()
			case VFloatID:
				*res = []float64{vc}
			default:
				return to, cantConvert(fromID, toID)
			}
		}
	case BoolID:
		{
			var vc bool
			if len(data) == 0 || data[0] > 1 {
				return to, errors.Errorf("Invalid value for bool %v", data)
			}
			vc = data[0] == 1

			switch toID {
			case BoolID:
				*res = vc
			case BinaryID:
				*res = []byte{0}
				if vc {
					*res = []byte{1}
				}
			case IntID:
				*res = int64(0)
				if vc {
					*res = int64(1)
				}
			case FloatID:
				*res = float64(0)
				if vc {
					*res = float64(1)
				}
			case VFloatID:
				asFloat := float64(0)
				if vc {
					asFloat = float64(1)
				}
				*res = []float64{asFloat}
			case StringID, DefaultID:
				*res = strconv.FormatBool(vc)
			default:
				return to, cantConvert(fromID, toID)
			}
		}
	case DateTimeID:
		{
			var t time.Time
			if err := t.UnmarshalBinary(data); err != nil {
				return to, err
			}
			switch toID {
			case DateTimeID:
				*res = t
			case BinaryID:
				r, err := t.MarshalBinary()
				if err != nil {
					return to, err
				}
				*res = r
			case StringID, DefaultID:
				val, err := t.MarshalText()
				if err != nil {
					return to, err
				}
				*res = string(val)
			case IntID:
				*res = t.Unix()
			case FloatID:
				*res = float64(t.UnixNano()) / float64(nanoSecondsInSec)
			default:
				return to, cantConvert(fromID, toID)
			}
		}
	case GeoID:
		{
			vc, err := wkb.Unmarshal(data)
			if err != nil {
				return to, err
			}
			switch toID {
			case GeoID:
				*res = vc
			case BinaryID:
				r, err := wkb.Marshal(vc, binary.LittleEndian)
				if err != nil {
					return to, err
				}
				*res = r
			case StringID, DefaultID:
				val, err := geojson.Marshal(vc)
				if err != nil {
					return to, nil
				}
				*res = string(bytes.Replace(val, []byte("\""), []byte("'"), -1))
			default:
				return to, cantConvert(fromID, toID)
			}
		}
	case PasswordID:
		{
			vc := string(data)
			switch toID {
			case BinaryID:
				*res = []byte(vc)
			case StringID, PasswordID:
				*res = vc
			default:
				return to, cantConvert(fromID, toID)
			}
		}
	case VFloatID:
		{
			// Note that we avoid invoking BytesAsFloatArray up front
			// because we don't want to pay the performance cost for it
			// if we are ultimately converting to BinaryID.
			// This kind of breaks the pattern that we established in other
			// branches, but we avoid wasting time.
			switch toID {
			case BinaryID:
				*res = data
			case VFloatID:
				vc := BytesAsFloatArray(data)
				*res = vc
			case StringID:
				vc := BytesAsFloatArray(data)
				sa := FloatArrayAsString(vc)
				*res = sa
			default:
				return to, cantConvert(fromID, toID)
			}
		}
	default:
		return to, cantConvert(fromID, toID)
	}
	return to, nil
}

func Marshal(from Val, to *Val) error {
	if to == nil {
		return errors.Errorf("Invalid conversion %s to nil", from.Tid.Name())
	}

	fromID := from.Tid
	toID := to.Tid
	val := from.Value
	res := &to.Value

	// This is a default value from sg.fillVars, don't convert it's empty.
	// Fixes issue #2980.
	if val == nil {
		*to = ValueForType(toID)
		return nil
	}

	switch fromID {
	case BinaryID:
		vc := val.([]byte)
		switch toID {
		case StringID, DefaultID:
			*res = string(vc)
		case BinaryID:
			*res = vc
		default:
			return cantConvert(fromID, toID)
		}
	case StringID, DefaultID:
		vc := val.(string)
		switch toID {
		case StringID, DefaultID:
			*res = vc
		case BinaryID:
			*res = []byte(vc)
		default:
			return cantConvert(fromID, toID)
		}
	case IntID:
		vc := val.(int64)
		switch toID {
		case StringID, DefaultID:
			*res = strconv.FormatInt(vc, 10)
		case BinaryID:
			var bs [8]byte
			binary.LittleEndian.PutUint64(bs[:], uint64(vc))
			*res = bs[:]
		default:
			return cantConvert(fromID, toID)
		}
	case FloatID:
		vc := val.(float64)
		switch toID {
		case StringID, DefaultID:
			*res = strconv.FormatFloat(vc, 'G', -1, 64)
		case BinaryID:
			var bs [8]byte
			u := math.Float64bits(vc)
			binary.LittleEndian.PutUint64(bs[:], u)
			*res = bs[:]
		default:
			return cantConvert(fromID, toID)
		}
	case BoolID:
		vc := val.(bool)
		switch toID {
		case StringID, DefaultID:
			*res = strconv.FormatBool(vc)
		case BinaryID:
			*res = []byte{0}
			if vc {
				*res = []byte{1}
			}
		default:
			return cantConvert(fromID, toID)
		}
	case DateTimeID:
		vc := val.(time.Time)
		switch toID {
		case StringID, DefaultID:
			val, err := vc.MarshalText()
			if err != nil {
				return err
			}
			*res = string(val)
		case BinaryID:
			r, err := vc.MarshalBinary()
			if err != nil {
				return err
			}
			*res = r
		default:
			return cantConvert(fromID, toID)
		}
	case GeoID:
		vc, ok := val.(geom.T)
		if !ok {
			return errors.Errorf("Expected a Geo type")
		}
		switch toID {
		case BinaryID:
			r, err := wkb.Marshal(vc, binary.LittleEndian)
			if err != nil {
				return err
			}
			*res = r
		case StringID, DefaultID:
			val, err := geojson.Marshal(vc)
			if err != nil {
				return nil
			}
			*res = string(bytes.Replace(val, []byte("\""), []byte("'"), -1))
		default:
			return cantConvert(fromID, toID)
		}
	case PasswordID:
		vc := val.(string)
		switch toID {
		case StringID:
			*res = vc
		case BinaryID:
			*res = []byte(vc)
		default:
			return cantConvert(fromID, toID)
		}
	case VFloatID:
		vc := val.([]float64)
		switch toID {
		case BinaryID:
			*res = FloatArrayAsBytes(vc)
		case StringID:
			*res = FloatArrayAsString(vc)
		default:
			return cantConvert(fromID, toID)
		}
	default:
		return cantConvert(fromID, toID)
	}
	return nil
}

// ObjectValue converts into api.Value.
func ObjectValue(id TypeID, value interface{}) (*api.Value, error) {
	def := &api.Value{Val: &api.Value_StrVal{StrVal: ""}}
	var ok bool
	// Lets set the object value according to the storage type.
	switch id {
	case StringID:
		var v string
		if v, ok = value.(string); !ok {
			return def, errors.Errorf("Expected value of type string. Got : %v", value)
		}
		return &api.Value{Val: &api.Value_StrVal{StrVal: v}}, nil
	case DefaultID:
		var v string
		if v, ok = value.(string); !ok {
			return def, errors.Errorf("Expected value of type string. Got : %v", value)
		}
		return &api.Value{Val: &api.Value_DefaultVal{DefaultVal: v}}, nil
	case IntID:
		var v int64
		if v, ok = value.(int64); !ok {
			return def, errors.Errorf("Expected value of type int64. Got : %v", value)
		}
		return &api.Value{Val: &api.Value_IntVal{IntVal: v}}, nil
	case FloatID:
		var v float64
		if v, ok = value.(float64); !ok {
			return def, errors.Errorf("Expected value of type float64. Got : %v", value)
		}
		return &api.Value{Val: &api.Value_DoubleVal{DoubleVal: v}}, nil
	case BoolID:
		var v bool
		if v, ok = value.(bool); !ok {
			return def, errors.Errorf("Expected value of type bool. Got : %v", value)
		}
		return &api.Value{Val: &api.Value_BoolVal{BoolVal: v}}, nil
	case BinaryID:
		var v []byte
		if v, ok = value.([]byte); !ok {
			return def, errors.Errorf("Expected value of type []byte. Got : %v", value)
		}
		return &api.Value{Val: &api.Value_BytesVal{BytesVal: v}}, nil
	// Geo and datetime are stored in binary format in the N-Quad, so lets
	// convert them here.
	case GeoID:
		b, err := toBinary(id, value)
		if err != nil {
			return def, err
		}
		return &api.Value{Val: &api.Value_GeoVal{GeoVal: b}}, nil
	case DateTimeID:
		b, err := toBinary(id, value)
		if err != nil {
			return def, err
		}
		return &api.Value{Val: &api.Value_DatetimeVal{DatetimeVal: b}}, nil
	case PasswordID:
		var v string
		if v, ok = value.(string); !ok {
			return def, errors.Errorf("Expected value of type password. Got : %v", value)
		}
		return &api.Value{Val: &api.Value_PasswordVal{PasswordVal: v}}, nil
	case VFloatID:
		vf, err := toBinary(id, value)
		if err != nil {
			return def, err
		}
		return &api.Value{Val: &api.Value_VdoubleVal{VdoubleVal: vf}}, nil
	default:
		return def, errors.Errorf("ObjectValue not available for: %v", id)
	}
}

func toBinary(id TypeID, b interface{}) ([]byte, error) {
	p1 := ValueForType(BinaryID)
	if err := Marshal(Val{id, b}, &p1); err != nil {
		return nil, err
	}
	return p1.Value.([]byte), nil
}

func cantConvert(from TypeID, to TypeID) error {
	return errors.Errorf("Cannot convert %s to type %s", from.Name(), to.Name())
}

// MarshalJSON makes Val satisfy the json.Marshaler interface.
func (v Val) MarshalJSON() ([]byte, error) {
	switch v.Tid {
	case IntID:
		return json.Marshal(v.Value.(int64))
	case BoolID:
		return json.Marshal(v.Value.(bool))
	case FloatID:
		return json.Marshal(v.Value.(float64))
	case DateTimeID:
		return json.Marshal(v.Value.(time.Time))
	case GeoID:
		return geojson.Marshal(v.Value.(geom.T))
	case StringID, DefaultID:
		return json.Marshal(v.Safe().(string))
	case PasswordID:
		return json.Marshal(v.Value.(string))
	}
	return nil, errors.Errorf("Invalid type for MarshalJSON: %v", v.Tid)
}
