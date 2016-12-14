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
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/dgraph-io/dgraph/x"
	geom "github.com/twpayne/go-geom"
	"github.com/twpayne/go-geom/encoding/geojson"
	"github.com/twpayne/go-geom/encoding/wkb"
)

// Convert converts the value to given scalar type.
func Convert(fromID TypeID, toID TypeID, data []byte, res *interface{}) error {
	var v interface{}

	// First decode binary to corresponding type interface
	// (Unmarshal from binary to the from type).
	switch fromID {
	case StringID:
		v = String(data)
	case Int32ID:
		if len(data) < 4 {
			return x.Errorf("Invalid data for int32 %v", data)
		}
		v = Int32(binary.LittleEndian.Uint32(data))
	case FloatID:
		if len(data) < 8 {
			return x.Errorf("Invalid data for float %v", data)
		}
		i := binary.LittleEndian.Uint64(data)
		val := math.Float64frombits(i)
		v = Float(val)
	case BoolID:
		if data[0] == 0 {
			v = Bool(false)
		} else if data[0] == 1 {
			v = Bool(true)
		} else {
			return x.Errorf("Invalid value for bool %v", data[0])
		}
	case DateID:
		if len(data) < 8 {
			return x.Errorf("Invalid data for date %v", data)
		}
		val := binary.LittleEndian.Uint64(data)
		tm := time.Unix(int64(val), 0)
		v = createDate(tm.Date())
	case DateTimeID:
		var t time.Time
		if err := t.UnmarshalBinary(data); err != nil {
			return err
		}
		v = Time{t}
	case GeoID:
		w, err := wkb.Unmarshal(data)
		if err != nil {
			return err
		}
		v = Geo{w}
	case BinaryID:
		// Nothing to decode here. handled later.
	}

	// Convert from-type to to-type and store in the result interface.
	switch fromID {
	case BinaryID:
		{
			// Unmarshal from Binary to type interfaces.
			switch toID {
			case BinaryID:
				*res = data
			case StringID:
				*res = String(data)
			case Int32ID:
				if len(data) < 4 {
					return x.Errorf("Invalid data for int32 %v", data)
				}
				*res = Int32(binary.LittleEndian.Uint32(data))
			case FloatID:
				if len(data) < 8 {
					return x.Errorf("Invalid data for float %v", data)
				}
				i := binary.LittleEndian.Uint64(data)
				val := math.Float64frombits(i)
				*res = Float(val)
			case BoolID:
				if data[0] == 0 {
					*res = Bool(false)
					return nil
				} else if data[0] == 1 {
					*res = Bool(true)
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
				*res = Time{t}
			case GeoID:
				w, err := wkb.Unmarshal(data)
				if err != nil {
					return err
				}
				*res = Geo{w}
			default:
				return cantConvert(fromID, toID)
			}
		}
	case StringID:
		{
			vc, ok := v.(String)
			if !ok {
				return x.Errorf("Expected a String type")
			}
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
				*res = Int32(val)
			case FloatID:
				val, err := strconv.ParseFloat(string(vc), 64)
				if err != nil {
					return err
				}
				*res = Float(val)
			case StringID:
				*res = String(vc)
			case BoolID:
				val, err := strconv.ParseBool(string(vc))
				if err != nil {
					return err
				}
				*res = Bool(val)
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
				*res = createDate(val.Date())
			case DateTimeID:
				var t time.Time
				if err := t.UnmarshalText([]byte(vc)); err != nil {
					// Try parsing without timezone since that is a valid format
					if t, err = time.Parse("2006-01-02T15:04:05", string(vc)); err != nil {
						return err
					}
				}
				*res = Time{t}
			case GeoID:
				var g geom.T
				text := bytes.Replace([]byte(vc), []byte("'"), []byte("\""), -1)
				if err := geojson.Unmarshal(text, &g); err != nil {
					return err
				}
				*res = Geo{g}
			default:
				return cantConvert(fromID, toID)
			}
		}
	case Int32ID:
		{
			vc, ok := v.(Int32)
			if !ok {
				return x.Errorf("Expected a Int32 type")
			}

			switch toID {
			case Int32ID:
				*res = vc
			case BinaryID:
				// Marshal Binary
				var bs [4]byte
				binary.LittleEndian.PutUint32(bs[:], uint32(vc))
				*res = bs[:]
			case FloatID:
				*res = Float(vc)
			case BoolID:
				*res = Bool(vc != 1)
			case StringID:
				*res = String(strconv.FormatInt(int64(vc), 10))
			default:
				return cantConvert(fromID, toID)
			}
		}
	case FloatID:
		{
			vc, ok := v.(Float)
			if !ok {
				return x.Errorf("Expected a Float type")
			}
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
				*res = Int32(vc)
			case BoolID:
				*res = Bool(vc != 1)
			case StringID:
				*res = String(strconv.FormatFloat(float64(vc), 'E', -1, 64))
			default:
				return cantConvert(fromID, toID)
			}
		}
	case BoolID:
		{
			vc, ok := v.(Bool)
			if !ok {
				return x.Errorf("Expected a Bool type")
			}
			switch toID {
			case BoolID:
				*res = vc
			case BinaryID:
				// Marshal Binary
				var bs [1]byte
				if vc {
					bs[0] = 1
				} else {
					bs[0] = 0
				}
				*res = bs[:]
			case Int32ID:
				if vc {
					*res = Int32(1)
				} else {
					*res = Int32(0)
				}
			case FloatID:
				if vc {
					*res = Float(1)
				} else {
					*res = Float(0)
				}
			case StringID:
				*res = String(strconv.FormatBool(bool(vc)))
			default:
				return cantConvert(fromID, toID)
			}
		}
	case DateID:
		{
			vc, ok := v.(Date)
			if !ok {
				return x.Errorf("Expected a Date type")
			}

			switch toID {
			case DateID:
				*res = vc
			case BinaryID:
				// Marshal Binary
				var bs [8]byte
				binary.LittleEndian.PutUint64(bs[:], uint64(vc.Time.Unix()))
				*res = bs[:]
			case DateTimeID:
				*res = Time(createDate(vc.Date()))
			case StringID:
				*res = String(vc.Format(dateFormatYMD))
			default:
				return cantConvert(fromID, toID)
			}
		}
	case DateTimeID:
		{
			vc, ok := v.(Time)
			if !ok {
				return x.Errorf("Expected a DateTime type")
			}
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
				*res = Date(vc)
			case StringID:
				*res = String(vc.Time.String())
			default:
				return cantConvert(fromID, toID)
			}
		}
	case GeoID:
		{
			vc, ok := v.(Geo)
			if !ok {
				return x.Errorf("Expected a Geo type")
			}

			switch toID {
			case GeoID:
				*res = vc
			case BinaryID:
				// Marshal Binary
				r, err := wkb.Marshal(vc.T, binary.LittleEndian)
				if err != nil {
					return err
				}
				*res = r
			case StringID:
				val, err := geojson.Marshal(vc.T)
				if err != nil {
					return nil
				}
				*res = String(bytes.Replace(val, []byte("\""), []byte("'"), -1))
			default:
				fmt.Println("wrong")
				return cantConvert(fromID, toID)
			}
		}
	}
	return nil
}

func ConvertFromInterface(fromID TypeID, toID TypeID, val interface{}, res *interface{}) error {
	switch fromID {
	case StringID:
		vc := val.(String)
		switch toID {
		case StringID:
			*res = String(vc)
		case BinaryID:
			// Marshal Binary
			*res = []byte(vc)
		}
	case Int32ID:
		vc := val.(Int32)
		switch toID {
		case StringID:
			*res = String(strconv.FormatInt(int64(vc), 10))
		case BinaryID:
			// Marshal Binary
			var bs [4]byte
			binary.LittleEndian.PutUint32(bs[:], uint32(vc))
			*res = bs[:]
		}
	case FloatID:
		vc := val.(Float)
		switch toID {
		case StringID:
			*res = String(strconv.FormatFloat(float64(vc), 'E', -1, 64))
		case BinaryID:
			// Marshal Binary
			var bs [8]byte
			u := math.Float64bits(float64(vc))
			binary.LittleEndian.PutUint64(bs[:], u)
			*res = bs[:]
		}
	case BoolID:
		vc := val.(Bool)
		switch toID {
		case StringID:
			*res = String(strconv.FormatBool(bool(vc)))
		case BinaryID:
			// Marshal Binary
			var bs [1]byte
			if vc {
				bs[0] = 1
			} else {
				bs[0] = 0
			}
			*res = bs[:]
		}
	case DateID:
		vc := val.(Date)
		switch toID {
		case StringID:
			*res = String(vc.Format(dateFormatYMD))
		case BinaryID:
			var bs [8]byte
			binary.LittleEndian.PutUint64(bs[:], uint64(vc.Time.Unix()))
			*res = Binary(bs[:])
		}
	case DateTimeID:
		vc := val.(Time)
		switch toID {
		case StringID:
			*res = String(vc.Time.String())
		case BinaryID:
			// Marshal Binary
			r, err := vc.MarshalBinary()
			if err != nil {
				return err
			}
			*res = Binary(r)
		}
	case GeoID:
		vc, ok := val.(Geo)
		if !ok {
			return x.Errorf("Expected a Geo type")
		}
		switch toID {
		case BinaryID:
			// Marshal Binary
			r, err := wkb.Marshal(vc.T, binary.LittleEndian)
			if err != nil {
				return err
			}
			*res = Binary(r)
		case StringID:
			val, err := geojson.Marshal(vc.T)
			if err != nil {
				return nil
			}
			*res = String(bytes.Replace(val, []byte("\""), []byte("'"), -1))
		default:
			fmt.Println("wrong")
			return cantConvert(fromID, toID)
		}
	}
	return nil
}

func cantConvert(from TypeID, to TypeID) error {
	fmt.Println(from, to)
	return x.Errorf("Cannot convert %s to type %s", from.Name(), to.Name())
}
