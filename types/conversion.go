/*
 * Copyright (C) 2017 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *    http://www.apache.org/licenses/LICENSE-2.0
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
	"fmt"
	"math"
	"strconv"
	"time"

	geom "github.com/twpayne/go-geom"
	"github.com/twpayne/go-geom/encoding/geojson"
	"github.com/twpayne/go-geom/encoding/wkb"

	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/x"
)

// Convert converts the value to given scalar type.
func Convert(from Val, toID TypeID) (Val, error) {
	to := ValueForType(toID)
	fromID := from.Tid
	data := from.Value.([]byte)
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
				*res = string(data)
			case IntID:
				if len(data) < 8 {
					return to, x.Errorf("Invalid data for int64 %v", data)
				}
				*res = int64(binary.LittleEndian.Uint64(data))
			case FloatID:
				if len(data) < 8 {
					return to, x.Errorf("Invalid data for float %v", data)
				}
				i := binary.LittleEndian.Uint64(data)
				val := math.Float64frombits(i)
				*res = float64(val)
			case BoolID:
				if data[0] == 0 {
					*res = bool(false)
					return to, nil
				} else if data[0] == 1 {
					*res = bool(true)
					return to, nil
				} else {
					return to, x.Errorf("Invalid value for bool %v", data[0])
				}
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
			default:
				return to, cantConvert(fromID, toID)
			}
		}
	case StringID, DefaultID:
		{
			vc := string(data)
			switch toID {
			case BinaryID:
				// Marshal Binary
				*res = []byte(vc)
			case IntID:
				// Marshal text.
				val, err := strconv.ParseInt(string(vc), 10, 64)
				if err != nil {
					return to, err
				}
				*res = int64(val)
			case FloatID:
				val, err := strconv.ParseFloat(string(vc), 64)
				if err != nil {
					return to, err
				}
				if math.IsNaN(val) {
					return to, fmt.Errorf("Got NaN as value.")
				}
				*res = float64(val)
			case StringID, DefaultID:
				*res = string(vc)
			case BoolID:
				val, err := strconv.ParseBool(string(vc))
				if err != nil {
					return to, err
				}
				*res = bool(val)
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
					return to, err
				}
				*res = g
			case PasswordID:
				password, err := Encrypt(vc)
				if err != nil {
					return to, err
				}
				*res = password
			default:
				return to, cantConvert(fromID, toID)
			}
		}
	case IntID:
		{
			if len(data) < 8 {
				return to, x.Errorf("Invalid data for int64 %v", data)
			}
			vc := int64(binary.LittleEndian.Uint64(data))
			switch toID {
			case IntID:
				*res = vc
			case BinaryID:
				// Marshal Binary
				var bs [8]byte
				binary.LittleEndian.PutUint64(bs[:], uint64(vc))
				*res = bs[:]
			case FloatID:
				*res = float64(vc)
			case BoolID:
				*res = bool(vc != 1)
			case StringID, DefaultID:
				*res = string(strconv.FormatInt(vc, 10))
			case DateTimeID:
				*res = time.Unix(vc, 0).UTC()
			default:
				return to, cantConvert(fromID, toID)
			}
		}
	case FloatID:
		{
			if len(data) < 8 {
				return to, x.Errorf("Invalid data for float %v", data)
			}
			i := binary.LittleEndian.Uint64(data)
			val := math.Float64frombits(i)
			vc := float64(val)
			switch toID {
			case FloatID:
				*res = vc
			case BinaryID:
				// Marshal Binary
				var bs [8]byte
				u := math.Float64bits(float64(vc))
				binary.LittleEndian.PutUint64(bs[:], u)
				*res = bs[:]
			case IntID:
				if vc > math.MaxInt64 || vc < math.MinInt64 || math.IsNaN(float64(vc)) {
					return to, x.Errorf("Float out of int64 range")
				}
				*res = int64(vc)
			case BoolID:
				*res = bool(vc != 1)
			case StringID, DefaultID:
				*res = string(strconv.FormatFloat(float64(vc), 'E', -1, 64))
			case DateTimeID:
				secs := int64(vc)
				fracSecs := vc - float64(secs)
				nsecs := int64(fracSecs * nanoSecondsInSec)
				*res = time.Unix(secs, nsecs).UTC()
			default:
				return to, cantConvert(fromID, toID)
			}
		}
	case BoolID:
		{
			var vc bool
			if data[0] == 0 {
				vc = bool(false)
			} else if data[0] == 1 {
				vc = bool(true)
			} else {
				return to, x.Errorf("Invalid value for bool %v", data[0])
			}

			switch toID {
			case BoolID:
				*res = vc
			case BinaryID:
				// Marshal Binary
				var bs [1]byte
				bs[0] = 0
				if vc {
					bs[0] = 1
				}
				*res = bs[:]
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
			case StringID, DefaultID:
				*res = string(strconv.FormatBool(bool(vc)))
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
			vc := t
			switch toID {
			case DateTimeID:
				*res = vc
			case BinaryID:
				// Marshal Binary
				r, err := vc.MarshalBinary()
				if err != nil {
					return to, err
				}
				*res = r
			case StringID, DefaultID:
				val, err := vc.MarshalText()
				if err != nil {
					return to, err
				}
				*res = string(val)
			case IntID:
				secs := vc.Unix()
				if secs > math.MaxInt64 || secs < math.MinInt64 {
					return to, x.Errorf("Time out of int64 range")
				}
				*res = int64(secs)
			case FloatID:
				secs := float64(vc.Unix())
				nano := float64(vc.Nanosecond())
				val := secs + nano/nanoSecondsInSec
				*res = float64(val)
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
				// Marshal Binary
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
				// Marshal Binary
				*res = []byte(vc)
			case StringID:
				*res = vc
			case PasswordID:
				*res = vc
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
	fromID := from.Tid
	toID := to.Tid
	val := from.Value
	res := &to.Value

	switch fromID {
	case BinaryID:
		vc := val.([]byte)
		switch toID {
		case StringID, DefaultID:
			*res = string(vc)
		case BinaryID:
			// Marshal Binary
			*res = []byte(vc)
		default:
			return cantConvert(fromID, toID)
		}
	case StringID, DefaultID:
		vc := val.(string)
		switch toID {
		case StringID, DefaultID:
			*res = vc
		case BinaryID:
			// Marshal Binary
			*res = []byte(vc)
		default:
			return cantConvert(fromID, toID)
		}
	case IntID:
		vc := val.(int64)
		switch toID {
		case StringID, DefaultID:
			*res = strconv.FormatInt(int64(vc), 10)
		case BinaryID:
			// Marshal Binary
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
			*res = strconv.FormatFloat(float64(vc), 'E', -1, 64)
		case BinaryID:
			// Marshal Binary
			var bs [8]byte
			u := math.Float64bits(float64(vc))
			binary.LittleEndian.PutUint64(bs[:], u)
			*res = bs[:]
		default:
			return cantConvert(fromID, toID)
		}
	case BoolID:
		vc := val.(bool)
		switch toID {
		case StringID, DefaultID:
			*res = strconv.FormatBool(bool(vc))
		case BinaryID:
			// Marshal Binary
			var bs [1]byte
			bs[0] = 0
			if vc {
				bs[0] = 1
			}
			*res = bs[:]
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
			// Marshal Binary
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
			return x.Errorf("Expected a Geo type")
		}
		switch toID {
		case BinaryID:
			// Marshal Binary
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
			// Marshal Binary
			*res = []byte(vc)
		default:
			return cantConvert(fromID, toID)
		}

	default:
		return cantConvert(fromID, toID)
	}
	return nil
}

// ObjectValue converts into protos.Value.
func ObjectValue(id TypeID, value interface{}) (*protos.Value, error) {
	def := &protos.Value{&protos.Value_StrVal{""}}
	var ok bool
	// Lets set the object value according to the storage type.
	switch id {
	case StringID:
		var v string
		if v, ok = value.(string); !ok {
			return def, x.Errorf("Expected value of type string. Got : %v", value)
		}
		return &protos.Value{&protos.Value_StrVal{v}}, nil
	case DefaultID:
		var v string
		if v, ok = value.(string); !ok {
			return def, x.Errorf("Expected value of type string. Got : %v", value)
		}
		return &protos.Value{&protos.Value_DefaultVal{v}}, nil
	case IntID:
		var v int64
		if v, ok = value.(int64); !ok {
			return def, x.Errorf("Expected value of type int64. Got : %v", value)
		}
		return &protos.Value{&protos.Value_IntVal{v}}, nil
	case FloatID:
		var v float64
		if v, ok = value.(float64); !ok {
			return def, x.Errorf("Expected value of type float64. Got : %v", value)
		}
		return &protos.Value{&protos.Value_DoubleVal{v}}, nil
	case BoolID:
		var v bool
		if v, ok = value.(bool); !ok {
			return def, x.Errorf("Expected value of type bool. Got : %v", value)
		}
		return &protos.Value{&protos.Value_BoolVal{v}}, nil
	case BinaryID:
		var v []byte
		if v, ok = value.([]byte); !ok {
			return def, x.Errorf("Expected value of type []byte. Got : %v", value)
		}
		return &protos.Value{&protos.Value_BytesVal{v}}, nil
	// Geo and datetime are stored in binary format in the NQuad, so lets
	// convert them here.
	case GeoID:
		b, err := toBinary(id, value)
		if err != nil {
			return def, err
		}
		return &protos.Value{&protos.Value_GeoVal{b}}, nil
	case DateTimeID:
		b, err := toBinary(id, value)
		if err != nil {
			return def, err
		}
		return &protos.Value{&protos.Value_DatetimeVal{b}}, nil
	case PasswordID:
		var v string
		if v, ok = value.(string); !ok {
			return def, x.Errorf("Expected value of type password. Got : %v", value)
		}
		return &protos.Value{&protos.Value_PasswordVal{v}}, nil
	default:
		return def, x.Errorf("ObjectValue not available for: %v", id)
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
	return x.Errorf("Cannot convert %s to type %s", from.Name(), to.Name())
}

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
		return json.Marshal(v.Value.(string))
	case PasswordID:
		return json.Marshal(v.Value.(string))
	}
	return nil, x.Errorf("Invalid type for MarshalJSON: %v", v.Tid)
}
