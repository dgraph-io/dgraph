/*
 * Copyright 2016 Dgraph Labs, Inc.
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

package index

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
	// Every directory containing index data will have this file.
	defaultConfigFile = flag.String("index_config", "config.json",
		"Filename of JSON config file inside indices directory")
)

// Configs is a list of configs.
type Configs struct {
	Cfg     []*Config `json:"Config"`
	Indexer string    // Which indexer to use, e.g., memtable.
}

// Config defines the index for a single predicate.
type Config struct {
	Attr string `json:"Attribute"`
	// Might add more fields in the future, e.g., field can be datetime or numeric.
}

func getDefaultConfig(dir string) string {
	return path.Join(dir, *defaultConfigFile)
}

// ReadConfigs creates Configs object from io.Reader object.
func ReadConfigs(reader io.Reader) (*Configs, error) {
	f, err := ioutil.ReadAll(reader)
	if err != nil {
		return nil, x.Wrap(err)
	}
	cfg := &Configs{}
	err = json.Unmarshal(f, cfg)
	if err != nil {
		return nil, x.Wrap(err)
	}
	err = cfg.validate()
	if err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *Configs) validate() error {
	attrMap := make(map[string]bool)
	for _, cfg := range c.Cfg {
		// Check that there are no duplicates in attributes.
		if attrMap[cfg.Attr] {
			return x.Errorf("Duplicate attr %s", cfg.Attr)
		}
		attrMap[cfg.Attr] = true
	}
	return nil
}

// write writes to a directory's default config file location. This is used for
// storing an internal copy of the given config, so that we get rid of any
// extraneous fields and do some formatting / normalization.
func (c *Configs) write(dir string) error {
	f, err := os.Create(getDefaultConfig(dir))
	if err != nil {
		return x.Wrap(err)
	}
	defer f.Close()

	js, err := json.MarshalIndent(c, "", "    ")
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
