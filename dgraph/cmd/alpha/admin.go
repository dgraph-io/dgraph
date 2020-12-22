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
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"

	"github.com/dgraph-io/dgraph/graphql/schema"
	"github.com/dgraph-io/dgraph/graphql/web"

	"github.com/dgraph-io/dgraph/worker"
	"github.com/dgraph-io/dgraph/x"
)

type allowedMethods map[string]bool

// hasPoormansAuth checks if poorman's auth is required and if so whether the given http request has
// poorman's auth in it or not
func hasPoormansAuth(r *http.Request) bool {
	if worker.Config.AuthToken != "" && worker.Config.AuthToken != r.Header.Get(
		"X-Dgraph-AuthToken") {
		return false
	}
	return true
}

func allowedMethodsHandler(allowedMethods allowedMethods, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := allowedMethods[r.Method]; !ok {
			x.AddCorsHeaders(w)
			if r.Method == http.MethodOptions {
				return
			}
			x.SetStatus(w, x.ErrorInvalidMethod, "Invalid method")
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// adminAuthHandler does some standard checks for admin endpoints.
// It returns if something is wrong. Otherwise, it lets the given handler serve the request.
func adminAuthHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !hasPoormansAuth(r) {
			x.AddCorsHeaders(w)
			x.SetStatus(w, x.ErrorUnauthorized, "Invalid X-Dgraph-AuthToken")
			return
		}

		next.ServeHTTP(w, r)
	})
}

func drainingHandler(w http.ResponseWriter, r *http.Request, adminServer web.IServeGraphQL) {
	enableStr := r.URL.Query().Get("enable")

	enable, err := strconv.ParseBool(enableStr)
	if err != nil {
		x.SetStatus(w, x.ErrorInvalidRequest,
			"Found invalid value for the enable parameter")
		return
	}

	gqlReq := &schema.Request{
		Query: `
		mutation draining($enable: Boolean) {
		  draining(enable: $enable) {
			response {
			  code
			}
		  }
		}`,
		Variables: map[string]interface{}{"enable": enable},
	}
	_ = resolveWithAdminServer(gqlReq, r, adminServer)
	w.Header().Set("Content-Type", "application/json")
	x.Check2(w.Write([]byte(fmt.Sprintf(`{"code": "Success",`+
		`"message": "draining mode has been set to %v"}`, enable))))
}

func shutDownHandler(w http.ResponseWriter, r *http.Request, adminServer web.IServeGraphQL) {
	gqlReq := &schema.Request{
		Query: `
		mutation {
			shutdown {
				response {
					code
				}
			}
		}`,
	}
	_ = resolveWithAdminServer(gqlReq, r, adminServer)
	w.Header().Set("Content-Type", "application/json")
	x.Check2(w.Write([]byte(`{"code": "Success", "message": "Server is shutting down"}`)))
}

func exportHandler(w http.ResponseWriter, r *http.Request, adminServer web.IServeGraphQL) {
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

	gqlReq := &schema.Request{
		Query: `
		mutation export($format: String) {
		  export(input: {format: $format}) {
			response {
			  code
			}
		  }
		}`,
		Variables: map[string]interface{}{},
	}
	resp := resolveWithAdminServer(gqlReq, r, adminServer)
	if len(resp.Errors) != 0 {
		x.SetStatus(w, resp.Errors[0].Message, "Export failed.")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	x.Check2(w.Write([]byte(`{"code": "Success", "message": "Export completed."}`)))
}

func memoryLimitHandler(w http.ResponseWriter, r *http.Request, adminServer web.IServeGraphQL) {
	switch r.Method {
	case http.MethodGet:
		memoryLimitGetHandler(w, r, adminServer)
	case http.MethodPut:
		memoryLimitPutHandler(w, r, adminServer)
	}
}

func memoryLimitPutHandler(w http.ResponseWriter, r *http.Request, adminServer web.IServeGraphQL) {
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
	gqlReq := &schema.Request{
		Query: `
		mutation config($cacheMb: Int) {
		  config(input: {cacheMb: $cacheMb}) {
			response {
			  code
			}
		  }
		}`,
		Variables: map[string]interface{}{"cacheMb": memoryMB},
	}
	resp := resolveWithAdminServer(gqlReq, r, adminServer)

	if len(resp.Errors) != 0 {
		w.WriteHeader(http.StatusBadRequest)
		x.Check2(fmt.Fprint(w, resp.Errors[0].Message))
		return
	}
	w.WriteHeader(http.StatusOK)
}

func memoryLimitGetHandler(w http.ResponseWriter, r *http.Request, adminServer web.IServeGraphQL) {
	gqlReq := &schema.Request{
		Query: `
		query {
		  config {
			cacheMb
		  }
		}`,
	}
	resp := resolveWithAdminServer(gqlReq, r, adminServer)
	var data struct {
		Config struct {
			CacheMb float64
		}
	}
	x.Check(json.Unmarshal(resp.Data.Bytes(), &data))

	if _, err := fmt.Fprintln(w, data.Config.CacheMb); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
