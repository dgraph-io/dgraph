package bidx

import (
	"bufio"
	"encoding/json"
	"io/ioutil"
	"os"
	"path"
)

type IndexConfig struct {
	Type      string
	Attribute string
	NumShards int
}

type IndicesConfig struct {
	Config []*IndexConfig
}

const (
	// Default filename for storing config in indices base directory.
	defaultConfigFile = "config.json"
)

func getDefaultConfig(basedir string) string {
	return path.Join(basedir, defaultConfigFile)
}

func NewIndicesConfig(filename string) (*IndicesConfig, error) {
	f, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	is := &IndicesConfig{}
	err = json.Unmarshal(f, is)
	if err != nil {
		return nil, err
	}
	// TODO: Add some validation of config, e.g, no duplicate in attributes.
	return is, nil
}

func (is *IndicesConfig) Write(basedir string) error {
	f, err := os.Create(getDefaultConfig(basedir))
	if err != nil {
		return err
	}
	defer f.Close()

	js, err := json.MarshalIndent(is, "", "    ")
	if err != nil {
		return err
	}

	w := bufio.NewWriter(f)
	defer w.Flush()

	_, err = w.Write(js)
	if err != nil {
		return err
	}
	return nil
}
