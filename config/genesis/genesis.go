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
	Id         string
	Bootnodes  []string
	ProtocolId string
	Genesis    GenesisFields
}

// Genesis stores the data parsed from the genesis configuration file
type GenesisData struct {
	Name          string
	Id            string
	Bootnodes     [][]byte
	ProtocolId    string
	genesisFields GenesisFields
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

func LoadGenesisData(file string) (*GenesisData, error) {
	g, err := LoadGenesisJsonFile(file)
	if err != nil {
		return nil, err
	}

	return &GenesisData{
		Name:          g.Name,
		Id:            g.Id,
		Bootnodes:     common.StringArrayToBytes(g.Bootnodes),
		ProtocolId:    g.ProtocolId,
		genesisFields: g.Genesis,
	}, nil
}

func (g *GenesisData) GenesisFields() GenesisFields {
	return g.genesisFields
}
