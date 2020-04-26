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
package rpc

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/ethereum/go-ethereum/log"
	"github.com/gorilla/websocket"
)

// ServeHTTP implemented to handle WebSocket connections
func (h *HTTPServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var upg = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}
	ws, err := upg.Upgrade(w, r, nil)
	if err != nil {
		log.Error("[rpc] websocket upgrade failed", "error", err)
		return
	}
	for {
		rpcHost := fmt.Sprintf("http://%s:%d/", h.serverConfig.Host, h.serverConfig.RPCPort)
		for {
			_, mbytes, err := ws.ReadMessage()
			if err != nil {
				log.Error("[rpc] websocket failed to read message", "error", err)
				return
			}
			log.Trace("[rpc] websocket received", "message", fmt.Sprintf("%s", mbytes))
			client := &http.Client{}
			buf := &bytes.Buffer{}
			_, err = buf.Write(mbytes)
			if err != nil {
				log.Error("[rpc] failed to write message to buffer", "error", err)
				return
			}

			req, err := http.NewRequest("POST", rpcHost, buf)
			if err != nil {
				log.Error("[rpc] failed request to rpc service", "error", err)
				return
			}

			req.Header.Set("Content-Type", "application/json;")

			res, err := client.Do(req)
			if err != nil {
				log.Error("[rpc] websocket error calling rpc", "error", err)
				return
			}

			body, err := ioutil.ReadAll(res.Body)
			if err != nil {
				log.Error("[rpc] error reading response body", "error", err)
				return
			}

			err = res.Body.Close()
			if err != nil {
				log.Error("[rpc] error closing response body", "error", err)
				return
			}
			var wsSend interface{}
			err = json.Unmarshal(body, &wsSend)
			if err != nil {
				log.Error("[rpc] error unmarshal rpc response", "error", err)
				return
			}

			err = ws.WriteJSON(wsSend)
			if err != nil {
				log.Error("[rpc] error writing json response", "error", err)
				return
			}
		}
	}
}
