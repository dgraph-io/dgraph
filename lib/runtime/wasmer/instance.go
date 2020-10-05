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

package wasmer

import (
	"fmt"
	"os"
	"sync"

	"github.com/ChainSafe/gossamer/lib/keystore"
	"github.com/ChainSafe/gossamer/lib/runtime"

	log "github.com/ChainSafe/log15"
	wasm "github.com/wasmerio/go-ext-wasm/wasmer"
)

var memory, memErr = wasm.NewMemory(17, 0)
var logger = log.New("pkg", "runtime")

// Config represents a wasmer configuration
type Config struct {
	Storage     runtime.Storage
	Keystore    *keystore.GenericKeystore
	Imports     func() (*wasm.Imports, error)
	LogLvl      log.Lvl
	Role        byte
	NodeStorage runtime.NodeStorage
	Network     runtime.BasicNetwork
}

// Instance represents a go-wasmer instance
type Instance struct {
	vm    wasm.Instance
	ctx   *runtime.Context
	mutex sync.Mutex
}

// NewInstanceFromFile instantiates a runtime from a .wasm file
func NewInstanceFromFile(fp string, cfg *Config) (*Instance, error) {
	// Reads the WebAssembly module as bytes.
	bytes, err := wasm.ReadBytes(fp)
	if err != nil {
		return nil, err
	}

	return NewInstance(bytes, cfg)
}

// NewInstance instantiates a runtime from raw wasm bytecode
func NewInstance(code []byte, cfg *Config) (*Instance, error) {
	// if cfg.LogLvl set to < 0, then don't change package log level
	if cfg.LogLvl >= 0 {
		h := log.StreamHandler(os.Stdout, log.TerminalFormat())
		h = log.CallerFileHandler(h)
		logger.SetHandler(log.LvlFilterHandler(cfg.LogLvl, h))
	}

	imports, err := cfg.Imports()
	if err != nil {
		return nil, err
	}

	// Instantiates the WebAssembly module.
	instance, err := wasm.NewInstanceWithImports(code, imports)
	if err != nil {
		return nil, err
	}

	if instance.Memory == nil {
		if memErr != nil {
			return nil, err
		}

		instance.Memory = memory
	}

	memAllocator := runtime.NewAllocator(instance.Memory.Data(), 0)

	validator := false
	if cfg.Role == byte(4) {
		validator = true
	}

	runtimeCtx := &runtime.Context{
		Storage:     cfg.Storage,
		Allocator:   memAllocator,
		Keystore:    cfg.Keystore,
		Validator:   validator,
		NodeStorage: cfg.NodeStorage,
		Network:     cfg.Network,
	}

	logger.Debug("NewInstance", "runtimeCtx", runtimeCtx)
	instance.SetContextData(runtimeCtx)

	r := Instance{
		vm:  instance,
		ctx: runtimeCtx,
	}

	return &r, nil
}

// SetContext sets the runtime's storage. It should be set before calls to the below functions.
func (r *Instance) SetContext(s runtime.Storage) {
	r.ctx.Storage = s
}

// Stop func
func (r *Instance) Stop() {
	r.vm.Close()
}

// Store func
func (r *Instance) store(data []byte, location int32) {
	mem := r.vm.Memory.Data()
	copy(mem[location:location+int32(len(data))], data)
}

// Load load
func (r *Instance) load(location, length int32) []byte {
	mem := r.vm.Memory.Data()
	return mem[location : location+length]
}

// Exec func
func (r *Instance) exec(function string, data []byte) ([]byte, error) {
	if r.ctx.Storage == nil {
		return nil, runtime.ErrNilStorage
	}

	ptr, err := r.malloc(uint32(len(data)))
	if err != nil {
		return nil, err
	}

	defer func() {
		err = r.free(ptr)
		if err != nil {
			logger.Error("exec: could not free ptr", "error", err)
		}
	}()

	r.mutex.Lock()
	defer r.mutex.Unlock()

	// Store the data into memory
	r.store(data, int32(ptr))
	datalen := int32(len(data))

	runtimeFunc, ok := r.vm.Exports[function]
	if !ok {
		return nil, fmt.Errorf("could not find exported function %s", function)
	}
	res, err := runtimeFunc(int32(ptr), datalen)
	if err != nil {
		return nil, err
	}
	resi := res.ToI64()

	length := int32(resi >> 32)
	offset := int32(resi)

	rawdata := r.load(offset, length)

	return rawdata, err
}

func (r *Instance) malloc(size uint32) (uint32, error) {
	return r.ctx.Allocator.Allocate(size)
}

func (r *Instance) free(ptr uint32) error {
	return r.ctx.Allocator.Deallocate(ptr)
}

// NodeStorage to get reference to runtime node service
func (r *Instance) NodeStorage() runtime.NodeStorage {
	return r.ctx.NodeStorage
}

// NetworkService to get referernce to runtime network service
func (r *Instance) NetworkService() runtime.BasicNetwork {
	return r.ctx.Network
}
