/*
 * Copyright (C) 2017 Dgraph Labs, Inc. and Contributors
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
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
