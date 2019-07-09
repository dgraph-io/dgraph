package runtime

import (
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"path/filepath"

	scale "github.com/ChainSafe/gossamer/codec"
	trie "github.com/ChainSafe/gossamer/trie"
	log "github.com/ChainSafe/log15"
	exec "github.com/perlin-network/life/exec"
)

var (
	DEFAULT_MEMORY_PAGES         = 4096
	DEFAULT_TABLE_SIZE           = 655360
	DEFAULT_MAX_CALL_STACK_DEPTH = 0
)

type Runtime struct {
	vm *exec.VirtualMachine
	t  *trie.Trie
}

type Version struct {
	Spec_name         []byte
	Impl_name         []byte
	Authoring_version int32
	Spec_version      int32
	Impl_version      int32
}

func NewRuntime(fp string, t *trie.Trie) (*Runtime, error) {
	input, err := ioutil.ReadFile(filepath.Clean(fp))
	if err != nil {
		return nil, err
	}

	vm, err := exec.NewVirtualMachine(input, exec.VMConfig{
		DefaultMemoryPages: DEFAULT_MEMORY_PAGES,
		DefaultTableSize:   DEFAULT_TABLE_SIZE,
		MaxCallStackDepth:  DEFAULT_MAX_CALL_STACK_DEPTH,
	}, &Resolver{t: t}, nil)

	return &Runtime{
		vm: vm,
		t:  t,
	}, err
}

func (r *Runtime) Exec(function string, param1, param2 int64) (interface{}, error) {
	entryID, ok := r.vm.GetFunctionExport(function)
	if !ok {
		return nil, errors.New("entry function not found")
	}

	ret, err := r.vm.Run(entryID, param1, param2)
	if err != nil {
		return nil, err
	}

	// ret is int64; top 4 bytes are the size of the returned data and bottom 4 bytes are
	// the offset in the wasm memory buffer
	size := int32(ret >> 32)
	offset := int32(ret)
	returnData := r.vm.Memory[offset : offset+size]
	log.Debug(fmt.Sprintf("call to %s", function), "returndata", returnData, "returndatalen", len(returnData))

	switch function {
	case "Core_version":
		return decodeToInterface(returnData, &Version{})
	case "Core_authorities":
		authLen, err := scale.Decode(returnData, int64(0))
		if err != nil {
			return nil, err
		}
		t := make([][32]byte, authLen.(int64))
		return decodeToInterface(returnData, &t)
	case "Core_execute_block":
		return nil, nil
	case "Core_initialise_block":
		return returnData, nil
	default:
		return nil, nil
	}
}

func decodeToInterface(in []byte, t interface{}) (interface{}, error) {
	buf := &bytes.Buffer{}
	sd := scale.Decoder{Reader: buf}
	_, err := buf.Write(in)
	if err != nil {
		return nil, err
	}

	output, err := sd.Decode(t)
	return output, err
}
