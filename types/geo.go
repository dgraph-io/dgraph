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

	"github.com/twpayne/go-geom"
	"github.com/twpayne/go-geom/encoding/geojson"
	"github.com/twpayne/go-geom/encoding/wkb"
)

// Geo represents geo-spatial data.
type Geo struct {
	geom.T
}

// MarshalBinary marshals to binary
func (v Geo) MarshalBinary() ([]byte, error) {
	return wkb.Marshal(v.T, binary.LittleEndian)
}

// MarshalText marshals to text
func (v Geo) MarshalText() ([]byte, error) {
	// The text format is geojson
	return geojson.Marshal(v.T)
}

// MarshalJSON marshals to json
func (v Geo) MarshalJSON() ([]byte, error) {
	// this same as MarshalText
	return v.MarshalText()
}

// Type returns the type of this value
func (v Geo) Type() Scalar {
	return geoType
}

// UnmarshalBinary unmarshals the data from WKB
func (v *Geo) UnmarshalBinary(data []byte) error {
	w, err := wkb.Unmarshal(data)
	if err != nil {
		return err
	}
	v.T = w
	return nil
}

// UnmarshalText parses the data from a Geojson
func (v *Geo) UnmarshalText(text []byte) error {
	var g geom.T
	if err := geojson.Unmarshal(text, &g); err != nil {
		return err
	}
	v.T = g
	return nil
}

func (v Geo) String() string {
	return "<geodata>"
}
