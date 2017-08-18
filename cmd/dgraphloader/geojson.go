package main

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"strconv"

	"github.com/dgraph-io/dgraph/client"
	"github.com/dgraph-io/dgraph/x"
	"github.com/twpayne/go-geom/encoding/geojson"
	"github.com/twpayne/go-geom/encoding/wkb"
)

func toMutations(f *geojson.Feature, c *client.Dgraph) {
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
	if err = c.BatchSet(e); err != nil {
		log.Fatal("While adding mutation to batch: ", err)
	}

	fmt.Println("here")
	g, err := wkb.Marshal(f.Geometry, binary.LittleEndian)
	x.Check(err)
	e = n.Edge("geometry")
	x.Check(e.SetValueGeoBytes(g))
	fmt.Println("set geo val")
	if err = c.BatchSet(e); err != nil {
		log.Fatal("While adding mutation to batch: ", err)
	}
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

func processGeoFile(ctx context.Context, file string, dgraphClient *client.Dgraph) error {
	r, f := gzipReader(file)
	defer f.Close()
	dec := json.NewDecoder(&r)
	err := findFeatureArray(dec)
	if err != nil {
		return err
	}

	// Read the features one at a time.
	for dec.More() {
		var f geojson.Feature
		err := dec.Decode(&f)
		if err != nil {
			return err
		}
		toMutations(&f, dgraphClient)
	}
	return nil
}
