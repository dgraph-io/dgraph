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
	"net/http"
	"os"
	"sync"

	"github.com/ChainSafe/gossamer/dot/rpc/modules"
	log "github.com/ChainSafe/log15"
	"github.com/gorilla/mux"
	"github.com/gorilla/rpc/v2"
	"github.com/gorilla/websocket"
)

// HTTPServer gateway for RPC server
type HTTPServer struct {
	logger       log.Logger
	rpcServer    *rpc.Server // Actual RPC call handler
	serverConfig *HTTPServerConfig
	wsConns      []*WSConn
}

// HTTPServerConfig configures the HTTPServer
type HTTPServerConfig struct {
	LogLvl              log.Lvl
	BlockAPI            modules.BlockAPI
	StorageAPI          modules.StorageAPI
	NetworkAPI          modules.NetworkAPI
	CoreAPI             modules.CoreAPI
	BlockProducerAPI    modules.BlockProducerAPI
	RuntimeAPI          modules.RuntimeAPI
	TransactionQueueAPI modules.TransactionStateAPI
	RPCAPI              modules.RPCAPI
	SystemAPI           modules.SystemAPI
	Host                string
	RPCPort             uint32
	WSEnabled           bool
	WSPort              uint32
	Modules             []string
}

// WSConn struct to hold WebSocket Connection references
type WSConn struct {
	wsconn             *websocket.Conn
	mu                 sync.Mutex
	blockSubChannels   map[int]byte
	storageSubChannels map[int]byte
	qtyListeners       int
	subscriptions      map[int]Listener
	storageAPI         modules.StorageAPI
	blockAPI           modules.BlockAPI
}

var logger log.Logger

// NewHTTPServer creates a new http server and registers an associated rpc server
func NewHTTPServer(cfg *HTTPServerConfig) *HTTPServer {
	logger = log.New("pkg", "rpc")
	h := log.StreamHandler(os.Stdout, log.TerminalFormat())
	h = log.CallerFileHandler(h)
	logger.SetHandler(log.LvlFilterHandler(cfg.LogLvl, h))

	server := &HTTPServer{
		logger:       logger,
		rpcServer:    rpc.NewServer(),
		serverConfig: cfg,
	}

	server.RegisterModules(cfg.Modules)
	return server
}

// RegisterModules registers the RPC services associated with the given API modules
func (h *HTTPServer) RegisterModules(mods []string) {

	for _, mod := range mods {
		h.logger.Debug("Enabling rpc module", "module", mod)
		var srvc interface{}
		switch mod {
		case "system":
			srvc = modules.NewSystemModule(h.serverConfig.NetworkAPI, h.serverConfig.SystemAPI)
		case "author":
			srvc = modules.NewAuthorModule(h.logger, h.serverConfig.CoreAPI, h.serverConfig.RuntimeAPI, h.serverConfig.TransactionQueueAPI)
		case "chain":
			srvc = modules.NewChainModule(h.serverConfig.BlockAPI)
		case "state":
			srvc = modules.NewStateModule(h.serverConfig.NetworkAPI, h.serverConfig.StorageAPI, h.serverConfig.CoreAPI)
		case "rpc":
			srvc = modules.NewRPCModule(h.serverConfig.RPCAPI)
		case "dev":
			srvc = modules.NewDevModule(h.serverConfig.BlockProducerAPI, h.serverConfig.NetworkAPI)
		default:
			h.logger.Warn("Unrecognized module", "module", mod)
			continue
		}

		err := h.rpcServer.RegisterService(srvc, mod)

		if err != nil {
			h.logger.Warn("Failed to register module", "mod", mod, "err", err)
		}

		h.serverConfig.RPCAPI.BuildMethodNames(srvc, mod)
	}
}

// Start registers the rpc handler function and starts the rpc http and websocket server
func (h *HTTPServer) Start() error {
	// use our DotUpCodec which will capture methods passed in json as _x that is
	//  underscore followed by lower case letter, instead of default RPC calls which
	//  use . followed by Upper case letter
	h.rpcServer.RegisterCodec(NewDotUpCodec(), "application/json")
	h.rpcServer.RegisterCodec(NewDotUpCodec(), "application/json;charset=UTF-8")

	h.logger.Info("Starting HTTP Server...", "host", h.serverConfig.Host, "port", h.serverConfig.RPCPort)
	r := mux.NewRouter()
	r.Handle("/", h.rpcServer)
	go func() {
		err := http.ListenAndServe(fmt.Sprintf(":%d", h.serverConfig.RPCPort), r)
		if err != nil {
			h.logger.Error("http error", "err", err)
		}
	}()

	if !h.serverConfig.WSEnabled {
		return nil
	}

	h.logger.Info("Starting WebSocket Server...", "host", h.serverConfig.Host, "port", h.serverConfig.WSPort)
	ws := mux.NewRouter()
	ws.Handle("/", h)
	go func() {
		err := http.ListenAndServe(fmt.Sprintf(":%d", h.serverConfig.WSPort), ws)
		if err != nil {
			h.logger.Error("http error", "err", err)
		}
	}()

	return nil
}

// Stop stops the server
func (h *HTTPServer) Stop() error {
	if h.serverConfig.WSEnabled {
		// close all channels and websocket connections
		for _, conn := range h.wsConns {
			for _, sub := range conn.subscriptions {
				switch v := sub.(type) {
				case *StorageChangeListener:
					h.serverConfig.StorageAPI.UnregisterStorageChangeChannel(v.chanID)
					close(v.channel)
				case *BlockListener:
					h.serverConfig.BlockAPI.UnregisterImportedChannel(v.chanID)
					close(v.channel)
				}
			}

			err := conn.wsconn.Close()
			if err != nil {
				h.logger.Error("error closing websocket connection", "error", err)
			}
		}
	}
	return nil
}
