/*
 * Copyright (C) 2017 Dgraph Labs, Inc. and Contributors
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 */

package server

import (
	"context"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"strconv"

	"github.com/dgraph-io/dgraph/edgraph"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/worker"
	"github.com/dgraph-io/dgraph/x"
)

// handlerInit does some standard checks. Returns false if something is wrong.
func handlerInit(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodGet {
		x.SetStatus(w, x.ErrorInvalidMethod, "Invalid method")
		return false
	}

	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil || !net.ParseIP(ip).IsLoopback() {
		x.SetStatus(w, x.ErrorUnauthorized, fmt.Sprintf("Request from IP: %v", ip))
		return false
	}
	return true
}

func shutDownHandler(w http.ResponseWriter, r *http.Request) {
	if !handlerInit(w, r) {
		return
	}

	shutdownServer()
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"code": "Success", "message": "Server is shutting down"}`))
}

func shutdownServer() {
	x.Printf("Got clean exit request")
	sdCh <- os.Interrupt
}

func exportHandler(w http.ResponseWriter, r *http.Request) {
	if !handlerInit(w, r) {
		return
	}
	ctx := context.Background()
	// Export logic can be moved to dgraphzero.
	if err := worker.ExportOverNetwork(ctx); err != nil {
		x.SetStatus(w, err.Error(), "Export failed.")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"code": "Success", "message": "Export completed."}`))
}

func memoryLimitHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		memoryLimitGetHandler(w, r)
	case http.MethodPut:
		memoryLimitPutHandler(w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func memoryLimitPutHandler(w http.ResponseWriter, r *http.Request) {
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	memoryMB, err := strconv.ParseFloat(string(body), 64)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if memoryMB < edgraph.MinAllottedMemory {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "memory_mb must be at least %.0f\n", edgraph.MinAllottedMemory)
		return
	}

	posting.Config.Mu.Lock()
	posting.Config.AllottedMemory = memoryMB
	posting.Config.Mu.Unlock()
	w.WriteHeader(http.StatusOK)
}

func memoryLimitGetHandler(w http.ResponseWriter, r *http.Request) {
	posting.Config.Mu.Lock()
	memoryMB := posting.Config.AllottedMemory
	posting.Config.Mu.Unlock()

	if _, err := fmt.Fprintln(w, memoryMB); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
