/*
 * Copyright 2017-2018 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package alpha

import (
	"context"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"strconv"

	"github.com/dgraph-io/dgraph/edgraph"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/worker"
	"github.com/dgraph-io/dgraph/x"
	"github.com/golang/glog"
)

// adminAuthOptions are used by adminAuthHandler
type adminAuthOptions struct {
	allowedMethods     map[string]bool
	skipIpWhitelisting bool
	skipPoormansAuth   bool
	skipGuardianAuth   bool
}

// hasPoormansAuth check if poorman's auth is required and if so whether the given http request has
// poorman's auth in it or not
func hasPoormansAuth(r *http.Request) bool {
	if worker.Config.AuthToken != "" && worker.Config.AuthToken != r.Header.Get(
		"X-Dgraph-AuthToken") {
		return false
	}
	return true
}

// adminAuthHandler does some standard checks for admin endpoints.
// It returns if something is wrong. Otherwise, it lets the given handler serve the request.
func adminAuthHandler(handler http.Handler, options adminAuthOptions) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := options.allowedMethods[r.Method]; !ok {
			x.SetStatus(w, x.ErrorInvalidMethod, "Invalid method")
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		if !options.skipIpWhitelisting {
			ip, _, err := net.SplitHostPort(r.RemoteAddr)
			if err != nil || !x.IsIpWhitelisted(ip) {
				x.SetStatus(w, x.ErrorUnauthorized, fmt.Sprintf("Request from IP: %v", ip))
				return
			}
		}

		if !options.skipPoormansAuth && !hasPoormansAuth(r) {
			x.SetStatus(w, x.ErrorUnauthorized, "Invalid X-Dgraph-AuthToken")
			return
		}

		if !options.skipGuardianAuth {
			err := edgraph.AuthorizeGuardians(x.AttachAccessJwt(context.Background(), r))
			if err != nil {
				x.SetStatus(w, x.ErrorUnauthorized, err.Error())
				return
			}
		}

		handler.ServeHTTP(w, r)
	})
}

func drainingHandler(w http.ResponseWriter, r *http.Request) {
	enableStr := r.URL.Query().Get("enable")

	enable, err := strconv.ParseBool(enableStr)
	if err != nil {
		x.SetStatus(w, x.ErrorInvalidRequest,
			"Found invalid value for the enable parameter")
		return
	}

	x.UpdateDrainingMode(enable)
	_, err = w.Write([]byte(fmt.Sprintf(`{"code": "Success",`+
		`"message": "draining mode has been set to %v"}`, enable)))
	if err != nil {
		glog.Errorf("Failed to write response: %v", err)
	}
}

func shutDownHandler(w http.ResponseWriter, r *http.Request) {
	close(worker.ShutdownCh)
	w.Header().Set("Content-Type", "application/json")
	x.Check2(w.Write([]byte(`{"code": "Success", "message": "Server is shutting down"}`)))
}

func exportHandler(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		x.SetHttpStatus(w, http.StatusBadRequest, "Parse of export request failed.")
		return
	}

	format := worker.DefaultExportFormat
	if vals, ok := r.Form["format"]; ok {
		if len(vals) > 1 {
			x.SetHttpStatus(w, http.StatusBadRequest,
				"Only one export format may be specified.")
			return
		}
		format = worker.NormalizeExportFormat(vals[0])
		if format == "" {
			x.SetHttpStatus(w, http.StatusBadRequest, "Invalid export format.")
			return
		}
	}
	if err := worker.ExportOverNetwork(context.Background(), format); err != nil {
		x.SetStatus(w, err.Error(), "Export failed.")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	x.Check2(w.Write([]byte(`{"code": "Success", "message": "Export completed."}`)))
}

func memoryLimitHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		memoryLimitGetHandler(w, r)
	case http.MethodPut:
		memoryLimitPutHandler(w, r)
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

	if err := worker.UpdateLruMb(memoryMB); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, err.Error())
		return
	}
	w.WriteHeader(http.StatusOK)
}

func memoryLimitGetHandler(w http.ResponseWriter, r *http.Request) {
	posting.Config.Lock()
	memoryMB := posting.Config.AllottedMemory
	posting.Config.Unlock()

	if _, err := fmt.Fprintln(w, memoryMB); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
