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

package main

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/dgraph-io/dgraph/x"
	"github.com/paulmach/go.geojson"
)

var (
	geoFile    = flag.String("geo", "", "Location of geo file to convert")
	outputFile = flag.String("out", "output.rdf.gz", "Location of output rdf.gz file")
	geoPred    = flag.String("geopred", "loc", "Predicate to use to store geometries")
)

func writeToFile(fpath string, ch chan []byte) error {
	f, err := os.Create(fpath)
	if err != nil {
		return err
	}

	defer f.Close()
	x.Check(err)
	w := bufio.NewWriterSize(f, 1000000)
	gw, err := gzip.NewWriterLevel(w, gzip.BestCompression)
	if err != nil {
		return err
	}

	for buf := range ch {
		if _, err := gw.Write(buf); err != nil {
			return err
		}
	}
	if err := gw.Flush(); err != nil {
		return err
	}
	if err := gw.Close(); err != nil {
		return err
	}
	return w.Flush()
}

func convertGeoFile(input string, output string) error {
	fmt.Printf("\nProcessing %s\n\n", input)
	b, err := ioutil.ReadFile(input)
	if err != nil {
		return err
	}
	basename := filepath.Base(input)
	name := strings.TrimSuffix(basename, filepath.Ext(basename))

	che := make(chan error, 1)
	chb := make(chan []byte, 1000)
	go func() {
		che <- writeToFile(output, chb)
	}()

	fc1 := geojson.NewFeatureCollection()
	err = json.Unmarshal(b, fc1)
	if err != nil {
		return err
	}

	count := 0
	for _, f := range fc1.Features {
		b, err := json.Marshal(f.Geometry)
		if err != nil {
			return err
		}

		geometry := strings.Replace(string(b), `"`, "'", -1)
		bn := fmt.Sprintf("_:%s-%d", name, count)
		rdf := fmt.Sprintf("%s <%s> \"%s\"^^<geo:geojson> .\n", bn, *geoPred, geometry)
		chb <- []byte(rdf)

		for k, _ := range f.Properties {
			// Maybe support other types later.
			if str, err := f.PropertyString(k); err == nil {
				rdf = fmt.Sprintf("%s <%s> \"%s\" .\n", bn, k, str)
				chb <- []byte(rdf)
			}
		}
		count++
		if count%1000 == 0 {
			fmt.Printf("%d features converted\r", count)
		}
	}
	close(chb)
	fmt.Printf("%d features converted", count)

	return <-che
}

func main() {
	flag.Parse()
	if len(*geoFile) > 0 {
		x.Check(convertGeoFile(*geoFile, *outputFile))
	}
}
