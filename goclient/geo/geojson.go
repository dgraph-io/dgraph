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

package geo

import (
	"encoding/binary"

	geom "github.com/twpayne/go-geom"
	"github.com/twpayne/go-geom/encoding/geojson"
	"github.com/twpayne/go-geom/encoding/wkb"

	"github.com/dgraph-io/dgraph/goclient/client"
)

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
func ValueFromJson(json string) (client.Value, error) {
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
	return client.Geo(b), nil
}
