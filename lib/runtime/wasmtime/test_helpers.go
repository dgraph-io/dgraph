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

package wasmtime

import (
	"path/filepath"
	"testing"

	database "github.com/ChainSafe/chaindb"
	"github.com/ChainSafe/gossamer/lib/keystore"
	"github.com/ChainSafe/gossamer/lib/runtime"
	"github.com/ChainSafe/gossamer/lib/trie"

	log "github.com/ChainSafe/log15"
	"github.com/bytecodealliance/wasmtime-go"
	"github.com/stretchr/testify/require"
)

// NewTestInstance will create a new runtime instance using the given target runtime
func NewTestInstance(t *testing.T, targetRuntime string) *Instance {
	return NewTestInstanceWithTrie(t, targetRuntime, nil, log.LvlTrace)
}

// NewTestInstanceWithTrie will create a new runtime (polkadot/test) with the supplied trie as the storage
func NewTestInstanceWithTrie(t *testing.T, targetRuntime string, tt *trie.Trie, lvl log.Lvl) *Instance {
	testRuntimeFilePath, testRuntimeURL := runtime.GetRuntimeVars(targetRuntime)
	importsFunc := GetRuntimeImports(t, targetRuntime)

	_, err := runtime.GetRuntimeBlob(testRuntimeFilePath, testRuntimeURL)
	require.Nil(t, err, "Fail: could not get runtime", "targetRuntime", targetRuntime)

	fp, err := filepath.Abs(testRuntimeFilePath)
	require.Nil(t, err, "could not create testRuntimeFilePath", "targetRuntime", targetRuntime)

	ns := runtime.NodeStorage{
		LocalStorage:      database.NewMemDatabase(),
		PersistentStorage: database.NewMemDatabase(), // we're using a local storage here since this is a test runtime
	}
	cfg := &Config{
		Imports: importsFunc,
	}
	cfg.Storage = runtime.NewTestRuntimeStorage(t, tt)
	cfg.Keystore = keystore.NewGenericKeystore("test")
	cfg.LogLvl = lvl
	cfg.NodeStorage = ns
	cfg.Network = new(runtime.TestRuntimeNetwork)

	r, err := NewInstanceFromFile(fp, cfg)
	require.Nil(t, err, "Got error when trying to create new VM", "targetRuntime", targetRuntime)
	require.NotNil(t, r, "Could not create new VM instance", "targetRuntime", targetRuntime)
	return r
}

// GetRuntimeImports ...
func GetRuntimeImports(t *testing.T, targetRuntime string) func(*wasmtime.Store) []*wasmtime.Extern {
	var imports func(*wasmtime.Store) []*wasmtime.Extern

	switch targetRuntime {
	case runtime.NODE_RUNTIME:
		imports = ImportsNodeRuntime
	default:
		t.Fatalf("unknown runtime type %s", targetRuntime)
	}

	return imports
}
