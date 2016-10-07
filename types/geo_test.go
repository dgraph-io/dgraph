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
	"reflect"
	"testing"
)

func TestParse(t *testing.T) {
	array := []string{
		`{"type":"Point","coordinates":[1,2]}`,
		`{"type":"MultiLineString","coordinates":[[[1,2,3],[4,5,6],[7,8,9],[1,2,3]]]}`,
	}
	for _, v := range array {
		if g, err := GeoID.Unmarshaler().FromText([]byte(v)); err != nil {
			t.Errorf("Error parsing %s: %v", v, err)
		} else {
			// Marshal it back to text
			if got, err := g.MarshalText(); err != nil || string(got) != v {
				t.Errorf("Marshal error expected %s, got %s. error %v", v, string(got), err)
			}

			// Marshal and unmarshal to WKB
			wkb, err := g.MarshalBinary()
			if err != nil {
				t.Errorf("Error marshaling to WKB: %v", err)
			}

			if bg, err := GeoID.Unmarshaler().FromBinary(wkb); err != nil {
				t.Errorf("Error unmarshaling WKB: %v", err)
			} else if !reflect.DeepEqual(g, bg) {
				t.Errorf("Expected %#v, got %#v", g, bg)
			}
		}
	}
}

func TestParseGeoJsonErrors(t *testing.T) {
	array := []string{
		`{"type":"Curve","coordinates":[1,2]}`,
		`{"type":"Feature","geometry":{"type":"Point","coordinates":[125.6,10.1]},"properties":{"name":"Dinagat Islands"}}`,
		`{}`,
		`thisisntjson`,
	}
	for _, v := range array {
		if _, err := GeoID.Unmarshaler().FromText([]byte(v)); err == nil {
			t.Errorf("Expected error parsing %s: %v", v, err)
		}
	}
}
