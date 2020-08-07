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
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/big"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/ChainSafe/gossamer/dot/types"
	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/ChainSafe/gossamer/lib/crypto"
	"github.com/ChainSafe/gossamer/lib/crypto/ed25519"
	"github.com/ChainSafe/gossamer/lib/crypto/sr25519"
	"github.com/ChainSafe/gossamer/lib/runtime"
	"github.com/ChainSafe/gossamer/lib/scale"
	"github.com/ChainSafe/gossamer/lib/trie"
)

// NewGenesisFromJSONRaw parses a JSON formatted genesis-raw file
func NewGenesisFromJSONRaw(file string) (*Genesis, error) {
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

// NewTrieFromGenesis creates a new trie from the raw genesis data
func NewTrieFromGenesis(g *Genesis) (*trie.Trie, error) {
	t := trie.NewEmptyTrie()

	r := g.GenesisFields().Raw[0]

	err := t.Load(r)
	if err != nil {
		return nil, fmt.Errorf("failed to create trie from genesis: %s", err)
	}

	return t, nil
}

// NewGenesisBlockFromTrie creates a genesis block from the provided trie
func NewGenesisBlockFromTrie(t *trie.Trie) (*types.Header, error) {

	// create state root from trie hash
	stateRoot, err := t.Hash()
	if err != nil {
		return nil, fmt.Errorf("failed to create state root from trie hash: %s", err)
	}

	// create genesis block header
	header, err := types.NewHeader(
		common.NewHash([]byte{0}), // parentHash
		big.NewInt(0),             // number
		stateRoot,                 // stateRoot
		trie.EmptyHash,            // extrinsicsRoot
		[][]byte{},                // digest
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create genesis block header: %s", err)
	}

	return header, nil
}

// NewRuntimeFromGenesis creates a runtime instance from the genesis data
func NewRuntimeFromGenesis(g *Genesis, storage runtime.Storage) (*runtime.Runtime, error) {
	codeStr := g.GenesisFields().Raw[0][common.BytesToHex(common.CodeKey)]
	if codeStr == "" {
		return nil, fmt.Errorf("cannot find :code in genesis")
	}

	code := common.MustHexToBytes(codeStr)
	cfg := &runtime.Config{
		Storage: storage,
		Imports: runtime.RegisterImports_NodeRuntime,
	}

	return runtime.NewRuntime(code, cfg)
}

// NewGenesisFromJSON parses Human Readable JSON formatted genesis file
func NewGenesisFromJSON(file string) (*Genesis, error) {
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
	if err != nil {
		return nil, err
	}

	grt := g.Genesis.Runtime
	res, err := buildRawMap(grt)
	if err != nil {
		return nil, err
	}

	g.Genesis.Raw[0] = res

	return g, err
}

// keyValue struct to hold data regarding entry
type keyValue struct {
	key      []string
	value    string
	valueLen *big.Int
}

func buildRawMap(m map[string]map[string]interface{}) (map[string]string, error) {
	res := make(map[string]string)
	for k, v := range m {
		kv := new(keyValue)
		kv.key = append(kv.key, k)
		buildRawMapInterface(v, kv)

		key, err := formatKey(kv.key)
		if err != nil {
			return nil, err
		}

		value, err := formatValue(kv)
		if err != nil {
			return nil, err
		}
		res[key] = value
	}
	return res, nil
}

func buildRawMapInterface(m map[string]interface{}, kv *keyValue) {
	for k, v := range m {
		kv.key = append(kv.key, k)
		switch v2 := v.(type) {
		case []interface{}:
			kv.valueLen = big.NewInt(int64(len(v2)))
			buildRawArrayInterface(v2, kv)
		case string:
			kv.value = v2
		}
	}
}

func buildRawArrayInterface(a []interface{}, kv *keyValue) {
	for _, v := range a {
		switch v2 := v.(type) {
		case []interface{}:
			buildRawArrayInterface(v2, kv)
		case string:
			// todo check to confirm it's an address
			tba := crypto.PublicAddressToByteArray(common.Address(v2))
			kv.value = kv.value + fmt.Sprintf("%x", tba)
		case float64:
			encVal, err := scale.Encode(uint64(v2))
			if err != nil {
				//todo determine how to handle this error
			}
			kv.value = kv.value + fmt.Sprintf("%x", encVal)
		}
	}
}

func formatKey(key []string) (string, error) {
	switch true {
	case reflect.DeepEqual([]string{"grandpa", "authorities"}, key):
		kb := []byte(`:grandpa_authorities`)
		return common.BytesToHex(kb), nil
	case reflect.DeepEqual([]string{"system", "code"}, key):
		kb := []byte(`:code`)
		return common.BytesToHex(kb), nil
	default:
		var fKey string
		for _, v := range key {
			fKey = fKey + v + " "
		}
		fKey = strings.Trim(fKey, " ")
		fKey = strings.Title(fKey)
		kb, err := common.Twox128Hash([]byte(fKey))
		if err != nil {
			return "", err
		}
		return common.BytesToHex(kb), nil
	}
}

func formatValue(kv *keyValue) (string, error) {
	switch true {
	case reflect.DeepEqual([]string{"grandpa", "authorities"}, kv.key):
		if kv.valueLen != nil {
			lenEnc, err := scale.Encode(kv.valueLen)
			if err != nil {
				return "", err
			}
			// prepend 01 to grandpa_authorities values
			return fmt.Sprintf("0x01%x%v", lenEnc, kv.value), nil
		}
		return "", fmt.Errorf("error formatting value for grandpa authorities")
	case reflect.DeepEqual([]string{"system", "code"}, kv.key):
		return kv.value, nil
	default:
		if kv.valueLen != nil {
			lenEnc, err := scale.Encode(kv.valueLen)
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("0x%x%v", lenEnc, kv.value), nil
		}
		return fmt.Sprintf("0x%x", kv.value), nil
	}
}

// BuildFromMap builds genesis fields data from map
func BuildFromMap(m map[string][]byte, gen *Genesis) error {
	for k, v := range m {
		key := fmt.Sprintf("0x%x", k)
		switch key {

		case "0x3a636f6465":
			// handle :code
			addCodeValue(v, gen)
			addRawValue(key, v, gen)
		case "0x3a6772616e6470615f617574686f726974696573":
			// handle :grandpa_authorities
			//  slice value since it was encoded starting with 0x01
			err := addAuthoritiesValues("grandpa", "authorities", crypto.Ed25519Type, v[1:], gen)
			if err != nil {
				return err
			}
			addRawValue(key, v, gen)
		case "0x886726f904d8372fdabb7707870c2fad":
			// handle Babe Authorities
			err := addAuthoritiesValues("babe", "authorities", crypto.Sr25519Type, v, gen)
			if err != nil {
				return err
			}
			addRawValue(key, v, gen)
		}
	}
	return nil
}

func addRawValue(key string, value []byte, gen *Genesis) {
	if gen.Genesis.Raw[0] == nil {
		gen.Genesis.Raw[0] = make(map[string]string)
	}
	gen.Genesis.Raw[0][key] = common.BytesToHex(value)
}

func addCodeValue(value []byte, gen *Genesis) {
	if gen.Genesis.Runtime["system"] == nil {
		gen.Genesis.Runtime["system"] = make(map[string]interface{})
	}
	gen.Genesis.Runtime["system"]["code"] = common.BytesToHex(value)
}

func addAuthoritiesValues(k1, k2 string, kt crypto.KeyType, value []byte, gen *Genesis) error {
	if gen.Genesis.Runtime[k1] == nil {
		gen.Genesis.Runtime[k1] = make(map[string]interface{})
	}

	// decode authorities values into []interface that will be decoded into json array
	ava := [][]interface{}{}
	buf := &bytes.Buffer{}
	sd := scale.Decoder{Reader: buf}
	_, err := buf.Write(value)
	if err != nil {
		return err
	}

	alen, err := sd.DecodeInteger()
	if err != nil {
		return err
	}
	for i := 0; i < int(alen); i++ {
		auth := []interface{}{}
		buf := make([]byte, 32)
		if _, err = sd.Reader.Read(buf); err == nil {
			var arr = [32]byte{}
			copy(arr[:], buf)
			pa, err := bytesToAddress(kt, arr[:])
			if err != nil {
				return err
			}
			auth = append(auth, pa)
		}
		iv, err := sd.DecodeFixedWidthInt(uint64(0))
		if err != nil {
			return err
		}
		auth = append(auth, iv.(uint64))
		ava = append(ava, auth)
	}

	gen.Genesis.Runtime[k1][k2] = ava
	return nil
}

func bytesToAddress(kt crypto.KeyType, v []byte) (common.Address, error) {
	var pk crypto.PublicKey
	var err error
	switch kt {
	case crypto.Ed25519Type:
		pk, err = ed25519.NewPublicKey(v)
	case crypto.Sr25519Type:
		pk, err = sr25519.NewPublicKey(v)
	}
	if err != nil {
		return "", err
	}
	return crypto.PublicKeyToAddress(pk), nil
}
