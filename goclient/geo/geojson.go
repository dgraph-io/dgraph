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
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"context"

	geom "github.com/twpayne/go-geom"
	"github.com/twpayne/go-geom/encoding/geojson"
	"github.com/twpayne/go-geom/encoding/wkb"

	"github.com/dgraph-io/dgraph/goclient/client"
	"github.com/dgraph-io/dgraph/query/graph"
)

type counters struct {
	read    uint64
	parsed  uint64
	writing uint64
	done    uint64
}

var idname = flag.String("geoid", "", "The name of property to use as the xid")
var ctr *counters

// Upload a geojson file to the server by converting it into the NQuads format.
func Upload(c graph.DgraphClient, r io.Reader) {
	cfeat := make(chan result, 1000)
	creq := make(chan client.Req, 1000)
	ctr = new(counters)
	ticker := time.NewTicker(time.Second)
	go func() {
		for _ = range ticker.C {
			printCounters()
			log.Println("len_cfeat", len(cfeat),
				"len_creq", len(creq))
		}
	}()

	go parse(r, cfeat)
	go createRequest(cfeat, creq)

	maxConcurrent := 20
	var wg sync.WaitGroup
	wg.Add(maxConcurrent)
	for i := 0; i < maxConcurrent; i++ {
		go makeRequests(&wg, c, creq)
	}
	wg.Wait()
	ticker.Stop()
	printCounters()
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

func printCounters() {
	log.Println(
		"read", atomic.LoadUint64(&ctr.read),
		"parsed", atomic.LoadUint64(&ctr.parsed),
		"writing", atomic.LoadUint64(&ctr.writing),
		"done", atomic.LoadUint64(&ctr.done))
}

func makeRequests(wg *sync.WaitGroup, c graph.DgraphClient, in <-chan client.Req) {
	defer wg.Done()
	ctx := context.Background()
	for req := range in {
		atomic.AddUint64(&ctr.writing, 1)
		_, err := c.Query(ctx, req.Request())
		if err != nil {
			log.Printf("Error in getting response from server, %v\n", err)
		}
		atomic.AddUint64(&ctr.done, 1)
	}
}

type result struct {
	f   *geojson.Feature
	err error
}

func createRequest(in <-chan result, out chan<- client.Req) {
	defer close(out)
	for v := range in {
		if v.err != nil {
			log.Printf("Error parsing file: %v\n", v.err)
			return
		}
		toMutations(v.f, out)
	}
}

func toMutations(f *geojson.Feature, out chan<- client.Req) {
	id, ok := getID(f)
	if !ok {
		log.Println("Skipping feature, cannot find id.")
		return
	}

	req := client.NewRequest()
	for k, v := range f.Properties {
		k := strings.Replace(k, " ", "_", -1)
		if f, ok := v.(float64); ok && f == float64(int32(f)) {
			// convert to int
			v = int32(f)
		}
		val := client.ToValue(v)
		if val != nil {
			if err := req.SetMutation(id, k, "", val, ""); err != nil {
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
	if req.SetMutation(id, "geometry", "", client.Geo(b), ""); err != nil {
		log.Fatalf("Error creating to geometry mutation: %v\n", err)
	}
	out <- req
	atomic.AddUint64(&ctr.parsed, 1)
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
		atomic.AddUint64(&ctr.read, 1)
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
