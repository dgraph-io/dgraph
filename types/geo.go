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
	geo geom.T
}

// MarshalBinary marshals to binary
func (v Geo) MarshalBinary() ([]byte, error) {
	return wkb.Marshal(v.geo, binary.LittleEndian)
}

// MarshalText marshals to text
func (v Geo) MarshalText() ([]byte, error) {
	// The text format is geojson
	return geojson.Marshal(v.geo)
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

type unmarshalGeo struct{}

func (u unmarshalGeo) FromBinary(data []byte) (Value, error) {
	v, err := wkb.Unmarshal(data)
	if err != nil {
		return nil, err
	}
	var g Geo
	g.geo = v
	return g, nil
}

// Parses geojson text.
func (u unmarshalGeo) FromText(text []byte) (Value, error) {
	var g geom.T
	if err := geojson.Unmarshal(text, &g); err != nil {
		return nil, err
	}
	var geo Geo
	geo.geo = g
	return geo, nil
}

var uGeo unmarshalGeo
