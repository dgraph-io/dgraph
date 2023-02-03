/*
 * Copyright 2017-2022 Dgraph Labs, Inc. and Contributors
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
	"strings"

	"github.com/dgraph-io/dgraph/graphql/admin"
	"github.com/dgraph-io/dgraph/graphql/schema"
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

func getAdminMux() *http.ServeMux {
	adminMux := http.NewServeMux()
	adminMux.Handle("/admin/schema", adminAuthHandler(http.HandlerFunc(adminSchemaHandler)))
	adminMux.Handle("/admin/schema/validate", schemaValidateHandler())
	adminMux.Handle("/admin/shutdown", allowedMethodsHandler(allowedMethods{http.MethodGet: true},
		adminAuthHandler(http.HandlerFunc(shutDownHandler))))
	adminMux.Handle("/admin/draining", allowedMethodsHandler(allowedMethods{
		http.MethodPut:  true,
		http.MethodPost: true,
	}, adminAuthHandler(http.HandlerFunc(drainingHandler))))
	adminMux.Handle("/admin/config/cache_mb", allowedMethodsHandler(allowedMethods{
		http.MethodGet: true,
		http.MethodPut: true,
	}, adminAuthHandler(http.HandlerFunc(memoryLimitHandler))))
	return adminMux
}

func schemaValidateHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sch := readRequest(w, r)
		w.Header().Set("Content-Type", "application/json")

		err := admin.SchemaValidate(string(sch))
		if err == nil {
			w.WriteHeader(http.StatusOK)
			x.SetStatus(w, "success", "Schema is valid")
			return
		}

		w.WriteHeader(http.StatusBadRequest)
		errs := strings.Split(strings.TrimSpace(err.Error()), "\n")
		x.SetStatusWithErrors(w, x.ErrorInvalidRequest, errs)
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
	if resp := resolveWithAdminServer(gqlReq, r, adminServer); len(resp.Errors) != 0 {
		x.SetStatus(w, resp.Errors[0].Message, "draining mode request failed.")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	x.Check2(w.Write([]byte(fmt.Sprintf(`{"code": "Success",`+
		`"message": "draining mode has been set to %v"}`, enable))))
}

func shutDownHandler(w http.ResponseWriter, r *http.Request) {
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

	if resp := resolveWithAdminServer(gqlReq, r, adminServer); len(resp.Errors) != 0 {
		x.SetStatus(w, resp.Errors[0].Message, "Shutdown failed.")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	x.Check2(w.Write([]byte(`{"code": "Success", "message": "Server is shutting down"}`)))
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
	gqlReq := &schema.Request{
		Query: `
		mutation config($cacheMb: Float) {
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

func memoryLimitGetHandler(w http.ResponseWriter, r *http.Request) {
	gqlReq := &schema.Request{
		Query: `
		query {
		  config {
			cacheMb
		  }
		}`,
	}
	resp := resolveWithAdminServer(gqlReq, r, adminServer)
	if len(resp.Errors) != 0 {
		x.SetStatus(w, resp.Errors[0].Message, "Get cache_mb failed")
		return
	}
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
