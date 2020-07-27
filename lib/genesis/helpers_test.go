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
	"encoding/hex"
	"encoding/json"
	"io/ioutil"
	"os"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestNewGenesisFromJSON
func TestNewGenesisRawFromJSON(t *testing.T) {
	// Create temp file
	file, err := ioutil.TempFile("", "genesis-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(file.Name())

	testBytes, err := ioutil.ReadFile(file.Name())
	if err != nil {
		t.Fatal(err)
	}

	testHex := hex.EncodeToString(testBytes)
	testRaw := [2]map[string]string{}
	testRaw[0] = map[string]string{"0x3a636f6465": "0x" + testHex}

	expected := TestGenesis
	expected.Genesis = Fields{Raw: testRaw}

	// Grab json encoded bytes
	bz, err := json.Marshal(expected)
	if err != nil {
		t.Fatal(err)
	}
	// Write to temp file
	_, err = file.Write(bz)
	if err != nil {
		t.Fatal(err)
	}

	genesis, err := NewGenesisFromJSONRaw(file.Name())
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(expected, genesis) {
		t.Fatalf("Fail: expected %v got %v", expected, genesis)
	}
}

func TestNewGenesisFromJSON(t *testing.T) {
	var expectedGenesis = &Genesis{}

	expRaw := [2]map[string]string{}
	expRaw[0] = make(map[string]string)
	expRaw[0]["0x3a636f6465"] = "0xfoo"                                                                                                                // raw system code entry
	expRaw[0]["0x3a6772616e6470615f617574686f726974696573"] = "0x010834602b88f60513f1c805d87ef52896934baf6a662bc37414dbdbf69356b1a6910000000000000000" // raw grandpa authorities
	expRaw[0]["0x886726f904d8372fdabb7707870c2fad"] = "0x08d43593c715fdd31c61141abd04a99fd6822c8558854ccde39a5684e7a56da27d0100000000000000"           // raw babe authorities
	expectedGenesis.Genesis = Fields{
		Raw: expRaw,
	}

	// Create temp file
	file, err := ioutil.TempFile("", "genesis_hr-test")
	require.NoError(t, err)

	defer os.Remove(file.Name())

	// create human readable test genesis
	testGenesis := &Genesis{}
	hrData := make(map[string]map[string]interface{})
	hrData["system"] = map[string]interface{}{"code": "0xfoo"} // system code entry
	hrData["babe"] = make(map[string]interface{})
	hrData["babe"]["authorities"] = []interface{}{"5GrwvaEF5zXb26Fz9rcQpDWS57CtERHpNehXCPcNoHGKutQY", 1} // babe authority data
	hrData["grandpa"] = make(map[string]interface{})
	hrData["grandpa"]["authorities"] = []interface{}{"5DFNv4Txc4b88qHqQ6GG4D646QcT4fN3jjS2G3r1PyZkfDut", 0} // grandpa authority data

	testGenesis.Genesis = Fields{
		Runtime: hrData,
	}

	// Grab json encoded bytes
	bz, err := json.Marshal(testGenesis)
	require.NoError(t, err)
	// Write to temp file
	_, err = file.Write(bz)
	require.NoError(t, err)

	// create genesis based on file just created, this will fill Raw field of genesis
	testGenesisProcessed, err := NewGenesisFromJSON(file.Name())
	require.NoError(t, err)

	require.Equal(t, expectedGenesis.Genesis.Raw, testGenesisProcessed.Genesis.Raw)
}
