/*
 * Copyright 2016 Dgraph Labs, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *    http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package client

import (
	"encoding/binary"
	"time"

	"github.com/dgraph-io/dgraph/query/graph"
	geom "github.com/twpayne/go-geom"
	"github.com/twpayne/go-geom/encoding/geojson"
	"github.com/twpayne/go-geom/encoding/wkb"
)

// Value represents a value sent in a mutation.
type Value *graph.Value

// Int returns an int graph.Value
func Int(val int32) Value {
	return &graph.Value{&graph.Value_IntVal{val}}
}

// Double returns an double graph.Value
func Double(val float64) Value {
	return &graph.Value{&graph.Value_DoubleVal{val}}
}

// Str returns an string graph.Value
func Str(val string) Value {
	return &graph.Value{&graph.Value_StrVal{val}}
}

// Bytes returns an byte array graph.Value
func Bytes(val []byte) Value {
	return &graph.Value{&graph.Value_BytesVal{val}}
}

// Bool returns an bool graph.Value
func Bool(val bool) Value {
	return &graph.Value{&graph.Value_BoolVal{val}}
}

// Geo returns a geo graph.Value
func Geo(val []byte) Value {
	return &graph.Value{&graph.Value_GeoVal{val}}
}

func Date(date time.Time) (Value, error) {
	b, err := date.MarshalBinary()
	if err != nil {
		return &graph.Value{}, err
	}
	return &graph.Value{&graph.Value_DateVal{b}}, nil
}

func Datetime(date time.Time) (Value, error) {
	b, err := date.MarshalBinary()
	if err != nil {
		return &graph.Value{}, err
	}
	return &graph.Value{&graph.Value_DatetimeVal{b}}, nil
}

// ToValue converts val into the appropriate Value
func ToValue(val interface{}) Value {
	switch v := val.(type) {
	case int32:
		return Int(v)
	case string:
		return Str(v)
	case float64:
		return Double(v)
	case bool:
		return Bool(v)
	default:
		return nil
	}
}

func IsEmpty(val *graph.Value) bool {
	switch val.Val.(type) {
	case *graph.Value_IntVal:
		return val.GetIntVal() == 0
	case *graph.Value_StrVal:
		return val.GetStrVal() == ""
	case *graph.Value_BoolVal:
		return val.GetBoolVal() == false
	case *graph.Value_DoubleVal:
		return val.GetDoubleVal() == 0.0
	case *graph.Value_GeoVal:
		return len(val.GetGeoVal()) == 0
	default:
		// Unknown type
		return false
	}
}

// ValueFromJson converts geojson into a client.Value
// Example usage
// req := client.Req{}

// loc, err := geo.ValueFromJson(`{"type":"Point","coordinates":[-122.2207184,37.72129059]}`)
// if err != nil {
// 	log.Fatal(err)
// }
// b, err := client.Date(time.Now())
// if err != nil {
// 	log.Fatal(err)
// }

// if err := req.AddMutation(graph.NQuad{
// 	Subject:     "alice",
// 	Predicate:   "birthday",
// 	ObjectValue: loc,
// }, client.SET); err != nil {
// 	log.Fatal(err)
// }
//
func ValueFromJson(json string) (Value, error) {
	var g geom.T
	// Parse the json
	err := geojson.Unmarshal([]byte(json), &g)
	if err != nil {
		return nil, err
	}

	// parse the geometry object to WKB
	b, err := wkb.Marshal(g, binary.LittleEndian)
	if err != nil {
		return nil, err
	}
	return Geo(b), nil
}
