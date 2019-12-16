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

package module

import (
	log "github.com/ChainSafe/log15"
)

type RuntimeModule struct {
	RuntimeApi RuntimeApi
}

// RuntimeApi is the interface for the runtime package
type RuntimeApi interface {
	Chain() string
	Name() string
	Properties() string
	Version() string
}

// RuntimeModule implements RuntimeApi
func NewRuntimeModule(runtimeApi RuntimeApi) *RuntimeModule {
	return &RuntimeModule{runtimeApi}
}

// Chain returns runtime Chain()
func (m *RuntimeModule) Chain() string {
	log.Debug("[rpc] Executing System.Chain", "params", nil)
	return m.RuntimeApi.Chain()
}

// Name returns runtime Name()
func (m *RuntimeModule) Name() string {
	log.Debug("[rpc] Executing System.Name", "params", nil)
	return m.RuntimeApi.Name()
}

// Properties returns runtime Properties()
func (m *RuntimeModule) Properties() string {
	log.Debug("[rpc] Executing System.Properties", "params", nil)
	return m.RuntimeApi.Properties()
}

// Version returns runtime Version()
func (m *RuntimeModule) Version() string {
	log.Debug("[rpc] Executing System.Version", "params", nil)
	return m.RuntimeApi.Version()
}
