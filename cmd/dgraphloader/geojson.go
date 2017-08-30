package main

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log"

	"github.com/dgraph-io/dgraph/client"
	"github.com/dgraph-io/dgraph/x"
	"github.com/twpayne/go-geom/encoding/geojson"
	"github.com/twpayne/go-geom/encoding/wkb"
)

func toMutations(f *geojson.Feature, c *client.Dgraph) {
	n, err := c.NodeBlank("")
	x.Check(err)

	g, err := wkb.Marshal(f.Geometry, binary.LittleEndian)
	x.Check(err)
	e := n.Edge(*geoPredicate)
	x.Check(e.SetValueGeoBytes(g))
	if err = c.BatchSet(e); err != nil {
		log.Fatal("While adding mutation to batch: ", err)
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

func processGeoFile(ctx context.Context, file string, dgraphClient *client.Dgraph) error {
	r, f := fileReader(file)
	defer f.Close()
	dec := json.NewDecoder(r)
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
