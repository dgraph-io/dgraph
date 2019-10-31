package genesis

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"reflect"
	"testing"
)

func TestParseGenesisJson(t *testing.T) {
	expected := &Genesis{
		Name:       "gossamer",
		Id:         "gossamer",
		Bootnodes:  []string{"/ip4/104.211.54.233/tcp/30363/p2p/16Uiu2HAmFWPUx45xYYeCpAryQbvU3dY8PWGdMwS2tLm1dB1CsmCj"},
		ProtocolId: "gossamer",
		Genesis: GenesisFields{
			Raw: map[string]string{"0x3a636f6465": "0x00"},
		},
	}

	// Create temp file
	file, err := ioutil.TempFile("", "genesis-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(file.Name())

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

	genesis, err := LoadGenesisJsonFile(file.Name())
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(expected, genesis) {
		t.Fatalf("Fail: expected %v got %v", expected, genesis)
	}
}
