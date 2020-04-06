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

package rpc

import (
	"fmt"

	"github.com/gorilla/mux"
	"github.com/gorilla/rpc/v2"

	"net/http"

	"github.com/ChainSafe/gossamer/dot/rpc/modules"

	log "github.com/ChainSafe/log15"
)

// HTTPServer gateway for RPC server
type HTTPServer struct {
	rpcServer    *rpc.Server // Actual RPC call handler
	serverConfig *HTTPServerConfig
}

// HTTPServerConfig configures the HTTPServer
type HTTPServerConfig struct {
	BlockAPI            modules.BlockAPI
	StorageAPI          modules.StorageAPI
	NetworkAPI          modules.NetworkAPI
	CoreAPI             modules.CoreAPI
	TransactionQueueAPI modules.TransactionQueueAPI
	Host                string
	Port                uint32
	Modules             []string
}

// NewHTTPServer creates a new http server and registers an associated rpc server
func NewHTTPServer(cfg *HTTPServerConfig) *HTTPServer {
	server := &HTTPServer{
		rpcServer:    rpc.NewServer(),
		serverConfig: cfg,
	}

	server.RegisterModules(cfg.Modules)
	return server
}

// RegisterModules registers the RPC services associated with the given API modules
func (h *HTTPServer) RegisterModules(mods []string) {

	for _, mod := range mods {
		log.Debug("[rpc] Enabling rpc module", "module", mod)
		var srvc interface{}
		switch mod {
		case "system":
			srvc = modules.NewSystemModule(h.serverConfig.NetworkAPI)
		case "author":
			srvc = modules.NewAuthorModule(h.serverConfig.CoreAPI, h.serverConfig.TransactionQueueAPI)
		case "chain":
			srvc = modules.NewChainModule(h.serverConfig.BlockAPI)
		default:
			log.Warn("[rpc] Unrecognized module", "module", mod)
			continue
		}

		err := h.rpcServer.RegisterService(srvc, mod)

		if err != nil {
			log.Warn("[rpc] Failed to register module", "mod", mod, "err", err)
		}

	}
}

// Start registers the rpc handler function and starts the server listening on `h.port`
func (h *HTTPServer) Start() error {
	// use our DotUpCodec which will capture methods passed in json as _x that is
	//  underscore followed by lower case letter, instead of default RPC calls which
	//  use . followed by Upper case letter
	h.rpcServer.RegisterCodec(NewDotUpCodec(), "application/json")
	h.rpcServer.RegisterCodec(NewDotUpCodec(), "application/json;charset=UTF-8")

	log.Debug("[rpc] Starting HTTP Server...", "host", h.serverConfig.Host, "port", h.serverConfig.Port)
	r := mux.NewRouter()
	r.Handle("/", h.rpcServer)
	go func() {
		err := http.ListenAndServe(fmt.Sprintf(":%d", h.serverConfig.Port), r)
		if err != nil {
			log.Error("[rpc] http error", "err", err)
		}
	}()

	return nil
}

// Stop stops the server
func (h *HTTPServer) Stop() error {
	return nil
}
