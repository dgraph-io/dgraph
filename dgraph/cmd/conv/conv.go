/*
 * Copyright 2018 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package conv

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/dgraph-io/dgraph/x"
	"github.com/paulmach/go.geojson"
)

// TODO: Reconsider if we need this binary.
func writeToFile(fpath string, ch chan []byte) error {
	f, err := os.Create(fpath)
	if err != nil {
		return err
	}

	defer f.Close()
	x.Check(err)
	w := bufio.NewWriterSize(f, 1e6)
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
	f, err := os.Open(input)
	if err != nil {
		return err
	}
	defer f.Close()

	var gz io.Reader
	gz = f
	if filepath.Ext(input) == ".gz" {
		gz, err = gzip.NewReader(f)
		if err != nil {
			return err
		}
	}

	// TODO - This might not be a good idea for large files. Use json.Decode to read features.
	b, err := ioutil.ReadAll(gz)
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

	fc := geojson.NewFeatureCollection()
	err = json.Unmarshal(b, fc)
	if err != nil {
		return err
	}

	count := 0
	rdfCount := 0
	for _, f := range fc.Features {
		b, err := json.Marshal(f.Geometry)
		if err != nil {
			return err
		}

		geometry := strings.Replace(string(b), `"`, "'", -1)
		bn := fmt.Sprintf("_:%s-%d", name, count)
		rdf := fmt.Sprintf("%s <%s> \"%s\"^^<geo:geojson> .\n", bn, opt.geopred, geometry)
		chb <- []byte(rdf)

		for k := range f.Properties {
			// TODO - Support other types later.
			if str, err := f.PropertyString(k); err == nil {
				rdfCount++
				rdf = fmt.Sprintf("%s <%s> \"%s\" .\n", bn, k, str)
				chb <- []byte(rdf)
			}
		}
		count++
		rdfCount++
		if count%1000 == 0 {
			fmt.Printf("%d features converted\r", count)
		}
	}
	close(chb)
	fmt.Printf("%d features converted. %d rdf's generated\n", count, rdfCount)
	return <-che
}
