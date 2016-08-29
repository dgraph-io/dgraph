package bidx

import (
	"bufio"
	"encoding/json"
	"io"
	"io/ioutil"
	"os"
	"path"

	"github.com/dgraph-io/dgraph/x"
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
	// Add more checks here in the future.
	attrMap := make(map[string]bool)
	for _, c := range is.Config {
		// Check that there are no duplicates in attributes.
		if attrMap[c.Attribute] {
			return x.Errorf("Duplicate attr %s", c.Attribute)
		}
		attrMap[c.Attribute] = true
		// Check NumShards >= 1.
		if c.NumShards < 1 {
			return x.Errorf("NumShards too small %d", c.NumShards)
		}
	}
	return nil
}

func (is *IndicesConfig) Write(basedir string) error {
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
