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
	defaultConfigFile = flag.String("index_config", "config.json",
		"Filename of JSON config file inside indices directory")
)

// IndicesConfig is a list of IndexConfig. We may add more fields in future.
type Configs struct {
	Cfg []*Config `json:"Config"`
}

// IndexConfig defines the index for a single predicate. Each predicate should
// have at most one index.
type Config struct {
	Type      string
	Attribute string
	NumChild  int
}

func getDefaultConfig(basedir string) string {
	return path.Join(basedir, *defaultConfigFile)
}

// NewConfigs creates Configs object from io.Reader object.
func NewConfigs(reader io.Reader) (*Configs, error) {
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
	// TODO(jchiu): Add more checks here in the future.
	attrMap := make(map[string]bool)
	for _, cfg := range c.Cfg {
		// Check that there are no duplicates in attributes.
		if attrMap[cfg.Attribute] {
			return x.Errorf("Duplicate attr %s", cfg.Attribute)
		}
		attrMap[cfg.Attribute] = true
		if cfg.NumChild < 1 {
			return x.Errorf("NumChild too small %d", cfg.NumChild)
		}
	}
	return nil
}

// write writes to a directory's default config file location.
func (c *Configs) write(basedir string) error {
	f, err := os.Create(getDefaultConfig(basedir))
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
