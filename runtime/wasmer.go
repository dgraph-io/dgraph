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
	"errors"
	"fmt"
	"unsafe"

	scale "github.com/ChainSafe/gossamer/codec"
	"github.com/ChainSafe/gossamer/common"
	allocator "github.com/ChainSafe/gossamer/runtime/allocator"
	trie "github.com/ChainSafe/gossamer/trie"
	log "github.com/ChainSafe/log15"
	wasm "github.com/wasmerio/go-ext-wasm/wasmer"
)

type RuntimeCtx struct {
	trie      *trie.Trie
	allocator *allocator.FreeingBumpHeapAllocator
}

type Runtime struct {
	vm   wasm.Instance
	trie *trie.Trie
}

// NewRuntimeFromFile instantiates a runtime from a .wasm file
func NewRuntimeFromFile(fp string, t *trie.Trie) (*Runtime, error) {
	// Reads the WebAssembly module as bytes.
	bytes, err := wasm.ReadBytes(fp)
	if err != nil {
		return nil, err
	}

	return NewRuntime(bytes, t)
}

// NewRuntime instantiates a runtime from raw wasm bytecode
func NewRuntime(code []byte, t *trie.Trie) (*Runtime, error) {
	if t == nil {
		return nil, errors.New("runtime does not have storage trie")
	}

	imports, err := registerImports()
	if err != nil {
		return nil, err
	}

	// Instantiates the WebAssembly module.
	instance, err := wasm.NewInstanceWithImports(code, imports)
	if err != nil {
		return nil, err
	}

	memAllocator := allocator.NewAllocator(&instance.Memory, 0)

	runtimeCtx := &RuntimeCtx{
		trie:      t,
		allocator: memAllocator,
	}
	// add runtimeCtx to registry
	// lock access to registry to avoid possible concurrent access
	mutex.Lock()
	index := handlers
	handlers++
	if registry == nil {
		registry = make(map[int]RuntimeCtx)
	}
	registry[index] = *runtimeCtx
	mutex.Unlock()

	log.Debug("[NewRuntime]", "index", index)
	log.Debug("[NewRuntime]", "runtimeCtx", runtimeCtx)
	//nolint:gosec
	data := unsafe.Pointer(&index)
	instance.SetContextData(data)

	return &Runtime{
		vm:   instance,
		trie: t,
	}, nil
}

func (r *Runtime) Stop() {
	r.vm.Close()
}

func (r *Runtime) StorageRoot() (common.Hash, error) {
	return r.trie.Hash()
}

func (r *Runtime) Store(data []byte, location int32) {
	mem := r.vm.Memory.Data()
	copy(mem[location:location+int32(len(data))], data)
}

func (r *Runtime) Load(location, length int32) []byte {
	mem := r.vm.Memory.Data()
	return mem[location : location+length]
}

func (r *Runtime) Exec(function string, data, len int32) ([]byte, error) {
	runtimeFunc, ok := r.vm.Exports[function]
	if !ok {
		return nil, errors.New("could not find exported function")
	}
	res, err := runtimeFunc(data, len)
	if err != nil {
		return nil, err
	}
	resi := res.ToI64()

	length := int32(resi >> 32)
	offset := int32(resi)
	fmt.Printf("offset %d length %d\n", offset, length)
	mem := r.vm.Memory.Data()
	rawdata := make([]byte, length)
	copy(rawdata, mem[offset:offset+length])

	return rawdata, err
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
