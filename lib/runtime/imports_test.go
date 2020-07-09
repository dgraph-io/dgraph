package runtime

import (
	"bytes"
	"testing"

	"github.com/ChainSafe/gossamer/lib/common"

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
