package genesis

import (
	"encoding/json"
	"io/ioutil"
	"path/filepath"
)

// Genesis stores the data parsed from the genesis configuration file
type Genesis struct {
	Name       string
	Id         string
	Bootnodes  []string
	ProtocolId string
	Genesis    GenesisFields
}

type GenesisFields struct {
	Raw map[string]string
}

// LoadGenesisJsonFile parses a JSON formatted genesis file
func LoadGenesisJsonFile(file string) (*Genesis, error) {
	fp, err := filepath.Abs(file)
	if err != nil {
		return nil, err
	}

	data, err := ioutil.ReadFile(filepath.Clean(fp))
	if err != nil {
		return nil, err
	}

	g := new(Genesis)
	err = json.Unmarshal(data, g)
	return g, err
}
