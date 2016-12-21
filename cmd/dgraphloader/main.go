// This script is used to load data into Dgraph from an RDF file by performing
// mutations using the HTTP interface.
//
// You can run the script like
// go build . && ./dgraphloader -r path-to-gzipped-rdf.gz
package main

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc"

	"github.com/dgraph-io/dgraph/goclient/client"
	"github.com/dgraph-io/dgraph/query/graph"
	"github.com/dgraph-io/dgraph/rdf"
	"github.com/dgraph-io/dgraph/x"
	geom "github.com/twpayne/go-geom"
	"github.com/twpayne/go-geom/encoding/geojson"
	"github.com/twpayne/go-geom/encoding/wkb"
)

var (
	files   = flag.String("r", "", "Location of rdf files to load")
	dgraph  = flag.String("d", "127.0.0.1:8080", "Dgraph server address")
	geoJSON = flag.String("json", "", "Json file from which to upload geo data")
	idname  = flag.String("geoid", "", "The name of property to use as the xid")
)

// Reads a single line from a buffered reader. The line is read into the
// passed in buffer to minimize allocations. This is the preferred
// method for loading long lines which could be longer than the buffer
// size of bufio.Scanner.
func readLine(r *bufio.Reader, buf *bytes.Buffer) error {
	isPrefix := true
	var err error
	for isPrefix && err == nil {
		var line []byte
		// The returned line is an internal buffer in bufio and is only
		// valid until the next call to ReadLine. It needs to be copied
		// over to our own buffer.
		line, isPrefix, err = r.ReadLine()
		if err == nil {
			buf.Write(line)
		}
	}
	return err
}

// processFile sends mutations for a given gz file.
func processFile(file string, batch *client.BatchMutation) {
	fmt.Printf("\nProcessing %s\n", file)
	f, err := os.Open(file)
	x.Check(err)
	defer f.Close()
	gr, err := gzip.NewReader(f)
	x.Check(err)

	var buf bytes.Buffer
	bufReader := bufio.NewReader(gr)
	for {
		err = readLine(bufReader, &buf)
		if err != nil {
			break
		}
		nq, err := rdf.Parse(buf.String())
		if err != nil {
			log.Fatal("While parsing RDF: ", err)
		}
		buf.Reset()
		if err = batch.AddMutation(nq, client.SET); err != nil {
			log.Fatal("While adding mutation to batch: ", err)
		}
	}
	if err != io.EOF {
		x.Checkf(err, "Error while reading file")
	}
}

func printCounters(batch *client.BatchMutation) {
	ticker := time.NewTicker(2 * time.Second)
	for range ticker.C {
		c := batch.Counter()
		rate := float64(c.Rdfs) / c.Elapsed.Seconds()
		fmt.Printf("[Request: %6d] Total RDFs done: %8d RDFs per second: %7.0f\r", c.Mutations, c.Rdfs, rate)
	}
}

func uploadJSON(json string, batch *client.BatchMutation) {
	f, err := os.Open(json)
	if err != nil {
		log.Fatalf("Error opening file %s: %v", json, err)
	}
	defer f.Close()

	var r io.Reader
	r = f

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
	var wg sync.WaitGroup
	wg.Add(1)
	go createRequest(cfeat, batch, &wg)
	wg.Wait()
	batch.Flush()
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

func createRequest(in <-chan result, batch *client.BatchMutation, wg *sync.WaitGroup) {
	for v := range in {
		if v.err != nil {
			log.Fatalf("Error parsing file: %v\n", v.err)
			return
		}
		toMutations(v.f, batch)
	}
	wg.Done()
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

func main() {
	x.Init()

	var err error
	conn, err := grpc.Dial(*dgraph, grpc.WithInsecure())
	if err != nil {
		log.Fatal("DialTCPConnection")
	}
	defer conn.Close()

	batch := client.NewBatchMutation(context.Background(), conn, 1000, 10)
	go printCounters(batch)

	if *geoJSON != "" {
		uploadJSON(*geoJSON, batch)
		return
	}

	filesList := strings.Split(*files, ",")
	x.AssertTrue(len(filesList) > 0)
	for _, file := range filesList {
		processFile(file, batch)
	}
	batch.Flush()

	c := batch.Counter()
	// Lets print an empty line, otherwise Number of Mutations overwrites the previous
	// printed line.
	fmt.Printf("%100s\r", "")
	fmt.Printf("Number of mutations run   : %d\n", c.Mutations)
	fmt.Printf("Number of RDFs processed  : %d\n", c.Rdfs)
	fmt.Printf("Time spent                : %v\n", c.Elapsed)
	fmt.Printf("RDFs processed per second : %d\n", c.Rdfs/uint64(c.Elapsed.Seconds()))
}
