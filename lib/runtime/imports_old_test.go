package runtime

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"math/big"
	"sort"
	"testing"

	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/ChainSafe/gossamer/lib/crypto"
	"github.com/ChainSafe/gossamer/lib/crypto/ed25519"
	"github.com/ChainSafe/gossamer/lib/crypto/sr25519"
	"github.com/ChainSafe/gossamer/lib/scale"
	"github.com/ChainSafe/gossamer/lib/trie"
	"github.com/stretchr/testify/require"
)

// tests that the function ext_get_storage_into can retrieve a value from the trie
// and store it in the wasm memory
func TestExt_get_storage_into(t *testing.T) {
	runtime := NewTestRuntime(t, TEST_RUNTIME)

	mem := runtime.vm.Memory.Data()

	// store kv pair in trie
	key := []byte(":noot")
	value := []byte{1, 3, 3, 7}
	err := runtime.ctx.storage.Set(key, value)
	require.Nil(t, err)

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
	require.Nil(t, err)
	if ret.ToI32() != int32(len(value)) {
		t.Error("return value does not match length of value in trie")
	} else if !bytes.Equal(mem[valueData:valueData+len(value)], value[valueOffset:]) {
		t.Error("did not store correct value in memory")
	}

	key = []byte("doesntexist")
	copy(mem[keyData:keyData+len(key)], key)
	expected := 1<<32 - 1
	ret, err = testFunc(keyData, len(key), valueData, len(value), valueOffset)
	require.Nil(t, err)

	if ret.ToI32() != int32(expected) {
		t.Errorf("return value should be 2^32 - 1 since value doesn't exist, got %d", ret.ToI32())
	}
}

// tests that ext_set_storage can storage a value in the trie
func TestExt_set_storage(t *testing.T) {
	runtime := NewTestRuntime(t, TEST_RUNTIME)

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

	_, err := testFunc(keyData, len(key), valueData, len(value))
	require.Nil(t, err)

	// make sure we can get the value from the trie
	trieValue, err := runtime.ctx.storage.Get(key)
	require.Nil(t, err)

	if !bytes.Equal(value, trieValue) {
		t.Error("did not store correct value in storage trie")
	}

	t.Log(trieValue)
}

// tests that we can retrieve the trie root hash and store it in wasm memory
func TestExt_storage_root(t *testing.T) {
	runtime := NewTestRuntime(t, TEST_RUNTIME)

	mem := runtime.vm.Memory.Data()
	// save result at `resultPtr` in memory
	resultPtr := 170
	hash, err := runtime.ctx.storage.Root()
	require.Nil(t, err)

	testFunc, ok := runtime.vm.Exports["test_ext_storage_root"]
	if !ok {
		t.Fatal("could not find exported function")
	}

	_, err = testFunc(resultPtr)
	require.Nil(t, err)

	if !bytes.Equal(mem[resultPtr:resultPtr+32], hash[:]) {
		t.Error("did not save trie hash to memory")
	}
}

// test that ext_get_allocated_storage can get a value from the trie and store it in memory
func TestSetAndGetAllocatedStorage(t *testing.T) {
	runtime := NewTestRuntime(t, TEST_RUNTIME)

	mem := runtime.vm.Memory.Data()

	// key,value we wish to store in the trie
	key := []byte(":noot")
	value := []byte{0, 0, 0, 0, 0, 0, 0, 0}

	// copy key and value into wasm memory
	keyData := 170
	valueData := 200
	copy(mem[keyData:keyData+len(key)], key)
	copy(mem[valueData:valueData+len(value)], value)

	testFunc, ok := runtime.vm.Exports["test_ext_set_storage"]
	if !ok {
		t.Fatal("could not find exported function")
	}

	// call ext_set_storage to set trie key-value
	_, err := testFunc(keyData, len(key), valueData, len(value))
	require.Nil(t, err)

	// key,value we wish to store in the trie
	key = []byte(":extrinsic_index")
	value = []byte{0, 0, 0, 0, 0, 0, 0, 0}

	// copy key and value into wasm memory
	copy(mem[keyData:keyData+len(key)], key)
	copy(mem[valueData:valueData+len(value)], value)

	// call ext_set_storage to set trie key-value again
	_, err = testFunc(keyData, len(key), valueData, len(value))
	require.Nil(t, err)

	// copy key to `keyData` in memory
	copy(mem[keyData:keyData+len(key)], key)
	// memory location where length of return value is stored
	var writtenOut int32 = 166

	testFunc, ok = runtime.vm.Exports["test_ext_get_allocated_storage"]
	if !ok {
		t.Fatal("could not find exported function")
	}

	ret, err := testFunc(keyData, len(key), writtenOut)
	require.Nil(t, err)

	// returns memory location where value is stored
	retInt := uint32(ret.ToI32())
	length := binary.LittleEndian.Uint32(mem[writtenOut : writtenOut+4])
	if length != uint32(len(value)) {
		t.Error("did not save correct value length to memory")
	} else if !bytes.Equal(mem[retInt:retInt+length], value) {
		t.Error("did not save value to memory")
	}
}

// test that ext_get_allocated_storage can get a value from the trie and store it in memory
func Test_ext_get_allocated_storage(t *testing.T) {
	runtime := NewTestRuntime(t, TEST_RUNTIME)

	mem := runtime.vm.Memory.Data()
	// put kv pair in trie
	key := []byte(":noot")
	value := []byte{1, 3, 3, 7}
	err := runtime.ctx.storage.Set(key, value)
	require.Nil(t, err)

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
	require.Nil(t, err)

	// returns memory location where value is stored
	retInt := uint32(ret.ToI32())
	if retInt == 0 {
		t.Fatalf("call failed")
	}

	length := binary.LittleEndian.Uint32(mem[writtenOut : writtenOut+4])
	if length != uint32(len(value)) {
		t.Error("did not save correct value length to memory")
	} else if !bytes.Equal(mem[retInt:retInt+length], value) {
		t.Error("did not save value to memory")
	}

	key = []byte("doesntexist")
	copy(mem[keyData:keyData+len(key)], key)
	ret, err = testFunc(keyData, len(key), writtenOut)
	require.Nil(t, err)

	if ret.ToI32() != int32(0) {
		t.Errorf("return value should be 0 since value doesn't exist, got %d", ret.ToI32())
	}
}

// test that ext_clear_storage can delete a value from the trie
func TestExt_clear_storage(t *testing.T) {
	runtime := NewTestRuntime(t, TEST_RUNTIME)

	mem := runtime.vm.Memory.Data()
	// save kv pair in trie
	key := []byte(":noot")
	value := []byte{1, 3, 3, 7}
	err := runtime.ctx.storage.Set(key, value)
	require.Nil(t, err)

	// copy key to wasm memory
	keyData := 170
	copy(mem[keyData:keyData+len(key)], key)

	testFunc, ok := runtime.vm.Exports["test_ext_clear_storage"]
	if !ok {
		t.Fatal("could not find exported function")
	}

	_, err = testFunc(keyData, len(key))
	require.Nil(t, err)

	// make sure value is deleted
	ret, err := runtime.ctx.storage.Get(key)
	require.Nil(t, err)

	if ret != nil {
		t.Error("did not delete key from storage trie")
	}
}

// test that ext_clear_prefix can delete all trie values with a certain prefix
func TestExt_clear_prefix(t *testing.T) {
	runtime := NewTestRuntime(t, TEST_RUNTIME)

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
		e := runtime.ctx.storage.Set(test.key, test.value)
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

	_, err := testFunc(prefixData, len(prefix))
	require.Nil(t, err)

	// make sure entries with that prefix were deleted
	runtimeTrieHash, err := runtime.ctx.storage.Root()
	require.Nil(t, err)

	expectedHash, err := expectedTrie.Hash()
	require.Nil(t, err)

	if !bytes.Equal(runtimeTrieHash[:], expectedHash[:]) {
		t.Error("did not get expected trie")
	}
}

// test that ext_blake2_128 performs a blake2b hash of the data
func TestExt_blake2_128(t *testing.T) {
	runtime := NewTestRuntime(t, TEST_RUNTIME)

	mem := runtime.vm.Memory.Data()
	// save data in memory
	data := []byte("helloworld")
	pos := 170
	out := 180
	copy(mem[pos:pos+len(data)], data)

	testFunc, ok := runtime.vm.Exports["test_ext_blake2_128"]
	if !ok {
		t.Fatal("could not find exported function")
	}

	_, err := testFunc(pos, len(data), out)
	require.Nil(t, err)

	// make sure hashes match
	hash, err := common.Blake2b128(data)
	require.Nil(t, err)

	if !bytes.Equal(hash, mem[out:out+16]) {
		t.Errorf("hash saved in memory does not equal calculated hash, got %x expected %x", mem[out:out+16], hash)
	}
}

// test that ext_blake2_256 performs a blake2b hash of the data
func TestExt_blake2_256(t *testing.T) {
	runtime := NewTestRuntime(t, TEST_RUNTIME)

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

	_, err := testFunc(pos, len(data), out)
	require.Nil(t, err)

	// make sure hashes match
	hash, err := common.Blake2bHash(data)
	require.Nil(t, err)

	if !bytes.Equal(hash[:], mem[out:out+32]) {
		t.Error("hash saved in memory does not equal calculated hash")
	}
}

// test that ext_ed25519_verify verifies a valid signature
func TestExt_ed25519_verify(t *testing.T) {
	runtime := NewTestRuntime(t, TEST_RUNTIME)

	mem := runtime.vm.Memory.Data()

	// copy message into memory
	msg := []byte("helloworld")
	msgData := 170
	copy(mem[msgData:msgData+len(msg)], msg)

	// create key
	kp, err := ed25519.GenerateKeypair()
	require.Nil(t, err)

	priv := kp.Private()
	pub := kp.Public()

	// copy public key into memory
	pubkeyData := 180
	copy(mem[pubkeyData:pubkeyData+len(pub.Encode())], pub.Encode())

	// sign message, copy signature into memory
	sig, err := priv.Sign(msg)
	require.Nil(t, err)

	sigData := 222
	copy(mem[sigData:sigData+len(sig)], sig)

	testFunc, ok := runtime.vm.Exports["test_ext_ed25519_verify"]
	if !ok {
		t.Fatal("could not find exported function")
	}

	verified, err := testFunc(msgData, len(msg), sigData, pubkeyData)
	require.Nil(t, err)

	if verified.ToI32() != 0 {
		t.Error("did not verify ed25519 signature")
	}

	// verification should fail on wrong signature
	sigData = 1
	verified, err = testFunc(msgData, len(msg), sigData, pubkeyData)
	require.Nil(t, err)

	if verified.ToI32() != 1 {
		t.Error("verified incorrect ed25519 signature")
	}
}

// test that ext_sr25519_verify verifies a valid signature
func TestExt_sr25519_verify(t *testing.T) {
	runtime := NewTestRuntime(t, TEST_RUNTIME)

	mem := runtime.vm.Memory.Data()

	// copy message into memory
	msg := []byte("helloworld")
	msgData := 170
	copy(mem[msgData:msgData+len(msg)], msg)

	// create key
	kp, err := sr25519.GenerateKeypair()
	require.Nil(t, err)

	// copy public key into memory
	pubkeyData := 180
	pub := kp.Public().Encode()
	copy(mem[pubkeyData:pubkeyData+len(pub)], pub)

	// sign message, copy signature into memory
	sig, err := kp.Private().Sign(msg)
	require.Nil(t, err)

	sigData := pubkeyData + len(pub)
	copy(mem[sigData:sigData+len(sig)], sig)

	testFunc, ok := runtime.vm.Exports["test_ext_sr25519_verify"]
	if !ok {
		t.Fatal("could not find exported function")
	}

	verified, err := testFunc(msgData, len(msg), sigData, pubkeyData)
	require.Nil(t, err)

	if verified.ToI32() != 0 {
		t.Error("did not verify sr25519 signature")
	}

	// verification should fail on wrong signature
	sigData = 1
	verified, err = testFunc(msgData, len(msg), sigData, pubkeyData)
	require.Nil(t, err)

	if verified.ToI32() != 1 {
		t.Error("verified incorrect sr25519 signature")
	}
}

// test that ext_blake2_256_enumerated_trie_root places values in an array into a trie
// with the key being the index of the value and returns the hash
func TestExt_blake2_256_enumerated_trie_root(t *testing.T) {
	runtime := NewTestRuntime(t, TEST_RUNTIME)

	mem := runtime.vm.Memory.Data()

	// construct expected trie
	// test values used in paritytech substrate tests
	//  https://github.com/paritytech/substrate/blob/6e242a5a9fcc5d5ea34386864ec064a01677efff/client/executor/src/integration_tests/mod.rs#L419
	//  Expected value:  0x9243f4bb6fa633dce97247652479ed7e2e2995a5ea641fd9d1e1a046f7601da6
	tests := []struct {
		key   []byte
		value []byte
	}{
		{key: []byte{0}, value: []byte("zero")},
		{key: []byte{1}, value: []byte("one")},
		{key: []byte{2}, value: []byte("two")},
	}

	expectedTrie := &trie.Trie{}
	valuesArray := []byte{}
	lensArray := []byte{}

	for _, test := range tests {
		keyBigInt := new(big.Int).SetBytes(test.key)
		encodedKey, err2 := scale.Encode(keyBigInt)
		require.Nil(t, err2)

		e := expectedTrie.Put(encodedKey, test.value)
		require.Nil(t, e)

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

	_, err := testFunc(valuesData, lensData, lensLen, result)
	require.Nil(t, err)

	expectedHash, err := expectedTrie.Hash()
	require.Nil(t, err)

	// confirm that returned hash matches expected hash
	if !bytes.Equal(mem[result:result+32], expectedHash[:]) {
		t.Error("did not get expected trie")
	}
}

// test that ext_twox_64 performs a xxHash64
func TestExt_twox_64(t *testing.T) {
	// test cases from https://github.com/paritytech/substrate/blob/13fc71c681cc9a3cc911c32c7890b52885092969/core/executor/src/wasm_executor.rs#L1701
	runtime := NewTestRuntime(t, TEST_RUNTIME)

	mem := runtime.vm.Memory.Data()
	// save data in memory
	// test for empty []byte
	data := []byte(nil)
	pos := 170
	out := pos + len(data)
	copy(mem[pos:pos+len(data)], data)

	// call wasm function
	testFunc, ok := runtime.vm.Exports["test_ext_twox_64"]
	if !ok {
		t.Fatal("could not find exported function")
	}

	_, err := testFunc(pos, len(data), out)
	require.Nil(t, err)

	//check result against expected value
	if "99e9d85137db46ef" != hex.EncodeToString(mem[out:out+8]) {
		t.Error("hash saved in memory does not equal calculated hash")
	}

	// test for data value "Hello world!"
	data = []byte("Hello world!")
	out = pos + len(data)
	copy(mem[pos:pos+len(data)], data)

	// call wasm function
	testFunc, ok = runtime.vm.Exports["test_ext_twox_64"]
	if !ok {
		t.Fatal("could not find exported function")
	}

	_, err = testFunc(pos, len(data), out)
	require.Nil(t, err)

	//check result against expected value
	if "b27dfd7f223f177f" != hex.EncodeToString(mem[out:out+8]) {
		t.Error("hash saved in memory does not equal calculated hash")
	}
}

// test that ext_twox_128 performs a xxHash64 twice on give byte array of the data
func TestExt_twox_128(t *testing.T) {
	// test cases from https://github.com/paritytech/substrate/blob/13fc71c681cc9a3cc911c32c7890b52885092969/core/executor/src/wasm_executor.rs#L1701
	runtime := NewTestRuntime(t, TEST_RUNTIME)

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

	_, err := testFunc(pos, len(data), out)
	require.Nil(t, err)

	//check result against expected value
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
	require.Nil(t, err)

	//check result against expected value
	if "b27dfd7f223f177f2a13647b533599af" != hex.EncodeToString(mem[out:out+16]) {
		t.Error("hash saved in memory does not equal calculated hash")
	}
}

// test that ext_keccak_256 returns the correct hash
func TestExt_keccak_256(t *testing.T) {
	runtime := NewTestRuntime(t, TEST_RUNTIME)

	mem := runtime.vm.Memory.Data()

	data := []byte(nil)
	pos := 170
	out := pos + len(data)
	copy(mem[pos:pos+len(data)], data)

	// call wasm function
	testFunc, ok := runtime.vm.Exports["test_ext_keccak_256"]
	if !ok {
		t.Fatal("could not find exported function")
	}

	_, err := testFunc(pos, len(data), out)
	require.Nil(t, err)

	// test case from https://github.com/debris/tiny-keccak/blob/master/tests/keccak.rs#L4
	expected, err := common.HexToHash("0xc5d2460186f7233c927e7db2dcc703c0e500b653ca82273b7bfad8045d85a470")
	require.Nil(t, err)

	if !bytes.Equal(expected[:], mem[out:out+32]) {
		t.Fatalf("fail: got %x expected %x", mem[out:out+32], expected[:])
	}
}

// test ext_malloc returns expected pointer value of 8
func TestExt_malloc(t *testing.T) {
	// given
	runtime := NewTestRuntime(t, TEST_RUNTIME)

	testFunc, ok := runtime.vm.Exports["test_ext_malloc"]
	if !ok {
		t.Fatal("could not find exported function")
	}
	// when
	res, err := testFunc(1)
	require.Nil(t, err)

	t.Log("[TestExt_malloc]", "pointer", res)
	if res.ToI64() != 8 {
		t.Errorf("malloc did not return expected pointer value, expected 8, got %v", res)
	}
}

// test ext_free, confirm ext_free frees memory without error
func TestExt_free(t *testing.T) {
	// given
	runtime := NewTestRuntime(t, TEST_RUNTIME)

	initFunc, ok := runtime.vm.Exports["test_ext_malloc"]
	if !ok {
		t.Fatal("could not find exported function")
	}

	ptr, err := initFunc(1)
	require.Nil(t, err)

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
	require.Nil(t, err)
}

// test that ext_secp256k1_ecdsa_recover returns the correct public key
func TestExt_secp256k1_ecdsa_recover(t *testing.T) {
	runtime := NewTestRuntime(t, TEST_RUNTIME)

	mem := runtime.vm.Memory.Data()

	msgData, err := common.HexToBytes("0xce0677bb30baa8cf067c88db9811f4333d131bf8bcf12fe7065d211dce971008")
	require.Nil(t, err)

	sigData, err := common.HexToBytes("0x90f27b8b488db00b00606796d2987f6a5f59ae62ea05effe84fef5b8b0e549984a691139ad57a3f0b906637673aa2f63d1f55cb1a69199d4009eea23ceaddc9301")
	require.Nil(t, err)

	msgPos := 1000
	sigPos := msgPos + len(msgData)
	copy(mem[msgPos:msgPos+len(msgData)], msgData)
	copy(mem[sigPos:sigPos+len(sigData)], sigData)
	pubkeyData := sigPos + len(sigData)

	// call wasm function
	testFunc, ok := runtime.vm.Exports["test_ext_secp256k1_ecdsa_recover"]
	if !ok {
		t.Fatal("could not find exported function")
	}

	_, err = testFunc(msgPos, sigPos, pubkeyData)
	require.Nil(t, err)

	// test case from https://github.com/ethereum/go-ethereum/blob/master/crypto/signature_test.go
	expected, err := common.HexToBytes("0x04e32df42865e97135acfb65f3bae71bdc86f4d49150ad6a440b6f15878109880a0a2b2667f7e725ceea70c673093bf67663e0312623c8e091b13cf2c0f11ef652")
	require.Nil(t, err)

	if !bytes.Equal(expected[:], mem[pubkeyData:pubkeyData+65]) {
		t.Fatalf("fail: got %x expected %x", mem[pubkeyData:pubkeyData+65], expected)
	}
}

// test that TestExt_sr25519_generate generates and saves a keypair in the keystore
func TestExt_sr25519_generate(t *testing.T) {
	runtime := NewTestRuntime(t, TEST_RUNTIME)

	mem := runtime.vm.Memory.Data()

	idData := []byte{1, 0, 0, 0}
	seedLen := 32

	seedData := make([]byte, seedLen)
	_, err := rand.Read(seedData)
	require.Nil(t, err)

	idLoc := 1000
	seedLoc := idLoc + len(idData)
	out := seedLoc + seedLen
	copy(mem[seedLoc:seedLoc+seedLen], seedData)
	copy(mem[idLoc:idLoc+len(idData)], idData)

	// call wasm function
	testFunc, ok := runtime.vm.Exports["test_ext_sr25519_generate"]
	if !ok {
		t.Fatal("could not find exported function")
	}

	_, err = testFunc(idLoc, seedLoc, seedLen, out)
	require.Nil(t, err)

	pubkeyData := mem[out : out+32]
	pubkey, err := sr25519.NewPublicKey(pubkeyData)
	require.Nil(t, err)

	kp := runtime.ctx.keystore.GetKeypair(pubkey)
	if kp == nil {
		t.Fatal("Fail: keypair was not saved in keystore")
	}
}

// test that TestExt_ed25519_generate generates and saves a keypair in the keystore
func TestExt_ed25519_generate(t *testing.T) {
	runtime := NewTestRuntime(t, TEST_RUNTIME)

	mem := runtime.vm.Memory.Data()

	idData := []byte{1, 0, 0, 0}
	seedLen := 32

	seedData := make([]byte, seedLen)
	_, err := rand.Read(seedData)
	require.Nil(t, err)

	idLoc := 1000
	seedLoc := idLoc + len(idData)
	out := seedLoc + seedLen
	copy(mem[seedLoc:seedLoc+seedLen], seedData)
	copy(mem[idLoc:idLoc+len(idData)], idData)

	// call wasm function
	testFunc, ok := runtime.vm.Exports["test_ext_ed25519_generate"]
	if !ok {
		t.Fatal("could not find exported function")
	}

	_, err = testFunc(idLoc, seedLoc, seedLen, out)
	require.Nil(t, err)

	pubkeyData := mem[out : out+32]
	pubkey, err := ed25519.NewPublicKey(pubkeyData)
	require.Nil(t, err)

	kp := runtime.ctx.keystore.GetKeypair(pubkey)
	if kp == nil {
		t.Fatal("Fail: keypair was not saved in keystore")
	}
}

// test that ext_ed25519_public_keys confirms that we can retrieve our public keys from the keystore
func TestExt_ed25519_public_keys(t *testing.T) {
	runtime := NewTestRuntime(t, TEST_RUNTIME)

	testKps := []crypto.Keypair{}
	expectedPubkeys := [][]byte{}
	numKps := 12

	for i := 0; i < numKps; i++ {
		kp, err := ed25519.GenerateKeypair()
		if err != nil {
			t.Fatal(err)
		}
		runtime.ctx.keystore.Insert(kp)
		testKps = append(testKps, kp)
		expected := testKps[i].Public().Encode()
		expectedPubkeys = append(expectedPubkeys, expected)
	}

	// put some sr25519 keypairs in the keystore to make sure they don't get returned
	for i := 0; i < numKps; i++ {
		kp, err := sr25519.GenerateKeypair()
		if err != nil {
			t.Fatal(err)
		}
		runtime.ctx.keystore.Insert(kp)
	}

	mem := runtime.vm.Memory.Data()

	idLoc := 0
	resultLoc := 1 << 9

	// call wasm function
	testFunc, ok := runtime.vm.Exports["test_ext_ed25519_public_keys"]
	if !ok {
		t.Fatal("could not find exported function")
	}

	out, err := testFunc(idLoc, resultLoc)
	require.Nil(t, err)

	if out.ToI32() == -1 {
		t.Fatal("call to test_ext_ed25519_public_keys failed")
	}

	resultLenBytes := mem[resultLoc : resultLoc+4]
	resultLen := binary.LittleEndian.Uint32(resultLenBytes)
	pubkeyData := mem[out.ToI32() : out.ToI32()+int32(resultLen*32)]

	pubkeys := [][]byte{}
	for i := 0; i < numKps; i++ {
		kpData := pubkeyData[i*32 : i*32+32]
		pubkeys = append(pubkeys, kpData)
	}

	sort.Slice(expectedPubkeys, func(i, j int) bool { return bytes.Compare(expectedPubkeys[i], expectedPubkeys[j]) < 0 })
	sort.Slice(pubkeys, func(i, j int) bool { return bytes.Compare(pubkeys[i], pubkeys[j]) < 0 })

	require.Equal(t, expectedPubkeys, pubkeys)
}

// test that ext_sr25519_public_keys confirms that we can retrieve our public keys from the keystore
func TestExt_sr25519_public_keys(t *testing.T) {
	runtime := NewTestRuntime(t, TEST_RUNTIME)

	testKps := []crypto.Keypair{}
	expectedPubkeys := [][]byte{}
	numKps := 12

	for i := 0; i < numKps; i++ {
		kp, err := sr25519.GenerateKeypair()
		require.Nil(t, err)

		runtime.ctx.keystore.Insert(kp)
		testKps = append(testKps, kp)
		expected := testKps[i].Public().Encode()
		expectedPubkeys = append(expectedPubkeys, expected)
	}

	// put some ed25519 keypairs in the keystore to make sure they don't get returned
	for i := 0; i < numKps; i++ {
		kp, err := ed25519.GenerateKeypair()
		require.Nil(t, err)

		runtime.ctx.keystore.Insert(kp)
	}

	mem := runtime.vm.Memory.Data()

	idLoc := 0
	resultLoc := 1 << 9

	// call wasm function
	testFunc, ok := runtime.vm.Exports["test_ext_sr25519_public_keys"]
	if !ok {
		t.Fatal("could not find exported function")
	}

	out, err := testFunc(idLoc, resultLoc)
	require.Nil(t, err)

	if out.ToI32() == -1 {
		t.Fatal("call to test_ext_sr25519_public_keys failed")
	}

	resultLenBytes := mem[resultLoc : resultLoc+4]
	resultLen := binary.LittleEndian.Uint32(resultLenBytes)
	pubkeyData := mem[out.ToI32() : out.ToI32()+int32(resultLen*32)]

	t.Log(resultLen)

	pubkeys := [][]byte{}
	for i := 0; i < numKps; i++ {
		kpData := pubkeyData[i*32 : i*32+32]
		pubkeys = append(pubkeys, kpData)
	}

	sort.Slice(expectedPubkeys, func(i, j int) bool { return bytes.Compare(expectedPubkeys[i], expectedPubkeys[j]) < 0 })
	sort.Slice(pubkeys, func(i, j int) bool { return bytes.Compare(pubkeys[i], pubkeys[j]) < 0 })

	require.Equal(t, expectedPubkeys, pubkeys)
}

// test that ext_ed25519_sign generates and saves a keypair in the keystore
func TestExt_ed25519_sign(t *testing.T) {
	runtime := NewTestRuntime(t, TEST_RUNTIME)

	mem := runtime.vm.Memory.Data()

	kp, err := ed25519.GenerateKeypair()
	require.Nil(t, err)

	runtime.ctx.keystore.Insert(kp)

	idLoc := 0
	pubkeyLoc := 0
	pubkeyData := kp.Public().Encode()
	msgLoc := pubkeyLoc + len(pubkeyData)
	msgData := []byte("helloworld")
	msgLen := msgLoc + len(msgData)
	out := msgLen + 4

	msgLenBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(msgLenBytes, uint32(len(msgData)))

	copy(mem[pubkeyLoc:pubkeyLoc+len(pubkeyData)], pubkeyData)
	copy(mem[msgLoc:msgLoc+len(msgData)], msgData)
	copy(mem[msgLen:msgLen+4], msgLenBytes)

	// call wasm function
	testFunc, ok := runtime.vm.Exports["test_ext_ed25519_sign"]
	if !ok {
		t.Fatal("could not find exported function")
	}

	_, err = testFunc(idLoc, pubkeyLoc, msgLoc, msgLen, out)
	require.Nil(t, err)

	sig := mem[out : out+ed25519.SignatureLength]

	ok, err = kp.Public().Verify(msgData, sig)
	require.Nil(t, err)

	if !ok {
		t.Fatalf("Fail: did not verify signature")
	}
}

// test that ext_sr25519_sign generates and saves a keypair in the keystore
func TestExt_sr25519_sign(t *testing.T) {
	runtime := NewTestRuntime(t, TEST_RUNTIME)

	mem := runtime.vm.Memory.Data()

	kp, err := sr25519.GenerateKeypair()
	require.Nil(t, err)

	runtime.ctx.keystore.Insert(kp)

	idLoc := 0
	pubkeyLoc := 0
	pubkeyData := kp.Public().Encode()
	msgLoc := pubkeyLoc + len(pubkeyData)
	msgData := []byte("helloworld")
	msgLen := msgLoc + len(msgData)
	out := msgLen + 4

	msgLenBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(msgLenBytes, uint32(len(msgData)))

	copy(mem[pubkeyLoc:pubkeyLoc+len(pubkeyData)], pubkeyData)
	copy(mem[msgLoc:msgLoc+len(msgData)], msgData)
	copy(mem[msgLen:msgLen+4], msgLenBytes)

	// call wasm function
	testFunc, ok := runtime.vm.Exports["test_ext_sr25519_sign"]
	if !ok {
		t.Fatal("could not find exported function")
	}

	_, err = testFunc(idLoc, pubkeyLoc, msgLoc, msgLen, out)
	require.Nil(t, err)

	sig := mem[out : out+sr25519.SignatureLength]
	t.Log(sig)

	ok, err = kp.Public().Verify(msgData, sig)
	require.Nil(t, err)

	if !ok {
		t.Fatalf("Fail: did not verify signature")
	}
}

// test that ext_get_child_storage_into retrieves a value stored in a child trie
func TestExt_get_child_storage_into(t *testing.T) {
	runtime := NewTestRuntime(t, TEST_RUNTIME)

	mem := runtime.vm.Memory.Data()

	storageKey := []byte("default")
	key := []byte("mykey")
	value := []byte("myvalue")

	err := runtime.ctx.storage.SetChild(storageKey, trie.NewEmptyTrie())
	require.Nil(t, err)

	err = runtime.ctx.storage.SetChildStorage(storageKey, key, value)
	require.Nil(t, err)

	storageKeyData := 0
	storageKeyLen := len(storageKey)
	keyData := storageKeyData + storageKeyLen
	keyLen := len(key)
	valueData := keyData + keyLen
	valueLen := len(value)
	valueOffset := 0

	copy(mem[storageKeyData:storageKeyData+storageKeyLen], storageKey)
	copy(mem[keyData:keyData+keyLen], key)

	// call wasm function
	testFunc, ok := runtime.vm.Exports["test_ext_get_child_storage_into"]
	if !ok {
		t.Fatal("could not find exported function")
	}

	_, err = testFunc(storageKeyData, storageKeyLen, keyData, keyLen, valueData, valueLen, valueOffset)
	require.Nil(t, err)

	res := mem[valueData : valueData+valueLen]
	if !bytes.Equal(res, value[valueOffset:]) {
		t.Fatalf("Fail: got %x expected %x", res, value[valueOffset:])
	}
}

// test that ext_set_child_storage sets a value stored in a child trie
func TestExt_set_child_storage(t *testing.T) {
	runtime := NewTestRuntime(t, TEST_RUNTIME)

	mem := runtime.vm.Memory.Data()

	storageKey := []byte("default")
	key := []byte("mykey")
	value := []byte("myvalue")

	err := runtime.ctx.storage.SetChild(storageKey, trie.NewEmptyTrie())
	require.Nil(t, err)

	storageKeyData := 0
	storageKeyLen := len(storageKey)
	keyData := storageKeyData + storageKeyLen
	keyLen := len(key)
	valueData := keyData + keyLen
	valueLen := len(value)

	copy(mem[storageKeyData:storageKeyData+storageKeyLen], storageKey)
	copy(mem[keyData:keyData+keyLen], key)
	copy(mem[valueData:valueData+valueLen], value)

	// call wasm function
	testFunc, ok := runtime.vm.Exports["test_ext_set_child_storage"]
	if !ok {
		t.Fatal("could not find exported function")
	}

	_, err = testFunc(storageKeyData, storageKeyLen, keyData, keyLen, valueData, valueLen)
	require.Nil(t, err)

	res, err := runtime.ctx.storage.GetChildStorage(storageKey, key)
	require.Nil(t, err)

	if !bytes.Equal(res, value) {
		t.Fatalf("Fail: got %x expected %x", res, value)
	}
}

func TestExt_local_storage_set_local(t *testing.T) {
	runtime := NewTestRuntime(t, TEST_RUNTIME)

	mem := runtime.vm.Memory.Data()

	key := []byte("mykey")
	value := []byte("myvalue")

	keyPtr := 0
	keyLen := len(key)
	valuePtr := keyPtr + keyLen
	valueLen := len(value)

	copy(mem[keyPtr:keyPtr+keyLen], key)
	copy(mem[valuePtr:valuePtr+valueLen], value)

	// call wasm function
	testFunc, ok := runtime.vm.Exports["test_ext_local_storage_set"]
	if !ok {
		t.Fatal("could not find exported function")
	}

	_, err := testFunc(NodeStorageTypeLocal, keyPtr, keyLen, valuePtr, valueLen)
	require.NoError(t, err)

	resValue, err := runtime.ctx.nodeStorage.LocalStorage.Get(key)
	require.NoError(t, err)
	require.Equal(t, value, resValue)
}

func TestExt_local_storage_set_persistent(t *testing.T) {
	runtime := NewTestRuntime(t, TEST_RUNTIME)

	mem := runtime.vm.Memory.Data()

	key := []byte("mykey")
	value := []byte("myvalue")

	keyPtr := 0
	keyLen := len(key)
	valuePtr := keyPtr + keyLen
	valueLen := len(value)

	copy(mem[keyPtr:keyPtr+keyLen], key)
	copy(mem[valuePtr:valuePtr+valueLen], value)

	// call wasm function
	testFunc, ok := runtime.vm.Exports["test_ext_local_storage_set"]
	if !ok {
		t.Fatal("could not find exported function")
	}

	_, err := testFunc(NodeStorageTypePersistent, keyPtr, keyLen, valuePtr, valueLen)
	require.NoError(t, err)

	resValue, err := runtime.ctx.nodeStorage.PersistentStorage.Get(key)
	require.NoError(t, err)
	require.Equal(t, value, resValue)
}

func TestExt_local_storage_get_local(t *testing.T) {
	runtime := NewTestRuntime(t, TEST_RUNTIME)
	mem := runtime.vm.Memory.Data()

	key := []byte("mykey")
	value := []byte("myvalue")
	runtime.ctx.nodeStorage.LocalStorage.Put(key, value)

	keyPtr := 0
	keyLen := len(key)
	valueLen := len(value)

	copy(mem[keyPtr:keyPtr+keyLen], key)

	// call wasm function
	testFunc, ok := runtime.vm.Exports["test_ext_local_storage_get"]
	if !ok {
		t.Fatal("could not find exported function")
	}

	res, err := testFunc(NodeStorageTypeLocal, keyPtr, keyLen, valueLen)
	require.Nil(t, err)

	require.Equal(t, value, mem[res.ToI32():res.ToI32()+int32(valueLen)])
}

func TestExt_local_storage_get_persistent(t *testing.T) {
	runtime := NewTestRuntime(t, TEST_RUNTIME)
	mem := runtime.vm.Memory.Data()

	key := []byte("mykey")
	value := []byte("myvalue")
	runtime.ctx.nodeStorage.PersistentStorage.Put(key, value)

	keyPtr := 0
	keyLen := len(key)
	valueLen := len(value)

	copy(mem[keyPtr:keyPtr+keyLen], key)

	// call wasm function
	testFunc, ok := runtime.vm.Exports["test_ext_local_storage_get"]
	if !ok {
		t.Fatal("could not find exported function")
	}

	res, err := testFunc(NodeStorageTypePersistent, keyPtr, keyLen, valueLen)
	require.Nil(t, err)

	require.Equal(t, value, mem[res.ToI32():res.ToI32()+int32(valueLen)])
}

func TestExt_is_validator(t *testing.T) {
	// test with validator
	runtime := NewTestRuntimeWithRole(t, TEST_RUNTIME, byte(4))
	// call wasm function
	testFunc, ok := runtime.vm.Exports["test_ext_is_validator"]
	if !ok {
		t.Fatal("could not find exported function")
	}
	res, err := testFunc()
	require.NoError(t, err)
	require.Equal(t, int32(1), res.ToI32())

	// test with non-validator
	runtime = NewTestRuntimeWithRole(t, TEST_RUNTIME, byte(1))
	// call wasm function
	testFunc, ok = runtime.vm.Exports["test_ext_is_validator"]
	if !ok {
		t.Fatal("could not find exported function")
	}
	res, err = testFunc()
	require.NoError(t, err)
	require.Equal(t, int32(0), res.ToI32())
}
