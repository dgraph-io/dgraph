package runtime

import (
	"bytes"
	"testing"

	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/ChainSafe/gossamer/lib/trie"
	"github.com/stretchr/testify/require"
)

func Test_ext_twox_256(t *testing.T) {
	runtime := NewTestRuntime(t, TEST_RUNTIME)

	mem := runtime.vm.Memory.Data()

	data := []byte("hello")
	pos := 170
	out := pos + len(data)
	copy(mem[pos:pos+len(data)], data)

	// call wasm function
	testFunc, ok := runtime.vm.Exports["test_ext_twox_256"]
	if !ok {
		t.Fatal("could not find exported function")
	}

	_, err := testFunc(pos, len(data), out)
	require.Nil(t, err)

	// test case from https://github.com/w3f/polkadot-spec/tree/master/test
	expected, err := common.HexToHash("0xa36d9f887d82c726b2a1d004cb71dd231fe2fb3bf584fc533914a80e276583e0")
	require.Nil(t, err)

	if !bytes.Equal(expected[:], mem[out:out+32]) {
		t.Fatalf("fail: got %x expected %x", mem[out:out+32], expected[:])
	}
}

func Test_ext_kill_child_storage(t *testing.T) {
	runtime := NewTestRuntime(t, TEST_RUNTIME)
	mem := runtime.vm.Memory.Data()
	// set child storage
	storageKey := []byte("childstore1")
	childKey := []byte("key1")
	value := []byte("value")
	err := runtime.ctx.storage.SetChild(storageKey, trie.NewEmptyTrie())
	require.Nil(t, err)

	storageKeyLen := uint32(len(storageKey))
	storageKeyPtr, err := runtime.malloc(storageKeyLen)
	require.NoError(t, err)

	childKeyLen := uint32(len(childKey))
	childKeyPtr, err := runtime.malloc(childKeyLen)
	require.NoError(t, err)

	valueLen := uint32(len(value))
	valuePtr, err := runtime.malloc(valueLen)
	require.NoError(t, err)

	copy(mem[storageKeyPtr:storageKeyPtr+storageKeyLen], storageKey)
	copy(mem[childKeyPtr:childKeyPtr+childKeyLen], childKey)
	copy(mem[valuePtr:valuePtr+valueLen], value)

	// call wasm function to set child storage
	testFunc, ok := runtime.vm.Exports["test_ext_set_child_storage"]
	if !ok {
		t.Fatal("could not find exported function")
	}

	_, err = testFunc(int32(storageKeyPtr), int32(storageKeyLen), int32(childKeyPtr), int32(childKeyLen), int32(valuePtr), int32(valueLen))
	require.Nil(t, err)

	// confirm set
	checkValue, err := runtime.ctx.storage.GetChildStorage(storageKey, childKey)
	require.NoError(t, err)
	require.Equal(t, value, checkValue)

	// call wasm function to kill child storage
	testDelete, ok := runtime.vm.Exports["test_ext_kill_child_storage"]
	if !ok {
		t.Fatal("could not find exported function")
	}

	_, err = testDelete(int32(storageKeyPtr), int32(storageKeyLen))
	require.NoError(t, err)

	// confirm value is deleted
	checkDelete, err := runtime.ctx.storage.GetChildStorage(storageKey, childKey)
	require.EqualError(t, err, "child trie does not exist at key :child_storage:default:"+string(storageKey))
	require.Equal(t, []byte(nil), checkDelete)
}

func Test_ext_clear_child_storage(t *testing.T) {
	runtime := NewTestRuntime(t, TEST_RUNTIME)
	mem := runtime.vm.Memory.Data()
	// set child storage
	storageKey := []byte("childstore1")
	childKey := []byte("key1")
	value := []byte("value")
	err := runtime.ctx.storage.SetChild(storageKey, trie.NewEmptyTrie())
	require.Nil(t, err)

	storageKeyLen := uint32(len(storageKey))
	storageKeyPtr, err := runtime.malloc(storageKeyLen)
	require.NoError(t, err)

	childKeyLen := uint32(len(childKey))
	childKeyPtr, err := runtime.malloc(childKeyLen)
	require.NoError(t, err)

	valueLen := uint32(len(value))
	valuePtr, err := runtime.malloc(valueLen)
	require.NoError(t, err)

	copy(mem[storageKeyPtr:storageKeyPtr+storageKeyLen], storageKey)
	copy(mem[childKeyPtr:childKeyPtr+childKeyLen], childKey)
	copy(mem[valuePtr:valuePtr+valueLen], value)

	// call wasm function to set child storage
	testFunc, ok := runtime.vm.Exports["test_ext_set_child_storage"]
	if !ok {
		t.Fatal("could not find exported function")
	}

	_, err = testFunc(int32(storageKeyPtr), int32(storageKeyLen), int32(childKeyPtr), int32(childKeyLen), int32(valuePtr), int32(valueLen))
	require.Nil(t, err)

	// confirm set
	checkValue, err := runtime.ctx.storage.GetChildStorage(storageKey, childKey)
	require.NoError(t, err)
	require.Equal(t, value, checkValue)

	// call wasm function to clear child storage
	testClear, ok := runtime.vm.Exports["test_ext_clear_child_storage"]
	if !ok {
		t.Fatal("could not find exported function")
	}

	_, err = testClear(int32(storageKeyPtr), int32(storageKeyLen), int32(childKeyPtr), int32(childKeyLen))
	require.NoError(t, err)

	// confirm value is deleted
	checkDelete, err := runtime.ctx.storage.GetChildStorage(storageKey, childKey)
	require.NoError(t, err)
	require.Equal(t, []byte(nil), checkDelete)
}

func Test_ext_get_allocated_child_storage(t *testing.T) {
	runtime := NewTestRuntime(t, TEST_RUNTIME)
	mem := runtime.vm.Memory.Data()

	// set child storage
	storageKey := []byte("childstore1")
	childKey := []byte("key1")
	err := runtime.ctx.storage.SetChild(storageKey, trie.NewEmptyTrie())
	require.Nil(t, err)

	storageKeyLen := uint32(len(storageKey))
	storageKeyPtr, err := runtime.malloc(storageKeyLen)
	require.NoError(t, err)

	childKeyLen := uint32(len(childKey))
	childKeyPtr, err := runtime.malloc(childKeyLen)
	require.NoError(t, err)

	copy(mem[storageKeyPtr:storageKeyPtr+storageKeyLen], storageKey)
	copy(mem[childKeyPtr:childKeyPtr+childKeyLen], childKey)

	// call wasm function to get child value (should be not found since we haven't set it yet)
	getValueFunc, ok := runtime.vm.Exports["test_ext_get_allocated_child_storage"]
	if !ok {
		t.Fatal("could not find exported function")
	}

	writtenOut, err := runtime.malloc(4)
	require.NoError(t, err)
	res, err := getValueFunc(int32(storageKeyPtr), int32(storageKeyLen), int32(childKeyPtr), int32(childKeyLen), int32(writtenOut))
	require.NoError(t, err)
	require.Equal(t, []byte{0xff, 0xff, 0xff, 0xff}, mem[writtenOut:writtenOut+4])
	require.Equal(t, int32(0), res.ToI32())

	// store the child value
	value := []byte("value")
	valueLen := uint32(len(value))
	valuePtr, err := runtime.malloc(valueLen)
	require.NoError(t, err)
	copy(mem[valuePtr:valuePtr+valueLen], value)

	// call wasm function to set child storage
	setValueFunc, ok := runtime.vm.Exports["test_ext_set_child_storage"]
	if !ok {
		t.Fatal("could not find exported function")
	}

	_, err = setValueFunc(int32(storageKeyPtr), int32(storageKeyLen), int32(childKeyPtr), int32(childKeyLen), int32(valuePtr), int32(valueLen))
	require.Nil(t, err)

	// call wasm function to check for value, this should be set now
	res, err = getValueFunc(int32(storageKeyPtr), int32(storageKeyLen), int32(childKeyPtr), int32(childKeyLen), int32(writtenOut))
	require.NoError(t, err)
	require.Equal(t, []byte{0x5, 0x0, 0x0, 0x0}, mem[writtenOut:writtenOut+4])
	require.Equal(t, value, mem[res.ToI32():res.ToI32()+5])
}
