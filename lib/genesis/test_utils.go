// Copyright 2019 ChainSafe Systems (ON) Corp.
// This file is part of gossamer.
//
// The gossamer library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The gossamer library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the gossamer library. If not, see <http://www.gnu.org/licenses/>.

package genesis

import (
	"encoding/json"
	"io/ioutil"
)

const testProtocolID = "/gossamer/test/0"

var testBootnodes = []string{
	"/dns4/p2p.cc3-0.kusama.network/tcp/30100/p2p/QmeCit3Nif4VfNqrEJsdYHZGcKzRCnZvGxg6hha1iNj4mk",
	"/dns4/p2p.cc3-1.kusama.network/tcp/30100/p2p/QmchDJtEGiEWf7Ag58HNoTg9jSGzxkSZ23VgmF6xiLKKsZ",
}

// TestGenesis instance of Genesis struct for testing
var TestGenesis = &Genesis{
	Name:       "gossamer",
	ID:         "gossamer",
	Bootnodes:  testBootnodes,
	ProtocolID: testProtocolID,
}

// TestFieldsHR instance of human-readable Fields struct for testing, use with TestGenesis
var TestFieldsHR = Fields{
	Raw: [2]map[string]string{},
	Runtime: map[string]map[string]interface{}{
		"system": {
			"code": "mocktestcode",
		},
	},
}

// TestFieldsRaw instance of raw Fields struct for testing use with TestGenesis
var TestFieldsRaw = Fields{
	Raw: [2]map[string]string{
		0: {"0x3a636f6465": "mocktestcode"},
	},
}

// CreateTestGenesisJSONFile utility to create mock test genesis JSON file
func CreateTestGenesisJSONFile(asRaw bool) (string, error) {
	// Create temp file
	file, err := ioutil.TempFile("", "genesis-test")
	if err != nil {
		return "", err
	}

	tGen := &Genesis{
		Name:       "test",
		ID:         "",
		Bootnodes:  nil,
		ProtocolID: "",
		Genesis:    Fields{},
	}

	if asRaw {
		tGen.Genesis = Fields{
			Raw: [2]map[string]string{},
			Runtime: map[string]map[string]interface{}{
				"system": {
					"code": "mocktestcode",
				},
			},
		}
	} else {
		tGen.Genesis = TestFieldsHR
	}

	bz, err := json.Marshal(tGen)
	if err != nil {
		return "", nil
	}
	// Write to temp file
	_, err = file.Write(bz)
	if err != nil {
		return "", nil
	}

	return file.Name(), nil
}
