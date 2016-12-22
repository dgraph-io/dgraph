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
	"fmt"
	"time"

	"github.com/dgraph-io/dgraph/query/graph"
	"github.com/dgraph-io/dgraph/types"
	geom "github.com/twpayne/go-geom"
	"github.com/twpayne/go-geom/encoding/geojson"
	"github.com/twpayne/go-geom/encoding/wkb"
)

// ValueFromJson converts geojson into a client.Value
// Example usage
// req := client.Req{}

// loc, err := client.ValueFromGeoJson(`{"type":"Point","coordinates":[-122.2207184,37.72129059]}`)
// if err != nil {
// 	log.Fatal(err)
// }
//
// if err := req.AddMutation(graph.NQuad{
// 	Subject:     "alice",
// 	Predicate:   "birthday",
// 	ObjectValue: loc,
// }, client.SET); err != nil {
// 	log.Fatal(err)
// }
//
func ValueFromGeoJson(json string) (types.Value, error) {
	var g geom.T
	// Parse the json
	err := geojson.Unmarshal([]byte(json), &g)
	if err != nil {
		return &graph.Value{}, err
	}

	// parse the geometry object to WKB
	b, err := wkb.Marshal(g, binary.LittleEndian)
	if err != nil {
		return &graph.Value{}, err
	}
	return types.Geo(b), nil
}

func Date(date time.Time) (types.Value, error) {
	b, err := date.MarshalBinary()
	if err != nil {
		return &graph.Value{}, err
	}
	return types.Date(b), nil
}

func Datetime(date time.Time) (types.Value, error) {
	b, err := date.MarshalBinary()
	if err != nil {
		return &graph.Value{}, err
	}
	return types.Datetime(b), nil
}

// Uid converts an uint64 to a string, which can be used as part of
// Subject and ObjectId fields in the graph.NQuad
func Uid(uid uint64) string {
	return fmt.Sprintf("%#x", uid)
}
