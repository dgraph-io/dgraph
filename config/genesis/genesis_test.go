package genesis

import (
	"encoding/hex"
	"encoding/json"
	"io/ioutil"
	"os"
	"reflect"
	"testing"
)

const TestProtocolID = "/gossamer/test/0"

var TestBootnodes = []string{
	"/dns4/p2p.cc3-0.kusama.network/tcp/30100/p2p/QmeCit3Nif4VfNqrEJsdYHZGcKzRCnZvGxg6hha1iNj4mk",
	"/dns4/p2p.cc3-1.kusama.network/tcp/30100/p2p/QmchDJtEGiEWf7Ag58HNoTg9jSGzxkSZ23VgmF6xiLKKsZ",
}

var TestGenesis = &Genesis{
	Name:       "gossamer",
	ID:         "gossamer",
	Bootnodes:  TestBootnodes,
	ProtocolID: TestProtocolID,
	Genesis:    GenesisFields{},
}

func TestParseGenesisJson(t *testing.T) {
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
	expected.Genesis = GenesisFields{Raw: testRaw}

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

	genesis, err := LoadGenesisFromJSON(file.Name())
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(expected, genesis) {
		t.Fatalf("Fail: expected %v got %v", expected, genesis)
	}
}
