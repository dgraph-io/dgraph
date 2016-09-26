/*
 * Copyright 2016 DGraph Labs, Inc.
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
)

// Date represents a date (YYYY-MM-DD). There is no timezone information
// attached.
type Date struct {
	time time.Time
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
