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
)

// TestNewGenesisFromJSON
func TestNewGenesisFromJSON(t *testing.T) {
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

	genesis, err := NewGenesisFromJSON(file.Name())
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(expected, genesis) {
		t.Fatalf("Fail: expected %v got %v", expected, genesis)
	}
}
