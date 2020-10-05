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
	"fmt"
	"os"
	"sync"

	"github.com/ChainSafe/gossamer/lib/keystore"
	log "github.com/ChainSafe/log15"
	wasm "github.com/wasmerio/go-ext-wasm/wasmer"
)

var memory, memErr = wasm.NewMemory(17, 0)
var logger = log.New("pkg", "runtime")

// NodeStorageType type to identify offchain storage type
type NodeStorageType byte

// NodeStorageTypePersistent flag to identify offchain storage as persistent (db)
const NodeStorageTypePersistent NodeStorageType = 1

// NodeStorageTypeLocal flog to identify offchain storage as local (memory)
const NodeStorageTypeLocal NodeStorageType = 2

// NodeStorage struct for storage of runtime offchain worker data
type NodeStorage struct {
	LocalStorage      BasicStorage
	PersistentStorage BasicStorage
}

// Ctx struct
type Ctx struct {
	storage     Storage
	allocator   *FreeingBumpHeapAllocator
	keystore    *keystore.GenericKeystore
	validator   bool
	nodeStorage NodeStorage
	network     BasicNetwork
}

// Config represents a runtime configuration
type Config struct {
	Storage     Storage
	Keystore    *keystore.GenericKeystore
	Imports     func() (*wasm.Imports, error)
	LogLvl      log.Lvl
	Role        byte
	NodeStorage NodeStorage
	Network     BasicNetwork
}

// Runtime struct
type Runtime struct {
	vm    wasm.Instance
	ctx   *Ctx
	mutex sync.Mutex
}

// NewRuntimeFromFile instantiates a runtime from a .wasm file
func NewRuntimeFromFile(fp string, cfg *Config) (*Runtime, error) {
	// Reads the WebAssembly module as bytes.
	bytes, err := wasm.ReadBytes(fp)
	if err != nil {
		return nil, err
	}

	return NewRuntime(bytes, cfg)
}

// NewRuntime instantiates a runtime from raw wasm bytecode
func NewRuntime(code []byte, cfg *Config) (*Runtime, error) {
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

	memAllocator := NewAllocator(instance.Memory.Data(), 0)

	validator := false
	if cfg.Role == byte(4) {
		validator = true
	}

	runtimeCtx := &Ctx{
		storage:     cfg.Storage,
		allocator:   memAllocator,
		keystore:    cfg.Keystore,
		validator:   validator,
		nodeStorage: cfg.NodeStorage,
		network:     cfg.Network,
	}

	logger.Debug("NewRuntime", "runtimeCtx", runtimeCtx)
	instance.SetContextData(runtimeCtx)

	r := Runtime{
		vm:  instance,
		ctx: runtimeCtx,
	}

	return &r, nil
}

// Stop func
func (r *Runtime) Stop() {
	r.vm.Close()
}

// Store func
func (r *Runtime) Store(data []byte, location int32) {
	mem := r.vm.Memory.Data()
	copy(mem[location:location+int32(len(data))], data)
}

// Load load
func (r *Runtime) Load(location, length int32) []byte {
	mem := r.vm.Memory.Data()
	return mem[location : location+length]
}

// Exec func
func (r *Runtime) Exec(function string, data []byte) ([]byte, error) {
	if r.ctx.storage == nil {
		return nil, ErrNilStorage
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
	r.Store(data, int32(ptr))
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

	rawdata := r.Load(offset, length)

	return rawdata, err
}

func (r *Runtime) malloc(size uint32) (uint32, error) {
	return r.ctx.allocator.Allocate(size)
}

func (r *Runtime) free(ptr uint32) error {
	return r.ctx.allocator.Deallocate(ptr)
}

// NodeStorage to get reference to runtime node service
func (r *Runtime) NodeStorage() NodeStorage {
	return r.ctx.nodeStorage
}

// NetworkService to get referernce to runtime network service
func (r *Runtime) NetworkService() BasicNetwork {
	return r.ctx.network
}
