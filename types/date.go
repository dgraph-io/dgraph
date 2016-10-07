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
	"encoding/binary"
	"encoding/json"
	"time"

	"github.com/dgraph-io/dgraph/x"
)

// Date represents a date (YYYY-MM-DD). There is no timezone information
// attached.
type Date struct {
	time time.Time
}

func createDate(y int, m time.Month, d int) Date {
	var dt Date
	dt.time = time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	return dt
}

const dateFormat = "2006-01-02"

// MarshalBinary marshals to binary
func (v Date) MarshalBinary() ([]byte, error) {
	var bs [8]byte
	binary.LittleEndian.PutUint64(bs[:], uint64(v.time.Unix()))
	return bs[:], nil
}

// MarshalText marshals to text
func (v Date) MarshalText() ([]byte, error) {
	s := v.time.Format(dateFormat)
	return []byte(s), nil
}

// MarshalJSON marshals to json
func (v Date) MarshalJSON() ([]byte, error) {
	str, err := v.MarshalText()
	if err != nil {
		return nil, err
	}
	return json.Marshal(str)
}

// Type returns the type of this value
func (v Date) Type() Type {
	return typeIDMap[dateID]
}

type unmarshalDate struct{}

func (u unmarshalDate) FromBinary(data []byte) (Value, error) {
	if len(data) < 8 {
		return nil, x.Errorf("Invalid data for date %v", data)
	}
	val := binary.LittleEndian.Uint64(data)
	tm := time.Unix(int64(val), 0)
	return u.fromTime(tm)
}

func (u unmarshalDate) FromText(text []byte) (Value, error) {
	val, err := time.Parse(dateFormat, string(text))
	if err != nil {
		return nil, err
	}
	return u.fromTime(val)
}

var uDate unmarshalDate

func (u unmarshalDate) fromFloat(f float64) (Value, error) {
	v, err := uTime.fromFloat(f)
	if err != nil {
		return nil, err
	}
	tm := v.(Time)
	return u.fromTime(tm.Time)
}

func (u unmarshalDate) fromTime(t time.Time) (Value, error) {
	// truncate the time to just a date.
	return createDate(t.Date()), nil
}

func (u unmarshalDate) fromInt(i int32) (Value, error) {
	v, err := uTime.fromInt(i)
	if err != nil {
		return nil, err
	}
	tm := v.(Time)
	return u.fromTime(tm.Time)
}

func (u unmarshalTime) fromDate(v Date) (Value, error) {
	return Time{v.time}, nil
}

func (u unmarshalFloat) fromDate(v Date) (Value, error) {
	return u.fromTime(v.time)
}

func (u unmarshalInt32) fromDate(v Date) (Value, error) {
	return u.fromTime(v.time)
}
