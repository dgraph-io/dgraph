package genesis

import (
	"encoding/json"
	"io/ioutil"
	"path/filepath"

	"github.com/ChainSafe/gossamer/common"
)

// Genesis stores the data parsed from the genesis configuration file
type Genesis struct {
	Name       string
	ID         string
	Bootnodes  []string
	ProtocolID string
	Genesis    GenesisFields
}

// GenesisData defines the genesis file data formatted for trie storage
type GenesisData struct {
	Name       string
	ID         string
	Bootnodes  [][]byte
	ProtocolID string
}

// GenesisFields stores genesis raw data
type GenesisFields struct {
	Raw [2]map[string]string
}

// LoadGenesisJSONFile parses a JSON formatted genesis file
func LoadGenesisJSONFile(file string) (*Genesis, error) {
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

// GenesisData formats genesis for trie storage
func (g *Genesis) GenesisData() *GenesisData {
	return &GenesisData{
		Name:       g.Name,
		ID:         g.ID,
		Bootnodes:  common.StringArrayToBytes(g.Bootnodes),
		ProtocolID: g.ProtocolID,
	}
}

// GenesisFields returns the genesis fields including genesis raw data
func (g *Genesis) GenesisFields() GenesisFields {
	return g.Genesis
}
