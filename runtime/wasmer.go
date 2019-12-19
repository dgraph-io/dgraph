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
	"sync"

	scale "github.com/ChainSafe/gossamer/codec"
	"github.com/ChainSafe/gossamer/common"
	"github.com/ChainSafe/gossamer/keystore"
	allocator "github.com/ChainSafe/gossamer/runtime/allocator"
	log "github.com/ChainSafe/log15"
	wasm "github.com/wasmerio/go-ext-wasm/wasmer"
)

type RuntimeCtx struct {
	storage   Storage
	allocator *allocator.FreeingBumpHeapAllocator
	keystore  *keystore.Keystore
}

type Runtime struct {
	vm       wasm.Instance
	storage  Storage
	keystore *keystore.Keystore
	mutex    sync.Mutex
}

// NewRuntimeFromFile instantiates a runtime from a .wasm file
func NewRuntimeFromFile(fp string, s Storage, ks *keystore.Keystore) (*Runtime, error) {
	// Reads the WebAssembly module as bytes.
	bytes, err := wasm.ReadBytes(fp)
	if err != nil {
		return nil, err
	}

	return NewRuntime(bytes, s, ks)
}

// NewRuntime instantiates a runtime from raw wasm bytecode
func NewRuntime(code []byte, s Storage, ks *keystore.Keystore) (*Runtime, error) {
	if s == nil {
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

	memAllocator := allocator.NewAllocator(instance.Memory, 0)

	runtimeCtx := RuntimeCtx{
		storage:   s,
		allocator: memAllocator,
		keystore:  ks,
	}

	log.Debug("[NewRuntime]", "runtimeCtx", runtimeCtx)
	instance.SetContextData(&runtimeCtx)

	r := Runtime{
		vm:       instance,
		storage:  s,
		mutex:    sync.Mutex{},
		keystore: ks,
	}

	return &r, nil
}

func (r *Runtime) Stop() {
	r.vm.Close()
}

// TODO, this should be removed once core is refactored
func (r *Runtime) StorageRoot() (common.Hash, error) {
	return r.storage.StorageRoot()
}

func (r *Runtime) Store(data []byte, location int32) {
	mem := r.vm.Memory.Data()
	copy(mem[location:location+int32(len(data))], data)
}

func (r *Runtime) Load(location, length int32) []byte {
	mem := r.vm.Memory.Data()
	return mem[location : location+length]
}

func (r *Runtime) Exec(function string, loc int32, data []byte) ([]byte, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	// Store the data into memory
	r.Store(data, loc)
	leng := int32(len(data))

	runtimeFunc, ok := r.vm.Exports[function]
	if !ok {
		return nil, fmt.Errorf("could not find exported function %s", function)
	}
	res, err := runtimeFunc(loc, leng)
	if err != nil {
		return nil, err
	}
	resi := res.ToI64()

	length := int32(resi >> 32)
	offset := int32(resi)

	rawdata := r.Load(offset, length)

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
