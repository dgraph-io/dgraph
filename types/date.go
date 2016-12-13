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

import "time"

// Date represents a date (YYYY-MM-DD). There is no timezone information
// attached.
type Date struct {
	time.Time
}

func createDate(y int, m time.Month, d int) Date {
	var dt Date
	dt.Time = time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	return dt
}

const dateFormatYMD = "2006-01-02"
const dateFormatYM = "2006-01"
const dateFormatY = "2006"

/*
// MarshalBinary marshals to binary
func (v Date) MarshalBinary() ([]byte, error) {
	var bs [8]byte
	binary.LittleEndian.PutUint64(bs[:], uint64(v.Time.Unix()))
	return bs[:], nil
}

// MarshalText marshals to text
func (v Date) MarshalText() ([]byte, error) {
	s := v.Time.Format(dateFormatYMD)
	return []byte(s), nil
}

// MarshalJSON marshals to json
func (v Date) MarshalJSON() ([]byte, error) {
	s := v.Time.Format(dateFormatYMD)
	return json.Marshal(s)
}

func (v Date) String() string {
	str, _ := v.MarshalText()
	return string(str)
}

// UnmarshalBinary unmarshals the data from a binary format.
func (v *Date) UnmarshalBinary(data []byte) error {
	if len(data) < 8 {
		return x.Errorf("Invalid data for date %v", data)
	}
	val := binary.LittleEndian.Uint64(data)
	tm := time.Unix(int64(val), 0)
	return v.fromTime(tm)
}

// UnmarshalText unmarshals the data from a text format.
func (v *Date) UnmarshalText(text []byte) error {
	val, err := time.Parse(dateFormatYMD, string(text))
	if err != nil {
		val, err = time.Parse(dateFormatYM, string(text))
		if err != nil {
			val, err = time.Parse(dateFormatY, string(text))
			if err != nil {
				return err
			}
		}
	}
	return v.fromTime(val)
}

func (v *Date) fromFloat(f float64) error {
	var t Time
	err := t.fromFloat(f)
	if err != nil {
		return err
	}
	return v.fromTime(t.Time)
}

func (v *Date) fromTime(t time.Time) error {
	// truncate the time to just a date.
	*v = createDate(t.Date())
	return nil
}

func (v *Date) fromInt(i int32) error {
	var t Time
	err := t.fromInt(i)
	if err != nil {
		return err
	}
	return v.fromTime(t.Time)
}

func (v *Time) fromDate(d Date) error {
	v.Time = d.Time
	return nil
}

func (v *Float) fromDate(d Date) error {
	return v.fromTime(d.Time)
}

func (v *Int32) fromDate(d Date) error {
	return v.fromTime(d.Time)
}
*/
