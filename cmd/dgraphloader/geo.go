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

package main

import (
	"compress/gzip"
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"strings"

	"google.golang.org/grpc"

	"context"

	geom "github.com/twpayne/go-geom"
	"github.com/twpayne/go-geom/encoding/geojson"
	"github.com/twpayne/go-geom/encoding/wkb"

	"github.com/dgraph-io/dgraph/goclient/client"
	"github.com/dgraph-io/dgraph/query/graph"
)

var (
	idname   = flag.String("geoid", "", "The name of property to use as the xid")
	jsonFile = flag.String("json", "", "Json file from which to upload geo data")
)

func uploadJSON(json string) {
	f, err := os.Open(json)
	if err != nil {
		log.Fatalf("Error opening file %s: %v", json, err)
	}
	defer f.Close()

	conn, err := grpc.Dial(*dgraph, grpc.WithInsecure())
	if err != nil {
		log.Fatal("DialTCPConnection")
	}
	defer conn.Close()

	var r io.Reader
	r = f
	batch := client.NewBatchMutation(context.Background(), conn, 1000, 10)

	if strings.HasSuffix(json, ".gz") {
		r, err = gzip.NewReader(f)
		if err != nil {
			log.Fatalf("Error reading gzip file %s: %v", json, err)
		}
	}
	Upload(batch, r)
}

// Upload a geojson file to the server by converting it into the NQuads format.
func Upload(batch *client.BatchMutation, r io.Reader) {
	cfeat := make(chan result, 1000)

	go parse(r, cfeat)
	go createRequest(cfeat, batch)

	// maxConcurrent := 20
	// var wg sync.WaitGroup
	// wg.Add(maxConcurrent)
	// for i := 0; i < maxConcurrent; i++ {
	// 	go makeRequests(&wg, c, creq)
	// }
	// wg.Wait()
}

// ValueFromJson converts geojson into a client.Value
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

type result struct {
	f   *geojson.Feature
	err error
}

func createRequest(in <-chan result, batch *client.BatchMutation) {
	for v := range in {
		if v.err != nil {
			log.Printf("Error parsing file: %v\n", v.err)
			return
		}
		toMutations(v.f, batch)
	}
}

func toMutations(f *geojson.Feature, batch *client.BatchMutation) {
	id, ok := getID(f)
	if !ok {
		log.Println("Skipping feature, cannot find id.")
		return
	}

	for k, v := range f.Properties {
		k := strings.Replace(k, " ", "_", -1)
		if f, ok := v.(float64); ok && f == float64(int32(f)) {
			// convert to int
			v = int32(f)
		}
		val := client.ToValue(v)
		if val != nil {
			if err := batch.AddMutation(graph.NQuad{
				Subject:     id,
				Predicate:   k,
				ObjectValue: val,
			}, client.SET); err != nil {
				log.Fatalf("Error creating mutation for (%s, %s): %v\n", id, k, err)
			}
		}
	}
	// parse the geometry object to WKB
	b, err := wkb.Marshal(f.Geometry, binary.LittleEndian)
	if err != nil {
		log.Printf("Error converting geometry to wkb: %s", err)
		return
	}
	if batch.AddMutation(graph.NQuad{
		Subject:     id,
		Predicate:   "geometry",
		ObjectValue: client.Geo(b),
	}, client.SET); err != nil {
		log.Fatalf("Error creating to geometry mutation: %v\n", err)
	}
}

func getID(f *geojson.Feature) (string, bool) {
	if f.ID != "" {
		return f.ID, true
	}
	// Some people define the id in the properties
	idKeys := []string{"id", "ID", "Id"}
	if *idname != "" {
		idKeys = append(idKeys, *idname)
	}
	for _, k := range idKeys {
		if v, ok := f.Properties[k]; ok {
			if s, ok := v.(string); ok && s != "" {
				return s, true
			}
			if f, ok := v.(float64); ok {
				if f == float64(int64(f)) {
					// whole number
					return strconv.FormatInt(int64(f), 10), true
				}
				// floats shouldn't be ids.
				return "", false
			}
		}
	}
	return "", false
}

func parse(r io.Reader, c chan<- result) {
	dec := json.NewDecoder(r)
	defer close(c)
	err := findFeatureArray(dec)
	if err != nil {
		c <- result{nil, err}
		return
	}

	// Read the features one at a time.
	for dec.More() {
		var f geojson.Feature
		err := dec.Decode(&f)
		if err != nil {
			c <- result{nil, err}
			return
		}
		c <- result{&f, nil}
	}
}

func findFeatureArray(dec *json.Decoder) error {
	for {
		t, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if s, ok := t.(string); ok && s == "features" && dec.More() {
			// we found the features element
			d, err := dec.Token()
			if err != nil {
				return err
			}
			if delim, ok := d.(json.Delim); ok {
				if delim.String() == "[" {
					// we have our start of the array
					break
				} else {
					// A different kind of delimiter
					return fmt.Errorf("Expected features to be an array.")
				}
			}
		}
	}

	if !dec.More() {
		return fmt.Errorf("Cannot find any features.")
	}
	return nil
}
