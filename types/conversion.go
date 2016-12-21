/*
 * Copyright 2016 Dgraph Labs, Inc.
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
	"bytes"
	"encoding/binary"
	"encoding/json"
	"math"
	"strconv"
	"time"

	"github.com/dgraph-io/dgraph/query/graph"
	"github.com/dgraph-io/dgraph/x"
	geom "github.com/twpayne/go-geom"
	"github.com/twpayne/go-geom/encoding/geojson"
	"github.com/twpayne/go-geom/encoding/wkb"
)

func ByteVal(nq *graph.NQuad) ([]byte, error) {
	if nq.ObjectType == int32(BinaryID) {
		return nq.ObjectValue.GetBytesVal(), nil
	}
	p := ValueForType(TypeID(nq.ObjectType))
	src := ValueForType(StringID)
	src.Value = []byte(nq.ObjectValue.GetStrVal())
	err := Convert(src, &p)
	if err != nil {
		return []byte{}, err
	}

	p1 := ValueForType(BinaryID)
	err = Marshal(p, &p1)
	if err != nil {
		return []byte{}, err
	}
	return []byte(p1.Value.([]byte)), nil
}

// Convert converts the value to given scalar type.
func Convert(from Val, to *Val) error {
	// fromID TypeID, toID TypeID, data []byte, res *interface{}) error {

	fromID := from.Tid
	toID := to.Tid
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
			case StringID:
				*res = string(data)
			case Int32ID:
				if len(data) < 4 {
					return x.Errorf("Invalid data for int32 %v", data)
				}
				*res = int32(binary.LittleEndian.Uint32(data))
			case FloatID:
				if len(data) < 8 {
					return x.Errorf("Invalid data for float %v", data)
				}
				i := binary.LittleEndian.Uint64(data)
				val := math.Float64frombits(i)
				*res = float64(val)
			case BoolID:
				if data[0] == 0 {
					*res = bool(false)
					return nil
				} else if data[0] == 1 {
					*res = bool(true)
					return nil
				} else {
					return x.Errorf("Invalid value for bool %v", data[0])
				}
			case DateID:
				if len(data) < 8 {
					return x.Errorf("Invalid data for date %v", data)
				}
				val := binary.LittleEndian.Uint64(data)
				tm := time.Unix(int64(val), 0)
				*res = createDate(tm.Date())
			case DateTimeID:
				var t time.Time
				if err := t.UnmarshalBinary(data); err != nil {
					return err
				}
				*res = t
			case GeoID:
				w, err := wkb.Unmarshal(data)
				if err != nil {
					return err
				}
				*res = w
			default:
				return cantConvert(fromID, toID)
			}
		}
	case StringID:
		{
			vc := string(data)
			switch toID {
			case BinaryID:
				// Marshal Binary
				*res = []byte(vc)
			case Int32ID:
				// Marshal text.
				val, err := strconv.ParseInt(string(vc), 10, 32)
				if err != nil {
					return err
				}
				*res = int32(val)
			case FloatID:
				val, err := strconv.ParseFloat(string(vc), 64)
				if err != nil {
					return err
				}
				*res = float64(val)
			case StringID:
				*res = string(vc)
			case BoolID:
				val, err := strconv.ParseBool(string(vc))
				if err != nil {
					return err
				}
				*res = bool(val)
			case DateID:
				val, err := time.Parse(dateFormatYMD, string(vc))
				if err != nil {
					val, err = time.Parse(dateFormatYM, string(vc))
					if err != nil {
						val, err = time.Parse(dateFormatY, string(vc))
						if err != nil {
							return err
						}
					}
				}
				*res = val
			case DateTimeID:
				var t time.Time
				if err := t.UnmarshalText([]byte(vc)); err != nil {
					// Try parsing without timezone since that is a valid format
					if t, err = time.Parse("2006-01-02T15:04:05", string(vc)); err != nil {
						return err
					}
				}
				*res = t
			case GeoID:
				var g geom.T
				text := bytes.Replace([]byte(vc), []byte("'"), []byte("\""), -1)
				if err := geojson.Unmarshal(text, &g); err != nil {
					return err
				}
				*res = g
			default:
				return cantConvert(fromID, toID)
			}
		}
	case Int32ID:
		{
			if len(data) < 4 {
				return x.Errorf("Invalid data for int32 %v", data)
			}
			vc := int32(binary.LittleEndian.Uint32(data))
			switch toID {
			case Int32ID:
				*res = vc
			case BinaryID:
				// Marshal Binary
				var bs [4]byte
				binary.LittleEndian.PutUint32(bs[:], uint32(vc))
				*res = bs[:]
			case FloatID:
				*res = float64(vc)
			case BoolID:
				*res = bool(vc != 1)
			case StringID:
				*res = string(strconv.FormatInt(int64(vc), 10))
			case DateID:
				date := time.Unix(int64(vc), 0).UTC()
				*res = createDate(date.Date())
			case DateTimeID:
				*res = time.Unix(int64(vc), 0).UTC()
			default:
				return cantConvert(fromID, toID)
			}
		}
	case FloatID:
		{
			if len(data) < 8 {
				return x.Errorf("Invalid data for float %v", data)
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
			case Int32ID:
				if vc > math.MaxInt32 || vc < math.MinInt32 || math.IsNaN(float64(vc)) {
					return x.Errorf("Float out of int32 range")
				}
				*res = int32(vc)
			case BoolID:
				*res = bool(vc != 1)
			case StringID:
				*res = string(strconv.FormatFloat(float64(vc), 'E', -1, 64))
			case DateID:
				secs := int64(vc)
				fracSecs := vc - float64(secs)
				nsecs := int64(fracSecs * nanoSecondsInSec)
				date := time.Unix(secs, nsecs).UTC()
				*res = createDate(date.Date())
			case DateTimeID:
				secs := int64(vc)
				fracSecs := vc - float64(secs)
				nsecs := int64(fracSecs * nanoSecondsInSec)
				*res = time.Unix(secs, nsecs).UTC()
			default:
				return cantConvert(fromID, toID)
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
				return x.Errorf("Invalid value for bool %v", data[0])
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
			case Int32ID:
				*res = int32(0)
				if vc {
					*res = int32(1)
				}
			case FloatID:
				*res = float64(0)
				if vc {
					*res = float64(1)
				}
			case StringID:
				*res = string(strconv.FormatBool(bool(vc)))
			default:
				return cantConvert(fromID, toID)
			}
		}
	case DateID:
		{
			if len(data) < 8 {
				return x.Errorf("Invalid data for date %v", data)
			}
			val := binary.LittleEndian.Uint64(data)
			tm := time.Unix(int64(val), 0)
			vc := createDate(tm.Date())

			switch toID {
			case DateID:
				*res = vc
			case BinaryID:
				// Marshal Binary
				var bs [8]byte
				binary.LittleEndian.PutUint64(bs[:], uint64(vc.Unix()))
				*res = bs[:]
			case DateTimeID:
				*res = createDate(vc.Date())
			case StringID:
				*res = string(vc.Format(dateFormatYMD))
			case Int32ID:
				secs := vc.Unix()
				if secs > math.MaxInt32 || secs < math.MinInt32 {
					return x.Errorf("Time out of int32 range")
				}
				*res = int32(secs)
			case FloatID:
				secs := float64(vc.Unix())
				nano := float64(vc.Nanosecond())
				val := secs + nano/nanoSecondsInSec
				*res = float64(val)
			default:
				return cantConvert(fromID, toID)
			}
		}
	case DateTimeID:
		{
			var t time.Time
			if err := t.UnmarshalBinary(data); err != nil {
				return err
			}
			vc := t
			switch toID {
			case DateTimeID:
				*res = vc
			case BinaryID:
				// Marshal Binary
				r, err := vc.MarshalBinary()
				if err != nil {
					return err
				}
				*res = r
			case DateID:
				*res = createDate(vc.Date())
			case StringID:
				*res = string(vc.String())
			case Int32ID:
				secs := vc.Unix()
				if secs > math.MaxInt32 || secs < math.MinInt32 {
					return x.Errorf("Time out of int32 range")
				}
				*res = int32(secs)
			case FloatID:
				secs := float64(vc.Unix())
				nano := float64(vc.Nanosecond())
				val := secs + nano/nanoSecondsInSec
				*res = float64(val)
			default:
				return cantConvert(fromID, toID)
			}
		}
	case GeoID:
		{
			vc, err := wkb.Unmarshal(data)
			if err != nil {
				return err
			}
			switch toID {
			case GeoID:
				*res = vc
			case BinaryID:
				// Marshal Binary
				r, err := wkb.Marshal(vc, binary.LittleEndian)
				if err != nil {
					return err
				}
				*res = r
			case StringID:
				val, err := geojson.Marshal(vc)
				if err != nil {
					return nil
				}
				*res = string(bytes.Replace(val, []byte("\""), []byte("'"), -1))
			default:
				return cantConvert(fromID, toID)
			}
		}
	default:
		return cantConvert(fromID, toID)
	}
	return nil
}

func Marshal(from Val, to *Val) error {
	// toID TypeID, val interface{}, res *interface{}) error {
	fromID := from.Tid
	toID := to.Tid
	val := from.Value
	res := &to.Value

	switch fromID {
	case BinaryID:
		vc := val.([]byte)
		switch toID {
		case StringID:
			*res = string(vc)
		case BinaryID:
			// Marshal Binary
			*res = []byte(vc)
		default:
			return cantConvert(fromID, toID)
		}
	case StringID:
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
	case Int32ID:
		vc := val.(int32)
		switch toID {
		case StringID:
			*res = strconv.FormatInt(int64(vc), 10)
		case BinaryID:
			// Marshal Binary
			var bs [4]byte
			binary.LittleEndian.PutUint32(bs[:], uint32(vc))
			*res = bs[:]
		default:
			return cantConvert(fromID, toID)
		}
	case FloatID:
		vc := val.(float64)
		switch toID {
		case StringID:
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
		case StringID:
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
	case DateID:
		vc := val.(time.Time)
		switch toID {
		case StringID:
			*res = vc.Format(dateFormatYMD)
		case BinaryID:
			var bs [8]byte
			binary.LittleEndian.PutUint64(bs[:], uint64(vc.Unix()))
			*res = bs[:]
		default:
			return cantConvert(fromID, toID)
		}
	case DateTimeID:
		vc := val.(time.Time)
		switch toID {
		case StringID:
			*res = vc.String()
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
		case StringID:
			val, err := geojson.Marshal(vc)
			if err != nil {
				return nil
			}
			*res = string(bytes.Replace(val, []byte("\""), []byte("'"), -1))
		default:
			return cantConvert(fromID, toID)
		}
	default:
		return cantConvert(fromID, toID)
	}
	return nil
}

func cantConvert(from TypeID, to TypeID) error {
	return x.Errorf("Cannot convert %s to type %s", from.Name(), to.Name())
}

func (v Val) MarshalJSON() ([]byte, error) {
	switch v.Tid {
	case Int32ID:
		return json.Marshal(v.Value.(int32))
	case BoolID:
		return json.Marshal(v.Value.(bool))
	case FloatID:
		return json.Marshal(v.Value.(float64))
	case DateID:
		s := v.Value.(time.Time).Format(dateFormatYMD)
		return json.Marshal(s)
	case DateTimeID:
		return json.Marshal(v.Value.(time.Time))
	case GeoID:
		return json.Marshal(v.Value.(geom.T))
	case StringID:
		return json.Marshal(v.Value.(string))
	}
	return nil, x.Errorf("Invalid type")
}
