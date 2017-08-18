package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"strconv"
	"strings"

	"github.com/dgraph-io/dgraph/client"
	"github.com/dgraph-io/dgraph/x"
	"github.com/twpayne/go-geom/encoding/geojson"
)

func batchEdges(ech chan client.Edge, dgraphClient *client.Dgraph, che chan error) {
	var batchSize int
	var err error
	for e := range ech {
		// TODO - Handle context.
		//		select {
		//		case <-ctx.Done():
		//			return ctx.Err()
		//		default:
		//		}
		if batchSize >= *numRdf {
			if err = dgraphClient.BatchSet(e); err != nil {
				log.Fatal("While adding mutation to batch: ", err)
			}
			batchSize = 0
		}
	}
	if err != io.EOF {
		x.Checkf(err, "Error while reading file")
	}
	che <- nil
}

func toMutations(f *geojson.Feature, ech chan client.Edge, c *client.Dgraph) {
	id, ok := getID(f)
	if !ok {
		//	log.Println("Skipping feature, cannot find id.")
		return
	}
	n, err := c.NodeBlank(id)
	x.Check(err)

	e := n.Edge("xid")
	err = e.SetValueString(id)
	x.Check(err)
	ech <- e

	fmt.Println("here")
	for k, v := range f.Properties {
		k := strings.Replace(k, " ", "_", -1)
		e = n.Edge(k)

		switch v.(type) {
		case int, int32, int64:
			x.Check(e.SetValueInt(v.(int64)))
		case float32, float64:
			x.Check(e.SetValueFloat(v.(float64)))
		case string:
			x.Check(e.SetValueString(v.(string)))
		case bool:
			x.Check(e.SetValueBool(v.(bool)))
		default:
			continue
		}
		ech <- e
	}

	gf := geojson.Feature{
		ID:         f.ID,
		Geometry:   f.Geometry,
		Properties: make(map[string]interface{}),
	}
	b, err := gf.MarshalJSON()
	x.Check(err)
	gj := string(b)
	// parse the geometry object to WKB
	//	b, err := wkb.Marshal(f.Geometry, binary.LittleEndian)
	//	if err != nil {
	//		log.Printf("Error converting geometry to wkb: %s", err)
	//		return
	//	}
	//
	//	fmt.Println("b", b, "geometry", f.Geometry)
	e = n.Edge("geometry")
	x.Check(e.SetValueGeoJson(gj))
	fmt.Println("set geo val")
	ech <- e
	//	if req.SetMutation(id, "geometry", "", client.Geo(b), ""); err != nil {
	//		log.Fatalf("Error creating to geometry mutation: %v\n", err)
	//	}
	//	out <- req
	//	atomic.AddUint64(&ctr.parsed, 1)
}

func getID(f *geojson.Feature) (string, bool) {
	if f.ID != "" {
		return f.ID, true
	}
	// Some people define the id in the properties
	idKeys := []string{"id", "ID", "Id"}
	if *geoId != "" {
		idKeys = append(idKeys, *geoId)
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

func uploadGeo(r io.Reader, dgraphClient *client.Dgraph) error {
	ech := make(chan client.Edge, 1000)
	che := make(chan error, 1)
	go batchEdges(ech, dgraphClient, che)
	dec := json.NewDecoder(r)
	err := findFeatureArray(dec)
	if err != nil {
		return err
	}

	count := 0
	// Read the features one at a time.
	for dec.More() {
		if count >= 10 {
			break
		}
		count++
		var f geojson.Feature
		err := dec.Decode(&f)
		if err != nil {
			return err
		}
		toMutations(&f, ech, dgraphClient)
		//	atomic.AddUint64(&ctr.read, 1)
	}
	return <-che
}
