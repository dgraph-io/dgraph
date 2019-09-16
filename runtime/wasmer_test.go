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

package runtime

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/ChainSafe/gossamer/common"
	"github.com/ChainSafe/gossamer/trie"
	"golang.org/x/crypto/ed25519"
)

const POLKADOT_RUNTIME_FP string = "../polkadot_runtime.wasm"
const POLKADOT_RUNTIME_URL string = "https://github.com/w3f/polkadot-re-tests/blob/master/polkadot-runtime/polkadot_runtime.compact.wasm?raw=true"

// getRuntimeBlob checks if the polkadot runtime wasm file exists and if not, it fetches it from github
func getRuntimeBlob() (n int64, err error) {
	if Exists(POLKADOT_RUNTIME_FP) {
		return 0, nil
	}

	out, err := os.Create(POLKADOT_RUNTIME_FP)
	if err != nil {
		return 0, err
	}
	defer out.Close()

	resp, err := http.Get(POLKADOT_RUNTIME_URL)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	n, err = io.Copy(out, resp.Body)
	return n, err
}

// Exists reports whether the named file or directory exists.
func Exists(name string) bool {
	if _, err := os.Stat(name); err != nil {
		if os.IsNotExist(err) {
			return false
		}
	}
	return true
}

func newRuntime(t *testing.T) (*Runtime, error) {
	_, err := getRuntimeBlob()
	if err != nil {
		t.Fatalf("Fail: could not get polkadot runtime")
	}

	fp, err := filepath.Abs(POLKADOT_RUNTIME_FP)
	if err != nil {
		t.Fatal("could not create filepath")
	}

	tt := &trie.Trie{}

	r, err := NewRuntime(fp, tt)
	if err != nil {
		t.Fatal(err)
	} else if r == nil {
		t.Fatal("did not create new VM")
	}

	return r, err
}

func TestExecVersion(t *testing.T) {
	expected := &Version{
		Spec_name:         []byte("kusama"),
		Impl_name:         []byte("parity-kusama"),
		Authoring_version: 1,
		Spec_version:      1002,
		Impl_version:      0,
	}

	r, err := newRuntime(t)
	if err != nil {
		t.Fatal(err)
	}

	ret, err := r.Exec("Core_version", 1, 1)
	if err != nil {
		t.Fatal(err)
	}

	t.Log(ret)

	res, err := decodeToInterface(ret, &Version{})
	if err != nil {
		t.Fatal(err)
	}

	version := res.(*Version)
	t.Logf("Spec_name: %s\n", version.Spec_name)
	t.Logf("Impl_name: %s\n", version.Impl_name)
	t.Logf("Authoring_version: %d\n", version.Authoring_version)
	t.Logf("Spec_version: %d\n", version.Spec_version)
	t.Logf("Impl_version: %d\n", version.Impl_version)

	if !reflect.DeepEqual(version, expected) {
		t.Errorf("Fail: got %v expected %v\n", version, expected)
	}
}

const TESTS_FP string = "./test_wasm.wasm"
const TEST_WASM_URL string = "https://github.com/ChainSafe/gossamer-test-wasm/blob/09d34b04fff635e92eaecfb192d42aae4f58ba54/target/wasm32-unknown-unknown/release/test_wasm.wasm?raw=true"

// getTestBlob checks if the test wasm file exists and if not, it fetches it from github
func getTestBlob() (n int64, err error) {
	if Exists(TESTS_FP) {
		return 0, nil
	}

	out, err := os.Create(TESTS_FP)
	if err != nil {
		return 0, err
	}
	defer out.Close()

	resp, err := http.Get(TEST_WASM_URL)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	n, err = io.Copy(out, resp.Body)
	return n, err
}

func newTestRuntime() (*Runtime, error) {
	_, err := getTestBlob()
	if err != nil {
		return nil, err
	}

	t := &trie.Trie{}
	fp, err := filepath.Abs(TESTS_FP)
	if err != nil {
		return nil, err
	}
	r, err := NewRuntime(fp, t)
	if err != nil {
		return nil, err
	}

	return r, nil
}

// tests that the function ext_get_storage_into can retrieve a value from the trie
// and store it in the wasm memory
func TestExt_get_storage_into(t *testing.T) {
	runtime, err := newTestRuntime()
	if err != nil {
		t.Fatal(err)
	}

	mem := runtime.vm.Memory.Data()

	// store kv pair in trie
	key := []byte(":noot")
	value := []byte{1, 3, 3, 7}
	err = runtime.trie.Put(key, value)
	if err != nil {
		t.Fatal(err)
	}

	// copy key to position `keyData` in memory
	keyData := 170
	// return value will be saved at position `valueData`
	valueData := 200
	// `valueOffset` is the position in the value following which its bytes should be stored
	valueOffset := 0
	copy(mem[keyData:keyData+len(key)], key)

	testFunc, ok := runtime.vm.Exports["test_ext_get_storage_into"]
	if !ok {
		t.Fatal("could not find exported function")
	}

	ret, err := testFunc(keyData, len(key), valueData, len(value), valueOffset)
	if err != nil {
		t.Fatal(err)
	} else if ret.ToI32() != int32(len(value)) {
		t.Error("return value does not match length of value in trie")
	} else if !bytes.Equal(mem[valueData:valueData+len(value)], value[valueOffset:]) {
		t.Error("did not store correct value in memory")
	}

	key = []byte("doesntexist")
	copy(mem[keyData:keyData+len(key)], key)
	expected := 1<<32 - 1
	ret, err = testFunc(keyData, len(key), valueData, len(value), valueOffset)
	if err != nil {
		t.Fatal(err)
	} else if ret.ToI32() != int32(expected) {
		t.Errorf("return value should be 2^32 - 1 since value doesn't exist, got %d", ret.ToI32())
	}
}

// tests that ext_set_storage can storage a value in the trie
func TestExt_set_storage(t *testing.T) {
	runtime, err := newTestRuntime()
	if err != nil {
		t.Fatal(err)
	}

	mem := runtime.vm.Memory.Data()

	// key,value we wish to store in the trie
	key := []byte(":noot")
	value := []byte{1, 3, 3, 7}

	// copy key and value into wasm memory
	keyData := 170
	valueData := 200
	copy(mem[keyData:keyData+len(key)], key)
	copy(mem[valueData:valueData+len(value)], value)

	testFunc, ok := runtime.vm.Exports["test_ext_set_storage"]
	if !ok {
		t.Fatal("could not find exported function")
	}

	_, err = testFunc(keyData, len(key), valueData, len(value))
	if err != nil {
		t.Fatal(err)
	}

	// make sure we can get the value from the trie
	trieValue, err := runtime.trie.Get(key)
	if err != nil {
		t.Fatal(err)
	} else if !bytes.Equal(value, trieValue) {
		t.Error("did not store correct value in storage trie")
	}

	t.Log(trieValue)
}

// tests that we can retrieve the trie root hash and store it in wasm memory
func TestExt_storage_root(t *testing.T) {
	runtime, err := newTestRuntime()
	if err != nil {
		t.Fatal(err)
	}

	mem := runtime.vm.Memory.Data()
	// save result at `resultPtr` in memory
	resultPtr := 170
	hash, err := runtime.trie.Hash()
	if err != nil {
		t.Fatal(err)
	}

	testFunc, ok := runtime.vm.Exports["test_ext_storage_root"]
	if !ok {
		t.Fatal("could not find exported function")
	}

	_, err = testFunc(resultPtr)
	if err != nil {
		t.Fatal(err)
	} else if !bytes.Equal(mem[resultPtr:resultPtr+32], hash[:]) {
		t.Error("did not save trie hash to memory")
	}
}

// test that ext_get_allocated_storage can get a value from the trie and store it
// in wasm memory
func TestExt_get_allocated_storage(t *testing.T) {
	runtime, err := newTestRuntime()
	if err != nil {
		t.Fatal(err)
	}

	mem := runtime.vm.Memory.Data()
	// put kv pair in trie
	key := []byte(":noot")
	value := []byte{1, 3, 3, 7}
	err = runtime.trie.Put(key, value)
	if err != nil {
		t.Fatal(err)
	}

	// copy key to `keyData` in memory
	keyData := 170
	copy(mem[keyData:keyData+len(key)], key)
	// memory location where length of return value is stored
	var writtenOut int32 = 169

	testFunc, ok := runtime.vm.Exports["test_ext_get_allocated_storage"]
	if !ok {
		t.Fatal("could not find exported function")
	}

	ret, err := testFunc(keyData, len(key), writtenOut)
	if err != nil {
		t.Fatal(err)
	}

	// returns memory location where value is stored
	retInt := uint32(ret.ToI32())
	loc := uint32(mem[writtenOut])
	length := binary.LittleEndian.Uint32(mem[loc : loc+4])
	if length != uint32(len(value)) {
		t.Error("did not save correct value length to memory")
	} else if !bytes.Equal(mem[retInt:retInt+length], value) {
		t.Error("did not save value to memory")
	}

	key = []byte("doesntexist")
	copy(mem[keyData:keyData+len(key)], key)
	ret, err = testFunc(keyData, len(key), writtenOut)
	if err != nil {
		t.Fatal(err)
	} else if ret.ToI32() != int32(0) {
		t.Errorf("return value should be 0 since value doesn't exist, got %d", ret.ToI32())
	}
}

// test that ext_clear_storage can delete a value from the trie
func TestExt_clear_storage(t *testing.T) {
	runtime, err := newTestRuntime()
	if err != nil {
		t.Fatal(err)
	}

	mem := runtime.vm.Memory.Data()
	// save kv pair in trie
	key := []byte(":noot")
	value := []byte{1, 3, 3, 7}
	err = runtime.trie.Put(key, value)
	if err != nil {
		t.Fatal(err)
	}

	// copy key to wasm memory
	keyData := 170
	copy(mem[keyData:keyData+len(key)], key)

	testFunc, ok := runtime.vm.Exports["test_ext_clear_storage"]
	if !ok {
		t.Fatal("could not find exported function")
	}

	_, err = testFunc(keyData, len(key))
	if err != nil {
		t.Fatal(err)
	}

	// make sure value is deleted
	ret, err := runtime.trie.Get(key)
	if err != nil {
		t.Fatal(err)
	} else if ret != nil {
		t.Error("did not delete key from storage trie")
	}
}

// test that ext_clear_prefix can delete all trie values with a certain prefix
func TestExt_clear_prefix(t *testing.T) {
	runtime, err := newTestRuntime()
	if err != nil {
		t.Fatal(err)
	}

	mem := runtime.vm.Memory.Data()

	// store some values in the trie
	tests := []struct {
		key   []byte
		value []byte
	}{
		{key: []byte{0x01, 0x35}, value: []byte("pen")},
		{key: []byte{0x01, 0x35, 0x79}, value: []byte("penguin")},
		{key: []byte{0xf2}, value: []byte("feather")},
		{key: []byte{0x09, 0xd3}, value: []byte("noot")},
	}

	for _, test := range tests {
		e := runtime.trie.Put(test.key, test.value)
		if e != nil {
			t.Fatal(e)
		}
	}

	// we are going to delete prefix 0x0135
	expected := []struct {
		key   []byte
		value []byte
	}{
		{key: []byte{0xf2}, value: []byte("feather")},
		{key: []byte{0x09, 0xd3}, value: []byte("noot")},
	}

	expectedTrie := &trie.Trie{}

	for _, test := range expected {
		e := expectedTrie.Put(test.key, test.value)
		if e != nil {
			t.Fatal(e)
		}
	}

	// copy prefix we want to delete to wasm memory
	prefix := []byte{0x01, 0x35}
	prefixData := 170
	copy(mem[prefixData:prefixData+len(prefix)], prefix)

	testFunc, ok := runtime.vm.Exports["test_ext_clear_prefix"]
	if !ok {
		t.Fatal("could not find exported function")
	}

	_, err = testFunc(prefixData, len(prefix))
	if err != nil {
		t.Fatal(err)
	}

	// make sure entries with that prefix were deleted
	runtimeTrieHash, err := runtime.trie.Hash()
	if err != nil {
		t.Fatal(err)
	}
	expectedHash, err := expectedTrie.Hash()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(runtimeTrieHash[:], expectedHash[:]) {
		t.Error("did not get expected trie")
	}
}

// test that ext_blake2_256 performs a blake2b hash of the data
func TestExt_blake2_256(t *testing.T) {
	runtime, err := newTestRuntime()
	if err != nil {
		t.Fatal(err)
	}

	mem := runtime.vm.Memory.Data()
	// save data in memory
	data := []byte("helloworld")
	pos := 170
	out := 180
	copy(mem[pos:pos+len(data)], data)

	testFunc, ok := runtime.vm.Exports["test_ext_blake2_256"]
	if !ok {
		t.Fatal("could not find exported function")
	}

	_, err = testFunc(pos, len(data), out)
	if err != nil {
		t.Fatal(err)
	}

	// make sure hashes match
	hash, err := common.Blake2bHash(data)
	if err != nil {
		t.Fatal(err)
	} else if !bytes.Equal(hash[:], mem[out:out+32]) {
		t.Error("hash saved in memory does not equal calculated hash")
	}
}

// test that ext_ed25519_verify verifies a valid signature
func TestExt_ed25519_verify(t *testing.T) {
	runtime, err := newTestRuntime()
	if err != nil {
		t.Fatal(err)
	}

	mem := runtime.vm.Memory.Data()

	// copy message into memory
	msg := []byte("helloworld")
	msgData := 170
	copy(mem[msgData:msgData+len(msg)], msg)

	// create key
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	// copy public key into memory
	pubkeyData := 180
	copy(mem[pubkeyData:pubkeyData+len(pub)], pub)

	// sign message, copy signature into memory
	sig := ed25519.Sign(priv, msg)
	sigData := 222
	copy(mem[sigData:sigData+len(sig)], sig)

	testFunc, ok := runtime.vm.Exports["test_ext_ed25519_verify"]
	if !ok {
		t.Fatal("could not find exported function")
	}

	verified, err := testFunc(msgData, len(msg), sigData, pubkeyData)
	if err != nil {
		t.Fatal(err)
	} else if verified.ToI32() != 0 {
		t.Error("did not verify ed25519 signature")
	}

	// verification should fail on wrong signature
	sigData = 1
	verified, err = testFunc(msgData, len(msg), sigData, pubkeyData)
	if err != nil {
		t.Fatal(err)
	} else if verified.ToI32() != 1 {
		t.Error("verified incorrect ed25519 signature")
	}
}

// test that ext_blake2_256_enumerated_trie_root places values in an array into a trie
// with the key being the index of the value and returns the hash
func TestExt_blake2_256_enumerated_trie_root(t *testing.T) {
	runtime, err := newTestRuntime()
	if err != nil {
		t.Fatal(err)
	}

	mem := runtime.vm.Memory.Data()

	// construct expected trie
	tests := []struct {
		key   []byte
		value []byte
	}{
		{key: []byte{0}, value: []byte("pen")},
		{key: []byte{1}, value: []byte("penguin")},
		{key: []byte{2}, value: []byte("feather")},
		{key: []byte{3}, value: []byte("noot")},
	}

	expectedTrie := &trie.Trie{}
	valuesArray := []byte{}
	lensArray := []byte{}

	for _, test := range tests {
		e := expectedTrie.Put(test.key, test.value)
		if e != nil {
			t.Fatal(e)
		}

		// construct array of values
		valuesArray = append(valuesArray, test.value...)
		lensVal := make([]byte, 4)
		binary.LittleEndian.PutUint32(lensVal, uint32(len(test.value)))
		// construct array of lengths of the values, where each length is int32
		lensArray = append(lensArray, lensVal...)
	}

	// save value array into memory at `valuesData`
	valuesData := 1
	// save lengths array into memory at `lensData`
	lensData := valuesData + len(valuesArray)
	// save length of lengths array in memory at `lensLen`
	lensLen := len(tests)
	// return value will be saved at `result` in memory
	result := lensLen + 1
	copy(mem[valuesData:valuesData+len(valuesArray)], valuesArray)
	copy(mem[lensData:lensData+len(lensArray)], lensArray)

	testFunc, ok := runtime.vm.Exports["test_ext_blake2_256_enumerated_trie_root"]
	if !ok {
		t.Fatal("could not find exported function")
	}

	_, err = testFunc(valuesData, lensData, lensLen, result)
	if err != nil {
		t.Fatal(err)
	}

	expectedHash, err := expectedTrie.Hash()
	if err != nil {
		t.Fatal(err)
	}

	// confirm that returned hash matches expected hash
	if !bytes.Equal(mem[result:result+32], expectedHash[:]) {
		t.Error("did not get expected trie")
	}
}

// test that ext_twox_128 performs a xxHash64 twice on give byte array of the data
func TestExt_twox_128(t *testing.T) {
	runtime, err := newTestRuntime()
	if err != nil {
		t.Fatal(err)
	}

	mem := runtime.vm.Memory.Data()
	// save data in memory
	// test for empty []byte
	data := []byte(nil)
	pos := 170
	out := pos + len(data)
	copy(mem[pos:pos+len(data)], data)

	// call wasm function
	testFunc, ok := runtime.vm.Exports["test_ext_twox_128"]
	if !ok {
		t.Fatal("could not find exported function")
	}

	_, err = testFunc(pos, len(data), out)
	if err != nil {
		t.Fatal(err)
	}

	//check result against expected value
	t.Logf("Ext_twox_128 data: %s, result: %s", data, hex.EncodeToString(mem[out:out+16]))
	if "99e9d85137db46ef4bbea33613baafd5" != hex.EncodeToString(mem[out:out+16]) {
		t.Error("hash saved in memory does not equal calculated hash")
	}

	// test for data value "Hello world!"
	data = []byte("Hello world!")
	out = pos + len(data)
	copy(mem[pos:pos+len(data)], data)

	// call wasm function
	testFunc, ok = runtime.vm.Exports["test_ext_twox_128"]
	if !ok {
		t.Fatal("could not find exported function")
	}

	_, err = testFunc(pos, len(data), out)
	if err != nil {
		t.Fatal(err)
	}

	//check result against expected value
	t.Logf("Ext_twox_128 data: %s, result: %s", data, hex.EncodeToString(mem[out:out+16]))
	if "b27dfd7f223f177f2a13647b533599af" != hex.EncodeToString(mem[out:out+16]) {
		t.Error("hash saved in memory does not equal calculated hash")
	}
}

// test ext_malloc returns expected pointer value of 8
func TestExt_malloc(t *testing.T) {
	// given
	runtime, err := newTestRuntime()
	if err != nil {
		t.Fatal(err)
	}

	testFunc, ok := runtime.vm.Exports["test_ext_malloc"]
	if !ok {
		t.Fatal("could not find exported function")
	}
	// when
	res, err := testFunc(1)
	if err != nil {
		t.Fatal(err)
	}
	t.Log("[TestExt_malloc]", "pointer", res)
	if res.ToI64() != 8 {
		t.Errorf("malloc did not return expected pointer value, expected 8, got %v", res)
	}
}

// test ext_free, confirm ext_free frees memory without error
func TestExt_free(t *testing.T) {
	// given
	runtime, err := newTestRuntime()
	if err != nil {
		t.Fatal(err)
	}

	initFunc, ok := runtime.vm.Exports["test_ext_malloc"]
	if !ok {
		t.Fatal("could not find exported function")
	}

	ptr, err := initFunc(1)
	if err != nil {
		t.Fatal(err)
	}
	if ptr.ToI64() != 8 {
		t.Errorf("malloc did not return expected pointer value, expected 8, got %v", ptr)
	}

	// when
	testFunc, ok := runtime.vm.Exports["test_ext_free"]
	if !ok {
		t.Fatal("could not find exported function")
	}
	_, err = testFunc(ptr)

	// then
	if err != nil {
		t.Fatal(err)
	}
}
