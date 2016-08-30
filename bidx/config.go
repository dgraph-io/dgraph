/*
 * Copyright 2016 DGraph Labs, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * 		http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */
package bidx

import (
	"bufio"
	"encoding/json"
	"flag"
	"io"
	"io/ioutil"
	"os"
	"path"

	"github.com/dgraph-io/dgraph/x"
)

var (
	defaultConfigFile = flag.String("index_config", "config.json",
		"Filename of JSON config file inside indices directory")
)

// IndexConfig defines the index for a single predicate. Each predicate should
// have at most one index.
type IndexConfig struct {
	Type      string
	Attribute string
	NumChild  int
}

// IndicesConfig is a list of IndexConfig. We may add more fields in future.
type IndicesConfig struct {
	Config []*IndexConfig
}

func getDefaultConfig(basedir string) string {
	return path.Join(basedir, *defaultConfigFile)
}

// NewIndicesConfig creates IndicesConfig from io.Reader object.
func NewIndicesConfig(reader io.Reader) (*IndicesConfig, error) {
	f, err := ioutil.ReadAll(reader)
	if err != nil {
		return nil, x.Wrap(err)
	}
	is := &IndicesConfig{}
	err = json.Unmarshal(f, is)
	if err != nil {
		return nil, x.Wrap(err)
	}
	err = is.validate()
	if err != nil {
		return nil, err
	}
	return is, nil
}

func (is *IndicesConfig) validate() error {
	// TODO(jchiu): Add more checks here in the future.
	attrMap := make(map[string]bool)
	for _, c := range is.Config {
		// Check that there are no duplicates in attributes.
		if attrMap[c.Attribute] {
			return x.Errorf("Duplicate attr %s", c.Attribute)
		}
		attrMap[c.Attribute] = true
		if c.NumChild < 1 {
			return x.Errorf("NumChild too small %d", c.NumChild)
		}
	}
	return nil
}

func (is *IndicesConfig) write(basedir string) error {
	f, err := os.Create(getDefaultConfig(basedir))
	if err != nil {
		return x.Wrap(err)
	}
	defer f.Close()

	js, err := json.MarshalIndent(is, "", "    ")
	if err != nil {
		return x.Wrap(err)
	}

	w := bufio.NewWriter(f)
	defer w.Flush()

	_, err = w.Write(js)
	if err != nil {
		return x.Wrap(err)
	}
	return nil
}
