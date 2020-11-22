/*
 * Copyright 2016-2018 Dgraph Labs, Inc. and Contributors
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
	"reflect"
	"testing"
)

func TestParse(t *testing.T) {
	array := []string{
		`{'type':'Point','coordinates':[1,2]}`,
		`{'type':'MultiLineString','coordinates':[[[1,2,3],[4,5,6],[7,8,9],[1,2,3]]]}`,
	}
	for _, v := range array {
		src := Val{StringID, []byte(v)}

		if g, err := Convert(src, GeoID); err != nil {
			t.Errorf("Error parsing %s: %v", v, err)
		} else {
			// Marshal it back to text
			got := ValueForType(StringID)
			if err := Marshal(g, &got); err != nil || string(got.Value.(string)) != v {
				t.Errorf("Marshal error expected %s, got %s. error %v", v, string(got.Value.(string)), err)
			}

			wkb := ValueForType(BinaryID)
			// Marshal and unmarshal to WKB
			err = Marshal(g, &wkb)
			if err != nil {
				t.Errorf("Error marshaling to WKB: %v", err)
			}

			src := Val{GeoID, []byte(wkb.Value.([]byte))}

			if bg, err := Convert(src, GeoID); err != nil {
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
		src := Val{StringID, []byte(v)}
		if _, err := Convert(src, GeoID); err == nil {
			t.Errorf("Expected error parsing %s: %v", v, err)
		}
	}
}
