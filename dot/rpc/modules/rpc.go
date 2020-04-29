// Copyright 2020 ChainSafe Systems (ON) Corp.
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
package modules

import "net/http"

// RPCModule is a RPC module providing access to RPC methods
type RPCModule struct {
	rPCAPI RPCAPI
}

// MethodsResponse struct representing methods
type MethodsResponse struct {
	Methods []string `json:"methods"`
}

// NewRPCModule creates a new RPC api module
func NewRPCModule(rpcapi RPCAPI) *RPCModule {
	return &RPCModule{
		rPCAPI: rpcapi,
	}
}

// Methods responds with list of methods available via RPC call
func (rm *RPCModule) Methods(r *http.Request, req *EmptyRequest, res *MethodsResponse) error {
	res.Methods = rm.rPCAPI.Methods()

	return nil
}
