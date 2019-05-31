package runtime

import (
	"bytes"
	"errors"
	"path/filepath"

	scale "github.com/ChainSafe/gossamer/codec"
	exec "github.com/perlin-network/life/exec"
	"io/ioutil"
)

var (
	DEFAULT_MEMORY_PAGES         = 4096
	DEFAULT_TABLE_SIZE           = 655360
	DEFAULT_MAX_CALL_STACK_DEPTH = 0
)

type Runtime struct {
	vm *exec.VirtualMachine
	// TODO: memory management on top of wasm memory buffer
}

type Version struct {
	Spec_name         []byte
	Impl_name         []byte
	Authoring_version int32
	Spec_version      int32
	Impl_version      int32
}

func NewRuntime(fp string) (*Runtime, error) {
	input, err := ioutil.ReadFile(filepath.Clean(fp))
	if err != nil {
		return nil, err
	}

	vm, err := exec.NewVirtualMachine(input, exec.VMConfig{
		DefaultMemoryPages: DEFAULT_MEMORY_PAGES,
		DefaultTableSize:   DEFAULT_TABLE_SIZE,
		MaxCallStackDepth:  DEFAULT_MAX_CALL_STACK_DEPTH,
	}, &Resolver{}, nil)

	return &Runtime{
		vm: vm,
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
		
	switch function {
	case "Core_version":
		return decodeToInterface(returnData, &Version{})
	case "Core_authorities":
		return nil, nil
	case "Core_execute_block":
		return nil, nil
	case "Core_initialise_block":
		return nil, nil
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
